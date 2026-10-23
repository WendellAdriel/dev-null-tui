package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func (m AppModel) avatarRequest(url, lane string, cols, rows int) graphics.Request {
	return graphics.Request{URL: url, Ring: lane, Cols: cols, Rows: rows}
}

func imageRequests(images []api.Image, width int) []graphics.Request {
	count := min(len(images), 4)
	if count == 0 {
		return nil
	}
	cols := min(48, max(1, (width-count+1)/count))
	var requests []graphics.Request
	for _, image := range images[:count] {
		w, h := image.Width, image.Height
		if w <= 0 || h <= 0 {
			w, h = 4, 3
		}
		c, r := graphics.Fit(w, h, cols, 12)
		requests = append(requests, graphics.Request{URL: image.URL, Cols: c, Rows: r})
	}
	return requests
}

func (m *AppModel) syncImages() tea.Cmd {
	var requests []graphics.Request
	if m.connected && !m.accountOpen && m.images.Enabled && m.account.width >= 30 && m.account.height >= 10 {
		p := colors(m.account.dark)
		f := &m.feeds[m.active]
		for i, c := range f.commits {
			if i >= len(f.starts) || f.ends[i] <= f.viewport.YOffset() || f.starts[i] >= f.viewport.YOffset()+f.viewport.Height() || c.DeletedAt != nil || c.Author == nil {
				continue
			}
			requests = append(requests, m.avatarRequest(c.Author.AvatarURL, authorColor(c.Author, p), 4, 2))
			requests = append(requests, imageRequests(c.Images, max(1, f.viewport.Width()-9))...)
			if c.Quote != nil && c.Quote.DeletedAt == nil && c.Quote.Author != nil {
				requests = append(requests, m.avatarRequest(c.Quote.Author.AvatarURL, authorColor(c.Quote.Author, p), 2, 1))
			}
			if c.ReplyPreview != nil {
				for _, reply := range c.ReplyPreview.Data {
					if reply.DeletedAt == nil && reply.Author != nil {
						requests = append(requests, m.avatarRequest(reply.Author.AvatarURL, authorColor(reply.Author, p), 2, 1))
					}
				}
			}
		}
		u := m.account.user
		requests = append(requests, m.avatarRequest(u.AvatarURL, laneColor(u.Lane, p), 2, 1))
		_, _, aside := m.columns()
		if aside > 0 {
			requests = append(requests, m.avatarRequest(u.AvatarURL, laneColor(u.Lane, p), 4, 2))
		}
	}
	jobs, cleanup := m.images.Plan(m.account.life.ctx, requests)
	var commands []tea.Cmd
	if cleanup != "" {
		commands = append(commands, tea.Raw(cleanup))
	}
	for _, job := range jobs {
		commands = append(commands, m.account.life.command(func() tea.Msg { return m.images.Fetch(job) }))
	}
	return tea.Batch(commands...)
}

func smallAvatar(author *api.Author, p palette, manager *graphics.Manager) string {
	if manager != nil {
		row := manager.Row(graphics.Request{URL: author.AvatarURL, Ring: authorColor(author, p), Cols: 2, Rows: 1}, 0)
		if row != "" {
			return row + " "
		}
	}
	return text(authorColor(author, p)).Render("● ")
}

func pictureLines(images []api.Image, width int, p palette, manager *graphics.Manager) []string {
	if manager == nil || !manager.Enabled {
		return nil
	}
	hasURL := false
	for _, image := range images {
		hasURL = hasURL || image.URL != ""
	}
	if !hasURL {
		return nil
	}
	requests := imageRequests(images, width)
	rows := 0
	for _, req := range requests {
		rows = max(rows, req.Rows)
	}
	var lines []string
	for row := range rows {
		var cells []string
		for i, req := range requests {
			line := manager.Row(req, row)
			if line == "" {
				line = strings.Repeat(" ", req.Cols)
				if row == 0 {
					label := "▧ Image"
					if images[i].Description != "" {
						label += " · " + safeText(images[i].Description)
					}
					line = fit(label, req.Cols)
				}
				line = withBackground(line+strings.Repeat(" ", max(0, req.Cols-ansi.StringWidth(line))), p.soft)
			}
			cells = append(cells, line)
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	return lines
}
