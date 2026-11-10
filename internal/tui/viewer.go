package tui

import (
	"context"
	"fmt"
	"slices"
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
	// avatar shows one profile picture, named by label, with no commit panel.
	avatar            bool
	label             string
	commit            api.Commit
	index, generation int
	offset            int
	replies           []api.Commit
	next, status      string
	ctx               context.Context
	cancel            context.CancelFunc
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

// avatarViewSize is the size the viewer asks for, as the web's profile picture dialog does.
const avatarViewSize = 1024

// openAvatar shows a profile picture in the image viewer.
func (m *AppModel) openAvatar(label, raw string) tea.Cmd {
	if strings.TrimSpace(raw) == "" {
		m.notice = "This avatar has no image to view."
		return nil
	}
	m.closeViewer()
	image := api.Image{URL: graphics.AvatarSizedURL(raw, avatarViewSize), Width: avatarViewSize, Height: avatarViewSize, Description: label}
	m.viewer = viewerState{open: true, avatar: true, label: label, commit: api.Commit{Author: &api.Author{}, Images: []api.Image{image}}, generation: m.viewer.generation + 1}
	m.resetSequence()
	return nil
}

// avatarHit is a drawn avatar that a click opens in the viewer.
type avatarHit struct {
	area       uiRect
	label, url string
}

func authorHit(area uiRect, a *api.Author) avatarHit {
	return avatarHit{area, "@" + safeText(a.Handle), a.AvatarURL}
}

func userHit(area uiRect, u *api.User) avatarHit {
	return avatarHit{area, "@" + safeText(u.Handle), u.AvatarURL}
}

// openAvatarAt opens the avatar under x and y, and reports whether there was one.
func (m *AppModel) openAvatarAt(hits []avatarHit, x, y int) (tea.Cmd, bool) {
	for _, hit := range hits {
		if hit.area.contains(x, y) {
			return m.openAvatar(hit.label, hit.url), true
		}
	}
	return nil, false
}

func (m *AppModel) openAuthorAvatar(a *api.Author) tea.Cmd {
	if a == nil {
		return nil
	}
	return m.openAvatar("@"+safeText(a.Handle), a.AvatarURL)
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
		v.offset = max(0, min(len(m.viewerPanelLines(m.viewerPanelWidth(), m.account.colors()))-height, v.offset+delta))
	case "n":
		if v.next == "" || v.loading {
			return m, nil
		}
		return m, m.fetchViewerReplies(v.ctx, v.next)
	case "p":
		v.hidePanel = !v.hidePanel || v.avatar
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
	if m.viewer.hidePanel || m.viewer.avatar || m.account.width < 70 {
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
	tools := []struct{ key, label string }{{"y", "Copy link"}, {"p", "Panel"}, {"esc", "Close"}}
	if v.avatar {
		tools = slices.Delete(tools, 1, 2)
	}
	for _, action := range tools {
		chip := actionChip(action.label, action.key, p)
		toolbar += "  " + chip
		areas = append(areas, hitArea{uiRect{lipgloss.Width(toolbar) - lipgloss.Width(chip), 0, lipgloss.Width(chip), 1}, action.key, 0})
	}
	title := " " + text(p.ink).Bold(true).Render(fmt.Sprintf("Image %d / %d", v.index+1, count))
	if v.avatar {
		title = " " + text(p.ink).Bold(true).Render("Avatar · "+v.label)
	}
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
	panel, panelAreas := m.viewerPanel(panelWidth, r.h, p)
	for _, area := range panelAreas {
		area.x += r.w + 1
		area.y += r.y
		areas = append(areas, area)
	}
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

// viewerPanel draws the reply panel's rows. Its hit areas are the scroll chip on the last row, in the panel's cells.
func (m AppModel) viewerPanel(width, height int, p palette) ([]string, []hitArea) {
	if width <= 0 {
		return nil, nil
	}
	lines := m.viewerPanelLines(width, p)
	start := max(0, min(m.viewer.offset, len(lines)-height))
	var areas []hitArea
	if start+height < len(lines) && height > 1 {
		more, moreAreas := hintRow([]statusPart{hintText("↓ more"), hintChip("Scroll", "j/k", "j")}, max(1, width-2), p)
		for _, area := range moreAreas {
			area.x += 2
			area.y = height - 1
			areas = append(areas, area)
		}
		lines = append(lines[start:start+height-1:start+height-1], more)
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
	return rows, areas
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
	rows, _ := commitLayout(commit, "", contentWidth, p, m.now, nil, m.statsStyle())
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
			node = smallAvatar(reply.Author, p, m.images, m.now)
		}
		heads = append(heads, len(rows))
		replyRows, _ := commitLayout(reply, tag, max(1, contentWidth-3), p, m.now, nil, m.statsStyle())
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
		lines = append(lines, "", indent+actionChip("More replies", "n", p))
	}
	return lines
}

func (m AppModel) viewerMouse(x, y int) (AppModel, tea.Cmd) {
	_, areas := m.viewerLayout(m.account.colors())
	for _, area := range areas {
		if area.contains(x, y) {
			pressed, _ := keyAction(area.action)
			return m.mouseKey(pressed)
		}
	}
	return m, nil
}
