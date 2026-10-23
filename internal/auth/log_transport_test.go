package auth

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONReadsSignExactCursorAndRejectForeignTargets(t *testing.T) {
	key, _ := GenerateKey()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Has("rate") {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"code":"rate_limited","message":"remote secret"}`)
			return
		}
		message := strings.Join([]string{r.Header.Get("X-Devnull-Timestamp"), r.Header.Get("X-Devnull-Nonce"), r.Method, r.RequestURI, ""}, "\n")
		verifySignature(t, key, r.Header.Get("X-SSH-Signature"), []byte(message))
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	var page struct{ Data []any }
	if err := client.GetJSON(t.Context(), server.URL+"/api/v1/log/latest?cursor=a%2Bb%3D", &page); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"https://foreign.example/api/v1/log/latest", "//foreign.example/api/v1/log/latest", "/settings", server.URL + "/api/v1/log/latest#fragment"} {
		if err := client.GetJSON(t.Context(), target, &page); err == nil {
			t.Fatalf("accepted unsafe target %q", target)
		}
	}
	ctx := t.Context()
	if requests != 1 {
		t.Fatalf("unsafe requests sent: %d", requests)
	}
	err := client.GetJSON(ctx, "/api/v1/log/latest?rate=1", &page)
	var failure *APIError
	if !errors.As(err, &failure) || failure.RetryAfter != "30" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("structured failure: %v", err)
	}
}
