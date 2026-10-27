package auth

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestAuthorizationCallbackIsBoundToAttempt(t *testing.T) {
	for _, suffix := range []string{
		"?code=example&state=wrong", "?code=example", "?state=STATE",
		"?code=one&code=two&state=STATE", "?code=one&state=STATE&state=STATE",
		"?code=one&error=access_denied&state=STATE", "?code=one&state=STATE#fragment",
		"?code=one&state=STATE#",
	} {
		a, err := NewAttempt("https://dev-null.sh", "SHA256:example", "Test device")
		if err != nil {
			t.Fatal(err)
		}
		callback := strings.ReplaceAll(RedirectURI+suffix, "STATE", a.State)
		if _, err := a.Accept(callback); err == nil {
			t.Fatalf("accepted invalid callback %q", suffix)
		}
	}
	a, err := NewAttempt("https://dev-null.sh", "SHA256:example", "Test device")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(a.URL)
	hash := sha256.Sum256([]byte(a.Verifier))
	if u.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(hash[:]) {
		t.Fatal("incorrect PKCE challenge")
	}
	for _, callback := range []string{"bare-code", "https://auth/callback?code=x&state=" + a.State, "devnull://other/callback?code=x&state=" + a.State} {
		if _, err := a.Accept(callback); err == nil {
			t.Fatal("accepted wrong callback destination")
		}
	}
	callback := RedirectURI + "?code=synthetic-code&state=" + a.State
	if code, err := a.Accept(callback); err != nil || code != "synthetic-code" {
		t.Fatal("valid callback rejected")
	}
	if _, err := a.Accept(callback); err == nil {
		t.Fatal("callback replay accepted")
	}
	denied, _ := NewAttempt("https://dev-null.sh", "SHA256:example", "Test device")
	if _, err := denied.Accept(RedirectURI + "?error=access_denied&state=" + denied.State); !errors.Is(err, ErrDenied) {
		t.Fatal("denial not recognized")
	}
	canceled, _ := NewAttempt("https://dev-null.sh", "SHA256:example", "Test device")
	canceled.Cancel()
	if _, err := canceled.Accept(RedirectURI + "?code=x&state=" + canceled.State); err == nil {
		t.Fatal("canceled attempt accepted")
	}
}

func TestSignedEnrollmentAndIdentityUseTransmittedBytes(t *testing.T) {
	identity, err := os.ReadFile("testdata/current-user.json")
	if err != nil {
		t.Fatal(err)
	}
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		message := strings.Join([]string{r.Header.Get("X-Devnull-Timestamp"), r.Header.Get("X-Devnull-Nonce"), r.Method, r.RequestURI, string(body)}, "\n")
		verifySignature(t, key, r.Header.Get("X-SSH-Signature"), []byte(message))
		if r.URL.Path == "/api/v1/devices" && string(body) != `{"code":"synthetic-code","code_verifier":"synthetic-verifier"}` {
			t.Errorf("unexpected enrollment body %q", body)
		}
		if r.Method == http.MethodGet && len(body) != 0 {
			t.Error("GET body not empty")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(identity)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	if user, err := client.Enroll(t.Context(), "synthetic-code", "synthetic-verifier"); err != nil || user.Handle != "fixture" || user.ID != "01900000-0000-7000-8000-000000000001" {
		t.Fatalf("enrollment: %v", err)
	}
	if _, err := client.Me(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.request(t.Context(), "POST", "/api/v1/me?x=%20y&x=two", []byte("{\"body\":\"λ\"}\n\n")); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("made %d requests", requests)
	}
}

func verifySignature(t *testing.T, key *Key, signature string, message []byte) {
	t.Helper()
	blob, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(blob) < 6 || string(blob[:6]) != "SSHSIG" {
		t.Fatal("invalid SSHSIG encoding")
	}
	var envelope struct {
		Version   uint32
		PublicKey []byte
		Namespace string
		Reserved  string
		Hash      string
		Signature []byte
	}
	if err := ssh.Unmarshal(blob[6:], &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Version != 1 || envelope.Namespace != "devnull-api" || envelope.Hash != "sha512" || envelope.Reserved != "" {
		t.Fatal("incorrect SSHSIG envelope")
	}
	public, err := ssh.ParsePublicKey(envelope.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if ssh.FingerprintSHA256(public) != key.Fingerprint() {
		t.Fatal("incorrect signing key")
	}
	var sig ssh.Signature
	if err := ssh.Unmarshal(envelope.Signature, &sig); err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512(message)
	signed := append([]byte("SSHSIG"), ssh.Marshal(struct {
		Namespace, Reserved, Hash string
		Digest                    []byte
	}{"devnull-api", "", "sha512", hash[:]})...)
	if err := public.Verify(signed, &sig); err != nil {
		t.Fatal("signature did not cover transmitted bytes")
	}
}

func TestClientDoesNotFollowRedirectsOrRetryRevokedKeys(t *testing.T) {
	key, _ := GenerateKey()
	for _, status := range []int{http.StatusFound, http.StatusUnauthorized} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			w.Header().Set("Location", "/unexpected")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"code":"device_not_approved","message":"unsafe remote text"}`)
		}))
		client, _ := NewClient(server.URL, key)
		_, err := client.Me(context.Background())
		server.Close()
		if err == nil || requests != 1 || strings.Contains(err.Error(), "unsafe remote text") {
			t.Fatalf("unsafe failure behavior: %v, requests=%d", err, requests)
		}
	}
}

func TestMalformedIdentityCannotEstablishAnAccount(t *testing.T) {
	key, _ := GenerateKey()
	for _, body := range []string{
		`{"data":{"id":1,"handle":"fixture"}}`,
		`{"data":{"id":null,"handle":"fixture"}}`,
		`{"data":{"id":"","handle":"fixture"}}`,
		`{"data":{"id":"01900000-0000-7000-8000-000000000001"}}`,
		`{"unexpected":"synthetic-do-not-log"}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		client, _ := NewClient(server.URL, key)
		_, err := client.Me(t.Context())
		server.Close()
		if err == nil || strings.Contains(err.Error(), "synthetic-do-not-log") {
			t.Fatal("malformed account response was accepted or disclosed")
		}
	}
}
