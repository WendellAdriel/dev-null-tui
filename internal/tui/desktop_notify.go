package tui

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/godbus/dbus/v5"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// alertState remembers what the last polls showed, so only what arrives later sends a desktop notification.
type alertState struct {
	session                            int
	notificationsPrimed, remotesPrimed bool
	// seen holds each conversation's newest incoming message ID from the polls so far.
	seen map[string]int64
}

const alertExcerpt = 100

// alertShownMS is how long, in milliseconds, the desktop keeps a notification up when it lets the sender choose.
const alertShownMS = 10_000

// alertIcon is the web app's dark icon, without the wordmark.
//
//go:embed devnull-icon.png
var alertIcon []byte

// alertFallback asks for the terminal code or OS command after the session bus refused a notification.
type alertFallback struct{ title, body string }

// alertBusFor reports whether notifications can go straight to the desktop over D-Bus, which sets the icon and how
// long they stay. Over SSH the bus belongs to the remote machine, so the terminal's code goes back to the user instead.
func alertBusFor(goos string, getenv func(string) string) bool {
	if goos == "darwin" || goos == "windows" || getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return false
	}
	return getenv("DBUS_SESSION_BUS_ADDRESS") != "" || getenv("XDG_RUNTIME_DIR") != ""
}

// alertIconPath writes the icon to the user cache, since notification servers take a file path, and returns it, or
// "" when it can't be written.
func alertIconPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "devnull", "notification-icon.png")
	if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, alertIcon) { // #nosec G304: Fixed name in the user's cache.
		return path
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil || os.WriteFile(path, alertIcon, 0o600) != nil {
		return ""
	}
	return path
}

// sendBusAlert shows a notification through the freedesktop notification service on the session bus.
func sendBusAlert(ctx context.Context, title, body string) error {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	icon := alertIconPath()
	// No desktop-entry hint: Plasma drops notifications that name an app with no installed .desktop file.
	hints := map[string]dbus.Variant{}
	if icon != "" {
		hints["image-path"] = dbus.MakeVariant(icon)
	}
	call := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(ctx,
		"org.freedesktop.Notifications.Notify", 0, "devnull", uint32(0), icon, title, body, []string{}, hints, int32(alertShownMS))
	return call.Err
}

// alertTerminalFor names the notification code the terminal shows: kitty's OSC 99, OSC 777 for Ghostty and WezTerm,
// or iTerm2's OSC 9. Empty means none is known, so the OS command sends it instead.
func alertTerminalFor(getenv func(string) string) string {
	term, program := getenv("TERM"), getenv("TERM_PROGRAM")
	switch {
	case getenv("KITTY_WINDOW_ID") != "" || term == "xterm-kitty":
		return "kitty"
	case program == "ghostty" || term == "xterm-ghostty" || getenv("GHOSTTY_RESOURCES_DIR") != "", program == "WezTerm" || getenv("WEZTERM_PANE") != "":
		return "osc777"
	case program == "iTerm.app" || getenv("ITERM_SESSION_ID") != "":
		return "osc9"
	}
	return ""
}

// alertSequence is the escape code for one notification, wrapped for tmux, which passes it on only with
// allow-passthrough on. id keeps kitty from replacing an earlier notification.
func alertSequence(kind string, tmux bool, id int, title, body string) string {
	var seq string
	switch kind {
	case "kitty":
		key := fmt.Sprintf("i=devnull-%d", id)
		seq = "\x1b]99;" + key + ":d=0;" + title + "\x1b\\" + "\x1b]99;" + key + ":p=body;" + body + "\x1b\\"
	case "osc777":
		// OSC 777 splits its fields on semicolons, so a semicolon in the text would cut it short.
		field := strings.NewReplacer(";", ",")
		seq = "\x1b]777;notify;" + field.Replace(title) + ";" + field.Replace(body) + "\x1b\\"
	case "osc9":
		seq = "\x1b]9;" + title + ": " + body + "\x1b\\"
	default:
		return ""
	}
	if tmux {
		return ansi.TmuxPassthrough(seq)
	}
	return seq
}

// runAlertCommand sends a notification through the OS: notify-send on Linux and the BSDs, osascript on macOS, and a
// toast from PowerShell on Windows. The text goes in as arguments or environment, never as script.
func runAlertCommand(ctx context.Context, title, body string) error {
	var args, env []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"osascript", "-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", title, body}
	case "windows":
		// A long toast stays about 25 seconds instead of 7, and the image template shows the icon beside the text.
		template, icon := "ToastText02", alertIconPath()
		if icon != "" {
			template, icon = "ToastImageAndText02", "file:///"+filepath.ToSlash(icon)
		}
		script := `$m=[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime];` +
			`$x=$m::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::` + template + `);` +
			`[void]$x.DocumentElement.SetAttribute('duration','long');` +
			`if($env:DEVNULL_ALERT_ICON){[void]$x.GetElementsByTagName('image').Item(0).SetAttribute('src',$env:DEVNULL_ALERT_ICON)};` +
			`$t=$x.GetElementsByTagName('text');` +
			`[void]$t.Item(0).AppendChild($x.CreateTextNode($env:DEVNULL_ALERT_TITLE));` +
			`[void]$t.Item(1).AppendChild($x.CreateTextNode($env:DEVNULL_ALERT_BODY));` +
			`$m::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show([Windows.UI.Notifications.ToastNotification]::new($x))`
		args = []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script}
		env = []string{"DEVNULL_ALERT_TITLE=" + title, "DEVNULL_ALERT_BODY=" + body, "DEVNULL_ALERT_ICON=" + icon}
	default:
		args = []string{"notify-send", "--app-name=devnull", fmt.Sprintf("--expire-time=%d", alertShownMS)}
		if icon := alertIconPath(); icon != "" {
			args = append(args, "--icon="+icon)
		}
		args = append(args, "--", title, body)
	}
	path, err := exec.LookPath(args[0])
	if err != nil {
		return errors.New("desktop notifications unavailable")
	}
	// #nosec G204: Executable and arguments come only from the fixed platform list, with the text as plain arguments.
	cmd := exec.CommandContext(ctx, path, args[1:]...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.WaitDelay = time.Second
	return cmd.Run()
}

// sendAlert shows one desktop notification, while the switch is on and the terminal isn't focused.
func (m *AppModel) sendAlert(title, body string) tea.Cmd {
	if !m.tuiSettings.DesktopNotifications || m.focused {
		return nil
	}
	title, body = safeText(title), safeText(body)
	if m.alertBus {
		send := m.runBusAlert
		return m.account.life.command(func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if send(ctx, title, body) != nil {
				return alertFallback{title, body}
			}
			return nil
		})
	}
	return m.alertWithoutBus(title, body)
}

// alertWithoutBus sends the terminal's notification code, or runs the OS command when the terminal has none.
func (m *AppModel) alertWithoutBus(title, body string) tea.Cmd {
	m.alertCount++
	if seq := alertSequence(m.alertTerminal, m.alertTmux, m.alertCount, title, body); seq != "" {
		return tea.Raw(seq)
	}
	run := m.runAlert
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = run(ctx, title, body)
		return nil
	})
}

// alerts resets the baselines when the account changes, so a switch never notifies about the new account's backlog.
func (m *AppModel) alerts() *alertState {
	if m.alertState.session != m.session || m.alertState.seen == nil {
		m.alertState = alertState{session: m.session, seen: map[string]int64{}}
	}
	return &m.alertState
}

// acceptNotificationAlert notifies about a rise in the unread count. The first count only sets the baseline.
func (m *AppModel) acceptNotificationAlert(previous, count int) tea.Cmd {
	a := m.alerts()
	if !a.notificationsPrimed {
		a.notificationsPrimed = true
		return nil
	}
	rise := count - previous
	if rise <= 0 {
		return nil
	}
	noun := "notifications"
	if rise == 1 {
		noun = "notification"
	}
	return m.sendAlert("/dev/null", fmt.Sprintf("%d new %s", rise, noun))
}

// acceptRemotesAlerts notifies about conversations with a newer unread incoming message than the last poll showed,
// leaving out muted ones and the open thread. The first inbox only sets the baseline.
func (m *AppModel) acceptRemotesAlerts(inbox []api.Conversation) tea.Cmd {
	a := m.alerts()
	var fresh []api.Conversation
	for _, c := range inbox {
		if c.LastIncomingMessageID == nil {
			continue
		}
		id := *c.LastIncomingMessageID
		last, known := a.seen[c.ID]
		if known && id <= last {
			continue
		}
		a.seen[c.ID] = id
		if a.remotesPrimed && c.Ahead > 0 && !m.muted(c) && !m.threadOpen(c.ID) {
			fresh = append(fresh, c)
		}
	}
	a.remotesPrimed = true
	switch len(fresh) {
	case 0:
		return nil
	case 1:
		return m.sendAlert(m.alertSender(fresh[0]), m.alertBody(fresh[0]))
	}
	return m.sendAlert("/dev/null", fmt.Sprintf("New messages in %d conversations", len(fresh)))
}

func (m AppModel) threadOpen(id string) bool {
	return m.remotes.open && m.remotes.thread != nil && m.remotes.thread.conversation.ID == id
}

// alertSender names who wrote, and the org, like the web's title for a new message.
func (m AppModel) alertSender(c api.Conversation) string {
	sender := "Someone"
	if c.LastIncomingSender != nil && strings.TrimSpace(*c.LastIncomingSender) != "" {
		sender = *c.LastIncomingSender
	}
	if c.Kind != "org" {
		return sender
	}
	org := "an unnamed org"
	if c.Name != nil && *c.Name != "" {
		org = *c.Name
	}
	return sender + " in " + org
}

// alertBody is the start of the new message, or what kind it is when the inbox's last message is a different one.
func (m AppModel) alertBody(c api.Conversation) string {
	message := c.LastMessage
	if message == nil || message.ID != *c.LastIncomingMessageID {
		return "Sent you a message"
	}
	body := strings.Join(strings.Fields(safeText(message.Body)), " ")
	switch {
	case message.Event == "ping":
		return "Sent you a ping"
	case message.Event != "":
		return m.eventText(*message, c.Partner)
	case body != "":
		return ansi.Truncate(body, alertExcerpt, "…")
	case message.Voice != nil:
		return "Sent a voice note"
	case len(message.Images) > 0:
		return "Sent an image"
	}
	return "Sent you a message"
}
