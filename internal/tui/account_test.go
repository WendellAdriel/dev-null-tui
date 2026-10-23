package tui

import (
	"context"
	"errors"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestAccountStatesFitAndKeepActionsVisible(t *testing.T) {
	key, _ := auth.GenerateKey()
	attempt, _ := auth.NewAttempt(auth.DefaultOrigin, key.Fingerprint(), "dev-null-cli")
	for _, state := range []string{"idle", "waiting", "verifying", "connected", "failed", "confirm"} {
		for _, theme := range []string{"light", "dark"} {
			for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}} {
				m := NewAccount(theme, auth.Store{}, auth.DefaultOrigin, context.Background())
				m.state, m.key = state, key
				m.flow = &auth.Flow{Key: key, Attempt: attempt}
				m.user = auth.User{ID: "01900000-0000-7000-8000-000000000001", Name: "Synthetic Developer", Username: "fixture"}
				updated, _ := m.Update(size)
				view := updated.View()
				if lipgloss.Width(view.Content) > size.Width || lipgloss.Height(view.Content) > size.Height {
					t.Fatalf("%s exceeds %dx%d", state, size.Width, size.Height)
				}
				if !strings.Contains(view.Content, "ctrl+c") {
					t.Fatal("exit hint hidden")
				}
				if state == "waiting" && !strings.Contains(view.Content, key.Fingerprint()) {
					t.Fatal("device fingerprint hidden")
				}
				if state == "connected" && !strings.Contains(view.Content, "@fixture") {
					t.Fatal("connected identity missing")
				}
				if state == "waiting" && !strings.Contains(ansi.Strip(view.Content), "Paste") {
					t.Fatal("callback recovery action hidden")
				}
				if strings.Contains(ansi.Strip(view.Content), "Device key: unencrypted") {
					t.Fatal("removed storage footer is still visible")
				}
			}
		}
	}
}

func TestAccountIgnoresStaleResultsAndTypingOwnsKeys(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, context.Background())
	m.generation = 2
	updated, _ := m.Update(accountResult{generation: 1, user: auth.User{ID: "01900000-0000-7000-8000-000000000001", Username: "wrong"}})
	m = updated.(AccountModel)
	if m.user.ID != "" {
		t.Fatal("stale request established an account")
	}
	m.state, m.pasting = "waiting", true
	m.input = textinput.New()
	_ = m.input.Focus()
	for _, character := range "?drq" {
		updated, _ = m.Update(tea.KeyPressMsg{Code: character, Text: string(character)})
		m = updated.(AccountModel)
	}
	if m.help || m.state != "waiting" || m.input.Value() != "?drq" {
		t.Fatal("input triggered global actions")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(AccountModel)
	if m.pasting || m.state != "waiting" || m.input.Value() != "" {
		t.Fatal("escape did not clear and dismiss callback input")
	}
}

func TestRemoteIdentityCannotInjectTerminalControls(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, context.Background())
	m.state = "connected"
	m.user = auth.User{ID: "01900000-0000-7000-8000-000000000001", Name: "Test\x1b]52;c;c2VjcmV0\a\r\nName", Username: "fixture\x1b[2J"}
	content := m.View().Content
	if strings.Contains(content, "\x1b]52") || strings.Contains(content, "\x1b[2J") || strings.Contains(content, "\r") {
		t.Fatal("remote terminal control escaped sanitization")
	}
}

func TestRejectedCallbackDoesNotLeaveAnUnstoredKeyForRetry(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	store := auth.Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	m := NewAccount("dark", store, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	updated, cmd := m.startAttempt()
	m = updated.(AccountModel)
	updated, _ = m.Update(cmd())
	m = updated.(AccountModel)
	updated, cmd = m.redeem(auth.RedirectURI + "?code=x&state=wrong")
	m = updated.(AccountModel)
	updated, _ = m.Update(cmd())
	m = updated.(AccountModel)
	if m.key != nil || m.state != "failed" {
		t.Fatal("rejected callback retained an unpersisted key")
	}
	if _, _, err := store.Load(); !os.IsNotExist(err) {
		t.Fatal("unsubmitted credential was not removed")
	}
}

func TestEscapeCancelsSavedKeyVerificationAndAllowsRetry(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	store := auth.Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	key, _ := auth.GenerateKey()
	if err := store.Save(key, server.URL); err != nil {
		t.Fatal(err)
	}
	m := NewAccount("dark", store, server.URL, t.Context())
	defer func() { _ = m.Close() }()
	m.key = key
	updated, cmd := m.verify()
	m = updated.(AccountModel)
	verified := make(chan tea.Msg, 1)
	go func() { verified <- cmd() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("verification never started")
	}
	updated, cleanup := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(AccountModel)
	cleaned := make(chan tea.Msg, 1)
	go func() { cleaned <- cleanup() }()
	select {
	case result := <-verified:
		if !errors.Is(result.(accountResult).err, context.Canceled) {
			t.Fatal("verification request was not canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("Escape left verification HTTP request running")
	}
	select {
	case result := <-cleaned:
		updated, _ = m.Update(result)
		m = updated.(AccountModel)
		if m.state != "failed" || m.key == nil {
			t.Fatal("canceled verification did not restore retry with the stored key")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation cleanup blocked behind verification")
	}
}

func TestAccountLayoutIsBoundedAndProvidesDirectRecovery(t *testing.T) {
	key, _ := auth.GenerateKey()
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	m.width, m.height, m.state, m.key = 160, 60, "failed", key
	m.status = "account server returned an invalid identity"
	content := ansi.Strip(m.View().Content)
	lines := strings.Split(content, "\n")
	found := false
	for row, line := range lines {
		if strings.Contains(line, "Connection needs attention") {
			found = true
			if row > 20 || len(line)-len(strings.TrimLeft(line, " ")) < 20 {
				t.Fatal("connection view is not positioned in a bounded reading column")
			}
		}
	}
	if !found || !strings.Contains(content, m.status) || !strings.Contains(content, "Verify saved connection") || !strings.Contains(content, "Browser approval") {
		t.Fatal("error, recovery action, or connection progress is missing")
	}
}

func TestConnectionColoursSetCanvasAndRespectNoColour(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	updated, _ := m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if updated.View().BackgroundColor == nil || updated.View().ForegroundColor == nil {
		t.Fatal("terminal default colours can leak through styled spans")
	}
	updated, _ = updated.Update(tea.ColorProfileMsg{Profile: colorprofile.Ascii})
	if updated.View().BackgroundColor != nil || updated.View().ForegroundColor != nil {
		t.Fatal("no-colour mode changed terminal colours")
	}
}

func TestDisconnectConfirmationDefaultsToKeepingTheKey(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	m.state = "connected"
	m.user = auth.User{ID: "01900000-0000-7000-8000-000000000001", Username: "fixture"}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.(AccountModel).state != "connected" || cmd != nil {
		t.Fatal("Enter deleted a key without explicitly selecting the destructive action")
	}
	updated, _ = updated.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	updated, cmd = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.(AccountModel).state != "disconnecting" || cmd == nil {
		t.Fatal("selected destructive action did not request disconnect")
	}
}

func TestSavedDeviceCanRecoverUUIDIdentityWithoutNewApproval(t *testing.T) {
	identity, err := os.ReadFile("../auth/testdata/current-user.json")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.Path != "/api/v1/me" {
			t.Error("recovery started a new approval")
		}
		_, _ = w.Write(identity)
	}))
	defer server.Close()
	key, _ := auth.GenerateKey()
	m := NewAccount("dark", auth.Store{}, server.URL, t.Context())
	defer func() { _ = m.Close() }()
	m.state, m.key = "failed", key
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(AccountModel)
	updated, _ = m.Update(cmd())
	m = updated.(AccountModel)
	if m.state != "connected" || m.user.ID != "01900000-0000-7000-8000-000000000001" || requests != 1 {
		t.Fatal("saved approved device could not recover from identity decoding failure")
	}
}

func TestAutomaticThemeReadsOriginalBackgroundBeforeSettingCanvas(t *testing.T) {
	m := NewAccount("auto", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	updated, _ := m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if updated.View().BackgroundColor != nil {
		t.Fatal("auto theme overwrote the terminal background before detecting it")
	}
	updated, _ = updated.Update(tea.BackgroundColorMsg{Color: color.White})
	if updated.(AccountModel).dark || updated.View().BackgroundColor == nil {
		t.Fatal("auto theme did not use the original light background")
	}
}

func TestSuccessfulVerificationShowsConfirmationWithoutClearingLaterErrors(t *testing.T) {
	m := NewAccount("dark", auth.Store{}, auth.DefaultOrigin, t.Context())
	defer func() { _ = m.Close() }()
	user := auth.User{ID: "01900000-0000-7000-8000-000000000001", Username: "fixture"}
	updated, notice := m.Update(accountResult{user: user})
	m = updated.(AccountModel)
	if notice == nil || !strings.Contains(ansi.Strip(m.View().Content), "Connection verified just now") {
		t.Fatal("successful verification has no visible confirmation")
	}
	updated, _ = m.Update(verificationNoticeExpired{generation: m.generation - 1})
	m = updated.(AccountModel)
	if !strings.Contains(ansi.Strip(m.View().Content), "Connection verified just now") {
		t.Fatal("stale timer cleared a newer confirmation")
	}
	updated, _ = m.Update(verificationNoticeExpired{generation: m.generation})
	m = updated.(AccountModel)
	if strings.Contains(ansi.Strip(m.View().Content), "Connection verified just now") {
		t.Fatal("temporary confirmation did not clear")
	}
	for _, dismiss := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: tea.KeyEnter}} {
		updated, _ = m.Update(accountResult{user: user})
		updated, _ = updated.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
		updated, _ = updated.Update(verificationNoticeExpired{generation: m.generation})
		updated, _ = updated.Update(dismiss)
		m = updated.(AccountModel)
		if m.state != "connected" || strings.Contains(ansi.Strip(m.View().Content), "Connection verified just now") {
			t.Fatal("expired notice survived disconnect confirmation")
		}
	}
	updated, _ = m.Update(accountResult{err: errors.New("synthetic verification failure")})
	updated, _ = updated.Update(verificationNoticeExpired{generation: m.generation})
	if !strings.Contains(ansi.Strip(updated.View().Content), "synthetic verification failure") {
		t.Fatal("confirmation timer cleared an error")
	}
}

func TestConnectedFingerprintIsToggleableAndApprovalFingerprintStaysVisible(t *testing.T) {
	key, _ := auth.GenerateKey()
	for _, theme := range []string{"light", "dark"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}} {
			m := NewAccount(theme, auth.Store{}, auth.DefaultOrigin, t.Context())
			m.width, m.height, m.key = size.Width, size.Height, key
			updated, _ := m.Update(accountResult{user: auth.User{ID: "01900000-0000-7000-8000-000000000001", Name: "Synthetic Developer", Username: "fixture"}})
			m = updated.(AccountModel)
			if strings.Contains(ansi.Strip(m.View().Content), key.Fingerprint()) {
				t.Fatal("connected fingerprint is not collapsed by default")
			}
			updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			updated, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updated.(AccountModel)
			content := ansi.Strip(m.View().Content)
			if cmd != nil || !strings.Contains(content, key.Fingerprint()) || !strings.Contains(content, "Connection verified just now") || !strings.Contains(content, "Hide") {
				t.Fatalf("device details or confirmation hidden at %dx%d", size.Width, size.Height)
			}
			if lipgloss.Width(m.View().Content) > size.Width || lipgloss.Height(m.View().Content) > size.Height {
				t.Fatal("expanded details overflow terminal")
			}
			updated, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
			m = updated.(AccountModel)
			if strings.Contains(ansi.Strip(m.View().Content), key.Fingerprint()) {
				t.Fatal("device details did not collapse")
			}
			m.state = "waiting"
			if !strings.Contains(ansi.Strip(m.View().Content), key.Fingerprint()) {
				t.Fatal("approval fingerprint was hidden")
			}
			_ = m.Close()
		}
	}
}
