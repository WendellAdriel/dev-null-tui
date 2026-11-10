package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// clickChip clicks a visible chip by its text and settles the result.
func clickChip(t *testing.T, m AppModel, label string) AppModel {
	t.Helper()
	next, cmd := clickText(t, m, label)
	return settleCmd(next, cmd)
}

func composerWithImage(t *testing.T) AppModel {
	t.Helper()
	m := composerApp(t)
	m.imageDir = attachDir(t)
	m = keys(typeText(keys(m, "ctrl+o"), "chart"), "enter")
	if len(m.composer.images) != 1 {
		t.Fatalf("setup attached %d images", len(m.composer.images))
	}
	for m.composer.focus != composerStrip {
		m = keys(m, "tab")
	}
	return m
}

func TestStripChipsFollowTheOpenField(t *testing.T) {
	m := keys(composerWithImage(t), "enter")
	m = typeText(m, "abc")
	if strings.Contains(frameText(m), "Remove  del") {
		t.Fatalf("the description field must not offer Remove:\n%s", frameText(m))
	}
	if m = clickChip(t, m, "Save  enter"); m.composer.editing != "" || m.composer.images[0].Description != "abc" {
		t.Fatalf("Save did not save the description: %q, %q", m.composer.editing, m.composer.images[0].Description)
	}
	m = typeText(keys(m, "ctrl+o"), "chart")
	if strings.Contains(frameText(m), "Describe  enter") {
		t.Fatalf("the strip must not offer chips while the browser owns the keys:\n%s", frameText(m))
	}
}

func TestMembersMenuHasItsOwnChips(t *testing.T) {
	m, state := membersApp(t, "maintainer")
	m = settle(t, m, "j", "enter")
	if frame := frameText(m); strings.Contains(frame, "Manage  enter") || !strings.Contains(frame, "Run  enter") {
		t.Fatalf("an open member menu must offer Run, not Manage:\n%s", frame)
	}
	if m = clickChip(t, m, "Back  esc"); m.remotes.members == nil || m.remotes.members.menu || strings.Contains(state.log.String(), "role") {
		t.Fatalf("Back must close only the menu and change nothing: %s", state.log)
	}
}

func TestInboxClearChipKeepsTheThread(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m.remotes.term = "fix"
	if m = clickChip(t, m, "Clear  esc"); m.remotes.term != "" || m.remotes.thread == nil {
		t.Fatalf("Clear must clear the search and keep the thread: %q, %v", m.remotes.term, m.remotes.thread != nil)
	}
}

func TestRemotesImageHeaderOffersEditOnlyWhileWriting(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	m.remotes.thread.composer.images = []api.ImageFile{{Path: "/tmp/x.png", Width: 10, Height: 10}}
	m.resizeThread()
	if strings.Contains(threadText(m), "Edit images  tab") {
		t.Fatalf("an unfocused composer must not offer Edit images:\n%s", threadText(m))
	}
}

func TestRemotesImageChipsChooseBothWays(t *testing.T) {
	m, _ := messagingApp(t)
	m = settle(t, openOrg(t, m), "w")
	mc := &m.remotes.thread.composer
	mc.images = []api.ImageFile{{Path: "/tmp/a.png", Width: 10, Height: 10}, {Path: "/tmp/b.png", Width: 10, Height: 10}}
	mc.stripFocus = true
	m.resizeThread()
	if m = clickChip(t, m, "Next  →"); m.remotes.thread.composer.strip != 1 {
		t.Fatalf("Next did not choose the next image: %d", m.remotes.thread.composer.strip)
	}
	if m = clickChip(t, m, "Previous  ←"); m.remotes.thread.composer.strip != 0 {
		t.Fatalf("Previous did not go back: %d", m.remotes.thread.composer.strip)
	}
}

func TestPeoplePickShowsOnlyOnTheList(t *testing.T) {
	m, _ := peopleApp(t)
	m = settle(t, m, "O")
	if strings.Contains(frameText(m), "Pick  space") {
		t.Fatalf("the name field must not offer Pick:\n%s", frameText(m))
	}
}

func TestBrowserUpHidesWhileFiltering(t *testing.T) {
	m := composerApp(t)
	m.imageDir = attachDir(t)
	m = typeText(keys(m, "ctrl+o"), "ch")
	if strings.Contains(frameText(m), "Up  backspace") {
		t.Fatalf("a typed filter must hide Up:\n%s", frameText(m))
	}
}

func TestScheduleChipsChangeBothWays(t *testing.T) {
	m := composerApp(t)
	m.commitCalls.schedule = func(context.Context, api.NewCommit, time.Time) (api.ScheduledCommit, error) {
		return api.ScheduledCommit{}, nil
	}
	m = keys(m, "ctrl+t")
	m.composer.schedule.field = 1
	hour := m.composer.schedule.hour
	if m = clickChip(t, m, "Earlier  ↓"); m.composer.schedule.hour != max(0, hour-1) {
		t.Fatalf("Earlier did not lower the hour: %d from %d", m.composer.schedule.hour, hour)
	}
	if m = clickChip(t, m, "Later  ↑"); m.composer.schedule.hour != max(0, hour-1)+1 {
		t.Fatalf("Later did not raise the hour: %d", m.composer.schedule.hour)
	}
}

func TestPopupActionChipsAct(t *testing.T) {
	t.Run("help scroll", func(t *testing.T) {
		if m := clickChip(t, pressApp(fixtureApp(t), "?"), "Scroll  ↑/↓"); m.helpScroll != 1 {
			t.Fatalf("Scroll did not scroll help: %d", m.helpScroll)
		}
	})
	t.Run("achievements", func(t *testing.T) {
		m := settle(t, achievementsApp(t), "A")
		if m = clickChip(t, m, "Choose  ←/→"); m.achievements.selected != 1 {
			t.Fatalf("Choose did not move: %d", m.achievements.selected)
		}
	})
	t.Run("pinned jump", func(t *testing.T) {
		m, _ := pinsApp(t, "maintainer", 8)
		m = settle(t, m, "L")
		if m = clickChip(t, m, "Jump  enter"); m.remotes.pinList != nil {
			t.Fatal("Jump did not close the list and jump")
		}
	})
}
