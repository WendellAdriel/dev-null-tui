package tui

import (
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestShiftEListsWhoReactedToTheSelectedMessage(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "down", "enter")
	if m.remotes.thread == nil || m.remotes.thread.data.Org == nil {
		t.Fatal("the fixture should open the org thread")
	}
	message := remoteMessage(9, "grace", "standup moved", m.now)
	message.Reactions = []api.Reaction{{UserID: "grace", Emoji: "😂"}, {UserID: "fixture", Emoji: "✅"}, {UserID: "linus", Emoji: "😂"}, {UserID: "gone", Emoji: "😂"}}
	m.remotes.thread.messages = []api.Message{message}
	m.renderRemoteThread()
	m = settle(t, m, "E")
	if m.picker == nil || m.picker.kind != "reactions" {
		t.Fatalf("E did not open who reacted:\n%s", frameText(m))
	}
	frame := frameText(m)
	for _, want := range []string{"😂", ":joy:", "Grace Hopper", "Linus Torvalds", "Someone who left", "✅", "Fixture"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("who reacted is missing %q:\n%s", want, frame)
		}
	}
	if strings.Index(frame, "Linus Torvalds") > strings.Index(frame, "✅") {
		t.Fatalf("people are not listed under their emoji:\n%s", frame)
	}
	if m = pressApp(m, "esc"); m.picker != nil || m.remotes.thread == nil {
		t.Fatal("esc should close who reacted and keep the thread")
	}
	message.Reactions = nil
	m.remotes.thread.messages = []api.Message{message}
	m.renderRemoteThread()
	if m = settle(t, m, "E"); m.picker != nil || m.notice != "No reactions yet." {
		t.Fatalf("E on a message without reactions: %v %q", m.picker != nil, m.notice)
	}
}

func TestTheReactionsChipShowsOnlyForAMessageWithReactions(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "down", "enter")
	message := remoteMessage(9, "grace", "standup moved", m.now)
	m.remotes.thread.messages = []api.Message{message}
	m.renderRemoteThread()
	if strings.Contains(frameText(m), "Reactions  E") {
		t.Fatal("the Reactions chip shows for a message nobody reacted to")
	}
	message.Reactions = []api.Reaction{{UserID: "grace", Emoji: "😂"}}
	m.remotes.thread.messages = []api.Message{message}
	m.renderRemoteThread()
	next, cmd := clickText(t, m, "Reactions  E")
	if m = settleCmd(next, cmd); m.picker == nil || m.picker.kind != "reactions" {
		t.Fatalf("clicking the Reactions chip did not open who reacted:\n%s", frameText(m))
	}
}
