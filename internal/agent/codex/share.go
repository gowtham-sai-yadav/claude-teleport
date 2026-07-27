package codex

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/agent"
)

// Sharing a Codex session, and why it works at all.
//
// A rollout file is self-sufficient: Codex resolves `codex resume <id>` by
// scanning the sessions tree for a matching filename and repairs its own index
// afterwards. So placing a well-formed file is enough - there is no database row
// to insert and nothing to register. That was confirmed by experiment, not
// assumed: a hand-written rollout with no index entry resumed, and Codex created
// the entry itself.
//
// Three details are load-bearing, and each one was learned the hard way:
//
//   - The `source` field must stay a value Codex recognises ("cli"). An
//     unrecognised value is parsed as Unknown and the session is filtered out of
//     every listing forever - present on disk, invisible in the picker.
//   - There must be at least one event_msg/user_message line. The session picker
//     builds its preview only from those, and refuses to show a session with an
//     empty preview. A file of session_meta plus response_item lines resumes by id
//     but cannot be found by a human.
//   - `history_mode` is the one field where an unrecognised value is a hard error
//     rather than being ignored, so it is passed through untouched or left out.
//
// Only the cwd is rewritten, and only where Codex reads it structurally. Prose
// inside the conversation is left alone, matching what the Claude importer does:
// silently editing the text of someone's conversation is worse than a stale path
// in a sentence.

const bundlePrefix = "agents/codex/"

// rolloutEntry is the single file a shared Codex session consists of.
const rolloutEntry = bundlePrefix + "rollout.jsonl"

// BundlePrefix implements agent.Sharer.
func (Provider) BundlePrefix() string { return bundlePrefix }

// Pack reads the rollout, scrubs it, and returns it in memory.
func (Provider) Pack(r agent.Roots, s agent.Session, opts agent.PackOptions) (*agent.Packed, error) {
	ref, ok := s.Ref.(Ref)
	if !ok || ref.Path == "" {
		return nil, fmt.Errorf("session %s has no rollout on this machine", s.ShortID)
	}
	raw, err := readRollout(ref.Path)
	if err != nil {
		return nil, fmt.Errorf("read rollout: %w", err)
	}

	masked := 0
	if opts.Scrub != nil {
		raw, masked = opts.Scrub(raw)
	}

	return &agent.Packed{
		Entries: []agent.Entry{{Name: rolloutEntry, Data: raw}},
		Preview: agent.Preview{
			Title:         s.Title,
			ShortID:       s.ShortID,
			ProjectPath:   s.ProjectPath,
			Messages:      s.Messages,
			Bytes:         int64(len(raw)),
			SecretsMasked: masked,
			AgentVersion:  ref.CLIVersion,
		},
	}, nil
}

// readRollout returns the rollout's bytes, decompressing a cold one.
func readRollout(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if !strings.HasSuffix(path, ".zst") {
		return io.ReadAll(f)
	}
	dec, err := zstd.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	// Always store the plain form in a bundle, so the receiving side never has to
	// care how the sender's copy happened to be stored.
	return io.ReadAll(dec.IOReadCloser())
}

// Unpack writes a shared rollout into this machine's Codex, rebound to targetDir.
func (Provider) Unpack(r agent.Roots, entries []agent.Entry, targetDir string) (*agent.Unpacked, error) {
	var raw []byte
	for _, e := range entries {
		if strings.HasSuffix(e.Name, "rollout.jsonl") {
			raw = e.Data
			break
		}
	}
	if raw == nil {
		return nil, errors.New("this bundle has no Codex rollout in it")
	}
	sessionsDir := r.Get(KeySessionsDir)
	if sessionsDir == "" {
		return nil, errors.New("could not work out where Codex keeps its sessions on this machine")
	}

	// A fresh id, because the receiver may already have the sender's session (both
	// people can have been in the same shared conversation) and two rollouts with
	// one id would make `codex resume` ambiguous.
	newID, err := newSessionID()
	if err != nil {
		return nil, err
	}
	rewritten, warn := rebindRollout(raw, newID, targetDir)

	now := time.Now()
	dir := filepath.Join(sessionsDir, now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// Codex names files with a local-time stamp, and derives the id from the tail
	// of the name, so this has to match its own convention to be found.
	name := fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), newID)
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("a session already exists at %s", dest)
	}
	if err := os.WriteFile(dest, rewritten, 0o644); err != nil {
		return nil, err
	}

	hint := "codex resume " + newID
	if warn != "" {
		hint += "   (" + warn + ")"
	}
	return &agent.Unpacked{SessionID: newID, Written: 1, ResumeHint: hint}, nil
}

// rebindRollout rewrites the session identity and working directory so the
// session belongs to this machine and this project.
//
// It edits line by line and only the fields Codex reads structurally. Unknown
// lines pass through untouched, which is what keeps this working when Codex adds
// something new.
func rebindRollout(raw []byte, newID, targetDir string) (out []byte, warning string) {
	var (
		b            strings.Builder
		sawUserEvent bool
		sawMeta      bool
	)
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var env map[string]json.RawMessage
		if json.Unmarshal(line, &env) != nil {
			b.Write(line) // not JSON we understand: keep it verbatim
			b.WriteByte('\n')
			continue
		}
		var lineType string
		_ = json.Unmarshal(env["type"], &lineType)

		switch lineType {
		case "session_meta":
			sawMeta = true
			if p, ok := patchObject(env["payload"], func(m map[string]any) {
				m["id"] = newID
				m["session_id"] = newID
				m["cwd"] = targetDir
				// Anything other than a value Codex knows makes the session
				// invisible in its picker, so this is forced back to "cli".
				m["source"] = "cli"
			}); ok {
				env["payload"] = p
			}
		case "turn_context":
			if p, ok := patchObject(env["payload"], func(m map[string]any) {
				m["cwd"] = targetDir
				if _, has := m["workspace_roots"]; has {
					m["workspace_roots"] = []any{targetDir}
				}
			}); ok {
				env["payload"] = p
			}
		case "event_msg":
			var sub struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(env["payload"], &sub) == nil && sub.Type == "user_message" {
				sawUserEvent = true
			}
		}

		if re, err := json.Marshal(env); err == nil {
			b.Write(re)
		} else {
			b.Write(line)
		}
		b.WriteByte('\n')
	}

	result := b.String()
	switch {
	case !sawMeta:
		warning = "no session header found; Codex may not list this"
	case !sawUserEvent:
		// Without one of these the session resumes by id but never appears in the
		// picker, so say it rather than let the receiver think it vanished.
		warning = "resumable by id, but it may not appear in Codex's session list"
	}
	return []byte(result), warning
}

// patchObject applies fn to a JSON object and re-encodes it.
func patchObject(raw json.RawMessage, fn func(map[string]any)) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	fn(m)
	re, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return re, true
}

// newSessionID makes a UUIDv7-shaped id: a millisecond timestamp followed by
// randomness, which is the form Codex writes and sorts by.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

var _ agent.Sharer = Provider{}
