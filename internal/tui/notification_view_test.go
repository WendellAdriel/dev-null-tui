package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func TestExpandedStarsAndFollowsUseCompactActorRowsAndReadyAvatars(t *testing.T) {
	for _, kind := range []string{"starred", "followed"} {
		t.Run(kind, func(t *testing.T) {
			m := inboxFixture(t)
			m.images.Enabled = true
			n := fixtureNotification("group")
			n.Type = kind
			n.Actor.AvatarURL = "https://example.test/actor.png"
			if kind == "followed" {
				n.Commit = nil
			}
			member := n
			member.ID = "member"
			member.Actor = &api.Author{Name: "Expanded Actor", Handle: "expanded", Lane: []byte("3"), AvatarURL: "https://example.test/member.png"}
			m = inboxPage(m, n)
			f := m.notificationFeed()
			f.groups[n.ID] = &notificationGroup{open: true, loaded: true, items: []api.Notification{member}}
			m.renderNotifications()
			req := m.avatarRequest(member.Actor.AvatarURL, authorColor(member.Actor, colors(true)), 4, 2)
			requests := []graphics.Request{req}
			star := starIconRequest(true, authorColor(member.Actor, colors(true)))
			if kind == "starred" {
				if !slices.Contains(m.notificationMedia(), star) {
					t.Fatal("member star icon is not requested in the actor's lane")
				}
				requests = append(requests, star)
			}
			m.images.Clear()
			jobs, _ := m.images.Plan(t.Context(), requests)
			for _, job := range jobs {
				m.images.Accept(graphics.Result{Job: job, Raw: "ready"})
			}
			m.renderNotifications()
			content := f.viewport.GetContent()
			plain := ansi.Strip(content)
			verb := "starred your commit"
			if kind == "followed" {
				verb = "followed you"
			}
			if strings.Count(plain, verb) != 1 || strings.Count(plain, "commit-group") > 1 {
				t.Fatalf("expanded row repeats parent text:\n%s", plain)
			}
			start, end := f.starts[1], f.ends[1]
			rows := strings.Split(content, "\n")[start:end]
			if strings.Count(rows[0], string(kitty.Placeholder)) < 4 || strings.Count(rows[1], string(kitty.Placeholder)) != 4 || !strings.Contains(ansi.Strip(rows[0]), "Expanded Actor @expanded") {
				t.Fatal("expanded identity lacks a two-row avatar")
			}
			if star := strings.Count(rows[0], string(kitty.Placeholder)) - 4; (kind == "starred") != (star == 2) {
				t.Fatalf("member star icon cells: %d", star)
			}
			if gap := strings.Split(plain, "\n")[start-1]; strings.Trim(gap, " ▌") != "│" {
				t.Fatalf("member list touches the hide label: %q", gap)
			}
			if end-start > 3 {
				t.Fatalf("expanded row is not compact: %d", end-start)
			}
			m, _ = m.selectNotification(1)
			selected := ansi.Strip(f.viewport.GetContent())
			label := ""
			for _, line := range strings.Split(selected, "\n") {
				if strings.Contains(line, "Hide "+notificationNoun(n)) {
					label += line
				}
			}
			if strings.Count(label, "Hide") != 1 || !strings.Contains(label, "⌄ Hide") || !strings.Contains(label, "space") {
				t.Fatalf("selected member must keep the hide shortcut on the group label only:\n%s", selected)
			}
		})
	}
}

func TestSelectedNotificationShowsInlineExpandAndHideShortcut(t *testing.T) {
	m := inboxPage(inboxFixture(t), fixtureNotification("a"), fixtureNotification("b"))
	f := m.notificationFeed()
	if strings.Count(ansi.Strip(f.viewport.GetContent()), "space") != 1 {
		t.Fatal("show expand shortcut only on the active group")
	}
	m, _ = m.notificationKey("space")
	if content := ansi.Strip(f.viewport.GetContent()); !strings.Contains(content, "Hide stars") || !strings.Contains(content, "space") {
		t.Fatal("hide shortcut missing")
	}
}

func TestNotificationIconUsesLargerCircularPlacementWithoutResizingAvatars(t *testing.T) {
	m := inboxFixture(t)
	m.images.Enabled = true
	n := fixtureNotification("star")
	n.Actor.AvatarURL = "https://example.test/avatar.png"
	m = inboxPage(m, n)
	var icon graphics.Request
	for _, req := range m.notificationMedia() {
		if req.Icon == "starred" {
			icon = req
		}
	}
	if icon.Icon == "" || icon.Cols != 5 || icon.Rows != 2 {
		t.Fatal("event icon lacks larger circular placement")
	}
	m.images.Clear()
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{icon})
	if len(jobs) != 1 {
		t.Fatal("icon job not scheduled")
	}
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	m.renderNotifications()
	f := m.notificationFeed()
	rows := strings.Split(f.viewport.GetContent(), "\n")
	for row := range 2 {
		if strings.Count(rows[f.starts[0]+row], string(kitty.Placeholder)) != 5 {
			t.Fatal("icon does not occupy two five-cell rows")
		}
	}
}

func TestGroupedNotificationActorsUseTwoRowAvatars(t *testing.T) {
	m := inboxFixture(t)
	m.images.Enabled = true
	n := fixtureNotification("group")
	n.Actor.AvatarURL = "https://example.test/actor.png"
	n.Actors[0].AvatarURL = n.Actor.AvatarURL
	m = inboxPage(m, n)
	p := colors(true)
	req := m.avatarRequest(n.Actor.AvatarURL, authorColor(n.Actor, p), 4, 2)
	if !slices.Contains(m.notificationMedia(), req) {
		t.Fatal("actor stack avatar is not requested at two rows")
	}
	m.images.Clear()
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{req})
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	m.renderNotifications()
	f := m.notificationFeed()
	rows := strings.Split(f.viewport.GetContent(), "\n")
	for row := range 2 {
		if strings.Count(rows[f.starts[0]+row], string(kitty.Placeholder)) != 4 {
			t.Fatalf("actor stack row %d lacks the two-row avatar", row)
		}
	}
}

func TestNotificationChipsSitUnderARuleLikeACommits(t *testing.T) {
	m, _ := profileApp(t)
	m.fetchNotifications = func(context.Context, string, string) (api.Notifications, error) {
		return api.Notifications{}, errors.New("fixture unavailable")
	}
	m, _ = m.openNotifications()
	m = inboxPage(m, fixtureNotification("star"))
	lines := strings.Split(ansi.Strip(m.notificationFeed().viewport.GetContent()), "\n")
	for i, line := range lines {
		if strings.Contains(line, "Open  enter") {
			if i == 0 || !strings.Contains(lines[i-1], "────") {
				t.Fatalf("the chips lack the rule above them:\n%s", strings.Join(lines, "\n"))
			}
			return
		}
	}
	t.Fatalf("no Open chip on the selected notification:\n%s", strings.Join(lines, "\n"))
}

func notificationText(m AppModel, n api.Notification) string {
	rows, _ := m.notificationRows(n, 80, colors(true), false)
	return ansi.Strip(strings.Join(rows, "\n"))
}

func TestTimeoutNotificationsSayWhyAndUntilWhen(t *testing.T) {
	m := inboxFixture(t)
	// The server makes you the actor of your own timeout, so the row links to your profile.
	you := &api.Author{Type: "user", ID: m.account.user.ID, Handle: m.account.user.Handle, Name: "Your Name"}
	ends := time.Date(2026, 11, 16, 13, 0, 0, 0, time.UTC)
	n := api.Notification{ID: "t", GroupID: "t", Type: "timed_out", Actor: you, EventCount: 1, CreatedAt: ends.Add(-3 * time.Hour), Timeout: &api.Timeout{Reason: "Spam or self-promotion", Duration: "3 hours", EndsAt: ends}}
	got := notificationText(m, n)
	for _, want := range []string{"You were put in timeout", "Reason: Spam or self-promotion · Duration: 3 hours", "Posting resumes " + scheduleTime(ends) + "."} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Your Name") || strings.Contains(got, "Notification") {
		t.Fatalf("a timeout names its actor or reads as unknown:\n%s", got)
	}
	m = inboxPage(m, n)
	m, _ = m.notificationKey("enter")
	if len(m.profiles) != 1 || m.profiles[0].handle != m.account.user.Handle {
		t.Fatal("enter did not open your profile, as the web's link does")
	}
}

func TestFirstCommitsAndLevelUpsReadLikeTheWeb(t *testing.T) {
	m := inboxFixture(t)
	first := fixtureNotification("f")
	first.Type, first.EventCount, first.ActorsCount = "first_commit", 1, 1
	if got := notificationText(m, first); !strings.Contains(got, "Made their first commit") || !strings.Contains(got, "commit-f") {
		t.Fatalf("first commit:\n%s", got)
	}
	level := fixtureNotification("l")
	level.Type, level.Commit, level.EventCount, level.ActorsCount = "leveled_up", nil, 1, 1
	if got := notificationText(m, level); !strings.Contains(got, "You reached a new level") || strings.Contains(got, level.Actor.Name) {
		t.Fatalf("level up:\n%s", got)
	}
}

func TestOrganizationNotificationsNameTheOrganization(t *testing.T) {
	m := inboxFixture(t)
	laravel := &api.Organization{Handle: "laravel", Name: "Laravel"}
	for kind, want := range map[string]string{
		"replied":   "Replied to @laravel's commit",
		"quoted":    "Quoted @laravel's commit",
		"starred":   "starred @laravel's commit",
		"mentioned": "Mentioned @laravel",
		"approved":  "@laravel's commit was approved",
		"rejected":  "Only @laravel's members can see this update and its rejected commit.",
	} {
		n := fixtureNotification(kind)
		n.Type, n.Organization = kind, laravel
		if got := notificationText(m, n); !strings.Contains(strings.Join(strings.Fields(got), " "), want) || strings.Contains(got, "your") {
			t.Fatalf("%s: missing %q in:\n%s", kind, want, got)
		}
	}
}
