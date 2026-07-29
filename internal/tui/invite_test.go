package tui

import (
	"strings"
	"testing"

	"github.com/gowtham-sai-yadav/entangle/internal/handoff"
)

// captureClipboard swaps in a recorder for the copy key and returns a pointer to
// whatever gets written, so these tests never touch the real clipboard.
func captureClipboard(t *testing.T) *string {
	t.Helper()
	var got string
	prev := writeClipboard
	writeClipboard = func(s string) error {
		got = s
		return nil
	}
	t.Cleanup(func() { writeClipboard = prev })
	return &got
}

// TestCopyPutsTheWholeInviteOnTheClipboard is the point of the whole thing. A
// sender pastes what this key gives them, and if that is only the code then a
// teammate without entangle gets "command not found" and the sender has to talk
// them through an install by hand - which is where most shares quietly die.
func TestCopyPutsTheWholeInviteOnTheClipboard(t *testing.T) {
	got := captureClipboard(t)

	m := fakeModel(t, 100, 40)
	m.mode, m.code = modeSend, "7-crossover-marbles"
	m = press(t, m, "c")

	if *got == "" {
		t.Fatal("the copy key wrote nothing to the clipboard")
	}
	if !strings.Contains(*got, handoff.InstallSh) {
		t.Errorf("no install line on the clipboard, so a teammate without entangle is stuck:\n%s", *got)
	}
	if !strings.Contains(*got, "entangle receive 7-crossover-marbles") {
		t.Errorf("the receive command is missing from the clipboard:\n%s", *got)
	}
	if !strings.Contains(m.copied, "paste") {
		t.Errorf("copied = %q, should tell them what to do with it", m.copied)
	}
}

// TestCopyDoesNothingWithoutACode: the send screen is up during the whole wait
// for a code, and copying an empty string then would silently wipe whatever the
// person had on their clipboard.
func TestCopyDoesNothingWithoutACode(t *testing.T) {
	got := captureClipboard(t)

	m := fakeModel(t, 100, 40)
	m.mode, m.code = modeSend, ""
	m = press(t, m, "c")

	if *got != "" {
		t.Errorf("wrote %q to the clipboard before a code existed", *got)
	}
}
