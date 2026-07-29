package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
	"github.com/gowtham-sai-yadav/entangle/internal/transfer"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func fakeModel(t *testing.T, w, h int) model {
	t.Helper()
	m := newModel("", transfer.Config{}, "0.5.0", "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = mm.(model)
	now := time.Now()
	sessions := []agent.Session{
		{Provider: agent.ClaudeCode, ID: "8e3d21d1-aa11-4f00-9c0f-abc", ShortID: "8e3d21d1", ProjectPath: "/Users/gowtham/work/api-service", Title: "refactor the auth layer", Messages: 42, ModTime: now.Add(-2 * time.Hour)},
		{Provider: agent.ClaudeCode, ID: "1f9c77a2-bb22-4e11-8d1a-def", ShortID: "1f9c77a2", ProjectPath: "/Users/gowtham/side/entangle", Title: "build the interactive TUI", Messages: 128, ModTime: now.Add(-25 * time.Hour)},
		{Provider: agent.ClaudeCode, ID: "44ab90ee-cc33-4d22-7e2b-ghi", ShortID: "44ab90ee", ProjectPath: "/Users/gowtham/work/dash", Title: "fix the flaky pagination test", Messages: 9, ModTime: now.Add(-9 * time.Minute)},
	}
	mm, _ = m.Update(sessionsMsg{sessions: sessions, present: []agent.ID{agent.ClaudeCode}})
	return mm.(model)
}

// TestRenderScreens prints each screen so the layout can be eyeballed:
//
//	go test ./internal/tui -run TestRenderScreens -v
func TestRenderScreens(t *testing.T) {
	m := fakeModel(t, 100, 40)

	fmt.Println("\n################## HOME (session list) ##################")
	fmt.Println(plain(m.View()))

	m.prepped = &prepared{
		provider: agent.ClaudeCode,
		name:     "entangle-session-8e3d21d1.tgz",
		preview: agent.Preview{
			Title: "refactor the auth layer", ShortID: "8e3d21d1",
			ProjectPath: "/Users/gowtham/work/api-service",
			Messages:    42, Bytes: 268_400, SecretsMasked: 3,
		},
	}
	m.mode = modeConfirm
	fmt.Println("\n################## CONFIRM (send) ##################")
	fmt.Println(plain(m.View()))

	m.mode, m.code, m.done, m.total = modeSend, "7-crossover-marbles", 184_320, 268_400
	fmt.Println("\n################## SEND (code + progress) ##################")
	fmt.Println(plain(m.View()))

	m = m.startInput(modeRecvCode)
	fmt.Println("\n################## RECEIVE (enter code) ##################")
	fmt.Println(plain(m.View()))

	m = m.toResult("Received.", "12 file(s) written, 0 skipped, 1 project(s) merged.\n1/1 project(s) resume-ready.\nOpen Claude Code in this folder to continue.", false)
	fmt.Println("\n################## RESULT ##################")
	fmt.Println(plain(m.View()))

	m.mode = modeHelp
	fmt.Println("\n################## HELP (the ? overlay) ##################")
	fmt.Println(plain(m.View()))

	// Narrow terminal: banner must fall back and nothing should panic.
	nm := fakeModel(t, 44, 30)
	if nm.bannerOK {
		t.Errorf("expected banner fallback at width 44, got bannerOK=true (banner width %d)", len(m.banner))
	}
	fmt.Println("\n################## HOME @ width 44 (banner fallback) ##################")
	fmt.Println(plain(nm.View()))
}

// TestCardsRectangular proves every card is a true rectangle: under lipgloss's
// own display-width model (the same one the terminal uses for box-drawing and
// wide glyphs), all lines must be equal width, or the border would not line up.
func TestCardsRectangular(t *testing.T) {
	m := fakeModel(t, 100, 40)
	m.prepped = &prepared{
		provider: agent.ClaudeCode,
		name:     "x.tgz",
		preview:  agent.Preview{Title: "refactor the auth layer", ShortID: "8e3d21d1", ProjectPath: "/Users/gowtham/work/api-service", Messages: 42, Bytes: 268_400, SecretsMasked: 3},
	}
	m.code, m.done, m.total, m.updLatest, m.status = "7-crossover-marbles", 180_000, 268_400, "0.6.0", "Receiving…"
	m = m.startInput(modeRecvCode)

	cards := map[string]string{
		"confirm": m.confirmView(),
		"send":    m.sendView(),
		"receive": m.transferView("Receiving a session"),
		"input":   m.inputView("Import a bundle", "Path:", "hint"),
		"busy":    m.busyView(),
		"update":  m.updateView(),
		"help":    m.helpView(),
		"result":  m.toResult("Received.", "line one\nline two is a bit longer", false).resultView(),
	}
	for name, card := range cards {
		lines := strings.Split(card, "\n")
		want := lipgloss.Width(lines[0])
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got != want {
				t.Errorf("%s card line %d width=%d, want %d (border not aligned)\n%q", name, i, got, want, plain(ln))
			}
		}
	}

	// A multi-line fallback notice is the widest thing that can appear inside a
	// transfer card, so check the border still squares up with one present.
	m.notice = "This network blocked the usual transfer server, so I switched to a backup.\n" +
		"The other side must run these first, or you will not find each other:\n" +
		"  export ENTANGLE_RENDEZVOUS=wss://mailbox.mw.leastauthority.com/v1\n" +
		"  export ENTANGLE_RELAY=relay.mw.leastauthority.com:4001"
	for name, card := range map[string]string{
		"send+notice":    m.sendView(),
		"receive+notice": m.transferView("Receiving a session"),
	} {
		lines := strings.Split(card, "\n")
		want := lipgloss.Width(lines[0])
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got != want {
				t.Errorf("%s line %d width=%d, want %d\n%q", name, i, got, want, plain(ln))
			}
		}
		if !strings.Contains(plain(card), "ENTANGLE_RENDEZVOUS") {
			t.Errorf("%s: the peer instruction must be visible, it is the actionable part", name)
		}
	}
}

// TestToolBadgeOnlyWhenMultipleTools: someone using one coding tool must see the
// row exactly as before, so the tool name appears only when there is a choice.
func TestToolBadgeOnlyWhenMultipleTools(t *testing.T) {
	s := agent.Session{Provider: agent.Codex, ID: "019f9e34-be2e", ShortID: "019f9e34", Title: "refactor", Messages: 8, ProjectPath: "/w/api"}

	solo := sessionItem{s: s, showTool: false}
	if strings.Contains(solo.Description(), "codex") {
		t.Errorf("single-tool row should not carry a tool name: %q", solo.Description())
	}
	multi := sessionItem{s: s, showTool: true}
	if !strings.Contains(multi.Description(), "codex") {
		t.Errorf("multi-tool row should name the tool: %q", multi.Description())
	}
	// The handle and counts must survive either way.
	for _, d := range []string{solo.Description(), multi.Description()} {
		if !strings.Contains(d, "019f9e34") || !strings.Contains(d, "8 msgs") {
			t.Errorf("row lost its details: %q", d)
		}
	}
}

// TestFilterMatchesToolName lets "/codex" narrow the list to one tool.
func TestFilterMatchesToolName(t *testing.T) {
	it := sessionItem{s: agent.Session{Provider: agent.OpenCode, ID: "ses_1", Title: "hello", ProjectPath: "/w"}}
	if !strings.Contains(it.FilterValue(), "opencode") {
		t.Errorf("FilterValue should include the tool so search can narrow by it: %q", it.FilterValue())
	}
}

// TestConfirmCardNamesTheTool: before a session leaves the machine, which tool it
// came from is part of what the user is agreeing to.
func TestConfirmCardNamesTheTool(t *testing.T) {
	m := fakeModel(t, 100, 40)
	m.prepped = &prepared{
		provider: agent.Codex,
		name:     "x.tgz",
		preview:  agent.Preview{Title: "refactor", ShortID: "019f9e34", ProjectPath: "/w/api", Messages: 8, Bytes: 1024},
	}
	m.mode = modeConfirm
	card := plain(m.confirmView())
	if !strings.Contains(card, "OpenAI Codex CLI") {
		t.Errorf("confirm card should name the tool:\n%s", card)
	}
	// And it must still be a rectangle with the extra row.
	lines := strings.Split(m.confirmView(), "\n")
	want := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != want {
			t.Fatalf("confirm card line %d width=%d, want %d", i, got, want)
		}
	}
}

// TestLoadProblemsSurface: if one tool cannot be read, say so rather than quietly
// showing a short list the user has no way to question.
func TestLoadProblemsSurface(t *testing.T) {
	m := fakeModel(t, 100, 40)
	mm, _ := m.Update(sessionsMsg{present: []agent.ID{agent.ClaudeCode}, problems: []string{"opencode: no such table: session"}})
	got := mm.(model)
	if !strings.Contains(got.notice, "opencode") {
		t.Errorf("a read failure should be surfaced, notice = %q", got.notice)
	}
}

// TestSendViewOffersCopyOnlyWithACode: the copy hint is meaningless before a code
// exists, and the send screen is on display during that whole wait.
func TestSendViewOffersCopyOnlyWithACode(t *testing.T) {
	m := fakeModel(t, 100, 40)
	m.mode = modeSend

	m.status = "Opening a secure channel…"
	if plain(m.sendView()) == "" {
		t.Fatal("empty send view")
	}
	if strings.Contains(plain(m.sendView()), "copy invite") {
		t.Error("no code yet, so nothing to copy - the hint should be absent")
	}

	m.code = "7-crossover-marbles"
	withCode := plain(m.sendView())
	if !strings.Contains(withCode, "copy invite") {
		t.Errorf("with a code present the copy hint should appear:\n%s", withCode)
	}
	if !strings.Contains(withCode, "press c to copy") {
		t.Errorf("the inline prompt should say which key:\n%s", withCode)
	}
}

// TestCopyResultReplacesThePrompt: after copying, the screen should confirm it
// rather than keep suggesting the thing you just did.
func TestCopyResultReplacesThePrompt(t *testing.T) {
	m := fakeModel(t, 100, 40)
	m.mode, m.code = modeSend, "7-crossover-marbles"
	m.copied = "copied to clipboard"

	got := plain(m.sendView())
	if !strings.Contains(got, "copied to clipboard") {
		t.Errorf("the confirmation should be shown:\n%s", got)
	}
	if strings.Contains(got, "press c to copy") {
		t.Error("the prompt should give way to the confirmation")
	}
	// A machine with no clipboard is a normal case, not an error state.
	m.copied = "no clipboard here - select the code above to copy it"
	if !strings.Contains(plain(m.sendView()), "select the code above") {
		t.Error("the no-clipboard case should tell the user what to do instead")
	}
	// Border must still line up with either message present.
	for _, msg := range []string{"copied to clipboard", "no clipboard here - select the code above to copy it"} {
		m.copied = msg
		lines := strings.Split(m.sendView(), "\n")
		want := lipgloss.Width(lines[0])
		for i, ln := range lines {
			if got := lipgloss.Width(ln); got != want {
				t.Fatalf("send card line %d width=%d, want %d (msg=%q)", i, got, want, msg)
			}
		}
	}
}

// TestNoFallbackBannerInSendView: switching transfer servers needs nothing from the
// user, and an amber warning at the exact moment they are waiting for a code reads
// as a failure. It was removed on purpose; this keeps it removed.
func TestNoFallbackBannerInSendView(t *testing.T) {
	m := fakeModel(t, 100, 40)
	m.mode, m.code = modeSend, "7-crossover-marbles"
	got := plain(m.sendView())
	for _, phrase := range []string{"not responding", "switched to a backup", "does not need to change"} {
		if strings.Contains(got, phrase) {
			t.Errorf("the fallback banner is back (%q):\n%s", phrase, got)
		}
	}
}
