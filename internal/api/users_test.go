package api

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestProfileDecodesSignedMetadataAndKeepsMissingCountsDistinct(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/users/ada" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"u1","handle":"ada","name":"Ada","website":"https://ada.dev","github_url":"https://github.com/ada","created_at":"2026-10-01T00:00:00Z","commits_count":3,"followers_count":0,"lane":4},
			"is_following":true,"follows_you":false,"is_muting":false,"pasteis_received":7,
			"activity":{"scale":"week","buckets":[{"date":"2026-10-21","count":0},{"date":"2026-10-22","count":2}]},
			"streaks":{"longest":4,"current":2},
			"achievements":{"level":5,"rank":"wheel","xp":1821,"next_level_xp":2500,"level_progress":72.8,"ranks":[{"rank":"wheel","level":5,"min_xp":1500,"achieved":true},{"rank":"root","level":6,"min_xp":2500,"achieved":false}],"badges":[{"id":"init","name":"init","unlocked":true},{"id":"uptime","name":"uptime","unlocked":false}]},
			"pinned_commit":{"id":"c9","body":"Pinned","author":{"id":"u1","handle":"ada"}}}`)
	})
	profile, err := client.Profile(t.Context(), "ada")
	if err != nil {
		t.Fatal(err)
	}
	user := profile.User
	if user.Handle != "ada" || user.Website != "https://ada.dev" || user.Commits == nil || *user.Commits != 3 || user.Followers == nil || *user.Followers != 0 || user.Following != nil {
		t.Fatalf("user: %+v", user)
	}
	if profile.IsFollowing == nil || !*profile.IsFollowing || profile.FollowsYou == nil || *profile.FollowsYou || profile.PasteisReceived == nil || *profile.PasteisReceived != 7 {
		t.Fatalf("relationship: %+v", profile)
	}
	if profile.Activity == nil || profile.Activity.Scale != "week" || len(profile.Activity.Buckets) != 2 || profile.Activity.Buckets[1].Count != 2 {
		t.Fatalf("activity: %+v", profile.Activity)
	}
	if profile.Streaks == nil || profile.Streaks.Longest != 4 || profile.Achievements == nil || profile.Achievements.Rank != "wheel" || profile.Achievements.NextLevelXP == nil || len(profile.Achievements.Badges) != 2 {
		t.Fatalf("achievements: %+v %+v", profile.Streaks, profile.Achievements)
	}
	if profile.Pinned == nil || profile.Pinned.ID != "c9" {
		t.Fatalf("pinned: %+v", profile.Pinned)
	}
}

func TestProfileAcceptsPublicResponsesAndRejectsUnsafeOrMismatchedHandles(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"u1","handle":"Ada","name":"Ada","created_at":"2026-10-01T00:00:00Z"}}`)
	})
	profile, err := client.Profile(t.Context(), "ada")
	if err != nil || profile.IsFollowing != nil || profile.Activity != nil || profile.Pinned != nil {
		t.Fatalf("public profile: %+v %v", profile, err)
	}
	for _, handle := range []string{"", "../me", "a/b", "ada?x=1"} {
		if _, err := client.Profile(t.Context(), handle); err == nil {
			t.Fatalf("accepted unsafe handle %q", handle)
		}
	}
	wrong := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"u2","handle":"grace"}}`)
	})
	if _, err := wrong.Profile(t.Context(), "ada"); err == nil {
		t.Fatal("accepted another user's profile")
	}
}

func TestUserCommitsAndConnectionsFollowCursorPages(t *testing.T) {
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/v1/users/ada/commits":
			_, _ = io.WriteString(w, `{"data":[{"id":"c1","author":{"id":"u1","handle":"ada"}}],"meta":{"next_cursor":"n1"}}`)
		default:
			_, _ = io.WriteString(w, `{"data":[{"id":"u2","handle":"grace","name":"Grace","is_followed":true,"follows_you":false}],"meta":{"next_cursor":null}}`)
		}
	})
	commits, err := client.UserCommits(t.Context(), "ada", true, "c0")
	if err != nil || len(commits.Data) != 1 || commits.Next != "n1" {
		t.Fatalf("commits: %+v %v", commits, err)
	}
	for _, list := range []Connections{Followers, FollowingList} {
		people, err := client.Connections(t.Context(), "ada", list, "")
		if err != nil || len(people.Data) != 1 || people.Data[0].IsFollowed == nil || !*people.Data[0].IsFollowed || people.Next != "" {
			t.Fatalf("%s: %+v %v", list, people, err)
		}
	}
	if fmt.Sprint(requests) != "[/api/v1/users/ada/commits?cursor=c0&replies=1 /api/v1/users/ada/followers /api/v1/users/ada/following]" {
		t.Fatal(requests)
	}
	if _, err := client.Connections(t.Context(), "ada", "mutuals", ""); err == nil {
		t.Fatal("accepted an unknown connection list")
	}
	repeated := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"u2","handle":"grace"}],"meta":{"next_cursor":"same"}}`)
	})
	if _, err := repeated.Connections(t.Context(), "ada", Followers, "same"); err == nil {
		t.Fatal("accepted a repeated cursor")
	}
	invalid := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"u2"}]}`)
	})
	if _, err := invalid.Connections(t.Context(), "ada", Followers, ""); err == nil {
		t.Fatal("accepted a person without a handle")
	}
}

func TestFollowUsesSignedNoContentWrites(t *testing.T) {
	var calls []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned follow")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	for _, follow := range []bool{true, false} {
		if err := client.SetFollow(t.Context(), "ada", follow); err != nil {
			t.Fatal(err)
		}
	}
	if fmt.Sprint(calls) != "[POST /api/v1/users/ada/follow DELETE /api/v1/users/ada/follow]" {
		t.Fatal(calls)
	}
	if err := client.SetFollow(t.Context(), "../me", true); err == nil {
		t.Fatal("unsafe follow handle")
	}
}
