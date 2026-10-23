package tui

import "charm.land/lipgloss/v2"

type palette struct {
	shell, ink, muted, hairline, blue, wash, soft, code, danger string
	lanes                                                       [6]string
}

func colors(dark bool) palette {
	if dark {
		return palette{
			shell: "#111112", ink: "#ececec", muted: "#a1a1a6",
			hairline: "#2a2a2d", blue: "#7aa2f7", wash: "#232326",
			soft: "#18233a", code: "#e5c07b", danger: "#ff6b6b",
			lanes: [6]string{"#ff8a4c", "#51cf66", "#da77f2", "#3bc9db", "#f06595", "#fcc419"},
		}
	}
	return palette{
		shell: "#f6f7f8", ink: "#121417", muted: "#5b616b",
		hairline: "#e1e4e8", blue: "#3b6fe0", wash: "#eef0f2",
		soft: "#e8effc", code: "#875f15", danger: "#c92a2a",
		lanes: [6]string{"#d9480f", "#2f9e44", "#9c36b5", "#0c8599", "#c2255c", "#b7791f"},
	}
}

func text(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
