package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// undoWindow is the server's Commit::RESTORABLE_SECONDS.
const undoWindow = 30 * time.Second

// listSpot is where a deleted commit sat in a loaded list, so a restore can put it back in place.
type listSpot struct {
	feed, list, index int
	handle            string
}

type undoState struct {
	id, hash  string
	deadline  time.Time
	toast     int
	restoring bool
	feeds     []listSpot
	profiles  []listSpot
	pinned    []string
	starred   int
}

type undoTick struct {
	session int
	id      string
}

type restoreResult struct {
	session int
	id      string
	commit  api.Commit
	err     error
}

// rememberSpots records where a commit is about to be removed from feeds, profiles, and Starred.
func (m *AppModel) rememberSpots(id string, sentAt time.Time) {
	u := undoState{id: id, deadline: sentAt.Add(undoWindow), starred: -1}
	m.eachCommit(func(c *api.Commit) {
		if c != nil && c.ID == id && c.DeletedAt == nil && u.hash == "" {
			u.hash = c.Hash
		}
	})
	for i, f := range m.feeds {
		if j := slices.IndexFunc(f.commits, func(c api.Commit) bool { return c.ID == id }); j >= 0 {
			u.feeds = append(u.feeds, listSpot{feed: i, index: j})
		}
	}
	for _, s := range m.profiles {
		if s.people {
			continue
		}
		for l, list := range s.lists {
			if j := slices.IndexFunc(list.commits, func(c api.Commit) bool { return c.ID == id }); j >= 0 {
				u.profiles = append(u.profiles, listSpot{list: l, index: j, handle: s.handle})
			}
		}
		if s.profile.Pinned != nil && s.profile.Pinned.ID == id {
			u.pinned = append(u.pinned, s.handle)
		}
	}
	if m.starredOpen {
		u.starred = slices.IndexFunc(m.starred.commits, func(c api.Commit) bool { return c.ID == id })
	}
	m.undo = u
}

// offerUndo shows the deleted toast with its countdown, refreshed every second until the window closes.
func (m *AppModel) offerUndo() tea.Cmd {
	m.showUndoToast(time.Now())
	return m.undoTicker()
}

func (m *AppModel) undoTicker() tea.Cmd {
	session, id := m.session, m.undo.id
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return undoTick{session, id} })
}

func (m *AppModel) showUndoToast(now time.Time) {
	left := max(0, int(m.undo.deadline.Sub(now).Round(time.Second)/time.Second))
	detail := fmt.Sprintf("%ds to undo. Press z or click here.", left)
	if m.undo.hash != "" {
		detail = m.undo.hash + " · " + detail
	}
	m.showToast("success", "Commit deleted", detail)
	m.undo.toast = m.toast.id
}

func (m *AppModel) acceptUndoTick(msg undoTick) tea.Cmd {
	if msg.session != m.session || msg.id != m.undo.id || m.undo.restoring {
		return nil
	}
	now := time.Now()
	if !now.Before(m.undo.deadline) {
		showing := m.toast.visible && m.toast.id == m.undo.toast
		m.undo = undoState{}
		if showing {
			return m.notify("info", "Undo window expired", "The commit stays deleted.")
		}
		return nil
	}
	if m.toast.visible && m.toast.id == m.undo.toast {
		m.showUndoToast(now)
	}
	return m.undoTicker()
}

func (m *AppModel) startUndo() tea.Cmd {
	if m.undo.id == "" || m.undo.restoring || m.commitCalls.restore == nil {
		return nil
	}
	m.undo.restoring = true
	m.showToast("info", "Restoring commit", "Waiting for the server to confirm.")
	session, id, restore, life := m.session, m.undo.id, m.commitCalls.restore, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		commit, err := restore(ctx, id)
		return restoreResult{session: session, id: id, commit: commit, err: err}
	})
}

func (m *AppModel) acceptRestore(msg restoreResult) tea.Cmd {
	if msg.session != m.session || msg.id != m.undo.id {
		return nil
	}
	u := m.undo
	m.undo = undoState{}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		detail := safeText(msg.err.Error())
		var failure *auth.APIError
		if errors.As(msg.err, &failure) && len(failure.FieldErrors) > 0 {
			detail = safeText(strings.Join(failure.FieldErrors, " "))
		}
		return m.notify("error", "Unable to restore this commit", detail)
	}
	commits := m.safeCommitsFor([]api.Commit{msg.commit}, true)
	if len(commits) == 0 {
		return m.notify("success", "Commit restored", "")
	}
	m.putBack(u, commits[0])
	return m.notify("success", "Commit restored", "It is back where it was.")
}

// putBack replaces tombstones with the restored commit and reinserts it where the delete removed it.
func (m *AppModel) putBack(u undoState, restored api.Commit) {
	delete(m.deletedNotificationCommits, restored.ID)
	m.eachCommit(func(c *api.Commit) {
		if c != nil && c.ID == restored.ID {
			replies := c.Replies
			*c = restored
			c.Replies = max(c.Replies, replies)
		}
	})
	for _, spot := range u.feeds {
		f := &m.feeds[spot.feed]
		if slices.ContainsFunc(f.commits, func(c api.Commit) bool { return c.ID == restored.ID }) {
			continue
		}
		at := min(spot.index, len(f.commits))
		f.commits = slices.Insert(f.commits, at, restored)
		if at <= f.selected && len(f.commits) > 1 {
			f.selected++
		}
		m.renderFeed(spot.feed)
	}
	for i := range m.profiles {
		s := &m.profiles[i]
		if s.people {
			continue
		}
		for _, spot := range u.profiles {
			list := &s.lists[spot.list]
			if spot.handle != s.handle || slices.ContainsFunc(list.commits, func(c api.Commit) bool { return c.ID == restored.ID }) {
				continue
			}
			at := min(spot.index, len(list.commits))
			list.commits = slices.Insert(list.commits, at, restored)
			// The selection counts a pinned commit above the Commits tab's list.
			shown := at
			if spot.list == 0 && s.profile.Pinned != nil {
				shown++
			}
			if shown <= list.selected && len(list.commits) > 1 {
				list.selected++
			}
		}
		if slices.Contains(u.pinned, s.handle) && s.profile.Pinned == nil && restored.Pinned != nil && *restored.Pinned {
			pinned := restored
			s.profile.Pinned = &pinned
		}
		m.renderProfile(s)
	}
	if u.starred >= 0 && m.starredOpen && !slices.ContainsFunc(m.starred.commits, func(c api.Commit) bool { return c.ID == restored.ID }) {
		delete(m.starredRemoved, restored.ID)
		m.starred.commits = slices.Insert(m.starred.commits, min(u.starred, len(m.starred.commits)), restored)
		m.renderStarred()
	}
	for i := range m.threads {
		m.renderThread(&m.threads[i])
	}
	if m.notificationsOpen {
		m.renderNotifications()
	}
	m.revealSelection()
}
