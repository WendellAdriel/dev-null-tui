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

// badgeGlyphs are the text stand-ins for the web's achievement icons, keyed by the icon names the API sends.
var badgeGlyphs = map[string]string{
	"terminal": "›", "file-code": "≡", "flame": "∆", "zap": "ϟ", "server": "☰", "box": "□", "shield": "◈", "star": "☆",
	"sparkles": "✧", "git-merge": "⑂", "key": "⌐", "pastel-de-nata": "◍", "bot-off": "⊘", "bomb": "◉",
	"dragon-ball": "✪", "mushroom-cloud": "♣", "award": "◎", "git-commit-horizontal": "⊸",
}

// badgeIconName falls back to the terminal icon for names it doesn't know, as the web does.
func badgeIconName(icon string) string {
	if _, ok := badgeGlyphs[icon]; ok {
		return icon
	}
	return "terminal"
}

func badgeRequest(icon, ink string) graphics.Request {
	icon = badgeIconName(icon)
	if icon == "pastel-de-nata" {
		return pastelIconRequest
	}
	return graphics.Request{Icon: "badge-" + icon, Ring: ink, Cols: 2, Rows: 1}
}

// badgeIcon draws an achievement icon in two cells, or its glyph until the image is ready.
func badgeIcon(icon, ink string, images *graphics.Manager) string {
	if images != nil {
		if row := images.Row(badgeRequest(icon, ink), 0); row != "" {
			return row
		}
	}
	return text(ink).Render(badgeGlyphs[badgeIconName(icon)])
}

// achievementsDialog is the web's All achievements popover: every badge, with the selected one's details below.
type achievementsDialog struct {
	badges   []api.Badge
	selected int
}

func (m *AppModel) openAchievements() {
	s := m.topProfile()
	if s == nil || s.people || s.organization || !s.loaded || s.profile.Achievements == nil || len(s.profile.Achievements.Badges) == 0 {
		return
	}
	m.achievements = &achievementsDialog{badges: s.profile.Achievements.Badges}
}

func badgeLabel(b api.Badge) string {
	label := safeText(b.Name)
	if !b.Unlocked && b.Target > 1 {
		label += fmt.Sprintf(" (%d/%d)", b.Progress, b.Target)
	}
	return label
}

// tallChip pads a chip with half blocks above and below, so its background reads a line taller.
func tallChip(content, background string) [3]string {
	middle := withBackground("  "+content+"  ", background)
	width := lipgloss.Width(middle)
	return [3]string{text(background).Render(strings.Repeat("▄", width)), middle, text(background).Render(strings.Repeat("▀", width))}
}

func (m AppModel) badgeChip(b api.Badge, selected bool, p palette) [3]string {
	ink, background := p.ink, p.wash
	if !b.Unlocked {
		ink, background = p.muted, p.shell
	}
	if selected {
		background = p.soft
	}
	return tallChip(badgeIcon(b.Icon, ink, m.images)+" "+text(ink).Render(badgeLabel(b)), background)
}

// achievementsLayout wraps the badge chips into rows, returning each row's lines and every chip's cell, where y is
// the chip row.
func (m AppModel) achievementsLayout(width int, p palette) ([][3]string, []uiRect) {
	var rows [][3]string
	var cells []uiRect
	var row [3]string
	for i, b := range m.achievements.badges {
		chip := m.badgeChip(b, i == m.achievements.selected, p)
		x := lipgloss.Width(row[1])
		if x > 0 && x+1+lipgloss.Width(chip[1]) > width {
			rows, row, x = append(rows, row), [3]string{}, 0
		}
		if x > 0 {
			x++
			for line := range row {
				row[line] += " "
			}
		}
		for line := range row {
			row[line] += chip[line]
		}
		cells = append(cells, uiRect{x, len(rows), lipgloss.Width(chip[1]), 3})
	}
	return append(rows, row), cells
}

func (m AppModel) achievementsModal(p palette) (string, uiRect, []hitArea) {
	width := max(1, min(60, m.account.width-4)-4)
	rows, cells := m.achievementsLayout(width, p)
	b := m.achievements.badges[m.achievements.selected]
	state := "✓"
	if !b.Unlocked {
		state = fmt.Sprintf("(%d/%d)", b.Progress, b.Target)
	}
	details := []string{text(p.ink).Bold(true).Render(fit(safeText(b.Name)+" "+state, width))}
	for _, line := range strings.Split(ansi.Wrap(safeText(b.Description), width, ""), "\n") {
		details = append(details, text(p.muted).Render(line))
	}
	// The panel keeps the details whole, so the chip rows scroll to the selected badge when they don't all fit.
	shown := min(len(rows), max(1, (m.account.height-4-6-2-len(details))/3))
	first := min(max(0, cells[m.achievements.selected].y-(shown-1)/2), len(rows)-shown)
	var lines []string
	for _, row := range rows[first : first+shown] {
		lines = append(lines, row[:]...)
	}
	body := strings.Join(lines, "\n") + "\n\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n" + strings.Join(details, "\n")
	hints := []statusPart{hintChip("Choose", "←/→", "right")}
	if first > 0 {
		hints = append(hints, hintText("↑ more"))
	}
	if first+shown < len(rows) {
		hints = append(hints, hintText("↓ more"))
	}
	footer, footerAreas := hintRow(append(hints, closeChip("Close")), width, p)
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), width+4, lipgloss.Height(body)+6)
	areas := append([]hitArea{{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0}}, panelFooterAreas(footerAreas, footer, r)...)
	for i, cell := range cells {
		if cell.y >= first && cell.y < first+shown {
			areas = append(areas, hitArea{uiRect{r.x + 2 + cell.x, r.y + 3 + (cell.y-first)*cell.h, cell.w, cell.h}, "badge", i})
		}
	}
	return panel("All achievements", body, footer, r, p, p.blue), r, areas
}

func (m AppModel) achievementsKey(pressed string) (AppModel, tea.Cmd) {
	d := m.achievements
	switch pressed {
	case "esc", "A", "q":
		m.achievements = nil
	case "right", "l", "tab", "down", "j":
		d.selected = (d.selected + 1) % len(d.badges)
	case "left", "h", "shift+tab", "up", "k":
		d.selected = (d.selected + len(d.badges) - 1) % len(d.badges)
	case "home", "g":
		d.selected = 0
	case "end", "G":
		d.selected = len(d.badges) - 1
	}
	return m, m.syncImages()
}

func (m AppModel) achievementsMouse(x, y int) (AppModel, tea.Cmd) {
	_, r, areas := m.achievementsModal(m.account.colors())
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if pressed, ok := keyAction(area.action); ok {
			return m.achievementsKey(pressed)
		}
		if area.action == "esc" {
			m.achievements = nil
		} else {
			m.achievements.selected = area.index
		}
		return m, m.syncImages()
	}
	if !r.contains(x, y) {
		m.achievements = nil
	}
	return m, nil
}

// achievementsMedia lists the popup's icons, one per badge and ink.
func (m AppModel) achievementsMedia(p palette) []graphics.Request {
	var requests []graphics.Request
	for _, b := range m.achievements.badges {
		ink := p.ink
		if !b.Unlocked {
			ink = p.muted
		}
		requests = append(requests, badgeRequest(b.Icon, ink))
	}
	return requests
}
