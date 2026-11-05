package tui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// newestApp is a loaded Following log whose server has the given commits above "a" and "b".
func newestApp(t *testing.T, arrivals ...api.Commit) (AppModel, *[]string) {
	t.Helper()
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = applyPage(next.(AppModel), 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}, Next: "older"})
	calls := &[]string{}
	m.fetch = func(_ context.Context, _ api.Log, next, since string) (api.Page, error) {
		if next != "" {
			return api.Page{}, nil
		}
		if since != "" {
			*calls = append(*calls, "count since "+since)
			return api.Page{NewCount: len(arrivals)}, nil
		}
		*calls = append(*calls, "newest")
		return api.Page{Data: append(append([]api.Commit(nil), arrivals...), fixtureCommit("a"), fixtureCommit("b")), Next: "older"}, nil
	}
	return m, calls
}

func authored(id, handle string) api.Commit {
	c := fixtureCommit(id)
	c.Author = &api.Author{ID: "id-" + handle, Name: handle, Handle: handle, Lane: []byte("2")}
	return c
}

func TestPollingRunsEvery15SecondsAndEvery30WhileTheTerminalIsAway(t *testing.T) {
	m := fixtureApp(t)
	if m.pollDelay(logPoll) != 15*time.Second || m.pollDelay(badgePoll) != 15*time.Second {
		t.Fatalf("focused polls every %v and %v", m.pollDelay(logPoll), m.pollDelay(badgePoll))
	}
	m.focused = false
	if m.pollDelay(logPoll) != 30*time.Second || m.pollDelay(badgePoll) != 30*time.Second || m.pollDelay(remotesSlowPoll) != 30*time.Second {
		t.Fatal("an unfocused terminal polls every 30 seconds")
	}
}

func TestTheLogAndBadgesStillPollWhileTheTerminalIsAway(t *testing.T) {
	m, calls := newestApp(t)
	counts := 0
	m.fetchNotificationCount = func(context.Context) (int, error) { counts++; return 2, nil }
	m.focused = false
	m = settleCmd(m, m.acceptLogTick(logTick{m.session, m.logTicks}))
	m = settleCmd(m, m.acceptBadgeTick(badgeTick{m.session, m.badgeTicks}))
	if strings.Join(*calls, "|") != "count since a" || counts != 1 || m.unreadNotifications != 2 {
		t.Fatalf("away polls: %v, %d counts", *calls, counts)
	}
}

func TestFocusRestartsThePollsSoAnAwayTickIsDropped(t *testing.T) {
	m, calls := newestApp(t)
	m.focused = false
	m.scheduleLogCheck()
	m.scheduleBadges()
	log, badge := logTick{m.session, m.logTicks}, badgeTick{m.session, m.badgeTicks}
	next, cmd := m.update(tea.FocusMsg{})
	m = next.(AppModel)
	if cmd == nil || m.acceptLogTick(log) != nil || m.acceptBadgeTick(badge) != nil || len(*calls) != 0 {
		t.Fatalf("an away tick ran after focus: %v", *calls)
	}
}

func TestNewCommitsGoAboveTheReaderWithANotice(t *testing.T) {
	own := authored("mine", "fixture")
	own.Author.ID = "fixture"
	m, calls := newestApp(t, authored("n1", "linus"), own, authored("n2", "grace"))
	// A short window keeps the viewport off its scroll limit, so the row's screen position can't be clamped.
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 18})
	m, more := resized.(AppModel).selectCommit(1)
	m = settleCmd(m, more)
	screenRow := func(f feed) int { return f.starts[f.selected] - f.viewport.YOffset() }
	before := screenRow(m.feeds[0])
	m = settleCmd(m, m.acceptLogTick(logTick{m.session, m.logTicks}))
	f := m.feeds[0]
	if strings.Join(*calls, "|") != "count since a|newest" || len(f.commits) != 5 || f.commits[f.selected].ID != "b" || screenRow(f) != before {
		t.Fatalf("arrivals: %v, %d commits, row %q on screen row %d, was %d", *calls, len(f.commits), f.commits[f.selected].ID, screenRow(f), before)
	}
	if len(f.arrivals) != 2 || f.arrivals[0].ID != "n1" || f.arrivals[1].ID != "n2" {
		t.Fatalf("your own commit is no news: %+v", f.arrivals)
	}
	frame := frameText(m)
	if !strings.Contains(frame, "↑") || !strings.Contains(frame, "New commits") || strings.Contains(frame, "new commits available") {
		t.Fatalf("the notice is missing:\n%s", frame)
	}
	// Without terminal images each author is a lane dot, and every cell keeps the pill's fill.
	pill, _ := m.arrivalsPill(140, colors(m.account.dark))
	if unfilled := regexp.MustCompile(`\x1b\[0?m[^\x1b\n]`); unfilled.MatchString(pill) {
		t.Fatalf("a pill cell lost its fill: %q", pill)
	}
	// The pill is three rows tall, so its top padding is part of the button too.
	for y, line := range strings.Split(frameText(m), "\n") {
		if pos := strings.Index(line, "New commits"); pos >= 0 {
			next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: ansi.StringWidth(line[:pos]) + 1, Y: y - 1})
			m = next.(AppModel)
			break
		}
	}
	if f := m.feeds[0]; f.selected != 0 || f.viewport.YOffset() != 0 || len(f.arrivals) != 0 || strings.Contains(frameText(m), "New commits") {
		t.Fatalf("the notice goes to the top and leaves: row %d offset %d", f.selected, f.viewport.YOffset())
	}
}

func TestNewCommitsAtTheTopShowWithoutANotice(t *testing.T) {
	m, _ := newestApp(t, authored("n1", "linus"))
	m = settleCmd(m, m.acceptLogTick(logTick{m.session, m.logTicks}))
	if f := m.feeds[0]; f.commits[f.selected].ID != "n1" || len(f.arrivals) != 0 || strings.Contains(frameText(m), "New commits") {
		t.Fatalf("a reader at the top sees the new commit: row %q, %d arrivals", f.commits[f.selected].ID, len(f.arrivals))
	}
}

func TestNewCommitsWhileAwayKeepTheRowUntilTheReaderGoesUp(t *testing.T) {
	m, _ := newestApp(t, authored("n1", "linus"))
	m.focused = false
	m = settleCmd(m, m.acceptLogTick(logTick{m.session, m.logTicks}))
	if len(m.feeds[0].arrivals) != 1 {
		t.Fatal("commits that arrive while away get the notice")
	}
	next, _ := m.Update(tea.FocusMsg{})
	// The reader's row stays put while away, so the notice waits for them to go back up.
	if m = next.(AppModel); len(m.feeds[0].arrivals) != 1 || m.feeds[0].commits[m.feeds[0].selected].ID != "a" {
		t.Fatal("coming back keeps the reader's row and the notice")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m = next.(AppModel); len(m.feeds[0].arrivals) != 0 {
		t.Fatal("the notice leaves once the newest commit is on screen")
	}
}

func TestRemotesKeepPollingWhileTheTerminalIsAway(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r")
	m.focused = false
	m.now = m.now.Add(time.Minute)
	if cmd := m.acceptRemotesTick(remotesTick{m.session, m.remotes.tickGeneration}); cmd == nil || !m.remotes.loading {
		t.Fatal("an unfocused Remotes tick refreshes the inbox")
	}
}

func TestACheckReadsBackToTheShownCommitsAndKeepsTheRow(t *testing.T) {
	m, _ := newestApp(t)
	m, more := m.selectCommit(1)
	m = settleCmd(m, more)
	var pages []string
	m.fetch = func(_ context.Context, _ api.Log, next, since string) (api.Page, error) {
		pages = append(pages, next)
		switch next {
		case "":
			return api.Page{Data: []api.Commit{authored("n1", "linus"), authored("n2", "grace")}, Next: "p2"}, nil
		case "p2":
			return api.Page{Data: []api.Commit{authored("n3", "linus"), fixtureCommit("a")}, Next: "p3"}, nil
		}
		return api.Page{}, nil
	}
	m = settleCmd(m, m.load("new"))
	f := m.feeds[0]
	if strings.Join(pages, "|") != "|p2" || len(f.commits) != 5 || f.commits[f.selected].ID != "b" || len(f.arrivals) != 3 {
		t.Fatalf("pages %v, %d commits, row %q, %d arrivals", pages, len(f.commits), f.commits[f.selected].ID, len(f.arrivals))
	}
	// With nothing shown within reach, the newest commits still go above and the reader keeps their place.
	pages = nil
	m.fetch = func(_ context.Context, _ api.Log, next, since string) (api.Page, error) {
		pages = append(pages, next)
		return api.Page{Data: []api.Commit{authored("x"+next, "grace")}, Next: next + "x"}, nil
	}
	m = settleCmd(m, m.load("new"))
	if f := m.feeds[0]; len(pages) != newestPages || len(f.commits) != 8 || f.commits[f.selected].ID != "b" || len(f.arrivals) != 6 {
		t.Fatalf("pages %v, %d commits, row %q, %d arrivals", pages, len(f.commits), f.commits[f.selected].ID, len(f.arrivals))
	}
}

func TestDeletingTheLastCommitDuringACheckDoesNotCrash(t *testing.T) {
	m, _ := newestApp(t, authored("n1", "linus"))
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	cmd := m.load("new")
	next, _ := m.Update(deleteResult{session: m.session, id: "a"})
	m = settleCmd(next.(AppModel), cmd)
	if f := m.feeds[0]; len(f.commits) != 3 || f.commits[0].ID != "n1" || f.selected != 0 {
		t.Fatalf("an emptied log takes the newest page: %d commits", len(f.commits))
	}
}

func TestAnEmptyLogGetsItsFirstCommitsFromTheCheck(t *testing.T) {
	m, calls := newestApp(t, authored("n1", "linus"))
	m = applyPage(m, 0, api.Page{Data: []api.Commit{}})
	m = settleCmd(m, m.acceptLogTick(logTick{m.session, m.logTicks}))
	if f := m.feeds[0]; strings.Join(*calls, "|") != "newest" || len(f.commits) != 3 || len(f.arrivals) != 0 {
		t.Fatalf("calls %v, %d commits, %d arrivals", *calls, len(f.commits), len(f.arrivals))
	}
}

func TestSwitchingLogsDropsAPendingCheck(t *testing.T) {
	m, _ := newestApp(t, authored("n1", "linus"))
	cmd := m.load("new")
	m, _ = m.switchLog(1)
	m, _ = m.switchLog(0)
	m = settleCmd(m, cmd)
	if len(m.feeds[0].commits) != 2 {
		t.Fatal("a check from before the switch placed its commits")
	}
}

func TestReadingOlderCommitsCancelsAQuietCheck(t *testing.T) {
	m, _ := newestApp(t, authored("n1", "linus"))
	cmd := m.load("count")
	m.feeds[0].selected = 1
	if more := m.load("more"); more == nil || m.feeds[0].loadingMode != "more" {
		t.Fatal("the reader's older page waited behind a quiet check")
	}
	if m = settleCmd(m, cmd); m.feeds[0].loadingMode != "more" || !m.feeds[0].loading {
		t.Fatal("the canceled check's result was accepted")
	}
	if m.load("count") != nil {
		t.Fatal("a quiet check interrupted the reader's load")
	}
}

func TestQuickFocusChangesKeepTheFocusedTicks(t *testing.T) {
	m, calls := newestApp(t, authored("n1", "linus"))
	log, badge := logTick{m.session, m.logTicks}, badgeTick{m.session, m.badgeTicks}
	next, _ := m.update(tea.BlurMsg{})
	next, _ = next.(AppModel).update(tea.FocusMsg{})
	m = next.(AppModel)
	if m.acceptBadgeTick(badge) == nil || m.acceptLogTick(log) == nil || m.feeds[0].loadingMode != "count" {
		t.Fatalf("a focus change dropped the focused ticks: %v", *calls)
	}
}

func TestANoticeOutlivesItsNewestArrivalLeaving(t *testing.T) {
	m, _ := newestApp(t, authored("n1", "linus"), authored("n2", "grace"))
	m, more := m.selectCommit(1)
	m = settleCmd(m, more)
	m = settleCmd(m, m.acceptLogTick(logTick{m.session, m.logTicks}))
	next, _ := m.Update(deleteResult{session: m.session, id: "n1"})
	if f := next.(AppModel).feeds[0]; len(f.arrivals) != 1 || f.arrivals[0].ID != "n2" {
		t.Fatalf("the other arrival lost its notice: %+v", f.arrivals)
	}
}

func TestCtrlRRefreshesTheLogAndBothBadges(t *testing.T) {
	m, log := remotesApp(t, auth.RemotePreferences{UnreadCount: 7})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = next.(AppModel)
	counts := 0
	m.fetchNotificationCount = func(context.Context) (int, error) { counts++; return 3, nil }
	m, cmd := m.handleKey("ctrl+r")
	if m.toast.title != "Refreshing" || m.toast.detail != "Checking Following, your notifications, and Remotes." {
		t.Fatalf("toast %q: %q", m.toast.title, m.toast.detail)
	}
	m = settleCmd(m, cmd)
	if counts != 1 || m.unreadNotifications != 3 || !strings.Contains(log.String(), "inbox open") || m.toast.title != "Refreshed" || m.toast.detail != "Following is up to date." {
		t.Fatalf("badges: %d counts, %s, toast %q: %q", counts, log, m.toast.title, m.toast.detail)
	}
	// A failed badge keeps its count quietly, as a poll does, so success names only the log it confirmed.
	m.fetchNotificationCount = func(context.Context) (int, error) { return 0, errors.New("offline") }
	m, cmd = m.handleKey("ctrl+r")
	m = settleCmd(m, cmd)
	if m.unreadNotifications != 3 || m.toast.detail != "Following is up to date." {
		t.Fatalf("a failed badge: count %d, toast %q", m.unreadNotifications, m.toast.detail)
	}
}
