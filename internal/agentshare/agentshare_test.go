package agentshare

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
	"github.com/gowtham-sai-yadav/entangle/internal/bundle"
	"github.com/gowtham-sai-yadav/entangle/internal/manifest"
)

// The properties tested here are the ones that protect users who have not
// updated. A binary released before multi-tool support routes a bundle's contents
// purely by matching path prefixes, so a foreign session sharing the Claude prefix
// would be written into someone's Claude directory. Both the prefix and the schema
// version exist to prevent that, and both are easy to break by accident.

// fake is a Sharer that records what it was asked to do.
type fake struct {
	id      agent.ID
	name    string
	prefix  string
	entries []agent.Entry
	// captured by Unpack
	gotEntries []agent.Entry
	gotDir     string
}

func (f *fake) ID() agent.ID         { return f.id }
func (f *fake) DisplayName() string  { return f.name }
func (f *fake) BundlePrefix() string { return f.prefix }
func (f *fake) Locate(string) (agent.Roots, bool, error) {
	return agent.Roots{Home: "/h"}, true, nil
}
func (f *fake) ListSessions(agent.Roots) ([]agent.Session, error) { return nil, nil }
func (f *fake) Pack(r agent.Roots, s agent.Session, opts agent.PackOptions) (*agent.Packed, error) {
	out := make([]agent.Entry, len(f.entries))
	masked := 0
	for i, e := range f.entries {
		data := e.Data
		if opts.Scrub != nil {
			var n int
			data, n = opts.Scrub(data)
			masked += n
		}
		out[i] = agent.Entry{Name: e.Name, Data: data}
	}
	return &agent.Packed{
		Entries: out,
		Preview: agent.Preview{Title: s.Title, ShortID: s.ShortID, ProjectPath: s.ProjectPath,
			Messages: s.Messages, Bytes: 42, SecretsMasked: masked, AgentVersion: "1.2.3"},
	}, nil
}
func (f *fake) Unpack(r agent.Roots, entries []agent.Entry, targetDir string) (*agent.Unpacked, error) {
	f.gotEntries, f.gotDir = entries, targetDir
	return &agent.Unpacked{SessionID: "new-id", Written: len(entries), ResumeHint: "carry on"}, nil
}

func register(t *testing.T, f *fake) {
	t.Helper()
	agent.Register(f)
	t.Cleanup(func() { agent.Unregister(f.id) })
}

func session() agent.Session {
	return agent.Session{ID: "sess-1", ShortID: "sess-1", Title: "refactor auth",
		ProjectPath: "/home/dev/api", Messages: 7}
}

func packToFile(t *testing.T, f *fake) string {
	t.Helper()
	b, err := Pack(f, agent.Roots{}, session(), Options{ToolVersion: "test", Redact: true})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "s.tgz")
	fh, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteBundle(fh); err != nil {
		fh.Close()
		t.Fatal(err)
	}
	fh.Close()
	return out
}

func names(t *testing.T, path string) []string {
	t.Helper()
	var out []string
	if err := bundle.ForEach(path, func(h *tar.Header, _ io.Reader) error {
		out = append(out, h.Name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func readManifestOf(t *testing.T, path string) manifest.Manifest {
	t.Helper()
	mb, err := bundle.ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestForeignBundleNeverUsesClaudePrefix is the one that protects existing users:
// an old binary decides where to write by prefix, so a foreign session under
// "projects/" would land in someone's Claude directory.
func TestForeignBundleNeverUsesClaudePrefix(t *testing.T) {
	f := &fake{id: "toolx", name: "Tool X", prefix: "agents/toolx/",
		entries: []agent.Entry{{Name: "agents/toolx/data.jsonl", Data: []byte("{}\n")}}}
	register(t, f)

	path := packToFile(t, f)
	for _, n := range names(t, path) {
		if n == "manifest.json" {
			continue
		}
		if strings.HasPrefix(n, "projects/") || strings.HasPrefix(n, "config/") ||
			strings.HasPrefix(n, "plans/") || strings.HasPrefix(n, "plugins/") {
			t.Errorf("entry %q uses a prefix the Claude importer claims; an older binary would write it into ~/.claude", n)
		}
		if !strings.HasPrefix(n, "agents/") {
			t.Errorf("entry %q should live under agents/", n)
		}
	}
}

// TestForeignBundleIsRefusedByOlderBuilds: the schema version has to be above what
// a pre-multi-tool binary accepts, so it says "update" instead of silently writing
// nothing and reporting success.
func TestForeignBundleIsRefusedByOlderBuilds(t *testing.T) {
	f := &fake{id: "toolx", name: "Tool X", prefix: "agents/toolx/",
		entries: []agent.Entry{{Name: "agents/toolx/data.jsonl", Data: []byte("{}")}}}
	register(t, f)

	man := readManifestOf(t, packToFile(t, f))
	if man.SchemaVersion <= manifest.SchemaVersion {
		t.Errorf("foreign bundle schemaVersion = %d, must exceed the Claude version (%d) so old builds refuse it",
			man.SchemaVersion, manifest.SchemaVersion)
	}
	if man.Agent != "toolx" {
		t.Errorf("manifest agent = %q, want toolx", man.Agent)
	}
	if !man.IsForeignAgent() {
		t.Error("IsForeignAgent should be true")
	}
	// Includes is printed verbatim by `inspect` on binaries that predate this
	// format, so it doubles as the explanation for a confused human.
	if !strings.Contains(strings.Join(man.Includes, " "), "requires entangle") {
		t.Errorf("Includes should tell an old build's inspect what is needed: %v", man.Includes)
	}
}

// TestClaudeManifestUnchanged: an absent agent field still means Claude Code, so
// every bundle already on a disk keeps working.
func TestClaudeManifestUnchanged(t *testing.T) {
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(`{"tool":"entangle","schemaVersion":1,"kind":"session"}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.IsForeignAgent() {
		t.Error("a bundle with no agent field must not be treated as foreign")
	}
	if m.AgentID() != "claude-code" {
		t.Errorf("AgentID() = %q, want claude-code", m.AgentID())
	}
	if err := m.Unsupported(); err != nil {
		t.Errorf("an existing Claude bundle must stay readable: %v", err)
	}
}

// TestScrubIsApplied: redaction is the caller's policy, applied for every tool
// rather than left to each provider to remember.
func TestScrubIsApplied(t *testing.T) {
	f := &fake{id: "toolx", name: "Tool X", prefix: "agents/toolx/",
		entries: []agent.Entry{{Name: "agents/toolx/data.jsonl",
			Data: []byte(`{"text":"my key is sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`)}}}
	register(t, f)

	b, err := Pack(f, agent.Roots{}, session(), Options{ToolVersion: "t", Redact: true})
	if err != nil {
		t.Fatal(err)
	}
	if b.Preview.SecretsMasked == 0 {
		t.Error("a bundle containing an API key should report masking it")
	}
	var buf strings.Builder
	if err := b.WriteBundle(&noopWriter{&buf}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "sk-ant-api03-AAAA") {
		t.Error("the key survived into the bundle")
	}

	// With redaction off, nothing is masked - the flag has to actually mean something.
	b2, err := Pack(f, agent.Roots{}, session(), Options{ToolVersion: "t", Redact: false})
	if err != nil {
		t.Fatal(err)
	}
	if b2.Preview.SecretsMasked != 0 {
		t.Errorf("Redact:false should mask nothing, got %d", b2.Preview.SecretsMasked)
	}
}

type noopWriter struct{ b *strings.Builder }

func (w *noopWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// TestUnpackRoutesToTheOwningTool: the manifest says which tool a bundle came
// from, so a receiver never has to be told.
func TestUnpackRoutesToTheOwningTool(t *testing.T) {
	f := &fake{id: "toolx", name: "Tool X", prefix: "agents/toolx/",
		entries: []agent.Entry{{Name: "agents/toolx/data.jsonl", Data: []byte("{}\n")}}}
	register(t, f)

	path := packToFile(t, f)
	target := t.TempDir()
	res, err := Unpack(path, target, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "toolx" || res.DisplayName != "Tool X" {
		t.Errorf("routed to %q/%q", res.Provider, res.DisplayName)
	}
	if f.gotDir != target {
		t.Errorf("provider was given dir %q, want %q - the session must bind to where the receiver is standing", f.gotDir, target)
	}
	if len(f.gotEntries) != 1 || f.gotEntries[0].Name != "agents/toolx/data.jsonl" {
		t.Errorf("provider got the wrong entries: %+v", f.gotEntries)
	}
	if res.ResumeHint == "" {
		t.Error("the result should say how to carry on")
	}
}

// TestUnpackUnknownToolIsClear: a bundle from a tool this build has never heard of
// should suggest updating, not fail obscurely.
func TestUnpackUnknownToolIsClear(t *testing.T) {
	f := &fake{id: "toolx", name: "Tool X", prefix: "agents/toolx/",
		entries: []agent.Entry{{Name: "agents/toolx/data.jsonl", Data: []byte("{}")}}}
	register(t, f)
	path := packToFile(t, f)

	// Forget the tool, as an older build would not know it.
	agent.Unregister("toolx")

	_, err := Unpack(path, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	if !strings.Contains(err.Error(), "update") {
		t.Errorf("the message should suggest updating, got: %v", err)
	}
	register(t, f) // restore so cleanup is symmetrical
}

// TestPeekAgent lets a caller route without unpacking.
func TestPeekAgent(t *testing.T) {
	f := &fake{id: "toolx", name: "Tool X", prefix: "agents/toolx/",
		entries: []agent.Entry{{Name: "agents/toolx/data.jsonl", Data: []byte("{}")}}}
	register(t, f)

	id, foreign, err := PeekAgent(packToFile(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if !foreign || id != "toolx" {
		t.Errorf("PeekAgent = %q/%v, want toolx/true", id, foreign)
	}
}

// TestPackWithoutSharerIsRefused: listing a tool does not imply being able to
// share it, and the message should say so plainly.
func TestPackWithoutSharerIsRefused(t *testing.T) {
	p := listOnly{}
	agent.Register(p)
	t.Cleanup(func() { agent.Unregister(p.ID()) })

	_, err := Pack(p, agent.Roots{}, session(), Options{})
	if err == nil || !strings.Contains(err.Error(), "cannot be shared yet") {
		t.Fatalf("want a clear refusal, got %v", err)
	}
}

type listOnly struct{}

func (listOnly) ID() agent.ID                                      { return "listonly" }
func (listOnly) DisplayName() string                               { return "List Only" }
func (listOnly) Locate(string) (agent.Roots, bool, error)          { return agent.Roots{}, true, nil }
func (listOnly) ListSessions(agent.Roots) ([]agent.Session, error) { return nil, nil }
