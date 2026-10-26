package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestCreateAndPreviewTextContracts(t *testing.T) {
	for _, status := range []string{"pending", "approved", "rejected"} {
		t.Run(status, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"body":"Synthetic draft"}` || r.Header.Get("X-SSH-Signature") == "" {
					t.Errorf("bad text payload: %s", body)
				}
				if r.URL.Path == "/api/v1/commits/preview" {
					_, _ = io.WriteString(w, `{"data":{"body":"Synthetic draft","body_segments":[{"type":"text","text":"Synthetic draft"}],"preview_segments":null,"mentions":[]}}`)
					return
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"data":{"id":"synthetic","body":"Synthetic draft","author":{"id":"fixture","username":"fixture"},"status":"`+status+`"}}`)
			}))
			defer server.Close()
			key, _ := auth.GenerateKey()
			client, _ := NewClient(server.URL, key)
			preview, err := client.Preview(t.Context(), "Synthetic draft")
			if err != nil || preview.Body != "Synthetic draft" || len(preview.Segments) != 1 {
				t.Fatalf("preview: %+v %v", preview, err)
			}
			commit, err := client.CreateCommit(t.Context(), NewCommit{Body: "Synthetic draft"})
			if err != nil || commit.Status != status || calls != 2 {
				t.Fatalf("submission: %+v %v calls=%d", commit, err, calls)
			}
		})
	}
}

func TestBodyBudgetMatchesServerProseAndCodeRules(t *testing.T) {
	for _, tc := range []struct {
		body  string
		count int
	}{
		{"  界👩🏽‍💻\r\n\r\n\r\nshipped  ", 14},
		{"https://example.com/" + strings.Repeat("a", 80), 23},
		{"example.com/" + strings.Repeat("a", 80) + ".", 24},
		{"`https://example.com/" + strings.Repeat("a", 80) + "`", 102},
		{"a\n\n\n\nb", 4},
		{"```go\na\n\n\n\nb\n```", 16},
	} {
		if got := BodyCharacterCount(tc.body); got != tc.count {
			t.Errorf("budget %q: got %d want %d", tc.body, got, tc.count)
		}
	}
	if ValidateBody(strings.Repeat("a", 4001)) == nil || ValidateBody(" \n ") == nil || ValidateBody("https://example.com/"+strings.Repeat("a", 2048)) == nil {
		t.Fatal("accepted invalid body")
	}
}

func TestBodyBudgetKeepsNonURLsLiteralAndAcceptsLongLinkDrafts(t *testing.T) {
	for _, body := range []string{"nativephp.com.123", "socket.io.js", "array.at(-1)", "example.Com", "café.com", "foo_https://example.com/long/path", "mail@nativephp.com/a/long/path"} {
		if got := BodyCharacterCount(body); got != utf8.RuneCountInString(body) {
			t.Errorf("non-URL %q counted as %d", body, got)
		}
	}
	body := strings.Repeat("https://example.com/"+strings.Repeat("a", 1380)+" ", 3)
	if err := ValidateBody(body); err != nil {
		t.Fatalf("valid draft over 4000 raw characters rejected: %v", err)
	}
	if err := ValidateBody(strings.Repeat(body, 20)); err == nil {
		t.Fatal("raw length guard missing")
	}
}

func TestBodyBudgetMatchesBackendUnicodeWhitespace(t *testing.T) {
	for _, space := range []string{"\u00a0", "\u2003", "\u2028", "\u2029", "\u0085"} {
		body := "https://example.com/long-path-here" + space + strings.Repeat("x", 100)
		if got := BodyCharacterCount(body); got != 124 {
			t.Errorf("delimiter %U: got %d want 124", []rune(space)[0], got)
		}
	}
	for _, space := range []string{"\u00a0", "\u200b", "\u034f", "\u2060", "\ufeff", "\U0001d159"} {
		if err := ValidateBody(space + strings.Repeat("x", 4000) + space); err != nil {
			t.Errorf("trim %U: %v", []rune(space)[0], err)
		}
	}
}
