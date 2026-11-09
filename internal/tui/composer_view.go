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
	return uiRect{r.x + 2, r.y + 3 + m.composerContextRows(), m.composer.input.Width(), m.composer.input.Height()}
}

func (m AppModel) composerContextRows() int {
	if m.composer.mode == "" || m.composer.target.Author == nil {
		return 0
	}
	return 3
}

func (m AppModel) composerStatusRows() int {
	editor := 1
	if m.composer.attachmentCount() > 0 {
		editor = 3
	}
	return max(0, min(3, m.composerHeight()-m.composerContextRows()-m.composerStripRows()-editor))
}

func (m AppModel) composerLabels() []string {
	preview, submit := "Preview", "Submit"
	if m.composer.scheduleAt != nil {
		submit = "Schedule"
	}
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
		label = fmt.Sprintf("%-*s", len([]string{"Preview", "Sending...", "Cancel"}[i]), label)
		actions = append(actions, modalAction{label, shortcut, ""})
	}
	focus := m.composer.focus
	if m.composer.sending {
		focus = 0
	}
	p := m.account.colors()
	width := max(1, r.w-4)
	counter := fmt.Sprintf("%d/%d", api.BodyCharacterCount(m.composer.input.Value()), api.MaxBodyLength)
	counterWidth := max(len(counter), len(fmt.Sprintf("%d/%d", api.MaxRawBodyLength, api.MaxBodyLength)))
	foot, areas := modalFooter(actions, max(1, width-counterWidth-2), focus, p)
	rows := strings.Split(foot, "\n")
	last := len(rows) - 1
	rows[last] += strings.Repeat(" ", max(1, width-lipgloss.Width(rows[last])-lipgloss.Width(counter))) + text(p.muted).Render(counter)
	foot = strings.Join(rows, "\n")
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
	title := m.composerTitle(p) + m.composerIdentity(p)
	footer, _ := m.composerFooter()
	return panel(title, body, footer, r, p, p.blue), r
}

func (m AppModel) composerTitle(p palette) string {
	title := map[string]string{"": "New commit", "reply": "Reply", "quote": "Quote commit"}[m.composer.mode]
	if m.composer.previewing {
		title = "Preview commit"
	}
	marker := "◌"
	if m.composer.hasDraft() {
		marker = "●"
	}
	return text(laneColor(m.account.user.Lane, p)).Render(marker) + " " + text(p.ink).Bold(true).Render(title) + " "
}

// composerIdentityArea is where the title shows who the commit is posted as, which a click changes like Ctrl+A.
func (m AppModel) composerIdentityArea() uiRect {
	p := m.account.colors()
	r := m.composerBounds()
	return uiRect{r.x + 2 + lipgloss.Width(m.composerTitle(p)), r.y + 1, lipgloss.Width(m.composerIdentity(p)), 1}
}

func (m *AppModel) sizeComposer() {
	c := &m.composer
	if !c.open {
		return
	}
	width := max(1, m.composerBounds().w-4)
	c.input.SetWidth(width)
	c.input.SetHeight(max(1, m.composerHeight()-m.composerContextRows()-m.composerStatusRows()-m.composerStripRows()))
	p := m.account.colors()
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
	inner := max(1, width)
	var rows []string
	if c.mode != "" && c.target.Author != nil {
		action := "↩ Replying to @"
		if c.mode == "quote" {
			action = "❝ Quoting @"
		}
		excerpt := strings.SplitN(ansi.Strip(bodyText(c.target, p, statsStyle{glyphs: m.icons})), "\n", 2)[0]
		for _, line := range []string{text(p.blue).Bold(true).Render(fit(action+safeText(c.target.Author.Handle), max(1, inner-2))), text(p.muted).Render(fit(excerpt, max(1, inner-2)))} {
			callout := text(p.blue).Render("│ ") + line
			rows = append(rows, withBackground(lipgloss.NewStyle().Width(inner).MaxWidth(inner).Render(callout), p.wash))
		}
		rows = append(rows, "")
	}
	body := c.input.View()
	if c.previewing {
		lines := bodyLines(api.Commit{Body: c.preview.Body, Segments: c.preview.Segments}, inner, p, m.statsStyle())
		if api.NormalizeBody(c.preview.Body) == "" {
			lines = nil
		}
		lines = append(lines, m.attachmentPreview(inner, p)...)
		start := min(c.previewOffset, max(0, len(lines)-c.input.Height()))
		body = strings.Join(lines[start:min(len(lines), start+c.input.Height())], "\n")
	}
	if c.owner != m.account.user.ID {
		body = text(p.muted).Render("Draft belongs to the previous account.")
	}
	rows = append(rows, lipgloss.NewStyle().Width(inner).Height(c.input.Height()).MaxHeight(c.input.Height()).Render(body))
	status := c.status
	if c.uncertain && c.status == "" {
		status = "Previous outcome unknown. Check the web before submitting again."
	}
	if notice := m.scheduleNotice(); notice != "" && status == "" {
		status = notice
	}
	statusView := ansi.Wrap(safeText(status), inner, "")
	if labels := c.suggest.labels(); len(labels) > 0 {
		var suggestions []string
		start := max(0, min(c.suggest.index-1, len(labels)-3))
		for i := start; i < min(len(labels), start+3); i++ {
			row := fit("  "+labels[i], inner)
			if i == c.suggest.index {
				row = text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render(fit("> "+labels[i], inner))
			}
			suggestions = append(suggestions, row)
		}
		statusView = strings.Join(suggestions, "\n")
	}
	if c.browser != nil {
		rows[len(rows)-1] = m.browserView(inner, c.input.Height()+m.composerStatusRows(), p)
	} else if c.picker != nil {
		rows[len(rows)-1] = m.authorPickerView(inner, c.input.Height()+m.composerStatusRows(), p)
	} else if c.schedule != nil {
		rows[len(rows)-1] = m.scheduleView(inner, c.input.Height()+m.composerStatusRows(), p)
	} else if height := m.composerStatusRows(); height > 0 {
		rows = append(rows, lipgloss.NewStyle().Width(inner).Height(height).MaxHeight(height).Foreground(lipgloss.Color(p.muted)).Render(statusView))
	}
	if strip := m.stripView(inner, p); strip != "" {
		rows = append(rows, strip)
	}
	return lipgloss.NewStyle().Width(width).Height(m.composerHeight()).MaxHeight(m.composerHeight()).Render(strings.Join(rows, "\n"))
}

func (m AppModel) confirmationModal(p palette) (string, uiRect, []hitArea) {
	c := m.composer
	saves := m.canSaveDrafts()
	title := "Discard this draft?"
	detail := "Keep the text for later, discard it, or go back to editing."
	if saves {
		detail = "Keep it in Drafts for later, discard it, or go back to editing."
	}
	if c.confirm == "exit" {
		title = "Exit and lose this draft?"
		detail = "Discarding exits the application. Keeping the draft stays in this session."
		if saves {
			title, detail = "Exit and keep this draft?", "Keeping saves it to Drafts and exits. Discarding exits without it."
		}
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
	summary := fmt.Sprintf("%d characters", api.BodyCharacterCount(c.input.Value()))
	switch {
	case c.poll != nil:
		summary += " · a poll"
	case len(c.images) == 1:
		summary += " · 1 image"
	case len(c.images) > 1:
		summary += fmt.Sprintf(" · %d images", len(c.images))
	}
	if saves {
		summary += " · Keep saves it on this device"
	} else {
		summary += " · memory only · nothing saved to disk"
	}
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
	border := p.danger
	if c.confirm == "retry" {
		border = p.blue
	}
	return panel("/dev/null · DRAFT PROTECTION", body, footer, r, p, border), r, areas
}
