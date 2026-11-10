package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func chipFor(parts []statusPart, key string) (statusPart, bool) {
	for _, part := range parts {
		if part.label != "" && part.key == key {
			return part, true
		}
	}
	return statusPart{}, false
}

func clickFooter(t *testing.T, m AppModel, action string) AppModel {
	t.Helper()
	_, areas := m.workspaceFooter(m.account.width, m.account.colors())
	for _, area := range areas {
		if area.action == action {
			next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: area.x, Y: m.account.height - 1})
			return settleCmd(next.(AppModel), cmd)
		}
	}
	t.Fatalf("no footer chip runs %q", action)
	return m
}

func TestAsideActionsAndShortcutsAreFullWidthChips(t *testing.T) {
	m := fixtureApp(t)
	p := colors(true)
	aside, areas := m.asideLayout(40, 34, p)
	lines := strings.Split(aside, "\n")
	labels := map[string]string{"a": "Account", "ctrl+r": "Refresh", "f2": "f2", "?": "?"}
	for _, area := range areas {
		label, ok := labels[area.action]
		if !ok {
			continue
		}
		row := ansi.Strip(ansi.Cut(lines[area.y], area.x, area.x+area.w))
		if area.w != 37 || !strings.HasSuffix(row, " "+area.action+" ") || !strings.Contains(row, label) {
			t.Fatalf("%s chip is %q, %d wide", area.action, row, area.w)
		}
		if !strings.Contains(lines[area.y], "48;2;") {
			t.Fatalf("%s chip has no background", area.action)
		}
	}
	for _, line := range lines {
		plain := ansi.Strip(line)
		if strings.TrimSpace(plain) != "Shortcuts" {
			continue
		}
		inner := strings.TrimRight(plain[2:], " ")
		if left := len(inner) - len(strings.TrimLeft(inner, " ")); left < 10 {
			t.Fatalf("Shortcuts heading is not centered: %q", plain)
		}
		return
	}
	t.Fatal("Shortcuts heading missing")
}

func TestRemotesThreadStatusBarIsChipsWithPing(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	parts := m.remotesStatus()
	if !strings.HasSuffix(parts[0].text, "messages") {
		t.Fatalf("the count must stay plain text: %+v", parts[0])
	}
	for key, label := range map[string]string{"j/k": "Select", "I": "Members", "p": "Ping everyone in the org", "esc": "Back"} {
		if chip, ok := chipFor(parts, key); !ok || chip.label != label {
			t.Fatalf("status is missing the %s chip: %+v", label, parts)
		}
	}
	for _, moved := range []string{"w", ".", "!"} {
		if _, ok := chipFor(parts, moved); ok {
			t.Fatalf("%q still sits in the status bar", moved)
		}
	}
	row := threadText(m)
	if !strings.Contains(row, "Write") || !strings.Contains(row, "Actions") || strings.Contains(row, "Ping everyone") {
		t.Fatalf("composer row must hold Write and Actions instead of Ping:\n%s", row)
	}
	if m = clickFooter(t, m, "I"); m.remotes.members == nil {
		t.Fatal("the Members chip did not open Members")
	}
}

func TestRemotesInboxStatusBarIsChips(t *testing.T) {
	m, _ := peopleApp(t)
	parts := m.remotesStatus()
	if parts[0].text != "1 / 2" {
		t.Fatalf("position must stay plain text: %+v", parts[0])
	}
	for _, key := range []string{"enter", "tab", "/", ".", "N", "O", "esc"} {
		if _, ok := chipFor(parts, key); !ok {
			t.Fatalf("inbox status is missing %q: %+v", key, parts)
		}
	}
	line, _ := statusLine(parts, 40, colors(true))
	if plain := ansi.Strip(line); !strings.Contains(plain, "1 / 2") || !strings.Contains(plain, "Back  esc") || strings.Contains(plain, "New org") {
		t.Fatalf("a narrow bar must drop chips from the end but keep Back: %q", plain)
	}
	if m = clickFooter(t, m, "N"); m.remotes.people == nil {
		t.Fatal("the New remote chip did not open its dialog")
	}
}

func TestGMenuListsDestinationsAsChips(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 35
	m, _ = m.handleKey("g")
	parts := m.statusParts()
	if parts[0].text != "g ..." || parts[len(parts)-1].text != "(1 second)" {
		t.Fatalf("the prefix and the note stay plain: %+v", parts)
	}
	for key, label := range map[string]string{"l": "Log", "e": "Trending", "n": "Notifications", "p": "Profile", "s": "Starred", "t": "Scheduled", "d": "Drafts"} {
		if chip, ok := chipFor(parts, key); !ok || chip.label != label {
			t.Fatalf("g menu is missing %s: %+v", label, parts)
		}
	}
	line, _ := statusLine(parts, 60, colors(true))
	if plain := ansi.Strip(line); strings.Contains(plain, "(1 second)") || !strings.Contains(plain, "g ...") || !strings.Contains(plain, "Log") {
		t.Fatalf("a narrow g menu must drop the note first: %q", plain)
	}
	if m = clickFooter(t, m, "s"); !m.starredOpen {
		t.Fatal("the Starred chip did not open Starred")
	}
}

func TestNotificationFiltersAreActionChips(t *testing.T) {
	m := inboxPage(inboxFixture(t), fixtureNotification("a"))
	m.notificationMask &^= 1 << 1
	p := colors(true)
	header, _ := m.notificationHeader(200, p)
	if !strings.Contains(header, actionChip("Replies", "1", p)) || !strings.Contains(header, chipStyled("Quotes", "2", p.muted, p)) {
		t.Fatalf("filters must be action chips with a muted label when off:\n%s", ansi.Strip(header))
	}
}

func TestRemotesRowAndStatusChipsAcceptClicks(t *testing.T) {
	m, state := messagingApp(t)
	m = openOrg(t, m)
	if m = clickFooter(t, m, "p"); !strings.Contains(state.log.String(), "ping "+fixtureOrg) {
		t.Fatalf("the Ping chip did not ping: %s", state.log)
	}
	next, cmd := clickText(t, m, "Actions  .")
	if m = settleCmd(next, cmd); m.remotes.dialog != "menu" {
		t.Fatalf("the Actions chip opened %q", m.remotes.dialog)
	}
	m = settle(t, m, "esc")
	next, cmd = clickText(t, m, "Write  w")
	if m = settleCmd(next, cmd); !m.remotes.thread.composer.focused {
		t.Fatal("the Write chip did not focus the composer")
	}
}

func TestSettingsOverRemotesKeepsRemotesChipsOutOfTheFooter(t *testing.T) {
	m, _ := peopleApp(t)
	m, _ = m.handleKey("g")
	m = settle(t, m, ",")
	if !m.settings.open {
		t.Fatal("g , did not open Settings")
	}
	if _, areas := m.workspaceFooter(m.account.width, m.account.colors()); len(areas) > 0 {
		t.Fatalf("Settings shows Remotes chips: %+v", areas)
	}
}

func TestVoiceNoteStatusBarIsChips(t *testing.T) {
	m, _ := voiceApp(t)
	parts := m.remotesStatus()
	if parts[0].text != "Voice note" {
		t.Fatalf("the voice note label stays plain: %+v", parts)
	}
	for key, label := range map[string]string{"space": "Play", "-": "Quieter", "+": "Louder", "esc": "Back"} {
		if chip, ok := chipFor(parts, key); !ok || chip.label != label {
			t.Fatalf("an idle note is missing %s: %+v", label, parts)
		}
	}
	if _, ok := chipFor(parts, "x"); ok {
		t.Fatal("speed only shows once the note is loaded")
	}
	if m = clickFooter(t, m, "space"); m.voice.playback.state != "playing" {
		t.Fatalf("the Play chip did not play: %q", m.voice.playback.state)
	}
	parts = m.remotesStatus()
	for key, label := range map[string]string{"space": "Pause", "←": "Rewind", "→": "Forward", "x": "Speed"} {
		if chip, ok := chipFor(parts, key); !ok || chip.label != label {
			t.Fatalf("a playing note is missing %s: %+v", label, parts)
		}
	}
	if m = clickFooter(t, m, "x"); m.voice.playback.speed == 1 {
		t.Fatal("the Speed chip did not change the speed")
	}
}

func TestRemotesSearchStatusBarIsChips(t *testing.T) {
	m, _ := peopleApp(t)
	m = settle(t, m, "/")
	parts := m.remotesStatus()
	if parts[0].text != "Search by name or @handle" {
		t.Fatalf("the prompt stays plain: %+v", parts)
	}
	for key, label := range map[string]string{"enter": "Search", "esc": "Cancel"} {
		if chip, ok := chipFor(parts, key); !ok || chip.label != label {
			t.Fatalf("search is missing %s: %+v", label, parts)
		}
	}
	if m = clickFooter(t, m, "esc"); m.remotes.searching {
		t.Fatal("the Cancel chip did not cancel the search")
	}
}

func TestRemotesWritingStatusBarIsChips(t *testing.T) {
	m, _ := messagingApp(t)
	m = settle(t, openOrg(t, m), "w")
	parts := m.remotesStatus()
	if !strings.HasPrefix(parts[0].text, "Write to ") {
		t.Fatalf("the conversation stays plain text: %+v", parts)
	}
	for key, label := range map[string]string{"enter": "Push", "esc": "Messages"} {
		if chip, ok := chipFor(parts, key); !ok || chip.label != label {
			t.Fatalf("the writing status is missing %s: %+v", label, parts)
		}
	}
	if m = clickFooter(t, m, "esc"); m.remotes.thread.composer.focused {
		t.Fatal("the Messages chip did not leave the composer")
	}
}

func TestAsideActionsAndShortcutsSitOnConsecutiveRows(t *testing.T) {
	m := fixtureApp(t)
	_, areas := m.asideLayout(40, 60, colors(true))
	rows := map[string]int{}
	for _, area := range areas {
		rows[area.action] = area.y
	}
	if rows["L"]-rows["a"] != 1 || rows["?"]-rows["f2"] != 1 || rows["tab"]-rows["down"] != 1 {
		t.Fatalf("action and shortcut chips must sit on consecutive rows: %+v", rows)
	}
}
