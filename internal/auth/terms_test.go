package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTermsAcceptanceSignsTheDisplayedVersion(t *testing.T) {
	key, _ := GenerateKey()
	identity, err := os.ReadFile("testdata/current-user.json")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/me/terms" || string(body) != `{"version":"2026-10-25T00:00:00Z"}` {
			t.Errorf("unexpected terms request: %s %s %s", r.Method, r.URL.Path, body)
		}
		message := strings.Join([]string{r.Header.Get("X-Devnull-Timestamp"), r.Header.Get("X-Devnull-Nonce"), r.Method, r.RequestURI, string(body)}, "\n")
		verifySignature(t, key, r.Header.Get("X-SSH-Signature"), []byte(message))
		_, _ = w.Write(identity)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	user, err := client.AcceptTerms(t.Context(), "2026-10-25T00:00:00Z")
	if err != nil || !user.TermsAccepted || user.TermsVersion != "2026-10-25T00:00:00Z" || user.Handle != "fixture" {
		t.Fatalf("terms acceptance: %+v %v", user, err)
	}
	if _, err := client.AcceptTerms(t.Context(), ""); err == nil || requests != 1 {
		t.Fatal("empty terms version made a request")
	}
}

func TestRestoreHandleIdentityKeepsTheSavedKey(t *testing.T) {
	identity, err := os.ReadFile("testdata/current-user.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/me" {
			t.Errorf("restore must not enroll: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write(identity)
	}))
	defer server.Close()
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	key, _ := GenerateKey()
	if err := store.Save(key, server.URL); err != nil {
		t.Fatal(err)
	}
	restored, _, user, err := Restore(t.Context(), store, server.URL)
	if err != nil || user.Handle != "fixture" || restored.Fingerprint() != key.Fingerprint() {
		t.Fatalf("restore: %+v %v", user, err)
	}
	saved, _, err := store.Load()
	if err != nil || saved.Fingerprint() != key.Fingerprint() {
		t.Fatal("restore changed the saved key")
	}
}

func TestTermsAcceptanceRequiresConfirmationOfTheSameVersion(t *testing.T) {
	for _, response := range []string{
		`{"data":{"id":"fixture","handle":"fixture","terms_version":"2026-10-25T00:00:00Z","terms_accepted":false}}`,
		`{"data":{"id":"fixture","handle":"fixture","terms_version":"2026-10-26T00:00:00Z","terms_accepted":true}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, response)
		}))
		key, _ := GenerateKey()
		client, _ := NewClient(server.URL, key)
		if _, err := client.AcceptTerms(t.Context(), "2026-10-25T00:00:00Z"); err == nil {
			t.Fatal("unconfirmed consent accepted")
		}
		server.Close()
	}
}
