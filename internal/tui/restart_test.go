package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// finishedUpdate puts the model where it lands the moment a self-update has
// installed: on the result card, still running the old build, with the new one
// sitting on disk.
func finishedUpdate(t *testing.T) model {
	t.Helper()
	m := fakeModel(t, 100, 40)
	m.updLatest = "0.6.2"
	mm, _ := m.Update(doneMsg{title: "Updated.", body: "Now on 0.6.2.", restart: "/usr/local/bin/entangle"})
	return mm.(model)
}

// quits reports whether a command resolves to bubbletea's quit message. Commands
// are opaque functions, so the only honest way to tell is to run it.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestUpdateResultOffersTheRestart is the whole complaint this fixes: installing
// from inside the cockpit used to leave you looking at the old version with no way
// forward except quitting and coming back, which is easy to just not do.
func TestUpdateResultOffersTheRestart(t *testing.T) {
	m := finishedUpdate(t)

	if m.restartTo == "" {
		t.Fatal("a finished update did not record which binary to hand over to")
	}
	view := plain(m.View())
	if !strings.Contains(view, "restart") {
		t.Errorf("the result card does not offer a restart:\n%s", view)
	}
	// Matched as one phrase, not as a bare "0.6.2" anywhere on screen: the body
	// above already says "Now on 0.6.2", so a loose check would pass even with the
	// version stripped off the offer itself.
	if !strings.Contains(view, "restart into 0.6.2") {
		t.Errorf("the restart offer does not name the version being installed:\n%s", view)
	}
}

// TestEnterHandsOverToTheNewBinary: pressing enter has to both quit and leave
// restartTo set, because those are two halves of one action - Run reads the field
// off the final model after the program exits. Quitting with it cleared would look
// identical on screen and silently drop the user back to a shell.
func TestEnterHandsOverToTheNewBinary(t *testing.T) {
	m := finishedUpdate(t)

	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := mm.(model)

	if !quits(cmd) {
		t.Error("enter did not quit, so the handover never happens")
	}
	if got.restartTo != "/usr/local/bin/entangle" {
		t.Errorf("restartTo = %q after enter; Run needs it to still be set", got.restartTo)
	}
}

// TestAnyOtherKeyStaysOnTheOldBuild covers declining. It has to clear restartTo,
// not merely skip the exec: a stale value would fire on the next ordinary quit,
// restarting the tool minutes later for somebody who already said no.
func TestAnyOtherKeyStaysOnTheOldBuild(t *testing.T) {
	m := press(t, finishedUpdate(t), "x")

	if m.restartTo != "" {
		t.Errorf("restartTo = %q after declining; it would exec on the next quit", m.restartTo)
	}
	if m.mode != modeList {
		t.Error("declining the restart should go back to the session list")
	}
}

// TestOrdinaryResultsNeverRestart: every other operation in the tool ends on this
// same card. If any of them set up a handover, finishing an export would relaunch
// the process.
func TestOrdinaryResultsNeverRestart(t *testing.T) {
	m := fakeModel(t, 100, 40)
	mm, _ := m.Update(doneMsg{title: "Shared to a file.", body: "Wrote it."})
	got := mm.(model)

	if got.restartTo != "" {
		t.Errorf("a non-update result set restartTo = %q", got.restartTo)
	}
	if view := plain(got.View()); strings.Contains(view, "restart") {
		t.Errorf("a non-update result offers a restart:\n%s", view)
	}
	if _, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEnter}); quits(cmd) {
		t.Error("enter on an ordinary result quit the program")
	}
}
