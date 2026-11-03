package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// composerStrip is the focus index of the attachment strip. Indexes 1 to 3 belong to the footer buttons.
const composerStrip = 4

const imageLimitReason = "Attach at most 3 images. Remove one first."

var pollDurationLabels = []string{"1 hour", "1 day", "3 days", "7 days"}

type pollDraft struct {
	options  []string
	duration int
}

func (c composer) hasDraft() bool { return c.input.Value() != "" || len(c.images) > 0 || c.poll != nil }

func (c composer) attachmentCount() int {
	if c.poll != nil {
		return len(c.poll.options)
	}
	return len(c.images)
}

func (c composer) pollBlocked() string {
	switch {
	case c.mode == "reply":
		return "Polls can't go on replies."
	case c.mode == "quote":
		return "Polls can't go on quotes."
	case len(c.images) > 0:
		return "Polls can't go with images. Remove them first."
	}
	return ""
}

func (m AppModel) composerDraft() api.NewCommit {
	c := m.composer
	draft := api.NewCommit{Body: c.input.Value(), Images: c.images}
	switch c.mode {
	case "reply":
		draft.ParentID = c.target.ID
	case "quote":
		draft.QuotedID = c.target.ID
	}
	if c.poll != nil {
		draft.Poll = &api.NewPoll{Options: c.poll.options, Duration: api.PollDurations[c.poll.duration]}
	}
	return draft
}

// attachImages adds images in order and stops at the first refusal, which it returns so the caller can show it.
func (m *AppModel) attachImages(paths []string) string {
	c := &m.composer
	reason := ""
	for _, path := range paths {
		if c.poll != nil {
			reason = "Remove the poll to add images."
			break
		}
		if len(c.images) >= api.MaxImages {
			reason = imageLimitReason
			break
		}
		file, err := api.ReadImage(path)
		if err != nil {
			reason = err.Error() + "."
			break
		}
		c.images = append(c.images, file)
	}
	m.attachmentsChanged()
	c.status = reason
	return reason
}

func (m *AppModel) attachmentsChanged() {
	c := &m.composer
	m.invalidatePreview()
	c.strip = max(0, min(c.strip, c.attachmentCount()-1))
	if c.attachmentCount() == 0 && c.focus == composerStrip {
		c.focus = 0
	}
	m.sizeComposer()
}

func (m *AppModel) startPoll() tea.Cmd {
	c := &m.composer
	if c.poll == nil {
		if reason := c.pollBlocked(); reason != "" {
			c.status = reason
			return nil
		}
		c.poll = &pollDraft{options: []string{"", ""}, duration: 1}
		c.strip = 0
		m.attachmentsChanged()
	}
	c.focus = composerStrip
	return m.editAttachment()
}

func (m *AppModel) editAttachment() tea.Cmd {
	c := &m.composer
	field := textinput.New()
	field.Prompt = ""
	if c.poll != nil {
		c.editing, field.CharLimit, field.Placeholder = "option", api.MaxPollOptionLength, fmt.Sprintf("Option %d", c.strip+1)
		field.SetValue(c.poll.options[c.strip])
	} else {
		c.editing, field.CharLimit, field.Placeholder = "description", api.MaxDescriptionLength, "Describe the image for screen readers"
		field.SetValue(c.images[c.strip].Description)
	}
	field.SetWidth(max(1, m.composerBounds().w-14))
	c.field, c.emojis = field, nil
	c.input.Blur()
	return c.field.Focus()
}

func (m *AppModel) fieldKey(msg tea.KeyPressMsg) tea.Cmd {
	c := &m.composer
	switch msg.String() {
	case "enter":
		if c.editing == "option" {
			c.poll.options[c.strip] = c.field.Value()
		} else {
			c.images[c.strip].Description = c.field.Value()
		}
		c.editing = ""
		m.invalidatePreview()
		return nil
	case "esc":
		c.editing = ""
		return nil
	}
	var cmd tea.Cmd
	c.field, cmd = c.field.Update(msg)
	return cmd
}

// stripKey handles the keys that act on the focused attachment. It reports false for keys the composer owns.
func (m *AppModel) stripKey(pressed string) (tea.Cmd, bool) {
	c := &m.composer
	count := c.attachmentCount()
	switch pressed {
	case "up":
		c.strip = max(0, c.strip-1)
	case "down":
		c.strip = min(count-1, c.strip+1)
	case "enter":
		return m.editAttachment(), true
	case "delete":
		m.removeAttachment()
	case "alt+up", "alt+down":
		if c.poll != nil {
			return nil, true
		}
		to := c.strip - 1
		if pressed == "alt+down" {
			to = c.strip + 1
		}
		if to >= 0 && to < count {
			c.images[c.strip], c.images[to] = c.images[to], c.images[c.strip]
			c.strip = to
			m.invalidatePreview()
		}
	case "ctrl+n", "left", "right", "ctrl+x":
		if c.poll == nil {
			return nil, true
		}
		return m.pollKey(pressed), true
	default:
		return nil, false
	}
	return nil, true
}

func (m *AppModel) removeAttachment() {
	c := &m.composer
	if c.poll == nil {
		c.images = slices.Delete(c.images, c.strip, c.strip+1)
		m.attachmentsChanged()
		if c.focus == 0 {
			m.focusComposer()
		}
		return
	}
	if len(c.poll.options) <= api.MinPollOptions {
		c.status = "A poll needs at least 2 options. Ctrl+X removes the poll."
		return
	}
	c.poll.options = slices.Delete(c.poll.options, c.strip, c.strip+1)
	m.attachmentsChanged()
}

func (m *AppModel) pollKey(pressed string) tea.Cmd {
	c := &m.composer
	switch pressed {
	case "ctrl+n":
		if len(c.poll.options) >= api.MaxPollOptions {
			c.status = "A poll can have at most 4 options."
			return nil
		}
		c.poll.options = append(c.poll.options, "")
		c.strip = len(c.poll.options) - 1
		m.attachmentsChanged()
		return m.editAttachment()
	case "left":
		c.poll.duration = max(0, c.poll.duration-1)
	case "right":
		c.poll.duration = min(len(api.PollDurations)-1, c.poll.duration+1)
	case "ctrl+x":
		c.poll = nil
		m.attachmentsChanged()
		return m.focusComposer()
	}
	m.invalidatePreview()
	return nil
}

// composerPaste attaches dropped image files and sends any other paste to the focused field.
func (m *AppModel) composerPaste(msg tea.PasteMsg) tea.Cmd {
	c := &m.composer
	switch {
	case c.owner != m.account.user.ID:
		return nil
	case c.browser != nil:
		m.filterBrowser(strings.Join(strings.Fields(msg.Content), " "))
		return nil
	case c.editing != "":
		var cmd tea.Cmd
		c.field, cmd = c.field.Update(msg)
		return cmd
	}
	if paths, ok := api.DroppedPaths(msg.Content); ok {
		m.attachImages(paths)
		return nil
	}
	if c.focus != 0 {
		return nil
	}
	return m.updateComposerInput(msg)
}

// composerStripRows keeps at least three editor rows. A short composer shows only the focused attachment and a
// count of the rest.
func (m AppModel) composerStripRows() int {
	c := m.composer
	room := m.composerHeight() - m.composerContextRows()
	count := c.attachmentCount()
	switch {
	case c.previewing, c.owner != m.account.user.ID:
		return 0
	case m.stripThumbnails() && stripThumbRows+2 <= room-6:
		return stripThumbRows + 2
	case count == 0 && room >= 8:
		return 1
	case count == 0:
		return 0
	case count+1 <= room-6:
		return count + 1
	}
	return 2
}

func (m AppModel) stripView(width int, p palette) string {
	c := m.composer
	rows := m.composerStripRows()
	if rows == 0 {
		return ""
	}
	focused := c.focus == composerStrip
	if c.attachmentCount() == 0 {
		hint := "Ctrl+O or drop images · Ctrl+L poll"
		if reason := c.pollBlocked(); reason != "" {
			hint = "Ctrl+O or drop images · Poll unavailable: " + strings.TrimSuffix(reason, ".")
		}
		return text(p.muted).Render(fit(hint, width))
	}
	thumbnails := rows == stripThumbRows+2 && m.stripThumbnails()
	var header string
	var items []string
	if c.poll != nil {
		header = "Poll · ‹ " + pollDurationLabels[c.poll.duration] + " › · Enter edit · Ctrl+N add · Del remove · Ctrl+X drop poll"
		for i, option := range c.poll.options {
			label := safeText(option)
			if strings.TrimSpace(label) == "" {
				label = text(p.muted).Render("empty")
			}
			items = append(items, fmt.Sprintf("%d. %s", i+1, label))
		}
	} else {
		header = fmt.Sprintf("Images %d/%d · Enter describe · Del remove · Alt+↑/↓ move", len(c.images), api.MaxImages)
		for i, image := range c.images {
			description := "No description"
			if strings.TrimSpace(image.Description) != "" {
				description = safeText(image.Description)
			}
			name := safeText(image.Name)
			if thumbnails {
				name = fmt.Sprintf("Image %d", i+1)
			}
			items = append(items, fmt.Sprintf("%s · %d×%d · %s", name, image.Width, image.Height, description))
		}
	}
	if !focused {
		header = strings.SplitN(header, " · Enter", 2)[0] + " · Tab to edit"
	}

	line := func(i int) string {
		marker := "  "
		if focused && i == c.strip {
			marker = "› "
		}
		if c.editing != "" && i == c.strip {
			prefix := ""
			if c.poll != nil {
				prefix = fmt.Sprintf("%d. ", i+1)
			}
			return text(p.blue).Render(marker) + prefix + c.field.View()
		}
		style := text(p.ink)
		if focused && i == c.strip {
			style = style.Bold(true).Background(lipgloss.Color(p.soft))
		}
		return style.Render(fit(marker+items[i], width))
	}
	if thumbnails {
		return strings.Join(append(append([]string{text(p.muted).Render(fit(header, width))}, m.thumbnailRows(width, p)...), line(c.strip)), "\n")
	}
	if rows < len(items)+1 {
		more := fmt.Sprintf("%d more · %s", len(items)-1, header)
		return line(c.strip) + "\n" + text(p.muted).Render(fit(more, width))
	}
	out := []string{text(p.muted).Render(fit(header, width))}
	for i := range items {
		out = append(out, line(i))
	}
	return strings.Join(out, "\n")
}

// Attachment strip thumbnails fit this many cells, so three images and their gaps fit a narrow composer.
const (
	stripThumbCols = 10
	stripThumbRows = 3
)

// stripThumbnails reports a strip that draws images instead of listing their names, which needs terminal graphics.
func (m AppModel) stripThumbnails() bool {
	return m.images != nil && m.images.Enabled && len(m.composer.images) > 0 && m.composer.poll == nil
}

func stripThumbnail(file api.ImageFile) graphics.Request {
	cols, rows := graphics.Fit(file.Width, file.Height, stripThumbCols, stripThumbRows)
	return graphics.Request{Path: file.Path, Cols: cols, Rows: rows}
}

// stripThumbnailRequests lists the strip's thumbnails while it draws them.
func (m AppModel) stripThumbnailRequests() []graphics.Request {
	c := m.composer
	if !c.open || c.previewing || c.browser != nil || m.composerStripRows() != stripThumbRows+2 || !m.stripThumbnails() {
		return nil
	}
	var requests []graphics.Request
	for _, file := range c.images {
		requests = append(requests, stripThumbnail(file))
	}
	return requests
}

// thumbnailRows draws the images side by side. A bar marks the focused one, and a numbered box stands in until it loads.
func (m AppModel) thumbnailRows(width int, p palette) []string {
	c := m.composer
	rows := make([]string, stripThumbRows)
	for row := range rows {
		var cells []string
		for i, file := range c.images {
			req := stripThumbnail(file)
			marker := " "
			if c.focus == composerStrip && i == c.strip {
				marker = text(p.blue).Render("▌")
			}
			cell := ""
			if row < req.Rows {
				cell = m.images.Row(req, row)
				if cell == "" {
					label := ""
					if row == 0 {
						label = fmt.Sprint(i + 1)
					}
					cell = withBackground(label+strings.Repeat(" ", req.Cols-len(label)), p.soft)
				}
			}
			cells = append(cells, marker+cell+strings.Repeat(" ", stripThumbCols-min(stripThumbCols, ansi.StringWidth(cell))))
		}
		rows[row] = ansi.Truncate(strings.Join(cells, " "), width, "")
	}
	return rows
}

// sentence turns a validation error into a status line.
func sentence(message string) string {
	if message == "" {
		return message
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if !strings.HasSuffix(message, ".") {
		message += "."
	}
	return message
}

// attachmentPreview draws the draft's poll or images below the server preview, which only renders the body.
func (m AppModel) attachmentPreview(width int, p palette) []string {
	c := m.composer
	if c.poll != nil {
		poll := &api.Poll{ID: "draft"}
		for i, option := range c.poll.options {
			poll.Options = append(poll.Options, api.PollOption{ID: fmt.Sprint(i + 1), Label: strings.TrimSpace(option)})
		}
		lines := pollLines(poll, width, p, m.now, -1)
		status := "Runs " + pollDurationLabels[c.poll.duration] + " once approved"
		return append([]string{""}, append(lines[:len(lines)-1], fit("  "+text(p.muted).Render(status), width))...)
	}
	if len(c.images) == 0 {
		return nil
	}
	images, requests := previewRequests(c.images, width)
	lines := pictureLines(images, requests, p, m.images)
	if len(lines) == 0 {
		for _, image := range images {
			label := "▧ Image"
			if image.Description != "" {
				label += " · " + safeText(image.Description)
			}
			lines = append(lines, fit(text(p.muted).Render(label), width))
		}
	}
	return append([]string{""}, lines...)
}
