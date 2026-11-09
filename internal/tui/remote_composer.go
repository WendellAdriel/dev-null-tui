package tui

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/audio"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// The message composer follows the web's Composer in resources/js/components/chat/conversation-thread.tsx and the
// sending in resources/js/lib/chat.ts.

// pendingMessage is a push the server hasn't stored yet. It keeps its client id, so a retry can't make a duplicate.
type pendingMessage struct {
	clientID string
	draft    api.NewMessage
	reply    *api.Message
	status   string
	reason   string
	at       time.Time
}

type messageComposer struct {
	input               textarea.Model
	focused             bool
	reply               *api.Message
	replyID             int64
	images              []api.ImageFile
	stripFocus, editing bool
	strip               int
	field               textinput.Model
	browser             *fileBrowser
	suggest             completion
	status              string
	typedAt             time.Time
	recording           *voiceRecording
	voiceConfirm        bool
}

// messagingState outlives the Remotes screen, so a push still in flight or a draft survives closing it.
type messagingState struct {
	pending        map[string][]pendingMessage
	drafts         map[string]auth.MessageDraft
	usage          []api.EmojiUse
	usageAsked     bool
	saveGeneration int
}

type remoteSendResult struct {
	session      int
	conversation api.Conversation
	clientID     string
	message      api.Message
	err          error
}

type remotePingResult struct {
	session      int
	conversation api.Conversation
	message      api.Message
	err          error
}

type reactionUsageResult struct {
	session int
	usage   []api.EmojiUse
}

type draftSaveTick struct{ session, generation int }

const (
	typingEvery     = 3 * time.Second
	draftSaveDelay  = time.Second
	composerMaxRows = 5
	// messageCountFrom is where the web starts showing the character count, at 80% of the limit.
	messageCountFrom = api.MaxMessageLength * 8 / 10
)

// newClientID is a random version 4 UUID, which the server takes as the message's client_id.
func newClientID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (m *AppModel) startMessaging() {
	m.messaging = messagingState{pending: map[string][]pendingMessage{}, drafts: m.account.store.MessageDrafts(m.account.user.ID)}
}

func (m AppModel) messagePlaceholder(c api.Conversation) string {
	if c.Kind != "org" {
		return m.conversationTitle(c)
	}
	if c.Name != nil && strings.TrimSpace(*c.Name) != "" {
		return safeText(*c.Name)
	}
	return "the org"
}

func (m AppModel) newMessageInput(c api.Conversation) textarea.Model {
	input := textarea.New()
	input.Prompt, input.Placeholder = "", "Message "+m.messagePlaceholder(c)
	input.ShowLineNumbers = false
	input.CharLimit, input.MaxWidth = 0, 0
	input.DynamicHeight, input.MinHeight, input.MaxHeight = true, 1, composerMaxRows
	// Enter pushes, so a new line takes Shift+Enter, as on the web, or Ctrl+J where the terminal can't tell them apart.
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j", "alt+enter"))
	p := m.account.colors()
	styles := textarea.DefaultStyles(m.account.dark)
	for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		state.Base = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink))
		state.Text, state.CursorLine = lipgloss.NewStyle(), lipgloss.NewStyle()
		state.Placeholder = text(p.muted)
		state.EndOfBuffer = text(p.muted)
	}
	input.SetStyles(styles)
	return input
}

// restoreComposer rebuilds a conversation's draft. Images are read again from disk, and the ones that moved or changed
// type are dropped with a note.
func (m AppModel) restoreComposer(c api.Conversation) messageComposer {
	mc := messageComposer{input: m.newMessageInput(c)}
	draft, ok := m.messaging.drafts[c.ID]
	if !ok {
		return mc
	}
	mc.input.SetValue(draft.Body)
	mc.replyID = draft.ReplyToID
	dropped := 0
	for _, image := range draft.Images {
		file, err := api.ReadImage(image.Path)
		if err != nil || len(mc.images) >= api.MaxImages {
			dropped++
			continue
		}
		file.Description = image.Description
		mc.images = append(mc.images, file)
	}
	switch {
	case dropped == 1:
		mc.status = "An image in this draft can't be read any more, so it was dropped."
	case dropped > 1:
		mc.status = fmt.Sprintf("%d images in this draft can't be read any more, so they were dropped.", dropped)
	}
	return mc
}

func (mc messageComposer) replyTarget() int64 {
	if mc.reply != nil {
		return mc.reply.ID
	}
	return mc.replyID
}

// keepMessageDraft copies the open thread's composer into the drafts. A draft with nothing typed or attached is
// removed, since a reply target alone isn't worth keeping.
func (m *AppModel) keepMessageDraft() {
	t := m.remotes.thread
	if t == nil || m.messaging.drafts == nil {
		return
	}
	mc := t.composer
	draft := auth.MessageDraft{Body: mc.input.Value(), ReplyToID: mc.replyTarget()}
	for _, image := range mc.images {
		draft.Images = append(draft.Images, auth.DraftImage{Path: image.Path, Description: image.Description})
	}
	if strings.TrimSpace(draft.Body) == "" && len(draft.Images) == 0 {
		delete(m.messaging.drafts, t.conversation.ID)
		return
	}
	m.messaging.drafts[t.conversation.ID] = draft
}

// saveMessageDrafts writes the drafts to disk. It stays off while an account is on its way out, so a late save can't
// bring back drafts that disconnecting just deleted.
func (m *AppModel) saveMessageDrafts() {
	if m.messaging.drafts == nil || !m.connected || m.settings.signingOut() {
		return
	}
	if err := m.account.store.SaveMessageDrafts(m.account.user.ID, m.messaging.drafts); err != nil {
		if t := m.remotes.thread; t != nil {
			t.composer.status = "Couldn't save this draft on this machine."
		}
	}
}

func (m *AppModel) flushMessageDrafts() {
	m.keepMessageDraft()
	m.saveMessageDrafts()
}

func (m *AppModel) quit() tea.Cmd {
	m.flushMessageDrafts()
	m.cancelRecording()
	if m.voice.volumePending {
		_ = m.account.store.SaveTUISettings(m.tuiSettings)
	}
	m.account.life.cancel()
	return tea.Quit
}

func (m *AppModel) scheduleDraftSave() tea.Cmd {
	m.messaging.saveGeneration++
	session, generation := m.session, m.messaging.saveGeneration
	return tea.Tick(draftSaveDelay, func(time.Time) tea.Msg { return draftSaveTick{session, generation} })
}

func (m *AppModel) acceptDraftSave(msg draftSaveTick) {
	if msg.session == m.session && msg.generation == m.messaging.saveGeneration {
		m.flushMessageDrafts()
	}
}

// messageComposer is the open thread's composer while it takes typed keys.
func (m AppModel) activeMessageComposer() *messageComposer {
	t := m.remotes.thread
	if !m.connected || m.accountOpen || !m.remotesOnTop() || m.settings.open || t == nil || !m.remotes.focusThread || !t.composer.focused || m.remotes.dialog != "" || t.react != nil {
		return nil
	}
	if m.help || m.whatsNew.open || m.upgradeVisible() || m.viewer.open || m.manualLink != "" || m.vote.commitID != "" {
		return nil
	}
	return &t.composer
}

// canPush reports a thread the user can write to: loaded, and open to them.
func (t *remoteThreadState) canPush() bool { return t.loaded && t.data.Access == "open" }

func (m *AppModel) focusMessageComposer() tea.Cmd {
	t := m.remotes.thread
	if t == nil || !t.canPush() {
		return nil
	}
	t.composer.focused, t.composer.stripFocus = true, false
	m.resizeThread()
	return t.composer.input.Focus()
}

func (m *AppModel) blurMessageComposer() tea.Cmd {
	t := m.remotes.thread
	t.composer.focused, t.composer.stripFocus = false, false
	t.composer.suggest.close()
	t.composer.input.Blur()
	m.resizeThread()
	m.flushMessageDrafts()
	return nil
}

func (m *AppModel) messageComposerKey(msg tea.KeyPressMsg) tea.Cmd {
	t := m.remotes.thread
	mc := &t.composer
	pressed := msg.String()
	if mc.browser != nil {
		return m.browserKey(msg)
	}
	if mc.editing {
		return m.messageDescriptionKey(msg)
	}
	if cmd, ok := m.recordingKey(pressed); ok {
		return cmd
	}
	switch pressed {
	case "esc":
		switch {
		case mc.suggest.count() > 0:
			mc.suggest.dismiss()
		case mc.stripFocus:
			mc.stripFocus = false
		case mc.reply != nil || mc.replyID != 0:
			mc.reply, mc.replyID = nil, 0
			m.resizeThread()
			return m.scheduleDraftSave()
		default:
			return m.blurMessageComposer()
		}
		m.resizeThread()
		return nil
	case "ctrl+o":
		return m.openMessageBrowser()
	case "tab", "shift+tab":
		if pressed != "tab" || mc.suggest.count() == 0 {
			if len(mc.images) > 0 {
				mc.stripFocus = !mc.stripFocus
				mc.suggest.close()
				mc.strip = max(0, min(mc.strip, len(mc.images)-1))
			}
			m.resizeThread()
			return nil
		}
	}
	if mc.stripFocus {
		return m.messageStripKey(pressed)
	}
	if mc.suggest.count() > 0 {
		switch pressed {
		case "down", "right":
			mc.suggest.move(1)
			return nil
		case "up", "left":
			mc.suggest.move(-1)
			return nil
		case "tab", "enter":
			acceptCompletion(&mc.suggest, &mc.input)
			lookup := m.refreshMessageSuggestions()
			m.resizeThread()
			return tea.Batch(lookup, m.scheduleDraftSave())
		}
	}
	if pressed == "enter" || pressed == "ctrl+s" {
		return m.pushMessage()
	}
	return m.updateMessageInput(msg)
}

func (m *AppModel) updateMessageInput(msg tea.Msg) tea.Cmd {
	t := m.remotes.thread
	mc := &t.composer
	before := mc.input.Value()
	var cmd tea.Cmd
	mc.input, cmd = mc.input.Update(msg)
	changed := mc.input.Value() != before
	if changed && !mc.input.HasSelection() {
		name, closed := emojiAtCaret(textBeforeCaret(mc.input))
		if value, ok := emojiCatalog[name]; closed && ok {
			replaceShortcode(&mc.input, name, value, true)
		}
	}
	lookup := m.refreshMessageSuggestions()
	m.resizeThread()
	if !changed {
		return tea.Batch(cmd, lookup)
	}
	mc.status = ""
	return tea.Batch(cmd, lookup, m.sendTyping(), m.scheduleDraftSave())
}

// refreshMessageSuggestions updates the message composer's emoji, mention, and issue suggestions.
func (m *AppModel) refreshMessageSuggestions() tea.Cmd {
	mc := &m.remotes.thread.composer
	if !mc.focused {
		mc.suggest.close()
		return nil
	}
	return m.refreshCompletion(&mc.suggest, mc.input, "message", m.messageMentionScope())
}

// messagePaste attaches dropped image files, and refuses a paste that would go past the limit, as the web does.
func (m *AppModel) messagePaste(msg tea.PasteMsg) tea.Cmd {
	mc := &m.remotes.thread.composer
	switch {
	case mc.browser != nil:
		m.filterBrowser(strings.Join(strings.Fields(msg.Content), " "))
		return nil
	case mc.editing:
		var cmd tea.Cmd
		mc.field, cmd = mc.field.Update(msg)
		return cmd
	}
	if paths, ok := api.DroppedPaths(msg.Content); ok {
		m.attachMessageImages(paths)
		return m.scheduleDraftSave()
	}
	if total := api.MessageLength(mc.input.Value() + msg.Content); total > api.MaxMessageLength {
		mc.status = fmt.Sprintf("fatal: paste rejected, message would be %s/%s chars", groupDigits(total), groupDigits(api.MaxMessageLength))
		return nil
	}
	return m.updateMessageInput(msg)
}

func groupDigits(n int) string {
	digits := fmt.Sprint(n)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func (m *AppModel) attachMessageImages(paths []string) string {
	mc := &m.remotes.thread.composer
	reason := ""
	for _, path := range paths {
		if len(mc.images) >= api.MaxImages {
			reason = imageLimitReason
			break
		}
		file, err := api.ReadImage(path)
		if err != nil {
			reason = sentence(err.Error())
			break
		}
		mc.images = append(mc.images, file)
	}
	mc.status = reason
	m.resizeThread()
	return reason
}

func (m *AppModel) openMessageBrowser() tea.Cmd {
	mc := &m.remotes.thread.composer
	if len(mc.images) >= api.MaxImages {
		mc.status = imageLimitReason
		return nil
	}
	b := &fileBrowser{}
	if err := b.load(m.browserStart()); err != nil {
		mc.status = "That folder cannot be read."
		return nil
	}
	mc.browser = b
	mc.suggest.close()
	mc.input.Blur()
	return nil
}

func (m *AppModel) messageStripKey(pressed string) tea.Cmd {
	mc := &m.remotes.thread.composer
	switch pressed {
	case "left", "up":
		mc.strip = max(0, mc.strip-1)
	case "right", "down":
		mc.strip = min(len(mc.images)-1, mc.strip+1)
	case "enter":
		field := textinput.New()
		field.Prompt, field.CharLimit, field.Placeholder = "", api.MaxDescriptionLength, "Describe the image for screen readers"
		field.SetValue(mc.images[mc.strip].Description)
		field.SetWidth(max(1, m.threadPaneWidth()-16))
		mc.field, mc.editing = field, true
		return mc.field.Focus()
	case "delete", "backspace", "x":
		mc.images = slices.Delete(mc.images, mc.strip, mc.strip+1)
		mc.strip = max(0, min(mc.strip, len(mc.images)-1))
		if len(mc.images) == 0 {
			mc.stripFocus = false
		}
		m.resizeThread()
		return m.scheduleDraftSave()
	}
	return nil
}

func (m *AppModel) messageDescriptionKey(msg tea.KeyPressMsg) tea.Cmd {
	mc := &m.remotes.thread.composer
	switch msg.String() {
	case "enter":
		mc.images[mc.strip].Description = mc.field.Value()
		mc.editing = false
		return m.scheduleDraftSave()
	case "esc":
		mc.editing = false
		return nil
	}
	var cmd tea.Cmd
	mc.field, cmd = mc.field.Update(msg)
	return cmd
}

// sendTyping tells the other side the user is typing, at most every three seconds like the web, and only in a
// conversation that has started.
func (m *AppModel) sendTyping() tea.Cmd {
	t := m.remotes.thread
	mc := &t.composer
	now := time.Now()
	if m.remoteCalls.typing == nil || strings.TrimSpace(mc.input.Value()) == "" || !t.canPush() || len(t.messages) == 0 || now.Sub(mc.typedAt) < typingEvery {
		return nil
	}
	mc.typedAt = now
	target, typing, life := t.conversation.Target(), m.remoteCalls.typing, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 10*time.Second)
		defer cancel()
		// A lost typing signal only hides the hint for a few seconds, so its error is not shown.
		_ = typing(ctx, target)
		return nil
	})
}

// pushMessage stages the draft and sends it. The composer clears at once, and the message waits in the thread as
// staged until the server answers.
func (m *AppModel) pushMessage() tea.Cmd {
	t := m.remotes.thread
	mc := &t.composer
	if !t.canPush() || m.remoteCalls.push == nil {
		return nil
	}
	if rec := mc.recording; rec != nil {
		if rec.samples == nil {
			return nil
		}
		m.cancelRecording()
		return m.stagePush(api.NewMessage{ClientID: newClientID(), ReplyToID: mc.replyTarget(), Voice: &api.VoiceNote{WAV: audio.EncodeWAV(rec.samples), Waveform: rec.waveform}})
	}
	var images []api.ImageFile
	for _, image := range mc.images {
		fresh, err := api.ReadImage(image.Path)
		if err != nil || !bytes.Equal(fresh.Data, image.Data) {
			continue
		}
		fresh.Description = image.Description
		images = append(images, fresh)
	}
	if len(images) != len(mc.images) {
		mc.images, mc.stripFocus, mc.editing = images, false, false
		mc.strip = max(0, min(mc.strip, len(images)-1))
		mc.status = "An attached image moved or changed, so it was dropped. Check the draft before pushing."
		m.resizeThread()
		return m.scheduleDraftSave()
	}
	mc.images = images
	draft := api.NewMessage{Body: mc.input.Value(), ClientID: newClientID(), ReplyToID: mc.replyTarget(), Images: mc.images}
	if err := draft.Validate(); err != nil {
		if api.MessageLength(draft.Body) > 0 || len(draft.Images) > 0 {
			mc.status = sentence(err.Error())
		}
		return nil
	}
	return m.stagePush(draft)
}

// stagePush draws the message as staged at once, clears the composer and its draft, and sends it.
func (m *AppModel) stagePush(draft api.NewMessage) tea.Cmd {
	t := m.remotes.thread
	mc := &t.composer
	pending := pendingMessage{clientID: draft.ClientID, draft: draft, reply: mc.reply, status: "staged", at: time.Now()}
	id := t.conversation.ID
	m.messaging.pending[id] = append(m.messaging.pending[id], pending)
	t.composer = messageComposer{input: m.newMessageInput(t.conversation), focused: true}
	t.composer.input.SetWidth(max(1, m.threadPaneWidth()-2))
	t.selected = ""
	delete(m.messaging.drafts, id)
	m.saveMessageDrafts()
	m.renderRemoteThread()
	m.resizeThread()
	t.viewport.GotoBottom()
	return tea.Batch(t.composer.input.Focus(), m.deliver(t.conversation, pending))
}

func (m *AppModel) deliver(c api.Conversation, pending pendingMessage) tea.Cmd {
	session, push, life := m.session, m.remoteCalls.push, m.account.life
	timeout := 30 * time.Second
	if len(pending.draft.Images) > 0 || pending.draft.Voice != nil {
		timeout = auth.UploadTimeout
	}
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, timeout)
		defer cancel()
		message, err := push(ctx, c.Target(), pending.draft)
		return remoteSendResult{session: session, conversation: c, clientID: pending.clientID, message: message, err: err}
	})
}

func (m *AppModel) acceptRemoteSend(msg remoteSendResult) tea.Cmd {
	if msg.session != m.session || !m.connected {
		return nil
	}
	id := msg.conversation.ID
	list := m.messaging.pending[id]
	i := slices.IndexFunc(list, func(p pendingMessage) bool { return p.clientID == msg.clientID })
	if i < 0 {
		return nil
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		list[i].status, list[i].reason = "rejected", m.messageError(msg.err)
		m.renderRemoteThread()
		return nil
	}
	m.confirmPending(id, msg.clientID)
	m.touchConversation(msg.conversation, msg.message)
	return m.addThreadMessage(id, msg.message)
}

func (m *AppModel) confirmPending(conversation, clientID string) bool {
	if clientID == "" {
		return false
	}
	list := m.messaging.pending[conversation]
	i := slices.IndexFunc(list, func(p pendingMessage) bool { return p.clientID == clientID })
	if i < 0 {
		return false
	}
	if list = slices.Delete(list, i, i+1); len(list) == 0 {
		delete(m.messaging.pending, conversation)
	} else {
		m.messaging.pending[conversation] = list
	}
	if t := m.remotes.thread; t != nil && t.conversation.ID == conversation && t.selected == "p:"+clientID {
		t.selected = ""
	}
	return true
}

// addThreadMessage puts the user's own new message or ping in the open thread. The first message of a conversation
// asks the thread again, since it can change what the user may do next, such as waiting for a merge.
func (m *AppModel) addThreadMessage(conversation string, message api.Message) tea.Cmd {
	t := m.remotes.thread
	if t == nil || t.conversation.ID != conversation {
		return nil
	}
	started := len(t.messages) == 0
	atBottom := t.viewport.AtBottom()
	t.messages = mergeMessages(t.messages, []api.Message{message})
	t.data.ReadThroughID = max(t.data.ReadThroughID, message.ID)
	t.active = m.now
	m.renderRemoteThread()
	if atBottom {
		t.viewport.GotoBottom()
	}
	if started {
		return m.fetchRemoteThread("poll")
	}
	return nil
}

// touchConversation moves the conversation to the top with its new message. Pushing pops a stashed conversation, as
// the server does.
func (m *AppModel) touchConversation(c api.Conversation, message api.Message) {
	r := &m.remotes
	if !r.open {
		return
	}
	index := slices.IndexFunc(r.inbox, func(row api.Conversation) bool { return row.ID == c.ID })
	if index < 0 {
		if r.tab != api.Open || r.term != "" || !r.loaded {
			return
		}
		r.inbox = append([]api.Conversation{c}, r.inbox...)
		index = 0
		if len(r.inbox) > 1 {
			r.selected++
		}
	}
	row := &r.inbox[index]
	row.LastMessage, row.LastMessageAt, row.IsArchived = &message, &message.CreatedAt, false
	if r.tab == api.Stashed {
		r.inbox = slices.Delete(r.inbox, index, index+1)
		r.selected = max(0, min(r.selected, len(r.inbox)-1))
		return
	}
	m.resortInbox()
}

func (m AppModel) messageError(err error) string {
	var failure *auth.APIError
	if errors.As(err, &failure) {
		switch {
		case failure.Status == http.StatusTooManyRequests && failure.RetryAfter != "":
			return "rate limited, retry after " + safeText(failure.RetryAfter)
		case failure.Status == http.StatusUnprocessableEntity && len(failure.BodyErrors)+len(failure.FieldErrors) > 0:
			return safeText(strings.Join(append(failure.BodyErrors, failure.FieldErrors...), " "))
		case failure.Message != "":
			return safeText(failure.Message)
		case failure.Status == http.StatusNotFound:
			return "this conversation is gone"
		}
	}
	return "couldn't reach /dev/null"
}

func (m *AppModel) retryMessage(clientID string) tea.Cmd {
	t := m.remotes.thread
	list := m.messaging.pending[t.conversation.ID]
	i := slices.IndexFunc(list, func(p pendingMessage) bool { return p.clientID == clientID })
	if i < 0 || list[i].status != "rejected" {
		return nil
	}
	list[i].status, list[i].reason = "staged", ""
	m.renderRemoteThread()
	return m.deliver(t.conversation, list[i])
}

func (m *AppModel) discardMessage(clientID string) {
	t := m.remotes.thread
	id := t.conversation.ID
	list := m.messaging.pending[id]
	i := slices.IndexFunc(list, func(p pendingMessage) bool { return p.clientID == clientID })
	if i < 0 || list[i].status != "rejected" {
		return
	}
	if list = slices.Delete(list, i, i+1); len(list) == 0 {
		delete(m.messaging.pending, id)
	} else {
		m.messaging.pending[id] = list
	}
	t.selected = ""
	m.renderRemoteThread()
}

// sendPing pings the conversation. The server refuses a ping before anyone has said hi, so the key does nothing then.
func (m *AppModel) sendPing() tea.Cmd {
	t := m.remotes.thread
	if t == nil || !t.canPush() || m.remoteCalls.ping == nil {
		return nil
	}
	if len(t.messages) == 0 {
		t.composer.status = "Say hi before pinging."
		if c := t.conversation; c.Kind != "org" && c.Partner != nil {
			t.composer.status = "Say hi to @" + safeText(c.Partner.Handle) + " before pinging them."
		}
		return nil
	}
	c, session, ping, life := t.conversation, m.session, m.remoteCalls.ping, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		message, err := ping(ctx, c.Target())
		return remotePingResult{session: session, conversation: c, message: message, err: err}
	})
}

func (m *AppModel) acceptRemotePing(msg remotePingResult) tea.Cmd {
	if msg.session != m.session || !m.connected {
		return nil
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		reason := sentence(m.messageError(msg.err))
		if t := m.remotes.thread; t != nil && t.conversation.ID == msg.conversation.ID {
			t.composer.status = reason
			return nil
		}
		return m.notify("error", "Couldn't ping", reason)
	}
	m.touchConversation(msg.conversation, msg.message)
	return m.addThreadMessage(msg.conversation.ID, msg.message)
}

func (m *AppModel) askReactionUsage() tea.Cmd {
	if m.messaging.usageAsked || m.remoteCalls.usage == nil {
		return nil
	}
	m.messaging.usageAsked = true
	session, usage, life := m.session, m.remoteCalls.usage, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		list, err := usage(ctx)
		if err != nil {
			return nil
		}
		return reactionUsageResult{session: session, usage: list}
	})
}
