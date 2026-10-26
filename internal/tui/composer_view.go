package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func (m AppModel) composerHeight() int {
	return max(1, m.composerBounds().h-6)
}

func (m AppModel) headerHeight() int {
	return feedHeaderHeight
}

func (m AppModel) composerBounds() uiRect {
	return centeredRect(m.account.width, m.account.height, 86, 24)
}
func (m AppModel) composerUsable() bool { return m.account.width >= 48 && m.account.height >= 18 }
func (m AppModel) composerEditorBounds() uiRect {
	r := m.composerBounds()
	return uiRect{r.x + 7, r.y + 4, m.composer.input.Width(), m.composer.input.Height()}
}

func (m AppModel) composerLabels() []string {
	preview, submit := "Preview", "Submit"
	if m.composer.previewing {
		preview = "Edit"
	}
	if m.composer.sending {
		submit = "Sending..."
	}
	return []string{preview, submit, "Cancel"}
}

func (m AppModel) composerControls() []hitArea {
	r := m.composerBounds()
	x := r.x + 7
	var controls []hitArea
	for i, label := range m.composerLabels() {
		controls = append(controls, hitArea{uiRect{x, r.y + r.h - 4, len(label) + 2, 1}, "composer", i + 1})
		x += len(label) + 3
	}
	return controls
}

func (m AppModel) composerModal(p palette) (string, uiRect) {
	r := m.composerBounds()
	body := m.composerView(max(1, r.w-4), p)
	if !m.composerUsable() {
		body = "Resize to at least 48x18.\nYour draft is kept.\nEsc to close safely."
	}
	title := "New commit"
	if m.composer.previewing {
		title = "Preview commit"
	}
	return panel(title, body, m.footer(max(1, r.w-4), p), r, p), r
}

func (m *AppModel) sizeComposer() {
	c := &m.composer
	if !c.open {
		return
	}
	width := max(1, m.composerBounds().w-4)
	c.input.SetWidth(max(1, width-7))
	c.input.SetHeight(max(1, m.composerHeight()-6))
	p := colors(m.account.dark)
	styles := textarea.DefaultStyles(m.account.dark)
	for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		state.Base = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell))
		state.Text, state.CursorLine = lipgloss.NewStyle(), lipgloss.NewStyle()
		state.Placeholder = text(p.muted)
		state.EndOfBuffer = text(p.shell)
	}
	c.input.SetStyles(styles)
}

func (m AppModel) composerView(width int, p palette) string {
	c := m.composer
	inner := max(1, width-7)
	lane := laneColor(m.account.user.Lane, p)
	marker := text(lane).Render("◌")
	if c.input.Value() != "" {
		marker = text(lane).Render("●")
	}
	label := "New commit"
	if c.previewing {
		label = "Preview"
	}
	rows := []string{"  " + marker + "  " + text(p.ink).Bold(true).Render(label) + " " + text(p.muted).Render("@"+safeText(m.account.user.Username))}
	body := c.input.View()
	if c.previewing {
		body = ansi.Wrap(bodyText(api.Commit{Body: c.preview.Body, Segments: c.preview.Segments}, p), inner, "")
		lines := strings.Split(body, "\n")
		start := min(c.previewOffset, max(0, len(lines)-c.input.Height()))
		body = strings.Join(lines[start:min(len(lines), start+c.input.Height())], "\n")
	}
	if c.owner != m.account.user.ID {
		body = text(p.muted).Render("Draft belongs to the previous account.")
	}
	rows = append(rows, lipgloss.NewStyle().Width(inner).Height(c.input.Height()).MaxHeight(c.input.Height()).MarginLeft(5).Render(body))
	count := api.BodyCharacterCount(c.input.Value())
	counter := fmt.Sprintf("%d/%d", count, api.MaxBodyLength)
	rows = append(rows, "     "+text(p.muted).Render(counter))
	status := c.status
	if status == "" {
		status = "Ctrl+Enter or Ctrl+S submits. Ctrl+P previews. Tab selects controls."
	}
	if c.uncertain && c.status == "" {
		status = "Previous outcome unknown. Check the web before submitting again."
	}
	statusView := ansi.Wrap(safeText(status), inner, "")
	if len(c.emojis) > 0 {
		var suggestions []string
		start := max(0, min(c.emojiIndex-1, len(c.emojis)-3))
		for i := start; i < min(len(c.emojis), start+3); i++ {
			choice := c.emojis[i]
			row := "  " + choice.value + " :" + choice.name + ":"
			if i == c.emojiIndex {
				row = text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render("> " + choice.value + " :" + choice.name + ":")
			}
			suggestions = append(suggestions, fit(row, inner))
		}
		statusView = strings.Join(suggestions, "\n")
	}
	rows = append(rows, lipgloss.NewStyle().Width(inner).Height(3).MaxHeight(3).MarginLeft(5).Foreground(lipgloss.Color(p.muted)).Render(statusView))
	button := func(label string, focused bool) string {
		if focused && !c.sending {
			return text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render("[" + label + "]")
		}
		return text(p.muted).Render(" " + label + " ")
	}
	var buttons []string
	for i, label := range m.composerLabels() {
		buttons = append(buttons, button(label, c.focus == i+1))
	}
	rows = append(rows, "     "+strings.Join(buttons, " "))
	return lipgloss.NewStyle().Width(width).Height(m.composerHeight()).MaxHeight(m.composerHeight()).Render(strings.Join(rows, "\n"))
}

func (m AppModel) confirmationModal(p palette) (string, uiRect, []hitArea) {
	c := m.composer
	title, action := "Discard this draft?", "Discard draft"
	detail := "Keep editing, or discard the text stored in this session."
	if c.confirm == "exit" {
		title, action = "Exit and lose this draft?", "Exit anyway"
		detail = "Your text stays in this session only. Keep editing to avoid losing it."
	}
	if c.confirm == "retry" {
		title, action = "Submit again?", "Submit again"
		detail = "The previous commit may exist. Another submission risks a duplicate."
	}
	keep := "[Keep draft]"
	if c.confirmAction {
		keep, action = " Keep draft ", "["+action+"]"
	}
	w, h := max(1, m.account.width), max(1, m.account.height)
	compactTitle := title
	if c.confirm == "retry" {
		compactTitle = "Risk a duplicate?"
	}
	if w < 48 || h < 18 {
		content := compactTitle + "\n" + keep + "\n" + action + "\nTab / Enter · f2 " + m.account.nextTheme()
		r := uiRect{0, 0, w, h}
		areas := []hitArea{{uiRect{0, 1, min(w, len(keep)), 1}, "confirm", 0}, {uiRect{0, 2, min(w, len(action)), 1}, "confirm", 1}}
		return lipgloss.NewStyle().Width(w).Height(h).MaxWidth(w).MaxHeight(h).Background(lipgloss.Color(p.shell)).Foreground(lipgloss.Color(p.ink)).Render(content), r, areas
	}
	r := centeredRect(w, h, 76, 18)
	width := r.w - 4
	node := "●  LOCAL DRAFT"
	if c.sending || c.uncertain {
		node = "◌  DELIVERY UNKNOWN"
		if c.confirm != "retry" {
			detail = "The server may still publish this commit. Check the web before submitting again."
		}
	}
	summary := fmt.Sprintf("%d characters · memory only · nothing saved to disk", api.BodyCharacterCount(c.input.Value()))
	if c.owner != m.account.user.ID {
		summary = "Draft belongs to another account. Its text stays private."
	}
	body := text(p.blue).Render(node) + "\n" + text(p.ink).Bold(true).Render(title) + "\n\n" + text(p.muted).Render(ansi.Wrap(detail, width, "")) + "\n" + text(p.muted).Render(ansi.Wrap(summary, width, ""))
	button := func(value string, focused, danger bool) string {
		style := text(p.ink)
		if focused {
			color := p.blue
			if danger {
				color = p.danger
			}
			style = style.Foreground(lipgloss.Color(p.shell)).Background(lipgloss.Color(color)).Bold(true)
		}
		return style.Render(" " + value + " ")
	}
	keepButton, actionButton := button(keep, !c.confirmAction, false), button(action, c.confirmAction, true)
	body = lipgloss.NewStyle().Width(width).Height(max(1, r.h-8)).MaxHeight(max(1, r.h-8)).Render(body) + "\n\n" + keepButton + "  " + actionButton
	areas := []hitArea{{uiRect{r.x + 2, r.y + r.h - 4, lipgloss.Width(keepButton), 1}, "confirm", 0}, {uiRect{r.x + 4 + lipgloss.Width(keepButton), r.y + r.h - 4, lipgloss.Width(actionButton), 1}, "confirm", 1}}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "confirm", 0})
	return panel("/dev/null · DRAFT PROTECTION", body, "tab choose · enter confirm · esc keep · f2 "+m.account.nextTheme(), r, p), r, areas
}
