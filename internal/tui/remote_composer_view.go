package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// chipArea is a reaction chip on a thread line, for toggling it with a click.
type chipArea struct {
	line, x, w int
	id         int64
	emoji      string
}

// threadItem is what the message cursor stops on: a stored message, or a push still waiting for the server.
type threadItem struct {
	message *api.Message
	pending *pendingMessage
}

func (item threadItem) key() string {
	if item.pending != nil {
		return "p:" + item.pending.clientID
	}
	return fmt.Sprint("m:", item.message.ID)
}

func (m AppModel) threadItems() []threadItem {
	t := m.remotes.thread
	var items []threadItem
	for i := range t.messages {
		if t.messages[i].Event == "" {
			items = append(items, threadItem{message: &t.messages[i]})
		}
	}
	pending := m.messaging.pending[t.conversation.ID]
	for i := range pending {
		items = append(items, threadItem{pending: &pending[i]})
	}
	return items
}

// selectedItem is the item under the cursor, or the newest one when the cursor follows the newest message.
func (m AppModel) selectedItem() (threadItem, bool) {
	items := m.threadItems()
	for _, item := range items {
		if item.key() == m.remotes.thread.selected {
			return item, true
		}
	}
	if len(items) == 0 {
		return threadItem{}, false
	}
	return items[len(items)-1], true
}

// moveMessageCursor steps through the messages. Past the newest one the cursor goes back to following new messages,
// and at the oldest it loads older ones.
func (m *AppModel) moveMessageCursor(delta int) tea.Cmd {
	t := m.remotes.thread
	items := m.threadItems()
	if len(items) == 0 {
		return nil
	}
	index := len(items)
	for i, item := range items {
		if item.key() == t.selected {
			index = i
		}
	}
	index += delta
	if index >= len(items) {
		t.selected = ""
		m.renderRemoteThread()
		t.viewport.GotoBottom()
		return m.markRemoteRead()
	}
	index = max(0, index)
	t.selected = items[index].key()
	m.renderRemoteThread()
	if span, ok := t.spans[t.selected]; ok {
		if span[0] < t.viewport.YOffset() {
			t.viewport.SetYOffset(span[0])
		} else if span[1] > t.viewport.YOffset()+t.viewport.Height() {
			t.viewport.SetYOffset(span[1] - t.viewport.Height())
		}
	}
	if index == 0 && delta < 0 {
		return m.fetchRemoteThread("older")
	}
	return m.markRemoteRead()
}

// resizeThread fits the message area around the composer as it grows, keeping the newest message in view.
func (m *AppModel) resizeThread() {
	t := m.remotes.thread
	if t == nil {
		return
	}
	atBottom := t.viewport.AtBottom()
	view, _ := m.messageComposerView(m.threadPaneWidth(), colors(m.account.dark))
	input := &t.composer.input
	overhead := lipgloss.Height(view) - input.Height()
	input.MaxHeight = max(1, min(composerMaxRows, m.mainHeight()-remotesHeaderHeight-4-overhead))
	input.SetWidth(max(1, m.threadPaneWidth()-2))
	view, _ = m.messageComposerView(m.threadPaneWidth(), colors(m.account.dark))
	t.viewport.SetHeight(max(1, m.mainHeight()-remotesHeaderHeight-3-lipgloss.Height(view)))
	if atBottom {
		t.viewport.GotoBottom()
	}
}

// accessText is the web's AccessBar, which takes the composer's place when the user can't push to the partner.
func accessText(access, handle string) string {
	handle = "@" + safeText(handle)
	switch access {
	case "request":
		return handle + " wants to push to you. Merge to reply, or drop it. They won't be told."
	case "waiting":
		return "pushed. waiting for " + handle + " to merge your request"
	case "restricted":
		return handle + " doesn't follow you, so you can't push to them yet"
	case "closed":
		return handle + " isn't taking remotes"
	}
	return ""
}

func (m AppModel) replyAuthor(message *api.Message) string {
	if message.UserID == m.account.user.ID {
		return "yourself"
	}
	return m.handleOf(m.messageSender(*message, m.remotes.thread))
}

func excerpt(message api.Message) string {
	body := strings.Join(strings.Fields(safeText(message.Body)), " ")
	switch {
	case body != "":
		return body
	case message.Voice != nil:
		return "voice note"
	case len(message.Images) > 0:
		return "image"
	}
	return ""
}

// messageComposerView draws the bottom of the thread: the composer, the reaction row while it is open, or the access
// bar. Its areas are relative to the view's first row.
func (m AppModel) messageComposerView(width int, p palette) (string, []hitArea) {
	t := m.remotes.thread
	if t == nil || !t.loaded {
		return "", nil
	}
	hairline := text(p.hairline).Render(strings.Repeat("─", width))
	inner := max(1, width-2)
	if !t.canPush() {
		if t.conversation.Kind == "org" || t.conversation.Partner == nil {
			return "", nil
		}
		bar := lipgloss.NewStyle().Width(inner).Foreground(lipgloss.Color(p.muted)).Render(accessText(t.data.Access, t.conversation.Partner.Handle))
		return hairline + "\n" + indent(bar), nil
	}
	if t.react != nil && !t.react.picking {
		row, areas := m.reactionRow(width, p)
		for i := range areas {
			areas[i].y++
		}
		return hairline + "\n" + row, areas
	}
	mc := t.composer
	lines := []string{hairline}
	var areas []hitArea
	if mc.reply != nil || mc.replyID != 0 {
		label := "↳ Replying to an earlier message"
		if mc.reply != nil {
			label = "↳ Replying to " + m.replyAuthor(mc.reply) + ": " + excerpt(*mc.reply)
		}
		hint := ""
		if mc.focused {
			hint = text(p.muted).Render("esc cancel")
		}
		lines = append(lines, " "+split(text(p.muted).Render(fit(label, inner-12)), hint, inner))
	}
	if len(mc.images) > 0 {
		header := fmt.Sprintf("Images %d/%d · tab edit images", len(mc.images), api.MaxImages)
		if mc.stripFocus {
			header = fmt.Sprintf("Images %d/%d · tab write · enter describe · del remove", len(mc.images), api.MaxImages)
		}
		if mc.editing {
			header = fmt.Sprintf("Images %d/%d · enter save · esc cancel", len(mc.images), api.MaxImages)
		}
		lines = append(lines, " "+text(p.muted).Render(fit(header, inner)))
		thumbnails := m.messageThumbnails()
		if thumbnails {
			for _, row := range m.thumbnailRows(mc.images, mc.strip, mc.stripFocus, inner, p) {
				lines = append(lines, " "+row)
			}
		}
		for i, image := range mc.images {
			if thumbnails && i != mc.strip {
				continue
			}
			marker := "  "
			if mc.stripFocus && i == mc.strip {
				marker = text(p.blue).Render("› ")
			}
			if mc.editing && i == mc.strip {
				lines = append(lines, " "+marker+mc.field.View())
				continue
			}
			description := "No description"
			if strings.TrimSpace(image.Description) != "" {
				description = safeText(image.Description)
			}
			name := "▧ " + safeText(image.Name)
			if thumbnails {
				name = fmt.Sprintf("Image %d", i+1)
			}
			lines = append(lines, " "+marker+text(p.ink).Render(fit(fmt.Sprintf("%s · %d×%d · %s", name, image.Width, image.Height, description), inner-2)))
		}
	}
	if len(mc.emojis) > 0 {
		var choices []string
		start := max(0, min(mc.emojiIndex-2, len(mc.emojis)-5))
		for i := start; i < min(len(mc.emojis), start+5); i++ {
			choice := mc.emojis[i].value + " :" + mc.emojis[i].name + ":"
			if i == mc.emojiIndex {
				choice = withBackground(text(p.ink).Bold(true).Render(choice), p.soft)
			} else {
				choice = text(p.muted).Render(choice)
			}
			choices = append(choices, choice)
		}
		lines = append(lines, " "+fit(strings.Join(choices, "  "), inner))
	}
	editorTop := len(lines)
	for _, line := range strings.Split(mc.input.View(), "\n") {
		lines = append(lines, " "+fit(line, inner))
	}
	areas = append(areas, hitArea{uiRect{0, editorTop, width, len(lines) - editorTop}, "write", 0})
	right := ""
	if len(t.messages) > 0 && m.remoteCalls.ping != nil && mc.status == "" {
		label := "Ping everyone in the org"
		if c := t.conversation; c.Kind != "org" && c.Partner != nil {
			label = "Ping @" + safeText(c.Partner.Handle)
		}
		right = withBackground(text(p.ink).Render(" "+label+" "), p.wash)
		if !mc.focused {
			right = keyChip("p", p) + " " + text(p.muted).Render(label)
		}
	}
	if length := api.MessageLength(mc.input.Value()); length >= messageCountFrom {
		style := text(p.muted)
		if length > api.MaxMessageLength {
			style = text(p.danger)
		}
		right += "  " + style.Render(groupDigits(length)+"/"+groupDigits(api.MaxMessageLength))
	}
	hint := text(p.muted).Render("w write · r reply · e react · j/k move")
	switch {
	case mc.status != "":
		hint = text(p.danger).Render(safeText(mc.status))
	case mc.editing:
		hint = text(p.muted).Render("enter save · esc cancel")
	case mc.stripFocus:
		hint = text(p.muted).Render("←/→ choose image · tab write")
	case mc.focused:
		hint = text(p.muted).Render("enter push · shift+enter new line · ctrl+o image · esc messages")
	}
	if right != "" {
		pingWidth := lipgloss.Width(right)
		areas = append(areas, hitArea{uiRect{1 + inner - pingWidth, len(lines), pingWidth, 1}, "ping", 0})
	}
	lines = append(lines, " "+split(fit(hint, inner-lipgloss.Width(right)-1), right, inner))
	return strings.Join(lines, "\n"), areas
}

func (m AppModel) messageThumbnails() bool {
	t := m.remotes.thread
	if t == nil || !t.canPush() || t.react != nil || m.images == nil || !m.images.Enabled || len(t.composer.images) == 0 {
		return false
	}
	mc := t.composer
	room := m.mainHeight() - remotesHeaderHeight - 7
	if mc.reply != nil || mc.replyID != 0 {
		room--
	}
	if len(mc.emojis) > 0 {
		room--
	}
	return room >= stripThumbRows+2 && m.threadPaneWidth()-2 >= len(mc.images)*(stripThumbCols+2)-1
}

func (m AppModel) messageThumbnailRequests() []graphics.Request {
	if !m.remotesOnTop() || !m.messageThumbnails() || m.remotes.thread.composer.browser != nil {
		return nil
	}
	var requests []graphics.Request
	for _, image := range m.remotes.thread.composer.images {
		requests = append(requests, stripThumbnail(image))
	}
	return requests
}

func indent(block string) string {
	lines := strings.Split(block, "\n")
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// pendingLines draws a push the server hasn't stored: a soft bubble on the right with its state, and how to retry or
// discard it once it was rejected.
func (m AppModel) pendingLines(pending pendingMessage, width, bubble int, p palette) []string {
	align := func(line string) string { return lipgloss.PlaceHorizontal(width-1, lipgloss.Right, line) }
	var lines []string
	if pending.reply != nil {
		lines = append(lines, align(text(p.muted).Render(fit("↳ "+excerpt(*pending.reply), bubble))))
	}
	style := statsStyle{icons: m.images, glyphs: m.icons, origin: m.account.origin}
	if body := strings.TrimSpace(pending.draft.Body); body != "" {
		for _, line := range bubbleLines(bodyLines(api.Commit{ID: "pending-" + pending.clientID, Body: body}, bubble-2, p, style), p.ink, p.wash) {
			lines = append(lines, align(line))
		}
	}
	if count := len(pending.draft.Images); count > 0 {
		lines = append(lines, align(text(p.muted).Render(fmt.Sprintf("▧ %d image%s", count, map[bool]string{true: "", false: "s"}[count == 1]))))
	}
	state := text(p.muted).Render("◌ staged")
	if pending.status == "rejected" {
		actions := " · r retry · d discard"
		state = text(p.danger).Render(fit("✕ rejected · "+pending.reason, max(1, width-2-len(actions)))) + text(p.muted).Render(actions)
	}
	return append(lines, align(state))
}

// markSelected draws the cursor's bar on the left edge of the selected item's lines.
func markSelected(lines []string, span [2]int, width int, p palette) {
	for i := span[0]; i < min(span[1], len(lines)); i++ {
		lines[i] = text(p.blue).Render("▌") + ansi.TruncateLeft(fit(lines[i], width), 1, "")
	}
}
