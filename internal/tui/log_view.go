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

func commitLines(commit api.Commit, width int, p palette, now time.Time, managers ...*graphics.Manager) []string {
	var manager *graphics.Manager
	if len(managers) > 0 {
		manager = managers[0]
	}
	if commit.DeletedAt != nil {
		return []string{text(p.muted).Render("Commit deleted")}
	}
	if commit.Author == nil {
		return []string{text(p.muted).Render("Commit unavailable")}
	}
	name := safeText(commit.Author.Name)
	if name == "" {
		name = safeText(commit.Author.Username)
	}
	stamp := relativeAge(commit.CreatedAt, now)
	meta := text(p.ink).Bold(true).Render(name) + " " + text(p.muted).Render("@"+safeText(commit.Author.Username))
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
				quotedName = safeText(quote.Author.Username)
			}
			header := smallAvatar(quote.Author, p, manager) + text(p.ink).Bold(true).Render(quotedName) + " " + text(p.muted).Render("@"+safeText(quote.Author.Username))
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
	pictures := pictureLines(commit.Images, width, p, manager)
	lines = append(lines, pictures...)
	for _, image := range commit.Images {
		if len(pictures) > 0 {
			break
		}
		label := "▧ Image"
		if image.Description != "" {
			label += " · " + safeText(image.Description)
		}
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
	if commit.ReplyPreview != nil {
		for _, reply := range commit.ReplyPreview.Data {
			if reply.DeletedAt != nil || reply.Author == nil {
				continue
			}
			lines = append(lines, fit(text(authorColor(commit.Author, p)).Render("╰─ ")+smallAvatar(reply.Author, p, manager)+text(p.ink).Bold(true).Render(safeText(reply.Author.Name))+text(p.muted).Render(" @"+safeText(reply.Author.Username)), width))
			body := strings.Split(ansi.Wrap(bodyText(reply, p), max(1, width-5), ""), "\n")
			if len(body) > 4 {
				body = append(body[:3], "…")
			}
			for _, line := range body {
				lines = append(lines, fit("     "+line, width))
			}
		}
	}
	return lines
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
	f.starts, f.ends = nil, nil
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
		rows := commitLines(c, contentWidth, p, m.now, m.images)
		for row, body := range rows {
			marker := " "
			if i == f.selected {
				marker = text(p.blue).Render("▌")
			}
			node := " "
			if c.ReplyPreview != nil && len(c.ReplyPreview.Data) > 0 {
				node = text(lane).Render("│")
			}
			if row == 0 {
				node = text(lane).Render("●")
				if c.Status == "pending" {
					node = text(lane).Render("◌")
				}
				if c.DeletedAt != nil || c.Author == nil {
					node = text(p.muted).Render("○")
				}
			}
			prefix := marker + " " + node + "  "
			if m.images.Enabled {
				prefix = marker + "  " + node + "   "
				if row < 2 && c.Author != nil && c.DeletedAt == nil {
					avatar := m.images.Row(m.avatarRequest(c.Author.AvatarURL, lane, 4, 2), row)
					if avatar != "" {
						prefix = marker + " " + avatar + " "
					}
				}
			}
			line := prefix + body
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
