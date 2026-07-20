package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
)

// TestPrepareShareBuildsSessionBundle checks the in-memory build path: given a
// fake config dir with one session, PrepareShare recovers the project, and the
// bundle it writes reads back as a valid single-session bundle.
func TestPrepareShareBuildsSessionBundle(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects", "-tmp-demo")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "abcdef01-2345-6789-abcd-ef0123456789"
	line := `{"type":"user","cwd":"/tmp/demo","sessionId":"` + id + `","message":{"role":"user","content":"hello there"}}` + "\n"
	if err := os.WriteFile(filepath.Join(projDir, id+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	b, err := PrepareShare(ShareOptions{ConfigDir: dir, Version: "test", SessionPrefix: id, Redact: true})
	if err != nil {
		t.Fatalf("PrepareShare: %v", err)
	}
	if b.Session.ID != id {
		t.Errorf("session id = %q, want %q", b.Session.ID, id)
	}
	if b.Preview.ProjectPath != "/tmp/demo" {
		t.Errorf("project path = %q, want /tmp/demo", b.Preview.ProjectPath)
	}
	if b.Preview.Messages == 0 {
		t.Errorf("expected a nonzero message count")
	}

	out := filepath.Join(dir, "out.tgz")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteBundle(f); err != nil {
		f.Close()
		t.Fatalf("WriteBundle: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	mb, err := bundle.ReadManifest(out)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	var man manifest.Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if !man.IsSession() {
		t.Errorf("expected a session bundle, got kind %q", man.Kind)
	}
	if man.SessionID != id {
		t.Errorf("manifest session id = %q, want %q", man.SessionID, id)
	}
	if len(man.Projects) != 1 || man.Projects[0].OriginalPath != "/tmp/demo" {
		t.Errorf("manifest projects = %+v", man.Projects)
	}
}

// TestPrepareShareUnknownSession errors clearly when the id matches nothing.
func TestPrepareShareUnknownSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareShare(ShareOptions{ConfigDir: dir, Version: "test", SessionPrefix: "does-not-exist"})
	if err == nil {
		t.Fatal("expected an error for an unknown session")
	}
	if !strings.Contains(err.Error(), "no session matches") {
		t.Errorf("unexpected error: %v", err)
	}
}
