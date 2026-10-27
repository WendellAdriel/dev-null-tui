package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func fixtureNotification(id string) api.Notification {
	c := fixtureCommit("commit-" + id)
	c.Body = "commit-" + id
	return api.Notification{ID: id, GroupID: id, Type: "starred", Actor: c.Author, Actors: []api.Author{*c.Author}, Commit: &c, EventCount: 3, ActorsCount: 2, CreatedAt: c.CreatedAt}
}

func inboxFixture(t *testing.T) AppModel {
	t.Helper()
	m := fixtureApp(t)
	m.fetchNotifications = func(context.Context, string, string) (api.Notifications, error) {
		return api.Notifications{}, errors.New("fixture unavailable")
	}
	m.fetchNotificationGroup = func(context.Context, string, string, string) (api.Notifications, error) {
		return api.Notifications{}, errors.New("fixture unavailable")
	}
	m, _ = m.openNotifications()
	return m
}

func inboxPage(m AppModel, notifications ...api.Notification) AppModel {
	f := m.notificationFeed()
	updated, _ := m.Update(notificationResult{session: m.session, generation: f.generation, filter: m.notificationFilter(), page: api.Notifications{Data: notifications, ReadAt: m.now}})
	return updated.(AppModel)
}

func TestInboxBoundaryPaginationAndThreadReturn(t *testing.T) {
	m := inboxFixture(t)
	newer, older := fixtureNotification("new"), fixtureNotification("old")
	read := m.now.Add(-time.Hour)
	older.ReadAt = &read
	m = inboxPage(m, newer, older)
	content := ansi.Strip(m.notificationFeed().viewport.GetContent())
	if strings.Index(content, "HEAD@{last visit}") <= strings.Index(content, "commit-new") || !strings.Contains(content, "oldest notification") {
		t.Fatalf("missing boundary or terminator:\n%s", content)
	}
	m, _ = m.selectNotification(1)
	offset := m.notificationFeed().viewport.YOffset()
	m, _ = m.notificationKey("enter")
	if len(m.threads) != 1 || m.threads[0].root.ID != older.Commit.ID {
		t.Fatal("did not open referenced commit")
	}
	m, _ = m.handleKey("esc")
	if m.notificationFeed().selected != 1 || m.notificationFeed().viewport.YOffset() != offset {
		t.Fatal("thread return lost inbox focus")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}} {
		updated, _ := m.Update(size)
		m = updated.(AppModel)
		for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if ansi.StringWidth(line) > size.Width {
				t.Fatalf("overflow at %dx%d: %q", size.Width, size.Height, line)
			}
		}
	}
}

func TestInboxFiltersExpansionAndStaleResults(t *testing.T) {
	m := inboxPage(inboxFixture(t), fixtureNotification("a"), fixtureNotification("b"))
	f := m.notificationFeed()
	m, _ = m.notificationKey("space")
	g := f.groups["a"]
	if g == nil || !g.open || !g.loading {
		t.Fatal("group not loading")
	}
	member := fixtureNotification("member")
	updated, _ := m.Update(notificationGroupResult{session: m.session, generation: g.generation, filter: m.notificationFilter(), id: "a", page: api.Notifications{Data: []api.Notification{member}}})
	m = updated.(AppModel)
	if len(m.notificationEntries()) != 3 {
		t.Fatal("missing expanded member")
	}
	m, _ = m.selectNotification(1)
	m, _ = m.notificationKey("space")
	if len(m.notificationEntries()) != 2 || m.notificationFeed().selected != 0 {
		t.Fatal("collapse failed to restore parent")
	}
	oldGeneration := f.generation
	m, _ = m.notificationKey("1")
	if m.notificationFilter() != "quotes,stars,mentions,follows,moderation" {
		t.Fatal("filter is not an independent toggle")
	}
	updated, _ = m.Update(notificationResult{session: m.session, generation: oldGeneration, filter: "all", page: api.Notifications{Data: []api.Notification{fixtureNotification("stale")}, ReadAt: m.now}})
	m = updated.(AppModel)
	if len(m.notificationFeed().items) != 0 {
		t.Fatal("stale filter response accepted")
	}
	m, _ = m.notificationKey("1")
	if len(m.notificationFeed().items) != 2 {
		t.Fatal("filter return lost cached rows")
	}
}

func TestBackgroundBadgeNeverOpensInboxAndStaleCountsCannotRestoreBadge(t *testing.T) {
	m := fixtureApp(t)
	lists, counts := 0, 0
	m.fetchNotifications = func(context.Context, string, string) (api.Notifications, error) {
		lists++
		return api.Notifications{}, nil
	}
	m.fetchNotificationCount = func(context.Context) (int, error) { counts++; return 4, nil }
	cmd := m.loadNotificationCount()
	msg := cmd()
	if lists != 0 || counts != 1 {
		t.Fatal("count fetched inbox")
	}
	m, _ = m.openNotifications()
	updated, _ := m.Update(msg)
	m = updated.(AppModel)
	if m.unreadNotifications != 0 {
		t.Fatal("stale count restored read badge")
	}
	m = inboxPage(m, fixtureNotification("a"))
	m, _ = m.switchLog(m.active)
	if m.notificationsOpen {
		t.Fatal("same log index did not leave inbox")
	}
	updated, _ = m.Update(notificationResult{session: m.session, generation: m.notificationGeneration - 1, filter: "all", page: api.Notifications{Data: []api.Notification{fixtureNotification("late")}, ReadAt: m.now}})
	if updated.(AppModel).notificationsOpen {
		t.Fatal("late result reopened inbox")
	}
}

func TestGroupArrivalPreservesAnotherRowAndPaginationKeepsReadBoundary(t *testing.T) {
	m := inboxPage(inboxFixture(t), fixtureNotification("a"), fixtureNotification("b"))
	m, _ = m.notificationKey("space")
	f := m.notificationFeed()
	g := f.groups["a"]
	m, _ = m.selectNotification(1)
	updated, _ := m.Update(notificationGroupResult{session: m.session, generation: g.generation, filter: "all", id: "a", page: api.Notifications{Data: []api.Notification{fixtureNotification("child")}}})
	m = updated.(AppModel)
	if entries := m.notificationEntries(); entries[f.selected].notification.ID != "b" {
		t.Fatal("group response moved focus to a different notification")
	}
	boundary := f.readAt
	f.next = "page2"
	cmd := m.loadNotifications("more")
	if cmd == nil {
		t.Fatal("pagination unavailable")
	}
	older := fixtureNotification("old")
	oldRead := boundary.Add(-time.Hour)
	older.ReadAt = &oldRead
	updated, _ = m.Update(notificationResult{session: m.session, generation: f.generation, filter: "all", mode: "more", page: api.Notifications{Data: []api.Notification{older}, ReadAt: boundary.Add(time.Minute)}})
	m = updated.(AppModel)
	if !f.readAt.Equal(boundary) || len(f.items) != 3 || strings.Count(ansi.Strip(f.viewport.GetContent()), "HEAD@{last visit}") != 1 {
		t.Fatal("pagination changed original read boundary")
	}
	selected := f.selected
	offset := f.viewport.YOffset()
	cmd = m.loadNotifications("refresh")
	if cmd == nil {
		t.Fatal("refresh unavailable")
	}
	updated, _ = m.Update(notificationResult{session: m.session, generation: f.generation, filter: "all", mode: "refresh", err: errors.New("offline")})
	m = updated.(AppModel)
	if f.selected != selected || f.viewport.YOffset() != offset || len(f.items) != 3 || !strings.Contains(f.status, "offline") {
		t.Fatal("failed refresh lost inbox")
	}
}

func TestInboxMouseTargetsFiltersAndGroupControls(t *testing.T) {
	m := inboxPage(inboxFixture(t), fixtureNotification("a"))
	_, width, _ := m.columns()
	header, areas := m.notificationHeader(width, colors(true))
	filter := areas[0]
	m, _ = m.notificationMouse(filter.x, filter.y)
	if m.notificationMask&1 != 0 {
		t.Fatal("filter click ignored")
	}
	m, _ = m.notificationKey("1")
	f := m.notificationFeed()
	m, _ = m.notificationMouse(9, lipgloss.Height(header)+f.ends[0]-2-f.viewport.YOffset())
	if g := f.groups["a"]; g == nil || !g.open {
		t.Fatal("group control click opened thread instead")
	}
}

func TestDeletingFromNotificationThreadClearsEveryCachedExcerpt(t *testing.T) {
	m := inboxPage(inboxFixture(t), fixtureNotification("a"), fixtureNotification("b"))
	deleted := m.notificationFeed().items[0].Commit.ID
	m.notificationFeed().groups["a"] = &notificationGroup{open: true, loaded: true, items: []api.Notification{fixtureNotification("a")}}
	m.notificationMask ^= 1
	m.notificationFeed().items = []api.Notification{fixtureNotification("a")}
	m.notificationMask = 63
	m.renderNotifications()
	m, _ = m.notificationKey("enter")
	updated, _ := m.Update(deleteResult{session: m.session, id: deleted})
	m = updated.(AppModel)
	m, _ = m.handleKey("esc")
	for _, f := range m.notificationFeeds {
		for _, n := range f.items {
			if n.Commit.ID == deleted && (n.Commit.DeletedAt == nil || n.Commit.Author != nil || n.Commit.Body != "") {
				t.Fatal("cached deleted commit retained content")
			}
		}
		for _, g := range f.groups {
			for _, n := range g.items {
				if n.Commit.ID == deleted && (n.Commit.DeletedAt == nil || n.Commit.Author != nil || n.Commit.Body != "") {
					t.Fatal("expanded member retained deleted content")
				}
			}
		}
	}
	if content := ansi.Strip(m.notificationFeed().viewport.GetContent()); strings.Contains(content, "commit-a") || !strings.Contains(content, "Commit deleted") {
		t.Fatal("deleted excerpt remains visible")
	}
	m, _ = m.notificationKey("enter")
	if len(m.threads) != 0 || m.notificationFeed().selected != 0 {
		t.Fatal("deleted commit reopened or focus changed")
	}
	m.notificationFeed().groups["a"].generation = 99
	updated, _ = m.Update(notificationGroupResult{session: m.session, generation: 99, filter: "all", id: "a", page: api.Notifications{Data: []api.Notification{fixtureNotification("a")}}})
	m = updated.(AppModel)
	if c := m.notificationFeed().groups["a"].items[0].Commit; c.DeletedAt == nil || c.Body != "" {
		t.Fatal("late response restored deleted content")
	}
}

func TestNotificationBadgeKeepsNavigationShortcutVisible(t *testing.T) {
	m := fixtureApp(t)
	m.unreadNotifications = 27
	column, _ := m.navigationLayout(25, 35, colors(true))
	var notificationRow string
	for _, line := range strings.Split(ansi.Strip(column), "\n") {
		if strings.Contains(line, "Notifications") {
			notificationRow = line
		}
	}
	if !strings.Contains(notificationRow, "g n") {
		t.Fatalf("badge clipped shortcut: %q", notificationRow)
	}
}
