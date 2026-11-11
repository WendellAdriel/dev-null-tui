package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// editorOn opens a new-theme editor on Dark at 140x40, with the cursor on the field for slot.
func editorOn(t *testing.T, slot string) AppModel {
	t.Helper()
	m := themeApp(t)
	m.newThemeEditor("dark")
	for i, s := range themeSlots {
		if s.key == slot {
			m.themeEditor.focusField(i + 1)
		}
	}
	return m
}

func setField(m AppModel, value string) AppModel {
	m = pressKey(m, "ctrl+u")
	return typeText(m, value)
}

func TestTheEditorTakesShortAndBareHexValues(t *testing.T) {
	m := editorOn(t, "ink")
	m = setField(m, "F00")
	if !strings.Contains(m.View().Content, "255;0;0") {
		t.Fatal("a short hex value without # must reach the preview")
	}
	m = setField(m, "00ff00")
	m.themeEditor.focusField(0)
	m = typeText(m, "Bare")
	m = pressKey(m, "ctrl+s")
	if len(m.tuiSettings.CustomThemes) != 1 || m.tuiSettings.CustomThemes[0].Colors["ink"] != "#00ff00" {
		t.Fatalf("the value must be saved in full form: %+v", m.tuiSettings.CustomThemes)
	}
}

func TestThePreviewShowsTheActivityGraphAndMarksTheFocusedColor(t *testing.T) {
	m := editorOn(t, "peak")
	m = setField(m, "#123456")
	if !strings.Contains(m.View().Content, "18;52;86") {
		t.Fatal("the activity peak must show in the preview")
	}
	m.themeEditor.focusField(8)
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, "go test") && !strings.Contains(line, "◂") {
			t.Fatalf("the preview must mark the rows the focused color changes: %q", line)
		}
	}
}

func TestEnterOpensSlidersThatWriteTheHexValue(t *testing.T) {
	m := editorOn(t, "shell")
	m = setField(m, "#ff0000")
	m = pressKey(m, "enter")
	frame := frameText(m)
	if !m.themeEditor.adjusting || !strings.Contains(frame, "Hue") || !strings.Contains(frame, "Lightness") || !strings.Contains(frame, "50%") {
		t.Fatalf("enter must open the sliders on the field's color:\n%s", frame)
	}
	m = pressKey(m, "down")
	m = pressKey(m, "down")
	m = pressKey(m, "left")
	if got := m.themeEditor.field.Value(); got != hslHex([3]int{0, 100, 49}) {
		t.Fatalf("lightness down by one gave %s", got)
	}
	m = pressKey(m, "shift+left")
	if got := m.themeEditor.field.Value(); got != hslHex([3]int{0, 100, 39}) || !strings.Contains(m.View().Content, rgbOf(got)) {
		t.Fatalf("shift steps by ten and the preview follows: %s", got)
	}
	m = pressKey(m, "up")
	m = pressKey(m, "up")
	m = pressKey(m, "left")
	if m.themeEditor.hsl[0] != 359 {
		t.Fatalf("hue must wrap around: %v", m.themeEditor.hsl)
	}
	m = pressKey(m, "esc")
	if m.themeEditor == nil || m.themeEditor.adjusting {
		t.Fatal("esc must close the sliders and keep the editor")
	}
	m = setField(m, "#0000ff")
	if m.themeEditor.field.Value() != "#0000ff" {
		t.Fatal("the hex field must still take typing after the sliders")
	}
}

func TestClickingASliderSetsItsValue(t *testing.T) {
	m := editorOn(t, "shell")
	m = setField(m, "#336699")
	m = pressKey(m, "enter")
	_, _, areas := m.themeEditorModal(m.account.colors())
	for _, area := range areas {
		if area.action == "slider" && area.index == 2 {
			next, _ := m.themeEditorMouse(area.x, area.y)
			m = next
		}
	}
	if got := m.themeEditor.field.Value(); got != "#000000" || m.themeEditor.slider != 2 {
		t.Fatalf("a click at the start of Lightness must make black: %s", got)
	}
}

func TestHSLRoundTrips(t *testing.T) {
	for _, hex := range []string{"#000000", "#ffffff", "#ff0000", "#00ff00", "#0000ff", "#808080", "#336699"} {
		if got := hslHex(hexHSL(hex)); got != hex {
			t.Errorf("%s came back as %s", hex, got)
		}
	}
}

// rgbOf is a hex color as the truecolor parameters a frame carries.
func rgbOf(hex string) string {
	var r, g, b int
	_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("%d;%d;%d", r, g, b)
}
