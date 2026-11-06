package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func pullMembers(through ...int64) []api.Member {
	members := []api.Member{{User: api.User{ID: "fixture", Handle: "fixture"}, ReadThroughID: 99}}
	for i, read := range through {
		handle := fmt.Sprintf("m%d", i+1)
		members = append(members, api.Member{User: api.User{ID: handle, Handle: handle}, ReadThroughID: read})
	}
	return members
}

func TestPulledByMirrorsTheWeb(t *testing.T) {
	cases := []struct {
		name    string
		members []api.Member
		author  string
		want    string
	}{
		{"nobody", pullMembers(1, 1), "fixture", ""},
		{"some", pullMembers(5, 1, 9), "fixture", "@m1, @m3"},
		{"author left out", pullMembers(5, 5), "m1", "@m2"},
		{"all of three", pullMembers(5, 5, 5), "fixture", "@m1, @m2, @m3"},
		{"everyone", pullMembers(5, 5, 5, 5), "fixture", "everyone"},
		{"more than five", pullMembers(5, 5, 5, 5, 5, 5, 5, 1), "fixture", "@m1, @m2, @m3, @m4, @m5, +2"},
	}
	for _, c := range cases {
		shown, rest, everyone := pulledBy(orgPulls(c.members, api.Message{ID: 5, UserID: c.author}, "fixture"))
		var got []string
		for _, user := range shown {
			got = append(got, "@"+user.Handle)
		}
		if rest > 0 {
			got = append(got, fmt.Sprintf("+%d", rest))
		}
		if everyone {
			got = []string{"everyone"}
		}
		if got := strings.Join(got, ", "); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOrgThreadsShowWhoPulledTheLatestMessage(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m = openOrg(t, m)
	th := m.remotes.thread
	th.data.Members = []api.Member{{User: api.User{ID: "fixture", Handle: "fixture"}, ReadThroughID: 9}, {User: api.User{ID: "grace", Handle: "grace"}, ReadThroughID: 9}, {User: api.User{ID: "linus", Handle: "linus"}, ReadThroughID: 9}}
	m.renderRemoteThread()
	if frame := threadText(m); !strings.Contains(frame, "· pulled by ●") || strings.Contains(frame, "@linus") {
		t.Fatalf("someone else's latest message lacks pulled by:\n%s", frame)
	}

	th.messages = append(th.messages, remoteMessage(10, "fixture", "on my way", m.now))
	m.renderRemoteThread()
	if frame := threadText(m); !strings.Contains(frame, "✓ pushed") {
		t.Fatalf("your unread message lacks pushed:\n%s", frame)
	}
	th.data.Members[2].ReadThroughID = 10
	m.renderRemoteThread()
	if frame := threadText(m); !strings.Contains(frame, "✓✓ pulled by ●") || strings.Contains(frame, "· pulled by") {
		t.Fatalf("only your latest message shows pulled by:\n%s", frame)
	}
}

func TestPulledByDrawsAvatarsAndFitsTheFooter(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m = openOrg(t, m)
	p := colors(m.account.dark)
	pulled := []api.User{{ID: "a", Handle: "a", AvatarURL: "https://example.test/a.png"}, {ID: "b", Handle: "b", AvatarURL: "https://example.test/b.png"}}
	row, requests := m.pulledByFaces(pulled, 3, 20, p)
	if ansi.Strip(row) != "●  ● " || len(requests) != 2 || requests[0].Cols != 2 || requests[0].Rows != 1 {
		t.Fatalf("row %q, requests %+v", ansi.Strip(row), requests)
	}
	if row, _ = m.pulledByFaces(pulled, 3, 4, p); ansi.Strip(row) != "+2" {
		t.Fatalf("a narrow footer drew %q, want the avatars counted", ansi.Strip(row))
	}
	if row, requests = m.pulledByFaces(append(pulled, pulled...), 4, 20, p); ansi.Strip(row) != "everyone" || requests != nil {
		t.Fatalf("everyone drew %q", ansi.Strip(row))
	}
	if row, _ = m.pulledByFaces(append(pulled, pulled...), 4, 7, p); row != "" {
		t.Fatalf("everyone overflowed a 7-cell budget: %q", ansi.Strip(row))
	}
	// A 30-column terminal leaves a narrow bubble, and the footer must stay inside it.
	m.account.width = 30
	m.remotes.thread.data.Members = pullMembers(10, 10, 10, 10, 10)
	m.remotes.thread.messages = append(m.remotes.thread.messages, remoteMessage(10, "fixture", "on my way", m.now))
	m.renderRemoteThread()
	for _, line := range strings.Split(m.remotes.thread.viewport.GetContent(), "\n") {
		if strings.Contains(line, "pulled") && lipgloss.Width(line) > m.remotes.thread.viewport.Width() {
			t.Fatalf("the footer is %d cells in a %d-cell pane: %q", lipgloss.Width(line), m.remotes.thread.viewport.Width(), ansi.Strip(line))
		}
	}
}
