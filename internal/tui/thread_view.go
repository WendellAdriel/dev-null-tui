package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

const threadHeaderHeight = 3

// threadParents maps each entry to the entry its line hangs from: the previous ancestor, or the replied commit.
func threadParents(entries []threadEntry) []int {
	parents := make([]int, len(entries))
	last := []int{-1, -1, -1}
	for i, entry := range entries {
		d := entry.depth
		parents[i] = -1
		switch {
		case d == 0:
			parents[i] = last[0]
		case last[d-1] >= 0:
			parents[i] = last[d-1]
		}
		last[d] = i
		for deeper := d + 1; deeper < len(last); deeper++ {
			last[deeper] = -1
		}
	}
	return parents
}

func (m *AppModel) renderThread(t *threadState) {
	p := colors(m.account.dark)
	width := max(1, t.viewport.Width())
	// Each depth puts its node one lane to the right, wide enough for a 4-cell avatar when graphics work.
	base, step, gap := 2, 3, 3
	if m.images.Enabled {
		base, step, gap = 3, 5, 4
	}
	column := func(depth int) int { return base + depth*step }
	entries := t.entries()
	t.restoreSelection(entries)
	t.starts, t.ends, t.widths, t.pictures, t.actions = nil, nil, nil, nil, nil
	var bodies []string
	var owners, nodes, stats []int
	for i, entry := range entries {
		indent := column(entry.depth) + gap
		contentWidth := max(1, width-indent-2)
		commit := entry.commit
		commit.ReplyPreview = nil
		tag := ""
		if entry.kind == "quote" {
			commit.QuotedID, tag = nil, "quoted"
		}
		rows, pictures := commitLayout(commit, tag, contentWidth, p, m.now, m.images)
		for _, picture := range pictures {
			picture.x += indent
			picture.y += len(bodies)
			picture.entry = i
			t.pictures = append(t.pictures, picture)
		}
		nodes, stats = append(nodes, len(bodies)), append(stats, len(bodies)+len(rows)-1)
		if i == t.selected {
			navigation := commitAction{}
			if entry.depth < 2 && (entry.kind == "focus" || entry.commit.Replies > 0) {
				navigation = commitAction{"Replies", "space"}
			}
			chips, areas := m.commitActions(entry.commit, navigation, contentWidth, p)
			for _, area := range areas {
				area.x += indent
				area.y += len(bodies) + len(rows)
				t.actions = append(t.actions, area)
			}
			rows = append(rows, chips...)
		}
		t.starts = append(t.starts, len(bodies))
		t.widths = append(t.widths, contentWidth)
		for _, row := range rows {
			bodies, owners = append(bodies, row), append(owners, i)
		}
		t.ends = append(t.ends, len(bodies))
		bodies, owners = append(bodies, ""), append(owners, -1)
	}
	more := -1
	if t.next != "" {
		more = len(bodies)
		bodies, owners = append(bodies, text(p.muted).Render("More replies ")+keyChip("n", p)), append(owners, -1)
	}

	lane := func(i int) string {
		if entries[i].commit.DeletedAt != nil || entries[i].commit.Author == nil {
			return p.muted
		}
		return authorColor(entries[i].commit.Author, p)
	}
	cells := make([][]string, len(bodies))
	for line := range cells {
		depth := 1
		if owners[line] >= 0 {
			depth = entries[owners[line]].depth
		}
		cells[line] = strings.Split(strings.Repeat(" ", column(depth)+gap), "")
	}
	put := func(line, col int, value string) {
		if line >= 0 && line < len(cells) && col < len(cells[line]) {
			cells[line][col] = value
		}
	}
	vertical := func(col, from, to int, glyph, color string) {
		for line := from; line < to; line++ {
			put(line, col, text(color).Render(glyph))
		}
	}

	// Ancestors form one solid trunk; replies hang off a dashed spine below their parent's ring.
	parents := threadParents(entries)
	children := make(map[int][]int)
	for i, parent := range parents {
		if parent < 0 {
			continue
		}
		if entries[i].depth == 0 {
			vertical(column(0), nodes[parent]+1, nodes[i], "│", lane(parent))
			continue
		}
		children[parent] = append(children[parent], i)
	}
	focus := len(t.ancestors)
	if more >= 0 && focus < len(entries) {
		children[focus] = append(children[focus], -1)
	}
	type branch struct {
		line, from, to int
		corner, color  string
	}
	var branches []branch
	for parent, list := range children {
		col := column(entries[parent].depth)
		ring := stats[parent]
		if ring <= nodes[parent] {
			ring = nodes[parent]
		} else {
			vertical(col, nodes[parent]+1, ring, "│", lane(parent))
			put(ring, col, text(lane(parent)).Render("◉"))
		}
		for n, child := range list {
			line, to := more, column(entries[parent].depth+1)-1
			if child >= 0 {
				line, to = nodes[child], column(entries[child].depth)
			}
			corner := "├"
			if n == len(list)-1 {
				corner = "╰"
				vertical(col, ring+1, line, "┆", lane(parent))
			}
			branches = append(branches, branch{line, col, to, corner, lane(parent)})
		}
	}
	for _, b := range branches {
		put(b.line, b.from, text(b.color).Render(b.corner))
		for col := b.from + 1; col < b.to; col++ {
			put(b.line, col, text(b.color).Render("╌"))
		}
	}
	for i, entry := range entries {
		col, line, c := column(entry.depth), nodes[i], entry.commit
		node := text(lane(i)).Render("●")
		switch {
		case c.DeletedAt != nil || c.Author == nil:
			node = text(p.muted).Render("○")
		case c.Status == "pending":
			node = text(lane(i)).Render("◌")
		}
		put(line, col, node)
		if !m.images.Enabled || c.DeletedAt != nil || c.Author == nil {
			continue
		}
		for row := range 2 {
			avatar := m.images.Row(m.avatarRequest(c.Author.AvatarURL, lane(i), 5, 2), row)
			if avatar == "" {
				break
			}
			put(line+row, col-2, avatar)
			for extra := col - 1; extra < col+3; extra++ {
				put(line+row, extra, "")
			}
		}
	}

	lines := make([]string, len(bodies))
	for line, body := range bodies {
		owner := owners[line]
		if owner == t.selected {
			cells[line][0] = text(p.blue).Render("▌")
		}
		content := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strings.Join(cells[line], "") + body)
		if owner == t.selected {
			content = withBackground(content, p.wash)
		}
		lines[line] = content
	}
	t.viewport.SetContent(strings.Join(lines, "\n"))
}

type commitAction struct{ label, key string }

func (m AppModel) commitActions(commit api.Commit, navigation commitAction, width int, p palette) ([]string, []hitArea) {
	if commit.DeletedAt != nil || commit.Author == nil {
		return []string{text(p.muted).Render(fit("Deleted commits cannot be replied to.", width))}, nil
	}
	actions := []commitAction{{"Reply", "r"}, {"Quote", "q"}}
	if navigation.key == "enter" {
		actions = append([]commitAction{navigation}, actions...)
	} else if navigation.key != "" {
		actions = append(actions, navigation)
	}
	if len(commit.Images) > 0 {
		actions = append(actions, commitAction{"View", "v"})
	}
	if m.ownCommit(commit) {
		actions = append(actions, commitAction{"Delete", "D"})
	}
	var rows []string
	var areas []hitArea
	line := ""
	for _, action := range actions {
		chip := text(p.ink).Render(action.label) + " " + keyChip(action.key, p)
		if line != "" && lipgloss.Width(line)+2+lipgloss.Width(chip) > width {
			rows, line = append(rows, line), ""
		}
		if line != "" {
			line += "  "
		}
		areas = append(areas, hitArea{uiRect{lipgloss.Width(line), len(rows), lipgloss.Width(chip), 1}, action.key, 0})
		line += chip
	}
	return append(rows, line), areas
}

func (m *AppModel) revealThreadSelection(t *threadState) {
	if t.selected >= len(t.starts) {
		return
	}
	start, end := t.starts[t.selected], t.ends[t.selected]
	if start < t.viewport.YOffset() || end-start >= t.viewport.Height() {
		t.viewport.SetYOffset(start)
	} else if end > t.viewport.YOffset()+t.viewport.Height() {
		t.viewport.SetYOffset(end - t.viewport.Height())
	}
}

func (m *AppModel) sizeThreads() {
	_, center, _ := m.columns()
	for i := range m.threads {
		t := &m.threads[i]
		t.viewport.SetWidth(center)
		t.viewport.SetHeight(max(1, m.mainHeight()-threadHeaderHeight))
		m.renderThread(t)
		m.revealThreadSelection(t)
	}
}

func (m AppModel) threadColumn(width int, p palette) string {
	t := m.threads[len(m.threads)-1]
	back := keyChip("esc", p) + text(p.muted).Render(" Back")
	title := "  " + text(p.ink).Bold(true).Render("Thread")
	if len(m.threads) > 1 {
		title += text(p.muted).Render(fmt.Sprintf(" · %d deep", len(m.threads)))
	}
	header := fit(title, max(1, width-lipgloss.Width(back)-2))
	header += strings.Repeat(" ", max(1, width-lipgloss.Width(header)-lipgloss.Width(back)-1)) + back
	content := "\n" + header + "\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n" + t.viewport.View()
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) threadStatus() string {
	t := m.threads[len(m.threads)-1]
	if t.status != "" {
		return t.status
	}
	if t.loading {
		return "Loading replies..."
	}
	ending := "end of thread"
	if t.next != "" {
		ending = "more replies available"
	}
	return fmt.Sprintf("%d / %d in thread  ·  %s", t.selected+1, len(t.entries()), ending)
}

func (m AppModel) threadMouse(x, y int) (AppModel, tea.Cmd) {
	t := &m.threads[len(m.threads)-1]
	if y == 1 && x >= m.threadBackX() {
		return m.mouseKey("esc")
	}
	if y < threadHeaderHeight || y >= m.mainHeight() {
		return m, nil
	}
	row := y - threadHeaderHeight + t.viewport.YOffset()
	for _, area := range t.actions {
		if area.contains(x, row) {
			return m.mouseKey(area.action)
		}
	}
	for _, picture := range t.pictures {
		if picture.contains(x, row) {
			m.selectEntry(t, picture.entry)
			return m, m.openViewer(t.entries()[picture.entry].commit, picture.image)
		}
	}
	for i, start := range t.starts {
		if row < start || row >= t.ends[i] {
			continue
		}
		if i == t.selected {
			return m.mouseKey("enter")
		}
		m.selectEntry(t, i)
		return m, nil
	}
	return m, nil
}

func (m AppModel) threadBackX() int {
	_, center, _ := m.columns()
	return center - lipgloss.Width(keyChip("esc", colors(m.account.dark))+" Back") - 1
}

func (m AppModel) deleteModal(p palette) (string, uiRect, []hitArea) {
	actions := []modalAction{{"Keep commit", "esc", "esc"}, {"Delete commit", "ctrl+d", "ctrl+d"}}
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 12)
	width := max(1, r.w-4)
	body := text(p.ink).Bold(true).Render("Delete this commit?") + "\n\n" + text(p.muted).Render(fit("It disappears from every log. This cannot be undone.", width)) + "\n" + text(p.muted).Render(fit("Replies stay and show it as deleted.", width))
	footer, areas := modalFooter(actions, width, m.deleting.focus+1, p)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel("/dev/null · DELETE", body, footer, r, p), r, areas
}
