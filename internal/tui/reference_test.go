package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestReferenceGeometryAndCompactSidebar(t *testing.T) {
	for _, size := range []struct{ width, nav, center, aside int }{{80, 5, 73, 0}, {100, 25, 73, 0}, {124, 25, 61, 36}, {134, 25, 67, 40}, {180, 25, 92, 40}} {
		m := fixtureApp(t)
		m.account.width = size.width
		nav, center, aside := m.columns()
		if nav != size.nav || center != size.center || aside != size.aside {
			t.Fatalf("%d columns: %d/%d/%d", size.width, nav, center, aside)
		}
		if m.mainHeight() != m.account.height-1 {
			t.Fatal("reference has one bottom status row")
		}
		if !strings.Contains(ansi.Strip(m.navigation(nav, 23, colors(true))), "◉") {
			t.Fatal("compact navigation lost the log icon")
		}
	}
}

func TestReferenceSelectionAndTabNavigation(t *testing.T) {
	m := fixtureApp(t)
	var commits []api.Commit
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		commits = append(commits, fixtureCommit(id))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	for _, pressed := range []string{"G", "end"} {
		m, _ = m.handleKey(pressed)
		if m.feeds[0].selected != 7 {
			t.Fatal("bottom jump must select the final loaded commit")
		}
	}
	m, _ = m.handleKey("g")
	m, _ = m.handleKey("g")
	if m.feeds[0].selected != 0 || m.feeds[0].viewport.YOffset() != 0 {
		t.Fatal("g g must select the first commit and show the day marker")
	}
	m, _ = m.handleKey("ctrl+d")
	if m.feeds[0].selected != 5 {
		t.Fatal("page navigation must move five commits")
	}
	m, _ = m.handleKey("home")
	for _, pressed := range []string{"]", "l", "right"} {
		m, _ = m.handleKey(pressed)
	}
	if m.active != 0 {
		t.Fatal("next-tab aliases must cycle the three logs")
	}
	m, _ = m.handleKey("[")
	if m.active != 2 {
		t.Fatal("previous-tab alias did not wrap")
	}
}

func TestMouseSelectsRowsAndHelpOwnsScrolling(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("c")}})
	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(AppModel)
	if m.feeds[0].selected != 1 {
		t.Fatal("wheel must select the next commit")
	}
	m, _ = m.handleKey("home")
	nav, center, aside := m.columns()
	x := (m.account.width-nav-center-aside-2)/2 + nav + 2
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: feedHeaderHeight + m.feeds[0].starts[1]})
	m = updated.(AppModel)
	if m.feeds[0].selected != 1 {
		t.Fatal("click must select the commit under the pointer")
	}
	m, _ = m.handleKey("?")
	before := m.feeds[0].viewport.YOffset()
	m, _ = m.handleKey("pgdown")
	if m.helpScroll == 0 || m.feeds[0].viewport.YOffset() != before {
		t.Fatal("help must scroll without moving the feed")
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Following") || !strings.Contains(plain, "Keyboard shortcuts") || !strings.Contains(plain, "╭") {
		t.Fatal("help must be a rounded overlay, not replace the feed")
	}
}

func TestHelpKeepsLabelsAndKeysOnOneRow(t *testing.T) {
	m := fixtureApp(t)
	plain := ansi.Strip(m.helpModal(colors(true)))
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "refresh") && !strings.Contains(line, "ctrl+r") {
			t.Fatal("help wrapped a shortcut away from its label")
		}
	}
	if !strings.Contains(plain, "╯") {
		t.Fatal("help clipped its bottom border")
	}
	if strings.Contains(ansi.Strip(m.navigation(5, 23, colors(true))), "…") {
		t.Fatal("compact navigation must show icons without truncation markers")
	}
}

func TestMouseIgnoresMinimumSizeNoticeAndRecoversAfterResize(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
	for _, size := range []tea.WindowSizeMsg{{Width: 8, Height: 18}, {Width: 80, Height: 8}} {
		updated, _ := m.Update(size)
		m = updated.(AppModel)
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 6, Y: 1})
		m = updated.(AppModel)
		if m.active != 0 {
			t.Fatal("hidden tabs accepted a mouse click")
		}
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(AppModel)
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 40, Y: 1})
	if updated.(AppModel).active != 1 {
		t.Fatal("visible tabs did not recover after resizing")
	}
}
