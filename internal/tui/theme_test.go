package tui

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestEventThemesFollowTheWebDates(t *testing.T) {
	for _, tc := range []struct {
		day   string
		event string
	}{
		{"2026-10-28", ""}, {"2026-10-29", "halloween"}, {"2026-11-01", "halloween"}, {"2026-11-02", ""},
		{"2026-12-22", ""}, {"2026-12-23", "christmas"}, {"2026-12-26", "christmas"}, {"2026-12-27", ""},
		{"2026-12-30", "newyear"}, {"2027-01-02", "newyear"}, {"2027-01-03", ""},
	} {
		day, _ := time.Parse(time.DateOnly, tc.day)
		if got := eventFor("auto", day); got != tc.event {
			t.Errorf("%s: got %q, want %q", tc.day, got, tc.event)
		}
	}
	halloween := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if eventFor("default", halloween) != "" {
		t.Fatal("Default must never show an event theme")
	}
	if eventFor("christmas", halloween) != "christmas" {
		t.Fatal("a fixed event must show on any date")
	}
	if eventFor("", halloween) != "" || eventFor("bogus", halloween) != "" || eventLabel("") != "Default" {
		t.Fatal("a missing or unknown choice must behave like Default")
	}
}

func TestEveryThemeFillsEveryPaletteSlotWithReadableColors(t *testing.T) {
	hex := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	all := append(append([]theme(nil), themes...), eventThemes...)
	for _, th := range all {
		for _, dark := range []bool{false, true} {
			p := th.palette(dark)
			v := reflect.ValueOf(p)
			for i := range v.NumField() {
				field := v.Field(i)
				values := []string{}
				if field.Kind() == reflect.Array {
					for j := range field.Len() {
						values = append(values, field.Index(j).String())
					}
				} else {
					values = append(values, field.String())
				}
				name := v.Type().Field(i).Name
				for _, value := range values {
					if (name == "graph" || name == "peak") && value == "" {
						continue
					}
					if !hex.MatchString(value) {
						t.Errorf("%s dark=%v: %s is %q", th.name, dark, name, value)
					}
				}
			}
			if c := contrast(p.ink, p.shell); c < 7 {
				t.Errorf("%s dark=%v: ink contrast %.2f", th.name, dark, c)
			}
			if c := contrast(p.muted, p.shell); c < 4.5 {
				t.Errorf("%s dark=%v: muted contrast %.2f", th.name, dark, c)
			}
			if c := contrast(p.blue, p.soft); c < 3 {
				t.Errorf("%s dark=%v: chip key contrast %.2f", th.name, dark, c)
			}
		}
	}
	if !reflect.DeepEqual(themeNamed("default").palette(true), colors(true)) || !reflect.DeepEqual(themeNamed("missing").palette(false), colors(false)) {
		t.Fatal("Default must keep today's palettes, and an unknown name must fall back to it")
	}
}

func TestEventThemeReplacesTheSurfaceColorsAndKeepsLanes(t *testing.T) {
	m := fixtureApp(t)
	m.tuiSettings.EventTheme = "halloween"
	m.applyEventTheme()
	p := m.account.colors()
	if p.shell != "#120c14" || p.blue != "#ff8a4c" || p.lanes != colors(true).lanes {
		t.Fatalf("dark Halloween palette not applied: %+v", p)
	}
	m.account.dark = false
	if m.account.colors().shell != "#fbf5ee" {
		t.Fatal("light Halloween palette not applied")
	}
	m.tuiSettings.EventTheme = "default"
	m.applyEventTheme()
	if !reflect.DeepEqual(m.account.colors(), colors(false)) {
		t.Fatal("Default must drop the event palette")
	}
}

func TestThemeModeIsSavedAndRestoredUnlessTheFlagOverridesIt(t *testing.T) {
	store := auth.Store{Dir: t.TempDir()}
	m := NewApp("", store, "https://example.test", t.Context())
	t.Cleanup(func() { _ = m.Close() })
	if m.account.theme != "auto" {
		t.Fatalf("no saved mode must start in auto, got %q", m.account.theme)
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	m = updated.(AppModel)
	if cmd == nil {
		t.Fatal("F2 must save the mode")
	}
	m.Update(cmd())
	if got := store.TUISettings().Mode; got != m.account.theme || (got != "light" && got != "dark") {
		t.Fatalf("saved mode %q, session %q", got, m.account.theme)
	}
	saved := store.TUISettings().Mode
	again := NewApp("", store, "https://example.test", t.Context())
	t.Cleanup(func() { _ = again.Close() })
	if again.account.theme != saved || again.account.dark != (saved == "dark") {
		t.Fatalf("saved mode not restored: %q", again.account.theme)
	}
	flagged := NewApp("auto", store, "https://example.test", t.Context())
	t.Cleanup(func() { _ = flagged.Close() })
	if flagged.account.theme != "auto" {
		t.Fatal("--theme must override the saved mode")
	}
}

func TestAppearanceSettingsCycleModePickEventThemeAndSaveThem(t *testing.T) {
	m := fixtureApp(t)
	m.now = time.Date(2026, 12, 24, 12, 0, 0, 0, time.Local)
	m.tuiSettings.EventTheme = "auto"
	m.applyEventTheme()
	m.account.theme, m.account.dark = "dark", true
	m.settings.tab = settingsAppearance
	titles := func() []string {
		var out []string
		for _, row := range m.settingsRows() {
			out = append(out, row.title)
		}
		return out
	}
	if got := titles(); !reflect.DeepEqual(got[:2], []string{"Mode: Dark", "Event theme: Auto (Christmas now)"}) {
		t.Fatalf("unexpected rows: %q", got)
	}
	run := func(action string) {
		if cmd := m.activateSetting(action, ""); cmd != nil {
			for _, msg := range collect(cmd) {
				updated, _ := m.Update(msg)
				m = updated.(AppModel)
			}
		}
	}
	run("theme-mode")
	if m.account.theme != "auto" {
		t.Fatalf("Dark must cycle to Auto, got %q", m.account.theme)
	}
	run("event-theme")
	if m.picker == nil || m.picker.kind != "event-theme" || m.picker.rows[m.picker.selected].value != "auto" || !strings.Contains(frameText(m), "Christmas now") {
		t.Fatal("Event theme must open a picker on the current choice")
	}
	m = pressApp(m, "j")
	m = pressApp(m, "esc")
	if m.picker != nil || m.tuiSettings.EventTheme != "auto" {
		t.Fatal("esc must close the picker without a change")
	}
	pick := func(value string) {
		run("event-theme")
		for m.picker.rows[m.picker.selected].value != value {
			m = pressApp(m, "j")
		}
		next, cmd := m.Update(keyMsg("enter"))
		m = next.(AppModel)
		for _, msg := range collect(cmd) {
			updated, _ := m.Update(msg)
			m = updated.(AppModel)
		}
	}
	pick("default")
	if m.picker != nil || m.tuiSettings.EventTheme != "default" || m.account.event != "" {
		t.Fatalf("Default must drop the event: %q %q", m.tuiSettings.EventTheme, m.account.event)
	}
	pick("halloween")
	if m.tuiSettings.EventTheme != "halloween" || m.account.event != "halloween" {
		t.Fatal("choosing Halloween must apply it")
	}
	saved := m.account.store.TUISettings()
	if saved.EventTheme != "halloween" || saved.Mode != "auto" {
		t.Fatalf("choices not saved: %+v", saved)
	}
}

// collect runs a command and any batch it returns, for settings commands that only write files.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collect(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestChristmasRecolorsTheActivityGraphWithARedPeak(t *testing.T) {
	p := themeNamed("christmas").palette(true)
	if activityColor(1, "#da77f2", p) != mixColor("#51cf66", p.shell, activityMix[1]) {
		t.Fatal("Christmas must draw the graph green instead of the lane")
	}
	if activityColor(4, "#da77f2", p) != mixColor("#ff6b6b", p.shell, 1) {
		t.Fatal("the busiest days must use the peak color")
	}
	if activityColor(4, "#da77f2", colors(true)) != mixColor("#da77f2", colors(true).shell, 1) {
		t.Fatal("other themes keep the profile's lane")
	}
}

func TestNewYearGreetsTheComingYearInTheAside(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 45
	m.now = time.Date(2026, 12, 31, 12, 0, 0, 0, time.Local)
	m.tuiSettings.EventTheme = "auto"
	m.applyEventTheme()
	aside := ansi.Strip(m.aside(40, 45, m.account.colors()))
	if !strings.Contains(aside, "// the year is over") || !strings.Contains(aside, `console.log("Happy 2027!");`) {
		t.Fatalf("New Year greeting missing:\n%s", aside)
	}
	m.now = time.Date(2027, 1, 2, 12, 0, 0, 0, time.Local)
	if aside := ansi.Strip(m.aside(40, 45, m.account.colors())); !strings.Contains(aside, "Happy 2027!") {
		t.Fatal("January greets the year that just started")
	}
	m.now = time.Date(2027, 1, 3, 12, 0, 0, 0, time.Local)
	m.applyEventTheme()
	if strings.Contains(ansi.Strip(m.aside(40, 45, m.account.colors())), "Happy") {
		t.Fatal("the greeting must leave with the theme")
	}
}

func TestHalloweenTurnsNotFoundIntoGhostNotFound(t *testing.T) {
	m := fixtureApp(t)
	missing := &auth.APIError{Status: 404}
	if got := m.ghostly(missing, "This profile doesn't exist."); got != "This profile doesn't exist." {
		t.Fatalf("no event must keep the text, got %q", got)
	}
	m.tuiSettings.EventTheme = "halloween"
	m.applyEventTheme()
	if got := m.ghostly(missing, "x"); !strings.HasPrefix(got, "Ghost not found.") {
		t.Fatalf("Halloween 404 text: %q", got)
	}
	if got := m.ghostly(&auth.APIError{Status: 403}, "kept"); got != "kept" {
		t.Fatal("only a 404 is a ghost")
	}
}

func TestHalloweenDecoratesOnlyWideScreensAndTheBatCrossesNowAndThen(t *testing.T) {
	m := fixtureApp(t)
	m.tuiSettings.EventTheme = "halloween"
	m.applyEventTheme()
	m.account.width, m.account.height = 160, 45
	m.reflow()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "\U0001f383 \U0001f480") || !strings.Contains(strings.Split(view, "\n")[0], "\U0001f578") {
		t.Fatal("wide Halloween screens need the cobweb, pumpkin and skull")
	}
	if cmd := m.startBat(); cmd == nil {
		t.Fatal("a focused wide Halloween screen must start the bat")
	}
	if m.startBat() != nil {
		t.Fatal("only one bat chain may run")
	}
	if _, _, ok := batSpot(time.Unix(1000*50+10, 0), 160); ok {
		t.Fatal("the bat hides for most of each cycle")
	}
	x0, _, ok0 := batSpot(time.Unix(1000*50+40, 0), 160)
	x1, y1, ok1 := batSpot(time.Unix(1000*50+45, 0), 160)
	if !ok0 || !ok1 || x1 <= x0 || y1 < 3 {
		t.Fatalf("the bat must fly left to right: %d -> %d (row %d)", x0, x1, y1)
	}
	m.focused = false
	if m.acceptBat(batTick{session: m.session, at: time.Now()}) != nil || m.batRunning {
		t.Fatal("an unfocused terminal stops the bat")
	}
	m.account.width = 90
	m.reflow()
	if strings.Contains(ansi.Strip(m.View().Content), "\U0001f383") {
		t.Fatal("narrow screens stay plain, like the web below 80rem")
	}
}

func TestActionChipsShareTheCommitChipLook(t *testing.T) {
	p := colors(true)
	chip := actionChip("Follow", "f", p)
	if ansi.Strip(chip) != " Follow  f " {
		t.Fatalf("chip text: %q", ansi.Strip(chip))
	}
	lines, _ := commitChips([]commitAction{{"Follow", "f"}}, 40, p)
	if lines[1] != chip {
		t.Fatal("commit chips and other actions must render the same chip")
	}
	if danger := dangerChip("Block", "B", p); danger == actionChip("Block", "B", p) {
		t.Fatal("a destructive action needs its own label color")
	}
}

func TestThemeLivesInTheTUITabAndPicksEventThemesTooWithATag(t *testing.T) {
	m := fixtureApp(t)
	m.now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.Local)
	m.applyEventTheme()
	m.settings.tab = settingsTUI
	if got := m.settingsRows()[0].title; got != "Theme: Default" {
		t.Fatalf("the TUI tab must start with Theme, got %q", got)
	}
	m.activateSetting("theme-name", "")
	var labels []string
	for _, row := range m.picker.rows {
		labels = append(labels, row.label)
	}
	if m.picker == nil || m.picker.kind != "theme" || strings.Join(labels, "|") != "TUI themes|Default||Event themes|Halloween|Christmas|New Year" || m.picker.rows[m.picker.selected].value != "default" {
		t.Fatalf("the picker must group TUI themes, then event themes, on the current choice: %q", labels)
	}
	if text := frameText(m); !strings.Contains(text, "Event themes") || !strings.Contains(text, "overrides") {
		t.Fatalf("the picker must show its groups and say an event overrides it:\n%s", text)
	}
	m = pressApp(m, "k")
	if m.picker.rows[m.picker.selected].value != "default" {
		t.Fatal("k must not select a heading")
	}
	m = pressApp(m, "j")
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(AppModel)
	for _, msg := range collect(cmd) {
		updated, _ := m.Update(msg)
		m = updated.(AppModel)
	}
	if m.account.themeName != "halloween" || m.account.store.TUISettings().Theme != "halloween" || m.account.colors().shell != themeNamed("halloween").palette(m.account.dark).shell {
		t.Fatal("choosing Halloween must apply and save it")
	}
	m.now = time.Date(2026, 12, 24, 12, 0, 0, 0, time.Local)
	m.tuiSettings.EventTheme = "auto"
	m.applyEventTheme()
	if m.account.colors().shell != themeNamed("christmas").palette(m.account.dark).shell || !strings.Contains(m.settingsRows()[0].title, "Christmas event theme on now") {
		t.Fatal("an active event theme must override the chosen theme and say so")
	}
}
