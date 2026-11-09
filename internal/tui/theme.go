package tui

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type palette struct {
	shell, ink, muted, hairline, blue, wash, soft, code, danger, success, warning, brand string
	lanes                                                                                [6]string
	// graph and peak recolor the activity graph, like the web's Christmas theme. Empty keeps the profile's lane.
	graph, peak string
}

var defaultDark = palette{
	shell: "#111112", ink: "#ececec", muted: "#a1a1a6",
	hairline: "#2a2a2d", blue: "#7aa2f7", wash: "#232326",
	soft: "#18233a", code: "#e5c07b", danger: "#ff6b6b",
	success: "#9ece6a", warning: "#e0af68", brand: "#ff2d20",
	lanes: [6]string{"#ff8a4c", "#51cf66", "#da77f2", "#3bc9db", "#f06595", "#fcc419"},
}

var defaultLight = palette{
	shell: "#f6f7f8", ink: "#121417", muted: "#5b616b",
	hairline: "#e1e4e8", blue: "#3b6fe0", wash: "#eef0f2",
	soft: "#e8effc", code: "#875f15", danger: "#c92a2a",
	success: "#2f8132", warning: "#9a6700", brand: "#ff2d20",
	lanes: [6]string{"#d9480f", "#2f9e44", "#9c36b5", "#0c8599", "#c2255c", "#b7791f"},
}

// theme is a named look with a dark palette, a light palette, or both. F2 switches between them when both exist.
type theme struct {
	name, label string
	dark, light *palette
}

// themes are the ones Settings lists. TUI-only themes such as Dracula or Catppuccin are added here.
var themes = []theme{{name: "default", label: "Default", dark: &defaultDark, light: &defaultLight}}

// eventThemes mirror the web's resources/css/app.css: the surface colors change, and lanes and status colors stay.
var eventThemes = []theme{
	surface("halloween", "Halloween",
		[7]string{"#120c14", "#f1e9f5", "#ae9fb8", "#2c2033", "#ff8a4c", "#241a2b", "#2e1a12"},
		[7]string{"#fbf5ee", "#1f1710", "#6b5a4a", "#eadccd", "#c2410c", "#f3e9de", "#fdeadb"}, "", ""),
	surface("christmas", "Christmas",
		[7]string{"#0e1512", "#ecf1ee", "#9fb0a6", "#243029", "#ff7b7b", "#1c2823", "#2b1717"},
		[7]string{"#f8f6f1", "#161a17", "#5e5a50", "#e3e0d6", "#b3262c", "#efece4", "#fbe9e9"}, "#51cf66/#ff6b6b", "#2f9e44/#c92a2a"),
	surface("newyear", "New Year",
		[7]string{"#0b0d17", "#eef0fa", "#9aa0c4", "#232744", "#f5c542", "#181c36", "#2a2410"},
		[7]string{"#faf7ef", "#1c1807", "#6b5f3e", "#ece3c8", "#a16207", "#f2ecda", "#fbf0d2"}, "", ""),
}

// surface builds an event theme from the web's background, foreground, muted-foreground, border, primary, muted, and
// primary-soft values, with optional "lane/peak" activity graph colors.
func surface(name, label string, dark, light [7]string, darkGraph, lightGraph string) theme {
	build := func(base palette, c [7]string, graph string) *palette {
		base.shell, base.ink, base.muted, base.hairline, base.blue, base.wash, base.soft = c[0], c[1], c[2], c[3], c[4], c[5], c[6]
		if graph != "" {
			base.graph, base.peak = graph[:7], graph[8:]
		}
		return &base
	}
	return theme{name: name, label: label, dark: build(defaultDark, dark, darkGraph), light: build(defaultLight, light, lightGraph)}
}

func themeNamed(name string) theme {
	for _, th := range append(append([]theme(nil), themes...), eventThemes...) {
		if th.name == name {
			return th
		}
	}
	return themes[0]
}

// palette returns the requested mode, or the theme's only one.
func (t theme) palette(dark bool) palette {
	if (dark && t.dark != nil) || t.light == nil {
		return *t.dark
	}
	return *t.light
}

func (t theme) bothModes() bool { return t.dark != nil && t.light != nil }

// colors is the Default palette, for code that draws before an account exists.
func colors(dark bool) palette { return themes[0].palette(dark) }

// colors is the palette on screen: the event theme while one is active, otherwise the chosen theme.
func (m AccountModel) colors() palette {
	if m.event != "" {
		return themeNamed(m.event).palette(m.dark)
	}
	return themeNamed(m.themeName).palette(m.dark)
}

// eventFor picks the event theme like the web's EventTheme::current: a fixed choice always shows, and Auto follows the
// dates, from two days before each day to the day after it. Default, a missing choice, or an unknown one never shows.
func eventFor(choice string, now time.Time) string {
	switch choice {
	case "halloween", "christmas", "newyear":
		return choice
	case "auto":
	default:
		return ""
	}
	day := int(now.Month())*100 + now.Day()
	switch {
	case day >= 1029 && day <= 1101:
		return "halloween"
	case day >= 1223 && day <= 1226:
		return "christmas"
	case day >= 1230 || day <= 102:
		return "newyear"
	}
	return ""
}

// readerNow is the clock in the account's time zone once Settings loaded it, otherwise the local one.
func (m AppModel) readerNow() time.Time {
	if zone := m.settings.region.Timezone; zone != "" {
		if location, err := time.LoadLocation(zone); err == nil {
			return m.now.In(location)
		}
	}
	return m.now.Local()
}

func (m *AppModel) applyEventTheme() {
	m.account.event = eventFor(m.tuiSettings.EventTheme, m.readerNow())
}

func text(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

func (m AccountModel) nextTheme() string {
	if m.dark {
		return "light"
	}
	return "dark"
}

// toggleTheme switches dark and light, and reports false for a theme with only one mode.
func (m *AccountModel) toggleTheme() bool {
	if !themeNamed(m.themeName).bothModes() {
		return false
	}
	m.theme = m.nextTheme()
	m.dark = !m.dark
	return true
}

// canvas paints every cell so screens never depend on the terminal honouring OSC 11.
func canvas(page string, p palette, width, height int) string {
	frame := lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).Width(width).Height(height).MaxWidth(width).MaxHeight(height)
	return withBackground(frame.Render(page), p.shell)
}

// toggleTheme is F2 everywhere in the app: it switches dark and light and saves the mode.
func (m *AppModel) toggleTheme() tea.Cmd {
	if !m.account.toggleTheme() {
		return m.notify("info", "One mode only", themeNamed(m.account.themeName).label+" has no "+m.account.nextTheme()+" mode.")
	}
	m.resetSequence()
	m.reflow()
	m.tuiSettings.Mode = m.account.theme
	return m.saveTUISettings(false)
}

// cycleMode is Settings' Mode row: Auto, then Light, then Dark.
func (m *AppModel) cycleMode() tea.Cmd {
	next := map[string]string{"auto": "light", "light": "dark", "dark": "auto"}[m.account.theme]
	m.account.theme, m.tuiSettings.Mode = next, next
	var cmd tea.Cmd
	if next == "auto" {
		// Auto follows the terminal background again, so ask for it.
		m.account.backgroundKnown = false
		cmd = tea.RequestBackgroundColor
	} else {
		m.account.dark = next == "dark"
	}
	m.reflow()
	return tea.Batch(cmd, m.saveTUISettings(false))
}

var eventChoices = []string{"auto", "default", "halloween", "christmas", "newyear"}

// openEventThemes is Settings' Event theme row: a picker in the web's order, Auto, Default, then each event.
func (m *AppModel) openEventThemes() {
	current := m.tuiSettings.EventTheme
	if current == "" {
		current = "default"
	}
	notes := map[string]string{
		"auto":      "Follows the dates",
		"default":   "Never shows an event",
		"halloween": "Oct 29 to Nov 1",
		"christmas": "Dec 23 to 26",
		"newyear":   "Dec 30 to Jan 2",
	}
	if m.account.event != "" {
		notes["auto"] += ", " + themeNamed(m.account.event).label + " now"
	}
	d := &picker{kind: "event-theme", title: "/dev/null · EVENT THEME", intro: "Event colors, like the web. Saved in tui.json."}
	for i, choice := range eventChoices {
		d.rows = append(d.rows, pickerRow{label: eventLabel(choice), note: notes[choice], value: choice})
		if choice == current {
			d.selected = i
		}
	}
	m.picker = d
}

// setEventTheme applies and saves an event theme choice.
func (m *AppModel) setEventTheme(choice string) tea.Cmd {
	m.tuiSettings.EventTheme = choice
	m.applyEventTheme()
	m.reflow()
	return tea.Batch(m.saveTUISettings(false), m.startBat())
}

// openThemes is the TUI tab's Theme row: the TUI's themes, then the event themes, which an active event overrides.
func (m *AppModel) openThemes() {
	d := &picker{kind: "theme", title: "/dev/null · THEME", intro: "An active event theme overrides this choice."}
	groups := []struct {
		label string
		list  []theme
	}{{"TUI themes", themes}, {"Event themes", eventThemes}}
	for g, group := range groups {
		if g > 0 {
			d.rows = append(d.rows, pickerRow{heading: true})
		}
		d.rows = append(d.rows, pickerRow{label: group.label, heading: true})
		for _, th := range group.list {
			if th.name == m.account.themeName {
				d.selected = len(d.rows)
			}
			d.rows = append(d.rows, pickerRow{label: th.label, value: th.name})
		}
	}
	if d.rows[d.selected].heading {
		d.first()
	}
	m.picker = d
}

// setTheme applies and saves a theme choice.
func (m *AppModel) setTheme(name string) tea.Cmd {
	next := themeNamed(name)
	m.account.themeName, m.tuiSettings.Theme = next.name, next.name
	if !next.bothModes() {
		m.account.dark = next.dark != nil
	}
	m.reflow()
	return m.saveTUISettings(false)
}

func eventLabel(name string) string {
	switch name {
	case "auto":
		return "Auto"
	case "", "default":
		return "Default"
	}
	return themeNamed(name).label
}

// newYearGreeting is the web's new-year-snippet.tsx, shown in the aside while the New Year theme is on.
func (m AppModel) newYearGreeting(width int, p palette) string {
	if m.account.event != "newyear" {
		return ""
	}
	now := m.readerNow()
	year := now.Year()
	if now.Month() == time.December {
		year++
	}
	comment := text(p.muted).Render(fit("// the year is over", width))
	call := text(p.ink).Render("console.") + text(p.blue).Render("log") + text(p.ink).Render("(") +
		text(p.success).Render(fmt.Sprintf(`"Happy %d!"`, year)) + text(p.ink).Render(");")
	return comment + "\n" + call
}

// ghostly swaps a 404's text for the web's Halloween "Ghost not found" copy while that theme is on.
func (m AppModel) ghostly(err error, status string) string {
	var failure *auth.APIError
	if m.account.event != "halloween" || !errors.As(err, &failure) || failure.Status != http.StatusNotFound {
		return status
	}
	return "Ghost not found. Whatever lived here has passed to the other side."
}

// batTick moves Halloween's bat one step a second.
type batTick struct {
	session int
	at      time.Time
}

// haunted reports whether Halloween's decorations show: the theme is on and the screen is wide enough for the aside,
// like the web's 80rem breakpoint.
func (m AppModel) haunted() bool {
	_, _, aside := m.columns()
	return m.account.event == "halloween" && aside > 0 && m.connected && !m.accountOpen
}

func (m *AppModel) startBat() tea.Cmd {
	if m.batRunning || !m.haunted() || !m.focused {
		return nil
	}
	m.batRunning = true
	return m.nextBat()
}

func (m AppModel) nextBat() tea.Cmd {
	session := m.session
	return tea.Tick(time.Second, func(at time.Time) tea.Msg { return batTick{session: session, at: at} })
}

// acceptBat keeps the chain going only while the bat can show, so an unfocused or narrow terminal stops redrawing.
func (m *AppModel) acceptBat(msg batTick) tea.Cmd {
	if msg.session != m.session {
		m.batRunning = false
		return m.startBat()
	}
	if !m.haunted() || !m.focused {
		m.batRunning, m.batAt = false, time.Time{}
		return nil
	}
	m.batAt = msg.at
	return m.nextBat()
}

// batSpot follows the web's bat-fly keyframes: hidden for 40 of every 50 seconds, then a 10 second dip across the
// screen through rows 4, 7, 3 and 8.
func batSpot(at time.Time, width int) (x, y int, ok bool) {
	if at.IsZero() || width < 4 {
		return 0, 0, false
	}
	phase := at.Unix() % 50
	if phase < 40 {
		return 0, 0, false
	}
	progress := float64(phase-40) / 10
	rows := [5]int{4, 7, 3, 8, 4}
	return min(width-2, int(progress*float64(width))), rows[int(progress*4)], true
}

// haunt draws the web's Halloween decorations over the workspace: a cobweb top right, a pumpkin and a skull bottom
// right above the footer, and the bat. They take no clicks, because hit areas come from the layouts underneath.
func (m AppModel) haunt(content string, p palette) string {
	if !m.haunted() {
		return content
	}
	width, height := m.account.width, m.account.height
	emoji := lipgloss.NewStyle().Background(lipgloss.Color(p.shell))
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(content),
		lipgloss.NewLayer(emoji.Render("\U0001f578\ufe0f")).X(width - 3).Y(0),
		lipgloss.NewLayer(emoji.Render("\U0001f383 \U0001f480")).X(width - 7).Y(height - 3),
	}
	if x, y, ok := batSpot(m.batAt, width); ok && y < height-1 {
		layers = append(layers, lipgloss.NewLayer(emoji.Render("\U0001f987")).X(x).Y(y))
	}
	return lipgloss.NewCompositor(layers...).Render()
}
