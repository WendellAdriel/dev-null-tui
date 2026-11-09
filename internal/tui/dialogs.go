package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// pickerRow is one choice in a picker: a person with an avatar, or a plain link.
type pickerRow struct {
	person      *api.User
	label, note string
	value       string
}

// picker is a centered list dialog: Starred by, the link picker, and the Live now picker.
type picker struct {
	kind, title, intro string
	rows               []pickerRow
	selected           int
	// The Starred by list pages from its commit, with next as the cursor.
	commitID, next, status string
	loading                bool
	generation             int
}

const pickerVisibleRows = 8

func (p *picker) move(delta int) {
	count := len(p.rows)
	if p.next != "" {
		count++
	}
	if count == 0 {
		return
	}
	p.selected = max(0, min(count-1, p.selected+delta))
}

// moreRow reports whether the Show more row is selected.
func (p *picker) moreRow() bool { return p.next != "" && p.selected == len(p.rows) }

func (m AppModel) listPickerModal(pal palette) (string, uiRect, []hitArea) {
	d := m.picker
	width := max(1, min(56, m.account.width-4)-4)
	var lines []string
	if d.intro != "" {
		lines = append(lines, text(pal.muted).Render(fit(d.intro, width)), "")
	}
	rows := len(d.rows)
	if d.next != "" {
		rows++
	}
	shown := min(rows, max(1, min(pickerVisibleRows, m.account.height-10-len(lines))))
	first := min(max(0, d.selected-shown/2), max(0, rows-shown))
	top := len(lines)
	var areas []hitArea
	for i := first; i < first+shown; i++ {
		line := ""
		if i == len(d.rows) {
			label := "Show more"
			if d.loading {
				label = "Loading…"
			}
			line = text(pal.blue).Render(fit(label, width))
		} else {
			line = m.listPickerRow(d.rows[i], width, pal)
		}
		if i == d.selected {
			line = withBackground(lipgloss.NewStyle().Width(width).Render(line), pal.soft)
		}
		areas = append(areas, hitArea{uiRect{0, top + i - first, width, 1}, "row", i})
		lines = append(lines, line)
	}
	if len(d.rows) == 0 {
		status := d.status
		if d.loading {
			status = "Loading…"
		}
		lines = append(lines, text(pal.muted).Render(fit(status, width)))
	} else if d.status != "" {
		lines = append(lines, "", text(pal.danger).Render(fit(d.status, width)))
	}
	body := strings.Join(lines, "\n")
	hint := "j/k choose  enter open  esc close"
	if d.next != "" {
		hint = "j/k choose  enter open  n more  esc close"
	}
	footer := text(pal.muted).Render(fit(hint, width))
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), width+4, lipgloss.Height(body)+6)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + 3
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel(d.title, body, footer, r, pal, pal.blue), r, areas
}

func (m AppModel) listPickerRow(row pickerRow, width int, pal palette) string {
	label := text(pal.ink).Bold(true).Render(safeText(row.label))
	if row.person != nil {
		_, avatar := m.userAvatar(row.person, 2, 1, pal)
		label = avatar[0] + " " + label
	}
	if row.note != "" {
		label += " " + text(pal.muted).Render(safeText(row.note))
	}
	return fit(label, width)
}

func (m AppModel) listPickerMedia(pal palette) []graphics.Request {
	var requests []graphics.Request
	for _, row := range m.picker.rows {
		if row.person != nil {
			req, _ := m.userAvatar(row.person, 2, 1, pal)
			requests = append(requests, req)
		}
	}
	return requests
}

func (m AppModel) listPickerKey(pressed string) (AppModel, tea.Cmd) {
	d := m.picker
	switch pressed {
	case "esc", "q":
		m.picker = nil
		return m, m.syncImages()
	case "j", "down", "tab":
		d.move(1)
	case "k", "up", "shift+tab":
		d.move(-1)
	case "home", "g":
		d.selected = 0
	case "end", "G":
		d.move(len(d.rows) + 1)
	case "n":
		return m, m.loadStargazers(false)
	case "enter":
		return m.choosePicker()
	}
	return m, nil
}

func (m AppModel) listPickerMouse(x, y int) (AppModel, tea.Cmd) {
	_, r, areas := m.listPickerModal(colors(m.account.dark))
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if area.action == "esc" {
			m.picker = nil
			return m, m.syncImages()
		}
		m.picker.selected = area.index
		return m.choosePicker()
	}
	if !r.contains(x, y) {
		m.picker = nil
		return m, m.syncImages()
	}
	return m, nil
}

func (m AppModel) choosePicker() (AppModel, tea.Cmd) {
	d := m.picker
	if d.moreRow() {
		return m, m.loadStargazers(false)
	}
	if d.selected >= len(d.rows) {
		return m, nil
	}
	row := d.rows[d.selected]
	m.picker = nil
	switch d.kind {
	case "stars":
		return m, tea.Batch(m.syncImages(), m.openProfile(row.person.Handle))
	case "live":
		return m, tea.Batch(m.syncImages(), m.watchLive(row.person.Live))
	}
	return m, tea.Batch(m.syncImages(), m.openLink(row.value))
}

// ask is a yes-or-no confirmation, such as replacing the pinned commit or canceling a scheduled one.
type ask struct {
	kind, title, question, detail string
	confirm                       string
	border                        string
	focus                         int
	commit                        api.Commit
	id                            string
}

func (m AppModel) askModal(pal palette) (string, uiRect, []hitArea) {
	a := m.ask
	actions := []modalAction{{"Cancel", "esc", "esc"}, {a.confirm, "ctrl+y", "ctrl+y"}}
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 11)
	width := max(1, r.w-4)
	// The detail wraps to two lines, so a narrow terminal still shows which commit it names.
	detail := strings.Split(ansi.Wrap(a.detail, width, ""), "\n")
	if len(detail) > 2 {
		detail = append(detail[:1], fit(strings.Join(detail[1:], " "), width))
	}
	for i := range detail {
		detail[i] = text(pal.muted).Render(fit(detail[i], width))
	}
	body := text(pal.ink).Bold(true).Render(fit(a.question, width)) + "\n\n" + strings.Join(detail, "\n")
	footer, areas := modalFooter(actions, width, a.focus+1, pal)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel(a.title, body, footer, r, pal, a.border), r, areas
}

func (m AppModel) askKey(pressed string) (AppModel, tea.Cmd) {
	switch pressed {
	case "tab", "shift+tab", "left", "right":
		m.ask.focus = 1 - m.ask.focus
	case "esc":
		m.ask = nil
	case "enter":
		if m.ask.focus == 0 {
			m.ask = nil
			return m, nil
		}
		return m.confirmAsk()
	case "ctrl+y":
		return m.confirmAsk()
	}
	return m, nil
}

func (m AppModel) askMouse(x, y int) (AppModel, tea.Cmd) {
	_, _, areas := m.askModal(colors(m.account.dark))
	for _, area := range areas {
		if area.contains(x, y) {
			return m.askKey(area.action)
		}
	}
	return m, nil
}

func (m AppModel) confirmAsk() (AppModel, tea.Cmd) {
	a := *m.ask
	m.ask = nil
	switch a.kind {
	case "pin":
		return m, m.savePin(a.commit, true)
	case "cancel-scheduled":
		return m, m.cancelScheduled(a.id)
	}
	return m, nil
}
