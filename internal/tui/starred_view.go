package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

const starredHeaderHeight = 6

func (m *AppModel) renderStarred() {
	if !m.starredOpen {
		return
	}
	f, p := &m.starred, colors(m.account.dark)
	width := max(1, f.viewport.Width())
	day := ""
	list := m.commitList(nil, f.commits, f.selected, width, func(_ int, c api.Commit) []string {
		date := dayLabel(c.CreatedAt, m.now)
		if date == day {
			return nil
		}
		day = date
		return []string{"    " + text(p.muted).Render(fit(date, width-6)), text(p.hairline).Render(strings.Repeat("─", width))}
	})
	f.starts, f.ends, f.pictures, f.actions = list.starts, list.ends, list.pictures, list.actions
	lines := list.lines
	add := func(value string) { lines = append(lines, "", "  "+text(p.muted).Render(fit(value, width-4))) }
	switch {
	case f.status != "":
		add(f.status)
	case f.loading:
		add("Reading Starred from /dev/null...")
	case f.loaded && len(f.commits) == 0:
		add("fatal: no starred commits yet")
		add("Star a commit to find it here.")
	case f.next != "":
		lines = append(lines, "", lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(text(p.muted).Render("More ")+keyChip("n", p)), "")
	case f.loaded:
		add("You've reached the end of your starred commits.")
	}
	f.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *AppModel) sizeStarred() {
	if !m.starredOpen {
		return
	}
	f := &m.starred
	_, width, _ := m.columns()
	offset := f.viewport.YOffset()
	if f.selected < len(f.starts) {
		offset -= f.starts[f.selected]
	}
	f.viewport.SetWidth(width)
	f.viewport.SetHeight(max(1, m.mainHeight()-starredHeaderHeight))
	m.renderStarred()
	if f.selected < len(f.starts) {
		f.viewport.SetYOffset(max(0, f.starts[f.selected]+offset))
	}
}

func (m *AppModel) revealStarredSelection() {
	f := &m.starred
	if f.selected >= len(f.starts) {
		return
	}
	start, end := f.starts[f.selected], f.ends[f.selected]
	if start < f.viewport.YOffset() || end-start >= f.viewport.Height() {
		f.viewport.SetYOffset(start)
	} else if end > f.viewport.YOffset()+f.viewport.Height() {
		f.viewport.SetYOffset(end - f.viewport.Height())
	}
}

func (m AppModel) starredColumn(width int, p palette) string {
	line := text(p.hairline).Render(strings.Repeat("─", width))
	header := split(text(p.ink).Bold(true).Render("Starred"), text(p.muted).Render(m.icons.lock+" Only you"), width-4)
	content := "\n  " + header + "\n" + line + "\n  " + text(p.muted).Render(fit("Commits you starred, newest first.", width-4)) + "\n\n" + line + "\n" + m.starred.viewport.View()
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) starredBindings(bind func(string, string) key.Binding) appKeys {
	short := []key.Binding{bind("j/k", "move"), bind("enter", "open thread"), bind("u", "author profile"), bind("ctrl+r", "refresh"), bind("esc", "back to log"), bind("?", "help"), bind("ctrl+c", "quit")}
	all := append(append([]key.Binding(nil), short...), bind("r / q", "reply / quote"), bind("v", "view images"), bind("D", "delete own commit"), bind("y", "copy link"), bind("n", "older starred commits"), bind("g g / home", "first commit"), bind("G / end", "last commit"), bind("pgup/pgdown", "move five entries"), bind("ctrl+u/ctrl+d", "move five entries"), bind("g s", "reopen Starred"), bind("g l / g e", "Log / Trending"), bind("g n", "notifications"), bind("g p", "your profile"), bind("c", "compose"), bind("a", "account"), bind("f2", m.account.nextTheme()+" theme"))
	all = append(all, m.engagementBindings()...)
	return appKeys{short, all}
}

func (m AppModel) starredMouse(x, y int) (AppModel, tea.Cmd) {
	if y < starredHeaderHeight || y >= m.mainHeight() {
		return m, nil
	}
	f := &m.starred
	row := y - starredHeaderHeight + f.viewport.YOffset()
	for _, area := range f.actions {
		if area.contains(x, row) {
			return m.mouseKey(area.action)
		}
	}
	for _, picture := range f.pictures {
		if picture.contains(x, row) {
			cmd := m.selectStarred(picture.entry)
			return m, tea.Batch(cmd, m.openViewer(f.commits[picture.entry], picture.image))
		}
	}
	for i, start := range f.starts {
		if row >= start && row < f.ends[i] {
			cmd := m.selectStarred(i)
			return m, tea.Batch(cmd, m.openThread(f.commits[i]))
		}
	}
	return m, nil
}
