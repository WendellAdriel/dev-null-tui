package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// sliderNames are the theme editor's hue, saturation and lightness sliders, in order, with their largest values.
var sliderNames = [3]string{"Hue", "Saturation", "Lightness"}

var sliderMax = [3]int{359, 100, 100}

// normalHex turns #rgb, rgb, rrggbb or #rrggbb into #rrggbb, and anything else into "".
func normalHex(value string) string {
	v := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	if len(v) == 3 {
		v = string([]byte{v[0], v[0], v[1], v[1], v[2], v[2]})
	}
	if !hexValue.MatchString("#" + v) {
		return ""
	}
	return "#" + v
}

// hexHSL is a #rrggbb color as whole-number hue (0 to 359), saturation and lightness (0 to 100).
func hexHSL(hex string) [3]int {
	n, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	r, g, b := float64(n>>16&255)/255, float64(n>>8&255)/255, float64(n&255)/255
	high, low := max(r, g, b), min(r, g, b)
	l := (high + low) / 2
	var h, s float64
	if d := high - low; d > 0 {
		s = d / (1 - math.Abs(2*l-1))
		switch high {
		case r:
			h = math.Mod((g-b)/d+6, 6)
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
	}
	return [3]int{int(math.Round(h*60)) % 360, int(math.Round(s * 100)), int(math.Round(l * 100))}
}

// hslHex is the #rrggbb color for whole-number hue, saturation and lightness.
func hslHex(hsl [3]int) string {
	h, s, l := float64(hsl[0]), float64(hsl[1])/100, float64(hsl[2])/100
	chroma := (1 - math.Abs(2*l-1)) * s
	x := chroma * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = chroma, x
	case h < 120:
		r, g = x, chroma
	case h < 180:
		g, b = chroma, x
	case h < 240:
		g, b = x, chroma
	case h < 300:
		r, b = x, chroma
	default:
		r, b = chroma, x
	}
	m := l - chroma/2
	channel := func(v float64) int { return int(math.Round((v + m) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", channel(r), channel(g), channel(b))
}

// openSliders starts the sliders on the focused color, or on the color the preview shows when the field isn't valid.
func (e *themeEditor) openSliders() {
	hex := normalHex(e.field.Value())
	if hex == "" {
		p := e.palette()
		if hex = *slotColor(&p, themeSlots[e.focus-1].key); hex == "" {
			hex = p.lanes[0]
		}
	}
	e.adjusting, e.slider, e.hsl = true, 0, hexHSL(hex)
}

// setSlider changes one slider and writes the color into the hex field, which stays the value that's saved.
func (e *themeEditor) setSlider(slider, value int) {
	if slider == 0 {
		value = (value%360 + 360) % 360
	}
	e.slider, e.hsl[slider], e.status = slider, min(max(0, value), sliderMax[slider]), ""
	e.field.SetValue(hslHex(e.hsl))
	e.field.CursorEnd()
}

// sliderKey handles keys while the sliders are open: ↑/↓ pick one, ←/→ move it, with Shift for steps of ten.
func (m *AppModel) sliderKey(pressed string) (tea.Cmd, bool) {
	e := m.themeEditor
	step := map[string]int{"left": -1, "right": 1, "shift+left": -10, "shift+right": 10, "h": -1, "l": 1}[pressed]
	switch {
	case step != 0:
		e.setSlider(e.slider, e.hsl[e.slider]+step)
	case pressed == "up" || pressed == "k":
		e.slider = max(0, e.slider-1)
	case pressed == "down" || pressed == "j":
		e.slider = min(len(sliderNames)-1, e.slider+1)
	case pressed == "enter" || pressed == "esc":
		e.adjusting = false
	case pressed == "tab" || pressed == "shift+tab":
		e.adjusting = false
		return nil, false
	case pressed == "ctrl+s":
		return m.saveCustomTheme(), true
	}
	return nil, true
}

// sliderLines draws the three sliders as color bars, each showing what its own change would give, with a mark at the
// current value. barWidth is how many cells each bar takes, for mouse clicks.
func (e *themeEditor) sliderLines(width int, p palette) (lines []string, barX, barWidth int) {
	const labelWidth = 11
	barX = 3 + labelWidth
	barWidth = max(8, width-barX-6)
	for i, name := range sliderNames {
		marker := "  "
		if i == e.slider {
			marker = text(p.blue).Render("› ")
		}
		label := text(p.muted).Width(labelWidth).Render(name)
		if i == e.slider {
			label = text(p.ink).Bold(true).Width(labelWidth).Render(name)
		}
		at := int(math.Round(float64(e.hsl[i]) / float64(sliderMax[i]) * float64(barWidth-1)))
		var bar strings.Builder
		for cell := range barWidth {
			hsl := e.hsl
			hsl[i] = int(math.Round(float64(cell) / float64(barWidth-1) * float64(sliderMax[i])))
			color := hslHex(hsl)
			style := lipgloss.NewStyle().Background(lipgloss.Color(color))
			if cell == at {
				mark := "#000000"
				if darkColor(color) {
					mark = "#ffffff"
				}
				bar.WriteString(style.Foreground(lipgloss.Color(mark)).Render("┃"))
				continue
			}
			bar.WriteString(style.Render(" "))
		}
		unit := "%"
		if i == 0 {
			unit = "°"
		}
		value := text(p.ink).Render(fmt.Sprintf(" %d%s", e.hsl[i], unit))
		lines = append(lines, fit(" "+marker+label+bar.String()+value, width))
	}
	return lines, barX, barWidth
}
