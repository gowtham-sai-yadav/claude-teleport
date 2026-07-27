// Package agentshare packs and unpacks a single session belonging to a coding
// tool other than Claude Code.
//
// Claude Code has its own, older path through internal/exporter and
// internal/importer, and it stays there untouched: its bundle layout is what every
// released binary reads, so freezing it is worth more than making the code look
// uniform. This package is the equivalent for every other tool, and it differs in
// exactly two deliberate ways.
//
// Foreign sessions live under an "agents/<tool>/" prefix, never under "projects/".
// A binary released before multi-tool support decides where to write a bundle's
// contents by matching that prefix, so reusing it would make an old binary unpack
// a Codex rollout into someone's Claude directory - real corruption of a working
// install. An unrecognised prefix is simply skipped instead.
//
// And they are stamped with a higher schema version, so those older binaries
// refuse the bundle outright with "run claude-teleport update" rather than
// silently writing nothing and reporting success.
package agentshare

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/agent"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/redact"
)

// Bundle is a packed foreign session held in memory, not yet written anywhere.
type Bundle struct {
	// Name is a suggested filename for the "save it to a file" flow.
	Name string
	// Preview is what to show the user before this leaves the machine.
	Preview agent.Preview
	// Provider is the tool it came from.
	Provider agent.ID

	manifestJSON []byte
	entries      []agent.Entry
}

// Options configures packing.
type Options struct {
	ToolVersion string // claude-teleport's own version, for the manifest
	Redact      bool   // scrub likely secrets before packing
}

// Pack captures one session in memory. Nothing is written and nothing leaves the
// machine; the caller decides the destination.
func Pack(p agent.Provider, r agent.Roots, s agent.Session, opts Options) (*Bundle, error) {
	sh, ok := agent.SharerFor(p)
	if !ok {
		return nil, fmt.Errorf("%s sessions cannot be shared yet", p.DisplayName())
	}

	scrub := func(b []byte) ([]byte, int) { return b, 0 }
	if opts.Redact {
		scrub = redact.Scrub
	}
	packed, err := sh.Pack(r, s, agent.PackOptions{Scrub: scrub})
	if err != nil {
		return nil, err
	}
	if len(packed.Entries) == 0 {
		return nil, errors.New("nothing to share: that session appears to be empty")
	}

	man := manifest.Manifest{
		Tool:          manifest.Tool,
		ToolVersion:   opts.ToolVersion,
		SchemaVersion: manifest.SchemaVersionAgent,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Kind:          manifest.KindSession,
		SessionID:     s.ID,
		Redacted:      opts.Redact,
		Source:        manifest.Source{OS: runtime.GOOS},
		// Includes is printed verbatim by `inspect` on binaries that predate this
		// format, so it doubles as the explanation for a human looking at a bundle
		// their build cannot unpack.
		Includes:     []string{sh.BundlePrefix(), "requires claude-teleport >= 0.6"},
		Agent:        string(p.ID()),
		AgentVersion: packed.Preview.AgentVersion,
		ProjectPath:  s.ProjectPath,
	}
	mb, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Name:         fmt.Sprintf("claude-teleport-%s-session-%s.tgz", p.ID(), safeName(s.ShortID)),
		Preview:      packed.Preview,
		Provider:     p.ID(),
		manifestJSON: mb,
		entries:      packed.Entries,
	}, nil
}

// WriteBundle writes the bundle to w, manifest first so `inspect` stays cheap. It
// flushes the archive framing but does not close w.
func (b *Bundle) WriteBundle(w io.Writer) error {
	bw := bundle.NewWriter(w)
	if err := bw.AddBytes("manifest.json", b.manifestJSON); err != nil {
		return err
	}
	for _, e := range b.entries {
		if err := bw.AddBytes(e.Name, e.Data); err != nil {
			return err
		}
	}
	return bw.Close()
}

// UnpackResult reports what an unpack did.
type UnpackResult struct {
	Provider    agent.ID
	DisplayName string
	SessionID   string
	Written     int
	ResumeHint  string
	TargetDir   string
}

// Unpack reads a foreign-session bundle and hands it to the owning tool's
// provider, binding it to targetDir.
//
// The tool has to be installed here: a Codex session means nothing without Codex,
// and saying so plainly beats writing files nothing will ever read.
func Unpack(bundlePath, targetDir, configDirOverride string) (*UnpackResult, error) {
	man, err := readManifest(bundlePath)
	if err != nil {
		return nil, err
	}
	if !man.IsForeignAgent() {
		return nil, errors.New("this is a Claude Code bundle; import it with the normal import path")
	}

	id := agent.ID(man.Agent)
	p, ok := agent.Get(id)
	if !ok {
		return nil, fmt.Errorf("this bundle is from %q, which this version of claude-teleport does not know about; try `claude-teleport update`", man.Agent)
	}
	sh, ok := agent.SharerFor(p)
	if !ok {
		return nil, fmt.Errorf("%s sessions cannot be imported yet", p.DisplayName())
	}
	roots, present, err := p.Locate(configDirOverride)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("this is a %s session, but %s does not appear to be set up on this machine. Install it first, then import again",
			p.DisplayName(), p.DisplayName())
	}

	prefix := sh.BundlePrefix()
	var entries []agent.Entry
	err = bundle.ForEach(bundlePath, func(h *tar.Header, r io.Reader) error {
		name := h.Name
		if name == "manifest.json" || !strings.HasPrefix(name, prefix) {
			return nil
		}
		data, rerr := io.ReadAll(r)
		if rerr != nil {
			return rerr
		}
		entries = append(entries, agent.Entry{Name: name, Data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("this bundle claims to hold a %s session but has no %s files in it", p.DisplayName(), prefix)
	}

	res, err := sh.Unpack(roots, entries, targetDir)
	if err != nil {
		return nil, err
	}
	return &UnpackResult{
		Provider:    id,
		DisplayName: p.DisplayName(),
		SessionID:   res.SessionID,
		Written:     res.Written,
		ResumeHint:  res.ResumeHint,
		TargetDir:   targetDir,
	}, nil
}

// PeekAgent reports which tool a bundle belongs to without unpacking it, so a
// caller can route between the Claude path and this one.
func PeekAgent(bundlePath string) (agent.ID, bool, error) {
	man, err := readManifest(bundlePath)
	if err != nil {
		return "", false, err
	}
	return agent.ID(man.AgentID()), man.IsForeignAgent(), nil
}

func readManifest(bundlePath string) (manifest.Manifest, error) {
	mb, err := bundle.ReadManifest(bundlePath)
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	if len(mb) == 0 {
		return manifest.Manifest{}, fmt.Errorf("no manifest.json found - is %q a claude-teleport bundle?", bundlePath)
	}
	var man manifest.Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		return manifest.Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := man.Unsupported(); err != nil {
		return manifest.Manifest{}, err
	}
	return man, nil
}

// safeName keeps a generated filename to characters that behave everywhere.
func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "session"
	}
	return b.String()
}
