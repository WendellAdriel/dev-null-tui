package tui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestSettingsNavigationAndTypingIsolation(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m = pressApp(pressApp(m, "g"), ",")
	if !m.settings.open || m.starredOpen {
		t.Fatal("g comma must open Settings without taking Starred's key")
	}
	m.settings.busy = false
	m.beginSettingsDialog("delete-account", "", "")
	for _, r := range "gscq" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(AppModel)
	}
	if m.pendingG || m.composer.open || m.settings.input.Value() != "gscq" {
		t.Fatal("form typing reached global shortcuts")
	}
	m = pressApp(pressApp(m, "esc"), "esc")
	if m.settings.open || m.settings.dialog != "" {
		t.Fatal("Esc must close the dialog before Settings")
	}
	m = pressApp(pressApp(m, "g"), "s")
	if !m.starredOpen {
		t.Fatal("Starred shortcut changed")
	}
}

func TestSettingsFixtureWritesOnceAndDeletesOnlyAfterServerConfirmation(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned request")
		}
		if r.Method == "DELETE" {
			calls++
			w.WriteHeader(204)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"fixture","handle":"fixture","pasteis_mode":true},"unread_notifications_count":0}`)
	}))
	defer server.Close()
	client, err := api.NewClient(server.URL, m.account.key)
	if err != nil {
		t.Fatal(err)
	}
	m.settingsAPI = client
	if err := m.account.store.Save(m.account.key, server.URL); err != nil {
		t.Fatal(err)
	}
	m.beginSettingsDialog("delete-account", "", "")
	m.settings.input.SetValue("fixture")
	cmd := m.submitSettingsDialog()
	if cmd == nil || m.submitSettingsDialog() != nil {
		t.Fatal("duplicate deletion was scheduled")
	}
	if _, err := os.Stat(filepath.Join(m.account.store.Dir, "connection.json")); err != nil {
		t.Fatal("key removed before server confirmation")
	}
	result := cmd().(settingsResult)
	if !result.signedOut || result.err != nil || result.cleanupErr != nil {
		t.Fatal(result)
	}
	m.acceptSettings(result)
	if m.connected || calls != 1 {
		t.Fatal("fixture deletion not applied exactly once")
	}
	if _, err := os.Stat(filepath.Join(m.account.store.Dir, "connection.json")); !os.IsNotExist(err) {
		t.Fatal("confirmed deletion kept the local key")
	}
}

func TestUncertainRefreshFailureKeepsWritesLockedAndSafetyFlagsIndependent(t *testing.T) {
	m := fixtureApp(t)
	m.settings.open, m.settings.uncertain = true, true
	cmd := m.settingsKey("ctrl+r")
	if cmd == nil || !m.settings.uncertain {
		t.Fatal("refresh must not unlock writes before it succeeds")
	}
	m.acceptSettings(settingsResult{session: m.session, generation: m.settings.generation, op: settingsOperation{kind: "refresh"}, err: errors.New("offline")})
	if !m.settings.uncertain || m.startSettings(settingsOperation{kind: "pasteis", enabled: true}) != nil {
		t.Fatal("failed refresh unlocked uncertain writes")
	}
	m.restrictSafetyContent("ada", "mute")
	m.restrictSafetyContent("ada", "block")
	m.settings.busy = true
	m.acceptSettings(settingsResult{session: m.session, generation: m.settings.generation, op: settingsOperation{kind: "block", handle: "ada", enabled: false}})
	if m.safetyHidden["ada"] != 1 {
		t.Fatal("unblocking also removed the mute")
	}
}

func TestSafetyPaginationAndStaleListResults(t *testing.T) {
	m := fixtureApp(t)
	m.settings.open = true
	f := &m.settings.lists[0]
	f.loading, f.generation = true, 3
	f.cursors = map[string]bool{}
	m.acceptSafetyList(settingsPeopleResult{session: m.session, generation: 3, list: 0, page: api.People{Data: []api.User{{ID: "ada", Handle: "ada"}}, Next: "older"}})
	if len(f.people) != 1 || f.next != "older" {
		t.Fatal("first safety page lost")
	}
	f.loading, f.generation = true, 4
	f.cursors["older"] = true
	m.acceptSafetyList(settingsPeopleResult{session: m.session, generation: 3, list: 0, page: api.People{Data: []api.User{{ID: "stale", Handle: "stale"}}}})
	if !f.loading || len(f.people) != 1 {
		t.Fatal("stale page was accepted")
	}
	m.acceptSafetyList(settingsPeopleResult{session: m.session, generation: 4, list: 0, cursor: "older", page: api.People{Data: []api.User{{ID: "ada", Handle: "ada"}, {ID: "grace", Handle: "grace"}}, Next: "older"}})
	if len(f.people) != 2 || f.next != "" || f.status == "" {
		t.Fatal("pagination must deduplicate and stop repeated cursors")
	}
}

func TestAccountDeletionRequiresTypedConsentAndConfirmedSuccess(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openSettings()
	m.settings.busy = false
	m.beginSettingsDialog("delete-account", "", "")
	if cmd := m.settingsDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("Enter on default Cancel must not delete")
	}
	m.beginSettingsDialog("delete-account", "", "")
	m.settings.focus = 1
	if cmd := m.submitSettingsDialog(); cmd != nil {
		t.Fatal("missing typed handle must not delete")
	}
	m.settings.input.SetValue(m.account.user.Handle)
	m.settings.generation++
	m.settings.busy = true
	gen, session := m.settings.generation, m.session
	m.acceptSettings(settingsResult{session: session, generation: gen, op: settingsOperation{kind: "delete-account"}, err: errors.New("network failed")})
	if !m.connected || m.account.key == nil || !m.settings.uncertain {
		t.Fatal("uncertain deletion must not claim success or remove the key")
	}
	m.settings.busy = true
	m.acceptSettings(settingsResult{session: session, generation: gen - 1, op: settingsOperation{kind: "delete-account"}, signedOut: true})
	if !m.connected {
		t.Fatal("stale deletion removed the session")
	}
	m.openComposer()
	m.composer.input.SetValue("private draft")
	m.settings.busy = true
	m.acceptSettings(settingsResult{session: session, generation: gen, op: settingsOperation{kind: "delete-account"}, signedOut: true})
	if m.connected || m.account.key != nil || m.composer.input.Value() != "" || m.settings.open {
		t.Fatal("confirmed deletion must erase local account state")
	}
}

func TestSettingsOnlyApplyMatchingConfirmedIdentity(t *testing.T) {
	m := fixtureApp(t)
	m.settings.busy = true
	m.settings.generation = 5
	before := m.account.user
	m.acceptSettings(settingsResult{session: m.session, generation: 5, user: auth.User{ID: "other", Handle: "other"}})
	if m.account.user.ID != before.ID {
		t.Fatal("another identity replaced the account")
	}
	m.settings.busy = true
	u := before
	u.PasteisMode = true
	m.acceptSettings(settingsResult{session: m.session, generation: 5, user: u, op: settingsOperation{kind: "pasteis"}})
	if !m.account.user.PasteisMode {
		t.Fatal("confirmed account preference was not applied")
	}
}

func TestSafetyChangesDiscardStaleReadsAndHiddenContent(t *testing.T) {
	m := fixtureApp(t)
	c := fixtureCommit("hidden")
	c.Author.Handle = "ada"
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c, fixtureCommit("keep")}})
	m.feeds[0].commits[1].Author.Handle = "grace"
	gen := m.feeds[0].generation
	m.restrictSafetyContent("ada", "mute")
	if len(m.feeds[0].commits) != 1 || m.feeds[0].commits[0].ID != "keep" {
		t.Fatal("safety change did not remove the intended author's content")
	}
	next, _ := m.Update(logResult{index: 0, generation: gen, page: api.Page{Data: []api.Commit{c}}})
	m = next.(AppModel)
	if len(m.feeds[0].commits) != 1 || m.feeds[0].commits[0].ID != "keep" {
		t.Fatal("stale response restored muted content")
	}
}

func TestSettingsViewsStayWithinTerminalAndShowOnlySupportedTabs(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}} {
			m := fixtureApp(t)
			m.account.width, m.account.height, m.account.dark = size[0], size[1], dark
			m.openSettings()
			m.settings.busy = false
			m.settings.region = api.AccountSettings{Timezone: "America/Argentina/ComodRivadavia", ShowLocalTime: true, TranslationLanguage: "pt", TranslationLanguages: []api.Language{{Code: "en", Name: "English", NativeName: "English"}, {Code: "pt", Name: "Portuguese", NativeName: "português"}}}
			m.settings.regionLoad.loaded = true
			m.settings.devices = []api.Device{{ID: "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", Name: strings.Repeat("workstation", 8), AppName: "/dev/null TUI", Fingerprint: m.account.key.Fingerprint(), ApprovedAt: "2026-11-01T10:00:00Z"}}
			m.settings.devicesLoad.loaded = true
			views := []string{"", "", "", "", "", "timezone", "translation-language", "revoke-device"}
			for i, dialog := range views {
				m.settings.tab = min(i, len(settingsTabs)-1)
				m.settings.dialog = ""
				if dialog != "" {
					m.beginSettingsDialog(dialog, "", m.settings.devices[0].ID)
				}
				m.reflow()
				plain := ansi.Strip(m.View().Content)
				if len(strings.Split(plain, "\n")) > size[1] {
					t.Fatal("settings overflowed terminal height")
				}
				for _, line := range strings.Split(plain, "\n") {
					if ansi.StringWidth(line) > size[0] {
						t.Fatal("settings overflowed terminal width")
					}
				}
				if strings.Contains(plain, "Export") {
					t.Fatal("unsupported data export exposed")
				}
			}
		}
	}
}
