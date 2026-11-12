package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// accountRefresh keeps the aside's counts and your live state current, since the login's GET /me is otherwise the
// only read. version grows with every account change, so a refresh started before one is dropped.
type accountRefresh struct {
	loading bool
	version int
}

// counts is the notification count generation when the request started. An inbox read since then cleared the
// badge, so the count this result carries is older than it.
type meResult struct {
	session, version, counts int
	user                     auth.User
	err                      error
}

func (m *AppModel) refreshAccount() tea.Cmd {
	if m.meCall == nil || m.accountRefresh.loading || !m.connected || m.accountOpen {
		return nil
	}
	m.accountRefresh.loading = true
	session, version, counts, load, base := m.session, m.accountRefresh.version, m.notificationCountGeneration, m.meCall, m.account.life.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		user, err := load(ctx)
		return meResult{session: session, version: version, counts: counts, user: user, err: err}
	})
}

func (m *AppModel) acceptAccountRefresh(msg meResult) {
	if msg.session != m.session {
		return
	}
	m.accountRefresh.loading = false
	if msg.err != nil || msg.version != m.accountRefresh.version || msg.user.ID != m.account.user.ID {
		m.revoked(msg.err)
		return
	}
	if msg.counts != m.notificationCountGeneration {
		msg.user.UnreadNotifications = nil
	}
	m.applyAccount(msg.user)
}

// refreshAside reloads what the side columns show while you can see it: your counts and live state in the left
// sidebar, and Live now in the right aside.
func (m *AppModel) refreshAside() tea.Cmd {
	if !m.focused || m.accountOpen {
		return nil
	}
	nav, _, aside := m.columns()
	var account, live tea.Cmd
	if nav >= sidebarWidth {
		account = m.refreshAccount()
	}
	var topics tea.Cmd
	if aside > 0 {
		live = m.fetchLiveNow()
	}
	if (aside > 0 && !m.remotesWide()) || m.topicsOnTop() {
		topics = m.fetchHeadlines()
	}
	return tea.Batch(account, live, topics)
}
