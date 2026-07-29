package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
)

// writeSession drops a minimal transcript (one user line + N assistant lines)
// into a fake config dir so runSessions has something to list.
func writeSession(t *testing.T, dir, folder, cwd, id, title string, assistants int) {
	t.Helper()
	d := filepath.Join(dir, "projects", folder)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`{"type":"user","cwd":"` + cwd + `","sessionId":"` + id + `","message":{"role":"user","content":"` + title + `"}}` + "\n")
	for i := 0; i < assistants; i++ {
		b.WriteString(`{"type":"assistant","cwd":"` + cwd + `","message":{"role":"assistant","content":"..."}}` + "\n")
	}
	if err := os.WriteFile(filepath.Join(d, id+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what it printed.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	return string(out)
}

func TestSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "-home-alice-api", "/home/alice/api", "11111111-aaaa-4000-8000-000000000001", "refactor the auth layer", 3)
	writeSession(t, dir, "-home-alice-web", "/home/alice/web", "22222222-bbbb-4000-8000-000000000002", "build the web ui", 1)
	// A temp-dir session must be excluded by the ephemeral-path filter.
	writeSession(t, dir, "-private-var-folders-x-T-ps-coding-Z", "/private/var/folders/x/T/ps-coding-Z", "33333333-cccc-4000-8000-000000000003", "junk grading call", 0)

	out := captureStdout(t, func() error { return Run([]string{"sessions", "--json", "--config-dir", dir}) })

	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sessions (temp one excluded), got %d:\n%s", len(got), out)
	}
	for _, s := range got {
		for _, k := range []string{"id", "shortId", "project", "folder", "messages", "modified", "sizeBytes", "title"} {
			if _, ok := s[k]; !ok {
				t.Errorf("session JSON missing field %q: %v", k, s)
			}
		}
		if id, _ := s["id"].(string); len(id) < 8 {
			t.Errorf("id looks wrong: %q", id)
		}
		if sid, _ := s["shortId"].(string); len(sid) != 8 {
			t.Errorf("shortId should be 8 chars, got %q", sid)
		}
	}
}

func TestSessionsJSONEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() error { return Run([]string{"sessions", "--json", "--config-dir", dir}) })
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty --json should print [], got %q", out)
	}
}

func TestSessionsJSONProjectFilter(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "-home-alice-api", "/home/alice/api", "11111111-aaaa-4000-8000-000000000001", "api work", 1)
	writeSession(t, dir, "-home-alice-web", "/home/alice/web", "22222222-bbbb-4000-8000-000000000002", "web work", 1)

	out := captureStdout(t, func() error {
		return Run([]string{"sessions", "--json", "--project", "web", "--config-dir", dir})
	})
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("--project web should match 1, got %d", len(got))
	}
	if p, _ := got[0]["project"].(string); !strings.Contains(p, "web") {
		t.Errorf("filtered session has wrong project: %q", p)
	}
}

// TestToolSelectionKeepsConfigDirWorking: --config-dir has only ever named the
// Claude Code directory. Now that commands default to every tool, an unset --tool
// beside a --config-dir has to still mean Claude Code, or every script and CI job
// that passes one starts failing.
func TestToolSelectionKeepsConfigDirWorking(t *testing.T) {
	for _, c := range []struct {
		name      string
		tool      string
		configDir string
		want      string
	}{
		{"nothing given lists every tool", "", "", toolAll},
		{"a config dir alone means Claude Code", "", "/tmp/x", string(agent.ClaudeCode)},
		{"an explicit tool is obeyed", "codex", "", "codex"},
		{"an explicit tool wins beside a config dir", "codex", "/tmp/x", "codex"},
		{"asking for all is not silently narrowed", toolAll, "", toolAll},
	} {
		if got := toolSelection(c.tool, c.configDir); got != c.want {
			t.Errorf("%s: toolSelection(%q, %q) = %q, want %q", c.name, c.tool, c.configDir, got, c.want)
		}
	}
}

// TestConfigDirWithAllToolsIsRefused: naming both is a contradiction - one
// directory cannot be the override for three tools - so it must be reported rather
// than quietly applied to one of them.
func TestConfigDirWithAllToolsIsRefused(t *testing.T) {
	_, _, err := listFor(toolAll, t.TempDir())
	if err == nil {
		t.Fatal("--config-dir with --tool all was accepted; it has no coherent meaning")
	}
	if !strings.Contains(err.Error(), "--tool") {
		t.Errorf("the error should say what to do instead, got: %v", err)
	}
}

// TestVerifyRejectsAFileArgument: `verify bundle.tgz` used to ignore the file and
// report on the local install instead, which reads as the bundle passing.
func TestVerifyRejectsAFileArgument(t *testing.T) {
	err := Run([]string{"verify", "some-bundle.tgz"})
	if err == nil {
		t.Fatal("verify accepted a file argument and reported on something else")
	}
	if !strings.Contains(err.Error(), "inspect") {
		t.Errorf("the error should point at the command that does read a bundle, got: %v", err)
	}
}
