package tui

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

var htmlTags = regexp.MustCompile(`<[^>]*>`)
var hexLane = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func remoteBody(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\u200d' || r == '\u200c' {
			return r
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, ansi.Strip(value))
}

func authorColor(author *api.Author, p palette) string {
	if author == nil {
		return p.muted
	}
	return laneColor(author.Lane, p)
}

func laneColor(raw json.RawMessage, p palette) string {
	var custom string
	if json.Unmarshal(raw, &custom) == nil && hexLane.MatchString(custom) {
		return custom
	}
	n, err := strconv.Atoi(string(raw))
	if err == nil && n >= 0 && n < len(p.lanes) {
		return p.lanes[n]
	}
	return p.muted
}

func withBackground(line, color string) string {
	background := ansi.Style{}.BackgroundColor(lipgloss.Color(color)).String()
	// Nested foreground styles reset the background as well.
	return background + strings.ReplaceAll(line, ansi.ResetStyle, ansi.ResetStyle+background) + ansi.ResetStyle
}

func relativeAge(created, now time.Time) string {
	if created.IsZero() {
		return ""
	}
	elapsed := now.Sub(created)
	switch {
	case elapsed < time.Minute:
		return "now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	case elapsed < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	default:
		return created.Format("Jan 02")
	}
}

func dayLabel(created, now time.Time) string {
	day, today := created.In(time.Local), now.In(time.Local)
	date := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	days := int(date(today).Sub(date(day)).Hours() / 24)
	switch {
	case days == 0:
		return "Today"
	case days == 1:
		return "Yesterday"
	case days >= 2 && days <= 6:
		return day.Format("Monday")
	case day.Year() == today.Year():
		return day.Format("Mon 2 Jan")
	default:
		return day.Format("Mon 2 Jan 2006")
	}
}

func fit(value string, width int) string { return ansi.Truncate(value, max(0, width), "…") }

func bodyText(commit api.Commit, p palette) string {
	segments := commit.Segments
	if len(commit.Preview) > 0 {
		segments = commit.Preview
	}
	if len(segments) == 0 {
		return remoteBody(commit.Body)
	}
	var b strings.Builder
	for _, s := range segments {
		switch s.Type {
		case "text":
			b.WriteString(remoteBody(s.Text))
		case "code":
			b.WriteString(text(p.code).Render("`" + remoteBody(s.Code) + "`"))
		case "block":
			plain := html.UnescapeString(htmlTags.ReplaceAllString(s.HTML, ""))
			b.WriteString("\n\n" + text(p.muted).Render(safeText(s.Language)) + "\n")
			for _, line := range strings.Split(remoteBody(plain), "\n") {
				b.WriteString(text(p.hairline).Render("│ ") + text(p.code).Render(line) + "\n")
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

type pictureArea struct {
	uiRect
	entry, image int
}

func commitLines(commit api.Commit, width int, p palette, now time.Time, managers ...*graphics.Manager) []string {
	var manager *graphics.Manager
	if len(managers) > 0 {
		manager = managers[0]
	}
	lines, _ := commitLayout(commit, "", width, p, now, manager)
	return lines
}

// commitLayout renders a commit, with an optional tag such as "quoted" after the handle.
func commitLayout(commit api.Commit, tag string, width int, p palette, now time.Time, manager *graphics.Manager) ([]string, []pictureArea) {
	if commit.DeletedAt != nil {
		return []string{text(p.muted).Render("Commit deleted")}, nil
	}
	if commit.Author == nil {
		return []string{text(p.muted).Render("Commit unavailable")}, nil
	}
	name := safeText(commit.Author.Name)
	if name == "" {
		name = safeText(commit.Author.Handle)
	}
	stamp := relativeAge(commit.CreatedAt, now)
	meta := text(p.ink).Bold(true).Render(name) + " " + text(p.muted).Render("@"+safeText(commit.Author.Handle))
	if tag != "" {
		meta += " " + text(p.blue).Render(tag)
	}
	if stamp != "" {
		meta += " " + text(p.muted).Render("· "+stamp)
	}
	if commit.Status != "" && commit.Status != "approved" {
		meta += text(p.code).Render(" · " + safeText(commit.Status))
	}
	hash := text(p.muted).Render(ansi.Truncate(safeText(commit.Hash), 7, ""))
	if hash != "" && width >= 40 {
		meta = fit(meta, width-lipgloss.Width(hash)-2)
		meta += strings.Repeat(" ", max(2, width-lipgloss.Width(meta)-lipgloss.Width(hash))) + hash
	}
	lines := []string{fit(meta, width)}
	for _, line := range strings.Split(ansi.Wrap(bodyText(commit, p), width, ""), "\n") {
		lines = append(lines, fit(line, width))
	}
	if commit.QuotedID != nil {
		if commit.Quote == nil || commit.Quote.DeletedAt != nil || commit.Quote.Author == nil {
			lines = append(lines, fit(text(p.muted).Render("╭ Quoted commit unavailable"), width))
		} else {
			quote := commit.Quote
			quoteWidth := max(1, width-4)
			lines = append(lines, text(p.hairline).Render("╭"+strings.Repeat("─", max(0, width-2))+"╮"))
			quotedName := safeText(quote.Author.Name)
			if quotedName == "" {
				quotedName = safeText(quote.Author.Handle)
			}
			header := smallAvatar(quote.Author, p, manager) + text(p.ink).Bold(true).Render(quotedName) + " " + text(p.muted).Render("@"+safeText(quote.Author.Handle))
			quoteLines := []string{fit(header, quoteWidth)}
			body := strings.Split(ansi.Wrap(bodyText(*quote, p), quoteWidth, ""), "\n")
			if len(body) > 5 {
				body = append(body[:4], text(p.muted).Render("…"))
			}
			quoteLines = append(quoteLines, body...)
			for _, line := range quoteLines {
				inside := lipgloss.NewStyle().Width(quoteWidth).MaxWidth(quoteWidth).Render(line)
				lines = append(lines, text(p.hairline).Render("│ ")+inside+text(p.hairline).Render(" │"))
			}
			lines = append(lines, text(p.hairline).Render("╰"+strings.Repeat("─", max(0, width-2))+"╯"))
		}
	}
	var areas []pictureArea
	pictures := pictureLines(commit.Images, width, p, manager)
	x := 0
	for i, req := range imageRequests(commit.Images, width) {
		if len(pictures) == 0 {
			break
		}
		areas = append(areas, pictureArea{uiRect{x, len(lines), req.Cols, len(pictures)}, 0, i})
		x += req.Cols + 1
	}
	lines = append(lines, pictures...)
	for i, image := range commit.Images {
		if len(pictures) > 0 {
			break
		}
		label := "▧ Image"
		if image.Description != "" {
			label += " · " + safeText(image.Description)
		}
		areas = append(areas, pictureArea{uiRect{0, len(lines), width, 1}, 0, i})
		lines = append(lines, fit(text(p.muted).Render(label), width))
	}
	if len(commit.Poll) > 0 && string(commit.Poll) != "null" {
		lines = append(lines, fit(text(p.muted).Render("▤ Poll · view voting on the web"), width))
	}
	label := "replies"
	if commit.Replies == 1 {
		label = "reply"
	}
	lines = append(lines, fit(text(p.muted).Render(fmt.Sprintf("☆ %-5d ↩ %d %s", commit.Stars, commit.Replies, label)), width))
	for _, reply := range previewReplies(commit) {
		preview := replyPreview(reply, width-3, p, manager)
		lines = append(lines, fit(text(authorColor(commit.Author, p)).Render("╰─ ")+preview[0], width))
		for _, line := range preview[1:] {
			lines = append(lines, fit("   "+line, width))
		}
	}
	return lines, areas
}

func previewReplies(commit api.Commit) []api.Commit {
	if commit.ReplyPreview == nil {
		return nil
	}
	var replies []api.Commit
	for _, reply := range commit.ReplyPreview.Data {
		if reply.DeletedAt == nil && reply.Author != nil {
			replies = append(replies, reply)
		}
	}
	return replies
}

// replyPreview renders a reply's author line and up to three body lines aligned under the name.
func replyPreview(reply api.Commit, width int, p palette, manager *graphics.Manager) []string {
	lines := []string{fit(smallAvatar(reply.Author, p, manager)+text(p.ink).Bold(true).Render(safeText(reply.Author.Name))+text(p.muted).Render(" @"+safeText(reply.Author.Handle)), width)}
	body := strings.Split(ansi.Wrap(bodyText(reply, p), max(1, width-3), ""), "\n")
	if len(body) > 4 {
		body = append(body[:3], "…")
	}
	for _, line := range body {
		lines = append(lines, fit("   "+line, width))
	}
	return lines
}

// feedRail prefixes a commit's rows with its rail: the avatar node, a solid line down to a ring at the stats row,
// and a dashed spine with elbows into each reply preview.
func (m AppModel) feedRail(c api.Commit, rows []string, ring int, heads []int, gutter int, lane string, p palette) []string {
	col := 2
	if m.images.Enabled {
		col = 3
	}
	last := -1
	if len(heads) > 0 {
		last = heads[len(heads)-1]
	}
	out := make([]string, len(rows))
	for row, body := range rows {
		cells := strings.Split(strings.Repeat(" ", gutter), "")
		glyph := ""
		switch {
		case row == 0:
			glyph = "●"
			if c.Status == "pending" {
				glyph = "◌"
			}
		case row < ring:
			glyph = "│"
		case row == ring:
			glyph = "◉"
		case row < last:
			glyph = "┆"
		}
		for _, head := range heads {
			if row != head {
				continue
			}
			glyph = "├"
			if head == last {
				glyph = "╰"
			}
			for extra := col + 1; extra < gutter; extra++ {
				cells[extra] = text(lane).Render("╌")
			}
		}
		if glyph != "" {
			cells[col] = text(lane).Render(glyph)
		}
		if row == 0 && (c.DeletedAt != nil || c.Author == nil) {
			cells[col] = text(p.muted).Render("○")
		}
		if m.images.Enabled && row < 2 && c.Author != nil && c.DeletedAt == nil {
			if avatar := m.images.Row(m.avatarRequest(c.Author.AvatarURL, lane, 5, 2), row); avatar != "" {
				cells[col-2], cells[col-1], cells[col], cells[col+1], cells[col+2] = avatar, "", "", "", ""
			}
		}
		out[row] = strings.Join(cells, "") + body
	}
	return out
}

func (m AppModel) columns() (nav, center, aside int) {
	w := m.account.width
	nav = 5
	if w >= 100 {
		nav = 25
	}
	if w >= 134 {
		aside = 40
	} else if w >= 124 {
		aside = 36
	}
	center = max(1, min(92, w-nav-aside-2))
	return
}

func (m *AppModel) reflow() {
	_, center, _ := m.columns()
	m.sizeComposer()
	for i := range m.feeds {
		f := &m.feeds[i]
		offset := f.viewport.YOffset()
		if f.selected < len(f.starts) {
			offset -= f.starts[f.selected]
		}
		f.viewport.SetWidth(center)
		header := feedHeaderHeight
		if i == m.active {
			header = m.headerHeight()
		}
		f.viewport.SetHeight(max(1, m.mainHeight()-header))
		m.renderFeed(i)
		if f.selected < len(f.starts) {
			f.viewport.SetYOffset(max(0, f.starts[f.selected]+offset))
			if f.starts[f.selected] >= f.viewport.YOffset()+f.viewport.Height() || f.ends[f.selected] <= f.viewport.YOffset() {
				f.viewport.SetYOffset(f.starts[f.selected])
			}
		}
	}
	m.sizeThreads()
}

func (m *AppModel) renderFeed(index int) {
	f := &m.feeds[index]
	p := colors(m.account.dark)
	width := max(1, f.viewport.Width())
	gutter := 5
	if m.images.Enabled {
		gutter = 7
	}
	contentWidth := max(1, width-gutter-2)
	lines := []string{}
	f.starts, f.ends, f.pictures, f.actions = nil, nil, nil, nil
	day := ""
	for i, c := range f.commits {
		lane := authorColor(c.Author, p)
		if c.DeletedAt != nil {
			lane = p.muted
		}
		date := dayLabel(c.CreatedAt, m.now)
		if index != 2 && date != day {
			lines = append(lines, "    "+text(p.muted).Render(fit(date, contentWidth)), text(p.hairline).Render(strings.Repeat("─", width)))
			day = date
		}
		f.starts = append(f.starts, len(lines))
		previews := previewReplies(c)
		c.ReplyPreview = nil
		rows, pictures := commitLayout(c, "", contentWidth, p, m.now, m.images)
		for _, picture := range pictures {
			picture.x += gutter
			picture.y += len(lines)
			picture.entry = i
			f.pictures = append(f.pictures, picture)
		}
		ring := -1
		if c.DeletedAt == nil && c.Author != nil && len(rows) > 1 {
			ring = len(rows) - 1
		}
		if i == f.selected {
			chips, areas := m.commitActions(c, commitAction{"Thread", "enter"}, contentWidth, p)
			for _, area := range areas {
				area.x += gutter
				area.y += len(lines) + len(rows)
				f.actions = append(f.actions, area)
			}
			rows = append(rows, chips...)
		}
		var heads []int
		if len(previews) > 0 {
			rows = append(rows, "")
		}
		for _, reply := range previews {
			heads = append(heads, len(rows))
			rows = append(rows, replyPreview(reply, contentWidth, p, m.images)...)
		}
		for _, line := range m.feedRail(c, rows, ring, heads, gutter, lane, p) {
			if i == f.selected {
				line = text(p.blue).Render("▌") + ansi.Cut(line, 1, width)
			}
			line = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line)
			if i == f.selected {
				line = withBackground(line, p.wash)
			}
			lines = append(lines, line)
		}
		blank := strings.Repeat(" ", width)
		if i == f.selected {
			blank = withBackground(blank, p.wash)
		}
		lines = append(lines, blank)
		f.ends = append(f.ends, len(lines))
		lines = append(lines, text(p.hairline).Render(strings.Repeat("─", width)))
	}
	if len(f.commits) == 0 && f.loaded {
		lines = append(lines, "", "     "+text(p.ink).Bold(true).Render("A quiet log."), "", "     "+text(p.muted).Render("Tab to explore another log."))
		if index == 0 && len(f.suggestions) > 0 {
			lines = append(lines, "", "     "+text(p.muted).Render("SUGGESTED · not your Following log"), "")
			for _, c := range f.suggestions {
				for _, line := range commitLines(c, contentWidth, p, m.now) {
					lines = append(lines, "         "+line)
				}
				lines = append(lines, "")
			}
		}
	}
	if len(f.commits) > 0 {
		ending := text(p.muted).Render("EOF")
		if f.next != "" {
			ending = text(p.muted).Render("Older commits ") + keyChip("n", p)
		}
		lines = append(lines, "", lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(ending), "")
	}
	f.viewport.SetContent(strings.Join(lines, "\n"))
}
