package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// borderIs reports whether the modal's top border is drawn in the given color.
func borderIs(view, color string) bool {
	sgr, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("╭"), "╭")
	top, _, _ := strings.Cut(view, "\n")
	return strings.Contains(top, sgr+"╭")
}

func TestModalBordersMatchTheActionRisk(t *testing.T) {
	for _, dark := range []bool{false, true} {
		m := fixtureApp(t)
		m.account.width, m.account.height, m.account.dark = 120, 35, dark
		p := colors(dark)
		destructive := map[string]string{}
		warning := map[string]string{}
		safe := map[string]string{}
		for _, kind := range []string{"delete-account", "revoke-device", "block", "mute", "report", "timezone", "translation-language", "lane"} {
			m.settings.busy = false
			m.beginSettingsDialog(kind, "ada", "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c")
			view, _, _ := m.settingsDialog(p)
			switch kind {
			case "delete-account", "revoke-device", "block":
				destructive[kind] = view
			case "mute", "report":
				warning[kind] = view
			default:
				safe[kind] = view
			}
		}
		destructive["delete-commit"], _, _ = m.deleteModal(p)
		for _, kind := range []string{"discard", "exit", "retry"} {
			m.composer.confirm = kind
			view, _, _ := m.confirmationModal(p)
			if kind == "retry" {
				safe[kind] = view
			} else {
				destructive[kind] = view
			}
		}
		for kind, view := range destructive {
			if !borderIs(view, p.danger) {
				t.Errorf("dark=%v: %s modal must use the danger border", dark, kind)
			}
		}
		for kind, view := range warning {
			if !borderIs(view, p.warning) {
				t.Errorf("dark=%v: %s modal must use the warning border", dark, kind)
			}
		}
		for kind, view := range safe {
			if !borderIs(view, p.blue) {
				t.Errorf("dark=%v: %s modal must keep the default border", dark, kind)
			}
		}
	}
}
