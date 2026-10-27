package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func (m AppModel) notificationHeader(width int, p palette) (string, []hitArea) {
	lines := []string{"", "  " + text(p.ink).Bold(true).Render("Notifications"), ""}
	row := "  "
	var areas []hitArea
	for i, label := range notificationLabels {
		label = fmt.Sprintf("%d %s", i+1, label)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(p.muted)).Background(lipgloss.Color(p.wash))
		if m.notificationMask&(1<<i) != 0 {
			style = style.Foreground(lipgloss.Color(p.blue)).Background(lipgloss.Color(p.soft))
		}
		chip := style.Render(" " + label + " ")
		if ansi.StringWidth(row)+ansi.StringWidth(chip) > width-1 && ansi.StringWidth(row) > 2 {
			lines = append(lines, row)
			row = "  "
		}
		areas = append(areas, hitArea{uiRect{ansi.StringWidth(row), len(lines), ansi.StringWidth(chip), 1}, fmt.Sprint(i + 1), 0})
		row += chip + " "
	}
	lines = append(lines, row, "", text(p.hairline).Render(strings.Repeat("─", width)))
	return strings.Join(lines, "\n"), areas
}

func (m *AppModel) sizeNotifications() {
	f := m.notificationFeed()
	_, width, _ := m.columns()
	offset := f.viewport.YOffset()
	if f.selected < len(f.starts) {
		offset -= f.starts[f.selected]
	}
	header, _ := m.notificationHeader(width, colors(m.account.dark))
	f.viewport.SetWidth(width)
	f.viewport.SetHeight(max(1, m.mainHeight()-lipgloss.Height(header)))
	m.renderNotifications()
	if f.selected < len(f.starts) {
		f.viewport.SetYOffset(max(0, f.starts[f.selected]+offset))
	}
}

func (m AppModel) notificationColumn(width int, p palette) string {
	header, _ := m.notificationHeader(width, p)
	f := m.notificationFeed()
	body := f.viewport.View()
	if !f.loaded {
		message := "Reading notifications..."
		if f.status != "" {
			message = "Couldn't load notifications\n\n" + f.status
		}
		body = lipgloss.NewStyle().Width(max(1, width-8)).MarginLeft(6).Render("\n" + text(p.muted).Render(message))
	}
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(header + "\n" + body)
}

func notificationVerb(n api.Notification) (string, string) {
	switch n.Type {
	case "followed":
		return "+", "followed you"
	case "starred":
		return "★", "starred your commit"
	case "replied":
		return "↩", "Replied to your commit"
	case "quoted":
		return "❝", "Quoted your commit"
	case "mentioned":
		return "@", "Mentioned you"
	case "approved":
		return "✓", "Your commit was approved"
	case "rejected":
		return "×", "Your commit was rejected"
	}
	return "●", "Notification"
}

func notificationNoun(n api.Notification) string {
	if n.Type == "starred" {
		return "stars"
	}
	if n.Type == "followed" {
		return "followers"
	}
	return "updates"
}

func notificationExcerpt(c *api.Commit, width int, p palette, boxed bool) []string {
	if c == nil {
		return nil
	}
	if c.DeletedAt != nil {
		return []string{text(p.muted).Render("Commit deleted")}
	}
	if c.Author == nil {
		return []string{text(p.muted).Render("Commit unavailable")}
	}
	inner := width
	if boxed {
		inner = max(1, width-4)
	}
	rows := strings.Split(ansi.Wrap(bodyText(*c, p), inner, ""), "\n")
	if len(rows) > 4 {
		rows = append(rows[:3], "…")
	}
	if len(c.Images) > 0 && bodyText(*c, p) == "" {
		rows = []string{"▧ Image"}
	}
	if !boxed {
		return rows
	}
	result := []string{text(p.hairline).Render("╭" + strings.Repeat("─", max(0, width-2)) + "╮")}
	for _, row := range rows {
		result = append(result, text(p.hairline).Render("│ ")+lipgloss.NewStyle().Width(inner).MaxWidth(inner).Foreground(lipgloss.Color(p.muted)).Render(row)+text(p.hairline).Render(" │"))
	}
	return append(result, text(p.hairline).Render("╰"+strings.Repeat("─", max(0, width-2))+"╯"))
}

func (m AppModel) notificationRows(n api.Notification, width int, p palette, member bool) []string {
	_, verb := notificationVerb(n)
	name := "Unknown actor"
	handle := ""
	if n.Actor != nil {
		name = safeText(n.Actor.Name)
		if name == "" {
			name = safeText(n.Actor.Handle)
		}
		handle = "@" + safeText(n.Actor.Handle)
	}
	identity := text(p.ink).Bold(true).Render(name)
	actorsCommit := n.Type == "replied" || n.Type == "quoted" || n.Type == "mentioned"
	stamp := text(p.muted).Render(relativeAge(n.CreatedAt, m.now))
	var rows []string
	if member && (n.Type == "starred" || n.Type == "followed") {
		top, bottom := "", ""
		if n.Actor != nil {
			top, bottom = m.largeAvatar(n.Actor, p)
		}
		icon, _ := notificationVerb(n)
		marker := text(authorColor(n.Actor, p)).Render(icon)
		if n.Type == "starred" {
			if image := m.images.Row(starIconRequest(true, authorColor(n.Actor, p)), 0); image != "" {
				marker = image
			}
		}
		meta := top + identity + " " + text(p.muted).Render(handle)
		tail := marker + " " + stamp
		meta = fit(meta, max(1, width-ansi.StringWidth(tail)-2))
		return []string{meta + strings.Repeat(" ", max(1, width-ansi.StringWidth(meta)-ansi.StringWidth(tail))) + tail, bottom}
	}
	if actorsCommit || member {
		meta := identity + " " + text(p.muted).Render(handle)
		if member && n.Actor != nil {
			meta = smallAvatar(n.Actor, p, m.images) + meta
		}
		meta = fit(meta, max(1, width-ansi.StringWidth(stamp)-2))
		rows = append(rows, meta+strings.Repeat(" ", max(1, width-ansi.StringWidth(meta)-ansi.StringWidth(stamp)))+stamp)
		rows = append(rows, text(p.muted).Render(verb))
	} else {
		actors := n.Actors
		if len(actors) == 0 && n.Actor != nil {
			actors = []api.Author{*n.Actor}
		}
		stack, under := "", ""
		for _, actor := range actors[:min(3, len(actors))] {
			top, bottom := m.largeAvatar(&actor, p)
			stack, under = stack+top, under+bottom
		}
		if n.ActorsCount > len(actors) {
			stack += text(p.muted).Render(fmt.Sprintf("+%d", n.ActorsCount-len(actors)))
		}
		stack = fit(stack, max(1, width-ansi.StringWidth(stamp)-2))
		rows = append(rows, stack+strings.Repeat(" ", max(1, width-ansi.StringWidth(stack)-ansi.StringWidth(stamp)))+stamp, under)
		if n.ActorsCount > 1 {
			noun := "others"
			if n.ActorsCount == 2 {
				noun = "other"
			}
			identity += fmt.Sprintf(" and %d %s", n.ActorsCount-1, noun)
		} else {
			identity += " " + text(p.muted).Render(handle)
		}
		if n.Type == "approved" || n.Type == "rejected" {
			identity = ""
		}
		rows = append(rows, strings.Split(ansi.Wrap(strings.TrimSpace(identity+" "+verb), width, ""), "\n")...)
	}
	boxed := n.Type == "starred" || n.Type == "approved" || n.Type == "rejected"
	rows = append(rows, notificationExcerpt(n.Commit, width, p, boxed)...)
	if n.Type == "quoted" && n.Commit != nil && n.Commit.Quote != nil {
		rows = append(rows, notificationExcerpt(n.Commit.Quote, width, p, true)...)
	}
	if n.Reason != "" {
		rows = append(rows, strings.Split(ansi.Wrap("reason: "+safeText(n.Reason), width, ""), "\n")...)
	}
	if n.Type == "rejected" {
		rows = append(rows, strings.Split(ansi.Wrap("Only you can see this update and your rejected commit.", width, ""), "\n")...)
	}
	if member {
		rows = append(rows, "")
	}
	return rows
}

func (m *AppModel) renderNotifications() {
	f := m.notificationFeed()
	p := colors(m.account.dark)
	width := max(1, f.viewport.Width())
	gutter := 7
	contentWidth := max(1, width-gutter-2)
	entries := m.notificationEntries()
	f.selected = max(0, min(f.selected, len(entries)-1))
	f.starts = nil
	f.ends = nil
	f.actions = nil
	var lines []string
	day := ""
	var previous *api.Notification
	selectedParent := ""
	if f.selected < len(entries) {
		selectedParent = entries[f.selected].parent
	}
	parentLanes := make(map[string]string, len(f.items))
	for _, n := range f.items {
		parentLanes[n.ID] = authorColor(n.Actor, p)
	}
	for i, entry := range entries {
		n := entry.notification
		member := entry.parent != ""
		lastMember := member && (i+1 == len(entries) || entries[i+1].parent != entry.parent)
		g := f.groups[n.ID]
		expanded := !member && g != nil && g.open && len(g.items) > 0
		if entry.parent == "" {
			if previous != nil && previous.NewSince(f.readAt) && !n.NewSince(f.readAt) {
				lines = append(lines, "", withBackground(lipgloss.NewStyle().Width(width).Render("       "+text(p.muted).Bold(true).Render("HEAD@{last visit}")), p.wash), "")
			}
			date := dayLabel(n.CreatedAt, m.now)
			if day != date {
				lines = append(lines, "       "+text(p.muted).Render(date), "       "+text(p.hairline).Render(strings.Repeat("─", max(0, width-7))))
				day = date
			}
			copy := n
			previous = &copy
		}
		f.starts = append(f.starts, len(lines))
		bodyWidth := contentWidth
		if member {
			bodyWidth = max(1, bodyWidth-2)
		}
		rows := m.notificationRows(n, bodyWidth, p, member)
		if i == f.selected && n.Actor != nil {
			chip := text(p.ink).Render("Profile") + " " + keyChip("u", p)
			f.actions = append(f.actions, hitArea{uiRect{gutter, len(lines) + len(rows), lipgloss.Width(chip), 1}, "u", i})
			rows = append(rows, chip)
		}
		ring := len(rows)
		if !member {
			rows = append(rows, "")
		}
		if entry.parent == "" && n.EventCount > 1 {
			label := fmt.Sprintf("› %d %s", n.EventCount, notificationNoun(n))
			if g := f.groups[n.ID]; g != nil && g.open {
				label = "⌄ Hide " + notificationNoun(n)
				if g.loading {
					label += " · Loading..."
				}
				if g.status != "" {
					rows = append(rows, strings.Split(ansi.Wrap(g.status, bodyWidth, ""), "\n")...)
				}
			}
			if i == f.selected || n.ID == selectedParent {
				label += "  " + keyChip("space", p)
			}
			f.actions = append(f.actions, hitArea{uiRect{gutter, len(lines) + len(rows), contentWidth, 1}, "space", i})
			rows = append(rows, text(p.blue).Render(label))
			if g := f.groups[n.ID]; g != nil && g.open && g.next != "" {
				f.actions = append(f.actions, hitArea{uiRect{gutter, len(lines) + len(rows), contentWidth, 1}, "n", i})
				rows = append(rows, text(p.blue).Render("n Show older "+notificationNoun(n)))
			}
			if expanded {
				rows = append(rows, "")
			}
		}
		lane := authorColor(n.Actor, p)
		icon, _ := notificationVerb(n)
		actorsCommit := n.Type == "replied" || n.Type == "quoted" || n.Type == "mentioned"
		for row, body := range rows {
			glyph := "│"
			if row == 0 {
				glyph = icon
			}
			if row == ring {
				glyph = "◉"
			}
			if row > ring {
				glyph = " "
			}
			rail := "   " + text(lane).Render(glyph) + "   "
			if !member && !actorsCommit && row < 2 {
				outline := "  │  "
				if row == 0 {
					outline = " (" + icon + ") "
				}
				node := text(lane).Render(outline)
				if image := m.images.Row(notificationIconRequest(n, p), row); image != "" {
					node = image
				}
				rail = " " + node + " "
			}
			if actorsCommit && row < 2 && n.Actor != nil {
				if avatar := m.images.Row(m.avatarRequest(n.Actor.AvatarURL, lane, 5, 2), row); avatar != "" {
					rail = " " + avatar + " "
				}
			}
			if expanded && row >= 2 {
				rail = "   " + text(lane).Render("│") + "   "
			}
			if member {
				rootLane := parentLanes[entry.parent]
				glyph := "│"
				if lastMember && row == len(rows)-1 {
					glyph = "◉"
				}
				rail = "   " + text(rootLane).Render(glyph) + "   "
				body = text(p.hairline).Render("│ ") + body
			}
			line := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(rail + fit(body, max(1, width-gutter)))
			if i == f.selected {
				line = withBackground(text(p.blue).Render("▌")+ansi.Cut(line, 1, width), p.wash)
			}
			lines = append(lines, line)
		}
		if (!member && !expanded) || lastMember {
			lines = append(lines, "       "+text(p.hairline).Render(strings.Repeat("─", max(0, width-7))))
		}
		f.ends = append(f.ends, len(lines))
	}
	if len(f.items) == 0 {
		message := "No notifications in the selected categories."
		if m.notificationMask == 0 {
			message = "All categories are turned off. Enable a category above."
		}
		lines = append(lines, "", lipgloss.NewStyle().Width(contentWidth).MarginLeft(gutter).Foreground(lipgloss.Color(p.muted)).Render(message))
	} else if f.next == "" && f.loaded {
		lines = append(lines, "", lipgloss.NewStyle().Width(contentWidth).MarginLeft(gutter).Foreground(lipgloss.Color(p.muted)).Render("That's the oldest notification. There's nothing before it."))
	} else if f.next != "" {
		f.actions = append(f.actions, hitArea{uiRect{gutter, len(lines), contentWidth, 1}, "more", 0})
		lines = append(lines, "       "+text(p.blue).Render("n Older notifications"))
	}
	f.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m AppModel) notificationMouse(x, y int) (AppModel, tea.Cmd) {
	_, width, _ := m.columns()
	header, areas := m.notificationHeader(width, colors(m.account.dark))
	for _, area := range areas {
		if area.contains(x, y) {
			return m.notificationKey(area.action)
		}
	}
	f := m.notificationFeed()
	row := y - lipgloss.Height(header) + f.viewport.YOffset()
	if y < lipgloss.Height(header) || y >= m.mainHeight() {
		return m, nil
	}
	for _, area := range f.actions {
		if area.contains(x, row) {
			if area.action == "more" {
				return m, m.loadNotifications("more")
			}
			f.selected = area.index
			return m, m.notificationControl(area.action)
		}
	}
	for i, start := range f.starts {
		if row >= start && row < f.ends[i] {
			f.selected = i
			m.renderNotifications()
			return m, m.openNotificationAt(i)
		}
	}
	return m, nil
}

func (m *AppModel) notificationControl(key string) tea.Cmd {
	next, cmd := m.notificationKey(key)
	*m = next
	return cmd
}

func (m *AppModel) openNotificationAt(i int) tea.Cmd {
	entry := m.notificationEntries()[i]
	n := entry.notification
	if profileRow(entry) {
		return m.openProfile(n.Actor.Handle)
	}
	if n.Commit == nil {
		m.notice = "This notification has no commit to open."
		return nil
	}
	return m.openThread(*n.Commit)
}

func (m *AppModel) notificationMedia() []graphics.Request {
	f := m.notificationFeed()
	p := colors(m.account.dark)
	var requests []graphics.Request
	for i, entry := range m.notificationEntries() {
		if i >= len(f.starts) || f.ends[i] <= f.viewport.YOffset() || f.starts[i] >= f.viewport.YOffset()+f.viewport.Height() {
			continue
		}
		n := entry.notification
		if entry.parent == "" && (n.Type == "starred" || n.Type == "followed" || n.Type == "approved" || n.Type == "rejected") {
			requests = append(requests, notificationIconRequest(n, p))
		}
		actorsCommit := n.Type == "replied" || n.Type == "quoted" || n.Type == "mentioned"
		if n.Actor != nil {
			cols, rows := 4, 2
			if entry.parent == "" && actorsCommit {
				cols = 5
			}
			if entry.parent != "" && actorsCommit {
				cols, rows = 2, 1
			}
			requests = append(requests, m.avatarRequest(n.Actor.AvatarURL, authorColor(n.Actor, p), cols, rows))
			if entry.parent != "" && n.Type == "starred" {
				requests = append(requests, starIconRequest(true, authorColor(n.Actor, p)))
			}
		}
		if entry.parent != "" {
			continue
		}
		for _, actor := range n.Actors[:min(3, len(n.Actors))] {
			requests = append(requests, m.avatarRequest(actor.AvatarURL, authorColor(&actor, p), 4, 2))
		}
	}
	return requests
}

func notificationIconRequest(n api.Notification, p palette) graphics.Request {
	return graphics.Request{Icon: n.Type, Ring: authorColor(n.Actor, p), Cols: 5, Rows: 2}
}
