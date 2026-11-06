package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSearchProfilesReadsPeopleAndOrganizations(t *testing.T) {
	var queries []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search/profiles" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		queries = append(queries, r.URL.Query().Get("q"))
		_, _ = io.WriteString(w, `{"users":[{"id":"u1","handle":"ada","name":"Ada","is_followed":true,"follows_you":false}],
			"organizations":[{"handle":"laravel","name":"Laravel","is_followed":false}]}`)
	})
	found, err := client.SearchProfiles(t.Context(), " ada ")
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Users) != 1 || found.Users[0].Handle != "ada" || found.Users[0].IsFollowed == nil || !*found.Users[0].IsFollowed {
		t.Fatalf("users: %+v", found.Users)
	}
	if len(found.Organizations) != 1 || found.Organizations[0].Handle != "laravel" {
		t.Fatalf("organizations: %+v", found.Organizations)
	}
	if len(queries) != 1 || queries[0] != "ada" {
		t.Fatalf("queries: %q", queries)
	}
	if _, err := client.SearchProfiles(t.Context(), strings.Repeat("a", 101)); err == nil {
		t.Fatal("accepted a term over 100 characters")
	}
	invalid := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"users":[{"handle":"../me"}],"organizations":[]}`)
	})
	if _, err := invalid.SearchProfiles(t.Context(), "ada"); err == nil {
		t.Fatal("accepted an invalid handle")
	}
}

func TestSearchCommitsSendsTheTermAndLimit(t *testing.T) {
	var uri string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		_, _ = io.WriteString(w, `{"data":[{"id":"c1","body":"caching","author":{"type":"user","id":"u1","handle":"ada"}}]}`)
	})
	commits, err := client.SearchCommits(t.Context(), "caching tips", 5)
	if err != nil || len(commits) != 1 || commits[0].ID != "c1" {
		t.Fatalf("commits: %+v %v", commits, err)
	}
	if uri != "/api/v1/search/commits?limit=5&q=caching+tips" {
		t.Fatalf("uri: %s", uri)
	}
	for _, term := range []string{"ab", " ab ", strings.Repeat("a", 101)} {
		if _, err := client.SearchCommits(t.Context(), term, 5); err == nil {
			t.Fatalf("accepted term %q", term)
		}
	}
	for _, limit := range []int{0, 101} {
		if _, err := client.SearchCommits(t.Context(), "caching", limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
}

func TestMentionSuggestionsScopeToTheEditor(t *testing.T) {
	var uris []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		uris = append(uris, r.URL.RequestURI())
		_, _ = io.WriteString(w, `{"data":[{"id":"u1","handle":"ada","name":"Ada"}],"organizations":[{"handle":"laravel","name":"Laravel"}]}`)
	})
	commit := "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
	org := "1b4e28ba-2fa1-11d2-883f-0016d3cca427"
	found, err := client.MentionSuggestions(t.Context(), MentionQuery{Term: "ad", Reply: commit})
	if err != nil || len(found.Users) != 1 || len(found.Organizations) != 1 {
		t.Fatalf("found: %+v %v", found, err)
	}
	if _, err := client.MentionSuggestions(t.Context(), MentionQuery{Org: org}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MentionSuggestions(t.Context(), MentionQuery{Quote: commit}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/api/v1/search/mentions?q=ad&reply=" + commit,
		"/api/v1/search/mentions?org=" + org + "&q=",
		"/api/v1/search/mentions?q=&quote=" + commit,
	}
	if strings.Join(uris, "\n") != strings.Join(want, "\n") {
		t.Fatalf("uris:\n%s", strings.Join(uris, "\n"))
	}
	for _, query := range []MentionQuery{
		{Org: org, Reply: commit},
		{Reply: commit, Quote: commit},
		{Org: "nope"},
		{Reply: "../x"},
		{Term: strings.Repeat("a", 101)},
	} {
		if _, err := client.MentionSuggestions(t.Context(), query); err == nil {
			t.Fatalf("accepted %+v", query)
		}
	}
}

func TestIssueSuggestionFindsOneIssueOrNone(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "42" {
			_, _ = io.WriteString(w, `{"issue":{"number":42,"title":"Dark mode","url":"https://github.com/nunomaduro/dev-null.sh/issues/42"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"issue":null}`)
	})
	issue, err := client.IssueSuggestion(t.Context(), "42")
	if err != nil || issue == nil || issue.Number != 42 || issue.Title != "Dark mode" {
		t.Fatalf("issue: %+v %v", issue, err)
	}
	none, err := client.IssueSuggestion(t.Context(), "7")
	if err != nil || none != nil {
		t.Fatalf("none: %+v %v", none, err)
	}
	for _, number := range []string{"0", "042", "12345678901", "4a"} {
		if _, err := client.IssueSuggestion(t.Context(), number); err == nil {
			t.Fatalf("accepted %q", number)
		}
	}
	wrong := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"issue":{"number":43,"title":"x","url":"https://github.com/x/y/issues/43"}}`)
	})
	if _, err := wrong.IssueSuggestion(t.Context(), "42"); err == nil {
		t.Fatal("accepted another issue")
	}
}
