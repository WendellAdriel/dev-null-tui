package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// guildIconRequest is a guild's uploaded icon as a square image with no ring, so it fills its cells on the page and on
// a highlighted row alike, as the emoji does.
func (m AppModel) guildIconRequest(g api.GuildBadge, cols, rows int) (graphics.Request, bool) {
	if g.IconURL == "" || m.images == nil || !m.images.Enabled {
		return graphics.Request{}, false
	}
	req := guildChipRequest(g)
	req.Cols, req.Rows = cols, rows
	return req, true
}

// guildIcon is the guild's icon when images are on, or else its emoji centered in the same cells.
func (m AppModel) guildIcon(g api.GuildBadge, cols, rows int) []string {
	if req, ok := m.guildIconRequest(g, cols, rows); ok {
		var lines []string
		for row := range rows {
			line := m.images.Row(req, row)
			if line == "" {
				lines = nil
				break
			}
			lines = append(lines, line)
		}
		if len(lines) > 0 {
			return lines
		}
	}
	emoji := safeText(g.Emoji)
	if emoji == "" {
		emoji = "#"
	}
	tile := lipgloss.NewStyle().Width(cols).Align(lipgloss.Center)
	lines := make([]string, rows)
	for row := range rows {
		value := ""
		if row == (rows-1)/2 {
			value = emoji
		}
		lines[row] = tile.Render(value)
	}
	return lines
}

func badgeOf(g api.Guild) api.GuildBadge {
	return api.GuildBadge{Handle: g.Handle, Name: g.Name, Emoji: g.Emoji, IconURL: g.IconURL}
}

// guildHeader mirrors the web's board header: icon and name, the meta line, the description, the hashtag hint, the
// buttons, and the composer row or the paused notice.
func (m AppModel) guildHeader(frame *profileFrame, s *profileState, width int, p palette) {
	inner := max(1, width-4)
	pad := "  "
	if !s.loaded {
		title, detail := "Reading this guild...", "Fetching its board with your signed connection."
		if s.lists[0].status != "" {
			title, detail = "Couldn't load this guild", s.lists[0].status
		}
		frame.add("", pad+text(p.ink).Bold(true).Render(fit(title, inner)), pad+text(p.muted).Render(fit(detail, inner)), "", text(p.hairline).Render(strings.Repeat("─", width)))
		return
	}
	g := s.board.Guild
	var buttons, keys []string
	switch {
	case s.board.CanModerate:
		buttons, keys = append(buttons, actionChip("Edit", "E", p)), append(keys, "E")
	default:
		buttons, keys = append(buttons, actionChip("Report guild", "R", p)), append(keys, "R")
	}
	membership, membershipKeys := membershipButtons(g, p)
	buttons, keys = append(buttons, membership...), append(keys, membershipKeys...)
	actions := strings.Join(buttons, "  ")
	frame.add("")
	icon := m.guildIcon(badgeOf(g), 4, 2)
	if req, ok := m.guildIconRequest(badgeOf(g), 4, 2); ok {
		frame.show(req)
	}
	name := text(p.ink).Bold(true).Render(safeText(g.Name))
	x := 2 + inner - lipgloss.Width(actions)
	for i, button := range buttons {
		frame.target(x, lipgloss.Width(button), keys[i])
		x += lipgloss.Width(button) + 2
	}
	frame.add(pad + split(icon[0]+"  "+name, actions, inner))
	members := plural(g.Members, "member", "members")
	meta := text(p.muted).Render("#"+safeText(g.Handle)+" · Public guild · ") + text(p.blue).Render(members)
	if g.Paused {
		meta += text(p.warning).Render(" · Paused")
	}
	frame.target(2+lipgloss.Width(icon[1])+2+ansi.StringWidth("#"+safeText(g.Handle)+" · Public guild · "), ansi.StringWidth(members), "U")
	frame.add(pad + fit(icon[1]+"  "+meta, inner))
	if g.Description != "" {
		frame.add("")
		for _, line := range strings.Split(ansi.Wrap(remoteBody(g.Description), inner, ""), "\n") {
			frame.add(pad + text(p.ink).Render(fit(line, inner)))
		}
	}
	frame.add("", pad+text(p.muted).Render(fit("Include #"+safeText(g.Handle)+" in any commit to post here.", inner)))
	frame.add("", text(p.hairline).Render(strings.Repeat("─", width)))
	if g.Paused {
		notice := "This guild is paused until it has a moderator. Its board and members are preserved."
		for _, line := range strings.Split(ansi.Wrap(notice, inner, ""), "\n") {
			frame.add(pad + text(p.warning).Render(line))
		}
	} else {
		compose := actionChip("What do you want to share with the guild?", "c", p)
		frame.target(2, lipgloss.Width(compose), "c")
		frame.add(pad+compose, pad+text(p.muted).Render(fit("Posted to this guild, not your profile log.", inner)))
	}
	frame.add(text(p.hairline).Render(strings.Repeat("─", width)), pad+text(p.ink).Bold(true).Render("Bulletin board")+text(p.muted).Render(" · Latest commits"), text(p.hairline).Render(strings.Repeat("─", width)))
}

// guildMembersHeader is the web's "‹ Name · Members" page heading.
func (m AppModel) guildMembersHeader(frame *profileFrame, s *profileState, width int, p palette) {
	g := s.board.Guild
	icon := m.guildIcon(badgeOf(g), 2, 1)
	if req, ok := m.guildIconRequest(badgeOf(g), 2, 1); ok {
		frame.show(req)
	}
	frame.add("", "  "+fit(icon[0]+" "+text(p.ink).Bold(true).Render(safeText(g.Name))+text(p.muted).Render(" · Members"), max(1, width-4)), text(p.hairline).Render(strings.Repeat("─", width)))
}

// boardList draws a board: the pinned commit first, labeled as on the web, then the rest without "posted in".
func (m AppModel) boardList(frame *profileFrame, s *profileState, width int, p palette) {
	entries := s.boardEntries()
	shown := make([]api.Commit, len(entries))
	for i, c := range entries {
		c.Guild = nil
		shown[i] = c
	}
	pinned := s.board.Pinned != nil
	list := m.commitList(frame.lines, shown, s.lists[0].selected, width, func(i int, _ api.Commit) []string {
		if i == 0 && pinned {
			return []string{"", "     " + text(p.code).Render("⚑ Pinned to board")}
		}
		return nil
	})
	frame.lines, s.starts, s.ends, s.pictures = list.lines, list.starts, list.ends, list.pictures
	frame.actions = append(frame.actions, list.actions...)
}

func guildEmpty(s *profileState) string {
	if s.people {
		return "This guild has no members yet."
	}
	return "Nothing on this board yet. Include #" + safeText(s.handle) + " in a commit to post here."
}

// guildStatus is the board's status bar: the position in the list, then the board's keys as chips.
func (m AppModel) guildStatus(s *profileState) []statusPart {
	if s.people || !s.loaded || s.status != "" || s.lists[0].status != "" {
		return plainStatusLine(m.profileStatus())
	}
	g := s.board.Guild
	join := "Join"
	if g.Joined {
		join = "Leave"
	}
	parts := []statusPart{plainStatus(m.profileStatus()), statusChip(join, "J")}
	if !g.Joined {
		mute := "Mute"
		if g.Muted {
			mute = "Unmute"
		}
		parts = append(parts, statusChip(mute, "M"))
	}
	if !g.Paused {
		parts = append(parts, statusChip("Post", "c"))
	}
	parts = append(parts, statusChip("Members", "U"))
	if s.board.CanModerate {
		if _, unpin, ok := s.boardPin(); ok {
			pin := "Pin"
			if unpin {
				pin = "Unpin"
			}
			parts = append(parts, statusChip(pin, "b"))
		}
		parts = append(parts, statusChip("Edit", "E"))
	} else {
		parts = append(parts, statusChip("Report", "R"))
	}
	return parts
}

func (m AppModel) guildBindings(bind func(string, string) key.Binding) appKeys {
	s := m.profiles[len(m.profiles)-1]
	if s.people {
		short := []key.Binding{bind("↑/↓", "move"), bind("enter", "open profile"), bind("f", "follow / unfollow"), bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit")}
		all := append(append([]key.Binding(nil), short...), bind("n", "more members"), bind("ctrl+r", "reload members"), bind("home / G / end", "first / last"))
		return appKeys{short, append(all, m.engagementBindings()...)}
	}
	short := []key.Binding{bind("↑/↓", "move"), bind("J", "join / leave"), bind("c", "post to this guild"), bind("enter", "open thread"), bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit")}
	all := append(append([]key.Binding(nil), short...), bind("M", "mute / unmute"), bind("U", "members"), bind("u", "author profile"), bind("r", "reply"), bind("q", "quote"), bind("v", "view images"), bind("D", "delete own commit"), bind("y", "copy link"), bind(".", "more commit actions"), bind("P", "pin to your profile"), bind("!", "report a commit"), bind("n", "older commits"), bind("ctrl+r", "reload board"), bind("home / G / end", "first / last"), bind("pgup/pgdown", "move five entries"), bind("f2", "choose a theme"), bind("click", "select, open, act, or view image"))
	if s.board.CanModerate {
		all = append(all, bind("b", "pin / unpin a top-level commit"), bind("E", "edit the guild"))
	} else {
		all = append(all, bind("R", "report the guild"))
	}
	return appKeys{short, append(all, m.engagementBindings()...)}
}

// guildSidebarRows are Your guilds under the navigation: icon, name, and the missed commits, as many as fit.
func (m AppModel) guildSidebarRows(inner, room int, p palette) ([]string, []hitArea) {
	// The heading and the border take two rows, and More guilds a third when some guilds don't fit.
	shown, more := min(len(m.joinedGuilds), sidebarGuilds), len(m.joinedGuilds) > sidebarGuilds
	if m.guildSidebarHeight() > room {
		shown, more = room-3, true
	}
	if shown < 1 {
		return nil, nil
	}
	rows := []string{" " + text(p.muted).Bold(true).Render(fit("Your guilds", inner-1))}
	var areas []hitArea
	for i, g := range m.joinedGuilds[:shown] {
		right := text(p.muted).Render(fmt.Sprintf("g %d", i+1))
		if g.Missed > 0 {
			right = text(p.blue).Render(countBadge(g.Missed)) + " " + right
		}
		label := m.guildIcon(g.GuildBadge, 2, 1)[0] + " " + text(p.ink).Render(safeText(g.Name))
		line := " " + split(label, right, max(1, inner-1))
		if top := m.topProfile(); top != nil && top.guild && top.handle == g.Handle {
			line = withBackground(fit(line, inner), p.soft)
		}
		rows = append(rows, fit(line, inner))
		areas = append(areas, hitArea{uiRect{1, len(rows) - 1, inner, 1}, "guild:" + g.Handle, i})
	}
	if more {
		rows = append(rows, " "+split(text(p.muted).Render("More guilds"), text(p.muted).Render("g m"), max(1, inner-1)))
		areas = append(areas, hitArea{uiRect{1, len(rows) - 1, inner, 1}, "more-guilds", 0})
	}
	// A border parts your guilds from the counts under them.
	rows = append(rows, text(p.hairline).Render(strings.Repeat("─", inner)))
	return rows, areas
}

// guildSidebarHeight is the rows Your guilds takes when it all fits: the heading, the guilds, More guilds, the border.
func (m AppModel) guildSidebarHeight() int {
	if len(m.joinedGuilds) == 0 {
		return 0
	}
	rows := 2 + min(len(m.joinedGuilds), sidebarGuilds)
	if len(m.joinedGuilds) > sidebarGuilds {
		rows++
	}
	return rows
}

// sidebarGuilds is how many joined guilds the sidebar lists. The rest are under More guilds.
const sidebarGuilds = 5

// openJoinedGuild opens the sidebar's nth guild, for g 1 to g 5.
func (m *AppModel) openJoinedGuild(n int) tea.Cmd {
	if n < 1 || n > min(len(m.joinedGuilds), sidebarGuilds) {
		return nil
	}
	return m.openGuild(m.joinedGuilds[n-1].Handle)
}

// openGuildsPicker lists every guild you joined with its icon or emoji, for g m and the More guilds row.
func (m *AppModel) openGuildsPicker() tea.Cmd {
	if len(m.joinedGuilds) == 0 {
		return m.notify("info", "No guilds yet", "Join a guild from the directory, g u, and it shows here.")
	}
	d := &picker{kind: "guilds", title: "Your guilds", intro: "Guilds you joined. Enter opens a board."}
	for _, g := range m.joinedGuilds {
		badge := g.GuildBadge
		note := ""
		if g.Missed > 0 {
			note = plural(g.Missed, "new commit", "new commits")
		}
		d.rows = append(d.rows, pickerRow{guild: &badge, label: g.Name, note: note, value: g.Handle})
	}
	m.picker = d
	return m.syncImages()
}

// membershipButtons are a guild's Joined chip, or its Mute and Join chips, with their keys.
func membershipButtons(g api.Guild, p palette) (buttons, keys []string) {
	if g.Joined {
		return []string{actionChip("Joined", "J", p)}, []string{"J"}
	}
	mute := "Mute"
	if g.Muted {
		mute = "Unmute"
	}
	return []string{actionChip(mute, "M", p), actionChip("Join", "J", p)}, []string{"M", "J"}
}

// guildIconRequests are the uploaded guild icons the directory and the sidebar's Your guilds draw.
func (m AppModel) guildIconRequests() []graphics.Request {
	var requests []graphics.Request
	if m.directoryOnTop() {
		for _, g := range m.directory.guilds {
			if req, ok := m.guildIconRequest(badgeOf(g), 4, 2); ok {
				requests = append(requests, req)
			}
		}
	}
	if nav, _, _ := m.columns(); nav >= 12 {
		for _, g := range m.joinedGuilds[:min(len(m.joinedGuilds), sidebarGuilds)] {
			if req, ok := m.guildIconRequest(g.GuildBadge, 2, 1); ok {
				requests = append(requests, req)
			}
		}
	}
	return requests
}

// commitGuildIcons are the icons a commit's guild chips draw: the board it went to and the guilds its body names.
func (m AppModel) commitGuildIcons(c api.Commit) []graphics.Request {
	var requests []graphics.Request
	if c.Guild != nil {
		if req, ok := m.guildIconRequest(*c.Guild, 2, 1); ok {
			requests = append(requests, req)
		}
	}
	for _, handle := range slices.Sorted(maps.Keys(c.GuildMentions)) {
		if req, ok := m.guildIconRequest(c.GuildMentions[handle], 2, 1); ok {
			requests = append(requests, req)
		}
	}
	return requests
}

// hasGuildTarget is false for g 1 to g 5 and g m when there is no such joined guild, so they leave Settings open.
func (m AppModel) hasGuildTarget(pressed string) bool {
	switch pressed {
	case "m":
		return len(m.joinedGuilds) > 0
	case "1", "2", "3", "4", "5":
		return int(pressed[0]-'0') <= min(len(m.joinedGuilds), sidebarGuilds)
	}
	return true
}
