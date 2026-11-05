package api

import (
	"io"
	"net/http"
	"testing"
)

func TestOrganizationReadsItsPageAndWhatTheViewerCanDo(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/organizations/laravel" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"handle":"Laravel","name":"Laravel","avatar_url":"https://avatars.githubusercontent.com/u/958072","bio":"The PHP framework","website":"https://laravel.com","github_url":"https://github.com/laravel","html_url":"https://dev-null.sh/@laravel","lane":null,
			"followers_count":12,"admins":[{"id":"u1","handle":"taylor","name":"Taylor"}]},
			"is_following":true,"can_claim":false,"settings_url":"https://dev-null.sh/@laravel/settings"}`)
	})
	page, err := client.Organization(t.Context(), "laravel")
	if err != nil {
		t.Fatal(err)
	}
	organization := page.Organization
	if organization.Handle != "Laravel" || organization.Bio != "The PHP framework" || organization.Followers == nil || *organization.Followers != 12 || len(organization.Admins) != 1 || organization.Admins[0].Handle != "taylor" {
		t.Fatalf("organization: %+v", organization)
	}
	if !page.IsFollowing || page.CanClaim || page.SettingsURL == nil || *page.SettingsURL != "https://dev-null.sh/@laravel/settings" {
		t.Fatalf("viewer: %+v", page)
	}
	for _, handle := range []string{"", "../me", "a/b"} {
		if _, err := client.Organization(t.Context(), handle); err == nil {
			t.Fatalf("accepted unsafe handle %q", handle)
		}
	}
	wrong := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"handle":"pestphp"}}`)
	})
	if _, err := wrong.Organization(t.Context(), "laravel"); err == nil {
		t.Fatal("accepted another organization's page")
	}
}

func TestOrganizationListsAndWrites(t *testing.T) {
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/v1/organizations/laravel/commits", "/api/v1/organizations/laravel/mentions":
			next := `"n1"`
			if r.URL.Query().Has("cursor") {
				next = "null"
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"c1","body":"Laravel 13","author":{"type":"organization","id":"o1","handle":"laravel"}}],"meta":{"next_cursor":`+next+`}}`)
		case "/api/v1/users/ada/following/organizations":
			_, _ = io.WriteString(w, `{"data":[{"handle":"laravel","name":"Laravel","is_followed":true}],"meta":{"next_cursor":null}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	commits, err := client.OrganizationCommits(t.Context(), "laravel", false, "")
	if err != nil || len(commits.Data) != 1 || commits.Next != "n1" {
		t.Fatalf("commits: %+v %v", commits, err)
	}
	if _, err := client.OrganizationCommits(t.Context(), "laravel", true, "n1"); err != nil {
		t.Fatal(err)
	}
	followed, err := client.FollowedOrganizations(t.Context(), "ada", "")
	if err != nil || len(followed.Data) != 1 || followed.Data[0].IsFollowed == nil || !*followed.Data[0].IsFollowed || followed.Next != "" {
		t.Fatalf("followed: %+v %v", followed, err)
	}
	if err := client.SetOrganizationFollow(t.Context(), "laravel", true); err != nil {
		t.Fatal(err)
	}
	if err := client.SetOrganizationFollow(t.Context(), "laravel", false); err != nil {
		t.Fatal(err)
	}
	if err := client.ClaimOrganization(t.Context(), "laravel"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /api/v1/organizations/laravel/commits",
		"GET /api/v1/organizations/laravel/mentions?cursor=n1",
		"GET /api/v1/users/ada/following/organizations",
		"POST /api/v1/organizations/laravel/follow",
		"DELETE /api/v1/organizations/laravel/follow",
		"POST /api/v1/organizations/laravel/claim",
	}
	if len(requests) != len(want) {
		t.Fatalf("requests %q", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request %d is %q, want %q", i, requests[i], want[i])
		}
	}
}

func TestFollowedOrganizationsRejectAnInvalidPage(t *testing.T) {
	for _, body := range []string{`{"data":null}`, `{"data":[{"handle":"../x"}]}`, `{"data":[{"handle":"laravel"}],"meta":{"next_cursor":"same"}}`} {
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
		if _, err := client.FollowedOrganizations(t.Context(), "ada", "same"); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
