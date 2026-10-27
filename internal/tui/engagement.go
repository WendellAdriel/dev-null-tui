package tui

import (
	"context"
	"errors"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type engagementState struct {
	id, kind   string
	generation int
	cancel     context.CancelFunc
}

type engagementResult struct {
	session, generation int
	id, kind            string
	commit              api.Commit
	remaining           *int
	err                 error
}

func (m AppModel) focusedEngagementCommit() (api.Commit, bool) {
	if s := m.topProfile(); s != nil {
		if entries := s.entries(); !s.people && s.lists[s.tab].selected < len(entries) {
			return entries[s.lists[s.tab].selected], true
		}
		return api.Commit{}, false
	}
	if m.threadOnTop() {
		t := m.threads[len(m.threads)-1]
		entries := t.entries()
		if t.selected < len(entries) {
			return entries[t.selected].commit, true
		}
	}
	if m.notificationsOpen {
		return api.Commit{}, false
	}
	if m.starredOnTop() {
		if f := m.starred; f.selected < len(f.commits) {
			return f.commits[f.selected], true
		}
		return api.Commit{}, false
	}
	f := m.feeds[m.active]
	if f.selected < len(f.commits) {
		return f.commits[f.selected], true
	}
	return api.Commit{}, false
}

func (m AppModel) canStar(c api.Commit) bool {
	return c.DeletedAt == nil && c.Author != nil && c.Author.ID != "" && (!m.ownCommit(c) || (c.Starred != nil && *c.Starred))
}

func (m AppModel) canGivePastel(c api.Commit) bool {
	u := m.account.user
	return c.DeletedAt == nil && c.Author != nil && c.Author.ID != "" && !m.ownCommit(c) && u.PasteisMode && u.PasteisRemaining != nil && *u.PasteisRemaining > 0
}

func (m *AppModel) startEngagement(c api.Commit, kind string) tea.Cmd {
	if m.engagement.id != "" {
		return nil
	}
	if (kind == "star" && (!m.canStar(c) || m.setStar == nil)) || (kind == "pastel" && (!m.canGivePastel(c) || m.givePastel == nil)) {
		return nil
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	m.engagement = engagementState{id: c.ID, kind: kind, generation: m.engagement.generation + 1, cancel: cancel}
	m.notice = "Updating selected commit..."
	session, generation, star, pastel, load := m.session, m.engagement.generation, m.setStar, m.givePastel, m.loadThread
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		result := engagementResult{session: session, generation: generation, id: c.ID, kind: kind}
		if kind == "pastel" {
			receipt, err := pastel(ctx, c.ID)
			result.commit, result.remaining, result.err = receipt.Commit, receipt.Remaining, err
			return result
		}
		if c.Starred == nil {
			thread, err := load(ctx, c.ID)
			if err != nil {
				result.err = err
				return result
			}
			c = thread.Commit
			if c.Starred == nil {
				result.err = errors.New("reload this commit before toggling its star")
				return result
			}
		}
		result.commit, result.err = star(ctx, c.ID, !*c.Starred)
		return result
	})
}

func (m *AppModel) acceptEngagement(msg engagementResult) tea.Cmd {
	if !m.connected || msg.session != m.session || msg.generation != m.engagement.generation || msg.id != m.engagement.id || msg.kind != m.engagement.kind {
		return nil
	}
	if m.engagement.cancel != nil {
		m.engagement.cancel()
	}
	m.engagement.id, m.engagement.kind, m.engagement.cancel = "", "", nil
	m.notice = ""
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		kind, detail := "error", safeText(msg.err.Error())+". Counts are unchanged."
		var failure *auth.APIError
		definite := errors.As(msg.err, &failure) && failure.Status >= 400 && failure.Status < 500 && failure.Status != http.StatusRequestTimeout
		if msg.kind == "pastel" {
			if !definite {
				m.account.user.PasteisRemaining = nil
				kind, detail = "warning", "Outcome unknown. Check the web, then verify your account before awarding again."
			}
			if failure != nil && failure.Code == "pasteis_exhausted" {
				m.account.user.PasteisRemaining = new(0)
				kind, detail = "warning", "No pastéis left today. Verify your account after the daily reset."
			}
		}
		m.reflow()
		return m.notify(kind, "Action failed", detail)
	}
	if msg.kind == "pastel" {
		m.account.user.PasteisRemaining = msg.remaining
	}
	m.reconcileEngagement(msg.commit, msg.kind)
	var reload tea.Cmd
	if msg.kind == "star" && msg.commit.Starred != nil && !*msg.commit.Starred {
		reload = m.removeStarred(msg.id)
	}
	m.reflow()
	if msg.kind == "pastel" {
		return m.notify("success", "Pastél awarded", "You awarded a pastél")
	}
	if *msg.commit.Starred {
		return m.notify("success", "Star", "You starred the commit")
	}
	return tea.Batch(reload, m.notify("success", "Unstar", "You unstarred the commit"))
}

func (m *AppModel) reconcileEngagement(receipt api.Commit, kind string) {
	var update func(*api.Commit)
	update = func(c *api.Commit) {
		if c == nil || c.DeletedAt != nil {
			return
		}
		if c.ID == receipt.ID {
			if kind == "star" {
				c.Stars, c.Starred = receipt.Stars, receipt.Starred
			} else {
				c.Pasteis = receipt.Pasteis
			}
		}
		update(c.Quote)
		if c.ReplyPreview != nil {
			for i := range c.ReplyPreview.Data {
				update(&c.ReplyPreview.Data[i])
			}
		}
	}
	for i := range m.feeds {
		f := &m.feeds[i]
		for _, list := range [][]api.Commit{f.commits, f.suggestions, f.submitted} {
			for j := range list {
				update(&list[j])
			}
		}
	}
	for i := range m.threads {
		t := &m.threads[i]
		update(&t.root)
		for _, list := range [][]api.Commit{t.ancestors, t.replies} {
			for j := range list {
				update(&list[j])
			}
		}
		for _, list := range t.children {
			for j := range list {
				update(&list[j])
			}
		}
	}
	for i := range m.profiles {
		s := &m.profiles[i]
		update(s.profile.Pinned)
		for l := range s.lists {
			for j := range s.lists[l].commits {
				update(&s.lists[l].commits[j])
			}
		}
	}
	for i := range m.starred.commits {
		update(&m.starred.commits[i])
	}
	for _, f := range m.notificationFeeds {
		for _, n := range f.items {
			update(n.Commit)
		}
		for _, g := range f.groups {
			for _, n := range g.items {
				update(n.Commit)
			}
		}
	}
	update(&m.viewer.commit)
	for i := range m.viewer.replies {
		update(&m.viewer.replies[i])
	}
	update(&m.composer.target)
}
