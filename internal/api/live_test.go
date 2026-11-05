package api

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestGoingLiveAndEndingTheStreamAnswerWithTheAccount(t *testing.T) {
	identity, err := os.ReadFile("../auth/testdata/current-user-organizations.json")
	if err != nil {
		t.Fatal(err)
	}
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+r.URL.Path+" "+string(body))
		_, _ = w.Write(identity)
	})
	user, err := client.GoLive(t.Context(), " https://twitch.tv/fixture ", 3)
	if err != nil || user.Live == nil || user.Live.Platform != "twitch" {
		t.Fatalf("go live: %+v %v", user, err)
	}
	if _, err := client.EndStream(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := `PUT /api/v1/me/live {"url":"https://twitch.tv/fixture","hours":3}|DELETE /api/v1/me/live `
	if got := strings.Join(requests, "|"); got != want {
		t.Fatalf("requests %q", got)
	}
	for _, hours := range []int{0, 13} {
		if _, err := client.GoLive(t.Context(), "https://twitch.tv/fixture", hours); err == nil {
			t.Fatalf("accepted %d hours", hours)
		}
	}
	if _, err := client.GoLive(t.Context(), "  ", 2); err == nil {
		t.Fatal("accepted an empty link")
	}
	if _, err := client.GoLive(t.Context(), "https://twitch.tv/"+strings.Repeat("a", 2048), 2); err == nil {
		t.Fatal("accepted a link longer than the server allows")
	}
}

func TestLiveFollowingDecodesWhoIsLive(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/me/live-following" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"u1","handle":"ada","name":"Ada","live":{"platform":"youtube","url":"https://www.youtube.com/live/abc","thumbnail_url":"https://i.ytimg.com/vi/abc/hqdefault_live.jpg","ends_at":"2026-11-05T15:00:00Z"}}]}`)
	})
	people, err := client.LiveFollowing(t.Context())
	if err != nil || len(people) != 1 || people[0].Live == nil || people[0].Live.Platform != "youtube" {
		t.Fatalf("live following: %+v %v", people, err)
	}
	invalid := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"data":[{"handle":"ada"}]}`) })
	if _, err := invalid.LiveFollowing(t.Context()); err == nil {
		t.Fatal("accepted a person without an ID")
	}
}
