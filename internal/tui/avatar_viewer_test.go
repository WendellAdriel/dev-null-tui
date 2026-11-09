package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func TestVShowsTheProfileAvatarLargeInTheViewer(t *testing.T) {
	m, _ := profileApp(t)
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		p := fixtureProfile(handle)
		p.User.AvatarURL = "https://avatars.githubusercontent.com/u/7"
		return p, nil
	}
	next, cmd := m.Update(keyMsg("u"))
	m = settleCmd(next.(AppModel), cmd)
	if !m.profileOnTop() {
		t.Fatal("u must open the author's profile")
	}
	m = pressApp(m, "V")
	v := m.viewer
	if !v.open || !v.avatar || v.commit.Images[0].URL != "https://avatars.githubusercontent.com/u/7?s=1024" {
		t.Fatalf("V must open the avatar at 1024px: %+v", v.commit.Images)
	}
	frame := frameText(m)
	if !strings.Contains(frame, "Avatar · @ada") || strings.Contains(frame, "Panel") {
		t.Fatalf("the avatar viewer must name the person and drop the commit panel:\n%s", frame)
	}
	m = pressApp(m, "esc")
	if m.viewer.open {
		t.Fatal("esc must close the avatar")
	}
}

func TestClickingANotificationActorsAvatarOpensIt(t *testing.T) {
	m := inboxFixture(t)
	n := fixtureNotification("group")
	n.Actor.AvatarURL = "https://example.test/actor.png"
	n.Actors[0].AvatarURL = n.Actor.AvatarURL
	m = inboxPage(m, n)
	f := m.notificationFeed()
	if len(f.avatars) == 0 {
		t.Fatal("the actor stack must have a click target")
	}
	hit := f.avatars[0]
	rows := strings.Split(f.viewport.GetContent(), "\n")
	if got := ansi.Strip(ansi.Cut(rows[hit.area.y], hit.area.x, hit.area.x+1)); got != "●" {
		t.Fatalf("the avatar target points at %q", got)
	}
	_, width, _ := m.columns()
	header, _ := m.notificationHeader(width, m.account.colors())
	m, _ = m.notificationMouse(hit.area.x, hit.area.y+lipgloss.Height(header)-f.viewport.YOffset())
	if !m.viewer.open || !m.viewer.avatar || m.viewer.commit.Images[0].URL != graphics.AvatarSizedURL(n.Actor.AvatarURL, avatarViewSize) {
		t.Fatalf("a click on the actor's avatar must open it: %+v", m.viewer)
	}
}

func TestClickingAFollowersAvatarOpensIt(t *testing.T) {
	m, _ := profileApp(t)
	m.images.Enabled = true
	load := m.loadConnections
	m.loadConnections = func(ctx context.Context, handle string, list api.Connections, cursor string) (api.People, error) {
		page, err := load(ctx, handle, list, cursor)
		page.Data[0].AvatarURL = "https://example.test/linus.png"
		return page, err
	}
	m = press(t, m, "u", "F")
	s := m.topProfile()
	person := s.lists[s.tab].people[0]
	req := m.avatarRequest(person.AvatarURL, laneColor(person.Lane, m.account.colors()), 4, 2)
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{req})
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	m.renderProfile(s)
	if len(s.avatars) != 1 {
		t.Fatalf("a follower with a drawn avatar needs one target, got %d", len(s.avatars))
	}
	hit := s.avatars[0]
	m, _ = m.profileMouse(hit.area.x, hit.area.y+threadHeaderHeight-s.viewport.YOffset())
	if !m.viewer.open || m.viewer.label != "@linus" {
		t.Fatalf("a click on the follower's avatar must open it: %+v", m.viewer)
	}
}

func TestClickingAPickerPersonsAvatarClosesThePickerAndOpensIt(t *testing.T) {
	m := fixtureApp(t)
	m.picker = &picker{kind: "stars", title: "Starred by", rows: []pickerRow{{person: &api.User{Handle: "linus", AvatarURL: "https://example.test/linus.png"}, label: "Linus"}}}
	_, _, areas := m.listPickerModal(m.account.colors())
	for _, area := range areas {
		if area.action != "avatar" {
			continue
		}
		m, _ = m.listPickerMouse(area.x, area.y)
		if m.picker != nil || !m.viewer.open || m.viewer.label != "@linus" {
			t.Fatalf("the avatar must close the picker and open: picker %v viewer %+v", m.picker != nil, m.viewer)
		}
		return
	}
	t.Fatal("a person row needs an avatar target")
}

func TestClickingARemotesSendersAvatarOpensIt(t *testing.T) {
	m, _ := messagingApp(t)
	m = openOrg(t, m)
	th := m.remotes.thread
	if len(th.avatars) == 0 {
		t.Fatal("another member's run needs an avatar target")
	}
	hit := th.avatars[0]
	lines := strings.Split(th.viewport.GetContent(), "\n")
	if got := ansi.Strip(ansi.Cut(lines[hit.area.y], hit.area.x, hit.area.x+1)); got != "●" {
		t.Fatalf("the sender target points at %q", got)
	}
	m.threadClick(hit.area.y, hit.area.x)
	if !m.viewer.open && m.notice != "This avatar has no image to view." {
		t.Fatalf("a click on the sender's avatar must ask the viewer: %+v %q", m.viewer, m.notice)
	}
}

func TestClickingAWhoToFollowAvatarClosesTheModalAndOpensIt(t *testing.T) {
	m, _ := suggestionApp(t, 140)
	m.images.Enabled = true
	m = pressApp(m, "w")
	_, _, areas := m.suggestionModal(m.account.colors())
	for _, area := range areas {
		if area.action == "wtf-avatar" && area.index == 0 {
			m, _ = m.suggestionModalMouse(area.x, area.y)
			if m.whoToFollow.open || !m.viewer.open && m.notice != "This avatar has no image to view." {
				t.Fatalf("the avatar must close the modal and ask the viewer: open %v %+v", m.whoToFollow.open, m.viewer)
			}
			return
		}
	}
	t.Fatal("a suggested person needs an avatar target")
}
