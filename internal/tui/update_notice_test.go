package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gowtham-sai-yadav/entangle/internal/transfer"
)

// header renders just the top of the cockpit for a model told about newVersion.
func header(t *testing.T, newVersion string) string {
	t.Helper()
	m := newModel("", transfer.Config{}, "0.6.0", newVersion)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return plain(mm.(model).headerView())
}

// TestHeaderAnnouncesANewerVersion: someone who only ever types `entangle` never
// sees the CLI's stderr note, so the header is the one place they can learn a
// release exists. It has to name the version and the key that installs it.
func TestHeaderAnnouncesANewerVersion(t *testing.T) {
	got := header(t, "0.7.0")
	for _, want := range []string{"v0.7.0 available", "press u"} {
		if !strings.Contains(got, want) {
			t.Errorf("header is missing %q:\n%s", want, got)
		}
	}
}

// TestHeaderSaysNothingWhenCurrent: the badge must be absent, not empty. A user
// on the latest version should see no hint that anything is out of date.
func TestHeaderSaysNothingWhenCurrent(t *testing.T) {
	got := header(t, "")
	for _, unwanted := range []string{"available", "press u"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("header mentions %q with no update pending:\n%s", unwanted, got)
		}
	}
}
