package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// requestApp opens @ada's thread as a fetched request, with merge and drop recorded.
func requestApp(t *testing.T, answer error) (AppModel, *messaging) {
	t.Helper()
	m, state := messagingApp(t)
	state.access = "request"
	m.remoteCalls.merge = func(_ context.Context, handle string) error {
		state.log.add("merge %s", handle)
		return answer
	}
	m.remoteCalls.drop = func(_ context.Context, handle string) error {
		state.log.add("drop %s", handle)
		return answer
	}
	return openDirect(t, m), state
}

func TestMergingARequestOpensTheComposer(t *testing.T) {
	m, state := requestApp(t, nil)
	if text := threadText(m); !strings.Contains(text, "@ada wants to push to you. Merge to reply, or drop it.") || !strings.Contains(text, " Drop  d     Merge  m ") {
		t.Fatalf("request bar:\n%s", text)
	}
	m = settle(t, m, "m")
	if !strings.Contains(state.log.String(), "merge ada") || m.remotes.thread.data.Access != "open" {
		t.Fatalf("merge: %s, access %q", state.log, m.remotes.thread.data.Access)
	}
	if m = send(m, typed("w")...); !m.remotes.thread.composer.focused {
		t.Fatal("the composer did not open after merging")
	}
}

func TestDroppingARequestAsksFirstAndClosesTheConversation(t *testing.T) {
	m, state := requestApp(t, nil)
	m = settle(t, m, "d")
	if m.remotes.dialog != "drop" || m.remotes.dialogFocus != 0 || !strings.Contains(frameText(m), "Drop @ada's request?") {
		t.Fatalf("drop dialog %q focus %d:\n%s", m.remotes.dialog, m.remotes.dialogFocus, frameText(m))
	}
	m = settle(t, m, "enter", "d", "esc")
	if strings.Contains(state.log.String(), "drop") || m.remotes.thread == nil {
		t.Fatalf("drop sent without confirming: %s", state.log)
	}
	m = settle(t, m, "d", "right", "enter")
	if !strings.Contains(state.log.String(), "drop ada") || m.remotes.thread != nil || slices.ContainsFunc(m.remotes.inbox, func(c api.Conversation) bool { return c.ID == "d1" }) {
		t.Fatalf("drop: %s, inbox %d", state.log, len(m.remotes.inbox))
	}
}

func TestAFailedAnswerKeepsTheRequest(t *testing.T) {
	m, _ := requestApp(t, errors.New("down"))
	m = settle(t, m, "m")
	if m.toast.title != "Could not update the request. Try again." || m.remotes.thread.data.Access != "request" {
		t.Fatalf("toast %q access %q", m.toast.title, m.remotes.thread.data.Access)
	}
	m = settle(t, m, "d", "right", "enter")
	if m.toast.title != "Could not update the request. Try again." || m.remotes.thread == nil {
		t.Fatal("a failed drop closed the conversation")
	}
}
