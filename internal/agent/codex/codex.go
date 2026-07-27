// Package codex implements the agent.Provider contract for the OpenAI Codex CLI.
//
// Codex stores each conversation as a "rollout": one JSON-per-line file under
//
//	$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<local-timestamp>-<uuid>.jsonl[.zst]
//
// Two things about that layout drive the code below. Sessions are bucketed by
// date rather than by project, so the project a session belongs to can only be
// found by reading the file. And the same conversation is recorded twice inside
// it: `response_item` lines are the model-facing history, while `event_msg` lines
// are a parallel UI stream. Either can supply a title, so both are accepted.
//
// None of this is documented by upstream; it was established by reading real
// rollout files and the Codex source. Everything here therefore tolerates missing
// and unknown fields rather than requiring a shape: fields have appeared and
// disappeared between Codex releases, and a listing must not break when the next
// one lands.
//
// Importing this package registers the provider.
package codex

import (
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
)

// KeySessionsDir is where the rollout tree lives, carried in agent.Roots.Extra.
const KeySessionsDir = "sessionsDir"

// Scan limits. A rollout can carry megabytes of tool output, and a listing must
// stay fast enough to run on every keystroke in the picker, so reading stops once
// there is plainly enough to describe the session.
const (
	maxScanLines = 20000
	maxScanBytes = 8 << 20 // 8 MiB
	maxLineBytes = 1 << 20 // skip pathological single lines
	titleMaxLen  = 60

	// shortIDMin matches the width Claude Code sessions display at, so a mixed
	// listing lines up. It grows per-session when ids collide.
	shortIDMin = 8
)

// Provider is the Codex CLI implementation of agent.Provider.
type Provider struct{}

func init() { agent.Register(Provider{}) }

func (Provider) ID() agent.ID { return agent.Codex }

func (Provider) DisplayName() string { return "OpenAI Codex CLI" }

// Locate resolves $CODEX_HOME (default ~/.codex).
//
// Presence is judged by the sessions directory, not the config directory: Codex
// writes config.toml and auth on first run, so a config dir alone means "logged
// in", not "has history worth listing". An explicit override is always treated as
// present, matching how the Claude provider treats a named directory.
func (Provider) Locate(override string) (agent.Roots, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return agent.Roots{}, false, err
	}
	configDir := override
	if configDir == "" {
		if env := os.Getenv("CODEX_HOME"); env != "" {
			configDir = env
		} else {
			configDir = filepath.Join(home, ".codex")
		}
	}
	sessionsDir := filepath.Join(configDir, "sessions")
	roots := agent.Roots{
		Home:      home,
		ConfigDir: configDir,
		Extra:     map[string]string{KeySessionsDir: sessionsDir},
	}
	if override != "" {
		return roots, true, nil
	}
	fi, statErr := os.Stat(sessionsDir)
	return roots, statErr == nil && fi.IsDir(), nil
}

// Ref is the provider-private handle to a rollout on disk.
type Ref struct {
	Path       string // absolute path to the rollout file
	Compressed bool   // the file is zstd-compressed (.zst)
	DateBucket string // the YYYY/MM/DD directory it sits in
	CLIVersion string // cli_version recorded in session_meta, if any
	Model      string // model from the last turn_context, if any
}

// ListSessions walks the rollout tree, newest first.
//
// A file that cannot be read or parsed is skipped rather than failing the whole
// listing: one truncated rollout, or one written by a future Codex, must not hide
// every other session.
func (Provider) ListSessions(r agent.Roots) ([]agent.Session, error) {
	dir := r.Get(KeySessionsDir)
	if dir == "" {
		return nil, nil
	}
	var out []agent.Session
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip it, keep walking
		}
		if d.IsDir() || !isRollout(d.Name()) {
			return nil
		}
		s, ok := readSession(path, dir)
		if ok {
			out = append(out, s)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	agent.SortSessions(out)
	// Codex ids are UUIDv7, so their leading characters are a timestamp: sessions
	// started moments apart share a prefix and a fixed 8 characters would name two
	// different sessions. Grow each handle until it is unique, git-style.
	agent.AssignShortIDs(out, shortIDMin)
	return out, nil
}

// isRollout reports whether a filename is a rollout, in either the plain or the
// compressed form Codex uses for cold sessions.
func isRollout(name string) bool {
	if !strings.HasPrefix(name, "rollout-") {
		return false
	}
	return strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst")
}

// rolloutLine is the envelope every line shares. Only the fields needed to
// describe a session are declared; everything else is ignored, which is what lets
// this survive Codex adding fields.
type rolloutLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type metaPayload struct {
	ID         string `json:"id"`
	SessionID  string `json:"session_id"`
	Cwd        string `json:"cwd"`
	CLIVersion string `json:"cli_version"`
}

type itemPayload struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Message string `json:"message"` // event_msg carries plain text here
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"` // response_item carries structured blocks here
}

type turnPayload struct {
	Model string `json:"model"`
}

// readSession derives a session description from one rollout file. ok is false
// when the file is not a usable rollout.
func readSession(path, sessionsRoot string) (agent.Session, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return agent.Session{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return agent.Session{}, false
	}
	defer f.Close()

	compressed := strings.HasSuffix(path, ".zst")
	var src io.Reader = f
	if compressed {
		// Cold rollouts are zstd-compressed. Reading them matters: skipping would
		// silently hide a user's older sessions, which is worse than being slow.
		dec, derr := zstd.NewReader(f)
		if derr != nil {
			return agent.Session{}, false
		}
		defer dec.Close()
		src = dec.IOReadCloser()
	}

	var (
		meta     metaPayload
		haveMeta bool
		title    string
		model    string
		messages int
		scanned  int
		bytesIn  int
	)

	br := bufio.NewReaderSize(src, 64<<10)
	for scanned < maxScanLines && bytesIn < maxScanBytes {
		line, rerr := br.ReadString('\n')
		bytesIn += len(line)
		scanned++
		if len(line) > 0 && len(line) <= maxLineBytes {
			var rl rolloutLine
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &rl) == nil {
				switch rl.Type {
				case "session_meta":
					if !haveMeta {
						if json.Unmarshal(rl.Payload, &meta) == nil {
							haveMeta = true
						}
					}
				case "turn_context":
					var tp turnPayload
					if json.Unmarshal(rl.Payload, &tp) == nil && tp.Model != "" {
						model = tp.Model // last one wins: the model in use most recently
					}
				case "response_item", "event_msg":
					var ip itemPayload
					if json.Unmarshal(rl.Payload, &ip) != nil {
						break
					}
					if rl.Type == "response_item" && ip.Type == "message" &&
						(ip.Role == "user" || ip.Role == "assistant") {
						messages++
					}
					if title == "" {
						title = cleanTitle(firstUserText(rl.Type, ip))
					}
				}
			}
		}
		if rerr != nil {
			break
		}
	}

	// A rollout without session_meta is not a session Codex would resume either.
	if !haveMeta {
		return agent.Session{}, false
	}
	id := meta.ID
	if id == "" {
		id = meta.SessionID
	}
	if id == "" {
		// Fall back to the id embedded in the filename, which is where Codex
		// itself looks when resolving a resume by id.
		id = idFromFilename(filepath.Base(path))
	}
	if id == "" {
		return agent.Session{}, false
	}

	bucket := ""
	if rel, rerr := filepath.Rel(sessionsRoot, filepath.Dir(path)); rerr == nil && rel != "." {
		bucket = filepath.ToSlash(rel)
	}

	return agent.Session{
		Provider:    agent.Codex,
		ID:          id,
		ShortID:     shortID(id),
		ProjectPath: meta.Cwd,
		GroupKey:    bucket,
		Title:       title,
		Messages:    messages,
		ModTime:     fi.ModTime(),
		Size:        fi.Size(),
		Ref: Ref{
			Path:       path,
			Compressed: compressed,
			DateBucket: bucket,
			CLIVersion: meta.CLIVersion,
			Model:      model,
		},
	}, true
}

// firstUserText pulls a candidate title out of a line. Codex records the same
// prompt twice, as an event_msg with plain text and as a response_item with
// structured blocks, so both shapes are accepted.
func firstUserText(lineType string, ip itemPayload) string {
	if lineType == "event_msg" {
		if ip.Type == "user_message" {
			return ip.Message
		}
		return ""
	}
	if ip.Type != "message" || ip.Role != "user" {
		return ""
	}
	var b strings.Builder
	for _, c := range ip.Content {
		if c.Text != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

// cleanTitle trims a prompt down to a one-line label, dropping the machine-
// generated preambles that would otherwise become the title. Mirrors the
// treatment Claude Code transcripts get, so a mixed listing reads consistently.
func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "<") {
			continue // <EXTERNAL SESSION IMPORTED>, injected markers, tags
		}
		return truncate(line, titleMaxLen)
	}
	return ""
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

// shortID is the display handle. Codex ids are UUIDv7, whose leading characters
// encode the timestamp, so the first 8 do distinguish sessions in practice.
func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// idFromFilename recovers the session id from a rollout filename of the form
// rollout-<timestamp>-<uuid>.jsonl. The uuid is a fixed 36 characters, so it is
// taken from the end rather than by counting the timestamp's dashes.
func idFromFilename(name string) string {
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".zst"), ".jsonl")
	const uuidLen = 36
	if len(stem) < uuidLen {
		return ""
	}
	cand := stem[len(stem)-uuidLen:]
	if !looksLikeUUID(cand) {
		return ""
	}
	return cand
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

// compile-time assertion that the provider satisfies the contract.
var _ agent.Provider = Provider{}
