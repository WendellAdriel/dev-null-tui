package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestLogContractPaginationAndRankedCounts(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned log request")
		}
		if r.URL.Path == "/api/v1/log/trending" {
			if r.URL.Query().Has("since") {
				t.Error("ranked request included chronological since")
			}
			_, _ = io.WriteString(w, `{"log":"trending","is_ranked":true,"data":[],"links":{"next":null},"meta":{"next_cursor":null}}`)
			return
		}
		if r.URL.Query().Get("cursor") == "opaque+/=" {
			_, _ = io.WriteString(w, `{"log":"latest","is_ranked":false,"data":[],"links":{"next":null},"meta":{"next_cursor":null}}`)
			return
		}
		_, _ = io.WriteString(w, `{"log":"latest","is_ranked":false,"new_commits_count":2,"data":[{"id":"01900000-0000-7000-8000-000000000001","body":"Synthetic fixture","short_hash":"0000001","html_url":"https://example.test/@fixture/commit/1","created_at":"2026-10-23T10:00:00Z","status":"pending","author":{"id":"user-1","handle":"fixture","name":"Fixture","lane":4},"body_segments":[{"type":"text","text":"Synthetic fixture"}],"quoted_commit_id":"hidden","quoted_commit":null,"reply_preview":{"data":[],"next_cursor":null}}],"suggestions":[],"links":{"next":"/api/v1/log/latest?cursor=opaque%2B%2F%3D"},"meta":{"next_cursor":"opaque+/="}}`)
	}))
	defer server.Close()
	key, _ := auth.GenerateKey()
	client, err := NewClient(server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.Log(t.Context(), Latest, "", "seen")
	if err != nil || len(page.Data) != 1 || page.NewCount != 2 || page.Data[0].Author.Handle != "fixture" || string(page.Data[0].Author.Lane) != "4" || page.Data[0].Status != "pending" {
		t.Fatalf("contract decode: %+v %v", page, err)
	}
	last, err := client.Log(t.Context(), Latest, page.Next, "")
	if err != nil || last.Next != "" {
		t.Fatalf("exhaustion: %+v %v", last, err)
	}
	if _, err := client.Log(t.Context(), Trending, "", "seen"); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestPaginationNeverLeavesOriginOrSelectedLog(t *testing.T) {
	for _, next := range []string{"https://foreign.example/api/v1/log/latest?cursor=x", "//foreign.example/api/v1/log/latest?cursor=x", "/api/v1/log/following?cursor=x", "/api/v1/me", "/api/v1/log/latest?cursor=x#bad"} {
		t.Run(next, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"log": "latest", "data": []any{}, "links": map[string]any{"next": next}})
			}))
			defer server.Close()
			key, _ := auth.GenerateKey()
			client, _ := NewClient(server.URL, key)
			if _, err := client.Log(t.Context(), Latest, "", ""); err == nil || strings.Contains(err.Error(), "foreign.example") {
				t.Fatalf("unsafe next accepted or echoed: %v", err)
			}
		})
	}
}
