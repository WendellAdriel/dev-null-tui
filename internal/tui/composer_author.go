package tui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// authorPicker chooses who a commit is posted as: from the account's organizations, or with typed set, a handle
// checked against its organization page, for servers that don't list the organizations yet.
type authorPicker struct {
	index    int
	typed    bool
	field    textinput.Model
	checking bool
	// pending is the handle being checked, so a reply for an earlier handle is ignored.
	pending    string
	status     string
	generation int
}

type authorCheckResult struct {
	session, generation int
	handle              string
	page                api.OrganizationPage
	err                 error
}

func (m *AppModel) openAuthorPicker() tea.Cmd {
	c := &m.composer
	if c.previewing || c.sending || c.owner != m.account.user.ID {
		return nil
	}
	u := m.account.user
	if u.Organizations == nil {
		field := textinput.New()
		field.Prompt, field.Placeholder = "@", "organization handle"
		if c.author != nil {
			field.SetValue(c.author.Handle)
		}
		c.picker = &authorPicker{typed: true, field: field, generation: m.composer.generation}
		c.input.Blur()
		return c.picker.field.Focus()
	}
	if u.CanPostAsOrganization != nil && !*u.CanPostAsOrganization {
		c.status = "Posting as an organization isn't available yet."
		return nil
	}
	if len(*u.Organizations) == 0 {
		c.status = "You aren't a member of any organization on /dev/null."
		return nil
	}
	index := 0
	for i, organization := range *u.Organizations {
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
	if picker.typed {
		if pressed == "enter" {
			return m.checkTypedAuthor()
		}
		if picker.checking {
			return nil
		}
		var cmd tea.Cmd
		picker.field, cmd = picker.field.Update(msg)
		picker.status = ""
		return cmd
	}
	choices := len(*m.account.user.Organizations) + 1
	picker.index = min(picker.index, choices-1)
	switch pressed {
	case "j", "down", "tab":
		picker.index = (picker.index + 1) % choices
	case "k", "up", "shift+tab":
		picker.index = (picker.index + choices - 1) % choices
	case "enter":
		c.author = nil
		if picker.index > 0 {
			organization := (*m.account.user.Organizations)[picker.index-1]
			c.author = &organization
		}
		return m.closeAuthorPicker()
	}
	return nil
}

func (m *AppModel) checkTypedAuthor() tea.Cmd {
	c := &m.composer
	picker := c.picker
	handle := strings.TrimPrefix(strings.TrimSpace(picker.field.Value()), "@")
	if handle == "" {
		c.author = nil
		return m.closeAuthorPicker()
	}
	if picker.checking || m.organizationCalls.load == nil {
		return nil
	}
	if !api.ValidHandle(handle) {
		picker.status = "That isn't a valid handle."
		return nil
	}
	picker.checking, picker.pending, picker.status = true, handle, "Checking @"+handle+"..."
	session, generation, load, base := m.session, picker.generation, m.organizationCalls.load, m.account.life.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		page, err := load(ctx, handle)
		return authorCheckResult{session: session, generation: generation, handle: handle, page: page, err: err}
	})
}

func (m *AppModel) acceptAuthorCheck(msg authorCheckResult) tea.Cmd {
	c := &m.composer
	picker := c.picker
	if msg.session != m.session || picker == nil || !picker.typed || msg.generation != picker.generation || msg.handle != picker.pending {
		return nil
	}
	picker.checking = false
	var failure *auth.APIError
	switch {
	case msg.err != nil && m.revoked(msg.err):
		return nil
	case errors.As(msg.err, &failure) && failure.Status == http.StatusNotFound:
		picker.status = "@" + safeText(msg.handle) + " isn't an organization on /dev/null."
	case msg.err != nil:
		picker.status = "Couldn't check @" + safeText(msg.handle) + ": " + safeText(msg.err.Error()) + "."
	case msg.page.SettingsURL == nil:
		// The page links its settings only for its members, who are the ones allowed to post as it.
		picker.status = "You can only post as an organization you are a member of."
	default:
		o := msg.page.Organization
		c.author = &auth.Organization{Handle: o.Handle, Name: o.Name, AvatarURL: o.AvatarURL, Lane: o.Lane}
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
	if picker.typed {
		rows = append(rows, picker.field.View(), "")
		for _, line := range strings.Split(ansi.Wrap("Type the handle of an organization you're a member of, or leave it empty to post as yourself.", width, ""), "\n") {
			rows = append(rows, text(p.muted).Render(line))
		}
		if picker.status != "" {
			rows = append(rows, "", text(p.warning).Render(fit(picker.status, width)))
		}
		hint, areas := hintRow([]statusPart{statusChip("Choose", "enter"), closeChip("Back")}, width, p)
		for i := range areas {
			areas[i].y = len(rows) + 1
		}
		rows = append(rows, "", hint)
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n")), areas
	}
	u := m.account.user
	choices := []string{text(laneColor(u.Lane, p)).Render("●") + " " + text(p.ink).Render(safeText(u.Name)) + " " + text(p.muted).Render("@"+safeText(u.Handle))}
	for _, o := range *u.Organizations {
		name := safeText(o.Name)
		if name == "" {
			name = safeText(o.Handle)
		}
		choices = append(choices, text(laneColor(o.Lane, p)).Render("■")+" "+text(p.ink).Render(name)+" "+text(p.muted).Render("@"+safeText(o.Handle)))
	}
	for i, choice := range choices {
		mark := "  "
		if (i == 0 && m.composer.author == nil) || (i > 0 && m.composer.author != nil && (*u.Organizations)[i-1].Handle == m.composer.author.Handle) {
			mark = text(p.blue).Render("✓ ")
		}
		row := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(mark + choice)
		if i == picker.index {
			row = withBackground(text(p.blue).Render("▌")+ansi.Cut(row, 1, width), p.wash)
		}
		rows = append(rows, row)
	}
	hint, areas := hintRow([]statusPart{hintChip("Move", "j/k", "j"), statusChip("Choose", "enter"), closeChip("Back")}, width, p)
	for i := range areas {
		areas[i].y = len(rows) + 1
	}
	rows = append(rows, "", hint)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n")), areas
}
