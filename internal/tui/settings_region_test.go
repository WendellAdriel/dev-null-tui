package tui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const regionFixture = `{"data":{"timezone":"Europe/Lisbon","show_local_time":false,"translation_language":null,"automatic_translation":true,"translation_languages":[{"code":"en","name":"English","native_name":"English"},{"code":"pt","name":"Portuguese","native_name":"português"}],"is_admin":false}}`

func settingsFixture(t *testing.T, handler http.HandlerFunc) AppModel {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.settingsAPI, _ = api.NewClient(server.URL, m.account.key)
	if err := m.account.store.Save(m.account.key, server.URL); err != nil {
		t.Fatal(err)
	}
	return m
}

func runSettings(m AppModel, cmd tea.Cmd) AppModel {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runSettings(m, c)
		}
		return m
	}
	switch msg.(type) {
	case settingsResult, settingsLoadResult, settingsPeopleResult:
		next, _ := m.Update(msg)
		return next.(AppModel)
	}
	return m
}

func TestRegionTabLoadsSettingsAndSavesConfirmedPreferences(t *testing.T) {
	var writes []string
	m := settingsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/me/settings" {
			_, _ = io.WriteString(w, regionFixture)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method == "PUT" {
			writes = append(writes, r.URL.Path+" "+string(body))
		}
		_, _ = io.WriteString(w, `{"data":{"id":"fixture","handle":"fixture"}}`)
	})
	m.settings.open, m.settings.tab = true, settingsProfile
	m = runSettings(m, m.settingsKey("right"))
	if m.settings.tab != settingsRegionTab || !m.settings.regionLoad.loaded || m.settings.region.Timezone != "Europe/Lisbon" {
		t.Fatalf("Region did not load: %+v", m.settings.region)
	}

	m = runSettings(m, m.activateSetting("automatic-translation", ""))
	m = runSettings(m, m.activateSetting("local-time", ""))
	if m.settings.region.AutomaticTranslation || !m.settings.region.ShowLocalTime {
		t.Fatal("confirmed toggles were not applied")
	}

	m = runSettings(m, m.activateSetting("timezone", ""))
	m.settings.input.SetValue("")
	m = runSettings(m, m.submitSettingsDialog())
	if m.settings.region.Timezone != "" || m.settings.region.ShowLocalTime || m.settings.dialog != "" {
		t.Fatal("unsetting the timezone must also hide local time")
	}

	m = runSettings(m, m.activateSetting("translation-language", ""))
	for _, r := range "port" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(AppModel)
	}
	m = runSettings(m, m.submitSettingsDialog())
	if m.settings.region.TranslationLanguage != "pt" {
		t.Fatal("language filter did not choose Portuguese")
	}

	want := []string{
		`/api/v1/me/automatic-translation {"enabled":false}`,
		`/api/v1/me/timezone {"timezone":"Europe/Lisbon","show_local_time":true}`,
		`/api/v1/me/timezone {"timezone":null,"show_local_time":false}`,
		`/api/v1/me/translation-language {"translation_language":"pt"}`,
	}
	if strings.Join(writes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("writes:\n%s", strings.Join(writes, "\n"))
	}
}

func devicesFixture(current string) string {
	return `{"data":[{"id":"0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b","name":"laptop","app_name":"/dev/null TUI","fingerprint":"` + current + `","approved_at":"2026-11-01T10:00:00Z","last_used_at":"2026-11-02T09:00:00Z"},{"id":"0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c","name":"phone","app_name":"/dev/null","fingerprint":"SHA256:other","approved_at":"2026-10-01T10:00:00Z","last_used_at":null}]}`
}

func TestRevokingAnotherDeviceRemovesOnlyThatRow(t *testing.T) {
	var revoked []string
	var m AppModel
	m = settingsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			revoked = append(revoked, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, devicesFixture(m.account.key.Fingerprint()))
	})
	m.settings.open, m.settings.tab = true, settingsSafety
	m = runSettings(m, m.settingsKey("right"))
	if len(m.settings.devices) != 2 {
		t.Fatal("devices not loaded")
	}
	rows := m.settingsRows()
	current := 0
	for _, row := range rows {
		if strings.Contains(row.title, "This device") {
			current++
		}
	}
	if current != 1 {
		t.Fatal("the signing device must be marked exactly once")
	}
	m = runSettings(m, m.activateSetting("revoke-device", "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
	if m.settings.dialog != "revoke-device" || len(revoked) != 0 {
		t.Fatal("revoking must ask first")
	}
	m = runSettings(m, m.submitSettingsDialog())
	if len(revoked) != 1 || !strings.HasSuffix(revoked[0], "4a5c") || len(m.settings.devices) != 1 || !m.connected {
		t.Fatalf("revoke result: %v %d", revoked, len(m.settings.devices))
	}
}

func TestRevokingThisDeviceSignsOutAndRemovesTheSavedKey(t *testing.T) {
	var m AppModel
	m = settingsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, devicesFixture(m.account.key.Fingerprint()))
	})
	m.settings.open, m.settings.tab = true, settingsSafety
	m = runSettings(m, m.settingsKey("right"))
	m.activateSetting("revoke-device", "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	cmd := m.submitSettingsDialog()
	if cmd == nil || !m.settings.operation.current {
		t.Fatal("current device revoke not scheduled")
	}
	if !m.revoked(&auth.APIError{Status: http.StatusUnauthorized}) || !m.connected {
		t.Fatal("a background 401 must not tear down a pending revoke of this device")
	}
	m = runSettings(m, cmd)
	if m.connected || m.account.key != nil {
		t.Fatal("revoking this device must sign out")
	}
	if _, err := os.Stat(filepath.Join(m.account.store.Dir, "connection.json")); !os.IsNotExist(err) {
		t.Fatal("revoked device kept its saved key")
	}
}

func TestRegionSelectionStartsOnTimezoneAfterLoading(t *testing.T) {
	m := settingsFixture(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, regionFixture) })
	m.settings.open, m.settings.tab = true, settingsProfile
	cmd := m.settingsKey("right")
	m = runSettings(m, cmd)
	if rows := m.settingsRows(); rows[m.settings.selected].action != "timezone" {
		t.Fatalf("selection after loading Region: %q", rows[m.settings.selected].action)
	}
}

func TestEnterSavesRegionDialogsButCancelsDestructiveOnes(t *testing.T) {
	var writes []string
	m := settingsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			writes = append(writes, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"fixture","handle":"fixture"}}`)
	})
	m.settings.open = true
	m.settings.region = api.AccountSettings{TranslationLanguages: []api.Language{{Code: "pt", Name: "Portuguese", NativeName: "português"}}}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}

	m.beginSettingsDialog("timezone", "", "")
	m.settings.input.SetValue("Europe/Lisbon")
	m = runSettings(m, m.settingsDialogKey(enter))
	m.beginSettingsDialog("translation-language", "", "")
	m = runSettings(m, m.settingsDialogKey(enter))
	if strings.Join(writes, " ") != "/api/v1/me/timezone /api/v1/me/translation-language" {
		t.Fatalf("Enter in Region dialogs: %v", writes)
	}

	m.beginSettingsDialog("timezone", "", "")
	m = pressApp(m, "tab")
	m = runSettings(m, m.settingsDialogKey(enter))
	for _, kind := range []string{"delete-account", "revoke-device"} {
		m.beginSettingsDialog(kind, "", "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c")
		m.settings.input.SetValue("fixture")
		m = runSettings(m, m.settingsDialogKey(enter))
	}
	if len(writes) != 2 || m.settings.dialog != "" {
		t.Fatalf("Enter on Cancel must not write: %v", writes)
	}
}
