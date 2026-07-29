package cli

import (
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/entangle/internal/handoff"
)

// TestSendOutputWorksForATeammateWhoHasNothing covers the one screen that gets
// read by somebody who has never installed entangle. Everything else in this tool
// is read by a user; this text gets forwarded to a stranger.
func TestSendOutputWorksForATeammateWhoHasNothing(t *testing.T) {
	const code = "6-pioneer-village"
	out := captureStdout(t, func() error {
		printSendCode(code)
		return nil
	})

	for _, want := range []string{
		code,                  // the code, to read aloud
		handoff.Command(code), // the command, for someone who already has it
		handoff.InstallSh,     // the install line, for someone who does not
		handoff.InstallPS1,    // ...on Windows
		"copy of the project", // where to run it, which cannot be guessed
		"Waiting for them",    // that the sender should now sit still
	} {
		if !strings.Contains(out, want) {
			t.Errorf("send output is missing %q:\n%s", want, out)
		}
	}
}
