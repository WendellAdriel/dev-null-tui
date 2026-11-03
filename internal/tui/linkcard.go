package tui

import (
	"net/url"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

const linkCardImageCols, linkCardImageRows = 10, 5

// linkCardRequest is the card's picture, drawn left of the text as the web's 80px square is.
func linkCardRequest(preview *api.LinkPreview) (graphics.Request, bool) {
	if preview == nil || preview.Pending || preview.Title == "" || preview.ImageURL == "" {
		return graphics.Request{}, false
	}
	return graphics.Request{URL: preview.ImageURL, Cols: linkCardImageCols, Rows: linkCardImageRows}, true
}

// linkCardLines draws a link card as the web's link-preview.tsx does: the site, a two-line title linking to the page,
// and a two-line description. A pending card waits, and a card without a title is not drawn.
func linkCardLines(preview *api.LinkPreview, width int, p palette, manager *graphics.Manager) []string {
	if preview == nil || width < 12 || (!preview.Pending && preview.Title == "") {
		return nil
	}
	inner := width - 4
	var body []string
	if preview.Pending {
		body = []string{text(p.muted).Render("Loading preview…")}
	} else {
		var picture []string
		if req, ok := linkCardRequest(preview); ok && manager != nil && manager.Enabled && inner >= 30 {
			for row := range linkCardImageRows {
				if line := manager.Row(req, row); line != "" {
					picture = append(picture, line)
				}
			}
			if len(picture) != linkCardImageRows {
				picture = nil
			}
		}
		textWidth := inner
		if picture != nil {
			textWidth = inner - linkCardImageCols - 2
		}
		site := safeText(preview.SiteName)
		if site == "" {
			site = linkHost(preview.URL)
		}
		body = []string{text(p.muted).Render(fit(site, textWidth))}
		title := clampLines(safeText(preview.Title), textWidth, 2)
		for _, line := range title {
			body = append(body, linked(preview.URL, text(p.ink).Bold(true).Render)(line))
		}
		for _, line := range clampLines(safeText(preview.Description), textWidth, 2) {
			body = append(body, text(p.muted).Render(line))
		}
		if picture != nil {
			for len(body) < len(picture) {
				body = append(body, "")
			}
			for i := range body {
				left := strings.Repeat(" ", linkCardImageCols)
				if i < len(picture) {
					left = picture[i]
				}
				body[i] = left + "  " + body[i]
			}
		}
	}
	border := text(p.hairline)
	lines := []string{border.Render("╭" + strings.Repeat("─", width-2) + "╮")}
	for _, line := range body {
		cell := lipgloss.NewStyle().Width(inner).MaxWidth(inner).Render(line)
		lines = append(lines, border.Render("│ ")+cell+border.Render(" │"))
	}
	return append(lines, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
}

func linkHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return safeText(raw)
	}
	return safeText(strings.TrimPrefix(u.Hostname(), "www."))
}

// clampLines wraps plain text to width and keeps at most limit lines, ending a cut one with an ellipsis.
func clampLines(value string, width, limit int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || width <= 0 {
		return nil
	}
	lines := strings.Split(ansi.Wrap(value, width, ""), "\n")
	if len(lines) <= limit {
		return lines
	}
	lines = lines[:limit]
	lines[limit-1] = ansi.Truncate(lines[limit-1]+" …", width, "…")
	return lines
}
