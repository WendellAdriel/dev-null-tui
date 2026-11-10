package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// Pinned messages follow the web's resources/js/components/chat/pinned-messages.tsx: a banner over org threads with
// the latest pin, a list of them all, and a warning before pinning, since everyone in the org sees pins.

// pinWarning asks before pinning message, with dismiss as its Don't show this again box.
type pinWarning struct {
	message api.Message
	dismiss bool
}

// pinList is the list of an org's pinned messages, with the cursor on index.
type pinList struct {
	index int
	err   string
	// loading is true while older pages load to reach the chosen message.
	loading bool
}

type pinResult struct {
	session int
	org     string
	pin     bool
	pins    api.MessagePins
	err     error
}

// canPin reports whether the user can pin in the open thread: only an org's maintainers can.
func (m AppModel) canPin() bool {
	t := m.remotes.thread
	return t != nil && t.conversation.Kind == "org" && t.data.Role == string(api.RoleMaintainer) && m.remoteCalls.setPinned != nil
}

func (m AppModel) isPinned(id int64) bool {
	return slices.ContainsFunc(m.remotes.thread.data.Pinned, func(message api.Message) bool { return message.ID == id })
}

// pinKey pins or unpins the message under the cursor. Pinning warns first until the user says not to.
func (m *AppModel) pinKey() tea.Cmd {
	if !m.canPin() {
		return nil
	}
	item, ok := m.selectedItem()
	if !ok || item.message == nil {
		return nil
	}
	switch {
	case m.isPinned(item.message.ID):
		return m.setPinned(*item.message, false)
	case m.tuiSettings.PinWarningDismissed:
		return m.setPinned(*item.message, true)
	}
	m.remotes.pinWarning = &pinWarning{message: *item.message}
	return nil
}

func (m *AppModel) setPinned(message api.Message, pin bool) tea.Cmd {
	calls, session, life, org := m.remoteCalls, m.session, m.account.life, m.remotes.thread.conversation.ID
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		pins, err := calls.setPinned(ctx, message.ID, pin)
		return pinResult{session: session, org: org, pin: pin, pins: pins, err: err}
	})
}

func (m *AppModel) acceptPin(msg pinResult) tea.Cmd {
	if !m.connected || msg.session != m.session {
		return nil
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		// Like the web, only a refusal's reason is shown, such as the 25-pin limit.
		failure := "Could not unpin. Try again."
		if msg.pin {
			failure = "Could not pin. Try again."
		}
		var refusal *auth.APIError
		if errors.As(msg.err, &refusal) && refusal.Status == http.StatusForbidden && refusal.Message != "" {
			failure = safeText(refusal.Message)
		}
		return m.notify("error", failure, "")
	}
	t := m.remotes.thread
	if t == nil || t.conversation.ID != msg.org {
		return nil
	}
	t.data.Pinned = msg.pins.Pinned
	if t.data.Pinned == nil {
		t.data.Pinned = []api.Message{}
	}
	if list := m.remotes.pinList; list != nil {
		if len(t.data.Pinned) == 0 {
			m.remotes.pinList = nil
		} else {
			list.index = min(list.index, len(t.data.Pinned)-1)
		}
	}
	m.resizeThread()
	return nil
}

func (m *AppModel) pinWarningKey(pressed string) tea.Cmd {
	w := m.remotes.pinWarning
	switch pressed {
	case "esc":
		m.remotes.pinWarning = nil
	case "space", "x":
		w.dismiss = !w.dismiss
	case "enter":
		m.remotes.pinWarning = nil
		pin := m.setPinned(w.message, true)
		if !w.dismiss {
			return pin
		}
		m.tuiSettings.PinWarningDismissed = true
		return tea.Batch(pin, m.saveTUISettings(false))
	}
	return nil
}

func (m *AppModel) openPinList() {
	if t := m.remotes.thread; t != nil && len(t.data.Pinned) > 0 {
		m.remotes.pinList = &pinList{}
	}
}

func (m *AppModel) pinListKey(pressed string) tea.Cmd {
	list, pinned := m.remotes.pinList, m.remotes.thread.data.Pinned
	if list.loading {
		if pressed == "esc" {
			m.remotes.pinList, m.remotes.thread.jumpTo = nil, 0
		}
		return nil
	}
	switch pressed {
	case "esc", "L":
		m.remotes.pinList = nil
	case "j", "down":
		list.index = min(list.index+1, len(pinned)-1)
	case "k", "up":
		list.index = max(list.index-1, 0)
	case "!":
		if m.canPin() {
			return m.setPinned(pinned[list.index], false)
		}
	case "enter":
		return m.jumpToMessage(pinned[list.index].ID)
	}
	return nil
}

// jumpToMessage puts the cursor on a message and scrolls to it, loading older pages until it arrives.
func (m *AppModel) jumpToMessage(id int64) tea.Cmd {
	t := m.remotes.thread
	if !slices.ContainsFunc(t.messages, func(message api.Message) bool { return message.ID == id }) {
		list := m.remotes.pinList
		if list == nil {
			list = &pinList{index: max(0, slices.IndexFunc(t.data.Pinned, func(message api.Message) bool { return message.ID == id }))}
			m.remotes.pinList = list
		}
		if !t.data.HasOlder {
			list.loading, list.err, t.jumpTo = false, "That message is no longer in this conversation.", 0
			return nil
		}
		list.loading, list.err, t.jumpTo = true, "", id
		return m.fetchRemoteThread("older")
	}
	m.remotes.pinList, t.jumpTo = nil, 0
	t.selected = fmt.Sprint("m:", id)
	m.renderRemoteThread()
	if span, ok := t.spans[t.selected]; ok {
		t.viewport.SetYOffset(span[0])
	}
	return nil
}

// pinnedText is a pinned message in one line of plain text, as the web's textOf.
func pinnedText(message api.Message) string {
	switch {
	case strings.TrimSpace(message.Body) != "":
		return strings.Join(strings.Fields(safeText(message.Body)), " ")
	case message.Voice != nil:
		return "Voice note"
	case len(message.Images) > 1:
		return fmt.Sprintf("%d images", len(message.Images))
	}
	return "Image"
}

func (m AppModel) pinAuthor(message api.Message) string {
	if user := m.messageSender(message, m.remotes.thread); user != nil {
		if name := safeText(user.Name); name != "" {
			return name
		}
		return "@" + safeText(user.Handle)
	}
	return "someone"
}

// pinBanner is the one row over an org thread with how many messages are pinned and the latest one, or empty.
func (m AppModel) pinBanner(width int, p palette) string {
	t := m.remotes.thread
	if t == nil || t.conversation.Kind != "org" || len(t.data.Pinned) == 0 {
		return ""
	}
	latest := t.data.Pinned[0]
	left := text(p.muted).Render(m.icons.pin+" "+fmt.Sprintf("%d pinned · ", len(t.data.Pinned))) + text(p.ink).Render(m.pinAuthor(latest)+": "+pinnedText(latest))
	right := actionChip("list", "L", p)
	return " " + split(fit(left, max(1, width-ansi.StringWidth(right)-4)), right, max(1, width-2))
}

// bannerRows is the height the pin banner takes from the messages.
func (m AppModel) bannerRows() int {
	if m.pinBanner(1, m.account.colors()) == "" {
		return 0
	}
	return 1
}

func (m AppModel) pinWarningModal(p palette) (string, uiRect, []hitArea) {
	w := m.remotes.pinWarning
	rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 14)
	width := max(1, rect.w-4)
	description := text(p.muted).Render(lipgloss.NewStyle().Width(width).Render("Everyone in the org sees pinned messages, including people added without Share history, who can't read anything pushed before they joined."))
	box := "[ ]"
	if w.dismiss {
		box = "[x]"
	}
	check := actionChip(box+" Don't show this again", "space", p)
	body := description + "\n\n" + check
	footer, areas := modalFooter([]modalAction{{"Cancel", "esc", "esc"}, {"Pin message", "enter", "enter"}}, width, 2, p)
	for i := range areas {
		areas[i].x += rect.x + 2
		areas[i].y += rect.y + rect.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{rect.x + 2, rect.y + 3 + lipgloss.Height(description) + 1, width, 1}, "space", 0}, hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
	return panel("Pin for the whole org?", body, footer, rect, p, p.blue), rect, areas
}

func (m AppModel) pinListModal(p palette) (string, uiRect, []hitArea) {
	list, pinned := m.remotes.pinList, m.remotes.thread.data.Pinned
	rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 72, 22)
	width := max(1, rect.w-4)
	rows := max(1, rect.h-8)
	start := max(0, min(list.index-rows+1, len(pinned)-rows))
	var lines []string
	var areas []hitArea
	for i := start; i < min(len(pinned), start+rows); i++ {
		message := pinned[i]
		when := text(p.muted).Render(relativeAge(message.CreatedAt, m.now))
		row := " " + split(fit(text(p.ink).Bold(true).Render(m.pinAuthor(message)+":")+" "+text(p.ink).Render(pinnedText(message)), max(1, width-10)), when, width-2) + " "
		if i == list.index {
			row = withBackground(fit(row, width), p.soft)
		}
		areas = append(areas, hitArea{uiRect{rect.x + 2, rect.y + 3 + len(lines), width, 1}, "row", i})
		lines = append(lines, row)
	}
	switch {
	case list.loading:
		lines = append(lines, "", text(p.muted).Render(fit("Loading older messages to find it…", width)))
	case list.err != "":
		lines = append(lines, "", text(p.danger).Render(fit(list.err, width)))
	}
	hints := []statusPart{statusChip("Jump", "enter"), closeChip("Close")}
	switch {
	case list.loading:
		hints = []statusPart{closeChip("Cancel")}
	case m.canPin():
		hints = []statusPart{statusChip("Jump", "enter"), statusChip("Unpin", "!"), closeChip("Close")}
	}
	footer, footerAreas := hintRow(hints, width, p)
	areas = append(append(areas, panelFooterAreas(footerAreas, footer, rect)...), hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
	return panel(fmt.Sprintf("Pinned · %d", len(pinned)), strings.Join(lines, "\n"), footer, rect, p, p.blue), rect, areas
}

func (m AppModel) pinModalMouse(x, y int) (AppModel, tea.Cmd) {
	var rect uiRect
	var areas []hitArea
	if m.remotes.pinWarning != nil {
		_, rect, areas = m.pinWarningModal(m.account.colors())
	} else {
		_, rect, areas = m.pinListModal(m.account.colors())
	}
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if area.action == "row" {
			if m.remotes.pinList.loading {
				return m, nil
			}
			m.remotes.pinList.index = area.index
			return m.remotesKey("enter")
		}
		if pressed, ok := keyAction(area.action); ok {
			return m.remotesKey(pressed)
		}
		return m.remotesKey(area.action)
	}
	if !rect.contains(x, y) {
		m.remotes.pinWarning, m.remotes.pinList, m.remotes.thread.jumpTo = nil, nil, 0
	}
	return m, nil
}
