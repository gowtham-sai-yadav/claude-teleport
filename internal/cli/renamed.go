package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// This project was called claude-teleport until it grew past Claude Code and
// picked up Codex and opencode. The name had become inaccurate: it told a Codex
// user the tool was not for them.
//
// A rename is only cheap for the person doing it. Anyone who already installed
// the old binary has it on their PATH, in their shell history, and in whatever
// notes they keep. So the old command keeps working, and says once - quietly,
// not as an error - what it is now called.

// LegacyBinaryName is what this program was called before the rename.
const LegacyBinaryName = "claude-teleport"

// BinaryName is what it is called now.
const BinaryName = "entangle"

// invokedAsLegacy reports whether this process was started through the old
// command name, whether that is the real binary, a copy, or a symlink.
//
// Both separators are handled rather than deferring to filepath.Base, which only
// recognises the host's own: argv[0] carries whatever the invoking shell used, so
// a Windows-shaped path is not necessarily seen on Windows.
func invokedAsLegacy() bool {
	if len(os.Args) == 0 {
		return false
	}
	arg0 := os.Args[0]
	if i := strings.LastIndexAny(arg0, `/\`); i >= 0 {
		arg0 = arg0[i+1:]
	}
	name := strings.TrimSuffix(strings.TrimSuffix(arg0, ".exe"), ".EXE")
	return name == LegacyBinaryName
}

// noticeIfLegacyName prints a one-line rename notice when the old command is
// used, and nothing at all otherwise.
//
// It goes to stderr so that piping `entangle sessions --json` into another
// program keeps working byte for byte: a notice that corrupts machine-readable
// output would be worse than no notice. It is also suppressed entirely when
// stdout is not a terminal, because a script does not need to be told.
func noticeIfLegacyName() {
	if !invokedAsLegacy() || os.Getenv("ENTANGLE_NO_RENAME_NOTICE") != "" {
		return
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	fmt.Fprintf(os.Stderr,
		"note: %s is now called %s. This command still works; to switch over, run:\n"+
			"      brew install gowtham-sai-yadav/tap/%s     (or) curl -fsSL https://gowthamsai.in/install.sh | sh\n\n",
		LegacyBinaryName, BinaryName, BinaryName)
}
