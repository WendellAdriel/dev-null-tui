package tui

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const (
	maxCustomThemes    = 20
	maxThemeNameLength = 32
	customThemePrefix  = "custom:"
)

// themeSlot is one color a custom theme sets, in the editor's order. Optional slots may be empty.
type themeSlot struct {
	key, label string
	optional   bool
}

var themeSlots = []themeSlot{
	{key: "shell", label: "Background"}, {key: "ink", label: "Text"}, {key: "muted", label: "Muted text"},
	{key: "hairline", label: "Lines"}, {key: "blue", label: "Links and chip keys"}, {key: "wash", label: "Selected row"},
	{key: "soft", label: "Chip key background"}, {key: "code", label: "Code"}, {key: "danger", label: "Danger"},
	{key: "success", label: "Success"}, {key: "warning", label: "Warning"},
	{key: "lane1", label: "Lane 1"}, {key: "lane2", label: "Lane 2"}, {key: "lane3", label: "Lane 3"},
	{key: "lane4", label: "Lane 4"}, {key: "lane5", label: "Lane 5"}, {key: "lane6", label: "Lane 6"},
	{key: "graph", label: "Activity graph", optional: true}, {key: "peak", label: "Activity peak", optional: true},
}

var hexValue = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// slotColor points at a palette's color for a slot key.
func slotColor(p *palette, key string) *string {
	switch key {
	case "shell":
		return &p.shell
	case "ink":
		return &p.ink
	case "muted":
		return &p.muted
	case "hairline":
		return &p.hairline
	case "blue":
		return &p.blue
	case "wash":
		return &p.wash
	case "soft":
		return &p.soft
	case "code":
		return &p.code
	case "danger":
		return &p.danger
	case "success":
		return &p.success
	case "warning":
		return &p.warning
	case "graph":
		return &p.graph
	case "peak":
		return &p.peak
	}
	lane := int(key[len(key)-1] - '1')
	return &p.lanes[lane]
}

// customTheme builds a saved custom theme on its base, which fills any missing or invalid color. Brand stays the
// /dev/null red, and the background decides whether the theme is dark or light.
func customTheme(c auth.CustomTheme) theme {
	p := themeNamed(c.Base).palette(true)
	for _, slot := range themeSlots {
		raw, set := c.Colors[slot.key]
		if value := normalHex(raw); value != "" || (slot.optional && set && strings.TrimSpace(raw) == "") {
			*slotColor(&p, slot.key) = value
		}
	}
	p.brand = defaultDark.brand
	th := theme{name: customThemePrefix + c.Name, label: c.Name, note: "Custom"}
	if darkColor(p.shell) {
		th.dark = &p
	} else {
		th.light = &p
	}
	return th
}

// darkColor reports that white text reads better than black on the color.
func darkColor(hex string) bool { return contrast(hex, "#ffffff") >= contrast(hex, "#000000") }

// readabilityIssues lists the pairs that miss the contrast the built-in themes keep: text 7, muted text and status
// colors 4.5, chip keys and lanes 3.
func readabilityIssues(p palette) []string {
	checks := []struct {
		label, color, on, onLabel string
		least                     float64
	}{
		{"Text", p.ink, p.shell, "Background", 7},
		{"Muted text", p.muted, p.shell, "Background", 4.5},
		{"Links and chip keys", p.blue, p.soft, "Chip key background", 3},
		{"Code", p.code, p.shell, "Background", 4.5},
		{"Danger", p.danger, p.shell, "Background", 4.5},
		{"Success", p.success, p.shell, "Background", 4.5},
		{"Warning", p.warning, p.shell, "Background", 4.5},
	}
	for i, lane := range p.lanes {
		checks = append(checks, struct {
			label, color, on, onLabel string
			least                     float64
		}{fmt.Sprint("Lane ", i+1), lane, p.shell, "Background", 3})
	}
	var issues []string
	for _, check := range checks {
		if c := contrast(check.color, check.on); c < check.least {
			issues = append(issues, fmt.Sprintf("%s on %s %.1f:1, needs %g", check.label, check.onLabel, c, check.least))
		}
	}
	return issues
}

// lookupTheme finds a theme by name among the user's custom themes, then the built-in ones.
func (m AccountModel) lookupTheme(name string) theme {
	for _, th := range m.custom {
		if th.name == name {
			return th
		}
	}
	return themeNamed(name)
}

// applyCustomThemes rebuilds the custom themes from tui.json's list.
func (m *AppModel) applyCustomThemes() {
	m.account.custom = nil
	for _, c := range m.tuiSettings.CustomThemes {
		m.account.custom = append(m.account.custom, customTheme(c))
	}
}

func (m AppModel) customIndex(name string) int {
	return slices.IndexFunc(m.tuiSettings.CustomThemes, func(c auth.CustomTheme) bool { return customThemePrefix+c.Name == name })
}

// themeEditor is the custom theme form: the name, then one #rrggbb field per slot, edited through one text field.
type themeEditor struct {
	// editing is the saved name of the theme being changed, or empty for a new one.
	editing string
	base    string
	values  []string
	focus   int
	field   textinput.Model
	status  string
	// adjusting shows the hue, saturation and lightness sliders under the focused color, with slider picked.
	adjusting bool
	slider    int
	hsl       [3]int
}

// openThemeBases asks whether a new custom theme starts from the theme in use, or from the default dark or light one.
func (m *AppModel) openThemeBases() {
	if len(m.tuiSettings.CustomThemes) >= maxCustomThemes {
		m.picker.status = fmt.Sprintf("You can keep up to %d custom themes. Delete one to make another.", maxCustomThemes)
		return
	}
	current := m.account.lookupTheme(m.account.themeName)
	note := current.label
	if current.bothModes() {
		note += map[bool]string{true: " (Dark)", false: " (Light)"}[m.account.isDark()]
	}
	m.picker = &picker{kind: "theme-base", title: "/dev/null · NEW THEME", intro: "Pick the colors to start from.", rows: []pickerRow{
		{label: "Current theme", note: note, value: currentBase},
		{label: "Default dark", value: "dark"},
		{label: "Default light", value: "light"},
	}}
}

// currentBase starts a new theme from the theme in use.
const currentBase = "current"

// newThemeEditor opens the form for a new theme, starting from a built-in theme's colors or, for currentBase, the
// theme in use. Its base is the built-in theme behind it, which fills any slot a saved theme lacks.
func (m *AppModel) newThemeEditor(base string) tea.Cmd {
	colors := themeNamed(base).palette(true)
	if base == currentBase {
		current := m.account.lookupTheme(m.account.themeName)
		base, colors = m.account.themeName, current.palette(m.account.isDark())
		switch i := m.customIndex(base); {
		case i >= 0:
			base = m.tuiSettings.CustomThemes[i].Base
		case current.bothModes():
			base = map[bool]string{true: "dark", false: "light"}[m.account.isDark()]
		}
	}
	return m.openThemeEditor(&themeEditor{base: base}, colors)
}

// editThemeEditor opens the form on a saved custom theme, picked by its full "custom:" name.
func (m *AppModel) editThemeEditor(full string) tea.Cmd {
	i := m.customIndex(full)
	if i < 0 {
		return nil
	}
	saved := m.tuiSettings.CustomThemes[i]
	e := &themeEditor{editing: saved.Name, base: saved.Base, values: []string{saved.Name}}
	return m.openThemeEditor(e, m.account.lookupTheme(full).palette(true))
}

func (m *AppModel) openThemeEditor(e *themeEditor, colors palette) tea.Cmd {
	if len(e.values) == 0 {
		e.values = []string{""}
	}
	for _, slot := range themeSlots {
		e.values = append(e.values, *slotColor(&colors, slot.key))
	}
	m.picker, m.themeEditor = nil, e
	return e.focusField(0)
}

func (e *themeEditor) focusField(i int) tea.Cmd {
	e.adjusting = false
	if e.field.Focused() {
		e.values[e.focus] = e.field.Value()
	}
	e.focus = min(max(0, i), len(e.values)-1)
	e.field = textinput.New()
	e.field.Prompt, e.field.CharLimit = "", 7
	e.field.SetWidth(maxThemeNameLength)
	if e.focus == 0 {
		e.field.CharLimit, e.field.Placeholder = maxThemeNameLength, "My theme"
	}
	e.field.SetValue(e.values[e.focus])
	e.field.CursorEnd()
	return e.field.Focus()
}

// value is a field's text, reading the one being typed into from the text field.
func (e *themeEditor) value(i int) string {
	if i == e.focus {
		return e.field.Value()
	}
	return e.values[i]
}

// palette is the theme as typed so far, with the base's color wherever a field isn't a valid color yet.
func (e *themeEditor) palette() palette {
	colors := map[string]string{}
	for i, slot := range themeSlots {
		colors[slot.key] = e.value(i + 1)
	}
	return customTheme(auth.CustomTheme{Base: e.base, Colors: colors}).palette(true)
}

func (m *AppModel) themeEditorKey(msg tea.KeyPressMsg) tea.Cmd {
	e := m.themeEditor
	if e.adjusting {
		if cmd, done := m.sliderKey(msg.String()); done {
			return cmd
		}
	}
	switch msg.String() {
	case "esc":
		m.themeEditor = nil
		m.openThemes()
		return nil
	case "enter":
		if e.focus > 0 {
			e.openSliders()
			return nil
		}
		return e.focusField(e.focus + 1)
	case "up", "shift+tab":
		return e.focusField(e.focus - 1)
	case "down", "tab":
		return e.focusField(e.focus + 1)
	case "ctrl+s":
		return m.saveCustomTheme()
	}
	var cmd tea.Cmd
	e.field, cmd = e.field.Update(msg)
	e.status = ""
	return cmd
}

// saveCustomTheme checks the form, then saves the theme in tui.json. A new theme, or the one in use, is applied.
func (m *AppModel) saveCustomTheme() tea.Cmd {
	e := m.themeEditor
	e.values[e.focus], e.adjusting = e.field.Value(), false
	name := strings.TrimSpace(safeText(e.values[0]))
	if name == "" {
		e.status = "Name the theme."
		return nil
	}
	for _, c := range m.tuiSettings.CustomThemes {
		if strings.EqualFold(c.Name, name) && c.Name != e.editing {
			e.status = "You already have a theme named " + c.Name + "."
			return nil
		}
	}
	colors := map[string]string{}
	for i, slot := range themeSlots {
		value := normalHex(e.values[i+1])
		if value == "" && (!slot.optional || strings.TrimSpace(e.values[i+1]) != "") {
			e.status = slot.label + " needs a color like #1a2b3c."
			return nil
		}
		colors[slot.key] = value
	}
	saved := auth.CustomTheme{Name: name, Base: e.base, Colors: colors}
	current := m.account.themeName
	if i := m.customIndex(customThemePrefix + e.editing); e.editing != "" && i >= 0 {
		m.tuiSettings.CustomThemes[i] = saved
	} else {
		m.tuiSettings.CustomThemes = append(m.tuiSettings.CustomThemes, saved)
	}
	m.applyCustomThemes()
	m.themeEditor = nil
	if e.editing == "" || current == customThemePrefix+e.editing {
		return m.setTheme(customThemePrefix + name)
	}
	// Under --theme the saved choice differs from the theme shown, so a rename must follow it in tui.json.
	if m.tuiSettings.Theme == customThemePrefix+e.editing {
		m.tuiSettings.Theme = customThemePrefix + name
	}
	m.reflow()
	return m.saveTUISettings(false)
}

// askDeleteTheme confirms deleting the highlighted custom theme. The theme in use can't be deleted.
func (m *AppModel) askDeleteTheme() {
	d := m.picker
	row := d.rows[d.selected]
	if m.customIndex(row.value) < 0 {
		return
	}
	if m.themeInUse(row.value) {
		d.status = "Pick another theme before deleting this one."
		return
	}
	m.picker = nil
	m.ask = &ask{kind: "delete-theme", title: "/dev/null · DELETE THEME", question: "Delete " + safeText(row.label) + "?",
		detail: "It's removed from tui.json. This can't be undone.", confirm: "Delete", border: m.account.colors().danger, id: row.value}
}

// themeInUse is the theme shown or the one saved in tui.json, which differ for a run with --theme.
func (m AppModel) themeInUse(name string) bool {
	return name == m.account.themeName || name == m.tuiSettings.Theme
}

func (m *AppModel) deleteCustomTheme(name string) tea.Cmd {
	i := m.customIndex(name)
	if i < 0 || m.themeInUse(name) {
		return nil
	}
	m.tuiSettings.CustomThemes = slices.Delete(m.tuiSettings.CustomThemes, i, i+1)
	m.applyCustomThemes()
	m.openThemes()
	return m.saveTUISettings(false)
}

// themeEditorModal draws the form on the left and a preview of the theme as typed on the right, with the pairs that
// are hard to read under it. Fields scroll to keep the focused one visible.
func (m AppModel) themeEditorModal(p palette) (string, uiRect, []hitArea) {
	e := m.themeEditor
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 100, 32)
	width := max(1, r.w-4)
	fieldWidth := min(46, width)
	previewWidth := width - fieldWidth - 3
	if previewWidth < 28 {
		fieldWidth, previewWidth = width, 0
	}
	hints := []statusPart{hintChip("Field", "↑/↓", "down"), statusChip("Adjust", "enter"), statusChip("Save", "ctrl+s"), closeChip("Cancel")}
	switch {
	case e.adjusting:
		hints = []statusPart{hintChip("Slider", "↑/↓", "down"), hintChip("Change", "←/→", "right"), statusChip("Type", "enter"), statusChip("Save", "ctrl+s")}
	case e.focus == 0:
		hints = slices.Delete(hints, 1, 2)
	}
	footer, footerAreas := hintRow(hints, width, p)
	// The same body height panel draws, with the error on top so it never scrolls out of view.
	rows := max(1, r.h-5-lipgloss.Height(footer))
	top := r.y + 3
	var status []string
	switch issues := len(readabilityIssues(e.palette())); {
	case e.status != "":
		status = []string{text(p.danger).Render(fit(e.status, width))}
	case previewWidth == 0 && issues > 0:
		// Without room for the preview, say how many pairs it would flag.
		status = []string{text(p.warning).Render(fit(fmt.Sprintf("%d color pairs are hard to read. Widen the terminal to see them.", issues), width))}
	}
	if status != nil {
		rows, top = max(1, rows-1), top+1
	}
	// Each line is a field, or one of the sliders open under the focused field. The window keeps the focus in view.
	type editorLine struct {
		text          string
		field, slider int
	}
	var all []editorLine
	focusLine, barX, barWidth := 0, 0, 0
	for i := range e.values {
		if i == e.focus {
			focusLine = len(all)
		}
		all = append(all, editorLine{m.themeEditorField(i, fieldWidth, p), i, -1})
		if i == e.focus && e.adjusting {
			var sliders []string
			sliders, barX, barWidth = e.sliderLines(fieldWidth, p)
			for s, line := range sliders {
				all = append(all, editorLine{line, i, s})
			}
			focusLine++
		}
	}
	first := min(max(0, focusLine-rows/2), max(0, len(all)-rows))
	var fields []string
	var areas []hitArea
	for _, line := range all[first:min(len(all), first+rows)] {
		fields = append(fields, line.text)
		y := top + len(fields) - 1
		if line.slider >= 0 {
			areas = append(areas, hitArea{uiRect{r.x + 2 + barX, y, barWidth, 1}, "slider", line.slider})
			continue
		}
		areas = append(areas, hitArea{uiRect{r.x + 2, y, fieldWidth, 1}, "field", line.field})
	}
	body := strings.Join(fields, "\n")
	if previewWidth > 0 {
		focused := ""
		if e.focus > 0 {
			focused = themeSlots[e.focus-1].key
		}
		preview := themePreview(e.palette(), focused, previewWidth, rows, p)
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(fieldWidth).Render(body), "   ", strings.Join(preview, "\n"))
	}
	body = strings.Join(append(status, body), "\n")
	title := "/dev/null · NEW THEME"
	if e.editing != "" {
		title = "/dev/null · EDIT THEME"
	}
	areas = append(areas, panelFooterAreas(footerAreas, footer, r)...)
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel(title, body, footer, r, p, p.blue), r, areas
}

// themeEditorField is one form row: a swatch for colors, the label, and the value, which shows the cursor when focused.
func (m AppModel) themeEditorField(i, width int, p palette) string {
	e := m.themeEditor
	label, swatch := "Name", "  "
	if i > 0 {
		slot := themeSlots[i-1]
		label = slot.label
		if value := normalHex(e.value(i)); value != "" {
			swatch = text(value).Render("██")
		} else {
			swatch = text(p.muted).Render("··")
		}
	}
	value := text(p.ink).Render(safeText(e.values[i]))
	if i == e.focus {
		value = e.field.View()
	} else if e.values[i] == "" && i > 0 {
		value = text(p.muted).Render("none")
	}
	line := swatch + " " + text(p.muted).Width(20).MaxWidth(20).Render(fit(label, 20)) + " " + value
	if i == e.focus {
		return withBackground(fit(line, width), p.wash)
	}
	return fit(line, width)
}

// themePreview draws a few of the app's parts in the theme being made, with a ◂ beside the rows the focused color
// changes, then the hard-to-read pairs in the app's colors, cut to height with a count of the pairs left out.
func themePreview(t palette, focused string, width, height int, p palette) []string {
	inner := max(1, width-2)
	row := func(line, background string, slots ...string) string {
		mark := "  "
		if focused != "" && focused != "shell" && slices.Contains(slots, focused) {
			mark = text(p.blue).Render(" ◂")
		}
		return withBackground(lipgloss.NewStyle().Width(inner).MaxWidth(inner).Render(" "+line), background) + mark
	}
	lanes := ""
	for _, lane := range t.lanes {
		lanes += text(lane).Render("● ")
	}
	graph := ""
	for level := range activityMix {
		graph += text(activityColor(level, t.lanes[0], t)).Render("■ ")
	}
	lines := []string{
		text(p.muted).Render("Preview"),
		row("", t.shell),
		row(text(t.ink).Bold(true).Render("Ada Lovelace")+" "+text(t.muted).Render("@ada · 2h"), t.shell, "ink", "muted"),
		row(text(t.ink).Render("Shipped the new parser ")+text(t.blue).Render("dev-null.sh"), t.shell, "ink", "blue"),
		row(text(t.code).Render("`go test ./...`"), t.shell, "code"),
		row(text(t.ink).Render("Selected row"), t.wash, "wash", "ink"),
		row(chipStyled("Reply", "r", t.ink, t)+" "+chipStyled("Star", "s", t.ink, t), t.shell, "hairline", "soft", "blue", "ink"),
		row(text(t.danger).Render("Danger")+"  "+text(t.success).Render("Success")+"  "+text(t.warning).Render("Warning"), t.shell, "danger", "success", "warning"),
		row(lanes, t.shell, "lane1", "lane2", "lane3", "lane4", "lane5", "lane6"),
		row(text(t.muted).Render("Activity ")+graph, t.shell, "graph", "peak", "lane1", "hairline", "muted"),
		row(text(t.hairline).Render(strings.Repeat("─", max(1, inner-2))), t.shell, "hairline"),
		row(text(t.brand).Bold(true).Render("/dev/null"), t.shell),
		"",
	}
	issues := readabilityIssues(t)
	if len(issues) == 0 {
		return append(lines, text(p.success).Render(fit("Every color is easy to read.", width)))
	}
	lines = append(lines, text(p.warning).Render(fit("Hard to read (you can still save):", width)))
	for i, issue := range issues {
		if left := len(issues) - i; len(lines) == height-1 && left > 1 {
			return append(lines, text(p.muted).Render(fit(fmt.Sprintf("+%d more", left), width)))
		}
		lines = append(lines, text(p.muted).Render(fit("· "+issue, width)))
	}
	return lines
}

func (m AppModel) themeEditorMouse(x, y int) (AppModel, tea.Cmd) {
	_, _, areas := m.themeEditorModal(m.account.colors())
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		switch area.action {
		case "field":
			if area.index == m.themeEditor.focus {
				return m, nil
			}
			return m, m.themeEditor.focusField(area.index)
		case "slider":
			at := float64(x-area.x) / float64(max(1, area.w-1))
			m.themeEditor.setSlider(area.index, int(math.Round(at*float64(sliderMax[area.index]))))
			return m, nil
		case "esc":
			return m, m.themeEditorKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		}
		if pressed, ok := keyAction(area.action); ok {
			return m.mouseKey(pressed)
		}
	}
	return m, nil
}
