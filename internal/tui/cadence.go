package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

const (
	// logPoll is how often a focused chronological log asks for new commits.
	logPoll = 15 * time.Second
	// awayPoll is every poll's pace while the terminal is unfocused, as a hidden web tab slows down.
	awayPoll = 30 * time.Second
	// arrivalAuthors is how many avatars the New commits notice shows, as the web's does.
	arrivalAuthors = 3
	// newestPages bounds how far back a check for new commits reads to reach the commits already shown.
	newestPages = 3
)

type logTick struct{ session, generation int }

// pollDelay is a poll's own pace while the terminal is focused, and awayPoll otherwise.
func (m AppModel) pollDelay(focused time.Duration) time.Duration {
	if m.focused {
		return focused
	}
	return awayPoll
}

// backgroundLoad reports the quiet checks for new commits, which yield to any load the reader asks for.
func backgroundLoad(mode string) bool { return mode == "count" || mode == "new" }

func (m *AppModel) scheduleLogCheck() tea.Cmd {
	m.logAway = !m.focused
	session, generation := m.session, m.logTicks
	return tea.Tick(m.pollDelay(logPoll), func(time.Time) tea.Msg { return logTick{session, generation} })
}

// acceptLogTick asks the open chronological log for new commits, then schedules the next check. An empty log reads
// its newest page instead, as the web asks for everything when it shows nothing.
func (m *AppModel) acceptLogTick(msg logTick) tea.Cmd {
	if msg.session != m.session || msg.generation != m.logTicks || !m.connected {
		return nil
	}
	var check tea.Cmd
	if f := m.feeds[m.active]; f.loaded && !m.accountOpen && !m.settings.signingOut() && !m.notificationsOpen && !m.starredOpen && !m.remotes.open && m.active != 2 {
		if len(f.commits) == 0 {
			check = m.load("new")
		} else {
			check = m.load("count")
		}
	}
	return tea.Batch(check, m.scheduleLogCheck())
}

// restartPolls drops the ticks scheduled at the away pace when the terminal gets focus back, so polling picks up its
// focused pace at once. Ticks already at the focused pace keep running, so quick focus changes never starve them.
func (m *AppModel) restartPolls() tea.Cmd {
	var commands []tea.Cmd
	if m.logAway {
		m.logTicks++
		commands = append(commands, m.scheduleLogCheck())
	}
	if m.badgeAway {
		m.badgeTicks++
		commands = append(commands, m.scheduleBadges())
	}
	if m.remotes.open && m.remotes.away {
		m.remotes.tickGeneration++
		m.remotes.ticking = false
		commands = append(commands, m.scheduleRemotes())
	}
	return tea.Batch(commands...)
}

// newestThrough reads older pages after the newest one until it reaches a commit already shown, at most newestPages
// in all, so the commits in between are placed too. It returns the pages joined.
func newestThrough(ctx context.Context, fetch func(context.Context, api.Log, string, string) (api.Page, error), log api.Log, page api.Page, known map[string]bool) (api.Page, error) {
	if len(known) == 0 {
		return page, nil
	}
	shown := func(c api.Commit) bool { return known[c.ID] }
	for range newestPages - 1 {
		if page.Next == "" || slices.ContainsFunc(page.Data, shown) {
			break
		}
		older, err := fetch(ctx, log, page.Next, "")
		if err != nil {
			return api.Page{}, err
		}
		page.Data, page.Next = append(page.Data, older.Data...), older.Next
	}
	return page, nil
}

// acceptNewest places the commits above the ones shown, keeping the reader's row as the web does. A reader at the top
// of a focused terminal sees them there. Anyone else gets the New commits notice, which leaves out their own commits.
// It returns how many commits it added.
func (m *AppModel) acceptNewest(index int, page api.Page) int {
	f := &m.feeds[index]
	data := m.safeCommits(page.Data)
	if len(f.commits) == 0 {
		f.commits, f.next, f.selected = uniqueCommits(data), page.Next, 0
		m.renderFeed(index)
		f.viewport.GotoTop()
		return len(f.commits)
	}
	overlap := slices.IndexFunc(data, func(c api.Commit) bool { return containsID(f.commits, c.ID) })
	if overlap < 0 {
		// The shown commits are further back than a check reads. The newest ones still go above them, and the
		// reader keeps their place.
		overlap = len(data)
	}
	arrivals := uniqueCommits(data[:overlap])
	if len(arrivals) == 0 {
		return 0
	}
	atTop := m.focused && f.selected == 0 && f.viewport.YOffset() == 0
	anchor, offset := f.commits[f.selected].ID, 0
	if f.selected < len(f.starts) {
		offset = f.viewport.YOffset() - f.starts[f.selected]
	}
	f.commits = uniqueCommits(append(slices.Clone(arrivals), f.commits...))
	f.selected = max(0, slices.IndexFunc(f.commits, func(c api.Commit) bool { return c.ID == anchor }))
	if atTop {
		f.selected = 0
	}
	m.renderFeed(index)
	switch {
	case f.selected == 0:
		f.viewport.GotoTop()
	case f.selected < len(f.starts):
		f.viewport.SetYOffset(max(0, f.starts[f.selected]+offset))
	}
	added := len(arrivals)
	if atTop {
		return added
	}
	news := slices.DeleteFunc(arrivals, m.ownCommit)
	f.arrivals = uniqueCommits(append(news, f.arrivals...))
	return added
}

func containsID(commits []api.Commit, id string) bool {
	return slices.ContainsFunc(commits, func(c api.Commit) bool { return c.ID == id })
}

// settleArrivals drops arrivals that left the log, then the whole New commits notice once the newest arrival is on
// screen in a focused terminal.
func (m *AppModel) settleArrivals() {
	f := &m.feeds[m.active]
	f.arrivals = slices.DeleteFunc(f.arrivals, func(c api.Commit) bool { return !containsID(f.commits, c.ID) })
	if len(f.arrivals) == 0 {
		f.arrivals = nil
		return
	}
	if !m.focused || !m.logOnTop() {
		return
	}
	i := slices.IndexFunc(f.commits, func(c api.Commit) bool { return c.ID == f.arrivals[0].ID })
	if i < len(f.starts) && f.starts[i] >= f.viewport.YOffset() && f.starts[i] < f.viewport.YOffset()+f.viewport.Height() {
		f.arrivals = nil
	}
}

// logOnTop reports the log as the screen being read, with nothing drawn over its column.
func (m AppModel) logOnTop() bool {
	return m.connected && !m.accountOpen && !m.settings.open && !m.notificationsOpen && !m.starredOpen && !m.remotes.open && !m.profileOnTop() && !m.threadOnTop() && m.manualLink == ""
}

// arrivalAvatars are the distinct authors the New commits notice draws, newest first.
func (m AppModel) arrivalAvatars() []*api.Author {
	if !m.logOnTop() {
		return nil
	}
	var authors []*api.Author
	for _, c := range m.feeds[m.active].arrivals {
		if len(authors) == arrivalAuthors {
			break
		}
		if !slices.ContainsFunc(authors, func(a *api.Author) bool { return a.ID == c.Author.ID && a.Handle == c.Author.Handle }) {
			authors = append(authors, c.Author)
		}
	}
	return authors
}

// arrivalsPill is the web's New commits button: an arrow, up to three author avatars, and the label, padded to three
// rows. It returns the pill and its left column within a feed of the given width.
func (m AppModel) arrivalsPill(width int, p palette) (string, int) {
	authors := m.arrivalAvatars()
	if len(authors) == 0 {
		return "", 0
	}
	fill := lipgloss.NewStyle().Background(lipgloss.Color(p.blue)).Foreground(lipgloss.Color(p.shell)).Bold(true)
	pill := fill.Render("   ↑  ")
	for _, author := range authors {
		avatar := m.images.Row(m.authorAvatar(author, p, 2, 1), 0)
		if avatar == "" {
			avatar = text(laneColor(author.Lane, p)).Render("● ")
		}
		pill += avatar + fill.Render(" ")
	}
	// Round avatars have transparent corners, so every cell, the avatars' too, keeps the pill's fill.
	pill = withBackground(pill+fill.Render(" New commits   "), p.blue)
	pad := fill.Render(strings.Repeat(" ", lipgloss.Width(pill)))
	return pad + "\n" + pill + "\n" + pad, max(0, (width-lipgloss.Width(pill))/2)
}
