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
	// key runs the row from the commit menu, and danger marks a destructive one.
	key    string
	danger bool
	// heading starts a group of rows, such as Event themes. It can't be selected.
	heading bool
}

// picker is a centered list dialog: Starred by, the link picker, the Live now picker, and the themes.
type picker struct {
	kind, title, intro string
	rows               []pickerRow
	selected           int
	// The Starred by list pages from its commit, with next as the cursor.
	commitID, next, status string
	loading                bool
	generation             int
	// The commit menu keeps its commit and, once read, the author's profile.
	commit api.Commit
	author *api.Profile
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
	step := 1
	if delta < 0 {
		step = -1
	}
	at := p.selected
	for remaining := max(delta, -delta); remaining > 0; remaining-- {
		next := at + step
		for next >= 0 && next < len(p.rows) && p.rows[next].heading {
			next += step
		}
		if next < 0 || next >= count {
			break
		}
		at = next
	}
	p.selected = at
}

// first selects the first row that isn't a heading.
func (p *picker) first() {
	p.selected = 0
	for p.selected < len(p.rows)-1 && p.rows[p.selected].heading {
		p.selected++
	}
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
	footerRows := 1
	if d.kind == "theme" {
		footerRows = 2
	}
	shown := min(rows, max(1, min(pickerVisibleRows, m.account.height-9-footerRows-len(lines))))
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
		if i < len(d.rows) && d.rows[i].person != nil && d.rows[i].key == "" {
			areas = append(areas, hitArea{uiRect{0, top + i - first, 2, 1}, "avatar", i})
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
	verb := "Open"
	switch d.kind {
	case "commit-menu":
		verb = "Run"
	case "event-theme", "theme":
		verb = "Apply"
	}
	hints := []statusPart{hintChip("Choose", "↑/↓", "down"), statusChip(verb, "enter")}
	if d.next != "" && verb == "Open" {
		hints = append(hints, statusChip("More", "n"))
	}
	footer, footerAreas := hintRow(append(hints, closeChip("Close")), width, pal)
	if d.kind == "theme" {
		// The custom theme actions get their own row, so the picker stays narrow.
		custom := []statusPart{statusChip("New theme", "n")}
		if d.selected < len(d.rows) && m.customIndex(d.rows[d.selected].value) >= 0 {
			custom = append(custom, statusChip("Edit", "e"), statusChip("Delete", "D"))
		}
		second, secondAreas := hintRow(custom, width, pal)
		for _, area := range secondAreas {
			area.y++
			footerAreas = append(footerAreas, area)
		}
		footer += "\n" + second
	}
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), width+4, lipgloss.Height(body)+5+lipgloss.Height(footer))
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + 3
	}
	areas = append(areas, panelFooterAreas(footerAreas, footer, r)...)
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel(d.title, body, footer, r, pal, pal.blue), r, areas
}

func (m AppModel) listPickerRow(row pickerRow, width int, pal palette) string {
	if row.heading {
		return text(pal.muted).Render(fit(safeText(row.label), width))
	}
	label := text(pal.ink).Bold(true).Render(safeText(row.label))
	if row.danger {
		label = text(pal.danger).Bold(true).Render(safeText(row.label))
	}
	if row.value == "" && row.person == nil {
		label = text(pal.muted).Render(safeText(row.label))
	}
	if row.key != "" {
		return split(label, text(pal.blue).Render(row.key), width)
	}
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
	if d.kind == "commit-menu" {
		for i, row := range d.rows {
			if row.key == pressed {
				d.selected = i
				return m.choosePicker()
			}
		}
	}
	if d.kind == "theme" {
		switch pressed {
		case "n":
			m.openThemeBases()
			return m, nil
		case "e":
			if row := d.rows[d.selected]; m.customIndex(row.value) >= 0 {
				return m, m.editThemeEditor(row.value)
			}
			return m, nil
		case "D":
			m.askDeleteTheme()
			return m, nil
		}
	}
	switch pressed {
	case "esc", "q":
		m.picker = nil
		return m, m.syncImages()
	case "j", "down", "tab":
		d.move(1)
	case "k", "up", "shift+tab":
		d.move(-1)
	case "home", "g":
		d.first()
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
	_, r, areas := m.listPickerModal(m.account.colors())
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if pressed, ok := keyAction(area.action); ok {
			return m.listPickerKey(pressed)
		}
		if area.action == "esc" {
			m.picker = nil
			return m, m.syncImages()
		}
		if area.action == "avatar" {
			// The picker routes keys before the viewer, so it closes first.
			person := m.picker.rows[area.index].person
			m.picker = nil
			return m, tea.Batch(m.openAvatar("@"+safeText(person.Handle), person.AvatarURL), m.syncImages())
		}
		if area.index < len(m.picker.rows) && m.picker.rows[area.index].heading {
			return m, nil
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
	if d.kind == "commit-menu" {
		return m.chooseCommitMenu(row)
	}
	m.picker = nil
	switch d.kind {
	case "stars":
		return m, tea.Batch(m.syncImages(), m.openProfile(row.person.Handle))
	case "reactions":
		if row.value == "" {
			return m, nil
		}
		return m, tea.Batch(m.syncImages(), m.openProfile(row.value))
	case "live":
		return m, tea.Batch(m.syncImages(), m.watchLive(row.person.Live))
	case "event-theme":
		return m, m.setEventTheme(row.value)
	case "theme":
		return m, m.setTheme(row.value)
	case "theme-base":
		return m, m.newThemeEditor(row.value)
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
	_, _, areas := m.askModal(m.account.colors())
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
	case "delete-draft":
		return m, m.deleteDraft(a.id)
	case "share-commit":
		return m, m.shareCommit(a.commit, a.id)
	case "delete-theme":
		return m, m.deleteCustomTheme(a.id)
	}
	return m, nil
}
