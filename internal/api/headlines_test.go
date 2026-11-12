package api

import (
	"io"
	"net/http"
	"testing"
)

const testHeadlineID = "0199a4c2-7b1e-7c3a-9f00-1234567890ab"

func TestHeadlinesReadTheHotTopicsWithTheirRecaps(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/headlines" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"`+testHeadlineID+`","name":"Laravel 13","summary":"People ship it."},{"id":"`+testHeadlineID+`","name":"PHP 9","summary":null}]}`)
	})
	topics, err := client.Headlines(t.Context())
	if err != nil || len(topics) != 2 || topics[0].Name != "Laravel 13" || topics[0].Summary == nil || *topics[0].Summary != "People ship it." || topics[1].Summary != nil {
		t.Fatalf("topics %+v, %v", topics, err)
	}
}

func TestHeadlineReadsTheWeeksCommitsWithTheNameAndRecap(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/headlines/"+testHeadlineID {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"c1","created_at":"2026-11-12T10:00:00Z","author":{"id":"u","handle":"ada"}}],"name":"Laravel 13","summary":"People ship it."}`)
	})
	page, err := client.Headline(t.Context(), testHeadlineID)
	if err != nil || len(page.Data) != 1 || page.Name != "Laravel 13" || page.Summary == nil {
		t.Fatalf("page %+v, %v", page, err)
	}
}

func TestHeadlinesRefuseInvalidTopicsAndIDs(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":[{"id":"x","name":"PHP"}]}`, `{"data":[{"id":"` + testHeadlineID + `","name":""}]}`} {
		client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
		if _, err := client.Headlines(t.Context()); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("sent a request for an invalid id: %s", r.URL)
	})
	if _, err := client.Headline(t.Context(), "../me"); err == nil {
		t.Fatal("accepted an invalid headline id")
	}
}
