package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestInlineSettingsActionsUseProgressAndResultToasts(t *testing.T) {
	for kind, title := range map[string]string{"github": "Syncing GitHub profile", "pasteis": "Updating pastéis mode", "remotes": "Updating Remotes", "remote-preferences": "Updating Remotes preferences"} {
		t.Run(kind, func(t *testing.T) {
			m := fixtureApp(t)
			m.account.width, m.account.height = 120, 35
			m.openSettings()
			m.settings.busy = false
			op := settingsOperation{kind: kind, enabled: true, from: "following"}
			cmd := m.startSettings(op)
			if cmd == nil || !m.settings.busy || !m.toast.visible || m.toast.kind != "info" || m.toast.title != title {
				t.Fatal("action must start with an info toast")
			}
			if batch, ok := cmd().(tea.BatchMsg); !ok || len(batch) != 2 {
				t.Fatal("request and toast expiry must both be scheduled")
			}
			column := ansi.Strip(m.settingsColumn(92, colors(m.account.dark)))
			if strings.Contains(column, "Working") || strings.Contains(column, title) {
				t.Fatal("progress must not be duplicated at the bottom of Settings")
			}
			progressID := m.toast.id
			if m.startSettings(op) != nil || m.toast.id != progressID {
				t.Fatal("duplicate input must not restart the request or toast")
			}
			m.acceptSettings(settingsResult{session: m.session, generation: m.settings.generation, op: op, user: m.account.user})
			if m.toast.kind != "success" || m.toast.id == progressID || m.settings.status != "" {
				t.Fatal("confirmed success must replace progress without duplicating footer text")
			}
			next, _ := m.Update(toastExpired{id: progressID, session: m.session})
			m = next.(AppModel)
			if !m.toast.visible || m.toast.kind != "success" {
				t.Fatal("old progress expiry dismissed the result toast")
			}
		})
	}
}

func TestInlineSettingsFailureReplacesProgressAndRetainsRecoveryText(t *testing.T) {
	m := fixtureApp(t)
	op := settingsOperation{kind: "github"}
	m.startSettings(op)
	progressID := m.toast.id
	m.acceptSettings(settingsResult{session: m.session, generation: m.settings.generation, op: op, err: errors.New("offline")})
	if !m.toast.visible || m.toast.kind != "error" || m.toast.id == progressID || !m.settings.uncertain || m.settings.status == "" {
		t.Fatal("failure must replace progress while retaining persistent recovery guidance")
	}
}
