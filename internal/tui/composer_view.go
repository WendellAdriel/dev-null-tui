package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func (m AppModel) composerHeight() int {
	return min(max(1, m.mainHeight()-5), max(8, min(14, m.account.height/2)))
}

func (m AppModel) headerHeight() int {
	if m.composer.open {
		return 5 + m.composerHeight()
	}
	return feedHeaderHeight
}

func (m *AppModel) sizeComposer(width int) {
	c := &m.composer
	if !c.open {
		return
	}
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
		if focused {
			return text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render("[" + label + "]")
		}
		return text(p.muted).Render(" " + label + " ")
	}
	previewLabel := "Preview"
	if c.previewing {
		previewLabel = "Edit"
	}
	submitLabel := "Submit"
	if c.sending {
		submitLabel = "Sending..."
	}
	buttons := []string{button(previewLabel, c.focus == 1), button(submitLabel, c.focus == 2), button("Cancel", c.focus == 3)}
	rows = append(rows, "     "+strings.Join(buttons, " "))
	return lipgloss.NewStyle().Width(width).Height(m.composerHeight()).MaxHeight(m.composerHeight()).Render(strings.Join(rows, "\n"))
}

func (m AppModel) draftConfirmationView() tea.View {
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
	p := colors(m.account.dark)
	compactTitle := title
	if c.confirm == "retry" {
		compactTitle = "Risk a duplicate?"
	}
	content := compactTitle + "\n" + keep + "\n" + action + "\nTab / Enter · f2 " + m.account.nextTheme()
	if w >= 60 && h >= 18 {
		width := min(88, w-8)
		label := text(p.muted).Render("DRAFT PROTECTION")
		head := brand(p) + strings.Repeat(" ", max(2, width-lipgloss.Width(brand(p))-lipgloss.Width(label))) + label + "\n" + text(p.hairline).Render(strings.Repeat("─", width))
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
		body := text(p.blue).Render(node) + "\n" + text(p.hairline).Render("│") + "\n" + text(p.ink).Bold(true).Render(title) + "\n" + text(p.muted).Render(ansi.Wrap(detail, width, "")) + "\n" + text(p.muted).Render(ansi.Wrap(summary, width, ""))
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
		actions := button(keep, !c.confirmAction, false) + "  " + button(action, c.confirmAction, true)
		footer := text(p.hairline).Render(strings.Repeat("─", width)) + "\n" + text(p.muted).Render("tab choose · enter confirm · esc keep · f2 "+m.account.nextTheme())
		page := head + "\n\n" + body + "\n\n" + actions + "\n\n" + footer
		top := max(0, min(6, (h-lipgloss.Height(page))/3))
		content = lipgloss.NewStyle().MarginLeft((w - width) / 2).MarginTop(top).Render(page)
	}
	v := m.account.View()
	v.ReportFocus = true
	v.Content = lipgloss.NewStyle().Width(w).Height(h).MaxWidth(w).MaxHeight(h).Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).Render(content)
	return v
}
