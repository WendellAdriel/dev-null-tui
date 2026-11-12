package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestListsWithReplyPreviewsSendRepliesPerPage(t *testing.T) {
	seen := map[string]string{}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if r.URL.Query().Has("cursor") {
			key += " next"
		}
		if r.URL.Query().Has("since") {
			key += " since"
		}
		seen[key] = r.URL.Query().Get("replies_per_page")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/log/"):
			var next any
			if !r.URL.Query().Has("cursor") {
				next = "http://" + r.Host + r.URL.Path + "?cursor=c2"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"log": "latest", "is_ranked": false, "data": []any{}, "links": map[string]any{"next": next}, "meta": map[string]any{"next_cursor": nil}})
		case r.URL.Path == "/api/v1/users/ada":
			_, _ = io.WriteString(w, `{"data":{"id":"u","handle":"ada"}}`)
		default:
			_, _ = io.WriteString(w, `{"data":[],"meta":{"next_cursor":null}}`)
		}
	})
	client.SetRepliesPerPage(10)
	ctx := t.Context()
	page, err := client.Log(ctx, Latest, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error { _, err := client.Log(ctx, Latest, page.Next, ""); return err },
		func() error { _, err := client.Log(ctx, Latest, "", "2026-11-12T10:00:00Z"); return err },
		func() error { _, err := client.UserCommits(ctx, "ada", false, "c2"); return err },
		func() error { _, err := client.OrganizationCommits(ctx, "acme", true, ""); return err },
		func() error { _, err := client.OrganizationCommits(ctx, "acme", false, ""); return err },
		func() error { _, err := client.Starred(ctx, ""); return err },
		func() error { _, err := client.Profile(ctx, "ada"); return err },
		func() error { _, err := client.Replies(ctx, "c1", ""); return err },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{
		"/api/v1/log/latest":                  "10",
		"/api/v1/log/latest next":             "10",
		"/api/v1/log/latest since":            "10",
		"/api/v1/users/ada/commits next":      "10",
		"/api/v1/organizations/acme/mentions": "10",
		"/api/v1/organizations/acme/commits":  "10",
		"/api/v1/starred":                     "10",
		"/api/v1/users/ada":                   "10",
		"/api/v1/commits/c1/replies":          "",
	}
	for path, count := range want {
		if got, ok := seen[path]; !ok || got != count {
			t.Errorf("%s sent replies_per_page %q (requested: %v), want %q", path, got, ok, count)
		}
	}
}

func TestRepliesPerPageIsLeftOutUntilSet(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("replies_per_page") {
			t.Errorf("sent replies_per_page before it was set: %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"next_cursor":null}}`)
	})
	if _, err := client.Starred(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
}
