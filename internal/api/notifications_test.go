package api

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestNotificationsReadBoundaryFiltersAndGroupPagination(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") == "" || r.URL.Query().Get("type") != "replies,stars" {
			t.Errorf("unsigned or unfiltered request: %s", r.URL)
		}
		if r.URL.Path == "/api/v1/notifications/n1/group" {
			_, _ = io.WriteString(w, `{"data":[{"id":"n1","group_id":"g1","type":"starred","actor":{"id":"u","handle":"ada"},"created_at":"2026-10-27T10:00:00Z"}],"meta":{"next_cursor":null}}`)
			return
		}
		if r.URL.Path != "/api/v1/notifications" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"n1","group_id":"g1","type":"starred","actor":{"id":"u","handle":"ada"},"actors":[{"id":"u","handle":"ada"}],"event_count":3,"actors_count":2,"read_at":"2026-10-27T12:00:00Z","created_at":"2026-10-27T10:00:00Z","commit":{"id":"c1","author":{"handle":"owner"}}}],"read_at":"2026-10-27T12:00:00Z","meta":{"next_cursor":"next/+="}}`)
	})
	page, err := client.Notifications(t.Context(), "replies,stars", "")
	if err != nil || page.Next != "next/+=" || len(page.Data) != 1 || !page.Data[0].NewSince(page.ReadAt) || page.Data[0].Actor.Handle != "ada" {
		t.Fatalf("page: %+v %v", page, err)
	}
	if _, err := client.Notifications(t.Context(), "replies,stars", page.Next); err == nil {
		t.Fatal("accepted repeated cursor")
	}
	group, err := client.NotificationGroup(t.Context(), "n1", "replies,stars", "")
	if err != nil || len(group.Data) != 1 || group.Next != "" {
		t.Fatalf("group: %+v %v", group, err)
	}
	if _, err := client.NotificationGroup(t.Context(), "../me", "all", ""); err == nil {
		t.Fatal("unsafe group id")
	}
	if _, err := client.Notifications(t.Context(), "bogus", ""); err == nil {
		t.Fatal("invalid filter")
	}
}

func TestNotificationReadBoundaryIncludesJustAcknowledgedRows(t *testing.T) {
	boundary := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	before, after := boundary.Add(-time.Second), boundary.Add(time.Second)
	for _, test := range []struct {
		read *time.Time
		want bool
	}{{nil, true}, {&boundary, true}, {&before, false}, {&after, true}} {
		if got := (Notification{ReadAt: test.read}).NewSince(boundary); got != test.want {
			t.Fatalf("read_at %v: got %v", test.read, got)
		}
	}
}

func TestNotificationPagesRejectMalformedContractAndEncodeOpaqueCursors(t *testing.T) {
	for _, response := range []string{
		`{"data":[],"meta":{"next_cursor":null}}`,
		`{"data":null,"read_at":"2026-10-27T12:00:00Z"}`,
		`{"data":[{"id":"n","group_id":"g","actor":{"handle":"ada"}}],"read_at":"2026-10-27T12:00:00Z"}`,
	} {
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, response) })
		if _, err := client.Notifications(t.Context(), "all", ""); err == nil {
			t.Fatalf("accepted malformed page %s", response)
		}
	}
	cursor := "https://untrusted.test/?cursor=opaque&extra=1"
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/notifications/n/group" || r.URL.Query().Get("cursor") != cursor || r.URL.Query().Get("type") != "none" || len(r.URL.Query()) != 2 {
			t.Errorf("unsafe cursor request: %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"next_cursor":null}}`)
	})
	if _, err := client.NotificationGroup(t.Context(), "n", "none", cursor); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationCountUsesOnlyMeAndKeepsRemotesSeparate(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/me" || r.Method != "GET" {
			t.Errorf("count mutated inbox: %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"u","handle":"ada","remotes":{"unread_count":19}},"unread_notifications_count":4}`)
	})
	count, err := client.NotificationCount(t.Context())
	if err != nil || count != 4 {
		t.Fatalf("count %d: %v", count, err)
	}
	missing := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"remotes":{"unread_count":19}}}`)
	})
	if _, err := missing.NotificationCount(t.Context()); err == nil {
		t.Fatal("missing count treated as zero")
	}
}
