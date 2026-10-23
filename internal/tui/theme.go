package tui

import "charm.land/lipgloss/v2"

type palette struct {
	shell, ink, muted, hairline, blue string
	lanes                             [2]string
}

func colors(dark bool) palette {
	if dark {
		return palette{
			shell: "#111112", ink: "#ececec", muted: "#a1a1a6",
			hairline: "#2a2a2d", blue: "#7aa2f7",
			lanes: [2]string{"#ff8a4c", "#51cf66"},
		}
	}
	return palette{
		shell: "#f6f7f8", ink: "#121417", muted: "#5b616b",
		hairline: "#e1e4e8", blue: "#3b6fe0",
		lanes: [2]string{"#d9480f", "#2f9e44"},
	}
}

func text(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
