package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

const liveThumbCols, liveThumbRows = 8, 2

// platformName spells a stream platform the way the web does.
func platformName(platform string) string {
	switch platform {
	case "youtube":
		return "YouTube"
	case "twitch":
		return "Twitch"
	}
	return safeText(platform)
}

func liveLabel(live *auth.LiveStream, p palette) string {
	label := "LIVE"
	if live != nil && live.Platform != "" {
		label += " on " + platformName(live.Platform)
	}
	return text(p.danger).Bold(true).Render("● " + label)
}

// liveRing turns an avatar's ring red while its owner is live, as the web does.
func liveRing(live *auth.LiveStream, lane string, p palette, now time.Time) string {
	if live.On(now) {
		return p.danger
	}
	return lane
}

func (m AppModel) profileAvatar(u api.User, lane string, p palette) graphics.Request {
	return m.avatarRequest(u.AvatarURL, liveRing(u.Live, lane, p, m.now), 8, 4)
}

func liveThumbnail(live *auth.LiveStream) graphics.Request {
	return graphics.Request{URL: live.ThumbnailURL, Cols: liveThumbCols, Rows: liveThumbRows}
}

func (m AppModel) liveModal(p palette) (string, uiRect, []hitArea) {
	d := m.live
	if d.ending {
		return m.endStreamModal(p)
	}
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 18)
	width := max(1, r.w-4)
	label := func(name string, focus int) string {
		if d.focus == focus {
			return text(p.blue).Render("▌ ") + text(p.ink).Bold(true).Render(name)
		}
		return "  " + text(p.muted).Render(name)
	}
	hours := fmt.Sprintf("‹ %s ›", plural(d.hours, "hour", "hours"))
	if d.focus == 1 {
		hours = text(p.ink).Bold(true).Render(hours)
	} else {
		hours = text(p.muted).Render(hours)
	}
	rows := []string{text(p.ink).Bold(true).Render("Go live"), "", label("Stream link", 0), "  " + d.field.View(), "", label("Show me as live for", 1), "  " + hours}
	if d.status != "" {
		color := p.warning
		if d.sending {
			color = p.muted
		}
		rows = append(rows, "")
		for _, line := range strings.Split(ansi.Wrap(d.status, width, ""), "\n") {
			rows = append(rows, text(color).Render(line))
		}
	}
	actions := []modalAction{{"Cancel", "esc", "esc"}, {"Go live", "enter", "enter"}}
	footer, areas := modalFooter(actions, width, 2, p)
	// The explanation goes first on a short terminal, so the fields and the server's reason stay visible.
	var about []string
	for _, line := range strings.Split(ansi.Wrap("Share your YouTube or Twitch stream. Your avatar gets a red ring and your commits say LIVE until it ends.", width, ""), "\n") {
		about = append(about, text(p.muted).Render(line))
	}
	room := r.h - 5 - lipgloss.Height(footer)
	switch {
	case len(rows)+len(about) <= room:
		rows = append(append([]string{rows[0]}, about...), rows[1:]...)
	case len(rows) > room:
		rows = slices.DeleteFunc(rows, func(row string) bool { return row == "" })
	}
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel("/dev/null · LIVE", strings.Join(rows, "\n"), footer, r, p, p.blue), r, areas
}

func (m AppModel) endStreamModal(p palette) (string, uiRect, []hitArea) {
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 10)
	width := max(1, r.w-4)
	body := text(p.ink).Bold(true).Render("End your stream?") + "\n\n" + text(p.muted).Render(fit("Your avatar and commits stop showing you as live.", width))
	footer, areas := modalFooter([]modalAction{{"Keep streaming", "esc", "esc"}, {"End stream", "enter", "enter"}}, width, 2, p)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel("/dev/null · LIVE", body, footer, r, p, p.blue), r, areas
}

// liveNowBlock is the web's Live now aside: up to three people you follow, each with its thumbnail and a click to watch.
func (m AppModel) liveNowBlock(width int, p palette) (string, []hitArea) {
	if len(m.liveNow) == 0 {
		return "", nil
	}
	rows := []string{split(text(p.danger).Bold(true).Render("● ")+text(p.ink).Bold(true).Render("Live now"), actionChip("Watch", "W", p), width)}
	var areas []hitArea
	for i, person := range m.liveNow {
		name := safeText(person.Name)
		if name == "" {
			name = safeText(person.Handle)
		}
		left := []string{text(p.danger).Render("▶"), " "}
		if req := liveThumbnail(person.Live); m.images.Enabled && req.URL != "" {
			if top, bottom := m.images.Row(req, 0), m.images.Row(req, 1); top != "" && bottom != "" {
				left = []string{top, bottom}
			}
		}
		gap := lipgloss.Width(left[0]) + 1
		inner := max(1, width-gap)
		detail := "@" + safeText(person.Handle)
		if platform := platformName(person.Live.Platform); platform != "" {
			detail += " · " + platform
		}
		areas = append(areas, hitArea{uiRect{0, len(rows), width, 2}, "watch", i})
		rows = append(rows,
			left[0]+" "+text(p.ink).Render(fit(name, inner)),
			left[1]+strings.Repeat(" ", max(0, gap-lipgloss.Width(left[1])))+text(p.muted).Render(fit(detail, inner)))
	}
	return strings.Join(rows, "\n"), areas
}

// liveAction is your profile's Go live or End stream action. It's blue, since red marks destructive actions.
func (m AppModel) liveAction(p palette) string {
	return actionChip(m.liveButton(), "L", p)
}

// liveButton is your own Go live or End stream action, shown on your profile and in the aside.
func (m AppModel) liveButton() string {
	if m.account.user.Live.On(m.now) {
		return "End stream"
	}
	return "Go live"
}

func (m AppModel) liveNowRequests() []graphics.Request {
	var requests []graphics.Request
	for _, person := range m.liveNow {
		if person.Live != nil && person.Live.ThumbnailURL != "" {
			requests = append(requests, liveThumbnail(person.Live))
		}
	}
	return requests
}
