// Package handoff builds the text a sender passes to whoever is receiving a
// session.
//
// It exists because of who is on the other end. Sending is the one moment this
// tool is put in front of somebody who has never heard of it, and they meet it
// with a reason to care already in hand: a teammate is trying to give them
// something. Printing only `entangle receive <code>` wastes that. They paste it,
// the shell says command not found, and now the sender has to explain what
// entangle is over chat while the transfer sits there waiting to time out.
//
// So the invite carries the install line too, and says to run it from inside the
// project. Both facts are things the receiver cannot guess and will otherwise
// have to be told by hand, once per share, forever.
package handoff

import "fmt"

const (
	// InstallSh and InstallPS1 are the published one-line installers. The scripts
	// themselves are install.sh and install.ps1 in this repo; these URLs are where
	// they are served from, and are what a person can actually be told to run.
	InstallSh  = "https://gowthamsai.in/install.sh"
	InstallPS1 = "https://gowthamsai.in/install.ps1"
)

// Command is the bare command the receiver runs once they already have entangle.
func Command(code string) string {
	return "entangle receive " + code
}

// Invite is the whole message, ready to paste into chat, and written for a
// receiver who may have never installed entangle and may be on Windows. It is
// what the TUI's copy key puts on the clipboard, so it is deliberately plain
// text with no styling and no leading indent of its own.
func Invite(code string) string {
	return fmt.Sprintf(`Run these from inside your copy of the project
(skip the first line if you already have entangle):

  curl -fsSL %s | sh
  %s

Windows PowerShell: irm %s | iex`, InstallSh, Command(code), InstallPS1)
}
