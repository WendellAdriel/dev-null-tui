package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var searchHeadings = map[string]string{
	"action":       "Actions",
	"developer":    "Developers",
	"organization": "Organizations",
	"commit":       "Most relevant commits",
	"recent":       "Recent searches",
}

var actionLabels = map[string]struct{ label, command string }{
	"commit":     {"Write a commit", ""},
	"feature":    {"Request a feature on the web", "/feature"},
	"bug":        {"Report a bug on the web", "/bug"},
	"go-live":    {"Go live", ""},
	"end-stream": {"End stream", ""},
}

// searchLine is one row of the dialog's list, and the option it opens, or -1 for a heading. clear marks the
// Recent searches heading, whose Clear link is clickable.
type searchLine struct {
	text   string
	option int
	clear  bool
}

func (m AppModel) searchModal(p palette) (string, uiRect, []hitArea) {
	d := m.search
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 76, 26)
	width := max(1, r.w-4)
	options := m.searchOptions()
	selected := min(d.index, max(0, len(options)-1))
	field := d.field
	field.SetWidth(max(1, width-4))
	box := text(p.muted).Render("/ ") + field.View()
	if m.searchLoading() {
		box = split(box, text(p.muted).Render("…"), width)
	}
	rows := []string{fit(box, width)}
	switch {
	case m.searchThrottled():
		rows = append(rows, text(p.warning).Render(fit("error: too many searches, give it a few seconds", width)))
	case m.searchFoundNothing():
		rows = append(rows, text(p.muted).Render(fit("fatal: nothing matches '"+safeText(d.term())+"'", width)))
	default:
		rows = append(rows, "")
	}
	var actions []modalAction
	if len(options) > 0 {
		actions = append(actions, modalAction{"Open", "enter", "enter"})
	}
	if len(options) > 0 && options[0].kind == "recent" {
		actions = append(actions, modalAction{"Clear recent", "ctrl+l", "ctrl+l"})
	}
	actions = append(actions, modalAction{"Close", "esc", "esc"})
	footer, areas := modalFooter(actions, width, -1, p)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	room := max(1, r.h-5-lipgloss.Height(footer)-len(rows))
	top := r.y + 3 + len(rows)
	if len(options) == 0 {
		rows = append(rows, text(p.muted).Render(fit(m.searchPlaceholder(), width)))
	}
	lines := m.searchLines(options, selected, width, p)
	start := 0
	for i, line := range lines {
		if line.option == selected && selected > 0 {
			start = max(0, min(i-room+1, len(lines)-room))
			break
		}
	}
	for i := start; i < min(len(lines), start+room); i++ {
		line := lines[i]
		switch {
		case line.option >= 0:
			areas = append(areas, hitArea{uiRect{r.x + 2, top + i - start, width, 1}, "search-option", line.option})
		case line.clear:
			areas = append(areas, hitArea{uiRect{r.x + 2 + width - 5, top + i - start, 5, 1}, "ctrl+l", 0})
		}
		rows = append(rows, line.text)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel("/dev/null · Search", strings.Join(rows, "\n"), footer, r, p, p.blue), r, areas
}

func (m AppModel) searchPlaceholder() string {
	d := m.search
	switch {
	case d.term() == "@":
		return "Type a name or handle to find a developer"
	case d.term() == "":
		return "Start typing to search. @ for developers."
	case m.searchFoundNothing():
		return ""
	}
	return "Searching…"
}

func (m AppModel) searchLines(options []searchOption, selected, width int, p palette) []searchLine {
	var lines []searchLine
	for i, option := range options {
		if heading, ok := searchHeadings[option.kind]; ok && (i == 0 || options[i-1].kind != option.kind) {
			if i > 0 {
				lines = append(lines, searchLine{text: "", option: -1})
			}
			label := text(p.muted).Render(heading)
			if option.kind == "recent" {
				label = split(label, text(p.blue).Render("Clear"), width)
			}
			lines = append(lines, searchLine{text: label, option: -1, clear: option.kind == "recent"})
		}
		marker := "  "
		if i == selected {
			marker = text(p.blue).Render("▌ ")
		}
		lines = append(lines, searchLine{text: marker + m.searchOptionText(option, i == selected, width-2, p), option: i})
	}
	return lines
}

func (m AppModel) searchOptionText(option searchOption, active bool, width int, p palette) string {
	ink := text(p.ink)
	if active {
		ink = ink.Bold(true)
	}
	switch option.kind {
	case "action":
		label := actionLabels[option.action]
		return split(ink.Render(label.label), text(p.muted).Render(label.command), width)
	case "developer":
		return m.profileLine(laneColor(option.user.Lane, p), option.user.Name, option.user.Handle, option.user.Bio, ink, width, p)
	case "organization":
		o := option.organization
		return m.profileLine(laneColor(o.Lane, p), o.Name, o.Handle, o.Bio, ink, width, p)
	case "commit":
		c := option.commit
		handle, lane := "", p.muted
		if c.Author != nil {
			handle, lane = "@"+safeText(c.Author.Handle), authorColor(c.Author, p)
		}
		head := text(lane).Render("●") + " " + text(p.muted).Render(handle+" · "+relativeAge(c.CreatedAt, m.now)) + "  "
		body := strings.Join(strings.Fields(safeText(c.Body)), " ")
		return fit(head+ink.Render(body), width)
	case "all":
		return text(p.blue).Bold(active).Render(fit("See all matching commits →", width))
	}
	return ink.Render(fit("↺ "+safeText(option.term), width))
}

func (m AppModel) profileLine(lane, name, handle, bio string, ink lipgloss.Style, width int, p palette) string {
	line := text(lane).Render("●") + " " + ink.Render(safeText(name)) + " " + text(p.muted).Render("@"+safeText(handle))
	if bio = strings.Join(strings.Fields(safeText(bio)), " "); bio != "" {
		line += text(p.muted).Render("  " + bio)
	}
	return fit(line, width)
}

func (m AppModel) searchMouse(x, y int) (AppModel, tea.Cmd) {
	_, _, areas := m.searchModal(colors(m.account.dark))
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if area.action == "search-option" {
			options := m.searchOptions()
			if area.index >= len(options) {
				return m, nil
			}
			m.search.index = area.index
			cmd := m.openSearchOption(options[area.index])
			return m, cmd
		}
		return m.mouseKey(area.action)
	}
	return m, nil
}

// searchRow is the aside's Search box, which opens search like Ctrl+K, as at the top of the web's aside.
func searchRow(width int, p palette) string {
	label := text(p.muted).Render("/ Search")
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.hairline)).Width(width).Padding(0, 1).
		Render(split(label, keyChip("ctrl+k", p), max(1, width-4)))
}
