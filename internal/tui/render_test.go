package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/exporter"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/transfer"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func fakeModel(t *testing.T, w, h int) model {
	t.Helper()
	m := newModel(claudedir.Paths{Home: "/Users/gowtham"}, "", transfer.Config{}, "0.5.0")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = mm.(model)
	now := time.Now()
	items := []list.Item{
		sessionItem{claudedir.Session{ID: "8e3d21d1-aa11-4f00-9c0f-abc", ProjectPath: "/Users/gowtham/work/api-service", Title: "refactor the auth layer", Messages: 42, ModTime: now.Add(-2 * time.Hour)}},
		sessionItem{claudedir.Session{ID: "1f9c77a2-bb22-4e11-8d1a-def", ProjectPath: "/Users/gowtham/side/claude-teleport", Title: "build the interactive TUI", Messages: 128, ModTime: now.Add(-25 * time.Hour)}},
		sessionItem{claudedir.Session{ID: "44ab90ee-cc33-4d22-7e2b-ghi", ProjectPath: "/Users/gowtham/work/dash", Title: "fix the flaky pagination test", Messages: 9, ModTime: now.Add(-9 * time.Minute)}},
	}
	mm, _ = m.Update(sessionsMsg{items: items})
	return mm.(model)
}

// TestRenderScreens prints each screen so the layout can be eyeballed:
//
//	go test ./internal/tui -run TestRenderScreens -v
func TestRenderScreens(t *testing.T) {
	m := fakeModel(t, 100, 40)

	fmt.Println("\n################## HOME (session list) ##################")
	fmt.Println(plain(m.View()))

	m.prepped = &exporter.SessionBundle{
		Name: "claude-teleport-session-8e3d21d1.tgz",
		Preview: exporter.SharePreview{
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
	m.prepped = &exporter.SessionBundle{
		Name:    "x.tgz",
		Preview: exporter.SharePreview{Title: "refactor the auth layer", ShortID: "8e3d21d1", ProjectPath: "/Users/gowtham/work/api-service", Messages: 42, Bytes: 268_400, SecretsMasked: 3},
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
}
