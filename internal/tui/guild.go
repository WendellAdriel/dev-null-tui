package tui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// guildCalls are the guild routes, which tests replace.
type guildCalls struct {
	directory  func(context.Context, string, int) (api.Guilds, error)
	board      func(context.Context, string, int) (api.GuildBoard, error)
	members    func(context.Context, string, int) (api.GuildMembers, error)
	joined     func(context.Context) ([]api.JoinedGuild, error)
	membership func(context.Context, string, bool) error
	mute       func(context.Context, string, bool) error
	read       func(context.Context, string) error
	report     func(context.Context, string, string) error
	pin        func(context.Context, string, string, bool) error
	create     func(context.Context, api.GuildForm) (api.GuildSaved, error)
	update     func(context.Context, string, api.GuildForm) (api.GuildSaved, error)
}

func newGuildCalls(client *api.Client) guildCalls {
	return guildCalls{
		directory: client.Guilds, board: client.GuildBoard, members: client.GuildMembers, joined: client.JoinedGuilds,
		membership: client.SetGuildMembership, mute: client.SetGuildMute, read: client.ReadGuild, report: client.ReportGuild,
		pin: client.SetGuildPin, create: client.CreateGuild, update: client.UpdateGuild,
	}
}

// guildAction is a join, leave, mute, unmute, pin or unpin from a board. Only one runs at a time.
type guildAction struct {
	kind, handle, commit string
	on                   bool
}

type guildActionResult struct {
	session int
	action  guildAction
	err     error
}

type joinedGuildsResult struct {
	session int
	guilds  []api.JoinedGuild
	err     error
}

// openGuild opens a guild's board and marks it read, as opening it on the web does.
func (m *AppModel) openGuild(handle string) tea.Cmd {
	if !api.ValidGuildHandle(handle) || m.guildCalls.board == nil {
		return nil
	}
	if top := m.topProfile(); top != nil && top.guild && !top.people && top.handle == handle {
		return nil
	}
	s := m.pushProfile(profileState{handle: handle, guild: true})
	return tea.Batch(m.fetchProfileList(s, false), m.markGuildRead(handle))
}

// openGuildMembers opens the members of the board on top.
func (m *AppModel) openGuildMembers() tea.Cmd {
	top := m.topProfile()
	if top == nil || !top.guild || top.people || !top.loaded {
		return nil
	}
	s := m.pushProfile(profileState{handle: top.handle, guild: true, people: true, board: top.board, loaded: true})
	return m.fetchProfileList(s, false)
}

func (m *AppModel) markGuildRead(handle string) tea.Cmd {
	for i := range m.joinedGuilds {
		if m.joinedGuilds[i].Handle == handle {
			m.joinedGuilds[i].Missed = 0
		}
	}
	read := m.guildCalls.read
	if read == nil {
		return nil
	}
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
		defer cancel()
		// A failed read only leaves the count for the next refresh to correct.
		_ = read(ctx, handle)
		return nil
	})
}

// fetchGuildPage loads a board or members page. Pages are numbers, kept in the list's cursor as text.
func (m *AppModel) fetchGuildPage(s *profileState, l *profileList, cursor string) tea.Cmd {
	page, _ := strconv.Atoi(cursor)
	session, generation, list, handle, base := m.session, s.generation, l.generation, s.handle, s.ctx
	if s.people {
		load := m.guildCalls.members
		return m.account.life.command(func() tea.Msg {
			ctx, cancel := context.WithTimeout(base, 30*time.Second)
			defer cancel()
			members, err := load(ctx, handle, max(1, page))
			return profileListResult{session: session, generation: generation, list: list, cursor: cursor, members: &members, err: err}
		})
	}
	load := m.guildCalls.board
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		board, err := load(ctx, handle, max(1, page))
		return profileListResult{session: session, generation: generation, list: list, cursor: cursor, board: &board, err: err}
	})
}

// acceptGuildPage stores a board or members page: the board's guild, pin and moderation come with every page.
func acceptGuildPage(s *profileState, l *profileList, msg profileListResult) {
	next := func(page, last int) string {
		if page < last {
			return strconv.Itoa(page + 1)
		}
		return ""
	}
	if msg.members != nil {
		l.people = append(l.people, msg.members.Data...)
		s.moderatorID, l.next = msg.members.ModeratorID, next(msg.members.Page, msg.members.LastPage)
		return
	}
	board := msg.board
	if msg.cursor == "" {
		s.board, s.loaded = *board, true
	}
	s.board.Guild, s.board.Pinned, s.board.CanModerate = board.Guild, board.Pinned, board.CanModerate
	l.commits = uniqueCommits(append(l.commits, board.Data...))
	l.next = next(board.Page, board.LastPage)
}

// boardEntries puts the pinned commit first, as the web does, without repeating it in the board below.
// boardPin is the selected commit and whether b unpins it. ok is false with nothing selected or on a reply, since the
// server only pins top-level commits.
func (s profileState) boardPin() (commit api.Commit, unpin, ok bool) {
	entries := s.boardEntries()
	if len(entries) == 0 {
		return api.Commit{}, false, false
	}
	commit = entries[min(s.lists[0].selected, len(entries)-1)]
	if commit.ParentID != nil {
		return commit, false, false
	}
	return commit, s.board.Pinned != nil && s.board.Pinned.ID == commit.ID, true
}

func (s profileState) boardEntries() []api.Commit {
	commits := s.lists[0].commits
	if s.board.Pinned == nil {
		return commits
	}
	entries := []api.Commit{*s.board.Pinned}
	for _, c := range commits {
		if c.ID != s.board.Pinned.ID {
			entries = append(entries, c)
		}
	}
	return entries
}

func (m AppModel) guildKey(s *profileState, pressed string) (AppModel, tea.Cmd, bool) {
	if s.people {
		if s.count() == 0 {
			return m, nil, false
		}
		person := s.lists[0].people[min(s.lists[0].selected, s.count()-1)]
		switch pressed {
		case "enter", "u":
			return m, m.openProfile(person.Handle), true
		case "f":
			return m, m.startFollow(person.Handle, person.IsFollowed), true
		}
		return m, nil, false
	}
	if !s.loaded {
		return m, nil, false
	}
	g := s.board.Guild
	switch pressed {
	case "J":
		return m, m.startGuildAction(guildAction{kind: "membership", handle: g.Handle, on: !g.Joined}), true
	case "M":
		if g.Joined {
			return m, nil, true
		}
		return m, m.startGuildAction(guildAction{kind: "mute", handle: g.Handle, on: !g.Muted}), true
	case "U":
		return m, m.openGuildMembers(), true
	case "c":
		if g.Paused {
			return m, m.notify("info", "Guild paused", "This guild is paused until it has a moderator."), true
		}
		return m, m.composeOnBoard(g), true
	case "R":
		if s.board.CanModerate {
			return m, nil, true
		}
		return m, m.beginGuildReport(g), true
	case "E":
		if !s.board.CanModerate {
			return m, nil, true
		}
		return m, m.openGuildForm(&g), true
	case "b":
		selected, unpin, ok := s.boardPin()
		if !s.board.CanModerate || !ok {
			return m, nil, true
		}
		return m, m.startGuildAction(guildAction{kind: "pin", handle: g.Handle, commit: selected.ID, on: !unpin}), true
	}
	return m, nil, false
}

func (m *AppModel) startGuildAction(action guildAction) tea.Cmd {
	if m.guildBusy || m.guildCalls.membership == nil {
		return nil
	}
	m.guildBusy = true
	session, calls := m.session, m.guildCalls
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
		defer cancel()
		var err error
		switch action.kind {
		case "membership":
			err = calls.membership(ctx, action.handle, action.on)
		case "mute":
			err = calls.mute(ctx, action.handle, action.on)
		case "pin":
			err = calls.pin(ctx, action.handle, action.commit, action.on)
		}
		return guildActionResult{session: session, action: action, err: err}
	})
}

func (m *AppModel) acceptGuildAction(msg guildActionResult) tea.Cmd {
	if msg.session != m.session {
		return nil
	}
	m.guildBusy = false
	a := msg.action
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		return m.notify("error", "Couldn't update the guild", m.ghostly(msg.err, safeText(msg.err.Error())))
	}
	var reloads []tea.Cmd
	for i := range m.profiles {
		s := &m.profiles[i]
		if !s.guild || s.handle != a.handle {
			continue
		}
		applyGuildAction(&s.board.Guild, a)
		if a.kind == "pin" && !s.people {
			// A pin reorders the board, so it loads again from the first page, as Ctrl+R does.
			s.lists[0] = profileList{cursors: make(map[string]bool)}
			reloads = append(reloads, m.fetchProfileList(s, false))
		}
		m.renderProfile(s)
	}
	m.directoryUpdated(a)
	title := map[string][2]string{"membership": {"Left", "Joined"}, "mute": {"Unmuted", "Muted"}, "pin": {"Unpinned", "Pinned to board"}}[a.kind][map[bool]int{false: 0, true: 1}[a.on]]
	return tea.Batch(append(reloads, m.refreshJoinedGuilds(), m.notify("success", title, "#"+a.handle))...)
}

// refreshJoinedGuilds reloads the navigation's Your guilds, with the sidebar counts and after any membership change.
func (m *AppModel) refreshJoinedGuilds() tea.Cmd {
	load := m.guildCalls.joined
	if load == nil {
		return nil
	}
	session := m.session
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
		defer cancel()
		guilds, err := load(ctx)
		return joinedGuildsResult{session: session, guilds: guilds, err: err}
	})
}

func (m *AppModel) acceptJoinedGuilds(msg joinedGuildsResult) {
	// The guilds feature answers 404 while it's off, which leaves Your guilds empty rather than showing an error.
	if msg.session != m.session || msg.err != nil {
		return
	}
	m.joinedGuilds = msg.guilds
	if top := m.topProfile(); top != nil && top.guild {
		for i := range m.joinedGuilds {
			if m.joinedGuilds[i].Handle == top.handle {
				m.joinedGuilds[i].Missed = 0
			}
		}
	}
}

// composeOnBoard opens the composer for a top-level commit on the guild's board.
func (m *AppModel) composeOnBoard(g api.Guild) tea.Cmd {
	return m.composeAt("", api.Commit{}, &api.GuildBadge{Handle: g.Handle, Name: g.Name, Emoji: g.Emoji, IconURL: g.IconURL})
}

func guildLabel(g api.GuildBadge) string {
	return strings.TrimSpace(safeText(g.Emoji) + " " + safeText(g.Name))
}

// applyGuildAction updates a guild after a join, leave or mute.
func applyGuildAction(g *api.Guild, a guildAction) {
	switch a.kind {
	case "membership":
		g.Joined, g.Muted = a.on, false
		g.Members = max(0, g.Members+map[bool]int{true: 1, false: -1}[a.on])
	case "mute":
		g.Muted = a.on
	}
}

// addToBoard puts a commit you just posted at the top of its open board and selects it, as the Log does. A load in
// flight is dropped, since a first page arriving after the post would replace the list without it.
func (m *AppModel) addToBoard(handle string, commit api.Commit) {
	for i := range m.profiles {
		s := &m.profiles[i]
		if !s.guild || s.people || s.handle != handle {
			continue
		}
		l := &s.lists[0]
		if l.loading {
			l.generation++
			l.loading = false
		}
		l.loaded = true
		l.commits = uniqueCommits(append([]api.Commit{commit}, l.commits...))
		l.selected = 0
		m.renderProfile(s)
		s.viewport.GotoTop()
	}
}
