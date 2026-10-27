package tui

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

var profileTabNames = [2]string{"Commits", "Replies"}
var connectionTabs = [2]string{"Followers", "Following"}
var connectionLists = [2]api.Connections{api.Followers, api.FollowingList}

type profileList struct {
	commits         []api.Commit
	people          []api.User
	next, status    string
	loaded, loading bool
	selected        int
	generation      int
	cursors         map[string]bool
}

// profileState is a profile, or with people set its Followers and Following lists, stacked above `below` threads.
type profileState struct {
	handle       string
	people       bool
	below, tab   int
	profile      api.Profile
	loaded       bool
	status       string
	lists        [2]profileList
	generation   int
	viewport     viewport.Model
	starts, ends []int
	pictures     []pictureArea
	actions      []hitArea
	ctx          context.Context
	cancel       context.CancelFunc
}

type profileResult struct {
	session, generation int
	profile             api.Profile
	err                 error
}

type profileListResult struct {
	session, generation, tab, list int
	cursor                         string
	commits                        api.CommitPage
	people                         api.People
	err                            error
}

type followState struct {
	handle     string
	generation int
	cancel     context.CancelFunc
}

type followResult struct {
	session, generation int
	handle              string
	follow              bool
	err                 error
}

func (m AppModel) profileOnTop() bool {
	return len(m.profiles) > 0 && m.profiles[len(m.profiles)-1].below == len(m.threads)
}

func (m AppModel) threadOnTop() bool { return len(m.threads) > 0 && !m.profileOnTop() }

func (m AppModel) topProfile() *profileState {
	if !m.profileOnTop() {
		return nil
	}
	return &m.profiles[len(m.profiles)-1]
}

// profileHandle is the handle of the profile on screen, which needs no shortcut to itself.
func (m AppModel) profileHandle() string {
	if p := m.topProfile(); p != nil && !p.people {
		return p.handle
	}
	return ""
}

func (s profileState) entries() []api.Commit { return s.entriesOf(s.tab) }

func (s profileState) entriesOf(tab int) []api.Commit {
	commits := s.lists[tab].commits
	if tab == 0 && s.profile.Pinned != nil {
		return append([]api.Commit{*s.profile.Pinned}, commits...)
	}
	return commits
}

func (s profileState) count() int {
	if s.people {
		return len(s.lists[s.tab].people)
	}
	return len(s.entries())
}

func (m *AppModel) openProfile(handle string) tea.Cmd {
	if handle == "" || strings.EqualFold(handle, m.profileHandle()) || m.loadProfile == nil {
		return nil
	}
	s := m.pushProfile(profileState{handle: handle})
	return tea.Batch(m.fetchProfile(s), m.fetchProfileList(s, false))
}

func (m *AppModel) openConnections(tab int) tea.Cmd {
	top := m.topProfile()
	if top == nil || m.loadConnections == nil {
		return nil
	}
	if top.people {
		return m.switchProfileTab(tab)
	}
	s := m.pushProfile(profileState{handle: top.handle, people: true, tab: tab, profile: top.profile, loaded: top.loaded})
	return m.fetchProfileList(s, false)
}

func (m *AppModel) pushProfile(s profileState) *profileState {
	m.profileGeneration++
	s.generation, s.below, s.viewport = m.profileGeneration, len(m.threads), viewport.New()
	s.ctx, s.cancel = context.WithCancel(m.account.life.ctx)
	for i := range s.lists {
		s.lists[i].cursors = make(map[string]bool)
	}
	m.profiles = append(m.profiles, s)
	m.notice = ""
	m.resetSequence()
	m.reflow()
	return &m.profiles[len(m.profiles)-1]
}

func (m *AppModel) closeProfile() {
	m.profiles[len(m.profiles)-1].cancel()
	m.profiles = m.profiles[:len(m.profiles)-1]
	m.notice = ""
	m.resetSequence()
	m.reflow()
}

func (m *AppModel) closeAllProfiles() {
	for _, s := range m.profiles {
		s.cancel()
	}
	m.profiles = nil
}

func (m *AppModel) profileFor(session, generation int) *profileState {
	if session != m.session {
		return nil
	}
	for i := range m.profiles {
		if m.profiles[i].generation == generation {
			return &m.profiles[i]
		}
	}
	return nil
}

func (m AppModel) fetchProfile(s *profileState) tea.Cmd {
	session, generation, handle, load, base := m.session, s.generation, s.handle, m.loadProfile, s.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		profile, err := load(ctx, handle)
		return profileResult{session: session, generation: generation, profile: profile, err: err}
	})
}

func (m *AppModel) fetchProfileList(s *profileState, more bool) tea.Cmd {
	l := &s.lists[s.tab]
	if l.loading || (more && l.next == "") {
		return nil
	}
	cursor := ""
	if more {
		cursor = l.next
		l.cursors[cursor] = true
	}
	m.profileGeneration++
	l.generation, l.loading, l.status = m.profileGeneration, true, ""
	session, generation, tab, list, handle, base := m.session, s.generation, s.tab, l.generation, s.handle, s.ctx
	if s.people {
		load, connections := m.loadConnections, connectionLists[tab]
		return m.account.life.command(func() tea.Msg {
			ctx, cancel := context.WithTimeout(base, 30*time.Second)
			defer cancel()
			people, err := load(ctx, handle, connections, cursor)
			return profileListResult{session: session, generation: generation, tab: tab, list: list, cursor: cursor, people: people, err: err}
		})
	}
	load := m.loadUserCommits
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		commits, err := load(ctx, handle, tab == 1, cursor)
		return profileListResult{session: session, generation: generation, tab: tab, list: list, cursor: cursor, commits: commits, err: err}
	})
}

func (m *AppModel) acceptProfile(msg profileResult) tea.Cmd {
	s := m.profileFor(msg.session, msg.generation)
	if s == nil {
		return nil
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		s.status = profileError(msg.err)
		m.renderProfile(s)
		return nil
	}
	s.profile, s.loaded, s.status = msg.profile, true, ""
	m.renderProfile(s)
	return nil
}

func profileError(err error) string {
	var failure *auth.APIError
	if errors.As(err, &failure) {
		switch failure.Status {
		case http.StatusNotFound:
			return "This profile doesn't exist."
		case http.StatusForbidden:
			return "This profile isn't available."
		}
	}
	return "Couldn't load this profile: " + safeText(err.Error()) + ". Ctrl+R to retry."
}

func (m *AppModel) acceptProfileList(msg profileListResult) tea.Cmd {
	s := m.profileFor(msg.session, msg.generation)
	if s == nil || msg.list != s.lists[msg.tab].generation {
		return nil
	}
	l := &s.lists[msg.tab]
	l.loading = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		l.status = profileError(msg.err)
		m.renderProfile(s)
		return nil
	}
	if s.people {
		if msg.cursor == "" {
			l.people = nil
		}
		l.people = append(l.people, msg.people.Data...)
		l.next = msg.people.Next
	} else {
		if msg.cursor == "" {
			l.commits = nil
		}
		l.commits = uniqueCommits(append(l.commits, msg.commits.Data...))
		l.next = msg.commits.Next
	}
	l.loaded = true
	if l.cursors[l.next] {
		l.next, l.status = "", "Repeated pagination cursor. Ctrl+R to refresh."
	}
	m.renderProfile(s)
	return nil
}

func (m *AppModel) switchProfileTab(tab int) tea.Cmd {
	s := m.topProfile()
	if s.tab == tab {
		return nil
	}
	s.tab = tab
	m.notice = ""
	m.renderProfile(s)
	s.viewport.GotoTop()
	if s.lists[tab].loaded {
		return nil
	}
	return m.fetchProfileList(s, false)
}

func (m *AppModel) selectProfileEntry(index int) tea.Cmd {
	s := m.topProfile()
	count := s.count()
	if count == 0 {
		return nil
	}
	l := &s.lists[s.tab]
	previous := l.selected
	l.selected = max(0, min(count-1, index))
	m.notice = ""
	m.renderProfile(s)
	m.revealProfileSelection(s)
	if l.selected == 0 {
		s.viewport.GotoTop()
	}
	if l.selected > previous && l.selected >= count-2 {
		return m.fetchProfileList(s, true)
	}
	return nil
}

func (m *AppModel) reloadProfile(s *profileState) tea.Cmd {
	s.lists[s.tab] = profileList{cursors: make(map[string]bool)}
	s.status = ""
	m.renderProfile(s)
	if s.people {
		return m.fetchProfileList(s, false)
	}
	return tea.Batch(m.fetchProfile(s), m.fetchProfileList(s, false))
}

func (m AppModel) profileKey(pressed string) (AppModel, tea.Cmd, bool) {
	s := m.topProfile()
	l := &s.lists[s.tab]
	switch pressed {
	case "esc":
		m.closeProfile()
		return m, nil, true
	case "j", "down":
		return m, m.selectProfileEntry(l.selected + 1), true
	case "k", "up":
		return m, m.selectProfileEntry(l.selected - 1), true
	case "pgdown", "ctrl+d":
		return m, m.selectProfileEntry(l.selected + 5), true
	case "pgup", "ctrl+u":
		return m, m.selectProfileEntry(l.selected - 5), true
	case "home":
		return m, m.selectProfileEntry(0), true
	case "G", "end":
		return m, m.selectProfileEntry(s.count() - 1), true
	case "tab", "shift+tab":
		return m, m.switchProfileTab(1 - s.tab), true
	case "n":
		return m, m.fetchProfileList(s, true), true
	case "ctrl+r":
		return m, m.reloadProfile(s), true
	}
	if s.people {
		return m.connectionKey(s, pressed)
	}
	switch pressed {
	case "1", "2":
		return m, m.switchProfileTab(int(pressed[0] - '1')), true
	case "3", "4":
		return m, m.openConnections(int(pressed[0] - '3')), true
	case "f":
		return m, m.startFollow(s.handle, s.profile.IsFollowing), true
	}
	entries := s.entries()
	if len(entries) == 0 {
		return m, nil, false
	}
	selected := entries[min(l.selected, len(entries)-1)]
	switch pressed {
	case "enter":
		return m, m.openThread(selected), true
	case "u":
		if selected.Author != nil {
			return m, m.openProfile(selected.Author.Handle), true
		}
	case "r", "q":
		mode := map[string]string{"r": "reply", "q": "quote"}[pressed]
		return m, m.composeTo(mode, selected), true
	case "s":
		return m, m.startEngagement(selected, "star"), true
	case "p":
		return m, m.startEngagement(selected, "pastel"), true
	case "v":
		return m, m.openViewer(selected, 0), true
	case "D":
		m.confirmDelete(selected)
	case "y":
		return m, m.copyCommitLink(selected), true
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m AppModel) connectionKey(s *profileState, pressed string) (AppModel, tea.Cmd, bool) {
	switch pressed {
	case "1", "2", "3", "4":
		return m, m.switchProfileTab(int(pressed[0]-'1') % 2), true
	}
	people := s.lists[s.tab].people
	if len(people) == 0 {
		return m, nil, false
	}
	person := people[min(s.lists[s.tab].selected, len(people)-1)]
	switch pressed {
	case "enter", "u":
		return m, m.openProfile(person.Handle), true
	case "f":
		return m, m.startFollow(person.Handle, person.IsFollowed), true
	}
	return m, nil, false
}

func (m *AppModel) startFollow(handle string, following *bool) tea.Cmd {
	if m.follow.handle != "" || m.setFollow == nil || following == nil || strings.EqualFold(handle, m.account.user.Handle) {
		return nil
	}
	follow := !*following
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	m.follow = followState{handle: handle, generation: m.follow.generation + 1, cancel: cancel}
	m.notice = "Updating follow..."
	session, generation, write := m.session, m.follow.generation, m.setFollow
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		return followResult{session: session, generation: generation, handle: handle, follow: follow, err: write(ctx, handle, follow)}
	})
}

func (m *AppModel) acceptFollow(msg followResult) tea.Cmd {
	if !m.connected || msg.session != m.session || msg.generation != m.follow.generation || msg.handle != m.follow.handle {
		return nil
	}
	m.follow.cancel()
	m.follow.handle, m.follow.cancel = "", nil
	m.notice = ""
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
			return m.notify("warning", verb+" rate limited", "Too many follows. Try again in a minute.")
		case errors.As(msg.err, &failure) && failure.Status == http.StatusForbidden:
			return m.notify("error", verb+" failed", "You can't follow @"+safeText(msg.handle)+".")
		case errors.As(msg.err, &failure) && failure.Status >= 400 && failure.Status < 500 && failure.Status != http.StatusRequestTimeout:
			return m.notify("error", verb+" failed", safeText(msg.err.Error())+".")
		}
		var reloads []tea.Cmd
		for i := range m.profiles {
			s := &m.profiles[i]
			if !s.people {
				if strings.EqualFold(s.handle, msg.handle) {
					reloads = append(reloads, m.fetchProfile(s))
				}
				continue
			}
			for tab := range s.lists {
				if !slices.ContainsFunc(s.lists[tab].people, func(person api.User) bool { return strings.EqualFold(person.Handle, msg.handle) }) {
					continue
				}
				// An unloaded hidden tab fetches again when it is opened.
				s.lists[tab] = profileList{cursors: make(map[string]bool)}
				if tab == s.tab {
					reloads = append(reloads, m.fetchProfileList(s, false))
				}
			}
		}
		return tea.Batch(append(reloads, m.notify("warning", verb+" outcome unknown", "Reloading @"+safeText(msg.handle)+" to check."))...)
	}
	m.reconcileFollow(msg.handle, msg.follow)
	m.reflow()
	if msg.follow {
		return m.notify("success", "Following", "You follow @"+safeText(msg.handle))
	}
	return m.notify("success", "Unfollowed", "You no longer follow @"+safeText(msg.handle))
}

// reconcileFollow applies a confirmed follow. Screens move counts only where their state changed,
// but your own Following count always moves, even after the followed profile closed.
func (m *AppModel) reconcileFollow(handle string, follow bool) {
	delta := 1
	if !follow {
		delta = -1
	}
	own := m.account.user.Handle
	for i := range m.profiles {
		s := &m.profiles[i]
		if strings.EqualFold(s.handle, handle) && s.profile.IsFollowing != nil && *s.profile.IsFollowing != follow {
			s.profile.IsFollowing = &follow
			if s.profile.User.Followers != nil {
				s.profile.User.Followers = new(max(0, *s.profile.User.Followers+delta))
			}
		}
		for l := range s.lists {
			for j := range s.lists[l].people {
				person := &s.lists[l].people[j]
				if strings.EqualFold(person.Handle, handle) && person.IsFollowed != nil && *person.IsFollowed != follow {
					person.IsFollowed = &follow
				}
			}
		}
	}
	if f := m.account.user.Following; f != nil {
		m.account.user.Following = new(max(0, *f+delta))
	}
	for i := range m.profiles {
		if s := &m.profiles[i]; strings.EqualFold(s.handle, own) && s.profile.User.Following != nil {
			s.profile.User.Following = new(max(0, *s.profile.User.Following+delta))
		}
	}
}
