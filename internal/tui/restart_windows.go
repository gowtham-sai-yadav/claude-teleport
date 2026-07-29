//go:build windows

package tui

import (
	"errors"
	"os"
	"os/exec"
)

// restartInto starts the freshly installed binary and exits with whatever it
// exits with.
//
// Windows has no exec that replaces the running image, so unlike the Unix build
// the old process has to stay alive as a parent. It lends the child its stdio and
// waits, which looks the same from the terminal's side; the only real difference
// is an extra process in the tree for the rest of the session.
func restartInto(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// The child ran and chose its own exit code. Passing it through keeps the
		// restart invisible to anything scripting around this.
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
