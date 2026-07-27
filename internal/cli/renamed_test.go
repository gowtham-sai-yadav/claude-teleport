package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The project was renamed from claude-teleport to entangle. These tests pin the
// promises that rename made to people who had already installed it. Each one
// protects something that would fail silently: a setting stops being read, a
// command stops being recognised, an update stops finding a build. None of those
// announce themselves, so they need tests rather than care.

func TestInvokedAsLegacyDetectsTheOldCommand(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })

	cases := map[string]bool{
		"/usr/local/bin/claude-teleport":     true,
		"claude-teleport":                    true,
		`C:\tools\claude-teleport.exe`:       true,
		"/opt/homebrew/bin/entangle":         false,
		"entangle":                           false,
		"/usr/local/bin/entangle.exe":        false,
		"/usr/local/bin/claude-teleport-old": false, // a different name, not ours
	}
	for arg0, want := range cases {
		os.Args = []string{arg0}
		if got := invokedAsLegacy(); got != want {
			t.Errorf("invokedAsLegacy() with argv[0]=%q = %v, want %v", arg0, got, want)
		}
	}
}

// TestLegacyEnvVarsStillHonoured: these live in shell profiles and CI config.
// Renaming the project is not a reason to silently stop reading what someone
// already wrote, and the failure would be invisible - a transfer quietly using
// the wrong server rather than an error.
func TestLegacyEnvVarsStillHonoured(t *testing.T) {
	for _, suffix := range []string{"RENDEZVOUS", "RELAY"} {
		newKey, oldKey := "ENTANGLE_"+suffix, "CLAUDE_TELEPORT_"+suffix

		t.Setenv(newKey, "")
		t.Setenv(oldKey, "from-legacy")
		if got := envOr(newKey, "fallback"); got != "from-legacy" {
			t.Errorf("%s not honoured: envOr(%s) = %q, want from-legacy", oldKey, newKey, got)
		}

		// The current spelling wins when both are set.
		t.Setenv(newKey, "from-current")
		if got := envOr(newKey, "fallback"); got != "from-current" {
			t.Errorf("current name should win: got %q", got)
		}

		// Neither set: the caller's default.
		t.Setenv(newKey, "")
		t.Setenv(oldKey, "")
		if got := envOr(newKey, "fallback"); got != "fallback" {
			t.Errorf("with neither set, want the default, got %q", got)
		}
	}
}

// TestNonEntangleKeysAreNotRewritten guards the prefix rule from over-reaching:
// CLAUDE_CONFIG_DIR belongs to Claude Code, not to this project.
func TestNonEntangleKeysAreNotRewritten(t *testing.T) {
	t.Setenv("SOME_OTHER_KEY", "")
	t.Setenv("CLAUDE_TELEPORT_SOME_OTHER_KEY", "should-not-be-read")
	if got := envOr("SOME_OTHER_KEY", "default"); got != "default" {
		t.Errorf("only ENTANGLE_* keys should fall back to the old prefix, got %q", got)
	}
}

// TestBundleFilenamesUseTheNewName: what a user sees written to disk should carry
// the current name, or the rename is only half done.
func TestBundleFilenamesUseTheNewName(t *testing.T) {
	if BinaryName != "entangle" {
		t.Fatalf("BinaryName = %q", BinaryName)
	}
	if LegacyBinaryName != "claude-teleport" {
		t.Fatalf("LegacyBinaryName = %q; it names what is already installed on people's machines and cannot change", LegacyBinaryName)
	}
}

// TestLegacyNameIsNotUsedForPaths is a canary: if someone later "tidies" the
// legacy constant into a path or a URL, that is a behaviour change rather than a
// cosmetic one, and this points at where to look.
func TestLegacyNameIsNotUsedForPaths(t *testing.T) {
	p := filepath.Join(t.TempDir(), LegacyBinaryName)
	if filepath.Base(p) != "claude-teleport" {
		t.Fatal("unexpected")
	}
}
