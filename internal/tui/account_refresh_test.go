package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func countedUser(m AppModel, commits, followers int) auth.User {
	u := m.account.user
	u.Commits, u.Followers = &commits, &followers
	return u
}

func TestCtrlRAndNewCommitsRefreshTheAccountAndLiveNow(t *testing.T) {
	m, log := liveApp(t)
	calls := 0
	m.meCall = func(context.Context) (auth.User, error) {
		calls++
		return countedUser(m, 10+calls, 3), nil
	}
	m, cmd := m.handleKey("ctrl+r")
	m = deliver(m, runAll(cmd))
	if calls != 1 || m.account.user.Commits == nil || *m.account.user.Commits != 11 {
		t.Fatalf("Ctrl+R made %d account calls, commits %v", calls, m.account.user.Commits)
	}
	if !strings.Contains(strings.Join(log.calls, ","), "following") {
		t.Fatalf("Ctrl+R did not refresh Live now: %v", log.calls)
	}
	if frame := ansi.Strip(m.View().Content); !strings.Contains(frame, "11 commits") {
		t.Fatalf("the aside kept the old count:\n%s", frame)
	}

	next, cmd := m.update(logResult{index: 0, generation: m.feeds[0].generation, mode: "new", page: api.Page{Data: []api.Commit{fixtureCommit("fresh")}}})
	m = deliver(next.(AppModel), runAll(cmd))
	if calls != 2 || *m.account.user.Commits != 12 {
		t.Fatalf("new commits made %d account calls", calls)
	}
}

func TestTheClockRefreshesTheAccountOnceAtATime(t *testing.T) {
	m, _ := liveApp(t)
	calls := 0
	m.meCall = func(context.Context) (auth.User, error) {
		calls++
		u := countedUser(m, 1, 1)
		u.Live = stream(m, "twitch", time.Hour)
		return u, nil
	}
	next, cmd := m.update(clockTick{session: m.session, at: m.now})
	m = next.(AppModel)
	if again := m.refreshAccount(); again != nil {
		t.Fatal("a second account refresh started while one was running")
	}
	m = deliver(m, runSkippingTicks(cmd))
	if calls != 1 || !m.account.user.Live.On(m.now) {
		t.Fatalf("the clock made %d calls, live %v", calls, m.account.user.Live)
	}
	if m.refreshAccount() == nil {
		t.Fatal("the finished refresh kept blocking the next one")
	}
}

func TestAStaleAccountRefreshKeepsANewerChange(t *testing.T) {
	m, _ := liveApp(t)
	m.meCall = func(context.Context) (auth.User, error) { return countedUser(m, 99, 99), nil }
	cmd := m.refreshAccount()
	changed := countedUser(m, 5, 5)
	m.applyAccount(changed)
	m = deliver(m, runAll(cmd))
	if *m.account.user.Commits != 5 {
		t.Fatal("an account refresh started before a change overwrote it")
	}
}

// runSkippingTicks runs a command like runAll but drops the commands still waiting after a moment, such as the
// one-minute clock.
func runSkippingTicks(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var msgs []tea.Msg
			for _, command := range batch {
				msgs = append(msgs, runSkippingTicks(command)...)
			}
			return msgs
		}
		return []tea.Msg{msg}
	case <-time.After(time.Second):
		return nil
	}
}

func TestAnAccountRefreshKeepsTheBadgeAnInboxReadCleared(t *testing.T) {
	m, _ := liveApp(t)
	five := 5
	m.meCall = func(context.Context) (auth.User, error) {
		u := m.account.user
		u.UnreadNotifications = &five
		return u, nil
	}
	cmd := m.refreshAccount()
	m, _ = m.openNotifications()
	m = inboxPage(m, fixtureNotification("n"))
	m = deliver(m, runAll(cmd))
	if m.unreadNotifications != 0 {
		t.Fatalf("a refresh from before the inbox read brought back %d unread", m.unreadNotifications)
	}
}
