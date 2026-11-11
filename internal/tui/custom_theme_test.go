package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func themeApp(t *testing.T) AppModel {
	t.Helper()
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	return next.(AppModel)
}

// pressKey presses a key through Update, so overlays get it as they would from the terminal.
func pressKey(m AppModel, name string) AppModel {
	msg := keyMsg(name)
	if name == "f2" {
		msg = tea.KeyPressMsg{Code: tea.KeyF2}
	}
	next, _ := m.Update(msg)
	return next.(AppModel)
}

// selectRow puts the open picker's cursor on the row with value.
func selectRow(t *testing.T, m AppModel, value string) {
	t.Helper()
	i := slices.IndexFunc(m.picker.rows, func(r pickerRow) bool { return r.value == value && !r.heading })
	if i < 0 {
		t.Fatalf("the picker has no %q row", value)
	}
	m.picker.selected = i
}

func TestANewCustomThemeStartsFromABuiltInAndComesBackAfterARestart(t *testing.T) {
	m := themeApp(t)
	m.setTheme("dracula")
	m = pressKey(m, "f2")
	if !strings.Contains(frameText(m), "New theme  n") {
		t.Fatalf("the theme picker has no New theme chip:\n%s", frameText(m))
	}
	m = pressKey(m, "n")
	if m.picker == nil || m.picker.kind != "theme-base" || len(m.picker.rows) != 3 || !strings.Contains(frameText(m), "Current theme") || !strings.Contains(frameText(m), "Default dark") || !strings.Contains(frameText(m), "Default light") {
		t.Fatalf("n should offer the current theme and the default dark and light ones:\n%s", frameText(m))
	}
	selectRow(t, m, currentBase)
	m = pressKey(m, "enter")
	if m.themeEditor == nil || !strings.Contains(frameText(m), "Background") || !strings.Contains(frameText(m), themeNamed("dracula").palette(true).shell) {
		t.Fatalf("the editor did not open on Dracula's colors:\n%s", frameText(m))
	}
	m = typeText(m, "Night owl")
	m = pressKey(m, "down")
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "#101010")
	m = pressKey(m, "down")
	m = pressKey(m, "down")
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "#151515")
	if frame := frameText(m); !strings.Contains(frame, "Muted text on Background") {
		t.Fatalf("a muted color close to the background is not flagged:\n%s", frame)
	}
	m = pressKey(m, "ctrl+s")
	if m.themeEditor != nil {
		t.Fatalf("a theme with a readability warning should still save:\n%s", frameText(m))
	}
	saved := m.tuiSettings.CustomThemes
	if len(saved) != 1 || saved[0].Name != "Night owl" || saved[0].Base != "dracula" || saved[0].Colors["shell"] != "#101010" || saved[0].Colors["muted"] != "#151515" {
		t.Fatalf("saved custom themes: %+v", saved)
	}
	if m.account.themeName != "custom:Night owl" || m.account.colors().shell != "#101010" || !m.account.isDark() {
		t.Fatalf("the new theme is not applied: %q %s dark=%v", m.account.themeName, m.account.colors().shell, m.account.isDark())
	}
	if err := m.account.store.SaveTUISettings(m.tuiSettings); err != nil {
		t.Fatal(err)
	}
	again := NewApp("", m.account.store, "https://example.test", t.Context())
	t.Cleanup(func() { _ = again.Close() })
	if again.account.themeName != "custom:Night owl" || again.account.colors().shell != "#101010" || again.account.colors().lanes != themeNamed("dracula").palette(true).lanes {
		t.Fatalf("the custom theme did not come back: %q %+v", again.account.themeName, again.account.colors())
	}
}

func TestALightBackgroundMakesALightCustomTheme(t *testing.T) {
	th := customTheme(auth.CustomTheme{Name: "Paper", Base: "dracula", Colors: map[string]string{"shell": "#fafafa", "ink": "#111111"}})
	if th.dark != nil || th.light == nil || th.palette(true).shell != "#fafafa" || th.palette(true).brand != "#ff2d20" {
		t.Fatalf("a light background should make a light theme with the fixed brand: %+v", th)
	}
}

func TestTheEditorRefusesAMissingOrRepeatedNameAndABadColor(t *testing.T) {
	m := themeApp(t)
	m.tuiSettings.CustomThemes = []auth.CustomTheme{{Name: "Mine", Base: "dark", Colors: map[string]string{}}}
	m.applyCustomThemes()
	m.newThemeEditor("dark")
	m = pressKey(m, "ctrl+s")
	if m.themeEditor == nil || !strings.Contains(frameText(m), "Name the theme") {
		t.Fatalf("an empty name saved:\n%s", frameText(m))
	}
	m = typeText(m, "mine")
	m = pressKey(m, "ctrl+s")
	if m.themeEditor == nil || !strings.Contains(frameText(m), "already have a theme named") {
		t.Fatalf("a repeated name saved:\n%s", frameText(m))
	}
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "Other")
	m = pressKey(m, "down")
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "#12")
	m = pressKey(m, "ctrl+s")
	if m.themeEditor == nil || !strings.Contains(frameText(m), "Background needs a color like #1a2b3c") {
		t.Fatalf("a bad color saved:\n%s", frameText(m))
	}
	if len(m.tuiSettings.CustomThemes) != 1 {
		t.Fatalf("refused saves changed the list: %+v", m.tuiSettings.CustomThemes)
	}
}

func TestCustomThemesCanBeEditedAndDeletedButNotTheOneInUse(t *testing.T) {
	m := themeApp(t)
	m.tuiSettings.CustomThemes = []auth.CustomTheme{{Name: "Used", Base: "dark", Colors: map[string]string{}}, {Name: "Spare", Base: "light", Colors: map[string]string{}}}
	m.applyCustomThemes()
	m.setTheme("custom:Used")
	m = pressKey(m, "f2")
	if frame := frameText(m); !strings.Contains(frame, "Custom themes") || !strings.Contains(frame, "Spare") || !strings.Contains(frame, "Edit  e") || !strings.Contains(frame, "Delete  D") {
		t.Fatalf("the picker does not list custom themes with their actions:\n%s", frame)
	}
	selectRow(t, m, "custom:Used")
	m = pressKey(m, "D")
	if m.ask != nil || !strings.Contains(frameText(m), "Pick another theme before deleting this one") {
		t.Fatalf("deleting the theme in use was not blocked:\n%s", frameText(m))
	}
	selectRow(t, m, "custom:Spare")
	m = pressKey(m, "e")
	if m.themeEditor == nil || m.themeEditor.values[0] != "Spare" {
		t.Fatalf("e did not edit the highlighted custom theme:\n%s", frameText(m))
	}
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "Renamed")
	m = pressKey(m, "ctrl+s")
	if names := customNames(m); strings.Join(names, ",") != "Used,Renamed" {
		t.Fatalf("editing should rename in place: %v", names)
	}
	m.openThemes()
	selectRow(t, m, "custom:Renamed")
	m = pressKey(m, "D")
	if m.ask == nil || m.ask.kind != "delete-theme" || m.ask.focus != 0 {
		t.Fatalf("D did not ask to delete, starting on Cancel:\n%s", frameText(m))
	}
	m = pressKey(m, "ctrl+y")
	if names := customNames(m); strings.Join(names, ",") != "Used" || m.account.themeName != "custom:Used" {
		t.Fatalf("delete removed the wrong theme: %v %q", names, m.account.themeName)
	}
}

func TestTheTwentyFirstCustomThemeIsRefused(t *testing.T) {
	m := themeApp(t)
	for i := range maxCustomThemes {
		m.tuiSettings.CustomThemes = append(m.tuiSettings.CustomThemes, auth.CustomTheme{Name: fmt.Sprint("Theme ", i), Base: "dark", Colors: map[string]string{}})
	}
	m.applyCustomThemes()
	m = pressKey(m, "f2")
	m = pressKey(m, "n")
	if m.picker == nil || m.picker.kind != "theme" || !strings.Contains(frameText(m), "You can keep up to 20 custom themes") {
		t.Fatalf("a 21st theme was allowed:\n%s", frameText(m))
	}
}

func customNames(m AppModel) []string {
	var names []string
	for _, c := range m.tuiSettings.CustomThemes {
		names = append(names, c.Name)
	}
	return names
}

func TestTheEditorKeepsItsErrorInViewWhenManyPairsAreHardToRead(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(AppModel)
	m.newThemeEditor("dracula")
	m = pressKey(m, "down")
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "#ffffff")
	m = pressKey(m, "ctrl+s")
	frame := frameText(m)
	if !strings.Contains(frame, "Name the theme.") || !strings.Contains(frame, "more") {
		t.Fatalf("the error must stay visible and the cut pairs must be counted:\n%s", frame)
	}
}

func TestTheSavedCustomThemeStaysInUseDuringAThemeFlagRun(t *testing.T) {
	// The fixture runs as with --theme dark, while tui.json holds the custom theme.
	m := themeApp(t)
	m.tuiSettings.Theme = "custom:Mine"
	m.tuiSettings.CustomThemes = []auth.CustomTheme{{Name: "Mine", Base: "dark", Colors: map[string]string{}}}
	m.applyCustomThemes()
	m = pressKey(m, "f2")
	selectRow(t, m, "custom:Mine")
	m = pressKey(m, "D")
	if m.ask != nil || !strings.Contains(frameText(m), "Pick another theme") {
		t.Fatalf("the saved theme must not be deletable under --theme:\n%s", frameText(m))
	}
	m = pressKey(m, "e")
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "Renamed")
	m = pressKey(m, "ctrl+s")
	if m.tuiSettings.Theme != "custom:Renamed" || m.account.themeName == "custom:Renamed" {
		t.Fatalf("a rename must follow the saved choice without applying it: saved %q shown %q", m.tuiSettings.Theme, m.account.themeName)
	}
}

func TestANarrowEditorStillCountsTheHardToReadPairs(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(AppModel)
	m.newThemeEditor("dracula")
	m = pressKey(m, "down")
	m = pressKey(m, "ctrl+u")
	m = typeText(m, "#ffffff")
	if frame := frameText(m); !strings.Contains(frame, "color pairs are hard to read") {
		t.Fatalf("a narrow editor must still warn:\n%s", frame)
	}
}

func TestANewThemeFromTheCurrentOneKeepsItsBuiltInBase(t *testing.T) {
	m := themeApp(t)
	m.tuiSettings.CustomThemes = []auth.CustomTheme{{Name: "Mine", Base: "solarized-light", Colors: map[string]string{"shell": "#fdf6e4"}}}
	m.applyCustomThemes()
	m.setTheme("custom:Mine")
	m.newThemeEditor(currentBase)
	if m.themeEditor.base != "solarized-light" || m.themeEditor.values[1] != "#fdf6e4" {
		t.Fatalf("a copy of a custom theme must start from its colors and keep its base: %q %q", m.themeEditor.base, m.themeEditor.values[1])
	}
	m.setTheme("default")
	m.newThemeEditor(currentBase)
	if want := map[bool]string{true: "dark", false: "light"}[m.account.isDark()]; m.themeEditor.base != want {
		t.Fatalf("a copy of Auto must start from the mode it shows: %q, want %q", m.themeEditor.base, want)
	}
	m.newThemeEditor("light")
	if m.themeEditor.base != "light" || m.themeEditor.values[1] != themeNamed("light").palette(false).shell {
		t.Fatalf("Default light must start from the light colors: %q", m.themeEditor.values[1])
	}
}
