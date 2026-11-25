package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestShiftZPutsTheFocusedCommitAtTheTopOfTheLog(t *testing.T) {
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(AppModel)
	commits := []api.Commit{}
	for i := range 12 {
		commits = append(commits, fixtureCommit(fmt.Sprint("c", i)))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	f := &m.feeds[0]
	for _, at := range []int{0, 5, 11} {
		for f.selected < at {
			m, _ = m.handleKey("j")
			f = &m.feeds[0]
		}
		m, _ = m.handleKey("Z")
		f = &m.feeds[0]
		if f.viewport.YOffset() != f.starts[at] {
			t.Fatalf("commit %d starts at line %d, but the log scrolled to %d", at, f.starts[at], f.viewport.YOffset())
		}
	}
	m.reflow()
	if f = &m.feeds[0]; f.viewport.YOffset() != f.starts[11] {
		t.Fatalf("a resize must keep the last commit at the top, got %d for %d", f.viewport.YOffset(), f.starts[11])
	}
}

func TestShiftZLeavesTheLogAloneUnderScreensWithoutCommits(t *testing.T) {
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(AppModel)
	commits := []api.Commit{}
	for i := range 12 {
		commits = append(commits, fixtureCommit(fmt.Sprint("c", i)))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	m.feeds[0].selected = 11
	m.directory.open = true
	m, _ = m.handleKey("Z")
	if offset := m.feeds[0].viewport.YOffset(); offset != 0 {
		t.Fatalf("Z on the Guilds directory scrolled the Log under it to %d", offset)
	}
}

func TestShiftZPutsTheFocusedCommitAtTheTopOfStarredAndThreads(t *testing.T) {
	m, _ := starredApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(AppModel)
	m, _ = m.handleKey("g")
	m = press(t, m, "s", "j", "j", "Z")
	if f := m.starred; f.viewport.YOffset() != f.starts[f.selected] {
		t.Fatalf("starred commit %d starts at %d, viewport at %d", f.selected, f.starts[f.selected], f.viewport.YOffset())
	}

	m = threadApp(t)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(AppModel)
	m = openThread(t, m)
	th := &m.threads[len(m.threads)-1]
	last := len(th.starts) - 1
	for th.selected < last {
		m, _ = m.handleKey("j")
		th = &m.threads[len(m.threads)-1]
	}
	m, _ = m.handleKey("Z")
	th = &m.threads[len(m.threads)-1]
	if th.viewport.YOffset() != th.starts[last] {
		t.Fatalf("the last reply starts at %d, viewport at %d", th.starts[last], th.viewport.YOffset())
	}
}

func TestTheLogScrollbarIgnoresTheRoomBelowTheLastCommit(t *testing.T) {
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(AppModel)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
	p := m.account.colors()
	if strings.Contains(m.feedViewport(p), "▐") {
		t.Fatal("a Log that fits must not draw a scrollbar")
	}
	commits := []api.Commit{}
	for i := range 12 {
		commits = append(commits, fixtureCommit(fmt.Sprint("c", i)))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	if !strings.Contains(m.feedViewport(p), "▐") {
		t.Fatal("a long Log must draw its scrollbar")
	}
}
