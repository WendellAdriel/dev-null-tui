package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestUpdateCheckRunsWhenOnForReleaseBuilds(t *testing.T) {
	for _, tc := range []struct {
		name, current string
		settings      auth.TUISettings
		due           bool
	}{
		{"turned on", "v0.2.0", auth.TUISettings{CheckForUpdates: true}, true},
		{"turned off", "v0.2.0", auth.TUISettings{}, false},
		{"development build", "dev", auth.TUISettings{CheckForUpdates: true}, false},
	} {
		if got := upgradeCheckDue(tc.settings, tc.current); got != tc.due {
			t.Errorf("%s: due=%v", tc.name, got)
		}
	}
}

func TestEveryLaunchChecksOnceAndFailsSilently(t *testing.T) {
	store := auth.Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	updates := Updates{Check: func(context.Context) (string, error) { calls++; return "", errors.New("offline") }}
	launch := func() (AppModel, tea.Cmd) {
		m := NewApp("dark", store, "https://example.test", t.Context()).WithVersion("v0.2.0").WithUpdates(updates)
		t.Cleanup(func() { _ = m.Close() })
		return m, m.startUpgradeCheck()
	}
	m, cmd := launch()
	m = deliver(m, runAll(cmd))
	if calls != 1 || m.upgrade.open || m.toast.visible {
		t.Fatalf("a failed check must stay silent: calls=%d open=%v toast=%v", calls, m.upgrade.open, m.toast.visible)
	}
	if m.startUpgradeCheck() != nil {
		t.Fatal("the check must run once per launch")
	}
	_, cmd = launch()
	deliver(m, runAll(cmd))
	if calls != 2 {
		t.Fatalf("a second launch checked %d times in all", calls)
	}
}

func upgradeApp(t *testing.T, updates Updates) (AppModel, auth.Store) {
	t.Helper()
	m, store := upgradedApp(t, "v0.1.2")
	m = m.WithUpdates(updates)
	next, _ := m.Update(upgradeChecked{latest: "v0.3.0"})
	return next.(AppModel), store
}

func TestUpdateModalWaitsForWhatsNewAndCanBePostponedOrSkipped(t *testing.T) {
	m, store := upgradeApp(t, Updates{Restart: true})
	if !m.whatsNew.open || strings.Contains(ansi.Strip(m.View().Content), "Update available") {
		t.Fatal("the update modal must wait for What's new to close")
	}
	m = pressApp(m, "esc")
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Update available", "devnull v0.3.0 is available", "v0.2.0", "Update and restart", "Later", "Skip this version"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("modal missing %q", want)
		}
	}
	m = pressApp(m, "c")
	if m.composer.open {
		t.Fatal("keys leaked past the update modal")
	}
	m = pressApp(m, "esc")
	if m.upgrade.open || store.TUISettings().SkippedVersion != "" {
		t.Fatal("Later must close without skipping the version")
	}

	m, store = upgradeApp(t, Updates{Restart: true})
	m = pressApp(m, "esc")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = deliver(next.(AppModel), runAll(cmd))
	if m.upgrade.open || store.TUISettings().SkippedVersion != "v0.3.0" {
		t.Fatal("Skip must close and remember the version")
	}
	for latest, open := range map[string]bool{"v0.3.0": false, "v0.4.0": true} {
		again := NewApp("dark", store, "https://example.test", t.Context()).WithVersion("v0.2.0")
		next, _ := again.Update(upgradeChecked{latest: latest})
		if next.(AppModel).upgrade.open != open {
			t.Fatalf("%s: open=%v", latest, !open)
		}
	}
}

func TestUpdateAndRestartQuitsOnlyAfterTheInstallSucceeds(t *testing.T) {
	installed := Updates{Restart: true, Install: func(context.Context) (string, error) { return "v0.3.0", nil }}
	m, _ := upgradeApp(t, installed)
	m = pressApp(m, "esc")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(AppModel)
	if !m.upgrade.installing || cmd == nil || !strings.Contains(ansi.Strip(m.View().Content), "Updating to v0.3.0") {
		t.Fatal("Enter must start the install")
	}
	if m = pressApp(m, "esc"); !m.upgrade.open {
		t.Fatal("the modal must stay open while installing")
	}
	next, quit := m.Update(cmd())
	m = next.(AppModel)
	if quit == nil || !m.RestartRequested() {
		t.Fatal("a successful install must quit and request a restart")
	}
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("expected the app to quit")
	}

	m, _ = upgradeApp(t, Updates{Install: installed.Install})
	m = pressApp(m, "esc")
	plain := ansi.Strip(m.View().Content)
	if strings.Contains(plain, "Update and restart") || !strings.Contains(plain, "Update") {
		t.Fatal("without restart support the action is a plain update")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, quit = next.(AppModel).Update(cmd())
	m = next.(AppModel)
	if quit != nil || m.RestartRequested() || !strings.Contains(ansi.Strip(m.View().Content), "Start devnull again") {
		t.Fatal("without restart support the modal must ask for a manual restart")
	}
	if m = pressApp(m, "enter"); m.upgrade.open {
		t.Fatal("Enter must close the finished update")
	}

	m, _ = upgradeApp(t, Updates{Restart: true, Install: func(context.Context) (string, error) { return "", errors.New("permission denied") }})
	m = pressApp(m, "esc")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, quit = next.(AppModel).Update(cmd())
	m = next.(AppModel)
	plain = ansi.Strip(m.View().Content)
	if quit != nil || m.RestartRequested() || !strings.Contains(plain, "permission denied") || !strings.Contains(plain, "devnull update") {
		t.Fatal("a failed install must explain the error in the modal")
	}
}

func TestUpdateModalFitsEveryTerminalSize(t *testing.T) {
	for _, size := range [][2]int{{160, 45}, {80, 24}, {60, 18}} {
		m, _ := upgradeApp(t, Updates{Restart: true})
		m = pressApp(m, "esc")
		m.upgrade.failure = strings.Repeat("a long install error ", 10) + "permission denied"
		m.account.width, m.account.height = size[0], size[1]
		m.reflow()
		plain := ansi.Strip(m.View().Content)
		lines := strings.Split(plain, "\n")
		if len(lines) > size[1] || !strings.Contains(plain, "Skip this version") || !strings.Contains(plain, "denied") {
			t.Fatalf("%dx%d: modal missing or too tall", size[0], size[1])
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%dx%d: line too wide", size[0], size[1])
			}
		}
	}
}

func TestTUISettingsTabTogglesUpdateChecks(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openSettings()
	m.settings.busy = false
	for range len(settingsTabs) - 1 {
		m = pressApp(m, "tab")
	}
	if m.settings.tab != settingsTUI || !strings.Contains(ansi.Strip(m.View().Content), "Check for updates: On") {
		t.Fatal("the TUI tab must show the update check as on by default")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = deliver(next.(AppModel), runAll(cmd))
	if m.account.store.TUISettings().CheckForUpdates || !strings.Contains(ansi.Strip(m.View().Content), "Check for updates: Off") {
		t.Fatal("toggling must save the setting")
	}
	next, _ = m.Update(tuiSettingsSaved{err: errTestWrite})
	if m = next.(AppModel); !m.toast.visible || m.toast.kind != "warning" {
		t.Fatal("a failed save must warn")
	}
}
