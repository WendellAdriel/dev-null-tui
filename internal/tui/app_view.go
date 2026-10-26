package tui

import (
	"fmt"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

const feedHeaderHeight = 8

type appKeys struct{ short, all []key.Binding }

func (k appKeys) ShortHelp() []key.Binding  { return k.short }
func (k appKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.all} }

func (m AppModel) bindings() appKeys {
	bind := func(k, label string) key.Binding { return key.NewBinding(key.WithKeys(k), key.WithHelp(k, label)) }
	if m.composer.open {
		keys := []key.Binding{bind("ctrl+enter", "submit"), bind("ctrl+s", "submit fallback"), bind("ctrl+p", "preview / edit"), bind("f2", m.account.nextTheme()+" theme"), bind("tab / shift+tab", "focus control"), bind("enter", "new line or activate control"), bind("esc", "back or cancel"), bind("ctrl+c", "request exit")}
		if m.composer.previewing {
			keys = append(keys, bind("up/down / pgup/pgdown", "scroll preview"))
		}
		return appKeys{keys, keys}
	}
	short := []key.Binding{bind("j/k", "move"), bind("tab", "log"), bind("c", "compose"), bind("ctrl+r", "refresh")}
	if len(m.feeds[m.active].commits) > 0 && m.account.width >= 100 {
		short = append(short, bind("y", "copy link"))
	}
	short = append(short, bind("f2", m.account.nextTheme()+" theme"), bind("?", "help"), bind("ctrl+c", "quit"))
	all := append([]key.Binding(nil), short...)
	all = append(all, bind("a", "account"), bind("1/2/3", "choose log"), bind("g l", "return to log"), bind("g e", "Trending"), bind("g g / home", "first commit"), bind("G / end", "last commit"), bind("pgup/pgdown", "move five commits"), bind("ctrl+u/ctrl+d", "move five commits"), bind("] / l / right", "next log"), bind("[ / shift+tab", "previous log"), bind("wheel", "move selection"), bind("click", "select commit"), bind("esc", "close overlay"))
	if len(m.feeds[m.active].commits) > 0 && m.account.width < 100 {
		all = append(all, bind("y", "copy link"))
	}
	if m.feeds[m.active].next != "" {
		all = append(all, bind("n", "older commits"))
	}
	return appKeys{short, all}
}

func keyChip(value string, p palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.wash)).Render(" " + value + " ")
}

func filledButton(label, shortcut string, width int, background, foreground string) string {
	content := label
	if shortcut != "" {
		content += strings.Repeat(" ", max(1, width-2-ansi.StringWidth(label)-len(shortcut))) + shortcut
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(background)).Foreground(lipgloss.Color(foreground)).Bold(true).Width(width).Padding(0, 1).Align(lipgloss.Center).Render(content)
}

func (m AppModel) View() tea.View {
	v := m.workspaceView()
	p := colors(m.account.dark)
	if m.composer.open && m.connected && !m.accountOpen {
		content, r := m.composerModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.composer.confirm != "" {
		content, r, _ := m.confirmationModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.toast.visible && m.composer.confirm == "" && m.account.width >= 20 && m.account.height >= 8 {
		content, r := m.toastView()
		v.Content = m.overlayToast(v.Content, content, r)
	}
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m AppModel) workspaceView() tea.View {
	if !m.connected || m.accountOpen {
		v := m.account.View()
		v.ReportFocus = true
		return v
	}
	w, h := m.account.width, m.account.height
	if w <= 0 || h <= 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}
	p := colors(m.account.dark)
	var page string
	if w < 30 || h < 10 {
		page = "/dev/null needs a bigger terminal."
	} else {
		nav, center, aside := m.columns()
		total := nav + center + aside + 2
		main := m.feedColumn(center, p)
		rail := text(p.hairline).Render(strings.TrimSuffix(strings.Repeat("│\n", m.mainHeight()), "\n"))
		main = lipgloss.JoinHorizontal(lipgloss.Top, m.navigation(nav, m.mainHeight(), p), rail, main, rail)
		if aside > 0 {
			main = lipgloss.JoinHorizontal(lipgloss.Top, main, m.aside(aside, m.mainHeight(), p))
		}
		page = lipgloss.NewStyle().MarginLeft((w-total)/2).Render(main) + "\n" + m.workspaceFooter(w)
	}
	frame := lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).Width(w).Height(h).MaxWidth(w).MaxHeight(h)
	page = frame.Render(page)
	if m.help && w >= 30 && h >= 10 {
		modal := m.helpModal(p)
		page = overlay(page, modal, m.helpBounds(), p, w, h)
	}
	v := tea.NewView(page)
	v.AltScreen, v.ReportFocus = true, true
	v.MouseMode = tea.MouseModeCellMotion
	if m.account.profile >= colorprofile.ANSI && (m.account.theme != "auto" || m.account.backgroundKnown) {
		v.BackgroundColor = lipgloss.Color(p.shell)
		v.ForegroundColor = lipgloss.Color(p.ink)
	}
	return v
}

func (m AppModel) mainHeight() int {
	return max(1, m.account.height-1)
}

func brand(p palette) string {
	return text(p.lanes[0]).Bold(true).Render("//") + "  " + text(p.ink).Bold(true).Render("/dev/null")
}

func (m AppModel) footer(width int, p palette) string {
	if m.composer.open {
		if len(m.composer.emojis) > 0 {
			return fit(" ↑/↓ emoji  enter/tab insert  esc dismiss  f2 "+m.account.nextTheme(), width)
		}
		return fit(" ctrl+enter send  ctrl+p preview  f2 "+m.account.nextTheme()+"  esc back", width)
	}
	return m.workspaceFooter(width)
}

func (m AppModel) workspaceFooter(width int) string {
	return fit(" "+m.feedStatus(), width)
}

func (m AppModel) feedTabs(width int, p palette) string {
	var labels, underline strings.Builder
	for i, name := range logNames {
		cellWidth := width / 3
		if i == 2 {
			cellWidth = width - 2*(width/3)
		}
		label := name
		if i == m.active {
			label = text(p.ink).Bold(true).Render(label)
		} else {
			label = text(p.muted).Render(label)
		}
		labels.WriteString(lipgloss.NewStyle().Width(cellWidth).Align(lipgloss.Center).Render(label))
		line := strings.Repeat(" ", cellWidth)
		if i == m.active {
			mark := min(len(name)+2, cellWidth)
			pad := (cellWidth - mark) / 2
			line = strings.Repeat(" ", pad) + text(p.blue).Render(strings.Repeat("━", mark)) + strings.Repeat(" ", cellWidth-pad-mark)
		}
		underline.WriteString(line)
	}
	return labels.String() + "\n" + underline.String()
}

func (m AppModel) feedColumn(width int, p palette) string {
	height := m.mainHeight()
	var content string
	switch {
	case m.manualLink != "":
		content, _ = m.manualLinkView(width, p)
	default:
		f := m.feeds[m.active]
		user := m.account.user
		avatar := m.images.Row(m.avatarRequest(user.AvatarURL, laneColor(user.Lane, p), 2, 1), 0)
		if avatar == "" {
			avatar = text(laneColor(user.Lane, p)).Render("◯ ")
		}
		reader := "  " + avatar + " " + text(p.muted).Render("What did you ship?")
		refresh := keyChip("c Commit", p)
		reader = fit(reader, max(1, width-lipgloss.Width(refresh)-2)) + strings.Repeat(" ", max(1, width-lipgloss.Width(reader)-lipgloss.Width(refresh)-1)) + refresh
		hairline := text(p.hairline).Render(strings.Repeat("─", width))
		content = "\n" + m.feedTabs(width, p) + "\n" + hairline + "\n\n" + reader + "\n\n" + hairline + "\n"
		if !f.loaded {
			title, detail := "Reading your log...", "Fetching commits with your signed connection."
			if !f.loading && f.status != "" {
				title, detail = "Couldn't load this log", f.status
			}
			placeholder := "\n\n" + text(p.ink).Bold(true).Render(title) + "\n\n" + text(p.muted).Render(detail)
			content += lipgloss.NewStyle().Width(max(1, width-12)).MarginLeft(6).Render(placeholder)
		} else {
			content += m.feedViewport(p)
		}
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(content)
}

func (m AppModel) navigation(width, height int, p palette) string {
	column, _ := m.navigationLayout(width, height, p)
	return column
}

func (m AppModel) navigationLayout(width, height int, p palette) (string, []hitArea) {
	inner := width - 2
	navRow := func(left, right string, active bool) string {
		if right == "" {
			line := lipgloss.NewStyle().Width(inner).Align(lipgloss.Center).Render(left)
			if active {
				return withBackground(line, p.soft)
			}
			return line
		}
		line := " " + left + strings.Repeat(" ", max(1, inner-lipgloss.Width(left)-len(right)-2)) + right + " "
		if active {
			return withBackground(fit(line, inner), p.soft)
		}
		return fit(line, inner)
	}
	logo := "   " + brand(p)
	log, logKey := "◉  Log", "g l"
	if width < 12 {
		logo = " //"
		log, logKey = "◉", ""
	}
	top := "\n" + logo + "\n\n\n " + navRow(text(p.blue).Bold(true).Render(log), logKey, true)
	compose, shortcut := "＋ Commit", "c"
	if width < 12 {
		compose, shortcut = "+", ""
	}
	button := filledButton(compose, shortcut, inner, p.blue, p.shell)
	buttonY := lipgloss.Height(top) + 1
	buttonX := (width - lipgloss.Width(button)) / 2
	top += "\n\n" + lipgloss.PlaceHorizontal(width, lipgloss.Center, button)
	areas := []hitArea{{uiRect{1, 4, inner, 1}, "log", 0}, {uiRect{buttonX, buttonY, lipgloss.Width(button), lipgloss.Height(button)}, "c", 0}}
	user := m.account.user
	lane := laneColor(user.Lane, p)
	avatar := m.images.Row(m.avatarRequest(user.AvatarURL, lane, 2, 1), 0)
	if avatar == "" {
		avatar = text(lane).Render("● ")
	}
	bottom := "  " + avatar
	if width >= 12 {
		bottom += text(p.muted).Render(fit("@"+safeText(user.Username), inner-4))
	}
	gap := max(1, height-lipgloss.Height(top)-lipgloss.Height(bottom))
	content := top + strings.Repeat("\n", gap) + bottom + "\n"
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).MaxWidth(width).Render(content), areas
}

func (m AppModel) manualLinkView(width int, p palette) (string, hitArea) {
	content := "  " + text(p.ink).Bold(true).Render("Copy commit link") + "\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n\n  Clipboard unavailable. Copy this link:\n\n" + lipgloss.NewStyle().Width(width-4).MarginLeft(2).Render(m.manualLink)
	back := "  " + keyChip("esc", p) + " Back to log"
	area := hitArea{uiRect{2, lipgloss.Height(content) + 1, min(width-2, lipgloss.Width(back)-2), 1}, "esc", 0}
	return content + "\n\n" + back, area
}

func (m AppModel) aside(width, height int, p palette) string {
	column, _ := m.asideLayout(width, height, p)
	return column
}

func (m AppModel) asideLayout(width, height int, p palette) (string, []hitArea) {
	var areas []hitArea
	u := m.account.user
	inner := width - 3
	lane := laneColor(u.Lane, p)
	identity := text(lane).Render("◉ ") + text(p.ink).Bold(true).Render(fit(safeText(u.Name), inner-2)) + "\n" + text(p.lanes[1]).Render("● ") + text(p.muted).Render(fit("@"+safeText(u.Username), inner-2))
	req := m.avatarRequest(u.AvatarURL, lane, 4, 2)
	if top, bottom := m.images.Row(req, 0), m.images.Row(req, 1); top != "" && bottom != "" {
		identity = top + " " + text(p.ink).Bold(true).Render(fit(safeText(u.Name), inner-5)) + "\n" + bottom + " " + text(p.muted).Render(fit("@"+safeText(u.Username), inner-5))
	}
	content := "\n" + identity + "\n\n"
	var stats []string
	for _, stat := range []struct {
		value *int
		label string
	}{{u.Commits, "commits"}, {u.Following, "following"}, {u.Followers, "followers"}} {
		if stat.value != nil {
			value := text(p.ink).Bold(true).Render(fmt.Sprint(*stat.value)) + text(p.muted).Render(" "+stat.label)
			stats = append(stats, value)
		}
	}
	if len(stats) > 0 {
		row := ""
		for _, stat := range stats {
			if row != "" && lipgloss.Width(row)+2+lipgloss.Width(stat) > inner {
				content += row + "\n"
				row = ""
			}
			if row != "" {
				row += "  "
			}
			row += stat
		}
		content += row + "\n\n"
	}
	if u.Bio != "" {
		content += lipgloss.NewStyle().Width(inner).MaxHeight(3).Foreground(lipgloss.Color(p.muted)).Render(safeText(u.Bio)) + "\n\n"
	}
	for _, line := range []string{u.Company, u.Location} {
		if line != "" {
			content += text(p.muted).Render(fit(safeText(line), inner)) + "\n"
		}
	}
	if u.Company != "" || u.Location != "" {
		content += "\n"
	}
	if u.PasteisMode && u.PasteisRemaining != nil {
		content += text(p.muted).Render(fit(fmt.Sprintf("◍ %d pastéis left today", *u.PasteisRemaining), inner)) + "\n\n"
	}
	line := text(p.hairline).Render(strings.Repeat("─", inner))
	controls := line + "\n" + text(p.ink).Bold(true).Width(inner).Align(lipgloss.Center).Render("Actions") + "\n"
	for i, action := range []struct{ label, key string }{{"Account", "a"}, {"Refresh", "ctrl+r"}} {
		if i > 0 {
			controls += "\n\n"
		}
		button := filledButton(action.label, action.key, inner-2, p.wash, p.ink)
		x := (inner - lipgloss.Width(button)) / 2
		areas = append(areas, hitArea{uiRect{2 + x, strings.Count(controls, "\n"), lipgloss.Width(button), lipgloss.Height(button)}, action.key, 0})
		controls += lipgloss.PlaceHorizontal(inner, lipgloss.Center, button)
	}
	controls += "\n" + line + "\n" + text(p.ink).Bold(true).Render("Shortcuts") + "\n"
	workspace := m
	workspace.composer.open = false
	for _, binding := range workspace.bindings().short {
		h := binding.Help()
		if h.Key == "c" || h.Key == "ctrl+r" || h.Key == "a" {
			continue
		}
		action := h.Key
		if action == "j/k" {
			action = "down"
		}
		areas = append(areas, hitArea{uiRect{2, strings.Count(controls, "\n"), inner, 1}, action, 0})
		chip := keyChip(h.Key, p)
		label := fit(h.Desc, max(1, inner-lipgloss.Width(chip)-1))
		controls += text(p.muted).Render(label) + strings.Repeat(" ", max(1, inner-lipgloss.Width(label)-lipgloss.Width(chip))) + chip + "\n"
	}
	origin, _ := url.Parse(m.account.origin)
	bottom := text(p.muted).Render(fit(safeText(origin.Host), inner))
	profileHeight := max(1, height-lipgloss.Height(controls)-3)
	content = strings.TrimRight(lipgloss.NewStyle().MaxHeight(profileHeight).Render(content), "\n") + "\n\n"
	for i := range areas {
		areas[i].y += strings.Count(content, "\n")
	}
	content += controls
	gap := max(1, height-lipgloss.Height(content)-lipgloss.Height(bottom))
	content += strings.Repeat("\n", gap) + bottom
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).MaxWidth(width).PaddingLeft(2).PaddingRight(1).Render(content), areas
}

func (m AppModel) feedViewport(p palette) string {
	f := m.feeds[m.active]
	view := f.viewport.View()
	height, total, width := f.viewport.Height(), f.viewport.TotalLineCount(), f.viewport.Width()
	if total <= height || height <= 2 || width <= 1 {
		return view
	}
	lines := strings.Split(view, "\n")
	thumb := max(1, height*height/total)
	top := f.viewport.YOffset() * (height - thumb) / max(1, total-height)
	for i := top; i < min(len(lines), top+thumb); i++ {
		lines[i] = ansi.Cut(lines[i], 0, width-1) + text(p.hairline).Render("▐")
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) helpLines(width int, p palette) []string {
	lines := []string{text(p.muted).Render("Log and composer controls."), ""}
	for _, binding := range m.bindings().all {
		h := binding.Help()
		chip := keyChip(h.Key, p)
		label := fit(h.Desc, max(1, width-lipgloss.Width(chip)-1))
		lines = append(lines, label+strings.Repeat(" ", max(1, width-lipgloss.Width(chip)-lipgloss.Width(label)))+chip)
	}
	return append(lines, "", text(p.muted).Render("↑/↓ scroll · esc close"))
}

func (m AppModel) helpMaxScroll() int {
	r := m.helpBounds()
	return max(0, len(m.helpLines(max(1, r.w-4), colors(m.account.dark)))-max(1, r.h-6))
}

func (m AppModel) helpBounds() uiRect { return centeredRect(m.account.width, m.account.height, 62, 48) }

func (m AppModel) helpModal(p palette) string {
	r := m.helpBounds()
	lines := m.helpLines(max(1, r.w-4), p)
	start := min(m.helpScroll, m.helpMaxScroll())
	content := strings.Join(lines[start:min(len(lines), start+max(1, r.h-6))], "\n")
	return panel("Keyboard shortcuts", content, "↑/↓ scroll · esc close", r, p)
}
