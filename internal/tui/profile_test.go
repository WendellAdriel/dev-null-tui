package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type profileCalls struct {
	profiles, commits, people, follows []string
}

func fixtureProfile(handle string) api.Profile {
	commits, followers, following := 3, 10, 2
	yes, no := true, false
	profile := api.Profile{
		User:         api.User{ID: "id-" + handle, Handle: handle, Name: "Name " + handle, Bio: "Bio of " + handle, Location: "Lisbon", Website: "https://" + handle + ".dev", Commits: &commits, Followers: &followers, Following: &following, Lane: []byte("2")},
		IsFollowing:  &no,
		FollowsYou:   &yes,
		Streaks:      &api.Streaks{Longest: 4, Current: 2},
		Achievements: &api.Achievements{Level: 5, Rank: "wheel", XP: 1821, NextLevelXP: new(2500), LevelProgress: 72, Ranks: []api.Rank{{Rank: "wheel", Level: 5, MinXP: 1500, Achieved: true}, {Rank: "root", Level: 6, MinXP: 2500}}, Badges: []api.Badge{{ID: "init", Name: "init", Unlocked: true}, {ID: "uptime", Name: "uptime"}}},
		Activity:     &api.Activity{Scale: "week"},
	}
	for day, count := range []int{0, 1, 3, 0, 2, 5, 1} {
		profile.Activity.Buckets = append(profile.Activity.Buckets, struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		}{fmt.Sprintf("2026-10-%02d", 21+day), count})
	}
	return profile
}

func profileApp(t *testing.T) (AppModel, *profileCalls) {
	t.Helper()
	m := threadApp(t)
	calls := &profileCalls{}
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		calls.profiles = append(calls.profiles, handle)
		return fixtureProfile(handle), nil
	}
	m.loadUserCommits = func(_ context.Context, handle string, replies bool, cursor string) (api.CommitPage, error) {
		calls.commits = append(calls.commits, fmt.Sprintf("%s replies=%v cursor=%s", handle, replies, cursor))
		c := fixtureCommit(handle + "-commit")
		c.Body = "Commit by " + handle
		if replies {
			c.Body = "Reply by " + handle
		}
		c.Author.ID, c.Author.Handle = "id-"+handle, handle
		f := false
		c.Starred = &f
		return api.CommitPage{Data: []api.Commit{c}}, nil
	}
	m.loadConnections = func(_ context.Context, handle string, list api.Connections, cursor string) (api.People, error) {
		calls.people = append(calls.people, fmt.Sprintf("%s %s cursor=%s", handle, list, cursor))
		no := false
		page := api.People{Data: []api.User{{ID: "id-linus", Handle: "linus", Name: "Linus", Bio: "Kernel", IsFollowed: &no, FollowsYou: &no}}}
		if cursor == "" {
			page.Next = "page2"
		}
		return page, nil
	}
	m.setFollow = func(_ context.Context, handle string, follow bool) error {
		calls.follows = append(calls.follows, fmt.Sprintf("%s %v", handle, follow))
		return nil
	}
	return m, calls
}

func press(t *testing.T, m AppModel, keys ...string) AppModel {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.handleKey(k)
		m = deliver(next, runAll(cmd))
	}
	return m
}

func frameText(m AppModel) string { return ansi.Strip(m.View().Content) }

// profileText is the frame plus the profile's whole scrollable page, which a short terminal cannot show at once.
func profileText(m AppModel) string {
	return frameText(m) + ansi.Strip(m.profiles[len(m.profiles)-1].viewport.GetContent())
}

func TestProfileOpensFromLogLikeTheWebAndEscRestoresTheRow(t *testing.T) {
	m, calls := profileApp(t)
	m, _ = m.selectCommit(1)
	offset := m.feeds[0].viewport.YOffset()
	if !strings.Contains(ansi.Strip(m.feeds[0].viewport.GetContent()), "Profile  u ") {
		t.Fatal("selected log entry lacks a profile chip")
	}
	m = press(t, m, "u")
	if len(m.profiles) != 1 || fmt.Sprint(calls.profiles) != "[ada]" || fmt.Sprint(calls.commits) != "[ada replies=false cursor=]" {
		t.Fatalf("profile requests: %+v", calls)
	}
	frame := profileText(m)
	for _, want := range []string{"Name ada", "@ada", "Follows you", "Lv. 5 · wheel", "Bio of ada", "Lisbon", "ada.dev", "3 commits", "2 following", "10 followers", "Follow  f", "Commit activity", "12 commits in the last 7 days", "longest streak 4 days", "current 2", "1,821 XP", "1 / 2 unlocked", "679 XP to root", "init", "Commits", "Replies", "Commit by ada"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("profile lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "uptime") {
		t.Fatal("locked badge shown on the shelf")
	}
	m = press(t, m, "esc")
	if len(m.profiles) != 0 || m.feeds[0].selected != 1 || m.feeds[0].viewport.YOffset() != offset {
		t.Fatal("leaving the profile lost the log row")
	}
}

func TestProfileShortcutFromThreadReturnsToTheSameEntry(t *testing.T) {
	m, calls := profileApp(t)
	m = openThread(t, m)
	m = press(t, m, "j", "j", "j")
	selected := m.threads[0].selectedID
	author := m.threads[0].entries()[m.threads[0].selected].commit.Author.Handle
	m = press(t, m, "u")
	if len(m.profiles) != 1 || m.profiles[0].handle != author || calls.profiles[0] != author {
		t.Fatalf("thread profile: %+v", calls.profiles)
	}
	if !strings.Contains(frameText(m), "Name "+author) {
		t.Fatal("thread profile did not replace the thread column")
	}
	m = press(t, m, "esc")
	if len(m.profiles) != 0 || len(m.threads) != 1 || m.threads[0].selectedID != selected {
		t.Fatal("profile return lost the thread entry")
	}
}

func TestThreadsOpenedFromAProfileStackAboveIt(t *testing.T) {
	m, _ := profileApp(t)
	m = press(t, m, "u", "enter")
	if len(m.threads) != 1 || len(m.profiles) != 1 || m.profileOnTop() {
		t.Fatal("enter on a profile commit did not open its thread above the profile")
	}
	m = press(t, m, "esc")
	if len(m.threads) != 0 || !m.profileOnTop() {
		t.Fatal("thread return did not restore the profile")
	}
}

func TestNotificationProfileShortcutsAndFollowRows(t *testing.T) {
	m, calls := profileApp(t)
	m.fetchNotifications = func(context.Context, string, string) (api.Notifications, error) {
		return api.Notifications{}, errors.New("fixture unavailable")
	}
	m.fetchNotificationGroup = func(context.Context, string, string, string) (api.Notifications, error) {
		member := fixtureNotification("member")
		member.Actor = &api.Author{ID: "id-ada", Handle: "ada"}
		return api.Notifications{Data: []api.Notification{member}}, nil
	}
	m, _ = m.openNotifications()
	follow := fixtureNotification("follow")
	follow.Type, follow.Commit, follow.EventCount, follow.ActorsCount = "followed", nil, 1, 1
	follow.Actor = &api.Author{ID: "id-grace", Handle: "grace", Name: "Grace"}
	grouped := fixtureNotification("group")
	grouped.Actor = &api.Author{ID: "id-linus", Handle: "linus", Name: "Linus"}
	m = inboxPage(m, follow, grouped)
	m = press(t, m, "enter")
	if len(m.profiles) != 1 || m.profiles[0].handle != "grace" {
		t.Fatalf("enter on a follow row: %+v", calls.profiles)
	}
	m = press(t, m, "esc")
	if !m.notificationsOpen || len(m.profiles) != 0 || m.notificationFeed().selected != 0 {
		t.Fatal("profile return lost the inbox row")
	}
	m = press(t, m, "j", "u")
	if len(m.profiles) != 1 || m.profiles[0].handle != "linus" {
		t.Fatalf("u on a grouped row: %+v", calls.profiles)
	}
	m = press(t, m, "esc", "space")
	if len(m.profiles) != 0 || m.notificationFeed().groups["group"] == nil || !m.notificationFeed().groups["group"].open {
		t.Fatal("space no longer expands the group")
	}
}

func TestOwnProfileOpensWithGPAndHidesFollow(t *testing.T) {
	m, calls := profileApp(t)
	m, _ = m.handleKey("g")
	m = press(t, m, "p")
	if len(m.profiles) != 1 || calls.profiles[0] != "fixture" {
		t.Fatalf("g p: %+v", calls.profiles)
	}
	frame := profileText(m)
	if strings.Contains(frame, "Follow  f") || strings.Contains(frame, "Following  f") {
		t.Fatal("own profile offers follow")
	}
	m = press(t, m, "f")
	if len(calls.follows) != 0 {
		t.Fatal("followed yourself")
	}
}

func TestFollowConfirmsOnceAndUpdatesCounts(t *testing.T) {
	m, calls := profileApp(t)
	m = press(t, m, "u")
	next, cmd := m.handleKey("f")
	again, second := next.handleKey("f")
	if second != nil || cmd == nil {
		t.Fatal("a second follow started while one was pending")
	}
	m = deliver(again, runAll(cmd))
	p := m.profiles[0].profile
	if fmt.Sprint(calls.follows) != "[ada true]" || p.IsFollowing == nil || !*p.IsFollowing || *p.User.Followers != 11 || m.toast.kind != "success" {
		t.Fatalf("follow: %+v %+v", calls.follows, m.toast)
	}
	if !strings.Contains(profileText(m), "Following  f") {
		t.Fatal("follow chip did not change")
	}
	m = press(t, m, "f")
	p = m.profiles[0].profile
	if *p.IsFollowing || *p.User.Followers != 10 {
		t.Fatal("unfollow did not reconcile")
	}
}

func TestFollowFailuresRollBackAndUnknownOutcomesReload(t *testing.T) {
	m, calls := profileApp(t)
	m = press(t, m, "u")
	m.setFollow = func(context.Context, string, bool) error {
		return &auth.APIError{Status: http.StatusForbidden}
	}
	m = press(t, m, "f")
	p := m.profiles[0].profile
	if *p.IsFollowing || *p.User.Followers != 10 || m.toast.kind != "error" || m.follow.handle != "" {
		t.Fatalf("forbidden follow changed state: %+v", m.toast)
	}
	m.setFollow = func(context.Context, string, bool) error {
		return &auth.APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", RetryAfter: "60"}
	}
	m = press(t, m, "f")
	if m.toast.kind != "warning" || *m.profiles[0].profile.IsFollowing {
		t.Fatalf("rate limit: %+v", m.toast)
	}
	before := len(calls.profiles)
	m.setFollow = func(context.Context, string, bool) error { return errors.New("connection reset") }
	next, cmd := m.handleKey("f")
	updated, followUp := next.update(runAll(cmd)[0])
	m = updated.(AppModel)
	if reload, ok := followUp().(tea.BatchMsg); ok {
		m = deliver(m, runAll(reload[0]))
	}
	if m.toast.kind != "warning" || len(calls.profiles) != before+1 {
		t.Fatalf("unknown outcome did not reload: %+v %v", m.toast, calls.profiles)
	}
}

func TestProfileTabsAndConnectionLists(t *testing.T) {
	m, calls := profileApp(t)
	m = press(t, m, "u", "tab")
	if !strings.Contains(profileText(m), "Reply by ada") || calls.commits[len(calls.commits)-1] != "ada replies=true cursor=" {
		t.Fatalf("replies tab: %v", calls.commits)
	}
	m = press(t, m, "4")
	if len(m.profiles) != 2 || !m.profiles[1].people || m.profiles[1].tab != 1 || calls.people[0] != "ada following cursor=" {
		t.Fatalf("following list: %v", calls.people)
	}
	frame := profileText(m)
	for _, want := range []string{"Followers", "Following", "Linus", "@linus", "Kernel", "Follow  f"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("connections lack %q:\n%s", want, frame)
		}
	}
	m = press(t, m, "n")
	if calls.people[len(calls.people)-1] != "ada following cursor=page2" {
		t.Fatalf("pagination: %v", calls.people)
	}
	m = press(t, m, "f")
	if fmt.Sprint(calls.follows) != "[linus true]" || !*m.profiles[1].lists[1].people[0].IsFollowed {
		t.Fatalf("row follow: %v", calls.follows)
	}
	m = press(t, m, "enter")
	if len(m.profiles) != 3 || m.profiles[2].handle != "linus" {
		t.Fatal("enter did not open the person")
	}
	m = press(t, m, "esc", "esc")
	if len(m.profiles) != 1 || m.profiles[0].tab != 1 {
		t.Fatal("connections did not return to the profile tab")
	}
}

func TestStaleProfileResponsesAreIgnored(t *testing.T) {
	m, _ := profileApp(t)
	next, cmd := m.handleKey("u")
	msgs := runAll(cmd)
	next, _ = next.handleKey("esc")
	next = deliver(next, msgs)
	if len(next.profiles) != 0 {
		t.Fatal("a closed profile came back")
	}
	if strings.Contains(frameText(next), "Name ada") {
		t.Fatal("stale profile rendered")
	}
}

func TestProfileCommitsFollowStarsAndDeletes(t *testing.T) {
	m, _ := profileApp(t)
	m = press(t, m, "u")
	m.setStar = func(_ context.Context, id string, star bool) (api.Commit, error) {
		c := fixtureCommit(id)
		c.Stars, c.Starred = 9, &star
		return c, nil
	}
	m = press(t, m, "s")
	if c := m.profiles[0].lists[0].commits[0]; c.Stars != 9 || c.Starred == nil || !*c.Starred {
		t.Fatalf("profile star: %+v", c)
	}
	m = deliver(m, []tea.Msg{deleteResult{session: m.session, id: "ada-commit"}})
	if len(m.profiles[0].lists[0].commits) != 0 || !strings.Contains(profileText(m), "No commits yet.") {
		t.Fatal("deleted commit stayed on the profile")
	}
}

func TestProfileImageClickNearTheEndLoadsTheNextPage(t *testing.T) {
	m, calls := profileApp(t)
	m.loadUserCommits = func(_ context.Context, handle string, _ bool, cursor string) (api.CommitPage, error) {
		calls.commits = append(calls.commits, "cursor="+cursor)
		var page api.CommitPage
		for _, id := range []string{"x1", "x2", "x3"} {
			c := fixtureCommit(cursor + id)
			c.Body = "Pictured " + id
			c.Images = []api.Image{{URL: "https://cdn.example.test/" + id + ".png", Description: "Diagram"}}
			page.Data = append(page.Data, c)
		}
		if cursor == "" {
			page.Next = "p2"
		}
		return page, nil
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 120})
	m = press(t, next.(AppModel), "u")
	var target pictureArea
	for _, picture := range m.profiles[0].pictures {
		if picture.entry == 1 {
			target = picture
		}
	}
	updated, cmd := m.profileMouse(target.x, target.y+threadHeaderHeight-m.profiles[0].viewport.YOffset())
	m = deliver(updated, runAll(cmd))
	if !m.viewer.open || fmt.Sprint(calls.commits) != "[cursor= cursor=p2]" || m.profiles[0].lists[0].loading {
		t.Fatalf("image click near the end: viewer=%v requests=%v", m.viewer.open, calls.commits)
	}
}

func TestUnknownFollowOutcomeFromAConnectionRowReloadsTheList(t *testing.T) {
	m, calls := profileApp(t)
	m = press(t, m, "u", "3")
	before := len(calls.people)
	m.setFollow = func(context.Context, string, bool) error { return context.DeadlineExceeded }
	next, cmd := m.handleKey("f")
	updated, followUp := next.update(runAll(cmd)[0])
	m = updated.(AppModel)
	if reload, ok := followUp().(tea.BatchMsg); ok {
		m = deliver(m, runAll(reload[0]))
	}
	if m.toast.kind != "warning" || len(calls.people) != before+1 || calls.people[len(calls.people)-1] != "ada followers cursor=" {
		t.Fatalf("unknown outcome did not reload the row: %+v %v", m.toast, calls.people)
	}
}

func TestConfirmedFollowMovesTheAccountCountAfterTheScreenCloses(t *testing.T) {
	m, _ := profileApp(t)
	m.account.user.Following = new(5)
	m = press(t, m, "u")
	next, cmd := m.handleKey("f")
	next, _ = next.handleKey("esc")
	m = deliver(next, runAll(cmd))
	if *m.account.user.Following != 6 || m.toast.kind != "success" {
		t.Fatalf("account following: %d", *m.account.user.Following)
	}
}

func TestDeletingAnEarlierProfileEntryKeepsTheSelectedCommit(t *testing.T) {
	m, _ := profileApp(t)
	m.loadUserCommits = func(context.Context, string, bool, string) (api.CommitPage, error) {
		return api.CommitPage{Data: []api.Commit{fixtureCommit("x1"), fixtureCommit("x2"), fixtureCommit("x3")}}, nil
	}
	m = press(t, m, "u", "j")
	m = deliver(m, []tea.Msg{deleteResult{session: m.session, id: "x1"}})
	if s := m.profiles[0]; s.entries()[s.lists[0].selected].ID != "x2" {
		t.Fatalf("selection moved to %s", s.entries()[s.lists[0].selected].ID)
	}
}

func TestGGOnAProfileSelectsItsFirstEntryAndKeepsTheLogRow(t *testing.T) {
	m, _ := profileApp(t)
	m.loadUserCommits = func(context.Context, string, bool, string) (api.CommitPage, error) {
		return api.CommitPage{Data: []api.Commit{fixtureCommit("x1"), fixtureCommit("x2"), fixtureCommit("x3")}}, nil
	}
	m, _ = m.selectCommit(2)
	m = press(t, m, "u", "j", "j")
	m, _ = m.handleKey("g")
	m = press(t, m, "g")
	if m.profiles[0].lists[0].selected != 0 || m.feeds[0].selected != 2 {
		t.Fatalf("g g: profile=%d log=%d", m.profiles[0].lists[0].selected, m.feeds[0].selected)
	}
}

func TestMonthActivityDrawsSeparateBarsAcrossTheWidth(t *testing.T) {
	m, _ := profileApp(t)
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		profile := fixtureProfile(handle)
		profile.Activity = &api.Activity{Scale: "month"}
		for day := range 30 {
			profile.Activity.Buckets = append(profile.Activity.Buckets, struct {
				Date  string `json:"date"`
				Count int    `json:"count"`
			}{fmt.Sprintf("2026-09-%02d", day+1), day % 4})
		}
		return profile, nil
	}
	for _, width := range []int{40, 61, 92} {
		var frame profileFrame
		profile, _ := m.loadProfile(context.Background(), "ada")
		m.activityGraph(&frame, profile.Activity, "#fcc419", width, colors(true))
		bars := strings.TrimPrefix(ansi.Strip(frame.lines[2]), "  ")
		separators := strings.Count(bars, " ") + strings.Count(bars, "▌")
		if ansi.StringWidth(bars) != width || separators < 29 {
			t.Fatalf("width %d: %d cells, %d separators: %q", width, ansi.StringWidth(bars), separators, bars)
		}
	}
}
