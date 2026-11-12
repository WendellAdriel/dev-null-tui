package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// listHeaderHeight is the rows above the list screen's commits: a blank row, the title, a rule, the about lines, a
// blank row and a rule.
func (m AppModel) listHeaderHeight() int {
	_, width, _ := m.columns()
	return 5 + len(m.listAbout(width))
}

// listAbout is the line under the list screen's title, or a hot topic's recap wrapped to four lines.
func (m AppModel) listAbout(width int) []string {
	switch {
	case m.headline != nil && m.headline.topic.Summary != nil:
		rows := strings.Split(ansi.Wrap(safeText(*m.headline.topic.Summary), max(1, width-4), ""), "\n")
		return rows[:min(len(rows), 4)]
	case m.headline != nil:
		return []string{"Commits about this topic from the past week, newest first."}
	case m.commitSearch != "":
		return []string{fit("Search: "+safeText(m.commitSearch), width-4)}
	case m.scheduled != nil:
		return []string{""}
	}
	return []string{"Commits you starred, newest first."}
}

func (m *AppModel) renderStarred() {
	if !m.starredOpen {
		return
	}
	if m.scheduled != nil {
		m.renderScheduled()
		return
	}
	f, p := &m.starred, m.account.colors()
	width := max(1, f.viewport.Width())
	day := ""
	list := m.commitList(nil, f.commits, f.selected, width, func(_ int, c api.Commit) []string {
		date := dayLabel(c.CreatedAt, m.now)
		// Search results are ranked by meaning, so day labels would interrupt them out of order.
		if date == day || m.commitSearch != "" {
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
		add("Reading " + m.listName() + " from /dev/null...")
	case f.loaded && len(f.commits) == 0 && m.commitSearch != "":
		add("fatal: nothing close to '" + safeText(m.commitSearch) + "'")
		add("Try describing the idea in other words.")
	case f.loaded && m.commitSearch != "":
		add("Those are the closest commits.")
	case f.loaded && len(f.commits) == 0 && m.headline != nil:
		add("No commits about this topic in the past week.")
	case f.loaded && m.headline != nil:
		add("That's every commit about this topic from the past week.")
	case f.loaded && len(f.commits) == 0:
		add("fatal: no starred commits yet")
		add("Star a commit to find it here.")
	case f.next != "":
		lines = append(lines, "", lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(actionChip("More", "n", p)), "")
	case f.loaded:
		add("You've reached the end of your starred commits.")
	}
	f.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *AppModel) sizeStarred() {
	if !m.starredOpen {
		return
	}
	_, width, _ := m.columns()
	if s := m.scheduled; s != nil {
		s.viewport.SetWidth(width)
		s.viewport.SetHeight(max(1, m.mainHeight()-m.listHeaderHeight()))
		m.renderScheduled()
		m.revealScheduledSelection()
		return
	}
	f := &m.starred
	offset := f.viewport.YOffset()
	if f.selected < len(f.starts) {
		offset -= f.starts[f.selected]
	}
	f.viewport.SetWidth(width)
	f.viewport.SetHeight(max(1, m.mainHeight()-m.listHeaderHeight()))
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
	if m.scheduled != nil {
		return m.scheduledColumn(width, p)
	}
	line := text(p.hairline).Render(strings.Repeat("─", width))
	header := split(text(p.ink).Bold(true).Render("Starred"), text(p.muted).Render(m.icons.lock+" Only you"), width-4)
	count := ""
	if m.starred.loaded && !m.starred.loading {
		count = plural(len(m.starred.commits), "match", "matches")
	}
	switch {
	case m.headline != nil:
		if m.starred.loaded && !m.starred.loading {
			count = plural(len(m.starred.commits), "commit", "commits")
		}
		name := text(p.lanes[0]).Render(m.icons.flame) + " " + text(p.ink).Bold(true).Render(fit(safeText(m.headline.topic.Name), width-6-lipgloss.Width(count)))
		header = split(name, text(p.muted).Render(count), width-4)
	case m.commitSearch != "":
		header = split(text(p.ink).Bold(true).Render("Most relevant commits"), text(p.muted).Render(count), width-4)
	}
	about := ""
	for _, row := range m.listAbout(width) {
		about += "\n  " + text(p.muted).Render(fit(row, width-4))
	}
	content := "\n  " + header + "\n" + line + about + "\n\n" + line + "\n" + m.starred.viewport.View()
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) starredBindings(bind func(string, string) key.Binding) appKeys {
	if m.scheduled != nil {
		return m.scheduledBindings(bind)
	}
	short := []key.Binding{bind("↑/↓", "move"), bind("enter", "open thread")}
	if c, ok := m.focusedEngagementCommit(); ok && hasProfile(c.Author) {
		short = append(short, bind("u", "author profile"))
	}
	short = append(short, bind("ctrl+r", "refresh"), bind("esc", "back to log"), bind("?", "help"), bind("ctrl+c", "quit"))
	all := append(append([]key.Binding(nil), short...), bind("r / q", "reply / quote"), bind("v", "view images"), bind("D", "delete own commit"), bind("y", "copy link"), bind(".", "more commit actions"), bind("Q", "open quoted commit"), bind("T", "copy own commit text"))
	if m.isStarred() {
		all = append(all, bind("n", "older starred commits"))
	}
	all = append(all, bind("g g / home", "first commit"), bind("G / end", "last commit"), bind("pgup/pgdown", "move five entries"), bind("ctrl+u/ctrl+d", "move five entries"))
	if m.isStarred() {
		all = append(all, bind("g s", "reopen Starred"))
	} else {
		all = append(all, bind("g s", "Starred"))
	}
	all = append(all, bind("g l / g e", "Log / Trending"), bind("g n", "notifications"), bind("g p", "your profile"), bind("c", "compose"), bind("a", "account"), bind("f2", "choose a theme"))
	all = append(all, m.engagementBindings()...)
	return appKeys{short, all}
}

func (m AppModel) starredMouse(x, y int) (AppModel, tea.Cmd) {
	if m.scheduled != nil {
		return m.scheduledMouse(x, y)
	}
	if y < m.listHeaderHeight() || y >= m.mainHeight() {
		return m, nil
	}
	f := &m.starred
	row := y - m.listHeaderHeight() + f.viewport.YOffset()
	for _, area := range f.actions {
		if area.contains(x, row) {
			if area.action == "avatar" {
				return m, m.openAuthorAvatar(f.commits[area.index].Author)
			}
			return m.mouseKey(area.action)
		}
	}
	for _, picture := range f.pictures {
		if !picture.contains(x, row) {
			continue
		}
		cmd := m.selectStarred(picture.entry)
		if !picture.poll {
			return m, tea.Batch(cmd, m.openViewer(f.commits[picture.entry], picture.index))
		}
		if m.voteOn(picture.index) {
			return m, cmd
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
