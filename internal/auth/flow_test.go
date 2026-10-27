package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCanceledFlowDeletesOnlyUnsubmittedKey(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	flow, err := Begin(t.Context(), store, DefaultOrigin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(); !os.IsNotExist(err) {
		t.Fatal("unsubmitted key survived cancellation")
	}
	key, _ := GenerateKey()
	if err := store.Save(key, DefaultOrigin); err != nil {
		t.Fatal(err)
	}
	flow, err = Begin(t.Context(), store, DefaultOrigin, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.Close(); err != nil {
		t.Fatal(err)
	}
	if loaded, _, err := store.Load(); err != nil || loaded.Fingerprint() != key.Fingerprint() {
		t.Fatal("cancel deleted previously stored key")
	}
}

func TestFlowVerifiesIdentityAndPreservesKeyAfterUncertainEnrollment(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			w.WriteHeader(status)
			if status == http.StatusOK {
				_, _ = w.Write([]byte(`{"data":{"id":"01900000-0000-7000-8000-000000000001","handle":"fixture","name":"Synthetic"}}`))
			} else {
				_, _ = w.Write([]byte(`{"code":"invalid_grant"}`))
			}
		}))
		store := Store{Dir: filepath.Join(t.TempDir(), "private")}
		if err := store.Prepare(); err != nil {
			t.Fatal(err)
		}
		flow, err := Begin(t.Context(), store, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		user, err := flow.Redeem(t.Context(), RedirectURI+"?code=synthetic-code&state="+flow.Attempt.State)
		if status == http.StatusOK && (err != nil || user.Handle != "fixture" || requests != 2) {
			t.Fatalf("enrollment did not verify /me: %v, requests=%d", err, requests)
		}
		if status != http.StatusOK && (err == nil || requests != 1) {
			t.Fatal("invalid grant established a connection")
		}
		if err := flow.Close(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Load(); err != nil {
			t.Fatal("potentially enrolled key was lost")
		}
		server.Close()
	}
}

func TestExpiredAttemptAndCanceledRequestCannotConnect(t *testing.T) {
	a, _ := NewAttempt(DefaultOrigin, "SHA256:example", "Test device")
	a.expires = time.Now().Add(-time.Second)
	if _, err := a.Accept(RedirectURI + "?code=x&state=" + a.State); err == nil {
		t.Fatal("expired attempt accepted")
	}
	key, _ := GenerateKey()
	client, _ := NewClient(DefaultOrigin, key)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Me(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled request was not canceled")
	}
}

func TestRestoreDoesNotSendCredentialsToAnUnselectedServer(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	key, _ := GenerateKey()
	if err := store.Save(key, server.URL); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Restore(t.Context(), store, DefaultOrigin); err == nil || requests != 0 {
		t.Fatal("restore contacted an unselected server")
	}
}
