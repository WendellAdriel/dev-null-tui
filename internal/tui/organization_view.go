package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func (m AppModel) organizationAvatar(o api.Organization, p palette, cols, rows int) graphics.Request {
	req := m.avatarRequest(o.AvatarURL, laneColor(o.Lane, p), cols, rows)
	req.Square = true
	return req
}

// organizationHeader mirrors the web's organization page: identity, its links, followers, admins, and the claim banner.
func (m AppModel) organizationHeader(frame *profileFrame, s *profileState, width int, p palette) {
	inner := max(1, width-4)
	pad := "  "
	if !s.loaded {
		title, detail := "Reading this organization...", "Fetching it with your signed connection."
		if s.status != "" {
			title, detail = "Couldn't load this organization", s.status
		}
		frame.add("", pad+text(p.ink).Bold(true).Render(fit(title, inner)), pad+text(p.muted).Render(fit(detail, inner)), "", text(p.hairline).Render(strings.Repeat("─", width)))
		return
	}
	o, page := s.page.Organization, s.page
	lane := laneColor(o.Lane, p)
	var buttons []string
	var keys []string
	if page.SettingsURL != nil {
		buttons, keys = append(buttons, text(p.ink).Render("Settings")+" "+keyChip("S", p)), append(keys, "S")
	}
	buttons, keys = append(buttons, text(p.ink).Bold(true).Render(followLabel(page.IsFollowing))+" "+keyChip("f", p)), append(keys, "f")
	actions := strings.Join(buttons, "  ")
	target := func() {
		x := 2 + inner - lipgloss.Width(actions)
		for i, button := range buttons {
			frame.target(x, lipgloss.Width(button), keys[i])
			x += lipgloss.Width(button) + 2
		}
	}
	frame.add("")
	var avatar []string
	req := m.organizationAvatar(o, p, 8, 4)
	for row := range 4 {
		line := m.images.Row(req, row)
		if line == "" {
			avatar = nil
			break
		}
		avatar = append(avatar, line)
	}
	name := text(p.ink).Bold(true).Render(safeText(o.Name))
	if o.Name == "" {
		name = text(p.ink).Bold(true).Render(safeText(o.Handle))
	}
	if len(avatar) > 0 {
		for row, line := range avatar {
			if row == 0 {
				target()
				line = split(line, actions, inner)
			}
			frame.add(pad + line)
		}
		frame.add("", pad+fit(name, inner))
	} else {
		target()
		frame.add(pad + split(fit(name, inner-lipgloss.Width(actions)-1), actions, inner))
	}
	frame.add(pad + fit(text(lane).Render("■")+" "+text(p.muted).Render("@"+safeText(o.Handle))+"  "+chip("Organization", p.muted, p), inner))
	if o.Bio != "" {
		frame.add("")
		for _, line := range strings.Split(ansi.Wrap(remoteBody(o.Bio), inner, ""), "\n") {
			frame.add(pad + text(p.ink).Render(fit(line, inner)))
		}
	}
	var meta []string
	for _, item := range []struct{ icon, value, color string }{
		{m.icons.location, safeText(o.Location), p.muted},
		{m.icons.link, displayURL(o.Website), p.blue},
		{m.icons.github, displayURL(o.GitHubURL), p.blue},
	} {
		if item.value != "" {
			meta = append(meta, text(p.muted).Render(item.icon+" ")+text(item.color).Render(item.value))
		}
	}
	if len(meta) > 0 {
		frame.add("")
		for _, row := range spread(meta, inner, "   ") {
			frame.add(pad + row)
		}
	}
	if o.Followers != nil {
		label := strings.SplitN(plural(*o.Followers, "follower", "followers"), " ", 2)
		frame.add("", pad+text(p.ink).Bold(true).Render(label[0])+text(p.muted).Render(" "+label[1]))
	}
	if len(o.Admins) > 0 {
		admins := []string{text(p.muted).Render("Admins")}
		for _, admin := range o.Admins {
			admins = append(admins, text(p.ink).Render("@"+safeText(admin.Handle)))
		}
		frame.add("")
		for _, row := range spread(admins, inner, " ") {
			frame.add(pad + row)
		}
	}
	if page.CanClaim {
		claim := text(p.ink).Bold(true).Render("Claim") + " " + keyChip("C", p)
		frame.add("", pad+text(p.blue).Render("⚑ ")+text(p.ink).Bold(true).Render(fit("Nobody has claimed @"+safeText(o.Handle)+" yet", inner-2)))
		about := "This page is waiting for its team. A member of " + safeText(o.Name) + " on GitHub can claim it, then post as the organization and add teammates."
		for _, line := range strings.Split(ansi.Wrap(about, inner-2, ""), "\n") {
			frame.add(pad + "  " + text(p.muted).Render(line))
		}
		frame.target(4, lipgloss.Width(claim), "C")
		frame.add(pad + "  " + claim)
	}
	frame.add("", text(p.hairline).Render(strings.Repeat("─", width)))
}

func organizationEmpty(s *profileState) string {
	if s.tab == 1 {
		return "Nobody has mentioned @" + safeText(s.handle) + " yet. Mention it in a commit and it shows up here."
	}
	return "@" + safeText(s.handle) + " hasn't committed anything yet."
}

// organizationRows lists the organizations a Following list shows above its people, each selectable like a person.
func (m AppModel) organizationRows(frame *profileFrame, s *profileState, width int, p palette) {
	l := s.lists[s.tab]
	if len(l.organizations) == 0 {
		return
	}
	gutter := 4
	if m.images.Enabled {
		gutter = 7
	}
	inner := max(1, width-gutter-2)
	frame.add("  "+text(p.muted).Bold(true).Render("Organizations"), text(p.hairline).Render(strings.Repeat("─", width)))
	for i, o := range l.organizations {
		lane := laneColor(o.Lane, p)
		s.starts = append(s.starts, len(frame.lines))
		name := text(p.ink).Bold(true).Render(safeText(o.Name))
		if o.Name == "" {
			name = text(p.ink).Bold(true).Render(safeText(o.Handle))
		}
		rows := []string{name + " " + text(p.muted).Render("@"+safeText(o.Handle))}
		if o.Bio != "" {
			rows = append(rows, text(p.muted).Render(fit(strings.ReplaceAll(remoteBody(o.Bio), "\n", " "), inner)))
		}
		if i == l.selected {
			actions := []string{text(p.ink).Render("Open") + " " + keyChip("enter", p)}
			if o.IsFollowed != nil {
				actions = append(actions, text(p.ink).Render(followLabel(*o.IsFollowed))+" "+keyChip("f", p))
			}
			x := gutter
			for n, action := range actions {
				frame.actions = append(frame.actions, hitArea{uiRect{x, len(frame.lines) + len(rows), lipgloss.Width(action), 1}, []string{"enter", "f"}[n], i})
				x += lipgloss.Width(action) + 2
			}
			rows = append(rows, strings.Join(actions, "  "))
		}
		rows = append(rows, "")
		for row, body := range rows {
			rail := strings.Repeat(" ", gutter)
			if row == 0 || (row == 1 && m.images.Enabled) {
				rail = "  " + text(lane).Render("■") + " "
				if m.images.Enabled {
					rail = "  " + m.images.Row(m.organizationAvatar(o, p, 4, 2), row) + " "
					if lipgloss.Width(rail) != gutter {
						rail = "  " + text(lane).Render("■") + strings.Repeat(" ", gutter-3)
					}
				}
			}
			line := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(rail + fit(body, inner))
			if i == l.selected {
				line = withBackground(text(p.blue).Render("▌")+ansi.Cut(line, 1, width), p.wash)
			}
			frame.add(line)
		}
		s.ends = append(s.ends, len(frame.lines))
		frame.add(text(p.hairline).Render(strings.Repeat("─", width)))
	}
	if l.organizationsNext != "" {
		more := text(p.muted).Render("More organizations ") + keyChip("o", p)
		frame.target((width-lipgloss.Width(more))/2, lipgloss.Width(more), "o")
		frame.add(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(more))
	}
	frame.add("", "  "+text(p.muted).Bold(true).Render("People"), text(p.hairline).Render(strings.Repeat("─", width)))
}

func (m AppModel) organizationBindings(bind func(string, string) key.Binding) appKeys {
	s := m.profiles[len(m.profiles)-1]
	short := []key.Binding{bind("j/k", "move"), bind("tab", "commits / mentions"), bind("f", "follow / unfollow"), bind("enter", "open thread"), bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit")}
	all := append(append([]key.Binding(nil), short...), bind("1 / 2", "commits / mentions"), bind("u", "author profile"), bind("r", "reply"), bind("q", "quote"), bind("v", "view images"), bind("D", "delete own commit"), bind("y", "copy link"), bind("n", "older commits"), bind("ctrl+r", "reload organization"), bind("home / G / end", "first / last"), bind("pgup/pgdown", "move five entries"), bind("c", "compose"), bind("f2", m.account.nextTheme()+" theme"), bind("click", "select, open, act, or view image"))
	if s.page.CanClaim {
		all = append(all, bind("C", "claim this organization"))
	}
	if s.page.SettingsURL != nil {
		all = append(all, bind("S", "open its settings on the web"))
	}
	return appKeys{short, append(all, m.engagementBindings()...)}
}
