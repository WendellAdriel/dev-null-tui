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
	footer, _ := m.composerFooter()
	return max(1, m.composerBounds().h-5-lipgloss.Height(footer))
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
	return uiRect{r.x + 7, r.y + 4 + m.composerContextRows(), m.composer.input.Width(), m.composer.input.Height()}
}

func (m AppModel) composerContextRows() int {
	if m.composer.mode == "" {
		return 0
	}
	return 2
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
	_, controls := m.composerFooter()
	return controls
}

func (m AppModel) composerFooter() (string, []hitArea) {
	r := m.composerBounds()
	var actions []modalAction
	for i, label := range m.composerLabels() {
		shortcut := []string{"ctrl+p", "ctrl+enter/ctrl+s", "esc"}[i]
		actions = append(actions, modalAction{label, shortcut, ""})
	}
	focus := m.composer.focus
	if m.composer.sending {
		focus = 0
	}
	foot, areas := modalFooter(actions, max(1, r.w-4), focus, colors(m.account.dark))
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(foot)
		areas[i].index++
	}
	return foot, areas
}

func (m AppModel) composerModal(p palette) (string, uiRect) {
	r := m.composerBounds()
	body := m.composerView(max(1, r.w-4), p)
	if !m.composerUsable() {
		body = "Resize to at least 48x18.\nYour draft is kept.\nEsc to close safely."
	}
	title := map[string]string{"": "New commit", "reply": "Reply", "quote": "Quote commit"}[m.composer.mode]
	if m.composer.previewing {
		title = "Preview commit"
	}
	footer, _ := m.composerFooter()
	return panel(title, body, footer, r, p), r
}

func (m *AppModel) sizeComposer() {
	c := &m.composer
	if !c.open {
		return
	}
	width := max(1, m.composerBounds().w-4)
	c.input.SetWidth(max(1, width-7))
	c.input.SetHeight(max(1, m.composerHeight()-5-m.composerContextRows()))
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
	label := map[string]string{"": "New commit", "reply": "Reply", "quote": "Quote"}[c.mode]
	if c.previewing {
		label = "Preview"
	}
	rows := []string{"  " + marker + "  " + text(p.ink).Bold(true).Render(label) + " " + text(p.muted).Render("@"+safeText(m.account.user.Handle))}
	if c.mode != "" && c.target.Author != nil {
		action := "↩ Replying to @"
		if c.mode == "quote" {
			action = "❝ Quoting @"
		}
		excerpt := strings.SplitN(ansi.Strip(bodyText(c.target, p)), "\n", 2)[0]
		rows = append(rows, "     "+text(p.blue).Render(fit(action+safeText(c.target.Author.Handle), inner)), "     "+text(p.muted).Render(fit(excerpt, inner)))
	}
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
	return lipgloss.NewStyle().Width(width).Height(m.composerHeight()).MaxHeight(m.composerHeight()).Render(strings.Join(rows, "\n"))
}

func (m AppModel) confirmationModal(p palette) (string, uiRect, []hitArea) {
	c := m.composer
	title := "Discard this draft?"
	detail := "Keep the text for later, discard it, or go back to editing."
	if c.confirm == "exit" {
		title = "Exit and lose this draft?"
		detail = "Discarding exits the application. Keeping the draft stays in this session."
	}
	actions := []modalAction{{"Keep draft", "ctrl+k", "ctrl+k"}, {"Discard draft", "ctrl+d", "ctrl+d"}, {"Back", "esc", "esc"}}
	if c.confirm == "retry" {
		title = "Submit again?"
		detail = "The previous commit may exist. Another submission risks a duplicate."
		actions[1] = modalAction{"Submit again", "ctrl+s", "ctrl+s"}
	}
	w, h := max(1, m.account.width), max(1, m.account.height)
	compactTitle := title
	if c.confirm == "retry" {
		compactTitle = "Risk a duplicate?"
	}
	if w < 48 || h < 18 {
		footer, areas := modalFooter(actions, w, c.confirmFocus+1, p)
		content := compactTitle + "\n" + footer
		r := uiRect{0, 0, w, h}
		for i := range areas {
			areas[i].y++
		}
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
	footer, areas := modalFooter(actions, width, c.confirmFocus+1, p)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 2})
	return panel("/dev/null · DRAFT PROTECTION", body, footer, r, p), r, areas
}
