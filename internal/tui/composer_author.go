package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// authorPicker chooses who a commit is posted as: you, or one of the account's organizations.
type authorPicker struct {
	index int
}

// canPostAs reports whether this draft could come from one of your organizations.
func (m AppModel) canPostAs() bool {
	c := m.composer
	return !c.previewing && !c.sending && c.owner == m.account.user.ID && !c.issue() && len(m.account.user.Organizations) > 0
}

func (m *AppModel) openAuthorPicker() tea.Cmd {
	c := &m.composer
	if c.previewing || c.sending || c.owner != m.account.user.ID || c.issue() {
		return nil
	}
	u := m.account.user
	if len(u.Organizations) == 0 {
		c.status = "You aren't a member of any organization on /dev/null."
		return nil
	}
	index := 0
	for i, organization := range u.Organizations {
		if c.author != nil && organization.Handle == c.author.Handle {
			index = i + 1
		}
	}
	c.picker = &authorPicker{index: index}
	c.input.Blur()
	return nil
}

func (m *AppModel) closeAuthorPicker() tea.Cmd {
	m.composer.picker = nil
	return m.focusComposer()
}

func (m *AppModel) authorPickerKey(msg tea.KeyPressMsg) tea.Cmd {
	c := &m.composer
	picker := c.picker
	pressed := msg.String()
	if pressed == "esc" {
		return m.closeAuthorPicker()
	}
	choices := len(m.account.user.Organizations) + 1
	picker.index = min(picker.index, choices-1)
	switch pressed {
	case "j", "down", "tab":
		picker.index = (picker.index + 1) % choices
	case "k", "up", "shift+tab":
		picker.index = (picker.index + choices - 1) % choices
	case "enter":
		c.author = nil
		if picker.index > 0 {
			organization := m.account.user.Organizations[picker.index-1]
			c.author = &organization
		}
		return m.closeAuthorPicker()
	}
	return nil
}

// composerIdentity is the composer title's author: you, or the organization chosen to post as.
func (m AppModel) composerIdentity(p palette) string {
	if a := m.composer.author; a != nil {
		name := safeText(a.Name)
		if name == "" {
			name = safeText(a.Handle)
		}
		return text(laneColor(a.Lane, p)).Render("■") + " " + text(p.ink).Render(name) + " " + text(p.muted).Render("@"+safeText(a.Handle))
	}
	return text(p.muted).Render("@" + safeText(m.account.user.Handle))
}

// authorPickerView draws Post as. Its hit areas are the hint chips.
func (m AppModel) authorPickerView(width, height int, p palette) (string, []hitArea) {
	picker := m.composer.picker
	rows := []string{text(p.ink).Bold(true).Render("Post as"), ""}
	u := m.account.user
	choices := []string{text(laneColor(u.Lane, p)).Render("●") + " " + text(p.ink).Render(safeText(u.Name)) + " " + text(p.muted).Render("@"+safeText(u.Handle))}
	for _, o := range u.Organizations {
		name := safeText(o.Name)
		if name == "" {
			name = safeText(o.Handle)
		}
		choices = append(choices, text(laneColor(o.Lane, p)).Render("■")+" "+text(p.ink).Render(name)+" "+text(p.muted).Render("@"+safeText(o.Handle)))
	}
	for i, choice := range choices {
		mark := "  "
		if (i == 0 && m.composer.author == nil) || (i > 0 && m.composer.author != nil && u.Organizations[i-1].Handle == m.composer.author.Handle) {
			mark = text(p.blue).Render("✓ ")
		}
		row := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(mark + choice)
		if i == picker.index {
			row = withBackground(text(p.blue).Render("▌")+ansi.Cut(row, 1, width), p.wash)
		}
		rows = append(rows, row)
	}
	hint, areas := hintRow([]statusPart{hintChip("Move", "↑/↓", "down"), statusChip("Choose", "enter"), closeChip("Back")}, width, p)
	for i := range areas {
		areas[i].y = len(rows) + 1
	}
	rows = append(rows, "", hint)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n")), areas
}
