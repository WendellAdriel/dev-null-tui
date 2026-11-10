package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// statusPart is one piece of a chip row: plain text, or a clickable action chip when label is set.
type statusPart struct {
	text, label, key, action string
	// pinned parts stay when a narrow terminal drops the others from the end.
	pinned bool
}

func plainStatus(text string) statusPart { return statusPart{text: text, pinned: true} }

func statusChip(label, key string) statusPart { return statusPart{label: label, key: key} }

// hintChip is a chip whose click sends action rather than the key it shows, such as "down" for "↑/↓".
func hintChip(label, key, action string) statusPart {
	return statusPart{label: label, key: key, action: action}
}

// closeChip is the pinned Esc chip that ends most status lines.
func closeChip(label string) statusPart { return statusPart{label: label, key: "esc", pinned: true} }

func plainStatusLine(text string) []statusPart { return []statusPart{plainStatus(text)} }

func (s statusPart) view(p palette) string {
	if s.label == "" {
		return text(p.ink).Render(s.text)
	}
	return actionChip(s.label, s.key, p)
}

// statusLine draws the bottom status bar after a leading space. Its hit areas carry the key a click sends.
func statusLine(parts []statusPart, width int, p palette) (string, []hitArea) {
	line, areas := chipRow(parts, max(0, width-1), p)
	for i := range areas {
		areas[i].x++
	}
	return ansi.Truncate(" "+line, max(0, width), "…"), areas
}

// chipRow draws the parts two cells apart. Unpinned parts drop from the end until the rest fits.
func chipRow(parts []statusPart, width int, p palette) (string, []hitArea) {
	shown := append([]statusPart(nil), parts...)
	for rowWidth(shown, p) > width {
		last := -1
		for i, part := range shown {
			if !part.pinned {
				last = i
			}
		}
		if last < 0 {
			break
		}
		shown = append(shown[:last], shown[last+1:]...)
	}
	line, x := "", 0
	var areas []hitArea
	for i, part := range shown {
		if i > 0 {
			line += "  "
			x += 2
		}
		drawn := part.view(p)
		w := lipgloss.Width(drawn)
		if part.label != "" && x+w <= width {
			action := part.action
			if action == "" {
				action = part.key
			}
			areas = append(areas, hitArea{uiRect{x, 0, w, 1}, action, 0})
		}
		line += drawn
		x += w
	}
	return ansi.Truncate(line, max(0, width), "…"), areas
}

func rowWidth(parts []statusPart, p palette) int {
	width := 0
	for i, part := range parts {
		if i > 0 {
			width += 2
		}
		width += lipgloss.Width(part.view(p))
	}
	return width
}
