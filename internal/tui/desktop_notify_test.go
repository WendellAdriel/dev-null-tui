package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type sentAlert struct{ title, body string }

// alertApp records desktop notifications instead of sending them, in an unfocused terminal.
func alertApp(t *testing.T) (AppModel, *[]sentAlert) {
	t.Helper()
	m, _ := remotesApp(t, auth.RemotePreferences{})
	sent := &[]sentAlert{}
	m.alertTerminal = ""
	m.runAlert = func(_ context.Context, title, body string) error {
		*sent = append(*sent, sentAlert{title, body})
		return nil
	}
	next, _ := m.Update(tea.BlurMsg{})
	return next.(AppModel), sent
}

func countAlert(m AppModel, count int) AppModel {
	m.notificationCountGeneration++
	next, cmd := m.Update(notificationCountResult{m.session, m.notificationCountGeneration, count, nil})
	runAll(cmd)
	return next.(AppModel)
}

func inboxAlert(m AppModel, inbox []api.Conversation) AppModel {
	next, cmd := m.Update(remotesCountResult{session: m.session, inbox: inbox})
	runAll(cmd)
	return next.(AppModel)
}

// incoming gives a conversation a newer unread message from someone else.
func incoming(c api.Conversation, id int64, sender, body string) api.Conversation {
	message := remoteMessage(id, "grace", body, time.Now())
	c.LastMessage, c.LastIncomingMessageID, c.LastIncomingSender, c.Ahead = &message, &id, &sender, c.Ahead+1
	return c
}

func TestARisingNotificationCountSendsOneDesktopNotificationWhileUnfocused(t *testing.T) {
	m, sent := alertApp(t)
	m = countAlert(m, 2)
	if len(*sent) != 0 {
		t.Fatalf("the first count must only set the baseline: %v", *sent)
	}
	m = countAlert(m, 5)
	m = countAlert(m, 5)
	if len(*sent) != 1 || (*sent)[0].body != "3 new notifications" {
		t.Fatalf("a rise must send one notification with its size: %v", *sent)
	}
	m = countAlert(m, 6)
	if (*sent)[1].body != "1 new notification" {
		t.Fatalf("one notification must be singular: %v", *sent)
	}
	next, _ := m.Update(tea.FocusMsg{})
	m = countAlert(next.(AppModel), 9)
	next, _ = m.Update(tea.BlurMsg{})
	m = next.(AppModel)
	m.tuiSettings.DesktopNotifications = false
	countAlert(m, 12)
	if len(*sent) != 2 {
		t.Fatalf("a focused terminal or the switch turned off must send nothing: %v", *sent)
	}
}

func TestNewRemotesMessagesSendTheSenderAndExcerpt(t *testing.T) {
	m, sent := alertApp(t)
	inbox := fixtureInbox(m.now)
	m = inboxAlert(m, inbox)
	if len(*sent) != 0 {
		t.Fatalf("the first inbox must only set the baseline: %v", *sent)
	}
	inbox[0] = incoming(inbox[0], 10, "Ada Lovelace", "are you  around?")
	m = inboxAlert(m, inbox)
	m = inboxAlert(m, inbox)
	if len(*sent) != 1 || (*sent)[0] != (sentAlert{"Ada Lovelace", "are you around?"}) {
		t.Fatalf("a new direct message must name its sender once: %v", *sent)
	}
	org := "Fixture Org"
	inbox[1].Name = &org
	inbox[1] = incoming(inbox[1], 11, "Grace Hopper", "deploy is done")
	m = inboxAlert(m, inbox)
	if (*sent)[1] != (sentAlert{"Grace Hopper in Fixture Org", "deploy is done"}) {
		t.Fatalf("an org message must name the org: %v", *sent)
	}
	inbox[0] = incoming(inbox[0], 12, "Ada Lovelace", "ping?")
	inbox[1] = incoming(inbox[1], 13, "Grace Hopper", "and again")
	m = inboxAlert(m, inbox)
	if (*sent)[2] != (sentAlert{"/dev/null", "New messages in 2 conversations"}) {
		t.Fatalf("several conversations must send one count: %v", *sent)
	}
	later := m.now.Add(time.Hour)
	inbox[0] = incoming(inbox[0], 14, "Ada Lovelace", "muted now")
	inbox[0].MutedUntilAt = &later
	inboxAlert(m, inbox)
	if len(*sent) != 3 {
		t.Fatalf("a muted conversation must stay quiet: %v", *sent)
	}
}

func TestTheOpenRemotesThreadSendsNoDesktopNotification(t *testing.T) {
	m, sent := alertApp(t)
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter")
	if m.remotes.thread == nil {
		t.Fatal("the fixture should open a thread")
	}
	inbox := fixtureInbox(m.now)
	m = inboxAlert(m, inbox)
	open := m.remotes.thread.conversation.ID
	for i := range inbox {
		if inbox[i].ID == open {
			inbox[i] = incoming(inbox[i], 20, "Ada Lovelace", "hi")
		}
	}
	runAll(m.acceptRemotesAlerts(inbox))
	if len(*sent) != 0 {
		t.Fatalf("the open thread must not notify: %v", *sent)
	}
}

func TestDesktopNotificationsUseTheTerminalsOwnCode(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, "\x1b]99;i=devnull-1:d=0;Ada\x1b\\\x1b]99;i=devnull-1:p=body;hi; there\x1b\\"},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, "\x1b]777;notify;Ada;hi, there\x1b\\"},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, "\x1b]777;notify;Ada;hi, there\x1b\\"},
		{"iterm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "\x1b]9;Ada: hi; there\x1b\\"},
		{"ghostty in tmux", map[string]string{"TERM_PROGRAM": "tmux", "GHOSTTY_RESOURCES_DIR": "/x", "TMUX": "/tmp/t"}, "\x1bPtmux;\x1b\x1b]777;notify;Ada;hi, there\x1b\x1b\\\x1b\\"},
		{"anything else", map[string]string{"TERM": "xterm-256color"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind := alertTerminalFor(func(name string) string { return tc.env[name] })
			if got := alertSequence(kind, tc.env["TMUX"] != "", 1, "Ada", "hi; there"); got != tc.want {
				t.Fatalf("sequence %q, want %q", got, tc.want)
			}
		})
	}
}

func TestATerminalCodeIsWrittenInsteadOfTheFallback(t *testing.T) {
	m, sent := alertApp(t)
	m.alertTerminal = "osc777"
	m = countAlert(m, 1)
	m.notificationCountGeneration++
	_, cmd := m.Update(notificationCountResult{m.session, m.notificationCountGeneration, 4, nil})
	var raw []string
	for _, msg := range runAll(cmd) {
		if r, ok := msg.(tea.RawMsg); ok {
			raw = append(raw, r.Msg.(string))
		}
	}
	if len(*sent) != 0 || len(raw) != 1 || raw[0] != "\x1b]777;notify;/dev/null;3 new notifications\x1b\\" {
		t.Fatalf("the terminal code must replace the command: %q %v", raw, *sent)
	}
}

func TestTheFallbackRunsNotifySend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("notify-send is the Linux fallback")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "\n"
	// #nosec G306 -- The fake notify-send must be executable to run.
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	if err := runAlertCommand(context.Background(), "Ada Lovelace", "-hi"); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(out) // #nosec G304
	icon := filepath.Join(dir, "devnull", "notification-icon.png")
	if string(args) != "--app-name=devnull\n--expire-time=10000\n--icon="+icon+"\n--\nAda Lovelace\n-hi\n" {
		t.Fatalf("notify-send got %q", args)
	}
	if saved, _ := os.ReadFile(icon); !bytes.Equal(saved, alertIcon) { // #nosec G304
		t.Fatal("the icon must be written to the cache")
	}
}

func TestLocalLinuxSessionsUseTheSessionBus(t *testing.T) {
	cases := []struct {
		name, goos string
		env        map[string]string
		want       bool
	}{
		{"local linux", "linux", map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"}, true},
		{"runtime dir only", "freebsd", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, true},
		{"over ssh", "linux", map[string]string{"DBUS_SESSION_BUS_ADDRESS": "x", "SSH_CONNECTION": "1 2 3 4"}, false},
		{"no bus", "linux", map[string]string{}, false},
		{"macos", "darwin", map[string]string{"XDG_RUNTIME_DIR": "/x"}, false},
	}
	for _, tc := range cases {
		if got := alertBusFor(tc.goos, func(name string) string { return tc.env[name] }); got != tc.want {
			t.Fatalf("%s: bus %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTheTerminalCodeFollowsARefusedBusNotification(t *testing.T) {
	m, sent := alertApp(t)
	var bus []string
	m.alertBus, m.alertTerminal = true, "osc777"
	m.runBusAlert = func(_ context.Context, title, body string) error {
		bus = append(bus, title+": "+body)
		return nil
	}
	m = countAlert(m, 1)
	m.notificationCountGeneration++
	next, cmd := m.Update(notificationCountResult{m.session, m.notificationCountGeneration, 2, nil})
	m = next.(AppModel)
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(alertFallback); ok {
			t.Fatal("a sent bus notification needs no fallback")
		}
	}
	if len(bus) != 1 || bus[0] != "/dev/null: 1 new notification" || len(*sent) != 0 {
		t.Fatalf("the bus must send it alone: %v %v", bus, *sent)
	}
	m.runBusAlert = func(context.Context, string, string) error { return errors.New("no notification service") }
	m.notificationCountGeneration++
	next, cmd = m.Update(notificationCountResult{m.session, m.notificationCountGeneration, 3, nil})
	m = next.(AppModel)
	var raw []string
	for _, msg := range runAll(cmd) {
		_, cmd := m.Update(msg)
		for _, out := range runAll(cmd) {
			if r, ok := out.(tea.RawMsg); ok {
				raw = append(raw, r.Msg.(string))
			}
		}
	}
	if len(raw) != 1 || raw[0] != "\x1b]777;notify;/dev/null;1 new notification\x1b\\" {
		t.Fatalf("a refused bus notification must fall back to the terminal code: %q", raw)
	}
}

func TestTheTUITabTurnsDesktopNotificationsOff(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openSettings()
	m.settings.busy = false
	for range len(settingsTabs) - 1 {
		m = pressApp(m, "tab")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Desktop notifications: On") {
		t.Fatal("desktop notifications must be on by default")
	}
	m = pressApp(m, "j")
	m = pressApp(m, "j")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = deliver(next.(AppModel), runAll(cmd))
	if m.account.store.TUISettings().DesktopNotifications || !strings.Contains(ansi.Strip(m.View().Content), "Desktop notifications: Off") {
		t.Fatal("turning desktop notifications off must save it")
	}
}
