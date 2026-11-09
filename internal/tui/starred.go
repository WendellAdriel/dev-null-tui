package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type starredResult struct {
	session, generation int
	mode                string
	page                api.CommitPage
	err                 error
}

func (m AppModel) starredOnTop() bool {
	return m.starredOpen && len(m.threads) == 0 && len(m.profiles) == 0
}

func (m *AppModel) closeStarred() {
	if m.starred.cancel != nil {
		m.starred.cancel()
	}
	m.starred = feed{generation: m.starred.generation + 1}
	m.starredOpen = false
	m.starredRemoved = nil
	m.commitSearch = ""
	if m.scheduled != nil && m.scheduled.cancelRequested != nil {
		m.scheduled.cancelRequested()
	}
	m.scheduled = nil
	m.drafts = draftsState{}
}

func (m AppModel) openStarred() (AppModel, tea.Cmd) { return m.openList("") }

// openCommitSearch shows a search's matching commits, the web's See all matching commits, in the Starred list screen.
func (m AppModel) openCommitSearch(term string) (AppModel, tea.Cmd) { return m.openList(term) }

func (m AppModel) openList(term string) (AppModel, tea.Cmd) {
	if m.loadStarred == nil || (term != "" && m.searchCalls.commits == nil) {
		return m, nil
	}
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	m.cancelNotifications()
	m.notificationsOpen = false
	m.closeStarred()
	m.closeRemotes()
	f := &m.feeds[m.active]
	if f.cancel != nil {
		f.cancel()
		f.cancel, f.loading = nil, false
		f.generation++
	}
	m.starredOpen, m.commitSearch = true, term
	m.starred.viewport = viewport.New()
	m.starred.cursors = make(map[string]bool)
	m.starredRemoved = make(map[string]bool)
	m.notice = ""
	m.resetSequence()
	m.reflow()
	cmd := m.fetchStarred("open")
	return m, cmd
}

func (m *AppModel) fetchStarred(mode string) tea.Cmd {
	f := &m.starred
	if !m.starredOpen || (mode == "more" && (f.loading || f.next == "")) {
		return nil
	}
	if f.cancel != nil {
		f.cancel()
	}
	cursor := ""
	if mode == "more" {
		cursor = f.next
		f.cursors[cursor] = true
	} else {
		f.cursors = make(map[string]bool)
	}
	f.generation++
	f.loading, f.loadingMode, f.status = true, mode, ""
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	f.cancel = cancel
	session, generation, load, term, search := m.session, f.generation, m.loadStarred, m.commitSearch, m.searchCalls.commits
	m.renderStarred()
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		if term != "" {
			commits, err := search(ctx, term, commitResultsLimit)
			return starredResult{session: session, generation: generation, mode: mode, page: api.CommitPage{Data: commits}, err: err}
		}
		page, err := load(ctx, cursor)
		return starredResult{session: session, generation: generation, mode: mode, page: page, err: err}
	})
}

func (m *AppModel) acceptStarred(msg starredResult) tea.Cmd {
	f := &m.starred
	if !m.connected || !m.starredOpen || msg.session != m.session || msg.generation != f.generation {
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
		var failure *auth.APIError
		if m.commitSearch != "" && errors.As(msg.err, &failure) && failure.Status == http.StatusTooManyRequests {
			f.status = "error: too many searches, give it a few seconds. Ctrl+R to retry."
			m.renderStarred()
			return nil
		}
		f.status = "Couldn't load " + m.listName() + ": " + safeText(msg.err.Error()) + ". Ctrl+R to retry."
		m.renderStarred()
		return m.notify("error", m.listName()+" failed", f.status)
	}
	if msg.mode != "more" {
		f.commits, f.selected = nil, 0
	}
	for _, c := range msg.page.Data {
		if !m.starredRemoved[c.ID] {
			f.commits = append(f.commits, c)
		}
	}
	f.commits = uniqueCommits(f.commits)
	f.next, f.loaded = msg.page.Next, true
	if f.cursors[f.next] {
		f.next, f.status = "", "Repeated pagination cursor. Ctrl+R to refresh."
	}
	m.renderStarred()
	if msg.mode != "more" {
		f.viewport.GotoTop()
	}
	if len(f.commits) == 0 && f.next != "" {
		return m.fetchStarred("more")
	}
	return nil
}

// listName names the list screen in its messages.
func (m AppModel) listName() string {
	if m.commitSearch != "" {
		return "search results"
	}
	return "Starred"
}

func (m *AppModel) removeStarred(id string) tea.Cmd {
	if !m.starredOpen {
		return nil
	}
	m.starredRemoved[id] = true
	f := &m.starred
	kept := make([]api.Commit, 0, len(f.commits))
	removed := false
	for i, c := range f.commits {
		if c.ID == id {
			removed = true
			if i < f.selected {
				f.selected--
			}
			continue
		}
		kept = append(kept, c)
	}
	f.commits = kept
	f.selected = max(0, min(f.selected, len(kept)-1))
	m.renderStarred()
	m.revealStarredSelection()
	if removed && len(kept) == 0 && f.next != "" {
		return m.fetchStarred("refresh")
	}
	return nil
}

func (m *AppModel) selectStarred(index int) tea.Cmd {
	f := &m.starred
	if len(f.commits) == 0 {
		return nil
	}
	previous := f.selected
	f.selected = max(0, min(index, len(f.commits)-1))
	m.notice = ""
	m.renderStarred()
	m.revealStarredSelection()
	if f.selected == 0 {
		f.viewport.GotoTop()
	}
	if f.selected > previous && f.selected >= len(f.commits)-2 {
		return m.fetchStarred("more")
	}
	return nil
}

func (m AppModel) starredKey(pressed string) (AppModel, tea.Cmd) {
	if m.scheduled != nil {
		return m.scheduledKey(pressed)
	}
	f := &m.starred
	switch pressed {
	case "esc":
		return m.switchLog(m.active)
	case "j", "down":
		return m, m.selectStarred(f.selected + 1)
	case "k", "up":
		return m, m.selectStarred(f.selected - 1)
	case "pgdown", "ctrl+d":
		return m, m.selectStarred(f.selected + 5)
	case "pgup", "ctrl+u":
		return m, m.selectStarred(f.selected - 5)
	case "home":
		return m, m.selectStarred(0)
	case "G", "end":
		return m, m.selectStarred(len(f.commits) - 1)
	case "n":
		return m, m.fetchStarred("more")
	case "ctrl+r":
		m.starredRemoved = make(map[string]bool)
		cmd := m.fetchStarred("refresh")
		return m, cmd
	}
	if len(f.commits) == 0 {
		return m, nil
	}
	c := f.commits[f.selected]
	if cmd, ok := m.commitShortcut(c, pressed); ok {
		return m, cmd
	}
	switch pressed {
	case "enter":
		return m, m.openThread(c)
	case "u":
		if hasProfile(c.Author) {
			return m, m.openAuthor(c.Author)
		}
	case "r", "q":
		return m, m.composeTo(map[string]string{"r": "reply", "q": "quote"}[pressed], c)
	case "s":
		return m, m.startEngagement(c, "star")
	case "p":
		return m, m.startEngagement(c, "pastel")
	case "v":
		return m, m.openViewer(c, 0)
	case "D":
		m.confirmDelete(c)
	}
	return m, nil
}

func (m AppModel) starredStatus() string {
	if m.scheduled != nil {
		return m.scheduledStatusLine()
	}
	f := m.starred
	if f.status != "" {
		return f.status
	}
	if f.loading {
		return "Reading " + m.listName() + " from /dev/null..."
	}
	if m.commitSearch != "" {
		if len(f.commits) == 0 {
			return "fatal: nothing close to '" + safeText(m.commitSearch) + "'"
		}
		return fmt.Sprintf("%d / %d matching commits", f.selected+1, len(f.commits))
	}
	if len(f.commits) == 0 {
		return "No starred commits yet."
	}
	ending := "end of list"
	if f.next != "" {
		ending = "more available"
	}
	return fmt.Sprintf("%d / %d starred commits  ·  %s", f.selected+1, len(f.commits), ending)
}
