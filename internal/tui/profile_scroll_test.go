package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestJScrollsAProfileHeaderBeforeLeavingThePinnedCommit(t *testing.T) {
	m, _ := profileApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(AppModel)
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		profile := fixtureProfile(handle)
		pinned := fixtureCommit("pinned")
		pinned.Body = "Pinned words"
		profile.Pinned = &pinned
		return profile, nil
	}
	m.loadUserCommits = func(context.Context, string, bool, string) (api.CommitPage, error) {
		later := fixtureCommit("later")
		later.Body = "Later words"
		return api.CommitPage{Data: []api.Commit{later}}, nil
	}
	m = deliver(m, runAll(m.openProfile("grace")))
	s := m.topProfile()
	if s.starts[0] < s.viewport.Height() {
		t.Skip("the header fits, so there is nothing to scroll")
	}
	sawPinned, sawTabs := false, false
	for range 20 {
		if s = m.topProfile(); s.lists[s.tab].selected != 0 {
			break
		}
		view := ansi.Strip(s.viewport.View())
		sawPinned = sawPinned || strings.Contains(view, "Pinned words")
		sawTabs = sawTabs || strings.Contains(view, "Commits") && strings.Contains(view, "Replies")
		m, _ = m.handleKey("j")
	}
	if !sawPinned || !sawTabs {
		t.Fatalf("j left the pinned commit before showing it (pinned %v, tabs %v)", sawPinned, sawTabs)
	}
	for range 20 {
		m, _ = m.handleKey("k")
	}
	if s = m.topProfile(); s.lists[s.tab].selected != 0 || s.viewport.YOffset() != 0 {
		t.Fatalf("k did not return to the top: selected %d, offset %d", s.lists[s.tab].selected, s.viewport.YOffset())
	}
}
