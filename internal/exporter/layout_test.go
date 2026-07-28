package exporter

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/entangle/internal/bundle"
	"github.com/gowtham-sai-yadav/entangle/internal/manifest"
)

// The archive layout is a compatibility surface, not an implementation detail:
// every released binary decides where to write a bundle's contents by matching
// these path prefixes. Renaming one makes an older binary either ignore the data
// or, worse, route it somewhere it does not belong inside a user's home
// directory. Nothing else in the suite pins these names, so these tests exist to
// make that class of change loud.
//
// If a change here is deliberate, the fix is not to edit the expected strings: it
// is to leave the existing prefixes alone and add new content under a NEW
// top-level prefix with a raised manifest.SchemaVersion, so old binaries skip
// what they cannot understand.

// entryNames lists every path inside a bundle, sorted.
func entryNames(t *testing.T, path string) []string {
	t.Helper()
	var names []string
	err := bundle.ForEach(path, func(h *tar.Header, _ io.Reader) error {
		names = append(names, h.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	sort.Strings(names)
	return names
}

// fakeClaudeHome builds a config dir with one project, one session, a sidecar,
// memory, and the config files a full backup carries.
func fakeClaudeHome(t *testing.T) (dir, sessionID string) {
	t.Helper()
	dir = t.TempDir()
	sessionID = "abcdef01-2345-6789-abcd-ef0123456789"
	cwd := "/home/demo/proj"
	enc := "-home-demo-proj"

	proj := filepath.Join(dir, "projects", enc)
	if err := os.MkdirAll(filepath.Join(proj, sessionID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := `{"type":"user","cwd":"` + cwd + `","sessionId":"` + sessionID + `","message":{"role":"user","content":"hello there"}}` + "\n"
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("projects", enc, sessionID+".jsonl"), transcript)
	write(filepath.Join("projects", enc, sessionID, "subagent.jsonl"), transcript)
	write(filepath.Join("projects", enc, "memory", "MEMORY.md"), "# notes\n")
	write("settings.json", `{"theme":"dark"}`)
	write("settings.local.json", `{"local":true}`)
	write("history.jsonl", `{"display":"hi","project":"`+cwd+`"}`+"\n")
	write(filepath.Join("plans", "plan-1.md"), "# plan\n")
	write(filepath.Join("plugins", "installed_plugins.json"), `{}`)
	write(filepath.Join("plugins", "known_marketplaces.json"), `{}`)
	// ~/.claude.json lives beside the config dir when ConfigDir is overridden.
	write(".claude.json", `{"projects":{"`+cwd+`":{"allowedTools":[]}},"oauthAccount":{"secret":"nope"}}`)
	return dir, sessionID
}

// TestSessionBundleLayout pins the archive paths for a single shared session,
// the bundle a teammate receives over `send`/`share`.
func TestSessionBundleLayout(t *testing.T) {
	dir, id := fakeClaudeHome(t)

	b, err := PrepareShare(ShareOptions{
		ConfigDir: dir, Version: "test", SessionPrefix: id, Redact: true, WithContext: true,
	})
	if err != nil {
		t.Fatalf("PrepareShare: %v", err)
	}
	out := filepath.Join(t.TempDir(), "session.tgz")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteBundle(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	want := []string{
		"manifest.json",
		"projects/-home-demo-proj/" + id + ".jsonl",
		"projects/-home-demo-proj/" + id + "/subagent.jsonl",
		"projects/-home-demo-proj/memory/MEMORY.md",
	}
	got := entryNames(t, out)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("session bundle layout changed.\n got:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	// The manifest must be the first entry so `inspect` stays cheap.
	var first string
	_ = bundle.ForEach(out, func(h *tar.Header, _ io.Reader) error {
		if first == "" {
			first = h.Name
		}
		return nil
	})
	if first != "manifest.json" {
		t.Errorf("first archive entry = %q, want manifest.json", first)
	}
}

// TestFullBackupLayout pins the archive paths for a whole-machine backup.
func TestFullBackupLayout(t *testing.T) {
	dir, id := fakeClaudeHome(t)
	outDir := t.TempDir()
	out := filepath.Join(outDir, "backup.tgz")

	if _, err := Run(Options{ConfigDir: dir, Version: "test", Out: out}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"manifest.json",
		"config/claude.json",
		"config/history.jsonl",
		"config/settings.json",
		"config/settings.local.json",
		"plans/plan-1.md",
		"plugins/installed_plugins.json",
		"plugins/known_marketplaces.json",
		"projects/-home-demo-proj/" + id + ".jsonl",
		"projects/-home-demo-proj/" + id + "/subagent.jsonl",
		"projects/-home-demo-proj/memory/MEMORY.md",
	}
	sort.Strings(want)
	got := entryNames(t, out)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("full backup layout changed.\n got:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestBundleStampsSupportedSchema guards the pair of constants that let an old
// binary refuse a newer bundle: a bundle must never claim a version this build
// would itself reject.
func TestBundleStampsSupportedSchema(t *testing.T) {
	if manifest.SchemaVersion > manifest.MaxSupportedSchema {
		t.Fatalf("SchemaVersion (%d) exceeds MaxSupportedSchema (%d): this build writes bundles it would refuse to read",
			manifest.SchemaVersion, manifest.MaxSupportedSchema)
	}

	dir, id := fakeClaudeHome(t)
	b, err := PrepareShare(ShareOptions{ConfigDir: dir, Version: "test", SessionPrefix: id, Redact: true})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "s.tgz")
	f, _ := os.Create(out)
	if err := b.WriteBundle(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	mb, err := bundle.ReadManifest(out)
	if err != nil {
		t.Fatal(err)
	}
	var man manifest.Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		t.Fatal(err)
	}
	if man.SchemaVersion != manifest.SchemaVersion {
		t.Errorf("bundle schemaVersion = %d, want %d", man.SchemaVersion, manifest.SchemaVersion)
	}
	if err := man.Unsupported(); err != nil {
		t.Errorf("a bundle we just wrote must be readable by this build: %v", err)
	}
}
