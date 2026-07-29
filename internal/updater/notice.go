package updater

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The update check in this file is deliberately passive, and every rule it
// follows exists because the obvious version of the feature is worse.
//
// It never delays a command. The check runs after the work is done and only
// records what it found; the notice appears the next time entangle is run. A
// version that fetched first would put a network round trip in front of the
// user's own session list, which is the one thing that must stay instant.
//
// It never speaks on stdout. `entangle sessions --json` is parsed by the editor
// extension, so a line of prose in that stream is a broken integration, not a
// cosmetic problem.
//
// It says nothing when it has nothing to say, and nothing when it fails. A user
// on a plane must not be shown a network error they did not ask for.

const (
	// checkInterval is how long a cached answer is trusted. Releases are not
	// frequent enough for anything shorter to tell the user something new, and
	// GitHub's unauthenticated rate limit is shared with everything else on the
	// machine's IP address.
	checkInterval = 24 * time.Hour

	// refreshTimeout bounds the post-command check. It is short because nothing
	// depends on the answer: missing it means the notice arrives a day later.
	refreshTimeout = 3 * time.Second

	// NoCheckEnv turns the check off completely, including the network call.
	NoCheckEnv = "ENTANGLE_NO_UPDATE_CHECK"
)

// userCacheDir and fetchLatest are variables so the caching and back-off rules
// can be tested against a disposable directory and without a network request.
var (
	userCacheDir = os.UserCacheDir
	fetchLatest  = LatestVersion
)

// checkState is what the last check recorded.
//
// LastCheck is written even when the request failed, so an offline machine backs
// off for a day instead of retrying on every single command.
type checkState struct {
	LastCheck time.Time `json:"lastCheck"`
	Latest    string    `json:"latest"`
}

// CheckDisabled reports whether the user has opted out.
func CheckDisabled() bool { return os.Getenv(NoCheckEnv) != "" }

func statePath() (string, error) {
	dir, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "entangle", "update-check.json"), nil
}

func readState() checkState {
	var s checkState
	p, err := statePath()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	// A corrupt or half-written cache is treated as no cache. There is nothing
	// here worth recovering and nothing worth reporting.
	_ = json.Unmarshal(b, &s)
	return s
}

func writeState(s checkState) error {
	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// Written via a temporary file and renamed, so a process killed mid-write
	// cannot leave a truncated cache behind for the next run to parse.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".update-check-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, p); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// comparableVersion reports whether current is a real release version that can
// be compared against a tag.
//
// A build from source reports "dev", which parses as 0.0.0 and would therefore
// be "behind" every release forever. Nagging someone who is running their own
// build to install an older one is worse than saying nothing.
func comparableVersion(current string) bool {
	v := strings.TrimPrefix(strings.TrimSpace(current), "v")
	if v == "" || v == "dev" {
		return false
	}
	parts := parse(v)
	if len(parts) == 0 {
		return false
	}
	for _, n := range parts {
		if n != 0 {
			return true
		}
	}
	// All zeroes means nothing recognisable was parsed out of it.
	return false
}

// AvailableUpdate returns the newer version the last check found, without a
// leading "v", or "" when there is nothing to report. It only reads the cache,
// so it never blocks.
func AvailableUpdate(current string) string {
	if CheckDisabled() || !comparableVersion(current) {
		return ""
	}
	s := readState()
	if s.Latest == "" || !Newer(s.Latest, current) {
		return ""
	}
	return strings.TrimPrefix(s.Latest, "v")
}

// RefreshCheck updates the cache if it is older than checkInterval. It is meant
// to be called after a command has finished its real work, and reports nothing:
// every failure here is one the user should never hear about.
func RefreshCheck(repo, current string) {
	if CheckDisabled() || !comparableVersion(current) {
		return
	}
	s := readState()
	if time.Since(s.LastCheck) < checkInterval {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	latest, err := fetchLatest(ctx, repo)
	s.LastCheck = time.Now()
	if err == nil {
		s.Latest = latest
	}
	_ = writeState(s)
}

// ManagedByHomebrew reports whether the running binary sits inside a Homebrew
// keg.
//
// It matters because `entangle update` replaces the executable in place. Inside
// a keg that succeeds and leaves Homebrew's record of the installed version
// disagreeing with the file on disk, so `brew upgrade` afterwards can put an
// older build back. Those users need `brew upgrade entangle` instead, and the
// notice has to say so rather than offer a command that quietly breaks their
// install.
func ManagedByHomebrew() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return isCellarPath(exe)
}

// isCellarPath is split out so the rule can be tested against the paths brew
// actually uses, rather than only against wherever the test binary happens to
// live. Every formula installs under <prefix>/Cellar/<name>/<version>/, whatever
// the prefix is, so this one check holds for Intel, Apple silicon and linuxbrew.
func isCellarPath(exe string) bool {
	return strings.Contains(filepath.ToSlash(exe), "/Cellar/")
}

// UpgradeCommand is what this particular installation should actually run to
// upgrade.
func UpgradeCommand() string {
	if ManagedByHomebrew() {
		return "brew upgrade entangle"
	}
	return "entangle update"
}
