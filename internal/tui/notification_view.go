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
		ink := p.muted
		if m.notificationMask&(1<<i) != 0 {
			ink = p.ink
		}
		chip := chipStyled(label, fmt.Sprint(i+1), ink, p)
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
	header, _ := m.notificationHeader(width, m.account.colors())
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

func notificationExcerpt(c *api.Commit, width int, p palette, style statsStyle, boxed bool) []string {
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
	rows := bodyLines(*c, inner, p, style)
	if len(rows) > 4 {
		rows = append(rows[:3], "…")
	}
	if len(c.Images) > 0 && len(rows) == 0 {
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

// notificationRows also returns where its avatars sit, relative to the body.
func (m AppModel) notificationRows(n api.Notification, width int, p palette, member bool) ([]string, []avatarHit) {
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
	var hits []avatarHit
	if member && (n.Type == "starred" || n.Type == "followed") {
		top, bottom := "", ""
		if n.Actor != nil {
			top, bottom = m.largeAvatar(n.Actor, p)
			hits = append(hits, authorHit(uiRect{0, 0, max(1, ansi.StringWidth(top)-1), 2}, n.Actor))
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
		return []string{meta + strings.Repeat(" ", max(1, width-ansi.StringWidth(meta)-ansi.StringWidth(tail))) + tail, bottom}, hits
	}
	if actorsCommit || member {
		meta := identity + " " + text(p.muted).Render(handle)
		if member && n.Actor != nil {
			avatar := smallAvatar(n.Actor, p, m.images, m.now)
			hits = append(hits, authorHit(uiRect{0, 0, max(1, ansi.StringWidth(avatar)-1), 1}, n.Actor))
			meta = avatar + meta
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
			hits = append(hits, authorHit(uiRect{ansi.StringWidth(stack), 0, max(1, ansi.StringWidth(top)-1), 2}, &actor))
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
	rows = append(rows, notificationExcerpt(n.Commit, width, p, m.statsStyle(), boxed)...)
	if n.Type == "quoted" && n.Commit != nil && n.Commit.Quote != nil {
		rows = append(rows, notificationExcerpt(n.Commit.Quote, width, p, m.statsStyle(), true)...)
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
	return rows, hits
}

func (m *AppModel) renderNotifications() {
	f := m.notificationFeed()
	p := m.account.colors()
	width := max(1, f.viewport.Width())
	gutter := 7
	contentWidth := max(1, width-gutter-2)
	entries := m.notificationEntries()
	f.selected = max(0, min(f.selected, len(entries)-1))
	f.starts = nil
	f.ends = nil
	f.actions = nil
	f.avatars = nil
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
		rows, hits := m.notificationRows(n, bodyWidth, p, member)
		indent := gutter
		if member {
			indent += 2
		}
		for _, hit := range hits {
			hit.area.x += indent
			hit.area.y += len(lines)
			f.avatars = append(f.avatars, hit)
		}
		if n.Type == "replied" || n.Type == "quoted" || n.Type == "mentioned" {
			if !member && n.Actor != nil && m.images.Row(m.authorAvatar(n.Actor, p, 5, 2), 0) != "" {
				f.avatars = append(f.avatars, authorHit(uiRect{1, len(lines), 5, 2}, n.Actor))
			}
		}
		if i == f.selected {
			var actions []commitAction
			for _, action := range notificationChips(entry) {
				actions = append(actions, commitAction{action[0], action[1]})
			}
			if len(actions) > 0 {
				chips, areas := commitChips(actions, bodyWidth, p)
				for _, area := range areas {
					area.x += gutter
					area.y += len(lines) + len(rows)
					area.index = i
					f.actions = append(f.actions, area)
				}
				rows = append(rows, chips...)
			}
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
			line := text(p.blue).Render(label)
			if i == f.selected || n.ID == selectedParent {
				line = actionChip(label, "space", p)
			}
			f.actions = append(f.actions, hitArea{uiRect{gutter, len(lines) + len(rows), contentWidth, 1}, "space", i})
			rows = append(rows, line)
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
				if avatar := m.images.Row(m.authorAvatar(n.Actor, p, 5, 2), row); avatar != "" {
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

// notificationChips are the selected entry's labeled shortcuts: Enter opens its commit, u its actor.
func notificationChips(entry notificationEntry) [][2]string {
	var chips [][2]string
	if entry.notification.Commit != nil {
		chips = append(chips, [2]string{"Open", "enter"})
	}
	if hasProfile(entry.notification.Actor) {
		chips = append(chips, [2]string{"Profile", "u"})
	}
	return chips
}

func (m AppModel) notificationMouse(x, y int) (AppModel, tea.Cmd) {
	_, width, _ := m.columns()
	header, areas := m.notificationHeader(width, m.account.colors())
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
	if cmd, ok := m.openAvatarAt(f.avatars, x, row); ok {
		return m, cmd
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
		return m.openAuthor(n.Actor)
	}
	if n.Commit == nil {
		m.notice = "This notification has no commit to open."
		return nil
	}
	return m.openThread(*n.Commit)
}

func (m *AppModel) notificationMedia() []graphics.Request {
	f := m.notificationFeed()
	p := m.account.colors()
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
			requests = append(requests, m.authorAvatar(n.Actor, p, cols, rows))
			if entry.parent != "" && n.Type == "starred" {
				requests = append(requests, starIconRequest(true, authorColor(n.Actor, p)))
			}
		}
		if entry.parent != "" {
			continue
		}
		for _, actor := range n.Actors[:min(3, len(n.Actors))] {
			requests = append(requests, m.authorAvatar(&actor, p, 4, 2))
		}
	}
	return requests
}

func notificationIconRequest(n api.Notification, p palette) graphics.Request {
	return graphics.Request{Icon: n.Type, Ring: authorColor(n.Actor, p), Cols: 5, Rows: 2}
}
