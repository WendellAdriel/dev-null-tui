package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func moderationApp(t *testing.T) (AppModel, *int) {
	t.Helper()
	m, _ := profileApp(t)
	m.now = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	reads := new(int)
	m.loadModerationHistory = func(context.Context) (api.ModerationHistory, error) {
		*reads++
		started := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
		return api.ModerationHistory{
			TimeoutsCount: 2, BansCount: 0,
			Timeouts: []api.ModerationTimeout{
				{ID: "t2", Reason: "Spam", Duration: "1 hour", StartedAt: started, EndsAt: started.Add(time.Hour)},
				{ID: "t1", Reason: "Off-topic", Duration: "10 minutes", StartedAt: started.Add(-48 * time.Hour), EndsAt: started.Add(-48*time.Hour + 10*time.Minute)},
			},
		}, nil
	}
	return m, reads
}

func openOwnProfile(t *testing.T, m AppModel) AppModel {
	t.Helper()
	m, _ = m.handleKey("g")
	return press(t, m, "p")
}

func TestOwnProfileShowsItsModerationHistoryAndOthersDont(t *testing.T) {
	m, reads := moderationApp(t)
	m = press(t, m, "u")
	if *reads != 0 || strings.Contains(profileText(m), "timeouts") {
		t.Fatal("another person's profile read your moderation history")
	}
	m, reads = moderationApp(t)
	m = openOwnProfile(t, m)
	if *reads != 1 || !strings.Contains(profileText(m), m.icons.moderation+" 2 timeouts · 0 bans  M") {
		t.Fatalf("reads %d:\n%s", *reads, profileText(m))
	}
	press(t, m, "ctrl+r")
	if *reads != 2 {
		t.Fatalf("Ctrl+R reads %d", *reads)
	}
}

func TestModerationHistoryOpensFromMAndFromAClick(t *testing.T) {
	m, _ := moderationApp(t)
	m = openOwnProfile(t, m)
	opened := press(t, m, "M")
	d := opened.picker
	if d == nil || d.kind != "moderation" || d.intro != "This record of timeouts and bans is visible to you and site moderators." {
		t.Fatalf("picker: %+v", d)
	}
	var labels []string
	for _, row := range d.rows {
		labels = append(labels, row.label)
	}
	if got := strings.Join(labels, "|"); got != "Timeouts · 2|Spam|Off-topic||Bans · 0|No bans." {
		t.Fatalf("rows: %s", got)
	}
	if d.rows[d.selected].label != "Spam" || !strings.Contains(d.rows[d.selected].detail, "Ended "+historyTime(time.Date(2026, 11, 15, 11, 0, 0, 0, time.UTC))) {
		t.Fatalf("selected: %+v", d.rows[d.selected])
	}
	if still := press(t, opened, "enter"); still.picker == nil {
		t.Fatal("Enter closed the history")
	}
	s := m.topProfile()
	for _, area := range s.actions {
		if area.action != "M" {
			continue
		}
		clicked, _ := m.profileMouse(area.x, area.y-s.viewport.YOffset()+threadHeaderHeight)
		if clicked.picker == nil || clicked.picker.kind != "moderation" {
			t.Fatal("clicking the history line didn't open it")
		}
		return
	}
	t.Fatal("the history line isn't clickable")
}

func TestANewNotificationRereadsYourOpenModerationHistory(t *testing.T) {
	m, reads := moderationApp(t)
	m = openOwnProfile(t, m)
	next, cmd := m.Update(notificationCountResult{m.session, m.notificationCountGeneration, 1, nil})
	m = deliver(next.(AppModel), runAll(cmd))
	if *reads != 2 {
		t.Fatalf("a rising count read the history %d times", *reads)
	}
	next, cmd = m.Update(notificationCountResult{m.session, m.notificationCountGeneration, 1, nil})
	deliver(next.(AppModel), runAll(cmd))
	if *reads != 2 {
		t.Fatalf("an unchanged count read the history again: %d", *reads)
	}
}

func TestACleanModerationHistorySelectsNothing(t *testing.T) {
	m, _ := profileApp(t)
	m.loadModerationHistory = func(context.Context) (api.ModerationHistory, error) { return api.ModerationHistory{}, nil }
	m = press(t, openOwnProfile(t, m), "M")
	d := m.picker
	if d == nil || d.selected != len(d.rows) || !strings.Contains(frameText(m), "No timeouts.") {
		t.Fatalf("clean history: %+v", d)
	}
	if still := press(t, m, "down", "up", "enter"); still.picker == nil || still.picker.selected != len(d.rows) {
		t.Fatal("moving in a clean history selected a heading or closed it")
	}
}

func TestAnOlderHistoryReadCantReplaceANewerOne(t *testing.T) {
	m, _ := moderationApp(t)
	m = openOwnProfile(t, m)
	s := m.topProfile()
	stale := moderationResult{session: m.session, generation: s.generation, read: s.historyRead - 1, history: api.ModerationHistory{TimeoutsCount: 9}}
	m.acceptModerationHistory(stale)
	if m.topProfile().history.TimeoutsCount != 2 {
		t.Fatal("an older read replaced the history")
	}
}
