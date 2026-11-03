package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// The conversation menu and its dialogs follow the web's resources/js/components/chat/conversation-actions.tsx.

type remoteActionResult struct {
	session      int
	kind         string
	conversation api.Conversation
	value        bool
	duration     api.MuteDuration
	at           *time.Time
	err          error
}

type menuItem struct{ key, label string }

var muteChoices = []struct {
	duration api.MuteDuration
	label    string
}{{api.MuteHour, "For one hour"}, {api.MuteTomorrow, "Until tomorrow"}, {api.MuteWeek, "For one week"}, {api.MuteForever, "Forever"}}

// conversationTitle is the web's row title: the partner's @handle, or an org's name, else its other members' first
// names.
func (m AppModel) conversationTitle(c api.Conversation) string {
	if c.Kind != "org" {
		if c.Partner == nil {
			return "someone"
		}
		return "@" + safeText(c.Partner.Handle)
	}
	if c.Name != nil && strings.TrimSpace(*c.Name) != "" {
		return safeText(*c.Name)
	}
	var names []string
	for _, member := range c.Members {
		if member.User.ID != m.account.user.ID {
			names = append(names, firstName(member.User))
		}
	}
	if len(names) == 0 {
		return "Just you"
	}
	return strings.Join(names, ", ")
}

func firstName(u api.User) string {
	if fields := strings.Fields(safeText(u.Name)); len(fields) > 0 {
		return fields[0]
	}
	return "@" + safeText(u.Handle)
}

func (m AppModel) muted(c api.Conversation) bool {
	return c.MutedUntilAt != nil && c.MutedUntilAt.After(m.now)
}

func (m AppModel) stashed(c api.Conversation) bool {
	return c.IsArchived || m.remotes.tab == api.Stashed
}

func (m AppModel) conversationMenu(c api.Conversation) []menuItem {
	var items []menuItem
	if c.Kind != "org" {
		items = append(items, menuItem{"u", "View profile"})
	}
	items = append(items, menuItem{"P", map[bool]string{true: "Unpin", false: "Pin"}[c.PinnedAt != nil]})
	items = append(items, menuItem{"S", map[bool]string{true: "Pop", false: "Stash"}[m.stashed(c)]})
	items = append(items, menuItem{"M", map[bool]string{true: "Unmute", false: "Mute"}[m.muted(c)]})
	if c.Kind == "org" {
		return append(items, menuItem{"B", "Leave org"})
	}
	return append(items, menuItem{"B", "Block " + m.conversationTitle(c)})
}

// conversationActionKey runs the menu's shortcuts on the focused conversation.
func (m *AppModel) conversationActionKey(pressed string) (tea.Cmd, bool) {
	c, ok := m.focusedConversation()
	if !ok {
		return nil, false
	}
	switch pressed {
	case ".":
		m.remotes.dialog, m.remotes.dialogFocus, m.remotes.dialogTarget = "menu", 0, c
	case "u":
		if c.Kind == "org" || c.Partner == nil {
			return nil, false
		}
		return m.openProfile(c.Partner.Handle), true
	case "P":
		return m.startRemoteAction("pin", c, c.PinnedAt == nil, ""), true
	case "S":
		return m.startRemoteAction("stash", c, !m.stashed(c), ""), true
	case "M":
		if m.muted(c) {
			return m.startRemoteAction("mute", c, false, api.MuteOff), true
		}
		m.remotes.dialog, m.remotes.dialogFocus, m.remotes.dialogTarget = "mute", 0, c
	case "B":
		m.remotes.dialog, m.remotes.dialogFocus, m.remotes.dialogTarget = "block", 0, c
	default:
		return nil, false
	}
	return nil, true
}

func (m *AppModel) remotesDialogKey(pressed string) tea.Cmd {
	r := &m.remotes
	c := r.dialogTarget
	count := map[string]int{"menu": len(m.conversationMenu(c)), "mute": len(muteChoices), "block": 2}[r.dialog]
	switch pressed {
	case "esc":
		r.dialog = ""
		return nil
	case "j", "down", "tab", "right":
		r.dialogFocus = (r.dialogFocus + 1) % count
		return nil
	case "k", "up", "shift+tab", "left":
		r.dialogFocus = (r.dialogFocus + count - 1) % count
		return nil
	}
	switch r.dialog {
	case "menu":
		items := m.conversationMenu(c)
		key := pressed
		if pressed == "enter" {
			key = items[r.dialogFocus].key
		}
		for _, item := range items {
			if item.key == key {
				r.dialog = ""
				cmd, _ := m.conversationActionKey(key)
				return cmd
			}
		}
	case "mute":
		if pressed == "enter" {
			r.dialog = ""
			return m.startRemoteAction("mute", c, true, muteChoices[r.dialogFocus].duration)
		}
	case "block":
		if pressed == "enter" {
			r.dialog = ""
			if r.dialogFocus == 1 {
				kind := "block"
				if c.Kind == "org" {
					kind = "leave"
				}
				return m.startRemoteAction(kind, c, true, "")
			}
		}
	}
	return nil
}

func (m *AppModel) startRemoteAction(kind string, c api.Conversation, value bool, duration api.MuteDuration) tea.Cmd {
	calls, session, life, handle := m.remoteCalls, m.session, m.account.life, m.account.user.Handle
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		result := remoteActionResult{session: session, kind: kind, conversation: c, value: value, duration: duration}
		switch kind {
		case "pin":
			result.at, result.err = calls.pin(ctx, c.Target(), value)
		case "mute":
			result.at, result.err = calls.mute(ctx, c.Target(), duration)
		case "stash":
			result.err = calls.stash(ctx, c.Target(), value)
		case "leave":
			result.err = calls.leave(ctx, c.ID, handle)
		case "block":
			result.err = calls.block(ctx, c.Partner.Handle)
		}
		return result
	})
}

func (m *AppModel) acceptRemoteAction(msg remoteActionResult) tea.Cmd {
	if !m.connected || msg.session != m.session {
		return nil
	}
	title := m.conversationTitle(msg.conversation)
	if msg.err == nil && msg.kind != "mute" {
		// An inbox request sent before the change would bring the old row back.
		if r := &m.remotes; r.cancel != nil {
			r.cancel()
			r.cancel, r.loading = nil, false
		}
		m.remotes.generation++
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		failure := map[string]string{"pin": "Could not update the pin. Try again.", "mute": "Could not update the mute. Try again.", "stash": "Could not update the conversation. Try again.", "leave": "Could not leave " + title + ". Try again.", "block": "Could not block " + title + ". Try again."}[msg.kind]
		return m.notify("error", failure, "")
	}
	switch msg.kind {
	case "pin":
		m.updateConversation(msg.conversation.ID, func(c *api.Conversation) { c.PinnedAt = msg.at })
		m.resortInbox()
	case "mute":
		m.updateConversation(msg.conversation.ID, func(c *api.Conversation) { c.MutedUntilAt = msg.at })
		if msg.duration == api.MuteOff {
			return m.notify("success", "Unmuted "+title+".", "")
		}
		return m.notify("success", "Muted "+title+".", "")
	case "stash":
		m.dropConversation(msg.conversation.ID)
		if msg.value {
			return m.notify("success", "Stashed "+title+".", "It pops back when someone writes.")
		}
		return m.notify("success", "Popped "+title+" back into Open.", "")
	case "leave":
		m.dropConversation(msg.conversation.ID)
	case "block":
		m.dropConversation(msg.conversation.ID)
		m.restrictSafetyContent(msg.conversation.Partner.Handle, "block")
	}
	return nil
}

func (m *AppModel) updateConversation(id string, change func(*api.Conversation)) {
	for i := range m.remotes.inbox {
		if m.remotes.inbox[i].ID == id {
			change(&m.remotes.inbox[i])
		}
	}
	if t := m.remotes.thread; t != nil && t.conversation.ID == id {
		change(&t.conversation)
		m.renderRemoteThread()
	}
}

func (m *AppModel) resortInbox() {
	r := &m.remotes
	anchor := ""
	if r.selected < len(r.inbox) {
		anchor = r.inbox[r.selected].ID
	}
	sortInbox(r.inbox)
	for i, c := range r.inbox {
		if c.ID == anchor {
			r.selected = i
		}
	}
}

func (m *AppModel) dropConversation(id string) {
	r := &m.remotes
	kept := r.inbox[:0]
	for i, c := range r.inbox {
		if c.ID == id {
			if i < r.selected {
				r.selected--
			}
			continue
		}
		kept = append(kept, c)
	}
	r.inbox = kept
	r.selected = max(0, min(r.selected, len(kept)-1))
	if r.thread != nil && r.thread.conversation.ID == id {
		m.closeRemoteThread()
	}
}
