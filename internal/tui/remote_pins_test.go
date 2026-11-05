package tui

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// pins is an org thread opened as role whose first load carries pinned, the IDs of the fixture messages pinned in it.
// Pin changes are recorded and answered with the new list, or with pinErr.
type pins struct {
	log      *remoteCallLog
	messages []api.Message
	pinned   []api.Message
	pinErr   error
}

func pinsApp(t *testing.T, role string, pinned ...int64) (AppModel, *pins) {
	t.Helper()
	m, people := peopleApp(t)
	state := &pins{log: people.log}
	thread := m.remoteCalls.thread
	m.remoteCalls.thread = func(ctx context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		data, err := thread(ctx, target, q)
		if target.Org == "" || q.After > 0 {
			return data, err
		}
		data.Role, state.messages = role, data.Messages
		if state.pinned == nil {
			for _, id := range pinned {
				i := slices.IndexFunc(data.Messages, func(message api.Message) bool { return message.ID == id })
				state.pinned = append(state.pinned, data.Messages[i])
			}
		}
		data.Pinned = slices.Clone(state.pinned)
		if data.Pinned == nil {
			data.Pinned = []api.Message{}
		}
		return data, err
	}
	m.remoteCalls.setPinned = func(_ context.Context, id int64, pin bool) (api.MessagePins, error) {
		state.log.add("pin %d %t", id, pin)
		if state.pinErr != nil {
			return api.MessagePins{}, state.pinErr
		}
		if pin {
			messages := state.messages
			i := slices.IndexFunc(messages, func(message api.Message) bool { return message.ID == id })
			state.pinned = append([]api.Message{messages[i]}, state.pinned...)
		} else {
			state.pinned = slices.DeleteFunc(state.pinned, func(message api.Message) bool { return message.ID == id })
		}
		return api.MessagePins{Pinned: slices.Clone(state.pinned)}, nil
	}
	return settle(t, m, "j", "enter"), state
}

func TestThePinnedBannerShowsTheLatestPin(t *testing.T) {
	m, _ := pinsApp(t, "contributor", 9, 8)
	if text := threadText(m); !strings.Contains(text, "2 pinned · Grace Hopper: standup moved") {
		t.Fatalf("banner:\n%s", text)
	}
	cmd := m.fetchRemoteThread("poll")
	m = settleCmd(m, cmd)
	if len(m.remotes.thread.data.Pinned) != 2 {
		t.Fatalf("a poll without pins dropped them: %+v", m.remotes.thread.data.Pinned)
	}
	for _, size := range [][2]int{{160, 45}, {120, 30}, {80, 24}, {60, 18}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		narrow := next.(AppModel)
		pane := narrow.threadPaneWidth()
		lines := strings.Split(narrow.remoteThreadPane(pane, narrow.mainHeight()-remotesHeaderHeight, colors(true)), "\n")
		if !strings.Contains(ansi.Strip(lines[2]), "2 pinned") {
			t.Fatalf("at %dx%d the banner is not under the header: %q", size[0], size[1], ansi.Strip(lines[2]))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > pane {
				t.Fatalf("at %dx%d a line is %d cells, wider than the %d-cell pane: %q", size[0], size[1], w, pane, ansi.Strip(line))
			}
		}
	}

	one, _ := pinsApp(t, "contributor", 8)
	if text := threadText(one); !strings.Contains(text, "1 pinned · Fixture: morning all") {
		t.Fatalf("one pin:\n%s", text)
	}
}

func TestPinningWarnsUntilToldNotTo(t *testing.T) {
	m, state := pinsApp(t, "maintainer")
	if !strings.Contains(threadText(m), "! pin") {
		t.Fatalf("no pin hint for a maintainer:\n%s", threadText(m))
	}
	m = settle(t, m, "k", "!")
	if !strings.Contains(frameText(m), "Pin for the whole org?") || strings.Contains(state.log.String(), "pin 9") {
		t.Fatalf("no warning:\n%s", frameText(m))
	}
	m = settle(t, m, "space", "enter")
	if !strings.HasSuffix(state.log.String(), "pin 9 true") || !strings.Contains(threadText(m), "1 pinned · Grace Hopper") {
		t.Fatalf("pin: %s\n%s", state.log, threadText(m))
	}
	if !m.tuiSettings.PinWarningDismissed || !savedSettings(m, func(s auth.TUISettings) bool { return s.PinWarningDismissed }) {
		t.Fatal("Don't show this again was not saved")
	}
	m = settle(t, m, "k", "!")
	if m.remotes.pinWarning != nil || !strings.HasSuffix(state.log.String(), "pin 8 true") {
		t.Fatalf("warned again: %s", state.log)
	}
	m = settle(t, m, "!")
	if !strings.HasSuffix(state.log.String(), "pin 8 false") || !strings.Contains(threadText(m), "1 pinned · Grace Hopper") {
		t.Fatalf("unpin: %s", state.log)
	}
}

func TestOnlyMaintainersPinMessages(t *testing.T) {
	m, state := pinsApp(t, "contributor")
	m = settle(t, m, "k", "!")
	if strings.Contains(threadText(m), "! pin") || m.remotes.pinWarning != nil || strings.Contains(state.log.String(), "pin ") || m.settings.dialog != "" {
		t.Fatalf("a contributor pinned: %s\n%s", state.log, threadText(m))
	}
}

func TestThePinnedListJumpsAndUnpins(t *testing.T) {
	m, state := pinsApp(t, "maintainer", 9, 8)
	if clicked, _ := clickText(t, m, "2 pinned"); clicked.remotes.pinList == nil {
		t.Fatal("clicking the banner did not open the list")
	}
	m = settle(t, m, "L")
	if frame := frameText(m); m.remotes.pinList == nil || !strings.Contains(frame, "Pinned · 2") || !strings.Contains(frame, "morning all") {
		t.Fatalf("pinned list:\n%s", frame)
	}
	m = settle(t, m, "j", "enter")
	if m.remotes.pinList != nil || m.remotes.thread.selected != "m:8" {
		t.Fatalf("jump selected %q", m.remotes.thread.selected)
	}
	m = settle(t, m, "L", "!")
	if !strings.HasSuffix(state.log.String(), "pin 9 false") || !strings.Contains(frameText(m), "Pinned · 1") {
		t.Fatalf("unpin from the list: %s", state.log)
	}
	m = settle(t, m, "esc")
	m.tuiSettings.PinWarningDismissed = true
	state.pinErr = &auth.APIError{Status: http.StatusForbidden, Message: "An org can pin up to 25 messages. Unpin one first."}
	m = settle(t, m, "G", "!")
	if !strings.Contains(m.toast.title, "An org can pin up to 25 messages.") {
		t.Fatalf("refusal toast %q", m.toast.title)
	}
	state.pinErr = &auth.APIError{Status: http.StatusInternalServerError}
	m = settle(t, m, "!")
	if m.toast.title != "Could not pin. Try again." {
		t.Fatalf("failure toast %q", m.toast.title)
	}
}

func TestStatusNamesTheManagementKeys(t *testing.T) {
	m, _ := peopleApp(t)
	if status := m.remotesStatus(); !strings.Contains(status, "N new remote") || !strings.Contains(status, "O new org") {
		t.Fatalf("inbox status %q", status)
	}
	maintainer, _ := pinsApp(t, "maintainer")
	if status := maintainer.remotesStatus(); !strings.Contains(status, "I members") || !strings.Contains(status, "! pin") {
		t.Fatalf("maintainer status %q", status)
	}
	contributor, _ := pinsApp(t, "contributor")
	if status := contributor.remotesStatus(); !strings.Contains(status, "I members") || strings.Contains(status, "! pin") {
		t.Fatalf("contributor status %q", status)
	}
}

func TestJumpingToAPinLoadsOlderMessagesFirst(t *testing.T) {
	m, state := pinsApp(t, "contributor", 9, 8)
	// Only the newest message is loaded, and each older page brings one more.
	all := state.messages
	thread := m.remoteCalls.thread
	m.remoteCalls.thread = func(ctx context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		if q.Before == 0 {
			return thread(ctx, target, q)
		}
		state.log.add("older %d", q.Before)
		i := slices.IndexFunc(all, func(message api.Message) bool { return message.ID == q.Before })
		return api.RemoteThread{Messages: all[i-1 : i], HasOlder: i > 1, Access: "open"}, nil
	}
	m.remotes.thread.messages = all[len(all)-1:]
	m.remotes.thread.data.HasOlder = true
	m, cmd := settle(t, m, "L", "j").handleKey("enter")
	if m.remotes.pinList == nil || !strings.Contains(frameText(m), "Loading older messages to find it") {
		t.Fatalf("no feedback while loading:\n%s", frameText(m))
	}
	m = settleCmd(m, cmd)
	if m.remotes.pinList != nil || m.remotes.thread.selected != "m:8" || !strings.Contains(state.log.String(), "older") {
		t.Fatalf("jump selected %q after %s", m.remotes.thread.selected, state.log)
	}

	m.remotes.thread.messages, m.remotes.thread.data.HasOlder, m.remotes.thread.selected = all[len(all)-1:], true, ""
	m, cmd = settle(t, m, "L", "j").handleKey("enter")
	m = settleCmd(settle(t, m, "esc"), cmd)
	if m.remotes.pinList != nil || m.remotes.thread.selected != "" {
		t.Fatalf("esc did not cancel the jump: %q", m.remotes.thread.selected)
	}

	m.remotes.thread.data.Pinned = append(m.remotes.thread.data.Pinned, api.Message{ID: 1, Body: "gone"})
	m.remotes.thread.data.HasOlder = false
	m = settle(t, m, "L", "j", "j", "enter")
	if m.remotes.pinList == nil || !strings.Contains(frameText(m), "That message is no longer in this conversation.") {
		t.Fatalf("a missing pin:\n%s", frameText(m))
	}
}
