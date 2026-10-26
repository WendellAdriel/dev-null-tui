package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	key, _ := auth.GenerateKey()
	client, err := NewClient(server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestThreadReadsCommitAncestorsAndTombstones(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/commits/c1" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"c1","parent_id":"p2","body":"Focused","author":{"id":"u","username":"ada"},"replies_count":3},"ancestors":[{"id":"p1","parent_id":null,"deleted_at":"2026-10-26T10:00:00Z","replies_count":1},{"id":"p2","parent_id":"p1","body":"Parent","author":{"id":"u","username":"ada"}}]}`)
	})
	thread, err := client.Thread(t.Context(), "c1")
	if err != nil || thread.Commit.ID != "c1" || thread.Commit.ParentID == nil || *thread.Commit.ParentID != "p2" || len(thread.Ancestors) != 2 {
		t.Fatalf("thread: %+v %v", thread, err)
	}
	if tomb := thread.Ancestors[0]; tomb.DeletedAt == nil || tomb.Author != nil || tomb.Body != "" {
		t.Fatalf("tombstone leaked content: %+v", tomb)
	}
	if _, err := client.Thread(t.Context(), "../me"); err == nil {
		t.Fatal("accepted an unsafe commit id")
	}
}

func TestThreadRejectsCommitsWithoutAuthor(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"c1","body":"Anonymous"},"ancestors":[]}`)
	})
	if _, err := client.Thread(t.Context(), "c1"); err == nil {
		t.Fatal("accepted a live commit without an author")
	}
}

func TestRepliesFollowTheServerCursorAndRefuseRepeats(t *testing.T) {
	var cursors []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/commits/c1/replies" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		page := `{"data":[{"id":"r1","parent_id":"c1","body":"Reply","author":{"id":"u","username":"ada"}}],"meta":{"next_cursor":"abc"}}`
		if cursor == "abc" {
			page = `{"data":[{"id":"r2","parent_id":"c1","body":"Reply","author":{"id":"u","username":"ada"}}],"meta":{"next_cursor":null}}`
		}
		_, _ = io.WriteString(w, page)
	})
	first, err := client.Replies(t.Context(), "c1", "")
	if err != nil || len(first.Data) != 1 || first.Next != "abc" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := client.Replies(t.Context(), "c1", first.Next)
	if err != nil || second.Next != "" || len(cursors) != 2 || cursors[1] != "abc" {
		t.Fatalf("second page: %+v %v %v", second, err, cursors)
	}
	repeat := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"meta":{"next_cursor":"same"}}`)
	})
	if _, err := repeat.Replies(t.Context(), "c1", "same"); err == nil {
		t.Fatal("accepted a repeated cursor")
	}
}

func TestReplyAndQuotePayloadsAreMutuallyExclusive(t *testing.T) {
	var payloads []map[string]any
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		payloads = append(payloads, payload)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"new","body":"Hi","author":{"id":"u","username":"ada"},"status":"pending"}}`)
	})
	for _, draft := range []NewCommit{{Body: "Hi"}, {Body: "Hi", ParentID: "p"}, {Body: "Hi", QuotedID: "q"}} {
		if _, err := client.CreateCommit(t.Context(), draft); err != nil {
			t.Fatal(err)
		}
	}
	if len(payloads[0]) != 1 || payloads[1]["parent_id"] != "p" || payloads[1]["quoted_commit_id"] != nil || payloads[2]["quoted_commit_id"] != "q" || payloads[2]["parent_id"] != nil {
		t.Fatalf("payloads: %v", payloads)
	}
	if _, err := client.CreateCommit(t.Context(), NewCommit{Body: "Hi", ParentID: "p", QuotedID: "q"}); err == nil || len(payloads) != 3 {
		t.Fatal("sent a reply that is also a quote")
	}
}

func TestMissingReplyParentIsAnErrorNotATopLevelCommit(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not found"}`)
	})
	_, err := client.CreateCommit(t.Context(), NewCommit{Body: "Hi", ParentID: "gone"})
	var failure *auth.APIError
	if err == nil || !errors.As(err, &failure) || failure.Status != http.StatusNotFound {
		t.Fatalf("missing parent: %v", err)
	}
}

func TestDeleteCommitAcceptsNoContent(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/commits/c1" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := client.DeleteCommit(t.Context(), "c1"); err != nil {
		t.Fatal(err)
	}
	forbidden := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	if err := forbidden.DeleteCommit(t.Context(), "c1"); err == nil {
		t.Fatal("forbidden delete reported success")
	}
}
