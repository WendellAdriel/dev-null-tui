package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// messaging is a Remotes app whose message calls are recorded, with pushErr deciding the next push's answer.
type messaging struct {
	log     *remoteCallLog
	pushes  []api.NewMessage
	pushErr error
	access  string
}

func messagingApp(t *testing.T) (AppModel, *messaging) {
	t.Helper()
	m, log := remotesApp(t, auth.RemotePreferences{})
	state := &messaging{log: log, access: "open"}
	thread := m.remoteCalls.thread
	m.remoteCalls.thread = func(ctx context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		data, err := thread(ctx, target, q)
		data.Access = state.access
		return data, err
	}
	m.remoteCalls.push = func(_ context.Context, target api.Target, draft api.NewMessage) (api.Message, error) {
		state.pushes = append(state.pushes, draft)
		log.add("push %s%s %d", target.Handle, target.Org, draft.ReplyToID)
		if state.pushErr != nil {
			return api.Message{}, state.pushErr
		}
		message := remoteMessage(int64(20+len(state.pushes)), "fixture", strings.TrimSpace(draft.Body), m.now)
		message.ClientID = draft.ClientID
		return message, nil
	}
	m.remoteCalls.typing = func(_ context.Context, target api.Target) error {
		log.add("typing %s%s", target.Handle, target.Org)
		return nil
	}
	m.remoteCalls.ping = func(_ context.Context, target api.Target) (api.Message, error) {
		log.add("ping %s%s", target.Handle, target.Org)
		ping := remoteMessage(30, "fixture", "", m.now)
		ping.Event = "ping"
		return ping, nil
	}
	m.remoteCalls.react = func(_ context.Context, id int64, emoji string, on bool) ([]api.Reaction, error) {
		log.add("react %d %s %t", id, emoji, on)
		if !on {
			return []api.Reaction{}, nil
		}
		return []api.Reaction{{UserID: "fixture", Emoji: emoji}}, nil
	}
	m.remoteCalls.usage = func(context.Context) ([]api.EmojiUse, error) {
		log.add("usage")
		return []api.EmojiUse{{Emoji: "🚀", Count: 4}}, nil
	}
	return m, state
}

func typed(value string) []tea.KeyPressMsg {
	var keys []tea.KeyPressMsg
	for _, r := range value {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

var (
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func send(m AppModel, keys ...tea.KeyPressMsg) AppModel {
	for _, k := range keys {
		next, cmd := m.update(k)
		m = settleCmd(next.(AppModel), cmd)
	}
	return m
}

// openDirect opens the conversation with @ada, which has no messages yet; openOrg opens the org thread.
func openDirect(t *testing.T, m AppModel) AppModel {
	m, _ = m.handleKey("g")
	return settle(t, m, "r", "enter")
}

func openOrg(t *testing.T, m AppModel) AppModel {
	m, _ = m.handleKey("g")
	return settle(t, m, "r", "j", "enter")
}

func threadText(m AppModel) string {
	return ansi.Strip(m.remoteThreadPane(m.threadPaneWidth(), m.mainHeight()-remotesHeaderHeight, colors(true)))
}

func TestPushingStagesTheMessageAndAddsItOnceStored(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	m = send(m, typed("i")...)
	if m.remotes.thread.composer.focused {
		t.Fatal("i must not focus the message composer")
	}
	m = send(m, typed("w")...)
	if !m.remotes.thread.composer.focused {
		t.Fatal("w must focus the composer")
	}
	m = send(m, escKey, enterKey)
	if !m.remotes.thread.composer.focused {
		t.Fatal("Enter must still focus the composer")
	}
	m = send(m, typed("ship it")...)
	next, cmd := m.update(enterKey)
	m = next.(AppModel)
	if got := m.messaging.pending[fixtureOrg]; len(got) != 1 || got[0].status != "staged" || m.remotes.thread.composer.input.Value() != "" {
		t.Fatalf("push not staged: %+v", got)
	}
	if !strings.Contains(threadText(m), "◌ staged") {
		t.Fatalf("staged message not drawn:\n%s", threadText(m))
	}
	m = settleCmd(m, cmd)
	if len(m.messaging.pending[fixtureOrg]) != 0 {
		t.Fatal("stored message still pending")
	}
	if len(state.pushes) != 1 || !uuidV4.MatchString(state.pushes[0].ClientID) || state.pushes[0].Body != "ship it" {
		t.Fatalf("pushed %+v", state.pushes)
	}
	last := m.remotes.thread.messages[len(m.remotes.thread.messages)-1]
	if last.Body != "ship it" || m.remotes.inbox[0].ID != fixtureOrg || m.remotes.inbox[0].LastMessage.Body != "ship it" {
		t.Fatalf("thread or inbox not updated: last %+v, first row %s", last, m.remotes.inbox[0].ID)
	}
}

func TestARejectedPushShowsTheReasonAndRetriesWithTheSameClientID(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	state.pushErr = &auth.APIError{Status: http.StatusForbidden, Message: "You can't push to this org right now."}
	m = send(m, typed("whello")...)
	m = send(m, enterKey)
	pending := m.messaging.pending[fixtureOrg]
	if len(pending) != 1 || pending[0].status != "rejected" || !strings.Contains(threadText(m), "rejected · You can't push to this org right now.") {
		t.Fatalf("rejection not shown: %+v\n%s", pending, threadText(m))
	}
	state.pushErr = nil
	m = send(m, escKey)
	m = settle(t, m, "r")
	if len(state.pushes) != 2 || state.pushes[1].ClientID != state.pushes[0].ClientID || len(m.messaging.pending[fixtureOrg]) != 0 {
		t.Fatalf("retry pushed %+v, pending %+v", state.pushes, m.messaging.pending[fixtureOrg])
	}
}

func TestDiscardDropsARejectedPush(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	state.pushErr = errors.New("offline")
	m = send(m, typed("whello")...)
	m = send(m, enterKey, escKey)
	m = settle(t, m, "d")
	if len(m.messaging.pending[fixtureOrg]) != 0 || strings.Contains(threadText(m), "rejected") {
		t.Fatal("rejected push not discarded")
	}
}

func TestReplyTargetsTheSelectedMessageAndEscCancelsItFirst(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	m = settle(t, m, "k", "r")
	mc := m.remotes.thread.composer
	if !mc.focused || mc.reply == nil || mc.reply.ID != 9 || !strings.Contains(threadText(m), "Replying to @grace: standup moved") {
		t.Fatalf("reply not set: %+v\n%s", mc.reply, threadText(m))
	}
	m = send(m, escKey)
	if m.remotes.thread.composer.reply != nil || !m.remotes.thread.composer.focused {
		t.Fatal("esc must cancel the reply and keep writing")
	}
	m = send(m, typed("draft")...)
	m = send(m, escKey)
	if m.remotes.thread.composer.focused || m.remotes.thread.composer.input.Value() != "draft" {
		t.Fatal("esc must go back to the messages and keep the draft")
	}
	m = settle(t, m, "k", "k", "r")
	m = send(m, enterKey)
	if len(state.pushes) != 1 || state.pushes[0].ReplyToID != 8 {
		t.Fatalf("reply pushed %+v", state.pushes)
	}
}

func TestQuickReactionsFollowUsageSlowly(t *testing.T) {
	if got := quickReactions(nil); !slices.Equal(got, []string{"😂", "✅", "❤️"}) {
		t.Fatalf("defaults %v", got)
	}
	if got := quickReactions([]api.EmojiUse{{Emoji: "🚀", Count: 2}}); !slices.Equal(got, []string{"😂", "✅", "❤️"}) {
		t.Fatalf("two uses took a slot: %v", got)
	}
	got := quickReactions([]api.EmojiUse{{Emoji: "🚀", Count: 5}, {Emoji: "✅", Count: 1}})
	if !slices.Equal(got, []string{"😂", "✅", "🚀"}) {
		t.Fatalf("rocket must take the last weakest slot: %v", got)
	}
	if usage := withReactionUse([]api.EmojiUse{{Emoji: "🚀", Count: 1}}, "🚀", -1); len(usage) != 0 {
		t.Fatalf("unused emoji kept: %v", usage)
	}
}

func TestReactingFromTheRowAndThePickerTogglesAtOnce(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	m = settle(t, m, "k", "e")
	if m.remotes.thread.react == nil || !strings.Contains(threadText(m), "React") {
		t.Fatalf("reaction row not open:\n%s", threadText(m))
	}
	next, cmd := m.update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = next.(AppModel)
	if got := m.remotes.thread.messages[2].Reactions; len(got) != 1 || got[0].Emoji != "😂" {
		t.Fatalf("reaction not shown at once: %+v", got)
	}
	m = settleCmd(m, cmd)
	m = settle(t, m, "e")
	m = send(m, tea.KeyPressMsg{Code: '+', Text: "+"})
	m = send(m, typed("rocket")...)
	m = send(m, enterKey)
	reactions := m.remotes.thread.messages[2].Reactions
	if !strings.Contains(state.log.String(), "react 9 😂 true") || !strings.Contains(state.log.String(), "react 9 🚀 true") || len(reactions) != 1 || reactions[0].Emoji != "🚀" {
		t.Fatalf("calls %s, reactions %+v", state.log, reactions)
	}
	m = settle(t, m, "3")
	if !strings.Contains(state.log.String(), "react 9 🚀 false") {
		t.Fatalf("rocket in a quick slot must toggle off: %s", state.log)
	}
}

func TestAFailedReactionIsPutBack(t *testing.T) {
	m, _ := messagingApp(t)
	m.remoteCalls.react = func(context.Context, int64, string, bool) ([]api.Reaction, error) {
		return nil, &auth.APIError{Status: http.StatusForbidden, Message: "You can't react until the request is merged."}
	}
	m = openOrg(t, m)
	m = settle(t, m, "k", "1")
	if got := m.remotes.thread.messages[2].Reactions; len(got) != 0 || m.toast.title != "Couldn't update the reaction" {
		t.Fatalf("reactions %+v, toast %q", got, m.toast.title)
	}
}

func TestPingNeedsAStartedConversation(t *testing.T) {
	m, state := messagingApp(t)
	m = openDirect(t, m)
	m = settle(t, m, "p")
	if strings.Contains(state.log.String(), "ping") || !strings.Contains(threadText(m), "Say hi to @ada before pinging them.") {
		t.Fatalf("pinged an empty conversation: %s", state.log)
	}
	m, state = messagingApp(t)
	m = openOrg(t, m)
	if !strings.Contains(threadText(m), "Ping everyone in the org") {
		t.Fatalf("ping chip missing:\n%s", threadText(m))
	}
	m = settle(t, m, "p")
	if !strings.Contains(state.log.String(), "ping "+fixtureOrg) || m.remotes.thread.messages[len(m.remotes.thread.messages)-1].Event != "ping" {
		t.Fatalf("ping not sent: %s", state.log)
	}
}

func TestTypingIsSentAtMostEveryFewSecondsInAStartedConversation(t *testing.T) {
	m, state := messagingApp(t)
	m = openDirect(t, m)
	send(m, typed("whi")...)
	if strings.Contains(state.log.String(), "typing") {
		t.Fatal("typing sent before the conversation started")
	}
	m, state = messagingApp(t)
	m = openOrg(t, m)
	send(m, typed("whello")...)
	if got := strings.Count(state.log.String(), "typing "+fixtureOrg); got != 1 {
		t.Fatalf("typing sent %d times: %s", got, state.log)
	}
}

func TestTheAccessBarTakesTheComposersPlace(t *testing.T) {
	m, state := messagingApp(t)
	state.access = "waiting"
	m = openDirect(t, m)
	m = send(m, typed("w")...)
	if m.remotes.thread.composer.focused || !strings.Contains(threadText(m), "pushed. waiting for @ada to merge your request") {
		t.Fatalf("waiting bar missing:\n%s", threadText(m))
	}
	if strings.Contains(threadText(m), "Message @ada") {
		t.Fatal("composer drawn while the user can't push")
	}
}

func TestDraftsSurviveClosingTheThreadAndGoOnPush(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = settle(t, m, "k", "r")
	m = send(m, typed("half written")...)
	m = send(m, escKey, escKey)
	m = settle(t, m, "esc")
	drafts := m.account.store.MessageDrafts("fixture")
	if drafts[fixtureOrg].Body != "half written" || drafts[fixtureOrg].ReplyToID != 0 {
		t.Fatalf("draft not saved: %+v", drafts)
	}
	m = settle(t, m, "j", "enter")
	if m.remotes.thread.composer.input.Value() != "half written" {
		t.Fatalf("draft not restored: %q", m.remotes.thread.composer.input.Value())
	}
	m = send(m, typed("w")...)
	m = send(m, enterKey)
	if drafts := m.account.store.MessageDrafts("fixture"); len(drafts) != 0 {
		t.Fatalf("pushed draft still on disk: %+v", drafts)
	}
}

func TestADraftKeepsItsReplyTargetByID(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = settle(t, m, "k", "r")
	m = send(m, typed("on it")...)
	m.flushMessageDrafts()
	raw, err := os.ReadFile(filepath.Join(m.account.store.Dir, "message-drafts-fixture.json"))
	if err != nil || !strings.Contains(string(raw), `"reply_to_id":9`) || strings.Contains(string(raw), "standup moved") {
		t.Fatalf("draft file %s, err %v", raw, err)
	}
	next, _ := messagingApp(t)
	next.account.store = m.account.store
	next.startMessaging()
	next = openOrg(t, next)
	if reply := next.remotes.thread.composer.reply; reply == nil || reply.ID != 9 || next.remotes.thread.composer.input.Value() != "on it" {
		t.Fatalf("draft not restored: %+v", reply)
	}
}

func TestRevokingTheDeviceDeletesTheDrafts(t *testing.T) {
	m, _ := messagingApp(t)
	if err := m.account.store.SaveMessageDrafts("fixture", map[string]auth.MessageDraft{"d1": {Body: "private"}}); err != nil {
		t.Fatal(err)
	}
	m.revoked(&auth.APIError{Status: http.StatusUnauthorized})
	if drafts := m.account.store.MessageDrafts("fixture"); len(drafts) != 0 {
		t.Fatalf("drafts survived revocation: %+v", drafts)
	}
}

func TestDroppedImagesAttachAndPushAsMultipart(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	path := pngFile(t, t.TempDir(), "chart.png")
	m = send(m, typed("w")...)
	next, _ := m.update(tea.PasteMsg{Content: path})
	m = next.(AppModel)
	if images := m.remotes.thread.composer.images; len(images) != 1 || !strings.Contains(threadText(m), "chart.png") {
		t.Fatalf("image not attached: %+v", images)
	}
	m = send(m, enterKey)
	if len(state.pushes) != 1 || len(state.pushes[0].Images) != 1 || state.pushes[0].Body != "" {
		t.Fatalf("pushed %+v", state.pushes)
	}
}

func TestAPastePastTheLimitIsRefused(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = send(m, typed("w")...)
	next, _ := m.update(tea.PasteMsg{Content: strings.Repeat("a", api.MaxMessageLength+1)})
	m = next.(AppModel)
	if m.remotes.thread.composer.input.Value() != "" || !strings.Contains(threadText(m), "fatal: paste rejected, message would be 2,001/") {
		t.Fatalf("long paste accepted:\n%s", threadText(m))
	}
}

func TestShortcodesBecomeEmojiInTheMessageComposer(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = send(m, typed("wship :rocket:")...)
	if got := m.remotes.thread.composer.input.Value(); got != "ship 🚀" {
		t.Fatalf("shortcode not replaced: %q", got)
	}
}

func TestTheMessageCursorSkipsEventsAndFollowsTheNewestPastTheEnd(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = settle(t, m, "k")
	if m.remotes.thread.selected != "m:9" {
		t.Fatalf("selected %q", m.remotes.thread.selected)
	}
	m = settle(t, m, "k", "k")
	if m.remotes.thread.selected != "m:8" {
		t.Fatalf("cursor stopped on an event: %q", m.remotes.thread.selected)
	}
	m = settle(t, m, "j", "j")
	if m.remotes.thread.selected != "" {
		t.Fatalf("cursor must follow the newest past the end: %q", m.remotes.thread.selected)
	}
}

func TestOwnReceiptsDoNotSkipIncomingMessages(t *testing.T) {
	for _, kind := range []string{"push", "ping"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := messagingApp(t)
			m = openOrg(t, m)
			own := remoteMessage(11, "fixture", "synthetic receipt", m.now)
			if kind == "push" {
				m = send(m, typed("wsynthetic receipt")...)
				next, _ := m.update(enterKey)
				m = next.(AppModel)
				own.ClientID = m.messaging.pending[fixtureOrg][0].clientID
				m.acceptRemoteSend(remoteSendResult{session: m.session, conversation: m.remotes.thread.conversation, clientID: own.ClientID, message: own})
			} else {
				own.Event = "ping"
				m.acceptRemotePing(remotePingResult{session: m.session, conversation: m.remotes.thread.conversation, message: own})
			}
			var after int64
			m.remoteCalls.thread = func(_ context.Context, _ api.Target, query api.ThreadQuery) (api.RemoteThread, error) {
				after = query.After
				return api.RemoteThread{Org: &api.Org{ID: fixtureOrg}, Access: "open", Messages: []api.Message{remoteMessage(10, "grace", "synthetic incoming", m.now), own}}, nil
			}
			m = settleCmd(m, m.fetchRemoteThread("poll"))
			if after != 9 || len(m.remotes.thread.messages) != 5 {
				t.Fatalf("receipt skipped incoming message: after=%d messages=%+v", after, m.remotes.thread.messages)
			}
		})
	}
}

func TestPollingConfirmsPendingBeforeALateSendFailure(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = send(m, typed("wsynthetic message")...)
	next, _ := m.update(enterKey)
	m = next.(AppModel)
	clientID := m.messaging.pending[fixtureOrg][0].clientID
	stored := remoteMessage(21, "fixture", "synthetic message", m.now)
	stored.ClientID = clientID
	m.acceptRemoteThread(remoteThreadResult{session: m.session, generation: m.remotes.thread.generation, mode: "poll", thread: api.RemoteThread{Access: "open", Messages: []api.Message{stored}}})
	m.acceptRemoteSend(remoteSendResult{session: m.session, conversation: m.remotes.thread.conversation, clientID: clientID, err: errors.New("lost response")})
	if len(m.messaging.pending[fixtureOrg]) != 0 || strings.Count(threadText(m), "synthetic message") != 1 || m.remotes.inbox[0].LastMessage.ClientID != clientID {
		t.Fatalf("polled receipt was not reconciled:\n%s", threadText(m))
	}
}

func TestReactionChangesAreSerializedAndOldThreadsCannotReplaceThem(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	first := m.toggleReaction(9, "🚀")
	if second := m.toggleReaction(9, "✅"); second != nil {
		t.Fatal("a second reaction write started before the first finished")
	}
	m = settleCmd(m, first)
	m.remoteCalls.react = func(context.Context, int64, string, bool) ([]api.Reaction, error) {
		return []api.Reaction{{UserID: "fixture", Emoji: "🚀"}, {UserID: "fixture", Emoji: "✅"}}, nil
	}
	m = settleCmd(m, m.toggleReaction(9, "✅"))
	if len(m.remotes.thread.messages[2].Reactions) != 2 {
		t.Fatal("confirmed reactions were lost")
	}
	late := m.toggleReaction(9, "😂")()
	conversation := m.remotes.thread.conversation
	m = settleCmd(m, m.openRemoteThread(conversation))
	m.remotes.thread.messages[2].Reactions = []api.Reaction{{UserID: "grace", Emoji: "❤️"}}
	next, _ := m.update(late)
	m = next.(AppModel)
	if reactions := m.remotes.thread.messages[2].Reactions; len(reactions) != 1 || reactions[0].Emoji != "❤️" {
		t.Fatalf("old thread response replaced reopened reactions: %+v", reactions)
	}
}

func TestUpdateWaitsForMessageInputsAndReactions(t *testing.T) {
	for _, owner := range []string{"composer", "browser", "description", "reactions"} {
		t.Run(owner, func(t *testing.T) {
			m, _ := messagingApp(t)
			m = openOrg(t, m)
			m = send(m, typed("wdraft")...)
			switch owner {
			case "browser":
				m.openMessageBrowser()
			case "description":
				m.attachMessageImages([]string{pngFile(t, t.TempDir(), "synthetic.png")})
				m = send(m, tea.KeyPressMsg{Code: tea.KeyTab}, enterKey)
			case "reactions":
				m = send(m, escKey)
				m = settle(t, m, "k", "e")
				m = send(m, typed("+")...)
			}
			m.acceptUpgradeChecked(upgradeChecked{latest: "v99.0.0"})
			if m.upgradeVisible() {
				t.Fatal("update modal took keys from message input")
			}
		})
	}
}

func TestCompactMessageComposerKeepsEditorAndActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m, _ := messagingApp(t)
			m.images.Enabled = true
			m = openOrg(t, m)
			m.account.width, m.account.height = size[0], size[1]
			m.reflow()
			m = settle(t, m, "k", "r")
			path := pngFile(t, t.TempDir(), "synthetic.png")
			m.attachMessageImages([]string{path, path, path})
			next, _ := m.update(tea.PasteMsg{Content: "one\ntwo\nthree\nfour\nfive :rock"})
			m = next.(AppModel)
			view, _ := m.messageComposerView(m.threadPaneWidth(), colors(true))
			if !strings.Contains(view, "Image 1 ·") || strings.Contains(view, "synthetic.png") {
				t.Fatalf("compact composer lost thumbnails at %v:\n%s", size, view)
			}
			paneHeight := m.mainHeight() - remotesHeaderHeight
			if lipgloss.Height(view)+m.remotes.thread.viewport.Height()+3 > paneHeight || !strings.Contains(threadText(m), "enter push") || !strings.Contains(threadText(m), "five :rock") {
				t.Fatalf("composer clipped at %v:\n%s", size, threadText(m))
			}
		})
	}
}

func TestMessageAttachmentsShowThumbnailsAndFocusHints(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = send(m, typed("w")...)
	m.images.Enabled = true
	dir := t.TempDir()
	paths := []string{pngFile(t, dir, "first.png"), pngFile(t, dir, "second.png")}
	m.attachMessageImages(paths)
	view, _ := m.messageComposerView(m.threadPaneWidth(), colors(true))
	if strings.Contains(view, "first.png") || !strings.Contains(view, "Image 1 ·") || !strings.Contains(view, "tab edit images") {
		t.Fatalf("missing thumbnails, details, or write hint:\n%s", view)
	}
	var locals []string
	for _, request := range m.visibleMedia() {
		if request.Path != "" {
			locals = append(locals, request.Path)
			if request.Rows > stripThumbRows || request.Cols > stripThumbCols {
				t.Fatalf("oversized thumbnail: %+v", request)
			}
		}
	}
	if !slices.Equal(locals, paths) {
		t.Fatalf("attachment thumbnails not scheduled: %v", locals)
	}
	jobs, _ := m.images.Plan(t.Context(), m.visibleMedia())
	if len(jobs) != len(paths) {
		t.Fatalf("thumbnails were not prioritized: %d jobs", len(jobs))
	}
	for _, job := range jobs {
		result := m.images.Fetch(job)
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		m.images.Accept(result)
		view, _ = m.messageComposerView(m.threadPaneWidth(), colors(true))
		if row := m.images.Row(job.Request, 0); row == "" || !strings.Contains(view, row) {
			t.Fatal("loaded thumbnail not rendered in the message composer")
		}
	}
	m = send(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyRight})
	view, _ = m.messageComposerView(m.threadPaneWidth(), colors(true))
	if !strings.Contains(view, "Image 2 ·") || !strings.Contains(view, "tab write") {
		t.Fatalf("selected image or return hint missing:\n%s", view)
	}
	m = send(m, enterKey)
	view, _ = m.messageComposerView(m.threadPaneWidth(), colors(true))
	if !strings.Contains(view, "enter save") || strings.Contains(view, "tab write") || strings.Contains(view, "enter push") {
		t.Fatalf("description field has misleading hints:\n%s", view)
	}
	m = send(m, typed("second image")...)
	m = send(m, enterKey, tea.KeyPressMsg{Code: tea.KeyTab})
	if mc := m.remotes.thread.composer; mc.stripFocus || mc.images[1].Description != "second image" {
		t.Fatal("description editing or return to writing failed")
	}
	m.images.Enabled = false
	view, _ = m.messageComposerView(m.threadPaneWidth(), colors(true))
	if !strings.Contains(view, "first.png") || !strings.Contains(view, "second.png") || !strings.Contains(view, "second image") {
		t.Fatalf("text fallback lost image details:\n%s", view)
	}
}

func TestMessageImagesAreCheckedAgainBeforeStaging(t *testing.T) {
	for _, change := range []string{"removed", "replaced"} {
		t.Run(change, func(t *testing.T) {
			m, state := messagingApp(t)
			m = openOrg(t, m)
			m = send(m, typed("wdraft")...)
			path := pngFile(t, t.TempDir(), "synthetic.png")
			m.attachMessageImages([]string{path})
			if change == "removed" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				data := append(slices.Clone(m.remotes.thread.composer.images[0].Data), 0)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			m = send(m, enterKey)
			mc := m.remotes.thread.composer
			if len(state.pushes) != 0 || len(mc.images) != 0 || mc.input.Value() != "draft" || !strings.Contains(mc.status, "dropped") {
				t.Fatalf("changed attachment sent or draft lost: pushes=%d composer=%+v", len(state.pushes), mc)
			}
		})
	}
}

func TestQuitFromAccountControlsFlushesMessageDraft(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m = send(m, typed("w")...)
	next, _ := m.update(tea.PasteMsg{Content: "synthetic unsaved draft"})
	m = next.(AppModel)
	m.loadProfile = func(context.Context, string) (api.Profile, error) { return api.Profile{}, nil }
	m, _ = m.handleMouse(tea.MouseClickMsg{X: 3, Y: 10, Button: tea.MouseLeft})
	if len(m.profiles) == 0 {
		t.Fatal("profile navigation failed")
	}
	next, _ = m.update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(AppModel)
	if !m.accountOpen {
		t.Fatal("account controls did not open")
	}
	m.update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if got := m.account.store.MessageDrafts("fixture")[fixtureOrg].Body; got != "synthetic unsaved draft" {
		t.Fatalf("draft lost on quit from account: %q", got)
	}
}

func TestTabCompletesMessageEmojiBeforeSelectingImages(t *testing.T) {
	for _, images := range []bool{false, true} {
		t.Run(fmt.Sprint(images), func(t *testing.T) {
			m, _ := messagingApp(t)
			m = openOrg(t, m)
			m = send(m, typed("w")...)
			if images {
				m.attachMessageImages([]string{pngFile(t, t.TempDir(), "synthetic.png")})
			}
			m = send(m, typed(":rocket")...)
			m = send(m, tea.KeyPressMsg{Code: tea.KeyTab})
			if mc := m.remotes.thread.composer; mc.input.Value() != "🚀" || mc.stripFocus {
				t.Fatalf("tab did not accept emoji: %q strip=%t", mc.input.Value(), mc.stripFocus)
			}
		})
	}
}
