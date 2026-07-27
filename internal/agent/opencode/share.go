package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/agent"
)

// Sharing an opencode session.
//
// This is the one tool of the three that supports it directly: `opencode export
// <id>` writes a session as JSON and `opencode import <file>` reads one back. So
// nothing here forges a file or writes to the database - both ends go through
// commands opencode documents.
//
// The important property is that opencode's import re-homes what it reads: it
// overwrites the project id, directory, and path from whatever instance is doing
// the import. That is exactly the path-rebinding problem this project exists to
// solve for Claude Code, and here the tool does it for us, provided the import is
// run from the directory the session should belong to.

const bundlePrefix = "agents/opencode/"

const sessionEntry = bundlePrefix + "session.json"

// BundlePrefix implements agent.Sharer.
func (Provider) BundlePrefix() string { return bundlePrefix }

// Pack exports one session through the opencode CLI and scrubs the result.
func (Provider) Pack(r agent.Roots, s agent.Session, opts agent.PackOptions) (*agent.Packed, error) {
	bin := r.Get(KeyBinary)
	if bin == "" {
		return nil, errors.New("opencode is not installed on this machine")
	}
	// Its own --sanitize is deliberately not used: it replaces whole fields with
	// placeholders, which would strip the conversation the receiver needs. Our
	// scrubbing masks only what looks like a secret and leaves the text intact.
	raw, err := runCmd(bin, "export", s.ID)
	if err != nil {
		return nil, fmt.Errorf("opencode export: %w", err)
	}
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("opencode exported nothing for session %s", s.ShortID)
	}
	// Fail here rather than hand a teammate something their opencode will reject.
	if !json.Valid(raw) {
		return nil, errors.New("opencode export did not return JSON (its export format may have changed)")
	}

	masked := 0
	if opts.Scrub != nil {
		scrubbed, n := opts.Scrub(raw)
		// Scrubbing edits bytes inside a JSON document, so confirm it is still a
		// document before shipping it.
		if json.Valid(scrubbed) {
			raw, masked = scrubbed, n
		}
	}

	return &agent.Packed{
		Entries: []agent.Entry{{Name: sessionEntry, Data: raw}},
		Preview: agent.Preview{
			Title:         s.Title,
			ShortID:       s.ShortID,
			ProjectPath:   s.ProjectPath,
			Messages:      s.Messages,
			Bytes:         int64(len(raw)),
			SecretsMasked: masked,
		},
	}, nil
}

// Unpack hands the session to `opencode import`, run from targetDir so opencode
// binds it to the project the receiver is standing in.
func (Provider) Unpack(r agent.Roots, entries []agent.Entry, targetDir string) (*agent.Unpacked, error) {
	bin := r.Get(KeyBinary)
	if bin == "" {
		return nil, errors.New("opencode is not installed on this machine, so this session cannot be imported")
	}
	var raw []byte
	for _, e := range entries {
		if strings.HasSuffix(e.Name, "session.json") {
			raw = e.Data
			break
		}
	}
	if raw == nil {
		return nil, errors.New("this bundle has no opencode session in it")
	}

	// A real file, because import takes a path.
	tmp, err := os.CreateTemp("", "claude-teleport-opencode-*.json")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	if _, err := runCmdIn(targetDir, bin, "import", tmpPath); err != nil {
		return nil, fmt.Errorf("opencode import: %w", err)
	}

	// Report the id opencode kept, so the receiver can find the session. Import
	// preserves the session id and only re-homes the project fields.
	id := sessionIDFrom(raw)
	hint := "run `opencode` in " + targetDir + " and pick the session"
	if id != "" {
		hint = "opencode --session " + id + "   (from " + targetDir + ")"
	}
	return &agent.Unpacked{SessionID: id, Written: 1, ResumeHint: hint}, nil
}

// sessionIDFrom pulls the session id out of an export document, tolerating a
// shape change by simply returning "" rather than failing an import that worked.
func sessionIDFrom(raw []byte) string {
	var doc struct {
		Info struct {
			ID string `json:"id"`
		} `json:"info"`
	}
	if json.Unmarshal(raw, &doc) == nil && doc.Info.ID != "" {
		return doc.Info.ID
	}
	return ""
}

// runCmdIn runs the CLI with a working directory, which is how opencode is told
// which project an imported session belongs to.
var runCmdIn = func(dir, bin string, args ...string) ([]byte, error) {
	return execRunnerIn(dir, bin, args...)
}

var _ agent.Sharer = Provider{}
