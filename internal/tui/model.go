package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/demo"
)

type Model struct {
	width, height int
	theme         string
	dark          bool
	help          help.Model
	keys          keyMap
}

func New(theme string) Model {
	m := Model{width: 80, height: 24, theme: theme, dark: theme != "light", help: help.New(), keys: newKeys()}
	m.help.Styles = help.DefaultStyles(m.dark)
	return m
}

func (m Model) Init() tea.Cmd {
	if m.theme == "auto" {
		return tea.RequestBackgroundColor
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		m.help.SetWidth(max(0, m.width-4))
	case tea.BackgroundColorMsg:
		if m.theme == "auto" {
			m.dark = msg.IsDark()
			m.help.Styles = help.DefaultStyles(m.dark)
		}
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Dismiss):
			m.help.ShowAll = false
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	p := colors(m.dark)
	var content string
	switch {
	case m.width == 0 || m.height == 0:
		content = ""
	case m.width < 60 || m.height < 18:
		content = "DEMO\nResize to at least 60x18.\nCtrl+C to quit."
	case m.help.ShowAll:
		content = text(p.ink).Bold(true).Render("Keyboard shortcuts") + "\n\n" + m.help.View(m.keys) + "\n\nOffline demo. No account or network connection."
	default:
		content = m.preview(p)
	}
	if m.width > 0 && m.height > 0 {
		content = lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).
			Width(m.width).Height(m.height).MaxWidth(m.width).MaxHeight(m.height).
			Render(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) preview(p palette) string {
	width := min(m.width-8, 76)
	muted := text(p.muted)
	var b strings.Builder
	b.WriteString("  " + text(p.ink).Bold(true).Render("/dev/null") + "  " + text(p.blue).Render("DEMO") + "\n")
	b.WriteString("  " + muted.Render("A social network for developers.") + "\n")
	b.WriteString("  " + text(p.hairline).Render(strings.Repeat("─", width)) + "\n\n")
	b.WriteString("  " + muted.Render("Offline preview · synthetic commits · no account connected") + "\n\n")
	for i, commit := range demo.Commits() {
		lane := text(p.lanes[i])
		b.WriteString("  " + lane.Render("●") + "  " + text(p.ink).Bold(true).Render(commit.Author) + "  " + muted.Render(commit.Handle+" · "+commit.Hash) + "\n")
		body := lipgloss.NewStyle().Width(width - 3).Render(commit.Body)
		for _, line := range strings.Split(body, "\n") {
			b.WriteString("  " + lane.Render("│") + "  " + line + "\n")
		}
		b.WriteString("  " + lane.Render("│") + "\n")
	}
	b.WriteString("  " + muted.Render("○  That's the root commit.") + "\n\n")
	b.WriteString("  " + muted.Render("Phase 0 · Login and live logs arrive in later releases.") + "\n")
	b.WriteString("  " + m.help.View(m.keys))
	return b.String()
}
