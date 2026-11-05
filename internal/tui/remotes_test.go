package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const fixtureOrg = "0b7f3c1e-8a4d-4e2b-9c6f-1d2e3f4a5b6c"

func remoteMessage(id int64, user, body string, at time.Time) api.Message {
	return api.Message{ID: id, UserID: user, Body: body, CreatedAt: at}
}

func fixtureInbox(now time.Time) []api.Conversation {
	ada := &api.User{ID: "ada", Handle: "ada", Name: "Ada Lovelace"}
	own := remoteMessage(7, "fixture", "see you there", now.Add(-time.Hour))
	org := remoteMessage(9, "grace", "standup moved", now.Add(-2*time.Hour))
	org.User = &api.User{ID: "grace", Handle: "grace", Name: "Grace Hopper"}
	return []api.Conversation{
		{ID: "d1", Kind: "direct", Partner: ada, LastMessage: &own, LastMessageAt: &own.CreatedAt, PartnerReadThroughID: 7},
		{ID: fixtureOrg, Kind: "org", Members: []api.Member{{User: api.User{ID: "fixture", Handle: "fixture", Name: "Fixture"}}, {User: api.User{ID: "grace", Handle: "grace", Name: "Grace Hopper"}}, {User: api.User{ID: "linus", Handle: "linus", Name: "Linus Torvalds"}}}, LastMessage: &org, LastMessageAt: &org.CreatedAt, Ahead: 2, LastIncomingMessageID: new(int64(9))},
	}
}

type remoteCallLog struct{ calls []string }

func (l *remoteCallLog) add(format string, args ...any) {
	l.calls = append(l.calls, fmt.Sprintf(format, args...))
}
func (l *remoteCallLog) String() string { return strings.Join(l.calls, "|") }

func remotesApp(t *testing.T, prefs auth.RemotePreferences) (AppModel, *remoteCallLog) {
	t.Helper()
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(AppModel)
	m.account.user.Remotes = &prefs
	m.remotes.unread = prefs.UnreadCount
	log := &remoteCallLog{}
	now := m.now
	m.remoteCalls = remoteCalls{
		inbox: func(_ context.Context, tab api.InboxTab, term, before string) (api.Inbox, error) {
			log.add("inbox %s %q %s", tab, term, before)
			if tab != api.Open {
				return api.Inbox{Data: []api.Conversation{}}, nil
			}
			// The server counts unread conversations plus fetched requests, and only the org is unread here.
			unread := 1
			if prefs.Requests {
				unread++
			}
			return api.Inbox{Data: fixtureInbox(now), UnreadCount: unread, RequestsCount: 1}, nil
		},
		thread: func(_ context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
			log.add("thread %s%s %d", target.Handle, target.Org, q.After)
			if target.Org == "" {
				return api.RemoteThread{Partner: &api.User{ID: "ada", Handle: "ada", Name: "Ada Lovelace"}, Messages: []api.Message{}, Access: "open"}, nil
			}
			created := remoteMessage(1, "grace", "", now.Add(-26*time.Hour))
			created.Event, created.User = "created", &api.User{ID: "grace", Handle: "grace", Name: "Grace Hopper"}
			hello := remoteMessage(8, "fixture", "morning all", now.Add(-3*time.Hour))
			moved := remoteMessage(9, "grace", "standup moved", now.Add(-2*time.Hour))
			moved.User = created.User
			if q.After > 0 {
				return api.RemoteThread{Org: &api.Org{ID: fixtureOrg}, Messages: []api.Message{}, Access: "open", Typing: []string{"linus"}}, nil
			}
			return api.RemoteThread{Org: &api.Org{ID: fixtureOrg}, Messages: []api.Message{created, hello, moved}, Members: fixtureInbox(now)[1].Members, Access: "open", Role: "contributor"}, nil
		},
		markRead: func(_ context.Context, target api.Target, through int64) error {
			log.add("receipt %s%s %d", target.Handle, target.Org, through)
			return nil
		},
		stash: func(_ context.Context, target api.Target, stashed bool) error {
			log.add("stash %s%s %t", target.Handle, target.Org, stashed)
			return nil
		},
		mute: func(_ context.Context, target api.Target, d api.MuteDuration) (*time.Time, error) {
			log.add("mute %s%s %s", target.Handle, target.Org, d)
			until := now.Add(time.Hour)
			return &until, nil
		},
		pin: func(_ context.Context, target api.Target, pinned bool) (*time.Time, error) {
			log.add("pin %s%s %t", target.Handle, target.Org, pinned)
			if !pinned {
				return nil, nil
			}
			return &now, nil
		},
		leave: func(_ context.Context, org, handle string) error {
			log.add("leave %s %s", org, handle)
			return nil
		},
		block: func(_ context.Context, handle string) error {
			log.add("block %s", handle)
			return nil
		},
	}
	return m, log
}

// settle presses keys and runs every follow-up command, skipping timers, which never finish within the wait.
func settle(t *testing.T, m AppModel, keys ...string) AppModel {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.handleKey(k)
		m = settleCmd(next, cmd)
	}
	return m
}

func settleCmd(m AppModel, cmd tea.Cmd) AppModel {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- next() }()
		select {
		case msg := <-done:
			if batch, ok := msg.(tea.BatchMsg); ok {
				queue = append(queue, batch...)
				continue
			}
			if msg == nil {
				continue
			}
			model, follow := m.update(msg)
			m = model.(AppModel)
			queue = append(queue, follow)
		case <-time.After(50 * time.Millisecond):
		}
	}
	return m
}

// savedSettings waits for a tui.json write that settleCmd stopped waiting for after 50 ms on a slow machine.
func savedSettings(m AppModel, saved func(auth.TUISettings) bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for !saved(m.account.store.TUISettings()) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func navigationText(m AppModel) string {
	nav, _, _ := m.columns()
	view, _ := m.navigationLayout(nav, m.mainHeight(), colors(true))
	return ansi.Strip(view)
}

func TestRemotesSidebarItemFollowsTheAccount(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = next.(AppModel)
	if strings.Contains(navigationText(m), "Remotes") {
		t.Fatal("the sidebar shows Remotes for an account without them")
	}
	if m, _ = m.handleKey("g"); m.pendingG {
		m = settle(t, m, "r")
	}
	if m.remotes.open {
		t.Fatal("g r opens Remotes for an account without them")
	}
	m, _ = remotesApp(t, auth.RemotePreferences{UnreadCount: 3})
	nav := navigationText(m)
	if !strings.Contains(nav, "Remotes") || !strings.Contains(nav, "3") || !strings.Contains(nav, "g r") {
		t.Fatalf("sidebar:\n%s", nav)
	}
	if strings.Index(nav, "Notifications") > strings.Index(nav, "Remotes") || strings.Index(nav, "Remotes") > strings.Index(nav, "Profile") {
		t.Fatalf("Remotes is not between Notifications and Profile:\n%s", nav)
	}
}

func TestRemotesInboxShowsTheWebRows(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	if !m.remotes.open || log.String() != `inbox open "" ` {
		t.Fatalf("open: %s", log)
	}
	frame := frameText(m)
	for _, want := range []string{"Remotes", "Open", "Stashed", "@ada", "Ada Lovelace", "you: see you there", "Grace, Lin", "org · 3", "Grace: standup moved", "2 ahead", "no conversation checked out"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("inbox misses %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Fetched") {
		t.Fatal("Fetched shows for an account that turned requests off")
	}
	if m.remotes.unread != 1 {
		t.Fatalf("unread badge %d, want the server's 1", m.remotes.unread)
	}
	m = settle(t, m, "tab")
	if m.remotes.tab != api.Stashed || !strings.Contains(frameText(m), "nothing stashed") {
		t.Fatalf("tab %s:\n%s", m.remotes.tab, frameText(m))
	}

	m, _ = remotesApp(t, auth.RemotePreferences{Requests: true})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	if !strings.Contains(frameText(m), "Fetched · 1 waiting") {
		t.Fatalf("Fetched tab:\n%s", frameText(m))
	}
	m = settle(t, m, "tab")
	if m.remotes.tab != api.Fetched || !strings.Contains(frameText(m), "no requests fetched") {
		t.Fatalf("tab %s", m.remotes.tab)
	}
}

func TestRemotesSearchFiltersTheInbox(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "/", "g", "r", "a", "enter")
	if !strings.HasSuffix(log.String(), `inbox open "gra" `) || m.remotes.term != "gra" {
		t.Fatalf("search: %s", log)
	}
	m = settle(t, m, "esc")
	if m.remotes.term != "" || !strings.HasSuffix(log.String(), `inbox open "" `) || !m.remotes.open {
		t.Fatalf("clearing search: %s", log)
	}
}

func TestRemotesOpensAThreadAndMovesTheReadReceipt(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "enter")
	if !strings.Contains(log.String(), "thread "+fixtureOrg+" 0") || !strings.Contains(log.String(), "receipt "+fixtureOrg+" 9") {
		t.Fatalf("calls: %s", log)
	}
	frame := frameText(m)
	for _, want := range []string{"* @grace created the org", "Today", "morning all", "standup moved", "Grace Hopper"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("thread misses %q:\n%s", want, frame)
		}
	}
	if m.remotes.inbox[1].Ahead != 0 || m.remotes.unread != 0 {
		t.Fatalf("read receipt kept ahead %d, unread %d", m.remotes.inbox[1].Ahead, m.remotes.unread)
	}
	calls := len(log.calls)
	poll := m.pollRemotes()
	m = settleCmd(m, poll)
	if !strings.Contains(strings.Join(log.calls[calls:], "|"), "thread "+fixtureOrg+" 9") {
		t.Fatalf("poll did not ask for newer messages: %s", log)
	}
	if !strings.Contains(frameText(m), "@linus is typing…") {
		t.Fatalf("typing:\n%s", frameText(m))
	}
	if strings.Count(log.String(), "receipt ") != 1 {
		t.Fatalf("polling moved the receipt again: %s", log)
	}
}

func TestRemotesDropsStaleThreadResults(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter")
	stale := remoteThreadResult{session: m.session, generation: m.remotes.thread.generation - 1, thread: api.RemoteThread{Partner: &api.User{Handle: "ada"}, Messages: []api.Message{remoteMessage(5, "ada", "stale body", m.now)}, Access: "open"}}
	next, _ := m.update(stale)
	if strings.Contains(frameText(next.(AppModel)), "stale body") {
		t.Fatal("a stale thread result replaced the open thread")
	}
}

func TestRemotesConversationActions(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "P")
	if !strings.HasSuffix(log.String(), "pin "+fixtureOrg+" true") || m.remotes.inbox[0].ID != fixtureOrg || m.remotes.selected != 0 {
		t.Fatalf("pin: %s, first %s", log, m.remotes.inbox[0].ID)
	}
	m = settle(t, m, "M")
	if m.remotes.dialog != "mute" || !strings.Contains(frameText(m), "For one hour") {
		t.Fatalf("mute dialog:\n%s", frameText(m))
	}
	m = settle(t, m, "j", "enter")
	if !strings.HasSuffix(log.String(), "mute "+fixtureOrg+" tomorrow") || m.remotes.inbox[0].MutedUntilAt == nil {
		t.Fatalf("mute: %s", log)
	}
	m = settle(t, m, "S")
	if !strings.HasSuffix(log.String(), "stash "+fixtureOrg+" true") || len(m.remotes.inbox) != 1 || m.toast.title != "Stashed Grace, Linus." || m.toast.detail != "It pops back when someone writes." {
		t.Fatalf("stash: %s, toast %q", log, m.toast.title)
	}
	m = settle(t, m, ".")
	if m.remotes.dialog != "menu" || !strings.Contains(frameText(m), "View profile") || !strings.Contains(frameText(m), "Block @ada") {
		t.Fatalf("menu:\n%s", frameText(m))
	}
	m = settle(t, m, "esc", "B")
	if m.remotes.dialog != "block" || !strings.Contains(frameText(m), "Block @ada?") {
		t.Fatalf("block modal:\n%s", frameText(m))
	}
	m = settle(t, m, "tab", "enter")
	if !strings.HasSuffix(log.String(), "block ada") || len(m.remotes.inbox) != 0 || m.remotes.dialog != "" {
		t.Fatalf("block: %s", log)
	}
}

func TestRemotesLeaveOrgAndActionFailures(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	m.remoteCalls.pin = func(context.Context, api.Target, bool) (*time.Time, error) { return nil, errors.New("down") }
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "P")
	if m.toast.title != "Could not update the pin. Try again." || m.remotes.inbox[0].PinnedAt != nil {
		t.Fatalf("pin failure toast %q", m.toast.title)
	}
	m = settle(t, m, "j", "B")
	if !strings.Contains(frameText(m), "Leave Grace, Linus?") {
		t.Fatalf("leave modal:\n%s", frameText(m))
	}
	m = settle(t, m, "tab", "enter")
	if !strings.HasSuffix(log.String(), "leave "+fixtureOrg+" fixture") || len(m.remotes.inbox) != 1 {
		t.Fatalf("leave: %s", log)
	}
}

func TestRemotesStateIsWipedOnSignOut(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter")
	m.leaveApplication()
	if m.remotes.open || m.remotes.inbox != nil || m.remotes.thread != nil || m.remoteCalls.inbox != nil {
		t.Fatal("sign-out kept Remotes state")
	}
}

func TestRemotesEventTextsFollowTheWeb(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	grace := &api.User{ID: "grace", Handle: "grace"}
	ada := &api.User{ID: "ada", Handle: "ada"}
	for _, c := range []struct {
		message api.Message
		want    string
	}{
		{api.Message{Event: "created", UserID: "grace", User: grace, Body: "Standups"}, "@grace created Standups"},
		{api.Message{Event: "added", UserID: "fixture", Subject: ada}, "you added @ada"},
		{api.Message{Event: "renamed", UserID: "grace", User: grace}, "@grace cleared the org's name"},
		{api.Message{Event: "promoted", UserID: "grace", User: grace, Subject: &api.User{ID: "fixture"}}, "@grace made you a maintainer"},
		{api.Message{Event: "call_missed", UserID: "fixture"}, "your call wasn't answered"},
		{api.Message{Event: "call_missed", UserID: "ada"}, "missed call from @ada"},
		{api.Message{Event: "ping", UserID: "ada"}, "you got a ping"},
		{api.Message{Event: "ping", UserID: "grace", User: grace}, "@grace sent a ping"},
	} {
		if got := m.eventText(c.message, ada); got != c.want {
			t.Errorf("%s: %q, want %q", c.message.Event, got, c.want)
		}
	}
}

func TestRemotesRetriesPendingLinkCardsThenGivesUp(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m.remoteCalls.linkPreview = func(context.Context, int64) (*api.LinkPreview, error) { return nil, nil }
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter")
	pending := func() {
		message := remoteMessage(5, "ada", "https://go.dev", m.now)
		message.LinkPreview = &api.LinkPreview{URL: "https://go.dev", Pending: true}
		m.remotes.thread.messages = []api.Message{message}
		m.renderRemoteThread()
	}
	content := func() string { return ansi.Strip(m.remotes.thread.viewport.GetContent()) }
	pending()
	if !strings.Contains(content(), "Loading preview…") {
		t.Fatal("a pending card does not say it is loading")
	}
	if m.refreshLinkPreviews() == nil || m.refreshLinkPreviews() != nil {
		t.Fatal("a pending card does not start exactly one refresh")
	}
	still := &api.LinkPreview{URL: "https://go.dev", Pending: true}
	if m.acceptLinkPreview(remoteLinkPreviewResult{session: m.session, id: 5, preview: still}) == nil {
		t.Fatal("a card still pending is not asked for again")
	}
	if m.acceptLinkPreview(remoteLinkPreviewResult{session: m.session, attempt: len(linkPreviewDelays) - 1, id: 5, preview: still}) != nil || strings.Contains(content(), "Loading preview…") {
		t.Fatalf("the card keeps loading after the last wait:\n%s", content())
	}
	pending()
	if m.acceptLinkPreview(remoteLinkPreviewResult{session: m.session, id: 5, err: errors.New("gone")}) != nil || strings.Contains(content(), "Loading preview…") {
		t.Fatal("a failed refresh keeps the card loading")
	}
	pending()
	next, _ := m.update(remoteLinkPreviewResult{session: m.session, id: 5, preview: &api.LinkPreview{URL: "https://go.dev", Title: "The Go Programming Language", SiteName: "go.dev"}})
	m = next.(AppModel)
	if !strings.Contains(content(), "The Go Programming Language") {
		t.Fatalf("the refreshed card is not drawn:\n%s", content())
	}
}

func TestRemotesInboxPollKeepsOlderPages(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	now := m.now
	page := func(from int) []api.Conversation {
		var rows []api.Conversation
		for i := from; i < from+30; i++ {
			at := now.Add(-time.Duration(i) * time.Minute)
			rows = append(rows, api.Conversation{ID: fmt.Sprint("c", i), Kind: "direct", Partner: &api.User{ID: fmt.Sprint("u", i), Handle: fmt.Sprint("user", i)}, LastMessageAt: &at})
		}
		return rows
	}
	m.remoteCalls.inbox = func(_ context.Context, _ api.InboxTab, _, before string) (api.Inbox, error) {
		if before != "" {
			return api.Inbox{Data: page(30)}, nil
		}
		return api.Inbox{Data: page(0), HasMore: true}, nil
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "n")
	m.remotes.selected = 45
	m = settleCmd(m, m.fetchInbox("poll"))
	if len(m.remotes.inbox) != 60 || m.remotes.selected != 45 || m.remotes.inbox[45].ID != "c45" || m.remotes.hasMore {
		t.Fatalf("poll kept %d rows, selection %d, more %t", len(m.remotes.inbox), m.remotes.selected, m.remotes.hasMore)
	}
}

func TestRemotesPollKeepsPinnedMessages(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	thread := m.remoteCalls.thread
	var pinned []api.Message
	m.remoteCalls.thread = func(ctx context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		result, err := thread(ctx, target, q)
		result.Pinned = []api.Message{remoteMessage(9, "grace", "standup moved", m.now)}
		if q.After > 0 {
			result.Pinned = pinned
		}
		return result, err
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "enter")
	m = settleCmd(m, m.fetchRemoteThread("poll"))
	if len(m.remotes.thread.data.Pinned) != 1 {
		t.Fatal("an unchanged poll dropped the pinned messages")
	}
	pinned = []api.Message{}
	m = settleCmd(m, m.fetchRemoteThread("poll"))
	if len(m.remotes.thread.data.Pinned) != 0 {
		t.Fatal("an empty pinned list did not unpin everything")
	}
}

func TestRemotesReadsOnlyAVisibleThreadAtTheNewestMessage(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	var messages []api.Message
	for id := int64(1); id <= 60; id++ {
		messages = append(messages, remoteMessage(id, "grace", fmt.Sprint("line ", id), m.now))
	}
	m.remoteCalls.thread = func(_ context.Context, _ api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		if q.After > 0 {
			ping := remoteMessage(q.After+1, "grace", "", m.now)
			ping.Event = "ping"
			return api.RemoteThread{Org: &api.Org{ID: fixtureOrg}, Messages: []api.Message{ping}, Access: "open"}, nil
		}
		return api.RemoteThread{Org: &api.Org{ID: fixtureOrg}, Messages: messages, Access: "open"}, nil
	}
	receipts := func() string {
		var found []string
		for _, call := range log.calls {
			if strings.HasPrefix(call, "receipt ") {
				found = append(found, call)
			}
		}
		return strings.Join(found, "|")
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "enter")
	if receipts() != "receipt "+fixtureOrg+" 60" {
		t.Fatalf("opening: %s", receipts())
	}
	m = settle(t, m, "home")
	m = settleCmd(m, m.fetchRemoteThread("poll"))
	if receipts() != "receipt "+fixtureOrg+" 60" {
		t.Fatalf("a scrolled-up thread moved the receipt: %s", receipts())
	}
	m = settle(t, m, "G")
	if !strings.HasSuffix(receipts(), "receipt "+fixtureOrg+" 61") {
		t.Fatalf("scrolling back to the newest ping did not read it: %s", receipts())
	}
	m.settings.open = true
	m = settleCmd(m, m.fetchRemoteThread("poll"))
	if strings.Contains(receipts(), " 62") {
		t.Fatalf("a covered thread moved the receipt: %s", receipts())
	}
}

func TestRemotesTickRunsOnceAndStopsWithTheScreen(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m.remoteCalls.inbox = func(context.Context, api.InboxTab, string, string) (api.Inbox, error) {
		return api.Inbox{}, errors.New("offline")
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	if !m.remotes.ticking || m.scheduleRemotes() != nil {
		t.Fatal("a failed first load does not keep one tick running")
	}
	tick := remotesTick{m.session, m.remotes.tickGeneration}
	m.focused = false
	next, cmd := m.update(tea.FocusMsg{})
	m = next.(AppModel)
	if cmd == nil || !m.remotes.loading {
		t.Fatal("focus does not poll at once")
	}
	m.account.user.Remotes = nil
	if m.acceptRemotesTick(remotesTick{m.session, m.remotes.tickGeneration}); m.remotes.open {
		t.Fatal("turning Remotes off leaves the screen polling")
	}
	m.remotes.open = true
	if m.acceptRemotesTick(tick) != nil {
		t.Fatal("a tick from a closed screen kept running")
	}
}

func TestRemotesMuteDialogClicksPickTheRowUnderThePointer(t *testing.T) {
	for _, choice := range muteChoices {
		m, log := remotesApp(t, auth.RemotePreferences{})
		m, _ = m.handleKey("g")
		m = settle(t, m, "r", "j", "M")
		lines := strings.Split(frameText(m), "\n")
		clicked := false
		for y, line := range lines {
			if x := strings.Index(line, choice.label); x >= 0 {
				next, cmd := m.handleMouse(tea.MouseClickMsg{X: ansi.StringWidth(line[:x]) + 1, Y: y, Button: tea.MouseLeft})
				settleCmd(next, cmd)
				clicked = true
				break
			}
		}
		if !clicked || !strings.HasSuffix(log.String(), "mute "+fixtureOrg+" "+string(choice.duration)) {
			t.Fatalf("clicking %q: %s", choice.label, log)
		}
	}
}

func TestRemotesActionsDropAnInboxReadSentBeforeThem(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	poll := m.fetchInbox("poll")
	next, _ := m.update(remoteActionResult{session: m.session, kind: "stash", conversation: m.remotes.inbox[0], value: true})
	m = settleCmd(next.(AppModel), poll)
	for _, c := range m.remotes.inbox {
		if c.ID == "d1" {
			t.Fatal("an older inbox read brought back the stashed conversation")
		}
	}
}

func TestRemotesMessagesLinkOnlyTheirMentions(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m.account.origin = "https://dev-null.test"
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter")
	message := remoteMessage(5, "ada", "@ada and @bob", m.now)
	message.Mentions = []api.Mention{{Handle: "Ada"}}
	m.remotes.thread.messages = []api.Message{message}
	m.renderRemoteThread()
	content := m.remotes.thread.viewport.GetContent()
	if !strings.Contains(content, "https://dev-null.test/@ada") || strings.Contains(content, "https://dev-null.test/@bob") {
		t.Fatalf("message mentions: %q", content)
	}
}

func TestRemotesSinglePaneTabAndSearchQuestionMark(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = next.(AppModel).handleKey("g")
	m = settle(t, m, "r", "enter", "tab")
	if m.remotes.thread != nil {
		t.Fatal("tab in a single pane leaves the hidden thread open")
	}
	m = settle(t, m, "/", "?")
	if m.help || m.remotes.query != "?" {
		t.Fatalf("? in search opened help %t, query %q", m.help, m.remotes.query)
	}
}

func TestRemotesBadgeRefreshesWhileClosed(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{UnreadCount: 7})
	m = settleCmd(m, m.refreshRemotesCount())
	if m.remotes.unread != 1 || log.String() != `inbox open "" ` {
		t.Fatalf("badge %d after %s", m.remotes.unread, log)
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	if m.refreshRemotesCount() != nil {
		t.Fatal("the badge refresh runs while the inbox is open")
	}
}

func TestBadgeTickRefreshesBothBadges(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{UnreadCount: 7})
	m.fetchNotificationCount = func(context.Context) (int, error) { return 4, nil }
	m = settleCmd(m, m.acceptBadgeTick(badgeTick{m.session, m.badgeTicks}))
	if m.remotes.unread != 1 || log.String() != `inbox open "" ` || m.unreadNotifications != 4 {
		t.Fatalf("remotes %d, notifications %d after %s", m.remotes.unread, m.unreadNotifications, log)
	}
	if m.acceptBadgeTick(badgeTick{m.session - 1, m.badgeTicks}) != nil {
		t.Fatal("a tick from an old session keeps running")
	}
}

func TestRemotesOlderMessagesLoadAboveTheThread(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	thread := m.remoteCalls.thread
	m.remoteCalls.thread = func(ctx context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		if q.Before > 0 {
			log.add("older %d", q.Before)
			return api.RemoteThread{Org: &api.Org{ID: fixtureOrg}, Messages: []api.Message{remoteMessage(1, "grace", "", m.now)}, Access: "open"}, nil
		}
		result, err := thread(ctx, target, q)
		result.HasOlder = true
		return result, err
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "enter", "n")
	if !strings.HasSuffix(log.String(), "older 1") || m.remotes.thread.data.HasOlder || len(m.remotes.thread.messages) != 3 {
		t.Fatalf("older: %s, %d messages", log, len(m.remotes.thread.messages))
	}
}

func TestRemotesStatusNamesOnlyOlderLoads(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "enter")
	m.remotes.thread.data.HasOlder = true
	if m.fetchRemoteThread("poll") == nil || strings.Contains(m.remotesStatus(), "older") {
		t.Fatalf("a poll reads %q", m.remotesStatus())
	}
	m.remotes.thread.loading = false
	if m.fetchRemoteThread("older") == nil || m.remotesStatus() != "Loading older messages..." {
		t.Fatalf("an older load reads %q", m.remotesStatus())
	}
}

func TestRemotesMouseOpensConversationsAndTheMenu(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	nav, _, _ := m.columns()
	click := func(x, y int) {
		next, cmd := m.handleMouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		m = settleCmd(next, cmd)
	}
	click(nav+3, remotesHeaderHeight+inboxHeaderHeight+inboxRowHeight)
	if m.remotes.thread == nil || m.remotes.thread.conversation.ID != fixtureOrg || !strings.Contains(log.String(), "thread "+fixtureOrg) {
		t.Fatalf("click did not open the org: %s", log)
	}
	click(nav+1+m.remotesWidth()-2, remotesHeaderHeight)
	if m.remotes.dialog != "menu" || m.remotes.dialogTarget.ID != fixtureOrg {
		t.Fatalf("header click dialog %q", m.remotes.dialog)
	}
	click(0, 0)
	if m.remotes.dialog != "" {
		t.Fatal("a click outside the menu does not close it")
	}
}

func TestRemotesMessagesSitInWebBubbles(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "j", "enter")
	p := colors(m.account.dark)
	content := m.remotes.thread.viewport.GetContent()
	background := func(color string) string {
		var r, g, b int
		_, _ = fmt.Sscanf(color, "#%02x%02x%02x", &r, &g, &b)
		return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	}
	for _, want := range []string{background(p.wash), background(p.blue)} {
		if !strings.Contains(content, want) {
			t.Fatalf("a bubble background %q is missing", want)
		}
	}
	plain := ansi.Strip(content)
	clock := m.remotes.thread.messages[2].CreatedAt.In(time.Local).Format("3:04 PM")
	if !strings.Contains(plain, "● Today") || !strings.Contains(plain, clock) {
		t.Fatalf("day chip or run time missing:\n%s", plain)
	}
	if strings.Index(plain, "Grace Hopper") > strings.Index(plain, "standup moved") {
		t.Fatal("the sender's name is not above the bubble")
	}
}

func TestRemotesLeavingAConversationClosesIt(t *testing.T) {
	for _, key := range []string{"esc", "left", "h"} {
		m, _ := remotesApp(t, auth.RemotePreferences{})
		m, _ = m.handleKey("g")
		m = settle(t, m, "r", "j", "enter", key)
		if m.remotes.thread != nil || !strings.Contains(frameText(m), "no conversation checked out") {
			t.Fatalf("%s keeps the conversation on screen:\n%s", key, frameText(m))
		}
	}
}

func TestRemotesInboxStatusShowsItsShortcuts(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	status := m.remotesStatus()
	for _, want := range []string{"1 / 2", "enter open", "tab stashed", "/ search", ". actions", "esc back"} {
		if !strings.Contains(status, want) {
			t.Fatalf("inbox status %q is missing %q", status, want)
		}
	}
}

func TestRemotesReactionsSitInBorderedChipsBelowTheBubble(t *testing.T) {
	chips, _ := reactionChips([]api.Reaction{{UserID: "a", Emoji: "🚀"}, {UserID: "b", Emoji: "🚀"}, {UserID: "a", Emoji: "👍"}}, "c", colors(true))
	if len(chips) != 3 {
		t.Fatalf("chips are %d rows, want a bordered row of 3", len(chips))
	}
	plain := ansi.Strip(strings.Join(chips, "\n"))
	if !strings.Contains(plain, "╭") || !strings.Contains(plain, "│ 🚀 2 │") || !strings.Contains(plain, "│ 👍 1 │") {
		t.Fatalf("chips:\n%s", plain)
	}
	var r, g, b int
	_, _ = fmt.Sscanf(colors(true).wash, "#%02x%02x%02x", &r, &g, &b)
	if strings.Contains(strings.Join(chips, ""), fmt.Sprintf("48;2;%d;%d;%d", r, g, b)) {
		t.Fatal("chips still paint a background")
	}
}
