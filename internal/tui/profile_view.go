package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

var activityWindows = map[string]string{"day": "24 hours", "week": "7 days", "month": "30 days", "year": "year"}

// activityMix is the web's share of the lane colour at each activity level.
var activityMix = [5]float64{0, 0.32, 0.56, 0.80, 1}

// formatCount matches the web: separators below 10,000, then a truncated compact form such as 10.2k.
func formatCount(n int) string {
	if n >= 10_000 || n <= -10_000 {
		value, unit := float64(n)/1_000, "k"
		if n >= 1_000_000 || n <= -1_000_000 {
			value, unit = float64(n)/1_000_000, "m"
		}
		return strings.TrimSuffix(strconv.FormatFloat(math.Trunc(value*10)/10, 'f', 1, 64), ".0") + unit
	}
	digits := strconv.Itoa(max(n, -n))
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	if n < 0 {
		return "-" + digits
	}
	return digits
}

func plural(n int, one, many string) string {
	if n == 1 {
		return formatCount(n) + " " + one
	}
	return formatCount(n) + " " + many
}

// mixColor blends two #rrggbb colours, taking share of the first.
func mixColor(a, b string, share float64) string {
	parse := func(hex string, i int) float64 {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		return float64(v)
	}
	if !hexLane.MatchString(a) || !hexLane.MatchString(b) {
		return a
	}
	out := "#"
	for i := range 3 {
		out += fmt.Sprintf("%02x", int(math.Round(parse(a, i)*share+parse(b, i)*(1-share))))
	}
	return out
}

func activityLevel(count, most int) int {
	if count <= 0 || most <= 0 {
		return 0
	}
	return max(1, int(math.Ceil(float64(count)/float64(most)*4)))
}

func activityColor(level int, lane string, p palette) string {
	if level == 0 {
		return p.hairline
	}
	return mixColor(lane, p.shell, activityMix[level])
}

func displayURL(raw string) string {
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	return safeText(strings.TrimSuffix(strings.TrimPrefix(raw, "www."), "/"))
}

// spread places items on rows of width, wrapping before an item that does not fit.
func spread(items []string, width int, gap string) []string {
	var rows []string
	row := ""
	for _, item := range items {
		if row != "" && lipgloss.Width(row)+lipgloss.Width(gap)+lipgloss.Width(item) > width {
			rows, row = append(rows, row), ""
		}
		if row != "" {
			row += gap
		}
		row += item
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}

// split puts left and right on one row of width, truncating left first.
func split(left, right string, width int) string {
	left = fit(left, max(0, width-lipgloss.Width(right)-1))
	return left + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

func chip(label, color string, p palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Background(lipgloss.Color(p.wash)).Render(" " + label + " ")
}

func followLabel(following bool) string {
	if following {
		return "Following"
	}
	return "Follow"
}

type profileFrame struct {
	lines   []string
	actions []hitArea
}

func (f *profileFrame) add(lines ...string) { f.lines = append(f.lines, lines...) }

func (f *profileFrame) target(x, width int, action string) {
	f.actions = append(f.actions, hitArea{uiRect{x, len(f.lines), width, 1}, action, 0})
}

func (m *AppModel) renderProfile(s *profileState) {
	p := colors(m.account.dark)
	width := max(1, s.viewport.Width())
	frame := &profileFrame{}
	if !s.people {
		m.profileHeader(frame, s, width, p)
	}
	tabs := profileTabNames
	if s.people {
		tabs = connectionTabs
	}
	m.profileTabs(frame, tabs, s.tab, width, p)
	l := s.lists[s.tab]
	s.starts, s.ends, s.pictures = nil, nil, nil
	if s.people {
		m.peopleRows(frame, s, width, p)
	} else {
		pinned := s.tab == 0 && s.profile.Pinned != nil
		list := m.commitList(frame.lines, s.entries(), l.selected, width, func(i int, _ api.Commit) []string {
			if i == 0 && pinned {
				return []string{"", "     " + text(p.muted).Render("Pinned")}
			}
			return nil
		})
		frame.lines, s.starts, s.ends, s.pictures = list.lines, list.starts, list.ends, list.pictures
		frame.actions = append(frame.actions, list.actions...)
	}
	switch {
	case l.status != "":
		frame.add("", "     "+text(p.muted).Render(fit(l.status, width-6)))
	case !l.loaded:
		frame.add("", "     "+text(p.muted).Render("Reading from /dev/null..."))
	case s.count() == 0:
		frame.add("", "     "+text(p.muted).Render(profileEmpty(s)))
	case l.next != "":
		frame.add("", lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(text(p.muted).Render("More ")+keyChip("n", p)), "")
	default:
		frame.add("", lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(text(p.muted).Render("EOF")), "")
	}
	s.actions = frame.actions
	s.viewport.SetContent(strings.Join(frame.lines, "\n"))
}

func profileEmpty(s *profileState) string {
	if s.people {
		return [2]string{"No followers yet.", "Not following anyone yet."}[s.tab]
	}
	return [2]string{"No commits yet.", "No replies yet."}[s.tab]
}

func (m AppModel) profileHeader(frame *profileFrame, s *profileState, width int, p palette) {
	inner := max(1, width-4)
	pad := "  "
	if !s.loaded {
		title, detail := "Reading this profile...", "Fetching it with your signed connection."
		if s.status != "" {
			title, detail = "Couldn't load this profile", s.status
		}
		frame.add("", pad+text(p.ink).Bold(true).Render(fit(title, inner)), pad+text(p.muted).Render(fit(detail, inner)), "", text(p.hairline).Render(strings.Repeat("─", width)))
		return
	}
	u, profile := s.profile.User, s.profile
	lane := laneColor(u.Lane, p)
	follow := ""
	if profile.IsFollowing != nil && !strings.EqualFold(u.Handle, m.account.user.Handle) {
		follow = text(p.ink).Bold(true).Render(followLabel(*profile.IsFollowing)) + " " + keyChip("f", p)
	}
	frame.add("")
	var avatar []string
	req := m.avatarRequest(u.AvatarURL, lane, 8, 4)
	for row := range 4 {
		line := m.images.Row(req, row)
		if line == "" {
			avatar = nil
			break
		}
		avatar = append(avatar, line)
	}
	name := text(p.ink).Bold(true).Render(safeText(u.Name))
	if u.Name == "" {
		name = text(p.ink).Bold(true).Render(safeText(u.Handle))
	}
	if len(avatar) > 0 {
		for row, line := range avatar {
			if row == 0 && follow != "" {
				frame.target(2+inner-lipgloss.Width(follow), lipgloss.Width(follow), "f")
				line = split(line, follow, inner)
			}
			frame.add(pad + line)
		}
		frame.add("", pad+fit(name, inner))
	} else {
		if follow != "" {
			frame.target(2+inner-lipgloss.Width(follow), lipgloss.Width(follow), "f")
			name = split(name, follow, inner)
		}
		frame.add(pad + fit(name, inner))
	}
	badges := []string{text(lane).Render("●") + " " + text(p.muted).Render("@"+safeText(u.Handle))}
	if a := profile.Achievements; a != nil {
		badges = append(badges, chip(fmt.Sprintf("Lv. %d · %s", a.Level, safeText(a.Rank)), lane, p))
	}
	if profile.FollowsYou != nil && *profile.FollowsYou {
		badges = append(badges, chip("Follows you", p.muted, p))
	}
	if profile.IsMuting != nil && *profile.IsMuting {
		badges = append(badges, chip("Muted", p.warning, p))
	}
	for _, row := range spread(badges, inner, "  ") {
		frame.add(pad + row)
	}
	if u.Bio != "" {
		frame.add("")
		for _, line := range strings.Split(ansi.Wrap(remoteBody(u.Bio), inner, ""), "\n") {
			frame.add(pad + text(p.ink).Render(fit(line, inner)))
		}
	}
	var meta []string
	for _, item := range []struct{ icon, value, color string }{
		{m.icons.company, safeText(u.Company), p.muted},
		{m.icons.location, safeText(u.Location), p.muted},
		{m.icons.link, displayURL(u.Website), p.blue},
		{m.icons.github, displayURL(u.GitHubURL), p.blue},
	} {
		if item.value != "" {
			meta = append(meta, text(p.muted).Render(item.icon+" ")+text(item.color).Render(item.value))
		}
	}
	if !u.CreatedAt.IsZero() {
		meta = append(meta, text(p.muted).Render(m.icons.joined+" Joined "+u.CreatedAt.In(time.Local).Format("January 2006")))
	}
	if len(meta) > 0 {
		frame.add("")
		for _, row := range spread(meta, inner, "   ") {
			frame.add(pad + row)
		}
	}
	var counts []string
	x := 2
	for _, count := range []struct {
		value               *int
		one, many, shortcut string
	}{{u.Commits, "commit", "commits", ""}, {u.Following, "following", "following", "4"}, {u.Followers, "follower", "followers", "3"}} {
		if count.value == nil {
			continue
		}
		label := strings.SplitN(plural(*count.value, count.one, count.many), " ", 2)
		item := text(p.ink).Bold(true).Render(label[0]) + text(p.muted).Render(" "+label[1])
		if len(counts) > 0 {
			x += 3
		}
		if count.shortcut != "" {
			frame.actions = append(frame.actions, hitArea{uiRect{x, len(frame.lines) + 1, lipgloss.Width(item), 1}, count.shortcut, 0})
		}
		x += lipgloss.Width(item)
		counts = append(counts, item)
	}
	if len(counts) > 0 {
		frame.add("", pad+fit(strings.Join(counts, "   "), inner))
	}
	if profile.PasteisReceived != nil && m.account.user.PasteisMode {
		frame.add(pad + text(p.muted).Render(fit("◍ "+plural(*profile.PasteisReceived, "pastél received", "pastéis received"), inner)))
	}
	hairline := text(p.hairline).Render(strings.Repeat("─", width))
	frame.add("", hairline)
	if activity := profile.Activity; activity != nil && len(activity.Buckets) > 0 {
		frame.add("")
		m.activityGraph(frame, activity, lane, inner, p)
		frame.add("", hairline)
	}
	if streaks := profile.Streaks; streaks != nil {
		line := text(p.muted).Render("longest streak ") + text(p.ink).Bold(true).Render(formatCount(streaks.Longest)) + text(p.muted).Render(" "+map[bool]string{true: "day", false: "days"}[streaks.Longest == 1]+" · ")
		line += text(lane).Render("●") + text(p.muted).Render(" current ") + text(p.ink).Bold(true).Render(formatCount(streaks.Current))
		frame.add("", pad+fit(line, inner), "", hairline)
	}
	if a := profile.Achievements; a != nil {
		frame.add("")
		achievements(frame, a, lane, inner, p)
		frame.add("", hairline)
	}
}

func (m AppModel) activityGraph(frame *profileFrame, activity *api.Activity, lane string, width int, p palette) {
	pad := "  "
	total, most := 0, 0
	for _, b := range activity.Buckets {
		total += b.Count
		most = max(most, b.Count)
	}
	window := activityWindows[activity.Scale]
	if window == "" {
		window = "period"
	}
	frame.add(pad + split(text(p.ink).Bold(true).Render("Commit activity"), text(p.muted).Render(plural(total, "commit", "commits")+" in the last "+window), width))
	frame.add("")
	cell := func(count int, value string) string {
		return text(activityColor(activityLevel(count, most), lane, p)).Render(value)
	}
	if activity.Scale == "year" {
		// The web lays a year out as weeks of seven days, starting on a Sunday.
		weeks := (len(activity.Buckets) + 6) / 7
		first := max(0, weeks-width)
		for day := range 7 {
			row := ""
			for week := first; week < weeks; week++ {
				if i := week*7 + day; i < len(activity.Buckets) {
					row += cell(activity.Buckets[i].Count, "■")
				}
			}
			frame.add(pad + row)
		}
	} else {
		// Bars share the full width like the web. Without room for a gap column, a half block keeps them apart.
		n := len(activity.Buckets)
		gap := 1
		if width < 2*n-1 {
			gap = 0
		}
		room := max(n, width-(n-1)*gap)
		bars, labels, at := "", "", 0
		for i, b := range activity.Buckets {
			if i > 0 {
				bars += strings.Repeat(" ", gap)
				at += gap
			}
			column := (i+1)*room/n - i*room/n
			bar := strings.Repeat("█", column)
			if gap == 0 {
				bar = strings.Repeat("█", column-1) + "▌"
			}
			bars += cell(b.Count, bar)
			label := activityLabel(activity.Scale, b.Date)
			if label != "" && at >= lipgloss.Width(labels) && at+len(label) <= width {
				labels += strings.Repeat(" ", at-lipgloss.Width(labels)) + label
			}
			at += column
		}
		frame.add(pad+bars, pad+bars, pad+text(p.muted).Render(labels))
	}
	legend := text(p.muted).Render("Less ")
	for level := range activityMix {
		legend += text(activityColor(level, lane, p)).Render("■")
	}
	legend += text(p.muted).Render(" More")
	frame.add(pad + strings.Repeat(" ", max(0, width-lipgloss.Width(legend))) + legend)
}

// activityLabel names a bucket the way the web axis does: every six hours, every day, or each Monday.
func activityLabel(scale, date string) string {
	at, err := time.Parse(time.RFC3339, date)
	if err != nil {
		if at, err = time.Parse(time.DateOnly, date); err != nil {
			return ""
		}
	} else {
		at = at.In(time.Local)
	}
	switch scale {
	case "day":
		if at.Hour()%6 == 0 {
			return at.Format("15:04")
		}
	case "week":
		return at.Format("Mon 2")
	case "month":
		if at.Weekday() == time.Monday {
			return at.Format("Jan 2")
		}
	}
	return ""
}

func achievements(frame *profileFrame, a *api.Achievements, lane string, width int, p palette) {
	pad := "  "
	unlocked := 0
	var shelf []string
	for _, badge := range a.Badges {
		if badge.Unlocked {
			unlocked++
			shelf = append(shelf, chip(safeText(badge.Name), p.ink, p))
		}
	}
	title := text(lane).Render("◎ ") + text(p.ink).Bold(true).Render(fmt.Sprintf("Lv. %d · %s", a.Level, safeText(a.Rank))) + text(p.muted).Render(" · "+formatCount(a.XP)+" XP")
	frame.add(pad + split(title, text(p.muted).Render(fmt.Sprintf("%d / %d unlocked", unlocked, len(a.Badges))), width))
	progress, remaining := formatCount(a.XP)+" XP", "max level"
	if a.NextLevelXP != nil {
		progress = formatCount(a.XP) + " / " + formatCount(*a.NextLevelXP) + " XP"
		remaining = ""
		for _, rank := range a.Ranks {
			if !rank.Achieved && rank.MinXP > a.XP {
				remaining = formatCount(rank.MinXP-a.XP) + " XP to " + safeText(rank.Rank)
				break
			}
		}
	}
	frame.add(pad + split(text(p.muted).Render(progress), text(p.muted).Render(remaining), width))
	filled := int(math.Round(max(0, min(100, a.LevelProgress)) / 100 * float64(width)))
	frame.add(pad + text(lane).Render(strings.Repeat("━", filled)) + text(p.hairline).Render(strings.Repeat("━", width-filled)))
	if len(shelf) == 0 {
		return
	}
	row, shown := "", 0
	for i, badge := range shelf {
		more := ""
		if rest := len(shelf) - i - 1; rest > 0 {
			more = fmt.Sprintf("  +%d more", rest)
		}
		if lipgloss.Width(row)+2+lipgloss.Width(badge)+len(more) > width && shown > 0 {
			break
		}
		if row != "" {
			row += "  "
		}
		row += badge
		shown++
	}
	if shown < len(shelf) {
		row += text(p.muted).Render(fmt.Sprintf("  +%d more", len(shelf)-shown))
	}
	frame.add("", pad+row)
}

func (m AppModel) profileTabs(frame *profileFrame, tabs [2]string, active, width int, p palette) {
	var labels, underline strings.Builder
	for i, name := range tabs {
		cellWidth := width / 2
		if i == 1 {
			cellWidth = width - width/2
		}
		label := text(p.muted).Render(name)
		line := strings.Repeat(" ", cellWidth)
		if i == active {
			label = text(p.ink).Bold(true).Render(name)
			mark := min(len(name)+2, cellWidth)
			pad := (cellWidth - mark) / 2
			line = strings.Repeat(" ", pad) + text(p.blue).Render(strings.Repeat("━", mark)) + strings.Repeat(" ", cellWidth-pad-mark)
		}
		frame.actions = append(frame.actions, hitArea{uiRect{i * (width / 2), len(frame.lines), cellWidth, 2}, strconv.Itoa(i + 1), 0})
		labels.WriteString(lipgloss.NewStyle().Width(cellWidth).Align(lipgloss.Center).Render(label))
		underline.WriteString(line)
	}
	frame.add(labels.String(), underline.String(), text(p.hairline).Render(strings.Repeat("─", width)))
}

func (m AppModel) peopleRows(frame *profileFrame, s *profileState, width int, p palette) {
	l := s.lists[s.tab]
	gutter := 4
	if m.images.Enabled {
		gutter = 7
	}
	inner := max(1, width-gutter-2)
	for i, person := range l.people {
		lane := laneColor(person.Lane, p)
		s.starts = append(s.starts, len(frame.lines))
		name := text(p.ink).Bold(true).Render(safeText(person.Name))
		if person.Name == "" {
			name = text(p.ink).Bold(true).Render(safeText(person.Handle))
		}
		identity := name + " " + text(p.muted).Render("@"+safeText(person.Handle))
		if person.FollowsYou != nil && *person.FollowsYou {
			identity += "  " + chip("Follows you", p.muted, p)
		}
		rows := []string{identity}
		if person.Bio != "" {
			rows = append(rows, text(p.muted).Render(fit(strings.ReplaceAll(remoteBody(person.Bio), "\n", " "), inner)))
		}
		if i == l.selected {
			actions := []string{text(p.ink).Render("Profile") + " " + keyChip("enter", p)}
			if person.IsFollowed != nil && !strings.EqualFold(person.Handle, m.account.user.Handle) {
				actions = append(actions, text(p.ink).Render(followLabel(*person.IsFollowed))+" "+keyChip("f", p))
			}
			x := gutter
			for n, action := range actions {
				frame.actions = append(frame.actions, hitArea{uiRect{x, len(frame.lines) + len(rows), lipgloss.Width(action), 1}, []string{"enter", "f"}[n], i})
				x += lipgloss.Width(action) + 2
			}
			rows = append(rows, strings.Join(actions, "  "))
		}
		rows = append(rows, "")
		for row, body := range rows {
			rail := strings.Repeat(" ", gutter)
			if row == 0 || (row == 1 && m.images.Enabled) {
				rail = "  " + text(lane).Render("●") + " "
				if m.images.Enabled {
					rail = "  " + m.images.Row(m.avatarRequest(person.AvatarURL, lane, 4, 2), row) + " "
					if lipgloss.Width(rail) != gutter {
						rail = "  " + text(lane).Render("●") + strings.Repeat(" ", gutter-3)
					}
				}
			}
			line := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(rail + fit(body, inner))
			if i == l.selected {
				line = withBackground(text(p.blue).Render("▌")+ansi.Cut(line, 1, width), p.wash)
			}
			frame.add(line)
		}
		s.ends = append(s.ends, len(frame.lines))
		frame.add(text(p.hairline).Render(strings.Repeat("─", width)))
	}
}

func (m AppModel) profileAvatars(s *profileState, p palette) []graphics.Request {
	if !s.people {
		u := s.profile.User
		if !s.loaded || s.viewport.YOffset() > 6 {
			return nil
		}
		return []graphics.Request{m.avatarRequest(u.AvatarURL, laneColor(u.Lane, p), 8, 4)}
	}
	var requests []graphics.Request
	view := s.viewport
	for i, person := range s.lists[s.tab].people {
		if i < len(s.starts) && s.ends[i] > view.YOffset() && s.starts[i] < view.YOffset()+view.Height() {
			requests = append(requests, m.avatarRequest(person.AvatarURL, laneColor(person.Lane, p), 4, 2))
		}
	}
	return requests
}

func (m *AppModel) revealProfileSelection(s *profileState) {
	selected := s.lists[s.tab].selected
	if selected >= len(s.starts) {
		return
	}
	start, end := s.starts[selected], s.ends[selected]
	if start < s.viewport.YOffset() || end-start >= s.viewport.Height() {
		s.viewport.SetYOffset(start)
	} else if end > s.viewport.YOffset()+s.viewport.Height() {
		s.viewport.SetYOffset(end - s.viewport.Height())
	}
}

func (m *AppModel) sizeProfiles() {
	_, center, _ := m.columns()
	for i := range m.profiles {
		s := &m.profiles[i]
		offset := s.viewport.YOffset()
		s.viewport.SetWidth(center)
		s.viewport.SetHeight(max(1, m.mainHeight()-threadHeaderHeight))
		m.renderProfile(s)
		s.viewport.SetYOffset(offset)
	}
}

func (m AppModel) profileColumn(width int, p palette) string {
	s := m.profiles[len(m.profiles)-1]
	back := keyChip("esc", p) + text(p.muted).Render(" Back")
	name := s.profile.User.Name
	if name == "" {
		name = s.handle
	}
	title := "  " + text(p.ink).Bold(true).Render(safeText(name))
	switch {
	case s.people:
		title += text(p.muted).Render(" @" + safeText(s.handle))
	case s.profile.User.Commits != nil:
		title += text(p.muted).Render(" " + plural(*s.profile.User.Commits, "commit", "commits"))
	}
	header := fit(title, max(1, width-lipgloss.Width(back)-2))
	header += strings.Repeat(" ", max(1, width-lipgloss.Width(header)-lipgloss.Width(back)-1)) + back
	content := "\n" + header + "\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n" + s.viewport.View()
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) profileStatus() string {
	s := m.profiles[len(m.profiles)-1]
	l := s.lists[s.tab]
	if m.follow.handle != "" {
		return "Updating follow..."
	}
	if s.status != "" {
		return s.status
	}
	if l.status != "" {
		return l.status
	}
	if l.loading {
		return "Reading from /dev/null..."
	}
	noun := [2]string{"commits", "replies"}[s.tab]
	if s.people {
		noun = "people"
	}
	if s.count() == 0 {
		return profileEmpty(&s)
	}
	ending := "end of list"
	if l.next != "" {
		ending = "more available"
	}
	return fmt.Sprintf("%d / %d %s  ·  %s", l.selected+1, s.count(), noun, ending)
}

func (m AppModel) profileBindings(bind func(string, string) key.Binding) appKeys {
	s := m.profiles[len(m.profiles)-1]
	if s.people {
		short := []key.Binding{bind("j/k", "move"), bind("enter", "open profile"), bind("f", "follow / unfollow"), bind("tab", "followers / following"), bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit")}
		all := append(append([]key.Binding(nil), short...), bind("1 / 2", "followers / following"), bind("n", "more people"), bind("ctrl+r", "reload list"), bind("home / G / end", "first / last"), bind("pgup/pgdown", "move five entries"), bind("c", "compose"), bind("f2", m.account.nextTheme()+" theme"), bind("click", "select, open, or follow"))
		return appKeys{short, all}
	}
	short := []key.Binding{bind("j/k", "move"), bind("tab", "commits / replies"), bind("f", "follow / unfollow"), bind("enter", "open thread"), bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit")}
	all := append(append([]key.Binding(nil), short...), bind("1 / 2", "commits / replies"), bind("3 / 4", "followers / following"), bind("u", "author profile"), bind("r", "reply"), bind("q", "quote"), bind("v", "view images"), bind("D", "delete own commit"), bind("y", "copy link"), bind("n", "older commits"), bind("ctrl+r", "reload profile"), bind("home / G / end", "first / last"), bind("pgup/pgdown", "move five entries"), bind("c", "compose"), bind("f2", m.account.nextTheme()+" theme"), bind("click", "select, open, act, or view image"))
	all = append(all, m.engagementBindings()...)
	return appKeys{short, all}
}

func (m AppModel) profileMouse(x, y int) (AppModel, tea.Cmd) {
	s := m.topProfile()
	if y == 1 && x >= m.threadBackX() {
		return m.mouseKey("esc")
	}
	if y < threadHeaderHeight || y >= m.mainHeight() {
		return m, nil
	}
	row := y - threadHeaderHeight + s.viewport.YOffset()
	l := &s.lists[s.tab]
	for _, area := range s.actions {
		if area.contains(x, row) {
			return m.mouseKey(area.action)
		}
	}
	for _, picture := range s.pictures {
		if picture.contains(x, row) {
			cmd := m.selectProfileEntry(picture.entry)
			return m, tea.Batch(cmd, m.openViewer(s.entries()[picture.entry], picture.image))
		}
	}
	for i, start := range s.starts {
		if row < start || row >= s.ends[i] {
			continue
		}
		if i == l.selected {
			return m.mouseKey("enter")
		}
		cmd := m.selectProfileEntry(i)
		return m, cmd
	}
	return m, nil
}
