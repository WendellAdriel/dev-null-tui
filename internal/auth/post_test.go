package auth

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestJSONPostSignsTransmittedBytesAndAcceptsCreated(t *testing.T) {
	key, _ := GenerateKey()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != `{"body":"Synthetic 界\ntext"}` || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("wrong write: %s %s", r.Method, body)
		}
		message := strings.Join([]string{r.Header.Get("X-Devnull-Timestamp"), r.Header.Get("X-Devnull-Nonce"), r.Method, r.RequestURI, string(body)}, "\n")
		verifySignature(t, key, r.Header.Get("X-SSH-Signature"), []byte(message))
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"synthetic"}}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	var result struct{ Data struct{ ID string } }
	if err := client.PostJSON(t.Context(), "/api/v1/commits", []byte(`{"body":"Synthetic 界\ntext"}`), &result); err != nil || result.Data.ID != "synthetic" {
		t.Fatalf("created response: %+v %v", result, err)
	}
	if err := client.PostJSON(t.Context(), "https://foreign.example/api/v1/commits", nil, &result); err == nil || calls != 1 {
		t.Fatal("write escaped origin or replayed")
	}
}

func TestJSONPostReturnsOnlyBodyValidationMessages(t *testing.T) {
	key, _ := GenerateKey()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"message":"private draft","errors":{"body":["The body field is required."],"secret":["private value"]}}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	err := client.PostJSON(t.Context(), "/api/v1/commits", []byte(`{}`), new(any))
	var failure *APIError
	if !errors.As(err, &failure) || len(failure.BodyErrors) != 1 || failure.BodyErrors[0] != "The body field is required." || strings.Contains(err.Error(), "private") {
		t.Fatalf("validation boundary: %v", err)
	}
}

func TestJSONPostDoesNotReplayADroppedConnection(t *testing.T) {
	key, _ := GenerateKey()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	if err := client.PostJSON(t.Context(), "/api/v1/commits", []byte(`{"body":"Synthetic draft"}`), new(any)); err == nil || calls.Load() != 1 {
		t.Fatalf("uncertain write replayed: calls=%d err=%v", calls.Load(), err)
	}
}
