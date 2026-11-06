package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// The screen follows the web's resources/js/pages/conversation/index.tsx, conversation-list.tsx and
// conversation-thread.tsx.

const (
	remotesHeaderHeight = 3
	inboxPaneWidth      = 36
	inboxHeaderHeight   = 3
	inboxRowHeight      = 3
)

// remoteMedia is an image or link card picture on a line range of the thread, for loading what is on screen.
type remoteMedia struct {
	start, end int
	req        graphics.Request
}

func (m AppModel) threadPaneWidth() int {
	if m.remotesSplit() {
		return max(1, m.remotesWidth()-inboxPaneWidth-1)
	}
	return m.remotesWidth()
}

func (m *AppModel) sizeRemotes() {
	t := m.remotes.thread
	if !m.remotes.open || t == nil {
		return
	}
	atBottom := t.viewport.AtBottom()
	t.viewport.SetWidth(m.threadPaneWidth())
	t.composer.input.SetWidth(max(1, m.threadPaneWidth()-2))
	m.renderRemoteThread()
	m.resizeThread()
	if atBottom {
		t.viewport.GotoBottom()
	}
}

func (m AppModel) remotesColumn(width int, p palette) string {
	height := max(1, m.mainHeight()-remotesHeaderHeight)
	line := text(p.hairline).Render(strings.Repeat("─", width))
	header := "\n  " + fit(text(p.ink).Bold(true).Render("Remotes"), width-4) + "\n" + line
	var body string
	switch {
	case m.remotesSplit():
		inbox, _ := m.inboxLayout(inboxPaneWidth, height, p)
		rail := text(p.hairline).Render(strings.TrimSuffix(strings.Repeat("│\n", height), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, inbox, rail, m.remoteThreadPane(m.threadPaneWidth(), height, p))
	case m.remotes.thread != nil:
		body = m.remoteThreadPane(width, height, p)
	default:
		body, _ = m.inboxLayout(width, height, p)
	}
	content := header + "\n" + body
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) tabLabel(tab api.InboxTab) string {
	switch tab {
	case api.Fetched:
		if m.remotes.requests > 0 {
			return fmt.Sprintf("Fetched · %d waiting", m.remotes.requests)
		}
		return "Fetched"
	case api.Stashed:
		return "Stashed"
	}
	return "Open"
}

// inboxLayout draws the tabs, the search line and the conversation rows, with areas relative to the pane.
func (m AppModel) inboxLayout(width, height int, p palette) (string, []hitArea) {
	r := m.remotes
	var areas []hitArea
	tabs, x := "", 1
	for i, tab := range m.remotesTabs() {
		label := m.tabLabel(tab)
		style := text(p.muted)
		if tab == r.tab {
			style = text(p.ink).Bold(true).Underline(true)
		}
		if i > 0 {
			tabs += "  "
			x += 2
		}
		tabs += style.Render(label)
		areas = append(areas, hitArea{uiRect{x, 0, ansi.StringWidth(label), 1}, "tab", i})
		x += ansi.StringWidth(label)
	}
	search := text(p.muted).Render("/ Search")
	switch {
	case r.searching:
		search = text(p.ink).Render("/ "+safeText(r.query)) + text(p.blue).Render("▏")
	case r.term != "":
		search = split(text(p.ink).Render("/ "+safeText(r.term)), text(p.muted).Render("esc clear"), width-2)
	}
	areas = append(areas, hitArea{uiRect{1, 1, width - 2, 1}, "/", 0})
	lines := []string{" " + fit(tabs, width-2), " " + fit(search, width-2), text(p.hairline).Render(strings.Repeat("─", width))}
	message := ""
	switch {
	case r.status != "" && len(r.inbox) == 0:
		message = r.status
	case !r.loaded:
		message = "Reading Remotes from /dev/null..."
	case len(r.inbox) > 0:
	case r.term != "":
		message = "fatal: nothing matches '" + safeText(r.term) + "'"
	case r.tab == api.Stashed:
		message = "nothing stashed"
	case r.tab == api.Fetched:
		message = "no requests fetched"
	default:
		message = "no conversations yet"
	}
	if message != "" {
		lines = append(lines, "", "  "+text(p.muted).Render(fit(message, width-4)))
	}
	start, end := m.inboxWindow(height)
	for i := start; i < end; i++ {
		row := m.conversationRow(r.inbox[i], width, p)
		active := i == r.selected && !r.focusThread
		if t := r.thread; t != nil && t.conversation.ID == r.inbox[i].ID {
			active = true
		}
		if active {
			// The web marks the open conversation with a blue bar on its left edge.
			for j := range row {
				row[j] = withBackground(text(p.blue).Render("▌")+fit(row[j][1:], width-1), p.soft)
			}
		}
		areas = append(areas, hitArea{uiRect{0, len(lines), width, inboxRowHeight}, "row", i})
		lines = append(append(lines, row...), "")
	}
	if len(r.candidates) > 0 {
		lines = append(lines, "  "+text(p.muted).Bold(true).Render("Start a conversation"))
		for i, candidate := range r.candidates {
			index := len(r.inbox) + i
			row := "  " + text(p.ink).Bold(true).Render(safeText(candidate.User.Name)) + " " + text(p.muted).Render("@"+safeText(candidate.User.Handle))
			if candidate.Reason != "" {
				row = "  " + text(p.muted).Render("@"+safeText(candidate.User.Handle)+" · "+safeText(candidate.Reason))
			}
			row = fit(row, width)
			if index == r.selected && !r.focusThread {
				row = withBackground(text(p.blue).Render("▌")+fit(row[1:], width-1), p.soft)
			}
			areas = append(areas, hitArea{uiRect{0, len(lines), width, 1}, "person", index})
			lines = append(lines, row)
		}
	}
	if r.hasMore && len(lines) < height {
		lines = append(lines, lipgloss.PlaceHorizontal(width, lipgloss.Center, text(p.muted).Render("More ")+keyChip("n", p)))
	}
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(content), areas
}

// conversationRow is the web's row: the title and name, then the latest message, with the pin, the unread count or the
// time on the right.
func (m AppModel) conversationRow(c api.Conversation, width int, p palette) []string {
	_, avatar := m.userAvatar(m.conversationFace(c), 4, 2, p)
	inner := max(1, width-8)
	right := ""
	if c.PinnedAt != nil {
		right = text(p.muted).Render(m.icons.pin) + " "
	}
	switch {
	case c.Ahead > 99:
		right += text(p.blue).Bold(true).Render("99+ ahead")
	case c.Ahead > 0:
		right += text(p.blue).Bold(true).Render(fmt.Sprintf("%d ahead", c.Ahead))
	case c.LastMessageAt != nil:
		right += text(p.muted).Render(relativeAge(*c.LastMessageAt, m.now))
	}
	name := ""
	if c.Kind == "org" {
		name = fmt.Sprintf("org · %d", len(c.Members))
	} else if c.Partner != nil {
		name = safeText(c.Partner.Name)
	}
	suffix := ""
	if m.muted(c) {
		suffix += " " + text(p.muted).Render(m.icons.muted)
	}
	if name != "" {
		suffix += "  " + text(p.muted).Render(name)
	}
	// Like the web, a long title is cut before the org's member count or the partner's name.
	room := max(1, inner-ansi.StringWidth(right)-1)
	left := fit(text(p.ink).Bold(true).Render(fit(m.conversationTitle(c), max(6, room-ansi.StringWidth(suffix))))+suffix, room)
	return []string{" " + avatar[0] + " " + split(left, right, inner), " " + avatar[1] + " " + text(p.muted).Render(fit(m.conversationPreview(c), inner))}
}

// conversationFace is the person a row's avatar shows: the partner, or an org's first other member.
func (m AppModel) conversationFace(c api.Conversation) *api.User {
	if c.Kind != "org" {
		return c.Partner
	}
	for _, member := range c.Members {
		if member.User.ID != m.account.user.ID {
			return &member.User
		}
	}
	return nil
}

// inboxWindow is the range of inbox rows on screen, which keeps the selected row in view.
func (m AppModel) inboxWindow(height int) (int, int) {
	people := 0
	if len(m.remotes.candidates) > 0 {
		people = len(m.remotes.candidates) + 1
	}
	fits := max(1, (height-inboxHeaderHeight-people)/inboxRowHeight)
	start := max(0, min(m.remotes.selected, len(m.remotes.inbox)-1)-fits+1)
	return start, min(len(m.remotes.inbox), start+fits)
}

// conversationPreview is the latest message with whose it is, or what an event or attachment says.
func (m AppModel) conversationPreview(c api.Conversation) string {
	message := c.LastMessage
	if message == nil {
		return ""
	}
	if message.Event != "" {
		return m.eventText(*message, c.Partner)
	}
	body := strings.Join(strings.Fields(safeText(message.Body)), " ")
	switch {
	case body != "":
	case message.Voice != nil:
		body = "sent a voice note"
	case len(message.Images) > 0:
		body = "sent an image"
	}
	switch {
	case message.UserID == m.account.user.ID && c.Kind != "org":
		glyph := "✓ "
		if c.PartnerReadThroughID >= message.ID {
			glyph = "✓✓ "
		}
		return glyph + "you: " + body
	case message.UserID == m.account.user.ID:
		return "you: " + body
	case c.Kind == "org" && message.User != nil:
		return firstName(*message.User) + ": " + body
	}
	return body
}

func (m AppModel) handleOf(u *api.User) string {
	switch {
	case u == nil:
		return "someone"
	case u.ID == m.account.user.ID:
		return "you"
	}
	return "@" + safeText(u.Handle)
}

// eventText is what a logged org change, call or ping says, from the reader's point of view.
func (m AppModel) eventText(message api.Message, partner *api.User) string {
	own := message.UserID == m.account.user.ID
	sender := message.User
	if sender == nil {
		sender = partner
	}
	actor, subject, body := m.handleOf(sender), m.handleOf(message.Subject), safeText(message.Body)
	if own {
		actor = "you"
	}
	switch message.Event {
	case "created":
		if body != "" {
			return actor + " created " + body
		}
		return actor + " created the org"
	case "added":
		return actor + " added " + subject
	case "removed":
		return actor + " removed " + subject
	case "left":
		return actor + " left"
	case "renamed":
		if body != "" {
			return actor + " renamed the org to " + body
		}
		return actor + " cleared the org's name"
	case "promoted":
		return actor + " made " + subject + " a maintainer"
	case "demoted":
		return actor + " made " + subject + " a contributor"
	case "inherited":
		return actor + " inherited maintainer"
	case "call_started":
		return actor + " started a call"
	case "call_answered":
		return actor + " answered the call"
	case "call_cancelled":
		return actor + " cancelled the call"
	case "call_declined":
		return actor + " declined the call"
	case "call_failed":
		return actor + " couldn't answer the call"
	case "call_missed":
		if own {
			return "your call wasn't answered"
		}
		return "missed call from " + actor
	case "ping":
		switch {
		case own:
			return "you sent a ping"
		case message.User != nil:
			return actor + " sent a ping"
		}
		return "you got a ping"
	}
	return body
}

func typingText(handles []string) string {
	switch len(handles) {
	case 0:
		return ""
	case 1:
		return "@" + safeText(handles[0]) + " is typing…"
	case 2:
		return "@" + safeText(handles[0]) + " and @" + safeText(handles[1]) + " are typing…"
	}
	return fmt.Sprintf("%d people are typing…", len(handles))
}

func (m AppModel) remoteThreadPane(width, height int, p palette) string {
	t := m.remotes.thread
	box := lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height)
	if t == nil {
		empty := lipgloss.PlaceVertical(height, lipgloss.Center, lipgloss.PlaceHorizontal(width, lipgloss.Center, text(p.muted).Render("no conversation checked out")))
		return box.Render(empty)
	}
	c := t.conversation
	subtitle := ""
	if c.Kind == "org" {
		subtitle = fmt.Sprintf("org · %d members", len(c.Members))
	} else if c.Partner != nil {
		subtitle = safeText(c.Partner.Name)
	}
	right := ""
	if m.muted(c) {
		right += text(p.muted).Render(m.icons.muted) + " "
	}
	if c.PinnedAt != nil {
		right += text(p.muted).Render(m.icons.pin) + " "
	}
	right += keyChip(".", p) + text(p.muted).Render(" ⋯")
	_, face := m.userAvatar(m.conversationFace(c), 2, 1, p)
	left := face[0] + " " + text(p.ink).Bold(true).Render(m.conversationTitle(c)) + "  " + text(p.muted).Render(subtitle)
	header := " " + split(fit(left, max(1, width-ansi.StringWidth(right)-4)), right, width-2)
	typing := t.data.Typing
	if t.data.PartnerTyping && c.Partner != nil {
		typing = []string{c.Partner.Handle}
	}
	footer := " " + text(p.muted).Italic(true).Render(fit(typingText(typing), width-2))
	messages := t.viewport.View()
	if t.composer.browser != nil {
		messages = indent(m.browserView(max(1, width-2), t.viewport.Height(), p))
	}
	content := header + "\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n"
	if banner := m.pinBanner(width, p); banner != "" {
		content += banner + "\n"
	}
	content += messages + "\n" + footer
	if composer, _ := m.messageComposerView(width, p); composer != "" {
		content += "\n" + composer
	}
	return box.Render(content)
}

func (m AppModel) messageSender(message api.Message, t *remoteThreadState) *api.User {
	if message.User != nil {
		return message.User
	}
	if message.UserID == m.account.user.ID {
		return &api.User{ID: m.account.user.ID, Handle: m.account.user.Handle, Name: m.account.user.Name}
	}
	if t.data.Partner != nil && t.data.Partner.ID == message.UserID {
		return t.data.Partner
	}
	for _, member := range append(append([]api.Member(nil), t.data.Members...), t.conversation.Members...) {
		if member.User.ID == message.UserID {
			return &member.User
		}
	}
	return t.conversation.Partner
}

// renderRemoteThread draws the messages the way the web's conversation-thread.tsx does: day chips and events centered,
// everyone else's messages in soft bubbles on the left under their avatar and name, the user's own in blue bubbles on
// the right, and the time under each run of messages.
func (m *AppModel) renderRemoteThread() {
	t := m.remotes.thread
	if t == nil {
		return
	}
	p := colors(m.account.dark)
	own := p
	own.ink, own.blue, own.muted = p.shell, p.shell, p.shell
	width := max(1, t.viewport.Width())
	bubble := max(1, min(width-4, max(24, width*3/4)))
	style := statsStyle{icons: m.images, glyphs: m.icons, origin: m.account.origin}
	center := func(value string) string { return lipgloss.PlaceHorizontal(width, lipgloss.Center, value) }
	var lines []string
	t.media, t.chips, t.voices, t.spans = nil, nil, nil, map[string][2]int{}
	if t.data.HasOlder {
		lines = append(lines, center(text(p.muted).Render("Older messages ")+keyChip("n", p)))
	}
	day := ""
	partner := t.data.Partner
	if partner == nil {
		partner = t.conversation.Partner
	}
	// A run is the messages one person sent in a row on the same day, at most five minutes apart.
	sameRun := func(a, b api.Message) bool {
		return a.Event == "" && b.Event == "" && a.UserID == b.UserID && dayLabel(a.CreatedAt, m.now) == dayLabel(b.CreatedAt, m.now) && b.CreatedAt.Sub(a.CreatedAt) <= 5*time.Minute
	}
	for i, message := range t.messages {
		if label := dayLabel(message.CreatedAt, m.now); label != day {
			lines = append(lines, "", center(paintLine(" ● "+label+" ", p.muted, p.wash)), "")
			day = label
		}
		if message.Event != "" {
			lines = append(lines, center(text(p.muted).Render(fit("* "+m.eventText(message, partner), width-2))))
			continue
		}
		mine := message.UserID == m.account.user.ID
		align := func(line string) string {
			if mine {
				return lipgloss.PlaceHorizontal(width-1, lipgloss.Right, line)
			}
			return " " + line
		}
		startsRun := i == 0 || !sameRun(t.messages[i-1], message)
		endsRun := i == len(t.messages)-1 || !sameRun(message, t.messages[i+1])
		switch {
		case startsRun && !mine:
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			sender := m.messageSender(message, t)
			name := m.handleOf(sender)
			if sender != nil && sender.Name != "" {
				name = safeText(sender.Name)
			}
			req, avatar := m.userAvatar(sender, 2, 1, p)
			t.media = append(t.media, remoteMedia{len(lines), len(lines) + 1, req})
			lines = append(lines, align(avatar[0]+" "+text(p.ink).Bold(true).Render(fit(name, bubble-3))))
		case startsRun:
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
		default:
			lines = append(lines, "")
		}
		spanStart := len(lines)
		if reply := message.ReplyTo; reply != nil {
			excerpt := strings.Join(strings.Fields(safeText(reply.Body)), " ")
			if excerpt == "" && reply.HasVoice {
				excerpt = "voice note"
			} else if excerpt == "" && reply.ImagesCount > 0 {
				excerpt = "image"
			}
			lines = append(lines, align(text(p.muted).Render(fit("↳ "+excerpt, bubble))))
		}
		mentioned := style
		mentioned.mentions = make(map[string]bool, len(message.Mentions))
		for _, mention := range message.Mentions {
			mentioned.mentions[strings.ToLower(mention.Handle)] = true
		}
		colors, foreground, background := p, p.ink, p.wash
		if mine {
			colors, foreground, background = own, p.shell, p.blue
		}
		body := bodyLines(api.Commit{ID: fmt.Sprint("message-", message.ID), Body: message.Body, Segments: message.Segments}, bubble-2, colors, mentioned)
		for _, line := range bubbleLines(body, foreground, background) {
			lines = append(lines, align(line))
		}
		if len(message.Images) > 0 {
			images := make([]api.Image, len(message.Images))
			for j, image := range message.Images {
				images[j] = api.Image{URL: image.URL, Width: image.Width, Height: image.Height, Description: image.Description}
			}
			requests := imageRequests(images, bubble)
			picture := pictureLines(images, requests, p, m.images)
			if picture == nil {
				picture = []string{text(p.muted).Render(fmt.Sprintf("▧ %d image%s", len(images), map[bool]string{true: "", false: "s"}[len(images) == 1]))}
			}
			for _, req := range requests {
				t.media = append(t.media, remoteMedia{len(lines), len(lines) + len(picture), req})
			}
			for _, line := range picture {
				lines = append(lines, align(line))
			}
		}
		if card := linkCardLines(message.LinkPreview, min(bubble, 64), p, m.images); card != nil {
			if req, ok := linkCardRequest(message.LinkPreview); ok {
				t.media = append(t.media, remoteMedia{len(lines), len(lines) + len(card), req})
			}
			for _, line := range card {
				lines = append(lines, align(line))
			}
		}
		if message.Voice != nil {
			line, play, bars, wave := m.voiceLine(message, bubble, foreground, background, p)
			left := 1
			if mine {
				left = width - 1 - lipgloss.Width(line)
			}
			t.voices = append(t.voices, voiceArea{len(lines), left + play, left + bars, wave, message.ID})
			lines = append(lines, align(line))
		}
		if chips, spans := reactionChips(message.Reactions, m.account.user.ID, p); chips != nil {
			left := 1
			if mine {
				left = width - 1 - lipgloss.Width(chips[0])
			}
			for _, span := range spans {
				t.chips = append(t.chips, chipArea{len(lines), left + span.x, span.w, message.ID, span.emoji})
			}
			for _, line := range chips {
				lines = append(lines, align(line))
			}
		}
		t.spans[fmt.Sprint("m:", message.ID)] = [2]int{spanStart, len(lines)}
		if !endsRun {
			continue
		}
		footer := text(p.muted).Render(message.CreatedAt.In(time.Local).Format("3:04 PM"))
		if mine && i == len(t.messages)-1 && t.conversation.Kind != "org" && len(m.messaging.pending[t.conversation.ID]) == 0 {
			state := "✓ pushed"
			if t.data.PartnerReadThroughID >= message.ID {
				state = "✓✓ pulled"
			}
			footer = text(p.muted).Render(state) + "  " + footer
		}
		lines = append(lines, align(footer))
	}
	for _, pending := range m.messaging.pending[t.conversation.ID] {
		lines = append(lines, "")
		start := len(lines)
		lines = append(lines, m.pendingLines(pending, width, bubble, p)...)
		t.spans["p:"+pending.clientID] = [2]int{start, len(lines)}
	}
	if span, ok := t.spans[t.selected]; ok {
		markSelected(lines, span, width, p)
	}
	switch {
	case t.status != "":
		lines = append(lines, "", "  "+text(p.muted).Render(fit(t.status, width-4)))
	case !t.loaded:
		lines = append(lines, "", "  "+text(p.muted).Render("Reading the conversation from /dev/null..."))
	case len(t.messages) == 0 && len(m.messaging.pending[t.conversation.ID]) == 0:
		lines = append(lines, "", center(text(p.muted).Render("nothing pushed yet")))
	}
	t.viewport.SetContent(strings.Join(lines, "\n"))
}

// bubbleLines pads a message body into a block of one background, with two cells of space on each side and a row
// above and below, like the web's padded bubbles.
func bubbleLines(body []string, foreground, background string) []string {
	if len(body) == 0 {
		return nil
	}
	widest := 0
	for _, line := range body {
		widest = max(widest, ansi.StringWidth(line))
	}
	blank := paintLine(strings.Repeat(" ", widest+4), foreground, background)
	result := []string{blank}
	for _, line := range body {
		result = append(result, paintLine("  "+line+strings.Repeat(" ", widest-ansi.StringWidth(line))+"  ", foreground, background))
	}
	return append(result, blank)
}

// paintLine sets a line's default colours, reapplied after the resets of the styles nested inside it.
func paintLine(line, foreground, background string) string {
	colours := ansi.Style{}.ForegroundColor(lipgloss.Color(foreground)).BackgroundColor(lipgloss.Color(background)).String()
	return colours + sgrSequence.ReplaceAllStringFunc(line, func(sgr string) string {
		if clearsBackground(sgr[2 : len(sgr)-1]) {
			return sgr + colours
		}
		return sgr
	}) + ansi.ResetStyle
}

// chipSpan is where one reaction chip sits on its row.
type chipSpan struct {
	x, w  int
	emoji string
}

// reactionChips are the web's outlined reaction pills: one rounded box per emoji with its count, a cell apart, in
// the order each was first left. The user's own reactions get a blue outline.
func reactionChips(reactions []api.Reaction, user string, p palette) ([]string, []chipSpan) {
	var order []string
	counts, mine := map[string]int{}, map[string]bool{}
	for _, reaction := range reactions {
		emoji := safeText(reaction.Emoji)
		if counts[emoji] == 0 {
			order = append(order, emoji)
		}
		counts[emoji]++
		mine[emoji] = mine[emoji] || reaction.UserID == user
	}
	if len(order) == 0 {
		return nil, nil
	}
	var parts []string
	var spans []chipSpan
	x := 0
	for i, emoji := range order {
		if i > 0 {
			parts = append(parts, " ")
			x++
		}
		border := p.hairline
		if mine[emoji] {
			border = p.blue
		}
		chip := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(border)).Foreground(lipgloss.Color(p.ink)).Padding(0, 1).Render(fmt.Sprintf("%s %d", emojiPresentation(emoji), counts[emoji]))
		parts = append(parts, chip)
		spans = append(spans, chipSpan{x, lipgloss.Width(chip), reactionOriginal(reactions, emoji)})
		x += lipgloss.Width(chip)
	}
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, parts...), "\n"), spans
}

// reactionOriginal is the emoji as the server sent it, for toggling a chip whose label was made safe to draw.
func reactionOriginal(reactions []api.Reaction, shown string) string {
	for _, reaction := range reactions {
		if safeText(reaction.Emoji) == shown {
			return reaction.Emoji
		}
	}
	return shown
}

// userAvatar is a Remotes user's picture, or their lane dot until it is ready.
func (m AppModel) userAvatar(u *api.User, cols, rows int, p palette) (graphics.Request, []string) {
	lines := make([]string, rows)
	if u == nil {
		for i := range lines {
			lines[i] = strings.Repeat(" ", cols)
		}
		return graphics.Request{}, lines
	}
	lane := laneColor(u.Lane, p)
	req := m.avatarRequest(u.AvatarURL, lane, cols, rows)
	ready := m.images != nil
	for i := range lines {
		if ready {
			lines[i] = m.images.Row(req, i)
			ready = lines[i] != ""
		}
	}
	if !ready {
		for i := range lines {
			lines[i] = strings.Repeat(" ", cols)
		}
		lines[0] = text(lane).Render("●") + strings.Repeat(" ", cols-1)
	}
	return req, lines
}

func (m AppModel) remotesMedia() []graphics.Request {
	p := colors(m.account.dark)
	var requests []graphics.Request
	add := func(u *api.User, cols, rows int) {
		if u != nil {
			req, _ := m.userAvatar(u, cols, rows, p)
			requests = append(requests, req)
		}
	}
	t := m.remotes.thread
	if t == nil || m.remotesSplit() {
		start, end := m.inboxWindow(max(1, m.mainHeight()-remotesHeaderHeight))
		for _, c := range m.remotes.inbox[start:end] {
			add(m.conversationFace(c), 4, 2)
		}
	}
	if t == nil {
		return requests
	}
	add(m.conversationFace(t.conversation), 2, 1)
	top, bottom := t.viewport.YOffset(), t.viewport.YOffset()+t.viewport.Height()
	for _, media := range t.media {
		if media.req.URL != "" && media.end > top && media.start < bottom {
			requests = append(requests, media.req)
		}
	}
	return requests
}

func (m AppModel) remotesModal(p palette) (string, uiRect, []hitArea) {
	r := m.remotes
	c := r.dialogTarget
	title := m.conversationTitle(c)
	var body string
	var areas []hitArea
	var footer string
	height := 0
	switch r.dialog {
	case "menu", "mute":
		var labels, keys []string
		if r.dialog == "menu" {
			for _, item := range m.conversationMenu(c) {
				labels, keys = append(labels, item.label), append(keys, item.key)
			}
		} else {
			for _, choice := range muteChoices {
				labels, keys = append(labels, choice.label), append(keys, "")
			}
			body = text(p.muted).Render("Silence browser notifications and sound on all your devices. Messages and unread counts stay visible.") + "\n\n"
		}
		rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 48, len(labels)+7)
		width := max(1, rect.w-4)
		if body != "" {
			body = lipgloss.NewStyle().Width(width).Render(strings.TrimSuffix(body, "\n\n")) + "\n\n"
		}
		// The wrapped description sets the height, so the last choice is not cut off.
		height = len(labels) + 6 + lipgloss.Height(body)
		rect = centeredRect(max(1, m.account.width), max(1, m.account.height), 48, height)
		// The body ends with a blank line, which lipgloss.Height counts as one more row.
		offset := max(0, lipgloss.Height(body)-1)
		for i, label := range labels {
			line := split(label, text(p.muted).Render(keys[i]), width-2)
			style := lipgloss.NewStyle()
			if i == r.dialogFocus {
				line = withBackground(fit(" "+line+" ", width), p.soft)
			} else {
				line = " " + line
			}
			body += style.Render(line) + "\n"
			areas = append(areas, hitArea{uiRect{rect.x + 2, rect.y + 3 + offset + i, width, 1}, "focus", i})
		}
		footer = text(p.muted).Render(fit("enter choose · esc close", width))
		heading := title
		if r.dialog == "mute" {
			heading = "Mute " + title
		}
		areas = append(areas, hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
		return panel(heading, strings.TrimSuffix(body, "\n"), footer, rect, p, p.hairline), rect, areas
	}
	verb, question, description := "Block", "Block "+title+"?", "This conversation goes away for both of you, and neither of you can message, follow, reply to, or star the other. They won't be told. You can unblock them any time from your settings."
	switch {
	case r.dialog == "drop":
		verb, question, description = "Drop", "Drop "+title+"'s request?", "It leaves your Fetched list without an answer. They won't be told."
	case c.Kind == "org":
		verb, question, description = "Leave", "Leave "+title+"?", "You stop seeing the org and its history. A maintainer would have to add you back. If you are its only maintainer, the longest-standing member takes over."
	}
	rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 13)
	width := max(1, rect.w-4)
	body = text(p.ink).Bold(true).Render(fit(question, width)) + "\n\n" + text(p.muted).Render(lipgloss.NewStyle().Width(width).Render(description))
	footer, footerAreas := modalFooter([]modalAction{{"Cancel", "esc", "esc"}, {verb, "enter", "confirm"}}, width, r.dialogFocus+1, p)
	for i := range footerAreas {
		footerAreas[i].x += rect.x + 2
		footerAreas[i].y += rect.y + rect.h - 1 - lipgloss.Height(footer)
	}
	areas = append(footerAreas, hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
	return panel("/dev/null · "+strings.ToUpper(verb), body, footer, rect, p, p.danger), rect, areas
}

func (m AppModel) remotesDialogMouse(x, y int) (AppModel, tea.Cmd) {
	_, rect, areas := m.remotesModal(colors(m.account.dark))
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		switch area.action {
		case "esc":
			return m.mouseKey("esc")
		case "confirm":
			m.remotes.dialogFocus = 1
		default:
			m.remotes.dialogFocus = area.index
		}
		return m.mouseKey("enter")
	}
	if !rect.contains(x, y) {
		m.remotes.dialog = ""
	}
	return m, nil
}

// remotesMouse handles a click at x, relative to the start of the Remotes column.
func (m AppModel) remotesMouse(x, y int) (AppModel, tea.Cmd) {
	if y < remotesHeaderHeight || y >= m.mainHeight() {
		return m, nil
	}
	p, row := colors(m.account.dark), y-remotesHeaderHeight
	inbox := m.remotesSplit() || m.remotes.thread == nil
	if inbox && (!m.remotesSplit() || x < inboxPaneWidth) {
		width := m.remotesWidth()
		if m.remotesSplit() {
			width = inboxPaneWidth
		}
		_, areas := m.inboxLayout(width, m.mainHeight()-remotesHeaderHeight, p)
		for _, area := range areas {
			if !area.contains(x, row) {
				continue
			}
			switch area.action {
			case "tab":
				m.remotes.focusThread = false
				tabs := m.remotesTabs()
				current := 0
				for i, tab := range tabs {
					if tab == m.remotes.tab {
						current = i
					}
				}
				return m, m.switchInboxTab(area.index - current)
			case "/":
				m.remotes.searching, m.remotes.query = true, m.remotes.term
				return m, nil
			case "row":
				m.remotes.selected = area.index
				return m, m.openRemoteThread(m.remotes.inbox[area.index])
			case "person":
				m.remotes.selected = area.index
				return m.remotesKey("enter")
			}
		}
		return m, nil
	}
	m.remotes.focusThread = true
	left := 0
	if m.remotesSplit() {
		left = inboxPaneWidth + 1
	}
	if row == 0 && x-left >= m.threadPaneWidth()-4 {
		cmd, _ := m.conversationActionKey(".")
		return m, cmd
	}
	t := m.remotes.thread
	if row == 0 && t != nil && t.conversation.Kind == "org" {
		// The header reads " ● title  org · N members"; the member count opens Members.
		if x-left >= 6+ansi.StringWidth(m.conversationTitle(t.conversation)) {
			m.openMembers()
		}
		return m, nil
	}
	if t == nil || t.composer.browser != nil {
		return m, nil
	}
	x -= left
	banner := m.bannerRows()
	if row == 2 && banner > 0 {
		m.openPinList()
		return m, nil
	}
	if line := t.viewport.YOffset() + row - 2 - banner; row >= 2+banner && row < 2+banner+t.viewport.Height() {
		return m, m.threadClick(line, x)
	}
	_, areas := m.messageComposerView(m.threadPaneWidth(), p)
	for _, area := range areas {
		if !area.contains(x, row-3-banner-t.viewport.Height()) {
			continue
		}
		switch area.action {
		case "write":
			return m, m.focusMessageComposer()
		case "ping":
			return m, m.sendPing()
		case "merge", "drop":
			return m.remoteThreadKey(area.action[:1])
		case "react":
			quick := quickReactions(m.messaging.usage)
			if area.index == len(quick) {
				t.react.picking = true
				return m, nil
			}
			return m, m.chooseReaction(quick[area.index])
		}
	}
	return m, nil
}

// threadClick toggles the reaction chip under the pointer, or puts the cursor on the message there.
func (m *AppModel) threadClick(line, x int) tea.Cmd {
	t := m.remotes.thread
	// While the microphone is open, a click could play a note into the recording, so the thread waits.
	if rec := t.composer.recording; rec != nil && rec.recorder != nil {
		return nil
	}
	for _, area := range t.voices {
		if area.line == line && x >= area.play && x < area.bars+area.width {
			if t.composer.focused {
				m.blurMessageComposer()
			}
			return m.voiceClick(area, x)
		}
	}
	for _, chip := range t.chips {
		if chip.line <= line && line < chip.line+3 && chip.x <= x && x < chip.x+chip.w {
			return m.toggleReaction(chip.id, chip.emoji)
		}
	}
	for key, span := range t.spans {
		if span[0] <= line && line < span[1] {
			if t.composer.focused {
				m.blurMessageComposer()
			}
			t.selected = key
			m.renderRemoteThread()
			return nil
		}
	}
	return nil
}

func (m AppModel) remotesBindings(bind func(string, string) key.Binding) appKeys {
	short := []key.Binding{bind("j/k", "move"), bind("enter", "open"), bind("tab", "inbox tab"), bind("/", "search"), bind(".", "actions"), bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit")}
	thread := m.remotes.thread != nil && m.remotes.focusThread
	if thread {
		short = []key.Binding{bind("j/k", "select"), bind("w", "write"), bind("ctrl+t", "voice"), bind("r", "reply"), bind("e", "react"), bind(".", "actions"), bind("esc", "inbox"), bind("?", "help"), bind("ctrl+c", "quit")}
	}
	all := append([]key.Binding(nil), short...)
	if thread {
		all = append(all, bind("enter", "push"), bind("shift+enter / ctrl+j", "new line"), bind("esc", "cancel the reply, then back to the messages"), bind("ctrl+o", "add an image"), bind("tab", "edit images / write"), bind("1 / 2 / 3", "quick reaction"), bind("p", "ping"), bind("r / d", "retry / discard a rejected push"), bind("space", "play or pause a voice note"), bind("ctrl+t", "record a voice note, up to a minute"), bind("left / right", "seek a playing note 2s"), bind("x", "voice note speed"), bind("- / +", "voice note volume"), bind("G", "newest message"), bind("n", "older messages"), bind("I", "org members: rename, add, roles, remove"), bind("!", "pin or unpin a message for the org (maintainers)"), bind("L", "pinned messages"), bind("m / d", "merge or drop a request"))
	}
	all = append(all, bind("N", "new remote"), bind("O", "new org"), bind("P", "pin or unpin"), bind("S", "stash or pop"), bind("M", "mute or unmute"), bind("B", "block or leave org"), bind("u", "partner profile"), bind("ctrl+r", "refresh"), bind("[ / ]", "previous / next tab"), bind("n", "more conversations"), bind("home / G / end", "first / last"), bind("pgup/pgdown", "move five"), bind("g r", "Remotes"), bind("g l / g e", "Log / Trending"), bind("g n", "notifications"), bind("g p", "your profile"), bind("c", "compose"), bind("a", "account"), bind("f2", m.account.nextTheme()+" theme"), bind("click", "open a conversation or tab"))
	return appKeys{short, all}
}
