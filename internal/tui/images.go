package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func (m AppModel) avatarRequest(url, lane string, cols, rows int) graphics.Request {
	return graphics.Request{URL: graphics.AvatarURL(url), Ring: lane, Cols: cols, Rows: rows}
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

// visibleMedia lists the images and avatars on screen, attached images first.
func (m AppModel) visibleMedia() []graphics.Request {
	if !m.connected || m.accountOpen || !m.images.Enabled || m.account.width < 30 || m.account.height < 10 {
		return nil
	}
	p := colors(m.account.dark)
	var requests, avatars []graphics.Request
	if m.viewer.open {
		requests = []graphics.Request{m.viewerRequest()}
		if c := m.viewer.commit; c.Author != nil && c.DeletedAt == nil {
			avatars = append(avatars, m.avatarRequest(c.Author.AvatarURL, authorColor(c.Author, p), 5, 2))
		}
		for _, reply := range m.viewer.replies {
			if reply.Author != nil && reply.DeletedAt == nil {
				avatars = append(avatars, m.avatarRequest(reply.Author.AvatarURL, authorColor(reply.Author, p), 2, 1))
			}
		}
	} else {
		f := &m.feeds[m.active]
		commits, starts, ends, widths, view := f.commits, f.starts, f.ends, []int(nil), f.viewport
		if len(m.threads) > 0 {
			t := m.threads[len(m.threads)-1]
			commits, starts, ends, widths, view = nil, t.starts, t.ends, t.widths, t.viewport
			for _, entry := range t.entries() {
				entry.commit.ReplyPreview = nil
				if entry.kind == "quote" {
					entry.commit.Quote = nil
				}
				commits = append(commits, entry.commit)
			}
		}
		for i, c := range commits {
			if i >= len(starts) || ends[i] <= view.YOffset() || starts[i] >= view.YOffset()+view.Height() || c.DeletedAt != nil || c.Author == nil {
				continue
			}
			width := max(1, view.Width()-9)
			if i < len(widths) {
				width = widths[i]
			}
			avatars = append(avatars, m.avatarRequest(c.Author.AvatarURL, authorColor(c.Author, p), 5, 2))
			requests = append(requests, imageRequests(c.Images, width)...)
			if c.Quote != nil && c.Quote.DeletedAt == nil && c.Quote.Author != nil {
				avatars = append(avatars, m.avatarRequest(c.Quote.Author.AvatarURL, authorColor(c.Quote.Author, p), 2, 1))
			}
			if c.ReplyPreview != nil {
				for _, reply := range c.ReplyPreview.Data {
					if reply.DeletedAt == nil && reply.Author != nil {
						avatars = append(avatars, m.avatarRequest(reply.Author.AvatarURL, authorColor(reply.Author, p), 2, 1))
					}
				}
			}
		}
	}
	u := m.account.user
	avatars = append(avatars, m.avatarRequest(u.AvatarURL, laneColor(u.Lane, p), 2, 1))
	if _, _, aside := m.columns(); aside > 0 {
		avatars = append(avatars, m.avatarRequest(u.AvatarURL, laneColor(u.Lane, p), 4, 2))
	}
	return append(requests, avatars...)
}

func (m *AppModel) syncImages() tea.Cmd {
	jobs, cleanup := m.images.Plan(m.account.life.ctx, m.visibleMedia())
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
		row := manager.Row(graphics.Request{URL: graphics.AvatarURL(author.AvatarURL), Ring: authorColor(author, p), Cols: 2, Rows: 1}, 0)
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
