package handoff

import (
	"os"
	"strings"
	"testing"
)

const code = "6-pioneer-village"

func TestCommandIsWhatAPersonTypes(t *testing.T) {
	if got, want := Command(code), "entangle receive 6-pioneer-village"; got != want {
		t.Errorf("Command() = %q, want %q", got, want)
	}
}

func TestInviteCarriesTheCode(t *testing.T) {
	// An invite without the code in it is worse than no invite: the receiver
	// follows every instruction and still has nothing to connect to.
	if !strings.Contains(Invite(code), code) {
		t.Errorf("the code is missing from the invite:\n%s", Invite(code))
	}
}

func TestInviteCarriesTheInstallLine(t *testing.T) {
	// This is the entire reason the package exists. Without it the invite is just
	// the receive command again, and a receiver who does not already have entangle
	// gets "command not found" and gives up.
	if !strings.Contains(Invite(code), InstallSh) {
		t.Errorf("no install line in the invite:\n%s", Invite(code))
	}
}

func TestInviteSaysWhereToRunIt(t *testing.T) {
	// A session is attached to the directory it is received in, so running this
	// from the wrong folder imports a conversation whose file paths point at
	// nothing. The receiver cannot guess that, so the invite has to say it.
	if !strings.Contains(Invite(code), "inside your copy of the project") {
		t.Errorf("the invite does not say where to run it:\n%s", Invite(code))
	}
}

func TestInviteCoversWindows(t *testing.T) {
	// The sender has no idea which OS the receiver is on, so a curl-only invite
	// silently excludes every Windows teammate.
	if !strings.Contains(Invite(code), InstallPS1) {
		t.Errorf("no Windows install line in the invite:\n%s", Invite(code))
	}
}

// TestInstallURLsMatchTheDocumentedOnes guards a specific way this goes wrong:
// the install URLs live in the README, on a website, and now also inside the
// binary. Move the domain or rename a script and the README gets updated because
// people read it, while the string burned into every send stays behind and starts
// handing out a 404 to exactly the people who are installing for the first time.
func TestInstallURLsMatchTheDocumentedOnes(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{InstallSh, InstallPS1} {
		if !strings.Contains(string(readme), url) {
			t.Errorf("the invite hands out %s but the README does not document it", url)
		}
	}
}
