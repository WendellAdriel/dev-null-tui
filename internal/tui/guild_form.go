package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// The form's focus order: the three fields, the icon, Remove icon on an edit, then the footer.
const (
	formName = iota
	formDescription
	formEmoji
	formIcon
	formRemoveIcon
	formCancel
	formSave
)

// maxGuildIcon is the server's icon limit, smaller than a commit image's.
const maxGuildIcon = 2 << 20

// guildForm creates a guild, or edits the one in editing, laid out like Settings > Profile.
type guildForm struct {
	editing                  *api.Guild
	name, description, emoji textinput.Model
	// emojiValue is the chosen emoji. Typing in the emoji field searches, and Enter picks the highlighted match.
	emojiValue string
	choice     int
	icon       *api.ImageFile
	removeIcon bool
	browser    *fileBrowser
	focus      int
	busy       bool
	status     string
	generation int
}

type guildSavedResult struct {
	session, generation int
	editing             string
	saved               api.GuildSaved
	err                 error
}

func (m *AppModel) beginGuildReport(g api.Guild) tea.Cmd {
	cmd := m.beginSettingsDialog("guild-report", g.Handle, "")
	m.settings.input.Placeholder = "What looks wrong?"
	return cmd
}

func newFormInput(placeholder string, limit int) textinput.Model {
	input := textinput.New()
	input.SetVirtualCursor(true)
	input.Placeholder = placeholder
	input.CharLimit = limit
	return input
}

// openGuildForm opens the create form, or with a guild the edit form for its moderator.
func (m *AppModel) openGuildForm(editing *api.Guild) tea.Cmd {
	if m.guildCalls.create == nil {
		return nil
	}
	f := &guildForm{editing: editing, generation: m.guildFormGeneration + 1}
	m.guildFormGeneration++
	f.name = newFormInput("Guild display name", 60)
	f.description = newFormInput("What the guild is about", 500)
	f.emoji = newFormInput("Search emoji, such as coffee", 40)
	if editing != nil {
		f.name.SetValue(editing.Name)
		f.description.SetValue(editing.Description)
		f.emojiValue = editing.Emoji
	}
	m.guildForm = f
	m.sizeGuildForm()
	return f.name.Focus()
}

func (m *AppModel) sizeGuildForm() {
	if f := m.guildForm; f != nil {
		width := max(1, min(68, m.account.width-12)-14)
		f.name.SetWidth(width)
		f.description.SetWidth(width)
		f.emoji.SetWidth(width)
	}
}

func (f *guildForm) inputs() []*textinput.Model {
	return []*textinput.Model{&f.name, &f.description, &f.emoji}
}

func (f *guildForm) setFocus(focus int) tea.Cmd {
	if focus == formRemoveIcon && (f.editing == nil || f.editing.IconURL == "") {
		if focus > f.focus {
			focus = formCancel
		} else {
			focus = formIcon
		}
	}
	f.focus = focus
	var cmd tea.Cmd
	for i, input := range f.inputs() {
		if i == focus {
			cmd = input.Focus()
		} else {
			input.Blur()
		}
	}
	return cmd
}

func (f *guildForm) emojiMatches() []emojiSuggestion {
	query := strings.TrimSpace(f.emoji.Value())
	if query == "" {
		return nil
	}
	return searchEmoji(query)
}

func (m *AppModel) guildFormKey(msg tea.KeyPressMsg) tea.Cmd {
	f := m.guildForm
	if f.browser != nil {
		return m.browserKey(msg)
	}
	if f.busy {
		return nil
	}
	pressed := msg.String()
	switch pressed {
	case "esc":
		m.guildForm = nil
		return nil
	case "ctrl+s":
		return m.submitGuildForm()
	case "tab", "down":
		if pressed == "down" && f.focus == formEmoji && len(f.emojiMatches()) > 0 {
			f.choice = min(f.choice+1, len(f.emojiMatches())-1)
			return nil
		}
		return f.setFocus((f.focus + 1) % (formSave + 1))
	case "shift+tab", "up":
		if pressed == "up" && f.focus == formEmoji && len(f.emojiMatches()) > 0 {
			f.choice = max(0, f.choice-1)
			return nil
		}
		return f.setFocus((f.focus + formSave) % (formSave + 1))
	case "enter", "space":
		switch f.focus {
		case formEmoji:
			if matches := f.emojiMatches(); pressed == "enter" && len(matches) > 0 {
				f.emojiValue = matches[min(f.choice, len(matches)-1)].value
				f.emoji.SetValue("")
				f.choice = 0
				return nil
			}
		case formIcon:
			return m.openFormBrowser()
		case formRemoveIcon:
			f.removeIcon = !f.removeIcon
			if f.removeIcon {
				f.icon = nil
			}
			return nil
		case formCancel:
			m.guildForm = nil
			return nil
		case formSave:
			return m.submitGuildForm()
		}
		if pressed == "enter" {
			return f.setFocus(f.focus + 1)
		}
	case "backspace":
		if f.focus == formIcon {
			f.icon = nil
			return nil
		}
	}
	if f.focus > formEmoji {
		return nil
	}
	input := f.inputs()[f.focus]
	before := input.Value()
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	if f.focus == formEmoji && input.Value() != before {
		f.choice = 0
	}
	return cmd
}

func (m *AppModel) openFormBrowser() tea.Cmd {
	b := &fileBrowser{}
	if err := b.load(m.browserStart()); err != nil {
		m.guildForm.status = "That folder cannot be read."
		return nil
	}
	m.guildForm.browser = b
	return nil
}

// pickGuildIcon checks a picked file against the server's icon rules before it is kept.
func (m *AppModel) pickGuildIcon(path string) string {
	image, err := api.ReadImage(path)
	if err != nil {
		return err.Error()
	}
	if len(image.Data) > maxGuildIcon {
		return filepath.Base(path) + " is larger than 2 MB"
	}
	m.guildForm.icon, m.guildForm.removeIcon = &image, false
	return ""
}

func (m *AppModel) submitGuildForm() tea.Cmd {
	f := m.guildForm
	form := api.GuildForm{Name: strings.TrimSpace(f.name.Value()), Description: strings.TrimSpace(f.description.Value()), Emoji: f.emojiValue, RemoveIcon: f.removeIcon}
	switch {
	case form.Name == "" || form.Description == "":
		f.status = "Give the guild a display name and a description."
		return nil
	case api.GuildHashtag(form.Name) == "" || !api.ValidGuildHandle(api.GuildHashtag(form.Name)):
		f.status = "Use a display name whose hashtag starts with a letter and contains only letters, numbers and underscores."
		return nil
	case form.Emoji == "":
		f.status = "Choose an emoji. An uploaded icon shows in its place."
		return nil
	}
	if f.icon != nil {
		form.Icon = &auth.Upload{Name: f.icon.Name, Type: f.icon.Type, Data: f.icon.Data}
	}
	f.busy, f.status = true, ""
	session, generation, calls := m.session, f.generation, m.guildCalls
	editing := ""
	if f.editing != nil {
		editing = f.editing.Handle
	}
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, auth.UploadTimeout)
		defer cancel()
		var saved api.GuildSaved
		var err error
		if editing != "" {
			saved, err = calls.update(ctx, editing, form)
		} else {
			saved, err = calls.create(ctx, form)
		}
		return guildSavedResult{session: session, generation: generation, editing: editing, saved: saved, err: err}
	})
}

func (m *AppModel) acceptGuildSaved(msg guildSavedResult) tea.Cmd {
	f := m.guildForm
	if f == nil || msg.session != m.session || msg.generation != f.generation {
		return nil
	}
	f.busy = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		var failure *auth.APIError
		if errors.As(msg.err, &failure) && len(failure.FieldErrors) > 0 {
			f.status = strings.Join(failure.FieldErrors, " ")
		} else {
			f.status = m.ghostly(msg.err, safeText(msg.err.Error()))
		}
		return nil
	}
	m.guildForm = nil
	if msg.saved.Guild == nil {
		return m.notify("info", "Waiting for review", msg.saved.Message)
	}
	g := *msg.saved.Guild
	if msg.editing == "" {
		return tea.Batch(m.openGuild(g.Handle), m.refreshJoinedGuilds(), m.notify("success", "Guild created", "#"+g.Handle))
	}
	for i := range m.profiles {
		if s := &m.profiles[i]; s.guild && s.handle == g.Handle {
			s.board.Guild = g
			m.renderProfile(s)
		}
	}
	return tea.Batch(m.refreshJoinedGuilds(), m.notify("success", "Saved.", "#"+g.Handle))
}

// guildFormView draws the form, or the file browser while it picks the icon.
func (m AppModel) guildFormView(p palette) (string, uiRect, []hitArea) {
	f := m.guildForm
	r := centeredRect(m.account.width, m.account.height, 72, 26)
	w := max(1, r.w-4)
	if f.browser != nil {
		body, _ := m.browserView(w, max(1, r.h-6), p)
		footer, areas := modalFooter([]modalAction{{"Back", "Esc", "esc"}, {"Choose", "Enter", "enter"}}, w, 0, p)
		return panel("Choose an icon", body, footer, r, p, p.blue), r, m.placeFooter(areas, footer, r)
	}
	title, about := "Create a guild", "Create a public group. Its name determines the hashtag that brings commits into the guild."
	if f.editing != nil {
		title, about = "Edit guild", "As the moderator, you can change the display name, icon and description."
	}
	label := func(field int, name string) string {
		if f.focus == field {
			return text(p.blue).Bold(true).Render("› " + name)
		}
		return text(p.ink).Bold(true).Render("  " + name)
	}
	var lines []string
	lines = append(lines, strings.Split(ansi.Wrap(about, w, ""), "\n")...)
	hashtag := api.GuildHashtag(f.name.Value())
	preview := text(p.muted).Render("Your hashtag appears here")
	if f.editing != nil {
		preview = text(p.blue).Render("#" + safeText(f.editing.Handle))
	} else if hashtag != "" {
		preview = text(p.blue).Render("#" + safeText(hashtag))
	}
	lines = append(lines, "", "  "+preview, "")
	nameHint := "Spaces and decoration are okay. Matching hashtags link to this guild."
	if f.editing != nil {
		nameHint = "Match #" + safeText(f.editing.Handle) + "'s letters, numbers and underscores. Spaces and decoration are okay."
	}
	lines = append(lines, label(formName, "Guild display name"), "  "+f.name.View(), "  "+text(p.muted).Render(fit(nameHint, w-2)))
	lines = append(lines, label(formDescription, "Description"), "  "+f.description.View())
	emoji := text(p.muted).Render("none chosen")
	if f.emojiValue != "" {
		emoji = f.emojiValue
	}
	lines = append(lines, label(formEmoji, "Emoji")+"  "+emoji, "  "+f.emoji.View())
	if f.focus == formEmoji {
		var row []string
		for i, match := range f.emojiMatches() {
			item := match.value + " " + match.name
			if i == f.choice {
				item = text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render(item)
			} else {
				item = text(p.muted).Render(item)
			}
			row = append(row, item)
		}
		if len(row) > 0 {
			lines = append(lines, "  "+fit(strings.Join(row, "  "), w-2))
		}
	}
	icon := text(p.muted).Render("Enter to choose an image")
	switch {
	case f.icon != nil:
		icon = text(p.ink).Render(safeText(filepath.Base(f.icon.Path))) + text(p.muted).Render("  backspace removes it")
	case f.removeIcon:
		icon = text(p.warning).Render("The icon is removed when you save")
	case f.editing != nil && f.editing.IconURL != "":
		icon = text(p.muted).Render("Current icon kept. Enter to replace it")
	}
	lines = append(lines, label(formIcon, "Icon")+"  "+icon, "  "+text(p.muted).Render(fit("JPEG, PNG or WebP, up to 2 MB. An upload replaces the emoji.", w-2)))
	if f.editing != nil && f.editing.IconURL != "" {
		box := "[ ]"
		if f.removeIcon {
			box = "[x]"
		}
		lines = append(lines, label(formRemoveIcon, box+" Remove icon"))
	}
	if f.status != "" {
		// One line per row, so scrollToFocus counts the rows the status takes.
		lines = append(lines, "")
		for _, line := range strings.Split(ansi.Wrap(safeText(f.status), w, ""), "\n") {
			lines = append(lines, text(p.warning).Render(line))
		}
	}
	if f.busy {
		lines = append(lines, "", "Working...")
	}
	save := "Create guild"
	if f.editing != nil {
		save = "Save changes"
	}
	focus := 0
	if f.focus >= formCancel {
		focus = f.focus - formCancel + 1
	}
	footer, areas := modalFooter([]modalAction{{"Cancel", "Esc", "esc"}, {save, "Ctrl+S", "ctrl+s"}}, w, focus, p)
	lines = scrollToFocus(lines, max(1, r.h-5-lipgloss.Height(footer)), f.status != "" || f.busy)
	return panel(title, strings.Join(lines, "\n"), footer, r, p, p.blue), r, m.placeFooter(areas, footer, r)
}

// scrollToFocus drops lines from the top of a form taller than its room, so the focused field and the two lines
// under it stay in view. A status or progress line at the end wins, since it answers the last submit.
func scrollToFocus(lines []string, room int, statusLast bool) []string {
	if len(lines) <= room {
		return lines
	}
	anchor := len(lines)
	if !statusLast {
		for i, line := range lines {
			if strings.HasPrefix(ansi.Strip(line), "› ") {
				anchor = min(len(lines), i+3)
				break
			}
		}
	}
	return lines[max(0, anchor-room):]
}

func (m AppModel) placeFooter(areas []hitArea, footer string, r uiRect) []hitArea {
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	return areas
}

func (m AppModel) guildFormMouse(msg tea.MouseClickMsg) (AppModel, tea.Cmd) {
	_, r, areas := m.guildFormView(m.account.colors())
	if (uiRect{r.x + r.w - 3, r.y + 1, 1, 1}).contains(msg.X, msg.Y) {
		m.guildForm = nil
		return m, nil
	}
	for _, area := range areas {
		if area.contains(msg.X, msg.Y) {
			return m.mouseKey(area.action)
		}
	}
	return m, nil
}

// guildFormInput passes pastes and cursor blinks to the focused field.
func (m *AppModel) guildFormInput(msg tea.Msg) tea.Cmd {
	f := m.guildForm
	if f.focus > formEmoji || f.browser != nil || f.busy {
		return nil
	}
	input := f.inputs()[f.focus]
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	return cmd
}
