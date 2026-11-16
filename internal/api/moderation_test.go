package api

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestModerationHistoryReadsYourTimeoutsAndBans(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/me/moderation-history" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"timeouts_count":2,"bans_count":1,
			"timeouts":[{"id":"t2","reason":"Spam","duration":"1 hour","started_at":"2026-11-15T10:00:00Z","ends_at":"2026-11-15T11:00:00Z"}],
			"bans":[{"id":"b1","reason":"Harassment","banned_at":"2026-10-01T08:00:00Z"}]}}`)
	})
	history, err := client.ModerationHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if history.TimeoutsCount != 2 || history.BansCount != 1 || len(history.Timeouts) != 1 || len(history.Bans) != 1 {
		t.Fatalf("history: %+v", history)
	}
	timeout := history.Timeouts[0]
	if timeout.Reason != "Spam" || timeout.Duration != "1 hour" || !timeout.EndsAt.Equal(time.Date(2026, 11, 15, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("timeout: %+v", timeout)
	}
	if ban := history.Bans[0]; ban.Reason != "Harassment" || !ban.BannedAt.Equal(time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("ban: %+v", ban)
	}
}

func TestModerationHistoryRejectsMissingDataAndReportsForbidden(t *testing.T) {
	empty := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if _, err := empty.ModerationHistory(t.Context()); err == nil {
		t.Fatal("accepted a response without data")
	}
	forbidden := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Forbidden"}`)
	})
	_, err := forbidden.ModerationHistory(t.Context())
	var failure *auth.APIError
	if !errors.As(err, &failure) || failure.Status != http.StatusForbidden {
		t.Fatalf("forbidden: %v", err)
	}
}
