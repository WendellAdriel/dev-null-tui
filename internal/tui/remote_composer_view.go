package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/audio"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// chipArea is a reaction chip on a thread line, for toggling it with a click.
// pendingKey is a rejected push's Retry or Discard chip.
type pendingKey struct {
	line, x, w int
	clientID   string
	key        string
}

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
	view, _ := m.messageComposerView(m.threadPaneWidth(), m.account.colors())
	input := &t.composer.input
	overhead := lipgloss.Height(view) - input.Height()
	input.MaxHeight = max(1, min(composerMaxRows, m.mainHeight()-remotesHeaderHeight-4-overhead))
	input.SetWidth(max(1, m.threadPaneWidth()-2))
	view, _ = m.messageComposerView(m.threadPaneWidth(), m.account.colors())
	t.viewport.SetHeight(max(1, m.mainHeight()-remotesHeaderHeight-3-m.bannerRows()-lipgloss.Height(view)))
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

// requestBar adds the web's Drop and Merge buttons under a fetched request's text, right-aligned.
func requestBar(bar string, inner int, p palette) (string, []hitArea) {
	drop := actionChip("Drop", "d", p)
	merge := actionChip("Merge", "m", p)
	row := lipgloss.Height(bar)
	mergeX := 1 + inner - lipgloss.Width(merge)
	dropX := mergeX - 3 - lipgloss.Width(drop)
	areas := []hitArea{{uiRect{dropX, row, lipgloss.Width(drop), 1}, "drop", 0}, {uiRect{mergeX, row, lipgloss.Width(merge), 1}, "merge", 0}}
	return bar + "\n " + split("", drop+"   "+merge, inner), areas
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
		if t.data.Access != "request" {
			return hairline + "\n" + indent(bar), nil
		}
		return requestBar(hairline+"\n"+indent(bar), inner, p)
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
	if labels := mc.suggest.labels(); len(labels) > 0 {
		var choices []string
		start := max(0, min(mc.suggest.index-2, len(labels)-5))
		for i := start; i < min(len(labels), start+5); i++ {
			choice := labels[i]
			if i == mc.suggest.index {
				choice = withBackground(text(p.ink).Bold(true).Render(choice), p.soft)
			} else {
				choice = text(p.muted).Render(choice)
			}
			choices = append(choices, choice)
		}
		lines = append(lines, " "+fit(strings.Join(choices, "  "), inner))
	}
	editorTop := len(lines)
	if mc.recording != nil {
		lines = append(lines, m.recordingLines(mc.recording, inner, p)...)
	} else {
		for _, line := range strings.Split(mc.input.View(), "\n") {
			lines = append(lines, " "+fit(line, inner))
		}
	}
	areas = append(areas, hitArea{uiRect{0, editorTop, width, len(lines) - editorTop}, "write", 0})
	// Write and Actions sit on the right of the idle row, where the ping chip used to be. Ping is in the status bar.
	voice := m.voiceStatus()
	idle := voice == nil && !mc.voiceConfirm && mc.recording == nil && mc.status == "" && !mc.editing && !mc.stripFocus && !mc.focused
	right, rightAreas := "", []hitArea(nil)
	if idle {
		right, rightAreas = chipLine([]commitAction{{"Write", "w"}, {"Actions", "."}}, inner, p)
	}
	if length := api.MessageLength(mc.input.Value()); length >= messageCountFrom {
		style := text(p.muted)
		if length > api.MaxMessageLength {
			style = text(p.danger)
		}
		right += "  " + style.Render(groupDigits(length)+"/"+groupDigits(api.MaxMessageLength))
	}
	hint := ""
	if voice != nil {
		var keys []string
		for _, part := range voice[1:] {
			keys = append(keys, part.key+" "+strings.ToLower(part.label))
		}
		hint = text(p.muted).Render(strings.Join(keys, " · "))
	} else if idle {
		actions := []commitAction{{"Reply", "r"}, {"React", "e"}, {"Voice", "ctrl+t"}}
		if m.canPin() {
			actions = append(actions, commitAction{"Pin", "!"})
		}
		var chipAreas []hitArea
		hint, chipAreas = chipLine(actions, inner-lipgloss.Width(right)-1, p)
		for _, area := range chipAreas {
			area.x++
			area.y = len(lines)
			areas = append(areas, area)
		}
	}
	switch {
	case mc.voiceConfirm:
		hint = text(p.warning).Render("Recording replaces the draft · enter record · esc keep writing")
	case mc.recording != nil && mc.recording.recorder != nil:
		hint = text(p.muted).Render("recording · enter stop · esc discard")
	case mc.status != "":
		hint = text(p.danger).Render(safeText(mc.status))
	case mc.recording != nil:
		hint = text(p.muted).Render("space listen · enter push · d discard · esc messages")
	case mc.editing:
		hint = text(p.muted).Render("enter save · esc cancel")
	case mc.stripFocus:
		hint = text(p.muted).Render("←/→ choose image · tab write")
	case mc.focused:
		hint = text(p.muted).Render(fitting(inner-lipgloss.Width(right)-1,
			"enter push · ctrl+t voice (max. 1 min) · shift+enter new line · ctrl+o image · esc messages",
			"enter push · ctrl+t voice · ctrl+o image · esc messages",
			"enter push · ctrl+t voice",
			"enter push, ctrl+t voice"))
	}
	for _, area := range rightAreas {
		area.x += 1 + inner - lipgloss.Width(right)
		area.y = len(lines)
		areas = append(areas, area)
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
	if mc.suggest.count() > 0 {
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
// fitting is the first hint that fits the width, so the keys users can't guess stay visible on narrow panes.
func fitting(width int, hints ...string) string {
	for _, hint := range hints {
		if lipgloss.Width(hint) <= width {
			return hint
		}
	}
	return hints[len(hints)-1]
}

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
	if voice := pending.draft.Voice; voice != nil {
		seconds := (len(voice.WAV) - 44) / (2 * audio.RecordRate)
		lines = append(lines, align(text(p.muted).Render(fmt.Sprintf("♪ voice note · %d:%02d", seconds/60, seconds%60))))
	}
	if count := len(pending.draft.Images); count > 0 {
		lines = append(lines, align(text(p.muted).Render(fmt.Sprintf("▧ %d image%s", count, map[bool]string{true: "", false: "s"}[count == 1]))))
	}
	state := text(p.muted).Render("◌ staged")
	if pending.status == "rejected" {
		actions := " " + actionChip("Retry", "r", p) + " " + actionChip("Discard", "d", p)
		state = text(p.danger).Render(fit("✕ rejected · "+pending.reason, max(1, width-2-lipgloss.Width(actions)))) + actions
	}
	return append(lines, align(state))
}

// markSelected draws the cursor's bar on the left edge of the selected item's lines.
func markSelected(lines []string, span [2]int, width int, p palette) {
	for i := span[0]; i < min(span[1], len(lines)); i++ {
		lines[i] = text(p.blue).Render("▌") + ansi.TruncateLeft(fit(lines[i], width), 1, "")
	}
}

// chipLine lays out action chips on one line, leaving out the ones that don't fit, with a hit area for each.
func chipLine(actions []commitAction, width int, p palette) (string, []hitArea) {
	line, x := "", 0
	var areas []hitArea
	for _, action := range actions {
		chip := actionChip(action.label, action.key, p)
		gap := 0
		if line != "" {
			gap = 2
		}
		if x+gap+lipgloss.Width(chip) > width {
			break
		}
		line += strings.Repeat(" ", gap) + chip
		x += gap
		areas = append(areas, hitArea{uiRect{x, 0, lipgloss.Width(chip), 1}, "key:" + action.key, 0})
		x += lipgloss.Width(chip)
	}
	return line, areas
}
