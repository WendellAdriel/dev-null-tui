package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

const RedirectURI = "devnull://auth/callback"

var ErrDenied = errors.New("browser approval was canceled; try connecting again")

type Attempt struct {
	URL, State, Verifier string
	mu                   sync.Mutex
	spent                bool
	expires              time.Time
}

func NewAttempt(origin, fingerprint, device string) (*Attempt, error) {
	base, err := parseOrigin(origin)
	if err != nil {
		return nil, err
	}
	verifier := make([]byte, 32)
	if _, err := rand.Read(verifier); err != nil {
		return nil, err
	}
	state := make([]byte, 16)
	if _, err := rand.Read(state); err != nil {
		return nil, err
	}
	a := &Attempt{State: hex.EncodeToString(state), Verifier: base64.RawURLEncoding.EncodeToString(verifier), expires: time.Now().Add(10 * time.Minute)}
	challenge := sha256.Sum256([]byte(a.Verifier))
	base.Path = "/mobile/authorize"
	base.RawQuery = url.Values{"fingerprint": {fingerprint}, "device_name": {device}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "state": {a.State}, "redirect_uri": {RedirectURI}}.Encode()
	a.URL = base.String()
	return a, nil
}

func (a *Attempt) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.spent = true
	a.Verifier = ""
}

func (a *Attempt) Accept(raw string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accept(raw)
}

func (a *Attempt) grant(raw string) (string, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	code, err := a.accept(raw)
	return code, a.Verifier, err
}

func (a *Attempt) accept(raw string) (string, error) {
	if a.spent || time.Now().After(a.expires) {
		return "", errors.New("this connection attempt ended; start again")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || len(raw) > 2048 || strings.Contains(raw, "#") || u.Scheme != "devnull" || u.Host != "auth" || u.Path != "/callback" || u.RawPath != "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("paste the complete devnull://auth/callback link")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(a.State)) != 1 {
		return "", errors.New("callback does not match this connection attempt")
	}
	if len(q["code"])+len(q["error"]) != 1 || len(q) != 2 {
		return "", errors.New("callback must contain one code or error and its state")
	}
	if q.Get("error") != "" {
		a.spent = true
		if q.Get("error") == "access_denied" {
			return "", ErrDenied
		}
		return "", errors.New("browser authorization failed; try again")
	}
	code := q.Get("code")
	if code == "" || len(code) > 255 {
		return "", errors.New("callback has no valid authorization code")
	}
	a.spent = true
	return code, nil
}
