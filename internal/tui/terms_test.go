package tui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestTermsConsentBlocksWorkspaceUntilExplicitAcceptance(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/me/terms" || string(body) != `{"version":"2026-10-25T00:00:00Z"}` {
			t.Errorf("unexpected terms request: %s %s %s", r.Method, r.URL.Path, body)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"fixture","handle":"fixture","terms_version":"2026-10-25T00:00:00Z","terms_accepted":true}}`)
	}))
	defer server.Close()
	m := NewApp("dark", auth.Store{}, server.URL, t.Context())
	defer func() { _ = m.Close() }()
	key, _ := auth.GenerateKey()
	user := auth.User{ID: "fixture", Handle: "fixture", TermsVersion: "2026-10-25T00:00:00Z"}
	next, cmd := m.Update(accountResult{key: key, user: user})
	m = next.(AppModel)
	if m.connected || m.account.state != "terms" || cmd != nil || requests != 0 {
		t.Fatal("unaccepted terms opened the workspace or wrote consent")
	}
	if m.account.actions()[0].key == "a" {
		t.Fatal("acceptance must not be the default action")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(AppModel)
	if cmd == nil || m.connected || m.account.state != "accepting-terms" || requests != 0 {
		t.Fatal("acceptance did not wait for the server")
	}
	if _, duplicate := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"}); duplicate != nil {
		t.Fatal("acceptance can be submitted twice")
	}
	next, _ = m.Update(cmd())
	m = next.(AppModel)
	if !m.connected || !m.account.user.TermsAccepted || requests != 1 {
		t.Fatalf("explicit acceptance did not connect: %s, %d requests", m.account.state, requests)
	}
}

func TestTermsFailureAndStaleResultsCannotGrantConsent(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	m.user = auth.User{ID: "fixture", Handle: "fixture", TermsVersion: "2026-10-25T00:00:00Z"}
	m.key, _ = auth.GenerateKey()
	m.state, m.generation = "accepting-terms", 3
	next, _ := m.Update(termsResult{generation: 2, user: auth.User{ID: "fixture", TermsAccepted: true}})
	m = next.(AccountModel)
	if m.state != "accepting-terms" || m.user.TermsAccepted {
		t.Fatal("stale result granted consent")
	}
	next, _ = m.Update(termsResult{generation: 3, err: &auth.APIError{Status: 422}})
	m = next.(AccountModel)
	if m.state != "terms" || m.user.TermsAccepted || m.user.ID != "fixture" || m.status == "" {
		t.Fatal("failed acceptance lost identity or granted consent")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if next.(AccountModel).state != "verifying" || cmd == nil {
		t.Fatal("terms error has no refresh action")
	}
}

func TestTermsScreenKeepsConsentAndPolicyLinksVisible(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}} {
			m := NewAccount(theme, auth.Store{}, auth.DefaultOrigin, context.Background())
			m.state = "terms"
			m.user.TermsVersion = "2026-10-25T00:00:00Z"
			for _, status := range []string{"", "Account request failed. Verify to refresh the terms version."} {
				m.status = status
				next, _ := m.Update(size)
				content := ansi.Strip(next.View().Content)
				for _, label := range []string{"t  Terms", "p  Privacy", "a  Agree", "2026-10-25", "/terms", "/privacy"} {
					if !strings.Contains(content, label) {
						t.Fatalf("%s missing at %dx%d (%s):\n%s", label, size.Width, size.Height, theme, content)
					}
				}
				account := next.(AccountModel)
				screen, areas := account.accountScreen(colors(account.dark))
				found := false
				for _, area := range areas {
					if area.action == "account" && area.index == 3 {
						rows := strings.Split(ansi.Strip(screen), "\n")
						if area.y < 0 || area.y >= len(rows) || !strings.Contains(rows[area.y], "a  Agree") {
							t.Fatalf("consent hit area does not match its button at %dx%d", size.Width, size.Height)
						}
						found = true
					}
				}
				if !found {
					t.Fatalf("consent mouse action hidden at %dx%d", size.Width, size.Height)
				}
			}
			_ = m.Close()
		}
	}
}

func TestHiddenTermsScreenCannotAccept(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	m.state, m.width, m.height = "terms", 40, 12
	m.key, _ = auth.GenerateKey()
	m.user = auth.User{ID: "fixture", Handle: "fixture", TermsVersion: "2026-10-25T00:00:00Z"}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if next.(AccountModel).state != "terms" || cmd != nil {
		t.Fatal("unseen consent screen accepted terms")
	}
}
