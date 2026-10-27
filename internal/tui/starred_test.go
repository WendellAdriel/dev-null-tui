package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func starredApp(t *testing.T) (AppModel, *[]string) {
	t.Helper()
	m, _ := profileApp(t)
	calls := &[]string{}
	m.loadStarred = func(_ context.Context, cursor string) (api.CommitPage, error) {
		*calls = append(*calls, cursor)
		page := api.CommitPage{Data: []api.Commit{}}
		ids := []string{"s1", "s2", "s3"}
		if cursor == "" {
			page.Next = "older"
		} else {
			ids = []string{"s3", "s4"}
		}
		for _, id := range ids {
			c := fixtureCommit(id)
			c.Author.ID = "ada"
			c.Starred, c.Stars = new(true), 3
			page.Data = append(page.Data, c)
		}
		return page, nil
	}
	m.setStar = func(_ context.Context, id string, star bool) (api.Commit, error) {
		c := fixtureCommit(id)
		c.Starred, c.Stars = &star, 2
		return c, nil
	}
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		c := fixtureCommit(id)
		c.Author.ID, c.Starred = "ada", new(true)
		return api.Thread{Commit: c}, nil
	}
	return m, calls
}

func TestStarredOpenPaginationAndStackedFocusReturn(t *testing.T) {
	m, calls := starredApp(t)
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	if !m.starredOpen || len(*calls) != 1 || (*calls)[0] != "" {
		t.Fatalf("open: %v", calls)
	}
	for _, want := range []string{"Starred", "Only you", "Commits you starred, newest first."} {
		if !strings.Contains(frameText(m), want) {
			t.Fatalf("missing %q", want)
		}
	}
	m = press(t, m, "j")
	if len(m.starred.commits) != 4 || m.starred.selected != 1 || fmt.Sprint(*calls) != "[ older]" {
		t.Fatalf("pagination: %+v %v", m.starred, calls)
	}
	offset := m.starred.viewport.YOffset()
	m = press(t, m, "enter", "u", "esc", "esc")
	if !m.starredOpen || len(m.profiles) != 0 || len(m.threads) != 0 || m.starred.selected != 1 || m.starred.viewport.YOffset() != offset || len(*calls) != 2 {
		t.Fatal("stack return lost the starred entry or reloaded")
	}
	if !strings.Contains(ansi.Strip(m.starred.viewport.GetContent()), "You've reached the end of your starred commits.") {
		t.Fatal("missing end state")
	}
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	if len(*calls) != 3 || m.starred.selected != 0 || len(m.starred.commits) != 3 {
		t.Fatal("reopen didn't fetch first page")
	}
}

func TestStarredConfirmedRemovalAndFailedUnstar(t *testing.T) {
	m, _ := starredApp(t)
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	m.starred.next = ""
	m.starred.selected = 1
	m.setStar = func(context.Context, string, bool) (api.Commit, error) { return api.Commit{}, errors.New("offline") }
	m = press(t, m, "s")
	if len(m.starred.commits) != 3 || m.starred.selected != 1 || m.toast.kind != "error" {
		t.Fatalf("failed unstar: count=%d selected=%d toast=%+v engagement=%+v", len(m.starred.commits), m.starred.selected, m.toast, m.engagement)
	}
	m.setStar = func(_ context.Context, id string, star bool) (api.Commit, error) {
		c := fixtureCommit(id)
		c.Starred, c.Stars = &star, 2
		return c, nil
	}
	next, cmd := m.handleKey("s")
	m = next
	if len(m.starred.commits) != 3 {
		t.Fatal("unstar removed before confirmation")
	}
	_, duplicate := m.handleKey("s")
	if duplicate != nil {
		t.Fatal("duplicate mutation")
	}
	m = deliver(m, runAll(cmd))
	if len(m.starred.commits) != 2 || m.starred.commits[m.starred.selected].ID != "s3" {
		t.Fatal("removal didn't keep next entry")
	}
	m = press(t, m, "s")
	if len(m.starred.commits) != 1 || m.starred.selected != 0 {
		t.Fatal("last removal didn't keep previous entry")
	}
	m = press(t, m, "enter", "s")
	if len(m.starred.commits) != 0 || len(m.threads) != 1 {
		t.Fatal("hidden list wasn't reconciled")
	}
	m = press(t, m, "esc")
	for _, want := range []string{"fatal: no starred commits yet", "Star a commit to find it here."} {
		if !strings.Contains(frameText(m), want) {
			t.Fatalf("missing empty state %q", want)
		}
	}
}

func TestStarredRefillsEmptyPageAndRejectsStaleResults(t *testing.T) {
	m, calls := starredApp(t)
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	m.starred.commits = m.starred.commits[:1]
	m, cmd := m.handleKey("s")
	msg := runAll(cmd)[0].(engagementResult)
	cmd = m.acceptEngagement(msg)
	m = deliver(m, runAll(cmd))
	if len(*calls) != 2 || (*calls)[1] != "" {
		t.Fatalf("empty page wasn't refetched: calls=%v count=%d engagement=%+v", *calls, len(m.starred.commits), m.engagement)
	}
	if m.starred.commits[0].ID == "s1" {
		t.Fatal("late page restored a confirmed unstar")
	}
	m, cmd = m.handleKey("ctrl+r")
	msgs := runAll(cmd)
	m = press(t, m, "esc")
	m = deliver(m, msgs)
	if m.starredOpen || len(m.starred.commits) != 0 {
		t.Fatal("closed screen accepted late data")
	}
}

func TestStarredSidebarAndResponsiveFrames(t *testing.T) {
	for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}} {
		for _, dark := range []bool{true, false} {
			m, calls := starredApp(t)
			m.account.width, m.account.height, m.account.dark = size[0], size[1], dark
			m.reflow()
			nav, _, _ := m.columns()
			_, areas := m.navigationLayout(nav, m.mainHeight(), colors(dark))
			var target hitArea
			for _, a := range areas {
				if a.action == "starred" {
					target = a
				}
			}
			if target.w == 0 || target.y != 10 {
				t.Fatal("missing starred sidebar target")
			}
			m, cmd := m.handleMouse(tea.MouseClickMsg{X: target.x, Y: target.y, Button: tea.MouseLeft})
			m = deliver(m, runAll(cmd))
			if !m.starredOpen || len(*calls) != 1 {
				t.Fatal("sidebar didn't open Starred")
			}
			for _, line := range strings.Split(m.View().Content, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("frame overflow")
				}
			}
		}
	}
}

func TestStarredGlobalNavigationAndIsolation(t *testing.T) {
	m, calls := starredApp(t)
	m = press(t, m, "enter")
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	if !m.starredOnTop() || len(*calls) != 1 {
		t.Fatal("g s from a thread didn't open Starred")
	}
	m = press(t, m, "u")
	c := fixtureCommit("not-in-starred")
	c.Starred = new(true)
	m.reconcileEngagement(c, "star")
	if len(m.starred.commits) != 3 {
		t.Fatal("star elsewhere inserted an entry")
	}
	c.ID, c.Starred = "s2", new(false)
	m.reconcileEngagement(c, "star")
	m.removeStarred(c.ID)
	if len(m.starred.commits) != 2 || !m.profileOnTop() {
		t.Fatal("hidden profile unstar didn't remove entry")
	}
	m = press(t, m, "esc")
	m, _ = m.handleKey("g")
	m = press(t, m, "l")
	if m.starredOpen {
		t.Fatal("g l didn't leave Starred")
	}
}

func TestStarredRefreshFailureRetainsPositionAndLatePagesCannotRestoreUnstars(t *testing.T) {
	m, _ := starredApp(t)
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	m = press(t, m, "j")
	offset := m.starred.viewport.YOffset()
	m.loadStarred = func(context.Context, string) (api.CommitPage, error) { return api.CommitPage{}, errors.New("offline") }
	m = press(t, m, "ctrl+r")
	if m.starred.selected != 1 || m.starred.viewport.YOffset() != offset || len(m.starred.commits) != 4 || m.starred.status == "" {
		t.Fatal("failed refresh lost position")
	}
	late := fixtureCommit("s2")
	late.Starred = new(true)
	m.starred.loading = true
	m.removeStarred("s2")
	m.acceptStarred(starredResult{session: m.session, generation: m.starred.generation, mode: "more", page: api.CommitPage{Data: []api.Commit{late}}})
	for _, c := range m.starred.commits {
		if c.ID == "s2" {
			t.Fatal("late page restored an unstar")
		}
	}
}

func TestStarredEscResumesAnInterruptedInitialLog(t *testing.T) {
	m, _ := starredApp(t)
	f := &m.feeds[m.active]
	f.commits, f.loaded, f.loading = nil, false, false
	initial := m.load("initial")
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	m = deliver(m, runAll(initial))
	if m.feeds[m.active].loaded {
		t.Fatal("canceled log response wasn't rejected")
	}
	m, cmd := m.handleKey("esc")
	if cmd == nil || !m.feeds[m.active].loading {
		t.Fatal("Esc didn't resume the canceled initial load")
	}
	m = deliver(m, runAll(cmd))
	if m.starredOpen || !m.feeds[m.active].loaded || len(m.feeds[m.active].commits) == 0 {
		t.Fatal("log didn't resume")
	}
}

func TestStarredReplyCountsReconcileFromTheListAndStackedThread(t *testing.T) {
	for _, thread := range []bool{false, true} {
		t.Run(fmt.Sprint("thread=", thread), func(t *testing.T) {
			m, calls := starredApp(t)
			m, _ = m.handleKey("g")
			m = press(t, m, "s", "j")
			before, offset, requests := m.starred.commits[1].Replies, m.starred.viewport.YOffset(), len(*calls)
			if thread {
				m = press(t, m, "enter")
			}
			m = press(t, m, "r")
			m.submit = func(_ context.Context, draft api.NewCommit) (api.Commit, error) {
				c := fixtureCommit("submitted-reply")
				c.ParentID = &draft.ParentID
				return c, nil
			}
			m.composer.input.SetValue("A synthetic reply")
			m = deliver(m, runAll(m.startSubmission()))
			if thread {
				m = press(t, m, "esc")
			}
			if m.starred.commits[1].Replies != before+1 || m.starred.selected != 1 || m.starred.viewport.YOffset() != offset || len(*calls) != requests {
				t.Fatal("confirmed reply didn't update Starred in place")
			}
			if !strings.Contains(ansi.Strip(m.starred.viewport.GetContent()), "1 reply") {
				t.Fatal("Starred didn't rerender the reply count")
			}
		})
	}
}
