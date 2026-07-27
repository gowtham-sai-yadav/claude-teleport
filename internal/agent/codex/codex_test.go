package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
)

// Fixtures mirror the real rollout shape, which was established by reading actual
// Codex files: an outer {timestamp,type,payload} envelope, session_meta carrying
// cwd, and the same prompt recorded twice - once as an event_msg with plain text
// and once as a response_item with structured blocks.

func metaLine(id, cwd string) string {
	return `{"timestamp":"2026-07-01T10:00:00.000Z","type":"session_meta","payload":{"session_id":"` + id +
		`","id":"` + id + `","timestamp":"2026-07-01T10:00:00.000Z","cwd":"` + cwd +
		`","originator":"codex-tui","cli_version":"0.145.0","source":"cli","model_provider":"openai",` +
		`"base_instructions":{"text":"you are codex"}}}`
}

func userEvent(text string) string {
	return `{"timestamp":"2026-07-01T10:00:01.000Z","type":"event_msg","payload":{"type":"user_message","message":"` + text + `","images":[]}}`
}

func userItem(text string) string {
	return `{"timestamp":"2026-07-01T10:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}`
}

func assistantItem(text string) string {
	return `{"timestamp":"2026-07-01T10:00:02.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}}`
}

func turnContext(model string) string {
	return `{"timestamp":"2026-07-01T10:00:00.500Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"/x","model":"` + model + `","approval_policy":"on-request"}}`
}

func reasoningItem() string {
	return `{"timestamp":"2026-07-01T10:00:01.500Z","type":"response_item","payload":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"gAAAAABopaque"}}`
}

// writeRollout drops a rollout into sessions/<date>/, compressing when asked. The
// date is given as "YYYY/MM/DD" and split into real path elements, so this builds
// native separators instead of mixing them on Windows.
func writeRollout(t *testing.T, root, date, name string, lines []string, compress bool) string {
	t.Helper()
	parts := append([]string{root, "sessions"}, strings.Split(date, "/")...)
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(lines, "\n") + "\n"
	path := filepath.Join(dir, name)
	if compress {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		enc, err := zstd.NewWriter(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := enc.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
		if err := enc.Close(); err != nil {
			t.Fatal(err)
		}
		f.Close()
		return path
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func locate(t *testing.T, root string) agent.Roots {
	t.Helper()
	r, ok, err := Provider{}.Locate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("an explicit override should always be present")
	}
	return r
}

func TestRegisteredAndIdentity(t *testing.T) {
	p, ok := agent.Get(agent.Codex)
	if !ok {
		t.Fatal("importing this package should register the Codex provider")
	}
	if p.DisplayName() == "" {
		t.Error("DisplayName must be set")
	}
}

// TestLocatePresence: a config dir alone means "logged in", not "has history", so
// presence keys off sessions/.
func TestLocatePresence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)

	if _, ok, err := (Provider{}).Locate(""); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("a codex home with no sessions/ should report not-present")
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, ok, err := (Provider{}).Locate("")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("once sessions/ exists it should report present")
	}
	if r.Get(KeySessionsDir) != filepath.Join(root, "sessions") {
		t.Errorf("sessionsDir = %q", r.Get(KeySessionsDir))
	}
}

func TestListSessionsParsesRollout(t *testing.T) {
	root := t.TempDir()
	id := "019f11e3-06e6-7d40-bd91-6600ac441e97"
	writeRollout(t, root, "2026/07/01", "rollout-2026-07-01T10-00-00-"+id+".jsonl", []string{
		metaLine(id, "/Users/dev/api"),
		turnContext("gpt-5.5"),
		userEvent("refactor the auth layer"),
		userItem("refactor the auth layer"),
		reasoningItem(),
		assistantItem("on it"),
		userItem("now add tests"),
		assistantItem("done"),
	}, false)

	got, err := Provider{}.ListSessions(locate(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 session, got %d", len(got))
	}
	s := got[0]
	if s.Provider != agent.Codex {
		t.Errorf("Provider = %q", s.Provider)
	}
	if s.ID != id {
		t.Errorf("ID = %q, want %q", s.ID, id)
	}
	if s.ProjectPath != "/Users/dev/api" {
		t.Errorf("ProjectPath = %q (must come from session_meta.cwd, since Codex buckets by date)", s.ProjectPath)
	}
	if s.Title != "refactor the auth layer" {
		t.Errorf("Title = %q", s.Title)
	}
	// Four response_item messages with role user/assistant; the doubled event_msg
	// copies and the reasoning item must not inflate the count.
	if s.Messages != 4 {
		t.Errorf("Messages = %d, want 4", s.Messages)
	}
	if s.GroupKey != "2026/07/01" {
		t.Errorf("GroupKey = %q, want the date bucket", s.GroupKey)
	}
	ref, ok := s.Ref.(Ref)
	if !ok {
		t.Fatalf("Ref = %T, want codex.Ref", s.Ref)
	}
	if ref.Model != "gpt-5.5" {
		t.Errorf("Ref.Model = %q, want gpt-5.5", ref.Model)
	}
	if ref.CLIVersion != "0.145.0" {
		t.Errorf("Ref.CLIVersion = %q", ref.CLIVersion)
	}
	if ref.Path == "" {
		t.Error("Ref.Path must point at the rollout")
	}
}

// TestTitleFromResponseItemOnly covers Codex's paginated history mode, where the
// event_msg copy of the prompt is absent.
func TestTitleFromResponseItemOnly(t *testing.T) {
	root := t.TempDir()
	id := "019f2222-06e6-7d40-bd91-6600ac441e97"
	writeRollout(t, root, "2026/07/02", "rollout-2026-07-02T10-00-00-"+id+".jsonl", []string{
		metaLine(id, "/w"),
		userItem("only structured form here"),
	}, false)

	got, err := Provider{}.ListSessions(locate(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "only structured form here" {
		t.Fatalf("title not recovered from response_item: %+v", got)
	}
}

// TestTitleSkipsMachineMarkers: Codex injects markers like <EXTERNAL SESSION
// IMPORTED>, which must not become the session's name.
func TestTitleSkipsMachineMarkers(t *testing.T) {
	root := t.TempDir()
	id := "019f3333-06e6-7d40-bd91-6600ac441e97"
	writeRollout(t, root, "2026/07/03", "rollout-2026-07-03T10-00-00-"+id+".jsonl", []string{
		metaLine(id, "/w"),
		userEvent("<EXTERNAL SESSION IMPORTED>\\nthe actual question"),
	}, false)

	got, err := Provider{}.ListSessions(locate(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if strings.HasPrefix(got[0].Title, "<") {
		t.Errorf("Title = %q, should skip the injected marker line", got[0].Title)
	}
}

// TestCompressedRolloutIsRead: Codex compresses cold rollouts. Skipping them would
// silently hide a user's older sessions, which is worse than being slow.
func TestCompressedRolloutIsRead(t *testing.T) {
	root := t.TempDir()
	id := "019f4444-06e6-7d40-bd91-6600ac441e97"
	writeRollout(t, root, "2026/06/01", "rollout-2026-06-01T10-00-00-"+id+".jsonl.zst", []string{
		metaLine(id, "/Users/dev/cold"),
		userEvent("an old conversation"),
		assistantItem("ok"),
	}, true)

	got, err := Provider{}.ListSessions(locate(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("compressed rollout was not read (got %d sessions)", len(got))
	}
	if got[0].Title != "an old conversation" || got[0].ProjectPath != "/Users/dev/cold" {
		t.Errorf("compressed rollout parsed wrong: %+v", got[0])
	}
	if ref, _ := got[0].Ref.(Ref); !ref.Compressed {
		t.Error("Ref.Compressed should be true for a .zst rollout")
	}
}

// TestUnusableFilesAreSkipped: one bad file must not hide every other session.
func TestUnusableFilesAreSkipped(t *testing.T) {
	root := t.TempDir()
	good := "019f5555-06e6-7d40-bd91-6600ac441e97"
	writeRollout(t, root, "2026/07/04", "rollout-2026-07-04T10-00-00-"+good+".jsonl", []string{
		metaLine(good, "/w"), userEvent("fine"),
	}, false)
	// No session_meta: not something Codex would resume either.
	writeRollout(t, root, "2026/07/04", "rollout-2026-07-04T11-00-00-019f6666-06e6-7d40-bd91-6600ac441e97.jsonl", []string{
		userEvent("orphan"),
	}, false)
	// Not JSON at all.
	writeRollout(t, root, "2026/07/04", "rollout-2026-07-04T12-00-00-019f7777-06e6-7d40-bd91-6600ac441e97.jsonl", []string{
		"this is not json", "neither is this",
	}, false)
	// Not a rollout filename.
	writeRollout(t, root, "2026/07/04", "notes.txt", []string{"ignore me"}, false)

	got, err := Provider{}.ListSessions(locate(t, root))
	if err != nil {
		t.Fatalf("a bad file must not fail the listing: %v", err)
	}
	if len(got) != 1 || got[0].ID != good {
		var ids []string
		for _, s := range got {
			ids = append(ids, s.ID)
		}
		t.Fatalf("want only the good session, got %v", ids)
	}
}

// TestShortIDsUniqueForTimeOrderedIDs is the regression test for a bug found
// against real data: Codex ids are UUIDv7, so two sessions started moments apart
// share a leading prefix and a fixed 8-character handle named both of them.
func TestShortIDsUniqueForTimeOrderedIDs(t *testing.T) {
	root := t.TempDir()
	// Differ only from the 12th character, exactly like real sibling sessions.
	a := "019f9e34-be2a-7000-8000-000000000001"
	b := "019f9e34-be7f-7000-8000-000000000002"
	writeRollout(t, root, "2026/07/26", "rollout-2026-07-26T17-00-00-"+a+".jsonl", []string{metaLine(a, "/w"), userEvent("first")}, false)
	writeRollout(t, root, "2026/07/26", "rollout-2026-07-26T17-00-05-"+b+".jsonl", []string{metaLine(b, "/w"), userEvent("second")}, false)

	got, err := Provider{}.ListSessions(locate(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(got))
	}
	if got[0].ShortID == got[1].ShortID {
		t.Fatalf("both sessions got the same handle %q", got[0].ShortID)
	}
	// The handle must remain a real prefix, since users paste it back as an id.
	for _, s := range got {
		if !strings.HasPrefix(s.ID, s.ShortID) {
			t.Errorf("ShortID %q is not a prefix of ID %q", s.ShortID, s.ID)
		}
	}
}

func TestIDFromFilename(t *testing.T) {
	id := "019f11e3-06e6-7d40-bd91-6600ac441e97"
	cases := map[string]string{
		"rollout-2026-06-29T11-08-38-" + id + ".jsonl":     id,
		"rollout-2026-06-29T11-08-38-" + id + ".jsonl.zst": id,
		"rollout-nope.jsonl":                               "",
		"rollout-2026-06-29T11-08-38-not-a-uuid-x.jsonl":   "",
	}
	for name, want := range cases {
		if got := idFromFilename(name); got != want {
			t.Errorf("idFromFilename(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestIsRollout(t *testing.T) {
	yes := []string{"rollout-x.jsonl", "rollout-x.jsonl.zst"}
	no := []string{"notes.txt", "rollout-x.json", "x.jsonl", "rollout-x.jsonl.gz"}
	for _, n := range yes {
		if !isRollout(n) {
			t.Errorf("isRollout(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isRollout(n) {
			t.Errorf("isRollout(%q) = true, want false", n)
		}
	}
}

// TestMissingSessionsDirIsEmptyNotError: a machine without Codex must list
// nothing quietly.
func TestMissingSessionsDirIsEmptyNotError(t *testing.T) {
	r := agent.Roots{Extra: map[string]string{KeySessionsDir: filepath.Join(t.TempDir(), "absent")}}
	got, err := Provider{}.ListSessions(r)
	if err != nil {
		t.Errorf("missing sessions dir should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want no sessions, got %d", len(got))
	}
}
