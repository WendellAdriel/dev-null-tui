package api

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestStarAndPastelRequestsUseSignedCommitRoutesAndReturnedCounts(t *testing.T) {
	var calls []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned action")
		}
		body := `{"data":{"id":"c1","author":{"id":"u","handle":"ada"},"stars_count":3,"starred":false,"pasteis_count":2},"pasteis_remaining":8}`
		if r.Method == http.MethodPost {
			body = `{"data":{"id":"c1","author":{"id":"u","handle":"ada"},"stars_count":3,"starred":true,"pasteis_count":2},"pasteis_remaining":8}`
		}
		_, _ = io.WriteString(w, body)
	})
	for _, want := range []bool{true, false} {
		commit, err := client.SetStar(t.Context(), "c1", want)
		if err != nil || commit.Starred == nil || *commit.Starred != want || commit.Stars != 3 {
			t.Fatalf("star receipt: %+v %v", commit, err)
		}
	}
	receipt, err := client.GivePastel(t.Context(), "c1")
	if err != nil || receipt.Remaining == nil || *receipt.Remaining != 8 || receipt.Commit.Pasteis == nil || *receipt.Commit.Pasteis != 2 {
		t.Fatalf("pastel receipt: %+v %v", receipt, err)
	}
	if fmt.Sprint(calls) != "[POST /api/v1/commits/c1/star DELETE /api/v1/commits/c1/star POST /api/v1/commits/c1/pasteis]" {
		t.Fatal(calls)
	}
	if _, err := client.SetStar(t.Context(), "../me", true); err == nil {
		t.Fatal("unsafe star id")
	}
	if _, err := client.GivePastel(t.Context(), "../me"); err == nil {
		t.Fatal("unsafe pastel id")
	}
	malformed := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"wrong","author":{"handle":"ada"}}}`)
	})
	if _, err := malformed.SetStar(t.Context(), "c1", true); err == nil {
		t.Fatal("invalid star receipt accepted")
	}
	if _, err := malformed.GivePastel(t.Context(), "c1"); err == nil {
		t.Fatal("invalid pastel receipt accepted")
	}
}
