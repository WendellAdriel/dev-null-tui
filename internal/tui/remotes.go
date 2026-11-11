package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// remoteCalls are the Remotes requests, kept apart so tests can replace them.
type remoteCalls struct {
	inbox       func(context.Context, api.InboxTab, string, string) (api.Inbox, error)
	thread      func(context.Context, api.Target, api.ThreadQuery) (api.RemoteThread, error)
	markRead    func(context.Context, api.Target, int64) error
	stash       func(context.Context, api.Target, bool) error
	mute        func(context.Context, api.Target, api.MuteDuration) (*time.Time, error)
	pin         func(context.Context, api.Target, bool) (*time.Time, error)
	leave       func(context.Context, string, string) error
	block       func(context.Context, string) error
	linkPreview func(context.Context, int64) (*api.LinkPreview, error)
	push        func(context.Context, api.Target, api.NewMessage) (api.Message, error)
	typing      func(context.Context, api.Target) error
	ping        func(context.Context, api.Target) (api.Message, error)
	react       func(context.Context, int64, string, bool) ([]api.Reaction, error)
	usage       func(context.Context) ([]api.EmojiUse, error)
	merge       func(context.Context, string) error
	drop        func(context.Context, string) error
	candidates  func(context.Context, api.CandidateQuery) ([]api.Candidate, error)
	createOrg   func(context.Context, string, []string) (string, error)
	addMembers  func(context.Context, string, []string, bool) error
	rename      func(context.Context, string, string) error
	setRole     func(context.Context, string, string, api.OrgRole) error
	setPinned   func(context.Context, int64, bool) (api.MessagePins, error)
}

func newRemoteCalls(client *api.Client) remoteCalls {
	return remoteCalls{
		inbox: client.Inbox, thread: client.RemoteThread, markRead: client.MarkRead, stash: client.SetStashed,
		mute: client.SetMute, pin: client.SetConversationPin, leave: client.RemoveOrgMember, linkPreview: client.MessageLinkPreview,
		push: client.PushMessage, typing: client.SendTyping, ping: client.Ping, react: client.SetReaction, usage: client.ReactionUsage,
		merge: client.Merge, drop: client.Drop, candidates: client.Candidates, createOrg: client.CreateOrg, addMembers: client.AddOrgMembers,
		rename: client.RenameOrg, setRole: client.SetOrgRole, setPinned: client.SetMessagePinned,
		block: func(ctx context.Context, handle string) error {
			_, err := client.SetSafety(ctx, handle, "block", true)
			return err
		},
	}
}

type remoteThreadState struct {
	conversation   api.Conversation
	data           api.RemoteThread
	messages       []api.Message
	fetchedThrough int64
	reacting       map[int64]bool
	loading        bool
	loadingMode    string
	loaded         bool
	status         string
	generation     int
	cancel         context.CancelFunc
	viewport       viewport.Model
	// active is when the thread last changed, which keeps polling fast for a minute.
	active    time.Time
	media     []remoteMedia
	refreshed map[int64]bool
	composer  messageComposer
	react     *reactionMenu
	// selected is the item the message cursor is on, by itemKey, or empty while it follows the newest message.
	selected string
	spans    map[string][2]int
	chips    []chipArea
	// pendingKeys are the Retry and Discard chips on rejected pushes.
	pendingKeys []pendingKey
	// avatars are the senders' faces above their runs.
	avatars []avatarHit
	voices  []voiceArea
	// jumpTo is a pinned message to select once older pages bring it in.
	jumpTo int64
	// unseen counts messages from others that arrived while the reader was scrolled up, for the Back to present pill.
	unseen int
}

type remotesState struct {
	open           bool
	tab            api.InboxTab
	term, query    string
	searching      bool
	inbox          []api.Conversation
	hasMore        bool
	requests       int
	unread         int
	selected       int
	loading        bool
	loaded         bool
	status         string
	generation     int
	cancel         context.CancelFunc
	thread         *remoteThreadState
	focusThread    bool
	tickGeneration int
	// away marks a tick scheduled at the away pace, which focus restarts.
	away         bool
	ticking      bool
	lastInbox    time.Time
	dialog       string
	dialogFocus  int
	dialogTarget api.Conversation
	countCancel  context.CancelFunc
	people       *peopleDialog
	members      *membersDialog
	pinWarning   *pinWarning
	pinList      *pinList
	// candidates are the people the Open tab's search offers to start a conversation with, after its rows.
	candidates []api.Candidate
}

type remotesInboxResult struct {
	session, generation int
	mode                string
	inbox               api.Inbox
	err                 error
}

type remoteThreadResult struct {
	session, generation int
	mode                string
	thread              api.RemoteThread
	err                 error
}

type remotesTick struct{ session, generation int }

type remoteLinkPreviewResult struct {
	session, attempt int
	id               int64
	preview          *api.LinkPreview
	err              error
}

// linkPreviewDelays are the web's waits between asks for a card the server is still fetching (link-preview.tsx).
var linkPreviewDelays = []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second, 8 * time.Second, 13 * time.Second}

const (
	remotesFastPoll = 1500 * time.Millisecond
	remotesSlowPoll = 4 * time.Second
)

func (m AppModel) remotesEnabled() bool {
	return m.account.user.Remotes != nil && m.remoteCalls.inbox != nil
}

func (m AppModel) remotesOnTop() bool {
	return m.remotes.open && len(m.threads) == 0 && len(m.profiles) == 0
}

// remotesSearching reports the inbox search field taking typed keys, including the ones with workspace meanings.
func (m AppModel) remotesSearching() bool {
	return m.remotesOnTop() && (m.remotes.searching || m.remotes.people != nil || m.remotes.members != nil) && !m.settings.open
}

// remotesOverlay reports a Remotes dialog taking every key.
func (m AppModel) remotesOverlay() bool {
	r := m.remotes
	return r.people != nil || r.members != nil || r.pinWarning != nil || r.pinList != nil
}

// remotesWidth is the center column plus the aside, which Remotes takes over for its two panes.
func (m AppModel) remotesWidth() int {
	_, center, aside := m.columns()
	return center + aside
}

func (m AppModel) remotesSplit() bool { return m.account.width >= 120 }

func (m AppModel) remotesTabs() []api.InboxTab {
	if m.account.user.Remotes != nil && m.account.user.Remotes.Requests {
		return []api.InboxTab{api.Open, api.Fetched, api.Stashed}
	}
	return []api.InboxTab{api.Open, api.Stashed}
}

func (m *AppModel) closeRemoteThread() {
	if t := m.remotes.thread; t != nil && t.cancel != nil {
		t.cancel()
	}
	m.flushMessageDrafts()
	m.cancelRecording()
	m.stopVoice()
	m.remotes.thread, m.remotes.members, m.remotes.pinWarning, m.remotes.pinList = nil, nil, nil, nil
	m.remotes.focusThread = false
}

func (m *AppModel) closeRemotes() {
	if m.remotes.cancel != nil {
		m.remotes.cancel()
	}
	m.closeRemoteThread()
	m.closePeople()
	unread, countCancel := m.remotes.unread, m.remotes.countCancel
	m.remotes = remotesState{generation: m.remotes.generation + 1, tickGeneration: m.remotes.tickGeneration + 1, unread: unread, countCancel: countCancel}
}

func (m AppModel) openRemotes() (AppModel, tea.Cmd) {
	if !m.remotesEnabled() {
		return m, nil
	}
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	m.cancelNotifications()
	m.notificationsOpen = false
	m.closeStarred()
	m.closeRemotes()
	f := &m.feeds[m.active]
	if f.cancel != nil {
		f.cancel()
		f.cancel, f.loading = nil, false
		f.generation++
	}
	m.remotes.open, m.remotes.tab = true, api.Open
	m.notice = ""
	m.resetSequence()
	m.reflow()
	return m, m.fetchInbox("open")
}

func (m *AppModel) fetchInbox(mode string) tea.Cmd {
	r := &m.remotes
	if !r.open || (mode == "more" && (r.loading || !r.hasMore || len(r.inbox) == 0)) {
		return nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	before := ""
	if mode == "more" {
		before = r.inbox[len(r.inbox)-1].ID
	}
	r.generation++
	r.loading, r.status = true, ""
	r.lastInbox = m.now
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	r.cancel = cancel
	session, generation, load, tab, term := m.session, r.generation, m.remoteCalls.inbox, r.tab, r.term
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		inbox, err := load(ctx, tab, term, before)
		return remotesInboxResult{session: session, generation: generation, mode: mode, inbox: inbox, err: err}
	})
}

func (m *AppModel) acceptInbox(msg remotesInboxResult) tea.Cmd {
	r := &m.remotes
	if !m.connected || !r.open || msg.session != m.session || msg.generation != r.generation {
		return nil
	}
	r.cancel, r.loading = nil, false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		if msg.mode == "poll" {
			return m.scheduleRemotes()
		}
		r.status = "Couldn't load Remotes: " + safeText(msg.err.Error()) + ". Ctrl+R to retry."
		return tea.Batch(m.notify("error", "Remotes failed", r.status), m.scheduleRemotes())
	}
	anchor := ""
	if r.selected < len(r.inbox) {
		anchor = r.inbox[r.selected].ID
	}
	hasMore := msg.inbox.HasMore
	switch msg.mode {
	case "more":
		r.inbox = append(r.inbox, msg.inbox.Data...)
	case "poll":
		var tail []api.Conversation
		r.inbox, tail = withFirstPage(r.inbox, msg.inbox.Data)
		if len(tail) > 0 {
			hasMore = r.hasMore
		}
	default:
		r.inbox = msg.inbox.Data
	}
	r.inbox = uniqueConversations(r.inbox)
	m.adoptConversation()
	if t := r.thread; t != nil {
		for i := range r.inbox {
			if r.inbox[i].ID == t.conversation.ID && t.data.ReadThroughID > 0 && lastIncoming(r.inbox[i]) <= t.data.ReadThroughID {
				r.inbox[i].Ahead = 0
			}
		}
	}
	r.hasMore, r.requests, r.unread, r.loaded = hasMore, msg.inbox.RequestsCount, msg.inbox.UnreadCount, true
	r.selected = 0
	for i, c := range r.inbox {
		if c.ID == anchor {
			r.selected = i
		}
	}
	return m.scheduleRemotes()
}

// adoptConversation gives a thread opened by handle the conversation the inbox now lists with that person.
func (m *AppModel) adoptConversation() {
	t := m.remotes.thread
	if t == nil || !strings.HasPrefix(t.conversation.ID, "@") {
		return
	}
	for _, c := range m.remotes.inbox {
		if c.Kind != "org" && c.Partner != nil && strings.EqualFold(c.Partner.Handle, t.conversation.Partner.Handle) {
			if draft, ok := m.messaging.drafts[t.conversation.ID]; ok {
				delete(m.messaging.drafts, t.conversation.ID)
				m.messaging.drafts[c.ID] = draft
			}
			t.conversation = c
			return
		}
	}
}

func uniqueConversations(list []api.Conversation) []api.Conversation {
	seen := make(map[string]bool, len(list))
	result := make([]api.Conversation, 0, len(list))
	for _, c := range list {
		if !seen[c.ID] {
			result = append(result, c)
			seen[c.ID] = true
		}
	}
	return result
}

func lastIncoming(c api.Conversation) int64 {
	if c.LastIncomingMessageID == nil {
		return 0
	}
	return *c.LastIncomingMessageID
}

// withFirstPage is the web's chat.ts withFirstPage: the fresh first page, then the loaded rows that sort after its
// last row, so a poll keeps the older pages already scrolled into. It also returns those kept rows.
func withFirstPage(current, firstPage []api.Conversation) (merged, tail []api.Conversation) {
	if len(firstPage) == 0 {
		return firstPage, nil
	}
	fresh := make(map[string]bool, len(firstPage))
	for _, c := range firstPage {
		fresh[c.ID] = true
	}
	oldest := firstPage[len(firstPage)-1]
	for _, c := range current {
		if !fresh[c.ID] && compareConversations(c, oldest) >= 0 {
			tail = append(tail, c)
		}
	}
	return append(append([]api.Conversation(nil), firstPage...), tail...), tail
}

// compareConversations puts pinned conversations first, newest pin first, then the rest by their latest message, as
// the server does.
func compareConversations(a, b api.Conversation) int {
	at := func(t *time.Time) time.Time {
		if t == nil {
			return time.Time{}
		}
		return *t
	}
	if (a.PinnedAt != nil) != (b.PinnedAt != nil) {
		if a.PinnedAt != nil {
			return -1
		}
		return 1
	}
	if a.PinnedAt != nil {
		return at(b.PinnedAt).Compare(at(a.PinnedAt))
	}
	return at(b.LastMessageAt).Compare(at(a.LastMessageAt))
}

func sortInbox(list []api.Conversation) { slices.SortStableFunc(list, compareConversations) }

func (m *AppModel) openRemoteThread(c api.Conversation) tea.Cmd {
	m.closeRemoteThread()
	t := &remoteThreadState{conversation: c, viewport: viewport.New(), active: m.now, refreshed: map[int64]bool{}, composer: m.restoreComposer(c)}
	m.remotes.thread, m.remotes.focusThread = t, true
	m.sizeRemotes()
	return tea.Batch(m.fetchRemoteThread("open"), m.askReactionUsage())
}

func (m *AppModel) fetchRemoteThread(mode string) tea.Cmd {
	t := m.remotes.thread
	if t == nil || (mode == "poll" && t.loading) {
		return nil
	}
	query := api.ThreadQuery{}
	switch mode {
	case "poll":
		query.After = t.fetchedThrough
		query.ReactionsAfter = t.data.ReactionsVersion
	case "older":
		if t.loading || !t.data.HasOlder || len(t.messages) == 0 {
			return nil
		}
		query.Before = t.messages[0].ID
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.generation++
	t.loading, t.loadingMode = true, mode
	if mode != "poll" {
		t.status = ""
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	t.cancel = cancel
	session, generation, load, target := m.session, t.generation, m.remoteCalls.thread, t.conversation.Target()
	m.renderRemoteThread()
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		thread, err := load(ctx, target, query)
		return remoteThreadResult{session: session, generation: generation, mode: mode, thread: thread, err: err}
	})
}

func (m *AppModel) acceptRemoteThread(msg remoteThreadResult) tea.Cmd {
	t := m.remotes.thread
	if !m.connected || !m.remotes.open || t == nil || msg.session != m.session || msg.generation != t.generation {
		return nil
	}
	t.cancel, t.loading = nil, false
	if msg.err != nil {
		t.jumpTo = 0
		if list := m.remotes.pinList; list != nil && list.loading {
			list.loading, list.err = false, "Could not load older messages. Try again."
		}
		if m.revoked(msg.err) {
			return nil
		}
		if msg.mode == "poll" {
			return m.scheduleRemotes()
		}
		t.status = "Couldn't load this conversation: " + safeText(msg.err.Error()) + ". Ctrl+R to retry."
		m.renderRemoteThread()
		return m.notify("error", "Conversation failed", t.status)
	}
	atBottom := !t.loaded || t.viewport.AtBottom()
	previousHeight := t.viewport.TotalLineCount()
	data := msg.thread
	if msg.mode != "older" {
		if msg.mode != "poll" {
			t.fetchedThrough = 0
		}
		for _, message := range data.Messages {
			t.fetchedThrough = max(t.fetchedThrough, message.ID)
		}
	}
	switch msg.mode {
	case "poll":
		if len(data.Messages) > 0 {
			t.active = m.now
		}
		if !atBottom {
			for _, message := range data.Messages {
				known := slices.ContainsFunc(t.messages, func(old api.Message) bool { return old.ID == message.ID })
				if !known && message.UserID != m.account.user.ID {
					t.unseen++
				}
			}
		}
		t.messages = mergeMessages(t.messages, data.Messages)
		applyReactionChanges(t.messages, data.ReactionChanges)
		if data.ReactionsVersion == 0 {
			data.ReactionsVersion = t.data.ReactionsVersion
		}
		if data.Members == nil {
			data.Members = t.data.Members
		}
		// The server sends pinned as null when it hasn't changed. An empty list means everything was unpinned.
		if data.Pinned == nil {
			data.Pinned = t.data.Pinned
		}
		data.HasOlder = t.data.HasOlder
		data.ReadThroughID = max(data.ReadThroughID, t.data.ReadThroughID)
	case "older":
		t.messages = mergeMessages(data.Messages, t.messages)
		older := data.HasOlder
		data = t.data
		data.HasOlder = older
	default:
		t.messages = data.Messages
		t.refreshed = map[int64]bool{}
	}
	for _, message := range data.Messages {
		if message.UserID == m.account.user.ID && m.confirmPending(t.conversation.ID, message.ClientID) {
			m.touchConversation(t.conversation, message)
		}
	}
	if mc := &t.composer; mc.reply == nil && mc.replyID != 0 {
		if i := slices.IndexFunc(t.messages, func(message api.Message) bool { return message.ID == mc.replyID }); i >= 0 {
			reply := t.messages[i]
			mc.reply = &reply
		}
	}
	data.Messages = nil
	t.data, t.loaded = data, true
	m.renderRemoteThread()
	m.resizeThread()
	switch {
	case msg.mode == "older":
		t.viewport.SetYOffset(t.viewport.YOffset() + t.viewport.TotalLineCount() - previousHeight)
	case atBottom:
		t.viewport.GotoBottom()
		t.unseen = 0
	}
	var jump tea.Cmd
	if id := t.jumpTo; id != 0 {
		t.jumpTo = 0
		jump = m.jumpToMessage(id)
	}
	return tea.Batch(jump, m.markRemoteRead(), m.refreshLinkPreviews(), m.scheduleRemotes())
}

func mergeMessages(first, second []api.Message) []api.Message {
	index := make(map[int64]int, len(first)+len(second))
	result := make([]api.Message, 0, len(first)+len(second))
	for _, message := range append(append([]api.Message(nil), first...), second...) {
		if i, ok := index[message.ID]; ok {
			result[i] = message
			continue
		}
		index[message.ID] = len(result)
		result = append(result, message)
	}
	slices.SortStableFunc(result, func(a, b api.Message) int { return int(a.ID - b.ID) })
	return result
}

func applyReactionChanges(messages []api.Message, changes []api.ReactionChange) {
	for _, change := range changes {
		for i := range messages {
			if messages[i].ID == change.ID {
				messages[i].Reactions = change.Reactions
			}
		}
	}
}

// markRemoteRead moves the receipt to the newest message, never back, and clears the row's count. Like the web's
// conversation-thread.tsx, it only reads a thread that is on screen and scrolled to the newest message.
func (m *AppModel) markRemoteRead() tea.Cmd {
	t := m.remotes.thread
	if t != nil && t.viewport.AtBottom() {
		t.unseen = 0
	}
	if t == nil || !m.focused || !m.remotesOnTop() || m.settings.open || m.accountOpen || !t.loaded || !t.viewport.AtBottom() || t.data.Access != "open" || len(t.messages) == 0 {
		return nil
	}
	newest := t.messages[len(t.messages)-1].ID
	if newest <= t.data.ReadThroughID {
		return nil
	}
	t.data.ReadThroughID = newest
	for i := range m.remotes.inbox {
		if c := &m.remotes.inbox[i]; c.ID == t.conversation.ID {
			// The badge counts conversations, not messages.
			if c.Ahead > 0 {
				m.remotes.unread = max(0, m.remotes.unread-1)
			}
			c.Ahead = 0
		}
	}
	target, mark, life := t.conversation.Target(), m.remoteCalls.markRead, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		// A lost receipt only leaves the count up until the next read, so its error is not shown.
		_ = mark(ctx, target, newest)
		return nil
	})
}

// refreshLinkPreviews starts asking again for each card the server was still fetching when the message arrived.
func (m *AppModel) refreshLinkPreviews() tea.Cmd {
	t := m.remotes.thread
	if t == nil || m.remoteCalls.linkPreview == nil {
		return nil
	}
	var commands []tea.Cmd
	for _, message := range t.messages {
		if message.LinkPreview == nil || !message.LinkPreview.Pending || t.refreshed[message.ID] {
			continue
		}
		t.refreshed[message.ID] = true
		commands = append(commands, m.askLinkPreview(message.ID, 0))
	}
	return tea.Batch(commands...)
}

func (m *AppModel) askLinkPreview(id int64, attempt int) tea.Cmd {
	session, load, life := m.session, m.remoteCalls.linkPreview, m.account.life
	return life.command(func() tea.Msg {
		select {
		case <-life.ctx.Done():
			return nil
		case <-time.After(linkPreviewDelays[attempt]):
		}
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		preview, err := load(ctx, id)
		return remoteLinkPreviewResult{session: session, attempt: attempt, id: id, preview: preview, err: err}
	})
}

// acceptLinkPreview draws a fetched card, asks again while it is still pending, and gives up after the web's last
// wait or a failed request, which stops drawing the loading row.
func (m *AppModel) acceptLinkPreview(msg remoteLinkPreviewResult) tea.Cmd {
	t := m.remotes.thread
	if t == nil || msg.session != m.session {
		return nil
	}
	ready := msg.err == nil && msg.preview != nil && !msg.preview.Pending
	if !ready && msg.err == nil && msg.attempt+1 < len(linkPreviewDelays) {
		return m.askLinkPreview(msg.id, msg.attempt+1)
	}
	for i := range t.messages {
		current := t.messages[i].LinkPreview
		if t.messages[i].ID != msg.id || current == nil || !current.Pending {
			continue
		}
		if ready {
			t.messages[i].LinkPreview = msg.preview
		} else {
			given := *current
			given.Pending = false
			t.messages[i].LinkPreview = &given
		}
	}
	m.renderRemoteThread()
	return nil
}

// scheduleRemotes keeps one poll tick running while Remotes is open.
func (m *AppModel) scheduleRemotes() tea.Cmd {
	if !m.remotes.open || m.remotes.ticking {
		return nil
	}
	m.remotes.ticking = true
	delay := remotesSlowPoll
	if t := m.remotes.thread; t != nil && m.now.Sub(t.active) < time.Minute {
		delay = remotesFastPoll
	}
	delay, m.remotes.away = m.pollDelay(delay), !m.focused
	session, generation := m.session, m.remotes.tickGeneration
	return tea.Tick(delay, func(time.Time) tea.Msg { return remotesTick{session, generation} })
}

func (m *AppModel) acceptRemotesTick(msg remotesTick) tea.Cmd {
	if msg.session != m.session || msg.generation != m.remotes.tickGeneration || !m.remotes.open {
		return nil
	}
	m.remotes.ticking = false
	m.now = time.Now()
	if !m.remotesEnabled() {
		next, cmd := m.switchLog(m.active)
		*m = next
		return cmd
	}
	if m.accountOpen {
		return m.scheduleRemotes()
	}
	return tea.Batch(m.pollRemotes(), m.scheduleRemotes())
}

// pollRemotes asks for newer messages in the open thread, and refreshes the inbox every 15 seconds, or 6 while a
// thread is open, since new rows only matter when the user looks at the list.
func (m *AppModel) pollRemotes() tea.Cmd {
	var commands []tea.Cmd
	if m.remotes.thread != nil && m.remotes.thread.loaded {
		commands = append(commands, m.fetchRemoteThread("poll"))
	}
	every := 15 * time.Second
	if m.remotes.thread != nil {
		every = 6 * time.Second
	}
	if !m.remotes.loading && m.now.Sub(m.remotes.lastInbox) >= every {
		commands = append(commands, m.fetchInbox("poll"))
	}
	return tea.Batch(commands...)
}

// refreshRemotesCount keeps the sidebar badge current while Remotes is closed, through the Open tab's unread count.
func (m *AppModel) refreshRemotesCount() tea.Cmd {
	if !m.remotesEnabled() || m.remotes.open {
		return nil
	}
	if m.remotes.countCancel != nil {
		m.remotes.countCancel()
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	m.remotes.countCancel = cancel
	session, load := m.session, m.remoteCalls.inbox
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		inbox, err := load(ctx, api.Open, "", "")
		if err != nil {
			return nil
		}
		return remotesCountResult{session: session, unread: inbox.UnreadCount}
	})
}

type remotesCountResult struct{ session, unread int }

func (m *AppModel) selectConversation(index int) tea.Cmd {
	r := &m.remotes
	rows := len(r.inbox) + len(r.candidates)
	if rows == 0 {
		return nil
	}
	previous := r.selected
	r.selected = max(0, min(index, rows-1))
	if r.selected > previous && r.selected < len(r.inbox) && r.selected >= len(r.inbox)-2 {
		return m.fetchInbox("more")
	}
	return nil
}

func (m *AppModel) switchInboxTab(delta int) tea.Cmd {
	tabs := m.remotesTabs()
	current := max(0, slices.Index(tabs, m.remotes.tab))
	m.remotes.tab = tabs[(current+delta+len(tabs))%len(tabs)]
	m.remotes.inbox, m.remotes.selected, m.remotes.loaded, m.remotes.hasMore, m.remotes.candidates = nil, 0, false, false, nil
	return m.fetchInbox("open")
}

// focusedConversation is the conversation the actions apply to: the open thread when it has focus, otherwise the
// selected row.
func (m AppModel) focusedConversation() (api.Conversation, bool) {
	r := m.remotes
	if r.thread != nil && (r.focusThread || len(r.inbox) == 0) {
		return r.thread.conversation, true
	}
	if r.selected < len(r.inbox) {
		return r.inbox[r.selected], true
	}
	return api.Conversation{}, false
}

func (m AppModel) remotesKey(pressed string) (AppModel, tea.Cmd) {
	r := &m.remotes
	if r.dialog != "" {
		cmd := m.remotesDialogKey(pressed)
		return m, cmd
	}
	if r.people != nil {
		cmd := m.peopleKey(pressed)
		return m, cmd
	}
	if r.members != nil {
		cmd := m.membersKey(pressed)
		return m, cmd
	}
	if r.pinWarning != nil {
		cmd := m.pinWarningKey(pressed)
		return m, cmd
	}
	if r.pinList != nil {
		cmd := m.pinListKey(pressed)
		return m, cmd
	}
	if r.searching {
		return m.remotesSearchKey(pressed)
	}
	if cmd, handled := m.conversationActionKey(pressed); handled {
		return m, cmd
	}
	if r.thread != nil && r.focusThread {
		return m.remoteThreadKey(pressed)
	}
	switch pressed {
	case "esc":
		if r.term != "" {
			r.term, r.query = "", ""
			r.inbox, r.selected, r.loaded, r.candidates = nil, 0, false, nil
			return m, m.fetchInbox("open")
		}
		return m.switchLog(m.active)
	case "j", "down":
		return m, m.selectConversation(r.selected + 1)
	case "k", "up":
		return m, m.selectConversation(r.selected - 1)
	case "pgdown", "ctrl+d":
		return m, m.selectConversation(r.selected + 5)
	case "pgup", "ctrl+u":
		return m, m.selectConversation(r.selected - 5)
	case "home":
		return m, m.selectConversation(0)
	case "G", "end":
		return m, m.selectConversation(len(r.inbox) + len(r.candidates) - 1)
	case "tab", "]":
		return m, m.switchInboxTab(1)
	case "shift+tab", "[":
		return m, m.switchInboxTab(-1)
	case "/":
		r.searching, r.query = true, r.term
		return m, nil
	case "n":
		return m, m.fetchInbox("more")
	case "ctrl+r":
		return m, m.fetchInbox("refresh")
	case "N":
		m.openPeople("remote", api.Conversation{})
	case "O":
		m.openPeople("org", api.Conversation{})
	case "enter", "l", "right":
		if r.selected < len(r.inbox) {
			return m, m.openRemoteThread(r.inbox[r.selected])
		}
		if i := r.selected - len(r.inbox); i < len(r.candidates) && r.candidates[i].Reason == "" {
			return m, m.openPerson(r.candidates[i].User)
		}
	}
	return m, nil
}

func (m AppModel) remotesSearchKey(pressed string) (AppModel, tea.Cmd) {
	r := &m.remotes
	switch pressed {
	case "esc":
		r.searching, r.query = false, r.term
	case "enter":
		r.searching = false
		r.term = strings.TrimSpace(r.query)
		r.inbox, r.selected, r.loaded, r.candidates = nil, 0, false, nil
		return m, tea.Batch(m.fetchInbox("open"), m.fetchInboxPeople())
	case "backspace":
		if runes := []rune(r.query); len(runes) > 0 {
			r.query = string(runes[:len(runes)-1])
		}
	case "space":
		r.query += " "
	default:
		if runes := []rune(pressed); len(runes) == 1 && len([]rune(r.query)) < 100 {
			r.query += pressed
		}
	}
	return m, nil
}

func (m AppModel) remoteThreadKey(pressed string) (AppModel, tea.Cmd) {
	t := m.remotes.thread
	if cmd, ok := m.voiceKey(pressed); ok {
		return m, cmd
	}
	if t.data.Access == "request" {
		switch pressed {
		case "m":
			return m, m.startRemoteAction("merge", t.conversation, true, "")
		case "d":
			m.remotes.dialog, m.remotes.dialogFocus, m.remotes.dialogTarget = "drop", 0, t.conversation
			return m, nil
		}
	}
	switch pressed {
	case "x":
		if item, ok := m.selectedItem(); ok && item.message != nil {
			msg := item.message
			c := api.Commit{ID: "message-links", Body: msg.Body, Segments: msg.Segments, LinkPreview: msg.LinkPreview}
			return m, m.openLinks(m.commitLinks(c))
		}
	case "I":
		m.openMembers()
	case "!":
		return m, m.pinKey()
	case "L":
		m.openPinList()
	case "esc", "h", "left":
		m.closeRemoteThread()
	case "tab":
		m.remotes.focusThread = false
		if !m.remotesSplit() {
			m.closeRemoteThread()
		}
	case "j", "down":
		return m, m.moveMessageCursor(1)
	case "k", "up":
		return m, m.moveMessageCursor(-1)
	case "pgdown", "ctrl+d":
		t.viewport.PageDown()
		return m, m.markRemoteRead()
	case "pgup", "ctrl+u":
		t.viewport.PageUp()
		if t.viewport.AtTop() {
			return m, m.fetchRemoteThread("older")
		}
	case "G", "end":
		return m, m.backToPresent()
	case "home":
		t.viewport.GotoTop()
		return m, m.fetchRemoteThread("older")
	case "n":
		return m, m.fetchRemoteThread("older")
	case "ctrl+r":
		return m, m.fetchRemoteThread("refresh")
	case "w", "enter":
		return m, m.focusMessageComposer()
	case "ctrl+t":
		focus := m.focusMessageComposer()
		if focus == nil {
			return m, nil
		}
		record, _ := m.recordingKey(pressed)
		return m, tea.Batch(focus, record)
	case "p":
		return m, m.sendPing()
	case "r", "d", "e", "1", "2", "3":
		return m, m.messageAction(pressed)
	case "E":
		m.showReactions()
	}
	return m, nil
}

// messageAction runs a message key on the item under the cursor, or the newest one: reply, react, or retry and
// discard for a rejected push.
func (m *AppModel) messageAction(pressed string) tea.Cmd {
	t := m.remotes.thread
	item, ok := m.selectedItem()
	if !ok {
		return nil
	}
	if item.pending != nil {
		switch pressed {
		case "r":
			return m.retryMessage(item.pending.clientID)
		case "d":
			m.discardMessage(item.pending.clientID)
		}
		return nil
	}
	message := item.message
	if _, ok := t.reactable(message.ID); !ok {
		return nil
	}
	switch pressed {
	case "r":
		reply := *message
		t.composer.reply, t.composer.replyID = &reply, reply.ID
		return tea.Batch(m.focusMessageComposer(), m.scheduleDraftSave())
	case "e":
		return m.openReactions(message.ID, false)
	case "1", "2", "3":
		return m.toggleReaction(message.ID, quickReactions(m.messaging.usage)[pressed[0]-'1'])
	}
	return nil
}

// remotesStatus is the Remotes status bar: counts and progress stay text, and shortcuts are chips.
func (m AppModel) remotesStatus() []statusPart {
	r := m.remotes
	back := closeChip("Back")
	if t := r.thread; t != nil && r.focusThread {
		switch {
		case t.status != "":
			return plainStatusLine(t.status)
		case t.loading && !t.loaded:
			return plainStatusLine("Reading the conversation from /dev/null...")
		case t.loading && t.loadingMode == "older":
			return plainStatusLine("Loading older messages...")
		}
		if t.composer.focused {
			return []statusPart{plainStatus("Write to " + m.conversationTitle(t.conversation)), statusChip("Push", "enter"), closeChip("Messages")}
		}
		if voice := m.voiceStatus(); voice != nil {
			return append(voice, back)
		}
		parts := []statusPart{plainStatus(fmt.Sprintf("%d messages", len(t.messages))), hintChip("Select", "↑/↓", "down")}
		if item, ok := m.selectedItem(); ok && item.message != nil && len(item.message.Reactions) > 0 {
			parts = append(parts, statusChip("Reactions", "E"))
		}
		if t.conversation.Kind == "org" {
			parts = append(parts, statusChip("Members", "I"))
		}
		if label, ok := m.pingLabel(); ok {
			parts = append(parts, statusChip(label, "p"))
		}
		return append(parts, back)
	}
	switch {
	case r.status != "":
		return plainStatusLine(r.status)
	case r.searching:
		return []statusPart{plainStatus("Search by name or @handle"), statusChip("Search", "enter"), closeChip("Cancel")}
	case r.loading && !r.loaded:
		return plainStatusLine("Reading Remotes from /dev/null...")
	case len(r.inbox)+len(r.candidates) == 0:
		return []statusPart{plainStatus("No conversations here."), statusChip("New remote", "N"), statusChip("New org", "O")}
	}
	tabs := m.remotesTabs()
	next := string(tabs[(max(0, slices.Index(tabs, r.tab))+1)%len(tabs)])
	parts := []statusPart{
		plainStatus(fmt.Sprintf("%d / %d", r.selected+1, len(r.inbox)+len(r.candidates))),
		statusChip("Open", "enter"), statusChip(strings.ToUpper(next[:1])+next[1:], "tab"), statusChip("Search", "/"), statusChip("Actions", "."),
	}
	if r.hasMore {
		parts = append(parts, statusChip("More", "n"))
	}
	return append(parts, statusChip("New remote", "N"), statusChip("New org", "O"), back)
}

// pingLabel names the thread's ping, which shows once the conversation has messages and the ping call exists.
func (m AppModel) pingLabel() (string, bool) {
	t := m.remotes.thread
	if t == nil || len(t.messages) == 0 || m.remoteCalls.ping == nil || t.composer.status != "" {
		return "", false
	}
	if c := t.conversation; c.Kind != "org" && c.Partner != nil {
		return "Ping @" + safeText(c.Partner.Handle), true
	}
	return "Ping everyone in the org", true
}

// backToPresent jumps to the newest message at once, with no animation, for G and the pill above the composer.
func (m *AppModel) backToPresent() tea.Cmd {
	t := m.remotes.thread
	if t == nil {
		return nil
	}
	t.selected = ""
	m.renderRemoteThread()
	t.viewport.GotoBottom()
	t.unseen = 0
	return m.markRemoteRead()
}
