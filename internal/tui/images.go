package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func (m AppModel) avatarRequest(url, lane string, cols, rows int) graphics.Request {
	return graphics.Request{URL: graphics.AvatarURL(url), Ring: lane, Cols: cols, Rows: rows}
}

func (m AppModel) authorAvatar(author *api.Author, p palette, cols, rows int) graphics.Request {
	return authorRequest(author, p, m.now, cols, rows)
}

// authorRequest rings a commit author's avatar in their lane, or red while they are live, and squares an organization's.
func authorRequest(author *api.Author, p palette, now time.Time, cols, rows int) graphics.Request {
	req := graphics.Request{URL: graphics.AvatarURL(author.AvatarURL), Ring: authorColor(author, p), Cols: cols, Rows: rows, Square: author.Organization()}
	if author.Live.On(now) {
		req.Ring = p.danger
	}
	return req
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
	p := m.account.colors()
	var requests, avatars []graphics.Request
	var stars []graphics.Request
	if !m.notificationsOpen || len(m.threads) > 0 || len(m.profiles) > 0 || m.viewer.open {
		stars = []graphics.Request{starIconRequest(false, p.muted), starIconRequest(true, p.muted)}
	}
	if m.viewer.open {
		requests = []graphics.Request{m.viewerRequest()}
		if c := m.viewer.commit; c.Author != nil && c.DeletedAt == nil {
			avatars = append(avatars, m.authorAvatar(c.Author, p, 5, 2))
		}
		for _, reply := range m.viewer.replies {
			if reply.Author != nil && reply.DeletedAt == nil {
				avatars = append(avatars, m.authorAvatar(reply.Author, p, 2, 1))
			}
		}
	} else if m.notificationsOpen && len(m.threads) == 0 && len(m.profiles) == 0 {
		avatars = append(avatars, m.notificationMedia()...)
	} else if m.remotesOnTop() {
		requests = m.remotesMedia()
	} else {
		f := &m.feeds[m.active]
		commits, starts, ends, widths, view := f.commits, f.starts, f.ends, []int(nil), f.viewport
		if m.starredOpen {
			s := &m.starred
			commits, starts, ends, view = s.commits, s.starts, s.ends, s.viewport
		}
		if s := m.topProfile(); s != nil {
			commits, starts, ends, view = nil, s.starts, s.ends, s.viewport
			avatars = append(avatars, m.profileAvatars(s, p)...)
			if !s.people {
				commits = s.entries()
			}
		}
		if m.threadOnTop() {
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
			avatars = append(avatars, m.authorAvatar(c.Author, p, 5, 2))
			requests = append(requests, imageRequests(c.Images, width)...)
			if req, ok := linkCardRequest(c.LinkPreview); ok {
				requests = append(requests, req)
			}
			if c.Quote != nil && c.Quote.DeletedAt == nil && c.Quote.Author != nil {
				avatars = append(avatars, m.authorAvatar(c.Quote.Author, p, 2, 1))
				if req, ok := linkCardRequest(c.Quote.LinkPreview); ok {
					requests = append(requests, req)
				}
			}
			if c.ReplyPreview != nil {
				for _, reply := range c.ReplyPreview.Data {
					if reply.DeletedAt == nil && reply.Author != nil {
						avatars = append(avatars, m.authorAvatar(reply.Author, p, 2, 1))
					}
				}
			}
		}
	}
	for _, author := range m.arrivalAvatars() {
		avatars = append(avatars, m.authorAvatar(author, p, 2, 1))
	}
	u := m.account.user
	avatars = append(avatars, m.avatarRequest(u.AvatarURL, laneColor(u.Lane, p), 2, 1))
	if u.PasteisMode {
		stars = append(stars, pastelIconRequest)
	}
	if m.achievements != nil {
		stars = append(stars, m.achievementsMedia(p)...)
	}
	if m.picker != nil {
		avatars = append(m.listPickerMedia(p), avatars...)
	}
	avatars = append(m.suggestionModalMedia(p), avatars...)
	if nav, _, _ := m.columns(); nav >= 12 {
		stars = append(stars, logoRequest(m.account.dark, m.account.event))
	}
	avatars = append(avatars, m.avatarRequest(u.AvatarURL, liveRing(u.Live, laneColor(u.Lane, p), p, m.now), 4, 2))
	if _, _, aside := m.columns(); aside > 0 {
		requests = append(requests, m.liveNowRequests()...)
	}
	requests = append(m.composerThumbnails(), requests...)
	if c := m.composer; c.open && c.previewing {
		_, preview := previewRequests(c.images, max(1, m.composerBounds().w-4))
		requests = append(preview, requests...)
	}
	return append(append(requests, avatars...), stars...)
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

func smallAvatar(author *api.Author, p palette, manager *graphics.Manager, now time.Time) string {
	if manager != nil {
		row := manager.Row(authorRequest(author, p, now, 2, 1), 0)
		if row != "" {
			return row + " "
		}
	}
	return text(authorColor(author, p)).Render("● ")
}

// largeAvatar returns both rows of a 4x2 avatar, or a lane dot when it is not ready.
func (m AppModel) largeAvatar(author *api.Author, p palette) (string, string) {
	req := m.authorAvatar(author, p, 4, 2)
	if top := m.images.Row(req, 0); top != "" {
		return top + " ", m.images.Row(req, 1) + " "
	}
	return text(authorColor(author, p)).Render("● "), "  "
}

// previewRequests draws the composer's images from their files, sized like posted images.
func previewRequests(files []api.ImageFile, width int) ([]api.Image, []graphics.Request) {
	images := make([]api.Image, len(files))
	for i, file := range files {
		images[i] = api.Image{Width: file.Width, Height: file.Height, Description: file.Description}
	}
	requests := imageRequests(images, width)
	for i := range requests {
		requests[i].Path = files[i].Path
	}
	return images, requests
}

func pictureLines(images []api.Image, requests []graphics.Request, p palette, manager *graphics.Manager) []string {
	if manager == nil || !manager.Enabled {
		return nil
	}
	hasSource := false
	for _, req := range requests {
		hasSource = hasSource || req.URL != "" || req.Path != ""
	}
	if !hasSource {
		return nil
	}
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

// composerThumbnails lists the composer's local thumbnails: the attachment strip, or the browser's selected image.
func (m AppModel) composerThumbnails() []graphics.Request {
	if req, ok := m.browserThumbnail(); ok {
		return []graphics.Request{req}
	}
	if m.remotesOnTop() && !m.composer.open {
		return m.messageThumbnailRequests()
	}
	return m.stripThumbnailRequests()
}
