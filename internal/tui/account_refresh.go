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

type meResult struct {
	session, version int
	user             auth.User
	err              error
}

func (m *AppModel) refreshAccount() tea.Cmd {
	if m.meCall == nil || m.accountRefresh.loading || !m.connected || m.accountOpen {
		return nil
	}
	m.accountRefresh.loading = true
	session, version, load, base := m.session, m.accountRefresh.version, m.meCall, m.account.life.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		user, err := load(ctx)
		return meResult{session: session, version: version, user: user, err: err}
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
	m.applyAccount(msg.user)
}

// refreshAside reloads the account and Live now together, as both show in the aside.
func (m *AppModel) refreshAside() tea.Cmd {
	return tea.Batch(m.refreshAccount(), m.fetchLiveNow())
}
