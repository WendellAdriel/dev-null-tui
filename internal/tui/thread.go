package tui

import (
	"context"
	"errors"
	"net/http"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type threadEntry struct {
	commit api.Commit
	depth  int
	kind   string
}

type threadState struct {
	root               api.Commit
	ancestors, replies []api.Commit
	children           map[string][]api.Commit
	next, status       string
	hidden, loading    bool
	selected           int
	selectedID         string
	generation         int
	viewport           viewport.Model
	starts, ends       []int
	widths             []int
	pictures           []pictureArea
	actions            []hitArea
	ctx                context.Context
	cancel             context.CancelFunc
}

type deleteConfirm struct {
	id    string
	focus int
}

type threadResult struct {
	session, generation int
	thread              api.Thread
	err                 error
}

type repliesResult struct {
	session, generation int
	viewer              bool
	parent, cursor      string
	replies             api.Replies
	err                 error
}

type deleteResult struct {
	session int
	id      string
	err     error
}

func (m *AppModel) openThread(target api.Commit) tea.Cmd {
	if target.ID == "" || target.DeletedAt != nil || target.Author == nil {
		return nil
	}
	m.threadGeneration++
	ctx, cancel := context.WithCancel(m.account.life.ctx)
	m.threads = append(m.threads, threadState{root: target, selectedID: target.ID, children: map[string][]api.Commit{}, loading: true, generation: m.threadGeneration, viewport: viewport.New(), ctx: ctx, cancel: cancel})
	m.notice = ""
	m.resetSequence()
	m.reflow()
	t := &m.threads[len(m.threads)-1]
	return tea.Batch(m.fetchThread(t), m.fetchReplies(t, target.ID, ""))
}

func (m *AppModel) closeThread() {
	t := m.threads[len(m.threads)-1]
	t.cancel()
	m.threads = m.threads[:len(m.threads)-1]
	m.notice = ""
	m.resetSequence()
	m.reflow()
}

func (m *AppModel) closeAllThreads() {
	for _, t := range m.threads {
		t.cancel()
	}
	m.threads = nil
}

func (m AppModel) fetchThread(t *threadState) tea.Cmd {
	session, generation, id, load, parent := m.session, t.generation, t.root.ID, m.loadThread, t.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		thread, err := load(ctx, id)
		return threadResult{session: session, generation: generation, thread: thread, err: err}
	})
}

func (m AppModel) fetchReplies(t *threadState, parent, cursor string) tea.Cmd {
	if parent == t.root.ID {
		t.loading = true
	}
	session, generation, load, base := m.session, t.generation, m.loadReplies, t.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		replies, err := load(ctx, parent, cursor)
		return repliesResult{session: session, generation: generation, parent: parent, cursor: cursor, replies: replies, err: err}
	})
}

func (m *AppModel) threadFor(session, generation int) *threadState {
	if session != m.session {
		return nil
	}
	for i := range m.threads {
		if m.threads[i].generation == generation {
			return &m.threads[i]
		}
	}
	return nil
}

func (m *AppModel) acceptThread(msg threadResult) tea.Cmd {
	t := m.threadFor(msg.session, msg.generation)
	if t == nil {
		return nil
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		t.status = "Couldn't load this commit: " + safeText(msg.err.Error()) + ". Ctrl+R to retry."
		m.renderThread(t)
		return nil
	}
	t.root, t.ancestors = msg.thread.Commit, msg.thread.Ancestors
	m.renderThread(t)
	m.revealThreadSelection(t)
	return nil
}

func (m *AppModel) acceptReplies(msg repliesResult) tea.Cmd {
	if msg.viewer {
		return m.acceptViewerReplies(msg)
	}
	t := m.threadFor(msg.session, msg.generation)
	if t == nil {
		return nil
	}
	if msg.parent == t.root.ID {
		t.loading = false
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		t.status = "Couldn't load replies: " + safeText(msg.err.Error()) + ". Ctrl+R to retry."
		m.renderThread(t)
		return nil
	}
	t.status = ""
	if msg.parent != t.root.ID {
		t.children[msg.parent] = uniqueCommits(msg.replies.Data)
		m.renderThread(t)
		return nil
	}
	if msg.cursor == "" {
		t.replies = uniqueCommits(msg.replies.Data)
	} else {
		t.replies = uniqueCommits(append(t.replies, msg.replies.Data...))
	}
	t.next = msg.replies.Next
	m.renderThread(t)
	return nil
}

func (m *AppModel) revoked(err error) bool {
	var failure *auth.APIError
	if !errors.As(err, &failure) || failure.Status != http.StatusUnauthorized {
		return false
	}
	m.leaveApplication()
	m.account.state, m.account.status = "failed", "Device approval is no longer valid. Connect again."
	m.account.approvalRequired = true
	return true
}

func (m AppModel) threadEntries() []threadEntry {
	if len(m.threads) == 0 {
		return nil
	}
	return m.threads[len(m.threads)-1].entries()
}

func (t threadState) entries() []threadEntry {
	var entries []threadEntry
	for _, c := range t.ancestors {
		entries = append(entries, threadEntry{c, 0, "ancestor"})
	}
	entries = append(entries, threadEntry{t.root, 0, "focus"})
	if t.hidden {
		return entries
	}
	for _, c := range t.replies {
		kind := "reply"
		if c.QuotedID != nil && *c.QuotedID == t.root.ID && (c.ParentID == nil || *c.ParentID != t.root.ID) {
			kind = "quote"
		}
		entries = append(entries, threadEntry{c, 1, kind})
		for _, child := range t.children[c.ID] {
			entries = append(entries, threadEntry{child, 2, "reply"})
		}
	}
	return entries
}

func (t *threadState) restoreSelection(entries []threadEntry) {
	for i, entry := range entries {
		if entry.commit.ID == t.selectedID {
			t.selected = i
			return
		}
	}
	t.selected = max(0, min(len(entries)-1, t.selected))
	t.selectedID = entries[t.selected].commit.ID
}

func (m *AppModel) selectEntry(t *threadState, index int) {
	entries := t.entries()
	t.selected = max(0, min(len(entries)-1, index))
	t.selectedID = entries[t.selected].commit.ID
	t.status, m.notice = "", ""
	m.renderThread(t)
	m.revealThreadSelection(t)
}

func (m AppModel) ownCommit(c api.Commit) bool {
	return c.DeletedAt == nil && c.Author != nil && c.Author.ID != "" && c.Author.ID == m.account.user.ID
}

func (m AppModel) threadKey(pressed string) (AppModel, tea.Cmd, bool) {
	t := &m.threads[len(m.threads)-1]
	entries := t.entries()
	t.restoreSelection(entries)
	entry := entries[t.selected]
	move := func(delta int) { m.selectEntry(t, t.selected+delta) }
	switch pressed {
	case "s":
		return m, m.startEngagement(entry.commit, "star"), true
	case "p":
		return m, m.startEngagement(entry.commit, "pastel"), true
	case "esc":
		m.closeThread()
	case "j", "down":
		move(1)
	case "k", "up":
		move(-1)
	case "pgdown", "ctrl+d":
		move(5)
	case "pgup", "ctrl+u":
		move(-5)
	case "home":
		move(-len(entries))
	case "G", "end":
		move(len(entries))
	case "enter":
		if entry.kind == "focus" {
			return m, nil, true
		}
		return m, m.openThread(entry.commit), true
	case "space":
		return m, m.toggleReplies(t, entry), true
	case "n":
		if t.next == "" || t.loading {
			return m, nil, true
		}
		return m, m.fetchReplies(t, t.root.ID, t.next), true
	case "r", "q":
		mode := map[string]string{"r": "reply", "q": "quote"}[pressed]
		return m, m.composeTo(mode, entry.commit), true
	case "D":
		m.confirmDelete(entry.commit)
	case "v":
		return m, m.openViewer(entry.commit, 0), true
	case "y":
		return m, m.copyCommitLink(entry.commit), true
	case "ctrl+r":
		m.threadGeneration++
		t.generation = m.threadGeneration
		t.status, t.next, t.children = "", "", map[string][]api.Commit{}
		m.renderThread(t)
		return m, tea.Batch(m.fetchThread(t), m.fetchReplies(t, t.root.ID, "")), true
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m *AppModel) toggleReplies(t *threadState, entry threadEntry) tea.Cmd {
	switch {
	case entry.kind == "focus":
		t.hidden = !t.hidden
	case entry.depth == 2:
		t.status = "Press Enter to open this reply's thread."
	case entry.depth == 1 && entry.commit.Replies > 0:
		if _, shown := t.children[entry.commit.ID]; shown {
			delete(t.children, entry.commit.ID)
			break
		}
		m.renderThread(t)
		return m.fetchReplies(t, entry.commit.ID, "")
	}
	m.renderThread(t)
	return nil
}

func (m *AppModel) addToThreads(mode, targetID string, commit api.Commit) {
	if mode == "" {
		return
	}
	for i := range m.threads {
		t := &m.threads[i]
		if t.root.ID == targetID {
			// With older pages unloaded, the reply arrives in its place with the last page.
			if t.next == "" {
				t.replies = uniqueCommits(append(t.replies, commit))
			}
			if mode == "reply" {
				t.root.Replies++
			}
		}
		for j := range t.replies {
			if t.replies[j].ID != targetID || mode != "reply" {
				continue
			}
			t.replies[j].Replies++
			if children, shown := t.children[targetID]; shown {
				t.children[targetID] = uniqueCommits(append(children, commit))
			}
		}
		m.renderThread(t)
	}
	if mode != "reply" {
		return
	}
	for i := range m.feeds {
		for j := range m.feeds[i].commits {
			if m.feeds[i].commits[j].ID == targetID {
				m.feeds[i].commits[j].Replies++
			}
		}
		m.renderFeed(i)
	}
}

func (m *AppModel) deleteKey(pressed string) tea.Cmd {
	switch pressed {
	case "tab", "shift+tab", "left", "right":
		m.deleting.focus = 1 - m.deleting.focus
	case "esc":
		m.deleting = deleteConfirm{}
	case "enter":
		if m.deleting.focus == 1 {
			return m.startDelete()
		}
		m.deleting = deleteConfirm{}
	case "ctrl+d":
		return m.startDelete()
	}
	return nil
}

func (m *AppModel) confirmDelete(commit api.Commit) {
	if m.ownCommit(commit) && m.deletingID == "" {
		m.deleting = deleteConfirm{id: commit.ID}
	}
}

func (m *AppModel) startDelete() tea.Cmd {
	id, session, remove, life := m.deleting.id, m.session, m.deleteCommit, m.account.life
	m.deleting, m.deletingID = deleteConfirm{}, id
	m.showToast("info", "Deleting commit", "Waiting for the server to confirm.")
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		return deleteResult{session: session, id: id, err: remove(ctx, id)}
	})
}

func (m *AppModel) acceptDelete(msg deleteResult) tea.Cmd {
	if msg.session != m.session {
		return nil
	}
	m.deletingID = ""
	var failure *auth.APIError
	missing := errors.As(msg.err, &failure) && failure.Status == http.StatusNotFound
	if msg.err != nil && !missing {
		if m.revoked(msg.err) {
			return nil
		}
		return m.notify("error", "Delete failed", safeText(msg.err.Error())+". The commit is unchanged.")
	}
	tombstone := func(c *api.Commit) {
		if c.ID == msg.id {
			deleted := m.now
			*c = api.Commit{ID: c.ID, ParentID: c.ParentID, DeletedAt: &deleted, Replies: c.Replies}
		}
	}
	for i := range m.threads {
		t := &m.threads[i]
		tombstone(&t.root)
		for j := range t.ancestors {
			tombstone(&t.ancestors[j])
		}
		for j := range t.replies {
			tombstone(&t.replies[j])
		}
		for _, children := range t.children {
			for j := range children {
				tombstone(&children[j])
			}
		}
		m.renderThread(t)
	}
	for i := range m.feeds {
		f := &m.feeds[i]
		kept := f.commits[:0]
		for j, c := range f.commits {
			if c.ID != msg.id {
				kept = append(kept, c)
			} else if j < f.selected {
				f.selected--
			}
		}
		f.commits = kept
		f.selected = max(0, min(f.selected, len(f.commits)-1))
		m.renderFeed(i)
	}
	m.revealSelection()
	m.deleteNotificationCommit(msg.id)
	if m.viewer.commit.ID == msg.id {
		m.closeViewer()
	}
	return m.notify("success", "Commit deleted", "It is gone from your logs. Deletion cannot be undone.")
}
