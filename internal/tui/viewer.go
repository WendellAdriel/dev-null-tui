package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

type viewerState struct {
	open, hidePanel, loading bool
	commit                   api.Commit
	index, generation        int
	offset                   int
	replies                  []api.Commit
	next, status             string
	ctx                      context.Context
	cancel                   context.CancelFunc
}

func (m *AppModel) openViewer(commit api.Commit, index int) tea.Cmd {
	if commit.DeletedAt != nil || commit.Author == nil || len(commit.Images) == 0 {
		m.notice = "This commit has no images to view."
		return nil
	}
	m.closeViewer()
	ctx, cancel := context.WithCancel(m.account.life.ctx)
	generation := m.viewer.generation + 1
	m.viewer = viewerState{open: true, commit: commit, index: max(0, min(index, len(commit.Images)-1)), generation: generation, ctx: ctx, cancel: cancel}
	m.resetSequence()
	return m.fetchViewerReplies(ctx, "")
}

func (m *AppModel) fetchViewerReplies(base context.Context, cursor string) tea.Cmd {
	m.viewer.loading = true
	session, generation, id, load := m.session, m.viewer.generation, m.viewer.commit.ID, m.loadReplies
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		replies, err := load(ctx, id, cursor)
		return repliesResult{session: session, generation: generation, viewer: true, parent: id, cursor: cursor, replies: replies, err: err}
	})
}

func (m *AppModel) closeViewer() {
	if m.viewer.cancel != nil {
		m.viewer.cancel()
	}
	m.viewer = viewerState{generation: m.viewer.generation}
}

func (m *AppModel) acceptViewerReplies(msg repliesResult) tea.Cmd {
	v := &m.viewer
	if msg.session != m.session || !v.open || msg.generation != v.generation || msg.parent != v.commit.ID {
		return nil
	}
	v.loading = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		v.status = "Couldn't load replies: " + safeText(msg.err.Error())
		return nil
	}
	if msg.cursor == "" {
		v.replies = nil
	}
	v.replies, v.next, v.status = uniqueCommits(append(v.replies, msg.replies.Data...)), msg.replies.Next, ""
	return nil
}

func (m AppModel) viewerKey(pressed string) (AppModel, tea.Cmd) {
	v := &m.viewer
	switch pressed {
	case "esc":
		m.closeViewer()
	case "left", "h":
		v.index = max(0, v.index-1)
	case "right", "l":
		v.index = min(len(v.commit.Images)-1, v.index+1)
	case "j", "down", "k", "up", "pgdown", "pgup", "ctrl+d", "ctrl+u", "home", "end", "G":
		delta := map[string]int{"j": 1, "down": 1, "k": -1, "up": -1, "pgdown": 5, "ctrl+d": 5, "pgup": -5, "ctrl+u": -5, "home": -1 << 20, "end": 1 << 20, "G": 1 << 20}[pressed]
		height := m.viewerImageBounds().h
		v.offset = max(0, min(len(m.viewerPanelLines(m.viewerPanelWidth(), colors(m.account.dark)))-height, v.offset+delta))
	case "n":
		if v.next == "" || v.loading {
			return m, nil
		}
		return m, m.fetchViewerReplies(v.ctx, v.next)
	case "p":
		v.hidePanel = !v.hidePanel
	case "y":
		raw := v.commit.Images[v.index].URL
		if !validImageURL(raw) {
			v.status = "This image has no safe link to copy."
			return m, nil
		}
		return m, m.copyText(raw, "Image link")
	}
	return m, nil
}

func (m AppModel) viewerPanelWidth() int {
	if m.viewer.hidePanel || m.account.width < 70 {
		return 0
	}
	return min(44, m.account.width*2/5)
}

func (m AppModel) viewerImageBounds() uiRect {
	w := m.account.width
	if panel := m.viewerPanelWidth(); panel > 0 {
		w -= panel + 1
	}
	return uiRect{0, 2, max(1, w), max(1, m.account.height-2)}
}

func (m AppModel) viewerRequest() graphics.Request {
	image := m.viewer.commit.Images[m.viewer.index]
	r := m.viewerImageBounds()
	w, h := image.Width, image.Height
	if w <= 0 || h <= 0 {
		w, h = 4, 3
	}
	cols, rows := graphics.Fit(w, h, max(1, r.w-8), max(1, r.h-3))
	return graphics.Request{URL: image.URL, Cols: cols, Rows: rows}
}

func (m AppModel) viewerLayout(p palette) (string, []hitArea) {
	v := m.viewer
	w, h := max(1, m.account.width), max(1, m.account.height)
	count := len(v.commit.Images)
	image := v.commit.Images[v.index]
	var areas []hitArea
	toolbar := ""
	for _, action := range []struct{ key, label string }{{"y", "Copy link"}, {"p", "Panel"}, {"esc", "Close"}} {
		chip := keyChip(action.key, p) + " " + text(p.ink).Render(action.label)
		toolbar += "  " + chip
		areas = append(areas, hitArea{uiRect{lipgloss.Width(toolbar) - lipgloss.Width(chip), 0, lipgloss.Width(chip), 1}, action.key, 0})
	}
	title := " " + text(p.ink).Bold(true).Render(fmt.Sprintf("Image %d / %d", v.index+1, count))
	if v.status != "" {
		title += "  " + text(p.muted).Render(v.status)
	}
	title = fit(title, max(1, w-lipgloss.Width(toolbar)-1))
	offset := max(lipgloss.Width(title)+1, w-lipgloss.Width(toolbar)-1)
	for i := range areas {
		areas[i].x += offset
	}
	lines := []string{title + strings.Repeat(" ", offset-lipgloss.Width(title)) + toolbar, text(p.hairline).Render(strings.Repeat("─", w))}

	r := m.viewerImageBounds()
	picture := make([]string, r.h)
	req := m.viewerRequest()
	caption := safeText(image.Description)
	var body []string
	if m.images.Enabled && m.images.Row(req, 0) != "" {
		for row := range req.Rows {
			body = append(body, m.images.Row(req, row))
		}
		if caption != "" {
			body = append(body, "", text(p.muted).Render(fit(caption, r.w-8)))
		}
	} else {
		if caption == "" {
			caption = "Image"
		}
		body = append(body, text(p.ink).Bold(true).Render(fit("▧ "+caption, r.w-8)), "")
		for _, line := range strings.Split(ansi.Hardwrap(safeText(image.URL), max(1, r.w-8), true), "\n") {
			body = append(body, text(p.muted).Render(line))
		}
	}
	top := max(0, (r.h-len(body))/2)
	for row := range picture {
		line := ""
		if row >= top && row-top < len(body) {
			line = body[row-top]
		}
		picture[row] = lipgloss.PlaceHorizontal(r.w, lipgloss.Center, line)
	}
	middle := r.h / 2
	if v.index > 0 {
		picture[middle] = text(p.ink).Bold(true).Render(" ‹") + ansi.Cut(picture[middle], 2, r.w)
		areas = append(areas, hitArea{uiRect{0, r.y + middle, 3, 1}, "left", 0})
	}
	if v.index < count-1 {
		picture[middle] = ansi.Cut(picture[middle], 0, r.w-2) + text(p.ink).Bold(true).Render("› ")
		areas = append(areas, hitArea{uiRect{r.w - 3, r.y + middle, 3, 1}, "right", 0})
	}

	panelWidth := m.viewerPanelWidth()
	panel := m.viewerPanel(panelWidth, r.h, p)
	rail := text(p.hairline).Render("│")
	for row := range picture {
		line := picture[row]
		if panelWidth > 0 {
			line += rail + panel[row]
		}
		lines = append(lines, line)
	}
	page := lipgloss.NewStyle().Width(w).Height(h).MaxWidth(w).MaxHeight(h).Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).Render(strings.Join(lines, "\n"))
	return page, areas
}

func (m AppModel) viewerPanel(width, height int, p palette) []string {
	if width <= 0 {
		return nil
	}
	lines := m.viewerPanelLines(width, p)
	start := max(0, min(m.viewer.offset, len(lines)-height))
	if start+height < len(lines) && height > 1 {
		lines = append(lines[start:start+height-1:start+height-1], text(p.muted).Render("↓ more · j/k scroll"))
	} else {
		lines = lines[start:]
	}
	rows := make([]string, height)
	for i := range rows {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		rows[i] = lipgloss.NewStyle().Width(width).MaxWidth(width).PaddingLeft(2).Render(line)
	}
	return rows
}

func (m AppModel) viewerPanelLines(width int, p palette) []string {
	if width <= 0 {
		return nil
	}
	v := m.viewer
	gutter := 5
	if m.images.Enabled {
		gutter = 7
	}
	contentWidth := max(1, width-3-gutter)
	commit := v.commit
	commit.Images, commit.ReplyPreview = nil, nil
	rows := commitLines(commit, contentWidth, p, m.now)
	ring := -1
	if commit.DeletedAt == nil && commit.Author != nil && len(rows) > 1 {
		ring = len(rows) - 1
	}
	var heads []int
	for _, reply := range v.replies {
		rows = append(rows, "")
		tag := ""
		if reply.QuotedID != nil && *reply.QuotedID == v.commit.ID {
			reply.QuotedID, tag = nil, "quoted"
		}
		node := text(p.muted).Render("○ ")
		if reply.DeletedAt == nil && reply.Author != nil {
			node = smallAvatar(reply.Author, p, m.images)
		}
		heads = append(heads, len(rows))
		replyRows, _ := commitLayout(reply, tag, max(1, contentWidth-3), p, m.now, nil)
		for i, line := range replyRows {
			if i == 0 {
				rows = append(rows, node+line)
				continue
			}
			rows = append(rows, "   "+line)
		}
	}
	lane := authorColor(commit.Author, p)
	if commit.DeletedAt != nil {
		lane = p.muted
	}
	lines := m.feedRail(commit, rows, ring, heads, gutter, lane, p)
	indent := strings.Repeat(" ", gutter)
	switch {
	case v.loading:
		lines = append(lines, "", indent+text(p.muted).Render("Loading replies..."))
	case len(v.replies) == 0:
		lines = append(lines, "", indent+text(p.muted).Render("No replies yet."))
	}
	if v.next != "" {
		lines = append(lines, "", indent+text(p.muted).Render("More replies ")+keyChip("n", p))
	}
	return lines
}

func (m AppModel) viewerMouse(x, y int) (AppModel, tea.Cmd) {
	_, areas := m.viewerLayout(colors(m.account.dark))
	for _, area := range areas {
		if area.contains(x, y) {
			return m.mouseKey(area.action)
		}
	}
	return m, nil
}
