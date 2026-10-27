package tui

import (
	"slices"
	"strings"
	"testing"

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
