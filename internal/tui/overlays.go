package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type uiRect struct{ x, y, w, h int }

func (r uiRect) contains(x, y int) bool { return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h }

type hitArea struct {
	uiRect
	action string
	index  int
}

func centeredRect(width, height, maxWidth, maxHeight int) uiRect {
	w, h := max(1, min(maxWidth, width-4)), max(1, min(maxHeight, height-4))
	return uiRect{(width - w) / 2, (height - h) / 2, w, h}
}

func panel(title, body, footer string, r uiRect, p palette) string {
	w, h := max(1, r.w-4), max(1, r.h-2)
	header := text(p.ink).Bold(true).Render(fit(title, max(1, w-3)))
	header += strings.Repeat(" ", max(1, w-lipgloss.Width(header)-1)) + text(p.muted).Render("×")
	line := text(p.hairline).Render(strings.Repeat("─", w))
	content := header + "\n" + line + "\n" + lipgloss.NewStyle().Width(w).Height(max(1, h-4)).MaxHeight(max(1, h-4)).Render(body) + "\n" + line + "\n" + fit(footer, w)
	content = lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).Render(content)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.blue)).Background(lipgloss.Color(p.shell)).Padding(0, 1).Render(content)
}

func overlay(base, content string, r uiRect, p palette, width, height int) string {
	shadow := lipgloss.NewStyle().Width(r.w).Height(r.h).Background(lipgloss.Color(p.hairline)).Render("")
	result := lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(shadow).X(r.x+1).Y(r.y+1), lipgloss.NewLayer(content).X(r.x).Y(r.y)).Render()
	return lipgloss.NewStyle().Width(max(1, width)).Height(max(1, height)).MaxWidth(max(1, width)).MaxHeight(max(1, height)).Render(result)
}

func (m AppModel) overlayToast(base, content string, r uiRect) string {
	p := colors(m.account.dark)
	bottom := text(p.hairline).Render(strings.Repeat("▀", max(1, r.w-1)))
	right := text(p.hairline).Render(strings.TrimSuffix(strings.Repeat("▌\n", max(0, r.h-1)), "\n"))
	result := lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(bottom).X(r.x+1).Y(r.y+r.h), lipgloss.NewLayer(right).X(r.x+r.w).Y(r.y+1), lipgloss.NewLayer(content).X(r.x).Y(r.y)).Render()
	return lipgloss.NewStyle().MaxWidth(m.account.width).MaxHeight(m.account.height).Render(result)
}

type toastNotice struct {
	id                  int
	visible             bool
	kind, title, detail string
}
type toastExpired struct{ id, session int }

func (m *AppModel) showToast(kind, title, detail string) {
	m.toast.id++
	m.toast.visible = true
	m.toast.kind, m.toast.title, m.toast.detail = kind, safeText(title), fit(safeText(detail), 400)
}

func (m *AppModel) notify(kind, title, detail string) tea.Cmd {
	m.showToast(kind, title, detail)
	id, session := m.toast.id, m.session
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return toastExpired{id, session} })
}

func (m AppModel) toastView() (string, uiRect) {
	p := colors(m.account.dark)
	background := p.shell
	w := max(1, min(46, m.account.width-2))
	inner := max(1, w-4)
	ink := p.blue
	if m.toast.kind == "error" {
		ink = p.danger
	}
	title := fit(m.toast.title, max(1, inner-3))
	line := text(ink).Background(lipgloss.Color(background)).Bold(true).Render(title) + strings.Repeat(" ", max(1, inner-ansi.StringWidth(title)-1)) + text(p.muted).Background(lipgloss.Color(background)).Render("×")
	if !m.composer.open || m.account.width >= 120 {
		detail := strings.Split(ansi.Wrap(m.toast.detail, inner, ""), "\n")
		line += "\n" + strings.Join(detail[:min(3, len(detail))], "\n")
	}
	view := lipgloss.NewStyle().Width(w).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(ink)).BorderBackground(lipgloss.Color(background)).Padding(0, 1).Background(lipgloss.Color(background)).Foreground(lipgloss.Color(p.ink)).Render(line)
	view = withBackground(view, background)
	return view, uiRect{max(0, m.account.width-lipgloss.Width(view)-1), 0, lipgloss.Width(view), lipgloss.Height(view)}
}
