package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestColorPickerRestoresTheSavedChoiceAndDisplaysItsName(t *testing.T) {
	for _, tc := range []struct {
		raw, label, value string
		choice            int
	}{{"0", "Orange", "", 1}, {"5", "Ochre", "", 6}, {"null", "Automatic", "", 0}, {`"#123abc"`, "Custom (#123abc)", "#123abc", 7}} {
		t.Run(tc.label, func(t *testing.T) {
			m := fixtureApp(t)
			m.account.user.PreferredLane = []byte(tc.raw)
			if got := m.settingsRows()[2].title; got != "Color: "+tc.label {
				t.Fatalf("wrong label: %q", got)
			}
			m.beginSettingsDialog("lane", "", "")
			if m.settings.choice != tc.choice || m.settings.input.Value() != tc.value {
				t.Fatalf("saved color not restored: choice=%d value=%q", m.settings.choice, m.settings.input.Value())
			}
			if m.settings.input.Focused() != (tc.choice == 7) {
				t.Fatal("custom input must not take focus from a palette choice")
			}
		})
	}
}

func TestColorPickerSupportsArrowSelectionAndSavesTheSelectedLane(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.account.user.PreferredLane = []byte("0")
	m.beginSettingsDialog("lane", "", "")
	m = pressApp(m, "right")
	if m.settings.choice != 2 {
		t.Fatal("Right must move from Orange to Green")
	}
	m = pressApp(m, "left")
	if m.settings.choice != 1 {
		t.Fatal("Left must move back to Orange")
	}
	cmd := m.settingsDialogKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil || m.settings.operation.lane == nil || *m.settings.operation.lane != 0 || m.settings.operation.color != "" {
		t.Fatal("saving Orange must send lane 0, not a custom color or Automatic")
	}
}

func TestColorPickerSwatchesAreClickableAfterWrappingAndResize(t *testing.T) {
	for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}} {
		m := fixtureApp(t)
		m.account.width, m.account.height = size[0], size[1]
		m.beginSettingsDialog("lane", "", "")
		panel, r, areas := m.settingsDialog(colors(m.account.dark))
		lines := strings.Split(panel, "\n")
		count := 0
		for _, area := range areas {
			if area.action != "lane-choice" {
				continue
			}
			count++
			visible := ansi.Cut(ansi.Strip(lines[area.y-r.y]), area.x-r.x, area.x-r.x+area.w)
			if !strings.Contains(visible, settingsLaneNames[area.index]) {
				t.Fatalf("swatch click target is not on its label: %q", visible)
			}
			next, _ := m.handleMouse(tea.MouseClickMsg{X: area.x + area.w/2, Y: area.y, Button: tea.MouseLeft})
			m = next
			if m.settings.choice != area.index {
				t.Fatalf("click at %dx%d did not select %d", size[0], size[1], area.index)
			}
		}
		if count != 8 {
			t.Fatalf("expected eight clickable choices at %dx%d, got %d", size[0], size[1], count)
		}
	}
}

func TestCustomColorFieldSelectsCustomAndKeepsCaretKeys(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 80, 24
	m.beginSettingsDialog("lane", "", "")
	_, _, areas := m.settingsDialog(colors(m.account.dark))
	clicked := false
	for _, area := range areas {
		if area.action == "lane-input" {
			m, _ = m.handleMouse(tea.MouseClickMsg{X: area.x, Y: area.y, Button: tea.MouseLeft})
			clicked = true
			break
		}
	}
	if !clicked || m.settings.choice != 7 || !m.settings.input.Focused() {
		t.Fatal("clicking the custom field must select and focus Custom")
	}
	for _, r := range "#123abc" {
		m = pressApp(m, string(r))
	}
	m = pressApp(m, "left")
	if m.settings.choice != 7 || !strings.HasPrefix(m.settings.input.Value(), "#123abc") || m.settings.input.Position() != 6 {
		t.Fatal("custom input must keep normal caret movement")
	}
}
