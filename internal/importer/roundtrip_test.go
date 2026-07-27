package importer

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/exporter"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/paths"
)

// These tests cover the thing the product actually promises: pack a machine up,
// unpack it somewhere else, and have the sessions be resumable. Everything below
// Import - deciding destinations, rewriting the paths baked into transcripts,
// merging config without clobbering it, refusing to escape the target directory -
// runs only here. It writes into temp dirs exclusively; no real Claude data is
// touched.

const testSessionID = "abcdef01-2345-6789-abcd-ef0123456789"

// sourceMachine builds a config dir that looks like a used Claude install whose
// projects live under a foreign home, so import has real remapping to do.
func sourceMachine(t *testing.T) (configDir, projectPath, encoded string) {
	t.Helper()
	configDir = t.TempDir()
	projectPath = "/home/olduser/work/api"
	encoded = paths.Encode(projectPath)

	proj := filepath.Join(configDir, "projects", encoded)
	if err := os.MkdirAll(filepath.Join(proj, testSessionID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A transcript mentions its cwd in the header event and again in prose, so we
	// can tell a cwd rewrite apart from a blanket search-and-replace.
	transcript := `{"type":"user","cwd":"` + projectPath + `","sessionId":"` + testSessionID + `","message":{"role":"user","content":"look at ` + projectPath + `/main.go"}}` + "\n" +
		`{"type":"assistant","cwd":"` + projectPath + `","message":{"role":"assistant","content":"done"}}` + "\n"
	write(filepath.Join(proj, testSessionID+".jsonl"), transcript)
	write(filepath.Join(proj, testSessionID, "sub.jsonl"), transcript)
	write(filepath.Join(proj, "memory", "MEMORY.md"), "# project notes\n")
	write(filepath.Join(configDir, "settings.json"), `{"theme":"dark"}`)
	write(filepath.Join(configDir, "history.jsonl"), `{"display":"hi","project":"`+projectPath+`"}`+"\n")
	write(filepath.Join(configDir, ".claude.json"),
		`{"projects":{"`+projectPath+`":{"allowedTools":["Bash"]}},"oauthAccount":{"accessToken":"SHOULD-NOT-TRAVEL"}}`)
	return configDir, projectPath, encoded
}

// exportBundle packs the source machine and returns the bundle path.
func exportBundle(t *testing.T, configDir string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "backup.tgz")
	if _, err := exporter.Run(exporter.Options{ConfigDir: configDir, Version: "test", Out: out}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return out
}

// importInto runs the full non-printing import pipeline into a fresh config dir.
func importInto(t *testing.T, bundlePath, targetConfig, targetHome string) *RunResult {
	t.Helper()
	man, err := LoadManifest(bundlePath)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	tp, err := claudedir.Locate(targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Bundle: bundlePath, ConfigDir: targetConfig, TargetHome: targetHome, AssumeYes: true}
	plan := BuildPlan(man, tp, opts)
	res, err := Import(tp, plan, opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return res
}

// TestExportImportRoundTrip is the end-to-end promise: a session packed on one
// machine lands on another with its paths corrected and is reported resume-ready.
func TestExportImportRoundTrip(t *testing.T) {
	srcConfig, oldPath, _ := sourceMachine(t)
	bundlePath := exportBundle(t, srcConfig)

	targetConfig := t.TempDir()
	targetHome := t.TempDir()
	res := importInto(t, bundlePath, targetConfig, targetHome)

	// The project should have been re-homed under the new machine's home.
	newPath := filepath.Join(targetHome, "work", "api")
	newEnc := paths.Encode(newPath)
	projDir := filepath.Join(targetConfig, "projects", newEnc)

	transcript := filepath.Join(projDir, testSessionID+".jsonl")
	if _, err := os.Stat(transcript); err != nil {
		entries, _ := os.ReadDir(filepath.Join(targetConfig, "projects"))
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		t.Fatalf("transcript not at %s (folders present: %v)", transcript, got)
	}
	for _, p := range []string{
		filepath.Join(projDir, testSessionID, "sub.jsonl"),
		filepath.Join(projDir, "memory", "MEMORY.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing after import: %s", p)
		}
	}

	// The cwd baked into the transcript must now name the new location, or
	// Claude Code would resume against a directory that does not exist.
	body, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"cwd":"`+newPath+`"`) {
		t.Errorf("cwd was not rewritten to %q:\n%s", newPath, firstLine(string(body)))
	}
	if strings.Contains(string(body), `"cwd":"`+oldPath+`"`) {
		t.Errorf("old cwd %q still present after import", oldPath)
	}
	// Without --deep, prose mentions are deliberately left alone.
	if !strings.Contains(string(body), oldPath+"/main.go") {
		t.Errorf("shallow import should not rewrite prose mentions of the old path")
	}

	// Config should be merged and re-keyed to the new path, and the credential
	// in the source .claude.json must never have travelled.
	cj, err := os.ReadFile(filepath.Join(targetConfig, ".claude.json"))
	if err != nil {
		t.Fatalf("no .claude.json written: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(cj, &parsed); err != nil {
		t.Fatalf("merged .claude.json is not valid JSON: %v", err)
	}
	projects, _ := parsed["projects"].(map[string]any)
	if _, ok := projects[newPath]; !ok {
		t.Errorf(".claude.json projects not re-keyed to %q, got keys: %v", newPath, keysOf(projects))
	}
	if strings.Contains(string(cj), "SHOULD-NOT-TRAVEL") {
		t.Error("a credential from the source .claude.json reached the target - export must not carry it")
	}

	// And the tool's own verdict.
	if res.Written == 0 {
		t.Error("Written = 0, expected files to be written")
	}
	if res.Rejected != 0 {
		t.Errorf("Rejected = %d, expected none", res.Rejected)
	}
	okCount := 0
	for _, v := range res.Verify {
		if v.OK {
			okCount++
		} else {
			t.Errorf("project not resume-ready: %s (%s)", v.Folder, v.Detail)
		}
	}
	if okCount == 0 {
		t.Error("no project reported resume-ready")
	}
}

// TestImportIsNonDestructive: a second import must not clobber existing work.
func TestImportIsNonDestructive(t *testing.T) {
	srcConfig, _, _ := sourceMachine(t)
	bundlePath := exportBundle(t, srcConfig)

	targetConfig := t.TempDir()
	targetHome := t.TempDir()
	importInto(t, bundlePath, targetConfig, targetHome)

	newEnc := paths.Encode(filepath.Join(targetHome, "work", "api"))
	transcript := filepath.Join(targetConfig, "projects", newEnc, testSessionID+".jsonl")

	// Simulate the user having continued the conversation locally.
	local := "LOCAL EDIT THAT MUST SURVIVE\n"
	if err := os.WriteFile(transcript, []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	res := importInto(t, bundlePath, targetConfig, targetHome)
	body, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != local {
		t.Errorf("re-import overwrote an existing transcript without --overwrite:\n%s", firstLine(string(body)))
	}
	if res.Skipped == 0 {
		t.Errorf("expected the existing file to be reported as skipped, Skipped = 0")
	}
}

// TestImportRejectsNewerSchema is the forward-compatibility gate: a bundle from a
// future release must be refused with an actionable message rather than
// half-applied into someone's home directory.
func TestImportRejectsNewerSchema(t *testing.T) {
	srcConfig, _, _ := sourceMachine(t)
	bundlePath := exportBundle(t, srcConfig)

	// Rewrite the manifest inside the bundle with an impossible schema version.
	future := rewriteManifest(t, bundlePath, func(m map[string]any) {
		m["schemaVersion"] = manifest.MaxSupportedSchema + 1
	})

	_, err := LoadManifest(future)
	if err == nil {
		t.Fatal("expected a newer-schema bundle to be refused")
	}
	var schemaErr *manifest.UnsupportedSchemaError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("want UnsupportedSchemaError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "update") {
		t.Errorf("message should tell the user how to fix it, got: %v", err)
	}
}

// TestLegacyManifestDefaults: bundles already on people's disks predate later
// fields. Absent values must keep meaning what they meant when written.
func TestLegacyManifestDefaults(t *testing.T) {
	// A v0.5.x session manifest, verbatim in shape: no schemaVersion gap, no
	// fields added after it shipped.
	legacy := `{
	  "tool": "claude-teleport",
	  "toolVersion": "0.5.0",
	  "schemaVersion": 1,
	  "createdAt": "2026-07-01T00:00:00Z",
	  "kind": "session",
	  "sessionId": "` + testSessionID + `",
	  "redacted": true,
	  "source": {"os": "darwin", "home": "/Users/old"},
	  "includes": ["projects/"],
	  "projects": [{"originalPath": "/Users/old/proj", "encodedFolder": "-Users-old-proj", "sessions": 1, "pathSource": "session"}]
	}`
	var man manifest.Manifest
	if err := json.Unmarshal([]byte(legacy), &man); err != nil {
		t.Fatalf("a released manifest must still parse: %v", err)
	}
	if err := man.Unsupported(); err != nil {
		t.Errorf("a v0.5.x bundle must remain readable: %v", err)
	}
	if !man.IsSession() {
		t.Error("kind=session should be recognised")
	}

	// And a manifest with no kind at all still means "full backup".
	var old manifest.Manifest
	if err := json.Unmarshal([]byte(`{"tool":"claude-teleport","schemaVersion":1,"source":{"os":"linux"}}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.IsSession() {
		t.Error(`an absent "kind" must mean a full backup, not a session`)
	}
	if err := old.Unsupported(); err != nil {
		t.Errorf("an early bundle must remain readable: %v", err)
	}
}

// TestSharedSessionAttachesToCwd covers a bug: the "put a received session in the
// folder I am standing in" rule used to live in the CLI wrapper only, so a session
// received through the TUI or the web GUI silently landed under the sender's
// remapped path while the UI claimed it was here. The rule belongs to plan
// building, which every entry point goes through, so this drives BuildPlan
// directly - the same way those two callers do.
func TestSharedSessionAttachesToCwd(t *testing.T) {
	srcConfig, srcProject, _ := sourceMachine(t)

	// Pack a single session, the shape `send`/`share` produces.
	b, err := exporter.PrepareShare(exporter.ShareOptions{
		ConfigDir: srcConfig, Version: "test", SessionPrefix: testSessionID, Redact: true,
	})
	if err != nil {
		t.Fatalf("PrepareShare: %v", err)
	}
	bundlePath := filepath.Join(t.TempDir(), "session.tgz")
	f, err := os.Create(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteBundle(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Stand in a project directory, as the receiving teammate would.
	here := t.TempDir()
	t.Chdir(here)

	man, err := LoadManifest(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	targetConfig := t.TempDir()
	tp, err := claudedir.Locate(targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Bundle: bundlePath, ConfigDir: targetConfig, AssumeYes: true}
	plan := BuildPlan(man, tp, opts)

	if plan.AttachedTo == "" {
		t.Fatal("BuildPlan did not attach the shared session to the current directory")
	}
	if _, err := Import(tp, plan, opts); err != nil {
		t.Fatalf("Import: %v", err)
	}

	// t.Chdir may hand back a symlinked path (/var vs /private/var on macOS), so
	// compare against what the plan actually resolved to.
	wantEnc := paths.Encode(plan.AttachedTo)
	landed := filepath.Join(targetConfig, "projects", wantEnc, testSessionID+".jsonl")
	if _, err := os.Stat(landed); err != nil {
		entries, _ := os.ReadDir(filepath.Join(targetConfig, "projects"))
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		t.Fatalf("session did not land in the current directory.\nwanted folder %s\ngot folders  %v", wantEnc, got)
	}
	body, err := os.ReadFile(landed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"cwd":"`+srcProject+`"`) {
		t.Errorf("transcript still points at the sender's path %q", srcProject)
	}
	if !strings.Contains(string(body), `"cwd":"`+plan.AttachedTo+`"`) {
		t.Errorf("transcript cwd was not rewritten to %q:\n%s", plan.AttachedTo, firstLine(string(body)))
	}
}

// rewriteManifest copies a bundle, letting the caller mutate manifest.json, so a
// test can forge a bundle this build should refuse.
func rewriteManifest(t *testing.T, src string, mutate func(map[string]any)) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "rewritten.tgz")
	w, err := bundle.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	err = bundle.ForEach(src, func(h *tar.Header, r io.Reader) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if h.Name == "manifest.json" {
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				return err
			}
			mutate(m)
			if data, err = json.Marshal(m); err != nil {
				return err
			}
		}
		return w.AddBytes(h.Name, data)
	})
	if err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
