package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestCommitExtrasUseTheirRoutesAndCheckReceipts(t *testing.T) {
	var calls []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/v1/commits/c1/related":
			_, _ = io.WriteString(w, `{"data":[{"id":"c2","author":{"id":"u","handle":"ada"}}]}`)
		case "/api/v1/commits/c1/stars":
			_, _ = io.WriteString(w, `{"data":[{"id":"u","handle":"ada"}],"meta":{"next_cursor":"n2"}}`)
		case "/api/v1/commits/c1/pin":
			body := `{"data":{"id":"c1","author":{"id":"u","handle":"ada"},"is_pinned":false}}`
			if r.Method == http.MethodPut {
				body = `{"data":{"id":"c1","author":{"id":"u","handle":"ada"},"is_pinned":true}}`
			}
			_, _ = io.WriteString(w, body)
		case "/api/v1/commits/c1/restoration":
			_, _ = io.WriteString(w, `{"data":{"id":"c1","author":{"id":"u","handle":"ada"},"deleted_at":null}}`)
		}
	})
	related, err := client.RelatedCommits(t.Context(), "c1")
	if err != nil || len(related) != 1 || related[0].ID != "c2" {
		t.Fatalf("related: %+v %v", related, err)
	}
	people, err := client.Stargazers(t.Context(), "c1", "")
	if err != nil || len(people.Data) != 1 || people.Next != "n2" {
		t.Fatalf("stargazers: %+v %v", people, err)
	}
	for _, want := range []bool{true, false} {
		commit, err := client.SetPin(t.Context(), "c1", want)
		if err != nil || commit.Pinned == nil || *commit.Pinned != want {
			t.Fatalf("pin %v: %+v %v", want, commit, err)
		}
	}
	if _, err := client.RestoreCommit(t.Context(), "c1"); err != nil {
		t.Fatal(err)
	}
	want := "[GET /api/v1/commits/c1/related GET /api/v1/commits/c1/stars PUT /api/v1/commits/c1/pin DELETE /api/v1/commits/c1/pin POST /api/v1/commits/c1/restoration]"
	if fmt.Sprint(calls) != want {
		t.Fatal(calls)
	}
}

func TestPinAndRelatedRejectMismatchedResponses(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/commits/c1/related" {
			_, _ = io.WriteString(w, `{"data":[{"id":"c1","author":{"id":"u","handle":"ada"}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"c1","author":{"id":"u","handle":"ada"},"is_pinned":false}}`)
	})
	if _, err := client.SetPin(t.Context(), "c1", true); err == nil {
		t.Fatal("unconfirmed pin accepted")
	}
	if _, err := client.RelatedCommits(t.Context(), "c1"); err == nil {
		t.Fatal("the commit itself accepted as related")
	}
}

func TestTranslateSendsTheLanguageAndReportsPendingAndUnavailable(t *testing.T) {
	responses := []string{
		`{"translation":{"language":"pt","source_language":"en","source_name":"English","available":true,"translated":null}}`,
		`{"translation":{"language":"pt","source_language":"en","source_name":"English","available":true,"translated":{"body_segments":[{"type":"text","text":"olá"}],"preview_segments":null}}}`,
		`{"translation":null}`,
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Language string }
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/commits/c1/translation" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Language != "pt" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, responses[0])
		responses = responses[1:]
	})
	pending, err := client.Translate(t.Context(), "c1", "pt")
	if err != nil || pending.Translated != nil || pending.SourceName != "English" {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	done, err := client.Translate(t.Context(), "c1", "pt")
	if err != nil || done.Translated == nil || done.Translated.Segments[0].Text != "olá" {
		t.Fatalf("done: %+v %v", done, err)
	}
	if _, err := client.Translate(t.Context(), "c1", "pt"); !errors.Is(err, ErrNoTranslation) {
		t.Fatalf("unavailable: %v", err)
	}
}

func TestScheduleCommitSendsUTCTimeAndRefusesRepliesAndFarTimes(t *testing.T) {
	var sent map[string]any
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/scheduled-commits" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a11","author":{"type":"organization","id":"o","handle":"laravel"},"body":"hi","scheduled_at":"2030-01-01T09:00:00+00:00","submitted_at":null,"status":"scheduled","image_count":0}}`)
	})
	at := time.Now().Add(2 * time.Hour).Truncate(time.Minute).In(time.FixedZone("WET+1", 3600))
	scheduled, err := client.ScheduleCommit(t.Context(), NewCommit{Body: "hi", Author: "laravel"}, at)
	if err != nil || scheduled.Author == nil || !scheduled.Author.Organization() {
		t.Fatalf("schedule: %+v %v", scheduled, err)
	}
	if sent["scheduled_at"] != at.UTC().Format("2006-01-02T15:04:05+00:00") || sent["author"] != "laravel" || sent["body"] != "hi" {
		t.Fatalf("payload: %v", sent)
	}
	if _, err := client.ScheduleCommit(t.Context(), NewCommit{Body: "hi", ParentID: "c1"}, at); err == nil {
		t.Fatal("scheduled reply accepted")
	}
	if _, err := client.ScheduleCommit(t.Context(), NewCommit{Body: "hi"}, time.Now().AddDate(0, 0, MaxScheduleDays+1)); err == nil {
		t.Fatal("time past the window accepted")
	}
}

func TestScheduledListActionsAndConflict(t *testing.T) {
	const id = "0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a11"
	var calls []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"data":[{"id":"`+id+`","body":"hi","scheduled_at":"2030-01-01T09:00:00+00:00","submitted_at":null,"status":"scheduled","image_count":2}],"meta":{"current_page":2,"last_page":3}}`)
		case http.MethodPost:
			_, _ = io.WriteString(w, `{"data":{"id":"`+id+`","body":null,"scheduled_at":"2030-01-01T09:00:00+00:00","submitted_at":"2029-12-31T10:00:00+00:00","status":"pending","image_count":0}}`)
		default:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"message":"This commit was already submitted or canceled. Refresh to see its status."}`)
		}
	})
	page, err := client.ScheduledCommits(t.Context(), 2)
	if err != nil || len(page.Data) != 1 || !page.More() || page.Data[0].ImageCount != 2 {
		t.Fatalf("list: %+v %v", page, err)
	}
	submitted, err := client.SubmitScheduled(t.Context(), id)
	if err != nil || submitted.Status != "pending" {
		t.Fatalf("submit: %+v %v", submitted, err)
	}
	var apiErr *auth.APIError
	if err := client.CancelScheduled(t.Context(), id); !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Message == "" {
		t.Fatalf("cancel conflict: %v", err)
	}
	if err := client.CancelScheduled(t.Context(), "../me"); err == nil {
		t.Fatal("unsafe id accepted")
	}
	want := "[GET /api/v1/scheduled-commits?page=2 POST /api/v1/scheduled-commits/" + id + "/submission DELETE /api/v1/scheduled-commits/" + id + "]"
	if fmt.Sprint(calls) != want {
		t.Fatal(calls)
	}
}
