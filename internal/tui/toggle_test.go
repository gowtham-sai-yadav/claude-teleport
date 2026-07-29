package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
	"github.com/gowtham-sai-yadav/entangle/internal/transfer"
)

// multiToolModel is a model loaded with sessions from three tools, which is the
// only configuration where the tool filter means anything.
func multiToolModel(t *testing.T) model {
	t.Helper()
	m := newModel("", transfer.Config{}, "0.6.0", "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(model)
	now := time.Now()
	sessions := []agent.Session{
		{Provider: agent.ClaudeCode, ID: "aaaa1111-cc", ShortID: "aaaa1111", ProjectPath: "/w/api", Title: "claude one", Messages: 4, ModTime: now},
		{Provider: agent.Codex, ID: "019fbbbb-cx", ShortID: "019fbbbb", ProjectPath: "/w/api", Title: "codex one", Messages: 5, ModTime: now.Add(-time.Hour)},
		{Provider: agent.OpenCode, ID: "ses_cccc11", ShortID: "ses_cccc", ProjectPath: "/w/api", Title: "opencode one", Messages: 6, ModTime: now.Add(-2 * time.Hour)},
	}
	mm, _ = m.Update(sessionsMsg{sessions: sessions, present: []agent.ID{agent.ClaudeCode, agent.Codex, agent.OpenCode}})
	return mm.(model)
}

func press(t *testing.T, m model, key string) model {
	t.Helper()
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return mm.(model)
}

// TestListShowsEveryToolByDefault: the list a user lands on is all their work,
// not one vendor's. A Codex-only user seeing an empty screen would conclude the
// tool does not support them.
func TestListShowsEveryToolByDefault(t *testing.T) {
	m := multiToolModel(t)
	if got := len(m.list.Items()); got != 3 {
		t.Fatalf("default list has %d rows, want all 3", got)
	}
	body := plain(m.View())
	for _, want := range []string{"claude one", "codex one", "opencode one"} {
		if !strings.Contains(body, want) {
			t.Errorf("default view is missing %q", want)
		}
	}
}

// TestToolCycleVisitsEveryToolAndReturns: `t` steps through each installed tool
// and back to all, so the key is a loop with no dead end to escape from.
func TestToolCycleVisitsEveryToolAndReturns(t *testing.T) {
	m := multiToolModel(t)
	want := []struct {
		filter agent.ID
		rows   int
	}{
		{agent.ClaudeCode, 1},
		{agent.Codex, 1},
		{agent.OpenCode, 1},
		{"", 3},
	}
	for i, step := range want {
		m = press(t, m, "t")
		if m.filter != step.filter {
			t.Fatalf("press %d: filter = %q, want %q", i+1, m.filter, step.filter)
		}
		if got := len(m.list.Items()); got != step.rows {
			t.Fatalf("press %d (%q): %d rows, want %d", i+1, step.filter, got, step.rows)
		}
	}
}

// TestFilteredListNamesOnlyItsOwnTool: narrowing to one tool must actually hide
// the others, or the key silently does nothing.
func TestFilteredListNamesOnlyItsOwnTool(t *testing.T) {
	m := press(t, multiToolModel(t), "t") // -> claude-code
	body := plain(m.View())
	if !strings.Contains(body, "claude one") {
		t.Error("claude session missing from the claude-only view")
	}
	if strings.Contains(body, "codex one") || strings.Contains(body, "opencode one") {
		t.Error("other tools' sessions are still shown after narrowing")
	}
	if !strings.Contains(body, "Claude Code") {
		t.Error("the header should name the tool currently being shown")
	}
}

// TestSingleToolHidesTheToolSwitch: with one tool installed there is nothing to
// switch to, so offering the key would promise something that does nothing.
func TestSingleToolHidesTheToolSwitch(t *testing.T) {
	m := fakeModel(t, 100, 40) // claude-code only
	if strings.Contains(plain(m.footerView()), "tool:") {
		t.Error("the tool hint is offered on a single-tool machine")
	}
	before := len(m.list.Items())
	m = press(t, m, "t")
	if m.filter != "" || len(m.list.Items()) != before {
		t.Error("`t` changed the list on a single-tool machine")
	}
}

// TestHelpMatchesWhatIsInstalled: the legend is where a user goes when something
// is unclear, so it must not name a key that does nothing here, nor hide the
// caveat that backup only covers Claude Code once other tools are in play.
func TestHelpMatchesWhatIsInstalled(t *testing.T) {
	single := plain(fakeModel(t, 100, 40).helpView())
	if strings.Contains(single, "show one tool at a time") {
		t.Error("the single-tool legend explains a key that does nothing")
	}
	if strings.Contains(single, "backup covers Claude Code only") {
		t.Error("the single-tool legend raises a distinction that does not exist there")
	}
	if n := doubleGap(single); n > 0 {
		t.Errorf("the single-tool legend has a gap at line %d where a row was dropped:\n%s", n, single)
	}

	multi := plain(multiToolModel(t).helpView())
	for _, want := range []string{"show one tool at a time", "backup covers Claude Code only"} {
		if !strings.Contains(multi, want) {
			t.Errorf("the multi-tool legend is missing %q", want)
		}
	}
	if n := doubleGap(multi); n > 0 {
		t.Errorf("the multi-tool legend has a gap at line %d:\n%s", n, multi)
	}
}

// doubleGap returns the 1-based line number of the first pair of consecutive
// blank lines, or 0 if there is none.
//
// The stripping matters more than it looks. cardStyle pads every row out to the
// card width and wraps the whole thing in a box, so a row omitted as "" reaches
// here as "│      │" - never as an empty string. Checking the raw text for "\n\n"
// finds nothing wrong no matter how many rows were dropped.
func doubleGap(card string) int {
	lines := strings.Split(strings.TrimSpace(card), "\n")
	blank := func(i int) bool {
		return strings.TrimSpace(strings.Trim(lines[i], "│ ")) == ""
	}
	for i := 1; i < len(lines); i++ {
		if blank(i) && blank(i-1) {
			return i + 1
		}
	}
	return 0
}

// TestSearchStillTypesTheLetterT: `/` search must own every key, or typing a
// query containing "t" reshuffles the list underneath the cursor.
func TestSearchStillTypesTheLetterT(t *testing.T) {
	m := multiToolModel(t)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = mm.(model)
	m = press(t, m, "t")
	if m.filter != "" {
		t.Errorf("typing t into the search box changed the tool filter to %q", m.filter)
	}
}
