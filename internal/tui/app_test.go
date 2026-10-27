package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func fixtureCommit(id string) api.Commit {
	return api.Commit{ID: id, Hash: id, Body: strings.Repeat("Unicode 界 👩🏽‍💻 and readable content. ", 4), URL: "https://example.test/commit/" + id, CreatedAt: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC), Author: &api.Author{Name: "Ada Fixture", Handle: "ada", Lane: []byte("4")}, Status: "approved"}
}

func fixtureApp(t *testing.T) AppModel {
	t.Helper()
	m := NewApp("dark", auth.Store{Dir: t.TempDir()}, "https://example.test", t.Context())
	m.now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { _ = m.Close() })
	m.fetch = func(context.Context, api.Log, string, string) (api.Page, error) {
		return api.Page{Log: api.Following, Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}}, nil
	}
	key, _ := auth.GenerateKey()
	updated, _ := m.Update(accountResult{key: key, user: auth.User{ID: "fixture", Handle: "fixture", Name: "Fixture"}})
	return updated.(AppModel)
}

func TestApplicationTransitionsFromVerificationAndLogin(t *testing.T) {
	m := NewApp("dark", auth.Store{Dir: t.TempDir()}, "https://example.test", t.Context())
	defer func() { _ = m.Close() }()
	updated, _ := m.Update(accountResult{})
	m = updated.(AppModel)
	if m.connected || m.account.state != "idle" {
		t.Fatal("missing session must show login")
	}
	updated, _ = m.Update(accountResult{err: errors.New("connection unavailable")})
	m = updated.(AppModel)
	if m.connected || !strings.Contains(m.View().Content, "connection unavailable") {
		t.Fatal("verification error hidden")
	}
	m = fixtureApp(t)
	if !m.connected || m.accountOpen || !strings.Contains(ansi.Strip(m.View().Content), "Following") {
		t.Fatal("verified identity must open application")
	}
}

func TestFeedsPreserveAnchorsDeduplicateAndIgnoreStaleResults(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}, Next: "next"})
	m, _ = m.handleKey("j")
	selected := m.feeds[0].selected
	m, _ = m.handleKey("tab")
	updated, _ := m.Update(logResult{index: 0, generation: m.feeds[0].generation - 1, page: api.Page{Data: []api.Commit{fixtureCommit("stale")}}})
	m = updated.(AppModel)
	m, _ = m.handleKey("shift+tab")
	if m.feeds[0].selected != selected || len(m.feeds[0].commits) != 2 {
		t.Fatal("switch lost selection or stale result overwrote feed")
	}
	m.feeds[0].loading = true
	updated, _ = m.Update(logResult{index: 0, generation: m.feeds[0].generation, mode: "more", page: api.Page{Data: []api.Commit{fixtureCommit("b"), fixtureCommit("c")}}})
	m = updated.(AppModel)
	if len(m.feeds[0].commits) != 3 || m.feeds[0].next != "" {
		t.Fatal("pagination duplicates or exhaustion")
	}
	m.feeds[0].loading = true
	updated, _ = m.Update(logResult{index: 0, generation: m.feeds[0].generation, mode: "refresh", page: api.Page{Data: []api.Commit{fixtureCommit("new"), fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("c")}}})
	m = updated.(AppModel)
	if m.feeds[0].commits[m.feeds[0].selected].ID != "new" || m.feeds[0].viewport.YOffset() != 0 {
		t.Fatal("explicit refresh must return to the newest page")
	}
}

func applyPage(m AppModel, index int, page api.Page) AppModel {
	updated, _ := m.Update(logResult{index: index, generation: m.feeds[index].generation, page: page})
	return updated.(AppModel)
}

func TestNavigationSequenceHelpAndPasteOwnership(t *testing.T) {
	m := fixtureApp(t)
	m, _ = m.handleKey("g")
	old := m.generation
	updated, _ := m.Update(gExpired{generation: old})
	m = updated.(AppModel)
	m, _ = m.handleKey("e")
	if m.active != 0 {
		t.Fatal("expired g navigated")
	}
	m, _ = m.handleKey("g")
	m, _ = m.handleKey("e")
	if m.active != 2 || m.pendingG {
		t.Fatal("g e did not open Trending")
	}
	m, _ = m.handleKey("g")
	updated, _ = m.Update(tea.BlurMsg{})
	m = updated.(AppModel)
	if m.pendingG {
		t.Fatal("blur kept navigation sequence")
	}
	updated, _ = m.Update(tea.PasteMsg{Content: "gl?j"})
	m = updated.(AppModel)
	if m.active != 2 || m.help {
		t.Fatal("paste invoked navigation")
	}
	m, _ = m.handleKey("?")
	if !strings.Contains(m.View().Content, "Keyboard shortcuts") {
		t.Fatal("help not shown")
	}
	m, _ = m.handleKey("esc")
	if m.help {
		t.Fatal("help not dismissed")
	}
}

func TestRemoteContentAndLayoutsStayInDisplayBounds(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}, {Width: 30, Height: 9}, {}} {
			m := fixtureApp(t)
			m.account.dark = dark
			c := fixtureCommit("a")
			c.Body = "before\x1b[2J\x1b]52;c;ZXZpbA==\aafter\r\b" + c.Body
			c.Author.Name = "Ada\x1b[31m\nINJECTED"
			m = applyPage(m, 0, api.Page{Data: []api.Commit{c, fixtureCommit("b")}})
			updated, _ := m.Update(size)
			view := updated.View()
			if size.Width == 0 || size.Height == 0 {
				if view.Content != "" {
					t.Fatal("zero-sized terminal should be empty")
				}
				continue
			}
			if lipgloss.Width(view.Content) > size.Width || lipgloss.Height(view.Content) > size.Height {
				t.Fatalf("overflow at %dx%d: %dx%d", size.Width, size.Height, lipgloss.Width(view.Content), lipgloss.Height(view.Content))
			}
			if strings.Contains(view.Content, "\x1b[2J") || strings.Contains(view.Content, "\x1b]52") {
				t.Fatal("remote terminal control survived")
			}
			plain := ansi.Strip(view.Content)
			if size.Width >= 60 && !strings.Contains(plain, "Following") {
				t.Fatalf("compact shell lost controls at %dx%d", size.Width, size.Height)
			}
			if size.Width >= 124 && !strings.Contains(plain, "ctrl+c") {
				t.Fatal("right sidebar lost exit shortcut")
			}
		}
	}
}

func TestRankedIndicatorsAndRefreshFailureKeepContent(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	m.feeds[0].loading = true
	updated, _ := m.Update(logResult{index: 0, generation: m.feeds[0].generation, mode: "refresh", err: errors.New("network unavailable")})
	m = updated.(AppModel)
	if len(m.feeds[0].commits) != 1 || m.feeds[0].status == "" {
		t.Fatal("refresh failure cleared content or recovery feedback")
	}
	m.active = 2
	m.feeds[2].loading = true
	updated, _ = m.Update(logResult{index: 2, generation: m.feeds[2].generation, mode: "count", page: api.Page{NewCount: 10}})
	m = updated.(AppModel)
	if m.feeds[2].newCount != 0 {
		t.Fatal("ranked feed received chronological count")
	}
}

func TestApplicationProgramRestoresTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	m := NewApp("dark", auth.Store{Dir: t.TempDir()}, "https://example.test", ctx)
	defer func() { _ = m.Close() }()
	var output bytes.Buffer
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&output), tea.WithWindowSize(80, 24))
	finished := make(chan error, 1)
	go func() { _, err := p.Run(); finished <- err }()
	p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[?1049h") || !strings.Contains(output.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen was not restored")
	}
}
