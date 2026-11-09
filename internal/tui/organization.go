package tui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

var organizationTabNames = [2]string{"Commits", "Mentions"}

// organizationCalls are the organization routes, which tests replace.
type organizationCalls struct {
	load     func(context.Context, string) (api.OrganizationPage, error)
	commits  func(context.Context, string, bool, string) (api.CommitPage, error)
	follow   func(context.Context, string, bool) error
	claim    func(context.Context, string) error
	followed func(context.Context, string, string) (api.Organizations, error)
}

func newOrganizationCalls(client *api.Client) organizationCalls {
	return organizationCalls{client.Organization, client.OrganizationCommits, client.SetOrganizationFollow, client.ClaimOrganization, client.FollowedOrganizations}
}

type claimResult struct {
	session int
	handle  string
	err     error
}

type followedOrganizationsResult struct {
	session, generation, list int
	cursor                    string
	page                      api.Organizations
	err                       error
}

type pageOpened struct {
	session int
	url     string
	err     error
}

func (m *AppModel) openOrganization(handle string) tea.Cmd {
	if handle == "" || strings.EqualFold(handle, m.profileHandle()) || m.organizationCalls.load == nil {
		return nil
	}
	s := m.pushProfile(profileState{handle: handle, organization: true})
	return tea.Batch(m.fetchProfile(s), m.fetchProfileList(s, false))
}

func (m AppModel) organizationKey(s *profileState, pressed string) (AppModel, tea.Cmd, bool) {
	switch pressed {
	case "1", "2":
		return m, m.switchProfileTab(int(pressed[0] - '1')), true
	case "f":
		if !s.loaded {
			return m, nil, true
		}
		return m, m.startOrganizationFollow(s.handle, s.page.IsFollowing), true
	case "C":
		return m, m.claimOrganization(s), true
	case "S":
		if s.page.SettingsURL == nil {
			return m, nil, false
		}
		return m, m.openLink(*s.page.SettingsURL), true
	}
	return m, nil, false
}

// organizationSettingsKey reports an organization page whose S opens its settings, where a click on a commit's star
// count opens Starred by instead.
func (m AppModel) organizationSettingsKey() bool {
	s := m.topProfile()
	return s != nil && s.organization && s.page.SettingsURL != nil && !m.threadOnTop()
}

func (m *AppModel) startOrganizationFollow(handle string, following bool) tea.Cmd {
	if m.follow.handle != "" || m.organizationCalls.follow == nil {
		return nil
	}
	follow := !following
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	m.follow = followState{handle: handle, generation: m.follow.generation + 1, cancel: cancel}
	m.notice = "Updating follow..."
	session, generation, write := m.session, m.follow.generation, m.organizationCalls.follow
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		return followResult{session: session, generation: generation, handle: handle, follow: follow, organization: true, err: write(ctx, handle, follow)}
	})
}

func (m *AppModel) acceptOrganizationFollow(msg followResult) tea.Cmd {
	verb := "Follow"
	if !msg.follow {
		verb = "Unfollow"
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		var failure *auth.APIError
		switch {
		case errors.As(msg.err, &failure) && failure.Status == http.StatusTooManyRequests:
			return m.notify("warning", verb+" rate limited", retryAfterText(failure.RetryAfter, "Too many follows. Try again in a minute."))
		case errors.As(msg.err, &failure) && failure.Status >= 400 && failure.Status < 500 && failure.Status != http.StatusRequestTimeout:
			return m.notify("error", verb+" failed", safeText(msg.err.Error())+".")
		}
		var reloads []tea.Cmd
		for i := range m.profiles {
			s := &m.profiles[i]
			if s.organization && strings.EqualFold(s.handle, msg.handle) {
				reloads = append(reloads, m.fetchProfile(s))
			}
			if s.people && slices.ContainsFunc(s.lists[s.tab].organizations, func(o api.Organization) bool { return strings.EqualFold(o.Handle, msg.handle) }) {
				reloads = append(reloads, m.fetchFollowedOrganizations(s, false))
			}
		}
		return tea.Batch(append(reloads, m.notify("warning", verb+" outcome unknown", "Reloading @"+safeText(msg.handle)+" to check."))...)
	}
	m.reconcileOrganizationFollow(msg.handle, msg.follow)
	m.reflow()
	if msg.follow {
		return m.notify("success", "Following", "You follow @"+safeText(msg.handle))
	}
	return m.notify("success", "Unfollowed", "You no longer follow @"+safeText(msg.handle))
}

// reconcileOrganizationFollow applies a confirmed follow. Followed organizations count toward your Following count.
func (m *AppModel) reconcileOrganizationFollow(handle string, follow bool) {
	delta := 1
	if !follow {
		delta = -1
	}
	for i := range m.profiles {
		s := &m.profiles[i]
		if s.organization && strings.EqualFold(s.handle, handle) && s.page.IsFollowing != follow {
			s.page.IsFollowing = follow
			if f := s.page.Organization.Followers; f != nil {
				s.page.Organization.Followers = new(max(0, *f+delta))
			}
		}
		for l := range s.lists {
			for j := range s.lists[l].organizations {
				if o := &s.lists[l].organizations[j]; strings.EqualFold(o.Handle, handle) && o.IsFollowed != nil {
					o.IsFollowed = &follow
				}
			}
		}
	}
	if f := m.account.user.Following; f != nil {
		m.account.user.Following = new(max(0, *f+delta))
	}
	for i := range m.profiles {
		if s := &m.profiles[i]; !s.organization && strings.EqualFold(s.handle, m.account.user.Handle) && s.profile.User.Following != nil {
			s.profile.User.Following = new(max(0, *s.profile.User.Following+delta))
		}
	}
}

func (m *AppModel) claimOrganization(s *profileState) tea.Cmd {
	if !s.page.CanClaim || m.claiming != "" || m.organizationCalls.claim == nil {
		return nil
	}
	m.claiming = s.handle
	session, handle, claim := m.session, s.handle, m.organizationCalls.claim
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	return tea.Batch(m.notify("info", "Claiming @"+safeText(handle), "Checking your membership on GitHub..."), m.account.life.command(func() tea.Msg {
		defer cancel()
		return claimResult{session: session, handle: handle, err: claim(ctx, handle)}
	}))
}

func (m *AppModel) acceptClaim(msg claimResult) tea.Cmd {
	if msg.session != m.session || !strings.EqualFold(msg.handle, m.claiming) {
		return nil
	}
	m.claiming = ""
	var reloads []tea.Cmd
	for i := range m.profiles {
		if s := &m.profiles[i]; s.organization && strings.EqualFold(s.handle, msg.handle) {
			reloads = append(reloads, m.fetchProfile(s))
		}
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		// The server says why: already claimed, or GitHub doesn't list the user as a public member.
		var failure *auth.APIError
		if errors.As(msg.err, &failure) && failure.Message != "" && (failure.Status == http.StatusForbidden || failure.Status == http.StatusConflict) {
			return tea.Batch(append(reloads, m.notify("error", "Couldn't claim @"+safeText(msg.handle), safeText(failure.Message)))...)
		}
		return tea.Batch(append(reloads, m.notify("error", "Couldn't claim @"+safeText(msg.handle), "Try again in a moment."))...)
	}
	return tea.Batch(append(reloads, m.notify("success", "Claimed @"+safeText(msg.handle), "You're its admin. Manage it from Settings on the web."))...)
}

func (m *AppModel) fetchFollowedOrganizations(s *profileState, more bool) tea.Cmd {
	l := &s.lists[s.tab]
	if !s.people || connectionLists[s.tab] != api.FollowingList || m.organizationCalls.followed == nil || l.organizationsLoading || (more && l.organizationsNext == "") {
		return nil
	}
	cursor := ""
	if more {
		cursor = l.organizationsNext
	}
	m.profileGeneration++
	l.organizationsGeneration, l.organizationsLoading = m.profileGeneration, true
	session, generation, list, handle, base, load := m.session, s.generation, l.organizationsGeneration, s.handle, s.ctx, m.organizationCalls.followed
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		page, err := load(ctx, handle, cursor)
		return followedOrganizationsResult{session: session, generation: generation, list: list, cursor: cursor, page: page, err: err}
	})
}

func (m *AppModel) acceptFollowedOrganizations(msg followedOrganizationsResult) tea.Cmd {
	s := m.profileFor(msg.session, msg.generation)
	if s == nil {
		return nil
	}
	tab := slices.Index(connectionLists[:], api.FollowingList)
	l := &s.lists[tab]
	if msg.list != l.organizationsGeneration {
		return nil
	}
	l.organizationsLoading = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		// The people still load, so a failed organizations page only stops offering more.
		l.organizationsNext = ""
		m.renderProfile(s)
		return nil
	}
	before := len(l.organizations)
	if msg.cursor == "" {
		l.organizations = nil
	}
	l.organizations = append(l.organizations, msg.page.Data...)
	// Organizations sit above the people, so a selected person keeps its row as organizations load. An untouched list stays at the top.
	if l.selected >= before && (before > 0 || l.selected > 0) {
		l.selected = max(0, l.selected+len(l.organizations)-before)
	}
	l.organizationsNext = msg.page.Next
	m.renderProfile(s)
	return nil
}

// openLink opens a server-given web page, such as an organization's settings or a stream, in the browser.
func (m *AppModel) openLink(raw string) tea.Cmd {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || m.openURL == nil {
		return m.notify("error", "Couldn't open the link", "The server sent an invalid address.")
	}
	session, open := m.session, m.openURL
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 10*time.Second)
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		return pageOpened{session: session, url: u.String(), err: open(ctx, u.String())}
	})
}

func (m *AppModel) acceptLink(msg pageOpened) tea.Cmd {
	if msg.session != m.session || msg.err == nil {
		return nil
	}
	// OpenBrowser's own error speaks of the sign-in link, so name the page instead.
	return m.notify("error", "Couldn't open the browser", "Open "+safeText(msg.url)+" yourself.")
}
