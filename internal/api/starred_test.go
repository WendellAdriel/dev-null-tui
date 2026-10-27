package api

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestStarredUsesSignedCursorPagesInServerOrder(t *testing.T) {
	var cursors []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/starred" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		if cursor == "" {
			_, _ = io.WriteString(w, `{"data":[{"id":"z","created_at":"2026-10-27T10:00:00Z","author":{"id":"u","handle":"ada"},"starred":true},{"id":"a","created_at":"2026-10-27T10:00:00Z","author":{"id":"u","handle":"ada"},"starred":true}],"meta":{"next_cursor":"equal+time/="}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"next_cursor":null}}`)
	})
	first, err := client.Starred(t.Context(), "")
	if err != nil || len(first.Data) != 2 || first.Data[0].ID != "z" || first.Data[1].ID != "a" || first.Next != "equal+time/=" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	last, err := client.Starred(t.Context(), first.Next)
	if err != nil || last.Next != "" || fmt.Sprint(cursors) != "[ equal+time/=]" {
		t.Fatalf("last page: %+v %v, cursors %v", last, err, cursors)
	}
}

func TestStarredRejectsInvalidPagesAndRepeatedCursors(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":[{"id":"x"}]}`, `{"data":[],"meta":{"next_cursor":"same"}}`} {
		t.Run(body, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := client.Starred(t.Context(), "same"); err == nil {
				t.Fatal("accepted an invalid page")
			}
		})
	}
}
