package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

var notificationCategories = [...]string{"replies", "quotes", "stars", "mentions", "follows", "moderation"}
var notificationLabels = [...]string{"Replies", "Quotes", "Stars", "Mentions", "Follows", "Moderation"}

type notificationFeed struct {
	items                []api.Notification
	groups               map[string]*notificationGroup
	viewport             viewport.Model
	starts, ends         []int
	actions              []hitArea
	selected, generation int
	loaded, loading      bool
	next, status         string
	readAt               time.Time
	cursors              map[string]bool
	cancel               context.CancelFunc
}

type notificationGroup struct {
	items                 []api.Notification
	open, loaded, loading bool
	next, status          string
	generation            int
	cursors               map[string]bool
	cancel                context.CancelFunc
}

type notificationEntry struct {
	notification api.Notification
	parent       string
}

type notificationResult struct {
	session, generation int
	filter, mode        string
	page                api.Notifications
	err                 error
}

type notificationGroupResult struct {
	session, generation int
	filter, id, cursor  string
	page                api.Notifications
	err                 error
}

type notificationCountResult struct {
	session, generation, count int
	err                        error
}

func (m AppModel) notificationFilter() string {
	if m.notificationMask == 63 {
		return "all"
	}
	var selected []string
	for i, category := range notificationCategories {
		if m.notificationMask&(1<<i) != 0 {
			selected = append(selected, category)
		}
	}
	if len(selected) == 0 {
		return "none"
	}
	return strings.Join(selected, ",")
}

func (m *AppModel) notificationFeed() *notificationFeed {
	if m.notificationFeeds == nil {
		m.notificationFeeds = make(map[string]*notificationFeed)
	}
	filter := m.notificationFilter()
	if m.notificationFeeds[filter] == nil {
		m.notificationFeeds[filter] = &notificationFeed{viewport: viewport.New(), groups: make(map[string]*notificationGroup), cursors: make(map[string]bool)}
	}
	return m.notificationFeeds[filter]
}

func (m *AppModel) cancelNotifications() {
	for _, f := range m.notificationFeeds {
		if f.cancel != nil {
			f.cancel()
			f.cancel = nil
		}
		f.loading = false
		m.notificationGeneration++
		f.generation = m.notificationGeneration
		for _, g := range f.groups {
			if g.cancel != nil {
				g.cancel()
				g.cancel = nil
			}
			g.loading = false
			m.notificationGeneration++
			g.generation = m.notificationGeneration
		}
	}
}

func (m AppModel) openNotifications() (AppModel, tea.Cmd) {
	m.closeAllThreads()
	m.closeViewer()
	m.resetSequence()
	if m.notificationsOpen {
		return m, nil
	}
	f := &m.feeds[m.active]
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
		f.loading = false
		f.generation++
	}
	m.notificationsOpen = true
	m.notice = ""
	m.reflow()
	return m, m.loadNotifications("open")
}

func (m *AppModel) invalidateNotificationCount() {
	if m.notificationCountCancel != nil {
		m.notificationCountCancel()
		m.notificationCountCancel = nil
	}
	m.notificationCountGeneration++
}

func (m *AppModel) loadNotificationCount() tea.Cmd {
	if m.fetchNotificationCount == nil || m.notificationCountCancel != nil {
		return nil
	}
	m.notificationCountGeneration++
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 20*time.Second)
	m.notificationCountCancel = cancel
	session, generation, fetch := m.session, m.notificationCountGeneration, m.fetchNotificationCount
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		count, err := fetch(ctx)
		return notificationCountResult{session, generation, count, err}
	})
}

func (m *AppModel) loadNotifications(mode string) tea.Cmd {
	f := m.notificationFeed()
	if f.loading || (mode == "more" && f.next == "") || m.fetchNotifications == nil {
		return nil
	}
	m.invalidateNotificationCount()
	m.notificationGeneration++
	f.generation = m.notificationGeneration
	f.loading = true
	f.status = ""
	cursor := ""
	if mode == "more" {
		cursor = f.next
		f.cursors[cursor] = true
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	f.cancel = cancel
	session, generation, filter, fetch := m.session, f.generation, m.notificationFilter(), m.fetchNotifications
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		page, err := fetch(ctx, filter, cursor)
		return notificationResult{session, generation, filter, mode, page, err}
	})
}

func (m *AppModel) acceptNotifications(msg notificationResult) tea.Cmd {
	if !m.connected || !m.notificationsOpen || msg.session != m.session || msg.filter != m.notificationFilter() {
		return nil
	}
	f := m.notificationFeed()
	if f.generation != msg.generation {
		return nil
	}
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.loading = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		f.status = safeText(msg.err.Error()) + ". Ctrl+R to retry."
		if msg.mode == "refresh" {
			return m.notify("error", "Refresh failed", f.status)
		}
		return nil
	}
	anchor, offset := m.notificationAnchor()
	m.reconcileNotificationItems(msg.page.Data)
	if msg.mode == "more" {
		f.items = uniqueNotifications(append(f.items, msg.page.Data...))
	} else {
		for _, g := range f.groups {
			if g.cancel != nil {
				g.cancel()
			}
		}
		f.items = uniqueNotifications(msg.page.Data)
		f.groups = make(map[string]*notificationGroup)
		f.cursors = make(map[string]bool)
		f.readAt = msg.page.ReadAt
		f.selected = 0
		f.viewport.GotoTop()
	}
	f.next, f.loaded = msg.page.Next, true
	if f.next != "" && f.cursors[f.next] {
		f.next = ""
		f.status = "Repeated pagination cursor. Ctrl+R to refresh."
	}
	m.invalidateNotificationCount()
	m.unreadNotifications = 0
	m.renderNotifications()
	if msg.mode == "more" || msg.mode == "filter" {
		m.restoreNotificationAnchor(anchor, offset)
	}
	if msg.mode == "refresh" {
		return tea.Batch(m.loadNotificationCount(), m.notify("success", "Notifications refreshed", "Inbox is up to date."))
	}
	return m.loadNotificationCount()
}

func uniqueNotifications(items []api.Notification) []api.Notification {
	seen := make(map[string]bool, len(items))
	result := make([]api.Notification, 0, len(items))
	for _, n := range items {
		if !seen[n.ID] {
			seen[n.ID] = true
			result = append(result, n)
		}
	}
	return result
}

func (m *AppModel) notificationEntries() []notificationEntry {
	f := m.notificationFeed()
	var entries []notificationEntry
	for _, n := range f.items {
		entries = append(entries, notificationEntry{notification: n})
		if g := f.groups[n.ID]; g != nil && g.open {
			for _, member := range g.items {
				entries = append(entries, notificationEntry{member, n.ID})
			}
		}
	}
	return entries
}

func (m *AppModel) loadNotificationGroup(id string, more bool) tea.Cmd {
	f := m.notificationFeed()
	g := f.groups[id]
	if g == nil || g.loading || m.fetchNotificationGroup == nil || (more && g.next == "") {
		return nil
	}
	cursor := ""
	if more {
		cursor = g.next
		g.cursors[cursor] = true
	}
	m.notificationGeneration++
	g.generation = m.notificationGeneration
	g.loading = true
	g.status = ""
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	g.cancel = cancel
	session, generation, filter, fetch := m.session, g.generation, m.notificationFilter(), m.fetchNotificationGroup
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		page, err := fetch(ctx, id, filter, cursor)
		return notificationGroupResult{session, generation, filter, id, cursor, page, err}
	})
}

func (m *AppModel) acceptNotificationGroup(msg notificationGroupResult) tea.Cmd {
	if !m.connected || !m.notificationsOpen || msg.session != m.session || msg.filter != m.notificationFilter() {
		return nil
	}
	f := m.notificationFeed()
	g := f.groups[msg.id]
	if g == nil || g.generation != msg.generation {
		return nil
	}
	anchor, offset := m.notificationAnchor()
	if g.cancel != nil {
		g.cancel()
		g.cancel = nil
	}
	g.loading = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		g.status = safeText(msg.err.Error()) + ". Space to close, reopen to retry."
	} else {
		m.reconcileNotificationItems(msg.page.Data)
		if msg.cursor == "" {
			g.items = uniqueNotifications(msg.page.Data)
		} else {
			g.items = uniqueNotifications(append(g.items, msg.page.Data...))
		}
		g.next, g.loaded = msg.page.Next, true
		if g.next != "" && g.cursors[g.next] {
			g.next = ""
			g.status = "Repeated group cursor. Close and reopen to retry."
		}
	}
	m.renderNotifications()
	m.restoreNotificationAnchor(anchor, offset)
	return nil
}

func (m *AppModel) reconcileNotificationItems(items []api.Notification) {
	var reconcile func(*api.Commit)
	reconcile = func(c *api.Commit) {
		if c == nil {
			return
		}
		if at, deleted := m.deletedNotificationCommits[c.ID]; deleted {
			*c = api.Commit{ID: c.ID, ParentID: c.ParentID, DeletedAt: &at, Replies: c.Replies}
			return
		}
		reconcile(c.Quote)
	}
	for _, n := range items {
		reconcile(n.Commit)
	}
}

func (m *AppModel) deleteNotificationCommit(id string) {
	if m.deletedNotificationCommits == nil {
		m.deletedNotificationCommits = make(map[string]time.Time)
	}
	m.deletedNotificationCommits[id] = m.now
	for _, f := range m.notificationFeeds {
		m.reconcileNotificationItems(f.items)
		for _, g := range f.groups {
			m.reconcileNotificationItems(g.items)
		}
	}
	if m.notificationsOpen {
		anchor, offset := m.notificationAnchor()
		m.renderNotifications()
		m.restoreNotificationAnchor(anchor, offset)
	}
}

func (m *AppModel) notificationAnchor() (notificationEntry, int) {
	f := m.notificationFeed()
	entries := m.notificationEntries()
	if len(entries) == 0 || f.selected >= len(f.starts) {
		return notificationEntry{}, 0
	}
	return entries[f.selected], f.viewport.YOffset() - f.starts[f.selected]
}

func (m *AppModel) restoreNotificationAnchor(anchor notificationEntry, offset int) {
	f := m.notificationFeed()
	for i, entry := range m.notificationEntries() {
		if entry.notification.ID == anchor.notification.ID && entry.parent == anchor.parent {
			f.selected = i
			m.renderNotifications()
			f.viewport.SetYOffset(max(0, f.starts[i]+offset))
			return
		}
	}
}

func (m AppModel) selectNotification(index int) (AppModel, tea.Cmd) {
	f := m.notificationFeed()
	entries := m.notificationEntries()
	if len(entries) == 0 {
		return m, nil
	}
	f.selected = max(0, min(len(entries)-1, index))
	m.renderNotifications()
	m.revealNotification()
	if f.selected == 0 {
		f.viewport.GotoTop()
	}
	if f.selected >= len(entries)-2 {
		return m, m.loadNotifications("more")
	}
	return m, nil
}

func (m *AppModel) revealNotification() {
	f := m.notificationFeed()
	if f.selected >= len(f.starts) {
		return
	}
	start, end := f.starts[f.selected], f.ends[f.selected]
	if start < f.viewport.YOffset() || end-start >= f.viewport.Height() {
		f.viewport.SetYOffset(start)
	} else if end > f.viewport.YOffset()+f.viewport.Height() {
		f.viewport.SetYOffset(end - f.viewport.Height())
	}
}

func (m AppModel) notificationKey(pressed string) (AppModel, tea.Cmd) {
	f := m.notificationFeed()
	entries := m.notificationEntries()
	switch pressed {
	case "esc":
		return m.switchLog(m.lastLog)
	case "1", "2", "3", "4", "5", "6":
		m.cancelNotifications()
		m.notificationMask ^= 1 << int(pressed[0]-'1')
		m.reflow()
		return m, m.loadNotifications("filter")
	case "ctrl+r":
		m.cancelNotifications()
		cmd := m.loadNotifications("refresh")
		return m, tea.Batch(cmd, m.notify("info", "Refreshing notifications", "Marking all notification categories read."))
	case "j", "down":
		return m.selectNotification(f.selected + 1)
	case "k", "up":
		return m.selectNotification(f.selected - 1)
	case "home":
		return m.selectNotification(0)
	case "end", "G":
		return m.selectNotification(len(entries) - 1)
	case "pgdown", "ctrl+d":
		return m.selectNotification(f.selected + 5)
	case "pgup", "ctrl+u":
		return m.selectNotification(f.selected - 5)
	}
	if len(entries) == 0 {
		return m, nil
	}
	entry := entries[f.selected]
	n := entry.notification
	id := n.ID
	if entry.parent != "" {
		id = entry.parent
	}
	switch pressed {
	case "space":
		g := f.groups[id]
		if g == nil {
			if n.EventCount <= 1 {
				return m, nil
			}
			g = &notificationGroup{cursors: make(map[string]bool)}
			f.groups[id] = g
		}
		g.open = !g.open
		if !g.open {
			if g.cancel != nil {
				g.cancel()
				g.cancel = nil
			}
			g.loading = false
			m.notificationGeneration++
			g.generation = m.notificationGeneration
			for i, e := range m.notificationEntries() {
				if e.parent == "" && e.notification.ID == id {
					f.selected = i
					break
				}
			}
		}
		var cmd tea.Cmd
		if g.open && (!g.loaded || g.status != "") {
			cmd = m.loadNotificationGroup(id, g.loaded && g.next != "")
		}
		m.renderNotifications()
		m.revealNotification()
		return m, cmd
	case "n":
		if g := f.groups[id]; g != nil && g.open && g.next != "" {
			return m, m.loadNotificationGroup(id, true)
		}
		return m, m.loadNotifications("more")
	case "enter", "r", "q", "y", "v":
		if n.Commit == nil {
			m.notice = "This notification has no commit to open."
			return m, nil
		}
		switch pressed {
		case "r":
			return m, m.composeTo("reply", *n.Commit)
		case "q":
			return m, m.composeTo("quote", *n.Commit)
		case "y":
			return m, m.copyCommitLink(*n.Commit)
		case "v":
			return m, m.openViewer(*n.Commit, 0)
		}
		return m, m.openThread(*n.Commit)
	}
	return m, nil
}

func (m *AppModel) notificationStatus() string {
	f := m.notificationFeed()
	if f.status != "" {
		return f.status
	}
	if f.loading {
		return "Reading notifications..."
	}
	if len(f.items) == 0 {
		return "No notifications in the selected categories."
	}
	return fmt.Sprintf("%d / %d notifications", f.selected+1, len(m.notificationEntries()))
}
