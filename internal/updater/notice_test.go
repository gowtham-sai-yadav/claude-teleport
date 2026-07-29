package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// isolate points the check's cache at a throwaway directory and clears the
// opt-out, so a developer's own cache and environment cannot change the result.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := userCacheDir
	userCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDir = old })
	t.Setenv(NoCheckEnv, "")
	return dir
}

// stubLatest replaces the network call for the duration of a test.
func stubLatest(t *testing.T, tag string, err error) *int {
	t.Helper()
	calls := 0
	old := fetchLatest
	fetchLatest = func(context.Context, string) (string, error) {
		calls++
		return tag, err
	}
	t.Cleanup(func() { fetchLatest = old })
	return &calls
}

// TestRefreshThenReportIsWhatTheUserSees: the whole feature in one pass. The
// command finishes, the check records what it found, and the next run reads it
// back without touching the network - which is the only reason the notice can be
// free at the point it is shown.
func TestRefreshThenReportIsWhatTheUserSees(t *testing.T) {
	dir := isolate(t)
	calls := stubLatest(t, "v0.9.0", nil)

	if got := AvailableUpdate("0.6.0"); got != "" {
		t.Fatalf("with an empty cache AvailableUpdate = %q, want none", got)
	}
	RefreshCheck("", "0.6.0")
	if *calls != 1 {
		t.Fatalf("refresh made %d requests, want 1", *calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "entangle", "update-check.json")); err != nil {
		t.Fatalf("refresh wrote no cache: %v", err)
	}
	if got := AvailableUpdate("0.6.0"); got != "0.9.0" {
		t.Errorf("AvailableUpdate = %q, want 0.9.0 without the leading v", got)
	}
	// Already on it: the same cache must now say nothing at all.
	if got := AvailableUpdate("0.9.0"); got != "" {
		t.Errorf("AvailableUpdate = %q for the newest version, want none", got)
	}
	if got := AvailableUpdate("1.2.0"); got != "" {
		t.Errorf("AvailableUpdate = %q for a version ahead of the release, want none", got)
	}
}

// TestFreshCacheIsNotRefetched: without this the check would hit GitHub on every
// single command, which is both slow and a fast route to a rate limit shared
// with everything else on the machine's IP.
func TestFreshCacheIsNotRefetched(t *testing.T) {
	isolate(t)
	calls := stubLatest(t, "v0.9.0", nil)
	RefreshCheck("", "0.6.0")
	RefreshCheck("", "0.6.0")
	RefreshCheck("", "0.6.0")
	if *calls != 1 {
		t.Errorf("made %d requests for one day's window, want 1", *calls)
	}
}

// TestStaleCacheIsRefetched: the flip side - a cache older than the window has
// to be renewed, or a user who ran entangle once a year ago never hears about
// anything again.
func TestStaleCacheIsRefetched(t *testing.T) {
	isolate(t)
	if err := writeState(checkState{LastCheck: time.Now().Add(-checkInterval - time.Minute), Latest: "v0.7.0"}); err != nil {
		t.Fatal(err)
	}
	calls := stubLatest(t, "v0.9.0", nil)
	RefreshCheck("", "0.6.0")
	if *calls != 1 {
		t.Fatalf("stale cache made %d requests, want 1", *calls)
	}
	if got := AvailableUpdate("0.6.0"); got != "0.9.0" {
		t.Errorf("AvailableUpdate = %q after refresh, want 0.9.0", got)
	}
}

// TestFailedCheckBacksOffAndStaysQuiet: an offline machine must not retry on
// every command, and must not be told about a network error it never asked for.
// The timestamp is therefore recorded even though the fetch failed.
func TestFailedCheckBacksOffAndStaysQuiet(t *testing.T) {
	isolate(t)
	calls := stubLatest(t, "", errors.New("dial tcp: no route to host"))
	RefreshCheck("", "0.6.0")
	RefreshCheck("", "0.6.0")
	if *calls != 1 {
		t.Errorf("a failed check retried %d times, want 1 then back off", *calls)
	}
	if got := AvailableUpdate("0.6.0"); got != "" {
		t.Errorf("AvailableUpdate = %q after a failed check, want none", got)
	}
}

// TestPreviousAnswerSurvivesAFailedCheck: losing the network should not lose the
// notice. The last known release stays in the cache when the refresh fails.
func TestPreviousAnswerSurvivesAFailedCheck(t *testing.T) {
	isolate(t)
	if err := writeState(checkState{LastCheck: time.Now().Add(-checkInterval - time.Minute), Latest: "v0.9.0"}); err != nil {
		t.Fatal(err)
	}
	stubLatest(t, "", errors.New("offline"))
	RefreshCheck("", "0.6.0")
	if got := AvailableUpdate("0.6.0"); got != "0.9.0" {
		t.Errorf("AvailableUpdate = %q, want the cached 0.9.0 to survive", got)
	}
}

// TestOptOutStopsEverything: ENTANGLE_NO_UPDATE_CHECK has to kill the request as
// well as the message. An opt-out that still phoned home would be a lie.
func TestOptOutStopsEverything(t *testing.T) {
	isolate(t)
	// Seeded through writeState rather than through RefreshCheck: a cache written
	// by a real refresh is fresh, and the staleness guard would then return early
	// on its own, so the test would pass whether or not the opt-out is honoured.
	// The timestamp is backdated for the same reason.
	if err := writeState(checkState{LastCheck: time.Now().Add(-checkInterval - time.Minute), Latest: "v0.9.0"}); err != nil {
		t.Fatal(err)
	}
	if got := AvailableUpdate("0.6.0"); got != "0.9.0" {
		t.Fatalf("AvailableUpdate = %q before opting out, want 0.9.0", got)
	}

	t.Setenv(NoCheckEnv, "1")
	calls := stubLatest(t, "v1.0.0", nil)
	RefreshCheck("", "0.6.0")
	if *calls != 0 {
		t.Errorf("made %d requests while opted out, want 0", *calls)
	}
	if got := AvailableUpdate("0.6.0"); got != "" {
		t.Errorf("AvailableUpdate = %q while opted out, want none", got)
	}
}

// TestBuiltFromSourceIsNeverNagged: `dev` parses as 0.0.0, so the naive version
// of this tells everyone building from source that they are behind and should
// install an older binary. It must stay silent and never even ask GitHub.
func TestBuiltFromSourceIsNeverNagged(t *testing.T) {
	isolate(t)
	if err := writeState(checkState{LastCheck: time.Now(), Latest: "v0.9.0"}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"dev", "", "  ", "unknown"} {
		if got := AvailableUpdate(v); got != "" {
			t.Errorf("AvailableUpdate(%q) = %q, want none for a non-release build", v, got)
		}
		calls := stubLatest(t, "v0.9.0", nil)
		RefreshCheck("", v)
		if *calls != 0 {
			t.Errorf("version %q triggered %d requests, want 0", v, *calls)
		}
	}
}

// TestCorruptCacheIsIgnored: a half-written file is treated as no cache. It must
// not crash and must not report a version it cannot actually vouch for.
func TestCorruptCacheIsIgnored(t *testing.T) {
	dir := isolate(t)
	p := filepath.Join(dir, "entangle")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "update-check.json"), []byte(`{"latest": "v0.9`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := AvailableUpdate("0.6.0"); got != "" {
		t.Errorf("AvailableUpdate = %q from a truncated cache, want none", got)
	}
	// And a truncated cache counts as no cache, so the next refresh goes ahead.
	calls := stubLatest(t, "v0.9.0", nil)
	RefreshCheck("", "0.6.0")
	if *calls != 1 {
		t.Errorf("a corrupt cache blocked the refresh (%d requests, want 1)", *calls)
	}
}

// TestUpgradeCommandMatchesHowThisCopyWasInstalled: the notice offers a command
// to run, so the wrong one is worse than none. A Homebrew keg needs `brew
// upgrade`, because replacing the file in place leaves brew's records stale and
// a later upgrade can put an older build back.
func TestUpgradeCommandMatchesHowThisCopyWasInstalled(t *testing.T) {
	// The test binary lives in a temp directory, never a keg.
	if ManagedByHomebrew() {
		t.Skip("test binary unexpectedly sits inside a Cellar path")
	}
	if got := UpgradeCommand(); got != "entangle update" {
		t.Errorf("UpgradeCommand() = %q for a plain install, want \"entangle update\"", got)
	}
	if !isCellarPath("/opt/homebrew/Cellar/entangle/0.6.0/bin/entangle") {
		t.Error("an Apple silicon keg path is not recognised as Homebrew")
	}
	if !isCellarPath("/home/linuxbrew/.linuxbrew/Cellar/entangle/0.6.0/bin/entangle") {
		t.Error("a linuxbrew keg path is not recognised as Homebrew")
	}
	if isCellarPath("/usr/local/bin/entangle") {
		t.Error("a plain /usr/local/bin install is mistaken for Homebrew")
	}
}
