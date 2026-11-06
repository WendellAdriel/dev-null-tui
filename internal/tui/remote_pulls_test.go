package tui

import (
	"fmt"
	"strings"
	"testing"

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
		{"more than five", pullMembers(5, 5, 5, 5, 5, 5, 5, 1), "fixture", "@m1, @m2, @m3, @m4, @m5 +2"},
	}
	for _, c := range cases {
		pulled, total := orgPulls(c.members, api.Message{ID: 5, UserID: c.author}, "fixture")
		if got := pulledByText(pulled, total); got != c.want {
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
	if frame := threadText(m); !strings.Contains(frame, "· pulled by @linus") {
		t.Fatalf("someone else's latest message lacks pulled by:\n%s", frame)
	}

	th.messages = append(th.messages, remoteMessage(10, "fixture", "on my way", m.now))
	m.renderRemoteThread()
	if frame := threadText(m); !strings.Contains(frame, "✓ pushed") {
		t.Fatalf("your unread message lacks pushed:\n%s", frame)
	}
	th.data.Members[2].ReadThroughID = 10
	m.renderRemoteThread()
	if frame := threadText(m); !strings.Contains(frame, "✓✓ pulled by @linus") || strings.Contains(frame, "· pulled by") {
		t.Fatalf("only your latest message shows pulled by:\n%s", frame)
	}
}
