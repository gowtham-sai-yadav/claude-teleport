//go:build !windows

package tui

import (
	"os"
	"syscall"
)

// restartInto hands this terminal over to the freshly installed binary.
//
// A real exec, not a spawn: the new build takes over the same pid, the same tty
// and the same exit status, so there is no old process left sitting behind it
// forwarding signals, and a Ctrl-C goes where the person is looking. On success
// this function does not return - the process it was called from no longer
// exists. It returns only when the handover itself failed.
func restartInto(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
