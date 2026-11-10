package tui

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// Reactions follow the web's resources/js/lib/reactions.ts: three quick reactions that follow what the user reacts
// with most, and a picker over the whole catalog.

var defaultQuickReactions = []string{"😂", "✅", "❤️"}

const (
	// reactionReplaceMargin is how many more uses an emoji needs than a quick reaction before it takes that slot.
	reactionReplaceMargin = 3
	maxOwnReactions       = 12
	pickerRows            = 8
)

// reactionMenu is the open reaction row for a message, or the picker when picking is set.
type reactionMenu struct {
	messageID int64
	focus     int
	picking   bool
	query     string
	index     int
}

type remoteReactionResult struct {
	thread    *remoteThreadState
	session   int
	messageID int64
	emoji     string
	on        bool
	reactions []api.Reaction
	err       error
}

// quickReactions starts from joy, check and heart. An emoji takes the weakest slot in place, only once it is used
// clearly more than that slot, so the row changes slowly.
func quickReactions(usage []api.EmojiUse) []string {
	uses := make(map[string]int, len(usage))
	for _, use := range usage {
		uses[use.Emoji] = use.Count
	}
	slots := slices.Clone(defaultQuickReactions)
	for _, use := range byUse(usage) {
		if slices.Contains(slots, use.Emoji) {
			continue
		}
		weakest := 0
		for i, slot := range slots {
			if uses[slot] <= uses[slots[weakest]] {
				weakest = i
			}
		}
		if use.Count >= uses[slots[weakest]]+reactionReplaceMargin {
			slots[weakest] = use.Emoji
		}
	}
	return slots
}

func byUse(usage []api.EmojiUse) []api.EmojiUse {
	sorted := slices.Clone(usage)
	slices.SortStableFunc(sorted, func(a, b api.EmojiUse) int { return b.Count - a.Count })
	return sorted
}

// withReactionUse counts a reaction the user just added or took back, without waiting for the server.
func withReactionUse(usage []api.EmojiUse, emoji string, change int) []api.EmojiUse {
	changed := make([]api.EmojiUse, 0, len(usage)+1)
	known := false
	for _, use := range usage {
		if use.Emoji == emoji {
			use.Count += change
			known = true
		}
		changed = append(changed, use)
	}
	if !known {
		changed = append(changed, api.EmojiUse{Emoji: emoji, Count: change})
	}
	return slices.DeleteFunc(changed, func(use api.EmojiUse) bool { return use.Count <= 0 })
}

// emojiNames is one shortcode per emoji, the shortest, for listing the catalog without its aliases.
var emojiNames = func() []emojiSuggestion {
	names := map[string]string{}
	for name, value := range emojiCatalog {
		if current, ok := names[value]; !ok || len(name) < len(current) || (len(name) == len(current) && name < current) {
			names[value] = name
		}
	}
	list := make([]emojiSuggestion, 0, len(names))
	for value, name := range names {
		list = append(list, emojiSuggestion{name, value})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
	return list
}()

func emojiName(value string) string {
	i := slices.IndexFunc(emojiNames, func(s emojiSuggestion) bool { return s.value == value })
	if i < 0 {
		return ""
	}
	return emojiNames[i].name
}

// pickerChoices lists what the user reacts with most, then the rest of the catalog by name. A query searches the
// shortcodes instead.
func (m AppModel) pickerChoices(query string) []emojiSuggestion {
	if query = strings.TrimSpace(query); query != "" {
		seen := map[string]bool{}
		var matches []emojiSuggestion
		for _, match := range searchEmoji(query) {
			if !seen[match.value] {
				matches, seen[match.value] = append(matches, match), true
			}
		}
		return matches
	}
	var first []emojiSuggestion
	seen := map[string]bool{}
	for _, use := range byUse(m.messaging.usage) {
		if name := emojiName(use.Emoji); name != "" && !seen[use.Emoji] {
			first, seen[use.Emoji] = append(first, emojiSuggestion{name, use.Emoji}), true
		}
	}
	rest := slices.DeleteFunc(slices.Clone(emojiNames), func(s emojiSuggestion) bool { return seen[s.value] })
	return append(first, rest...)
}

// reactable is a message the user can react to: a stored message, not an event, in a conversation open to them.
func (t *remoteThreadState) reactable(id int64) (*api.Message, bool) {
	if !t.canPush() {
		return nil, false
	}
	for i := range t.messages {
		if t.messages[i].ID == id && t.messages[i].Event == "" {
			return &t.messages[i], true
		}
	}
	return nil, false
}

func (m *AppModel) openReactions(id int64, picking bool) tea.Cmd {
	t := m.remotes.thread
	if _, ok := t.reactable(id); !ok {
		return nil
	}
	t.react = &reactionMenu{messageID: id, picking: picking}
	m.resizeThread()
	return m.askReactionUsage()
}

func (m *AppModel) closeReactions() {
	m.remotes.thread.react = nil
	m.resizeThread()
}

// activeReactions is the open reaction row or picker, which takes typed keys before any workspace shortcut.
func (m AppModel) activeReactions() *reactionMenu {
	t := m.remotes.thread
	if !m.connected || m.accountOpen || !m.remotesOnTop() || m.settings.open || t == nil || !m.remotes.focusThread || m.remotes.dialog != "" || m.help || m.whatsNew.open || m.upgradeVisible() || m.viewer.open || m.manualLink != "" || m.vote.commitID != "" {
		return nil
	}
	return t.react
}

func (m *AppModel) reactionKey(msg tea.KeyPressMsg) tea.Cmd {
	menu := m.remotes.thread.react
	pressed := msg.String()
	if menu.picking {
		return m.pickerKey(pressed, msg.Text)
	}
	quick := quickReactions(m.messaging.usage)
	switch pressed {
	case "esc":
		m.closeReactions()
	case "left", "h", "shift+tab":
		menu.focus = (menu.focus + len(quick)) % (len(quick) + 1)
	case "right", "l", "tab":
		menu.focus = (menu.focus + 1) % (len(quick) + 1)
	case "1", "2", "3":
		return m.chooseReaction(quick[pressed[0]-'1'])
	case "+", "4":
		menu.picking, menu.focus = true, 0
	case "enter":
		if menu.focus == len(quick) {
			menu.picking, menu.focus = true, 0
			return nil
		}
		return m.chooseReaction(quick[menu.focus])
	}
	return nil
}

func (m *AppModel) pickerKey(pressed, typed string) tea.Cmd {
	menu := m.remotes.thread.react
	choices := m.pickerChoices(menu.query)
	switch pressed {
	case "esc":
		m.closeReactions()
	case "down", "ctrl+n":
		menu.index = min(max(0, len(choices)-1), menu.index+1)
	case "up", "ctrl+p":
		menu.index = max(0, menu.index-1)
	case "pgdown":
		menu.index = min(max(0, len(choices)-1), menu.index+pickerRows)
	case "pgup":
		menu.index = max(0, menu.index-pickerRows)
	case "backspace":
		if runes := []rune(menu.query); len(runes) > 0 {
			menu.query, menu.index = string(runes[:len(runes)-1]), 0
		}
	case "enter":
		if menu.index < len(choices) {
			return m.chooseReaction(choices[menu.index].value)
		}
	default:
		if typed != "" && len([]rune(menu.query)) < 32 && !strings.ContainsAny(typed, " \t") {
			menu.query, menu.index = menu.query+typed, 0
		}
	}
	return nil
}

func (m *AppModel) chooseReaction(emoji string) tea.Cmd {
	id := m.remotes.thread.react.messageID
	m.closeReactions()
	return m.toggleReaction(id, emoji)
}

func ownReactions(reactions []api.Reaction, user string) int {
	count := 0
	for _, reaction := range reactions {
		if reaction.UserID == user {
			count++
		}
	}
	return count
}

// toggleReaction adds the user's reaction, or takes it back, at once. The server's answer replaces the message's
// reactions, and a refusal puts them back as they were.
func (m *AppModel) toggleReaction(id int64, emoji string) tea.Cmd {
	t := m.remotes.thread
	message, ok := t.reactable(id)
	if !ok || m.remoteCalls.react == nil {
		return nil
	}
	if t.reacting[id] {
		return nil
	}
	user := m.account.user.ID
	mine := slices.ContainsFunc(message.Reactions, func(r api.Reaction) bool { return r.UserID == user && r.Emoji == emoji })
	if !mine && ownReactions(message.Reactions, user) >= maxOwnReactions {
		t.composer.status = "You can leave up to 12 reactions on a message."
		return nil
	}
	change := 1
	if mine {
		message.Reactions = slices.DeleteFunc(slices.Clone(message.Reactions), func(r api.Reaction) bool { return r.UserID == user && r.Emoji == emoji })
		change = -1
	} else {
		message.Reactions = append(slices.Clone(message.Reactions), api.Reaction{UserID: user, Emoji: emoji})
	}
	m.messaging.usage = withReactionUse(m.messaging.usage, emoji, change)
	if t.reacting == nil {
		t.reacting = map[int64]bool{}
	}
	t.reacting[id] = true
	m.renderRemoteThread()
	session, react, life := m.session, m.remoteCalls.react, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		reactions, err := react(ctx, id, emoji, !mine)
		return remoteReactionResult{thread: t, session: session, messageID: id, emoji: emoji, on: !mine, reactions: reactions, err: err}
	})
}

func (m *AppModel) acceptReaction(msg remoteReactionResult) tea.Cmd {
	if msg.session != m.session || !m.connected {
		return nil
	}
	if msg.thread == nil || !msg.thread.reacting[msg.messageID] {
		return nil
	}
	delete(msg.thread.reacting, msg.messageID)
	if msg.err != nil && m.revoked(msg.err) {
		return nil
	}
	if msg.err != nil {
		change := -1
		if !msg.on {
			change = 1
		}
		m.messaging.usage = withReactionUse(m.messaging.usage, msg.emoji, change)
	}
	if t := m.remotes.thread; t != nil && t == msg.thread {
		for i := range t.messages {
			if t.messages[i].ID != msg.messageID {
				continue
			}
			switch {
			case msg.err == nil:
				t.messages[i].Reactions = msg.reactions
			case msg.on:
				t.messages[i].Reactions = slices.DeleteFunc(slices.Clone(t.messages[i].Reactions), func(r api.Reaction) bool { return r.UserID == m.account.user.ID && r.Emoji == msg.emoji })
			default:
				t.messages[i].Reactions = append(slices.Clone(t.messages[i].Reactions), api.Reaction{UserID: m.account.user.ID, Emoji: msg.emoji})
			}
		}
		m.renderRemoteThread()
	}
	if msg.err != nil {
		return m.notify("error", "Couldn't update the reaction", sentence(m.messageError(msg.err)))
	}
	return nil
}

// reactionRow is the quick reaction row that takes the composer's place while it is open.
func (m AppModel) reactionRow(width int, p palette) (string, []hitArea) {
	menu := m.remotes.thread.react
	quick := quickReactions(m.messaging.usage)
	var areas []hitArea
	line, x := " "+text(p.muted).Render("React "), 7
	for i, emoji := range append(slices.Clone(quick), "+") {
		label := emoji
		if i == len(quick) {
			label = "+ more"
		}
		chip := actionChip(label, string(rune('1'+i)), p)
		if i == menu.focus {
			chip = withBackground(chip+" ", p.soft)
		} else {
			chip += " "
		}
		areas = append(areas, hitArea{uiRect{x, 0, lipgloss.Width(chip), 1}, "react", i})
		line += chip + " "
		x += lipgloss.Width(chip) + 1
	}
	cancel, cancelAreas := hintRow([]statusPart{closeChip("Cancel")}, width, p)
	for _, area := range cancelAreas {
		area.x += width - lipgloss.Width(cancel) - 1
		areas = append(areas, area)
	}
	return split(line, cancel+" ", width), areas
}

func (m AppModel) reactionModal(p palette) (string, uiRect, []hitArea) {
	t := m.remotes.thread
	menu := t.react
	choices := m.pickerChoices(menu.query)
	rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 44, pickerRows+7)
	width := max(1, rect.w-4)
	rows := max(1, min(pickerRows, rect.h-7))
	var mine []api.Reaction
	if message, ok := t.reactable(menu.messageID); ok {
		mine = message.Reactions
	}
	lines := []string{text(p.ink).Render("/ "+safeText(menu.query)) + text(p.blue).Render("▏"), ""}
	start := max(0, min(menu.index-rows/2, len(choices)-rows))
	var areas []hitArea
	for i := start; i < min(len(choices), start+rows); i++ {
		choice := choices[i]
		marker := " "
		if slices.ContainsFunc(mine, func(r api.Reaction) bool { return r.UserID == m.account.user.ID && r.Emoji == choice.value }) {
			marker = text(p.blue).Render("✓")
		}
		row := " " + choice.value + "  " + text(p.muted).Render(":"+choice.name+":")
		row = split(row, marker+" ", width)
		if i == menu.index {
			row = withBackground(fit(row, width), p.soft)
		}
		areas = append(areas, hitArea{uiRect{rect.x + 2, rect.y + 3 + len(lines), width, 1}, "pick", i})
		lines = append(lines, row)
	}
	if len(choices) == 0 {
		lines = append(lines, text(p.muted).Render(" fatal: no emoji matches"))
	}
	footer, footerAreas := hintRow([]statusPart{hintText("Type to search"), statusChip("React", "enter"), closeChip("Close")}, width, p)
	areas = append(append(areas, panelFooterAreas(footerAreas, footer, rect)...), hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
	return panel("React", strings.Join(lines, "\n"), footer, rect, p, p.blue), rect, areas
}

func (m AppModel) reactionModalMouse(x, y int) (AppModel, tea.Cmd) {
	menu := m.remotes.thread.react
	_, rect, areas := m.reactionModal(m.account.colors())
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if area.action == "esc" {
			m.closeReactions()
			return m, nil
		}
		if pressed, ok := keyAction(area.action); ok {
			return m.mouseKey(pressed)
		}
		choices := m.pickerChoices(menu.query)
		cmd := m.chooseReaction(choices[area.index].value)
		return m, cmd
	}
	if !rect.contains(x, y) {
		m.closeReactions()
	}
	return m, nil
}
