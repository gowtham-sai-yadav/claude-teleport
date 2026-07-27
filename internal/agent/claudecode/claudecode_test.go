package claudecode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
)

// TestRegistered proves the init-time registration works, which is what makes
// importing this package the only thing needed to enable the provider.
func TestRegistered(t *testing.T) {
	p, ok := agent.Get(agent.ClaudeCode)
	if !ok {
		t.Fatal("importing this package should register the Claude Code provider")
	}
	if p.DisplayName() != "Claude Code" {
		t.Errorf("DisplayName = %q", p.DisplayName())
	}
}

// TestLocatePresence: a bare ~/.claude with no history should not count as set up
// (running `claude --version` creates one), but an explicit --config-dir always
// should, because naming a directory is a statement of intent and import needs to
// target a directory that does not exist yet.
func TestLocatePresence(t *testing.T) {
	var p Provider

	empty := t.TempDir() // exists, but has no projects/ inside
	r, ok, err := p.Locate(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("an explicit override must always be treated as present")
	}
	if r.ConfigDir != empty {
		t.Errorf("ConfigDir = %q, want %q", r.ConfigDir, empty)
	}

	// With CLAUDE_CONFIG_DIR pointed at a dir with no projects/, absence is
	// reported rather than an empty listing.
	t.Setenv("CLAUDE_CONFIG_DIR", empty)
	if _, ok, err = p.Locate(""); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("a config dir with no projects/ should report not-present")
	}

	// Once projects/ exists, it is present.
	if err := os.MkdirAll(filepath.Join(empty, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = p.Locate(""); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Error("a config dir with projects/ should report present")
	}
}

// TestRootsRoundTrip: Wrap and Unwrap are the single boundary between the shared
// Roots shape and Claude's own paths, so they must not lose anything.
func TestRootsRoundTrip(t *testing.T) {
	var p Provider
	dir := t.TempDir()
	r, _, err := p.Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	back := Unwrap(r)
	if back.ConfigDir != dir {
		t.Errorf("ConfigDir lost: %q", back.ConfigDir)
	}
	if back.ProjectsDir != filepath.Join(dir, "projects") {
		t.Errorf("ProjectsDir = %q", back.ProjectsDir)
	}
	if back.JSONPath == "" || back.Home == "" {
		t.Errorf("JSONPath/Home lost: %+v", back)
	}
}

// TestListSessionsMapsFields checks the translation into the shared Session shape,
// including that Ref carries enough to find the bytes again without another scan.
func TestListSessionsMapsFields(t *testing.T) {
	dir := t.TempDir()
	id := "abcdef01-2345-6789-abcd-ef0123456789"
	cwd := "/home/demo/proj"
	projDir := filepath.Join(dir, "projects", "-home-demo-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","cwd":"` + cwd + `","sessionId":"` + id + `","message":{"role":"user","content":"fix the parser"}}` + "\n"
	if err := os.WriteFile(filepath.Join(projDir, id+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	var p Provider
	r, _, err := p.Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.ListSessions(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 session, got %d", len(got))
	}
	s := got[0]
	if s.Provider != agent.ClaudeCode {
		t.Errorf("Provider = %q", s.Provider)
	}
	if s.ID != id {
		t.Errorf("ID = %q, want %q", s.ID, id)
	}
	if s.ShortID != id[:8] {
		t.Errorf("ShortID = %q, want %q", s.ShortID, id[:8])
	}
	if s.ProjectPath != cwd {
		t.Errorf("ProjectPath = %q, want %q", s.ProjectPath, cwd)
	}
	if s.GroupKey != "-home-demo-proj" {
		t.Errorf("GroupKey = %q", s.GroupKey)
	}
	if s.Title != "fix the parser" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.Messages == 0 || s.Size == 0 || s.ModTime.IsZero() {
		t.Errorf("metadata not populated: %+v", s)
	}
	ref, ok := SessionRef(s)
	if !ok {
		t.Fatal("Ref should carry the underlying claudedir.Session")
	}
	if ref.File == "" || ref.ID != id {
		t.Errorf("Ref is not usable: %+v", ref)
	}
}
