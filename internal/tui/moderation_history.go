package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type moderationResult struct {
	session, generation, read int
	history                   api.ModerationHistory
	err                       error
}

// fetchModerationHistory reads your timeouts and bans, which only your own profile shows.
func (m AppModel) fetchModerationHistory(s *profileState) tea.Cmd {
	if m.loadModerationHistory == nil || !m.ownProfile(s) {
		return nil
	}
	// Ctrl+R and a new notification can overlap, so only the latest read counts.
	s.historyRead++
	session, generation, read, load, base := m.session, s.generation, s.historyRead, m.loadModerationHistory, s.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		history, err := load(ctx)
		return moderationResult{session: session, generation: generation, read: read, history: history, err: err}
	})
}

// acceptModerationHistory keeps the last history read when a later read fails, so a blip doesn't hide the line.
func (m *AppModel) acceptModerationHistory(msg moderationResult) tea.Cmd {
	s := m.profileFor(msg.session, msg.generation)
	if msg.err != nil {
		m.revoked(msg.err)
	}
	if s == nil || msg.err != nil || msg.read != s.historyRead {
		return nil
	}
	s.history = &msg.history
	m.renderProfile(s)
	return nil
}

// refreshModerationHistory rereads the history on your open profiles, as a new timeout adds to it.
func (m *AppModel) refreshModerationHistory() tea.Cmd {
	var reads []tea.Cmd
	for i := range m.profiles {
		reads = append(reads, m.fetchModerationHistory(&m.profiles[i]))
	}
	return tea.Batch(reads...)
}

func (m *AppModel) openModerationHistory() bool {
	s := m.topProfile()
	if s == nil || !m.ownProfile(s) || s.history == nil {
		return false
	}
	h := s.history
	m.pickerGeneration++
	d := &picker{kind: "moderation", title: "/dev/null · MODERATION HISTORY", intro: "This record of timeouts and bans is visible to you and site moderators.", generation: m.pickerGeneration}
	d.rows = append(d.rows, pickerRow{label: "Timeouts · " + formatCount(h.TimeoutsCount), heading: true})
	for _, t := range h.Timeouts {
		end := " · Ended "
		if t.EndsAt.After(m.now) {
			end = " · Ends "
		}
		detail := "Started " + historyTime(t.StartedAt) + end + historyTime(t.EndsAt)
		d.rows = append(d.rows, pickerRow{label: t.Reason, note: t.Duration, value: t.ID, detail: detail})
	}
	if len(h.Timeouts) == 0 {
		d.rows = append(d.rows, pickerRow{label: "No timeouts.", heading: true})
	}
	d.rows = append(d.rows, pickerRow{heading: true}, pickerRow{label: "Bans · " + formatCount(h.BansCount), heading: true})
	for _, b := range h.Bans {
		d.rows = append(d.rows, pickerRow{label: b.Reason, note: historyTime(b.BannedAt), value: b.ID})
	}
	if len(h.Bans) == 0 {
		d.rows = append(d.rows, pickerRow{label: "No bans.", heading: true})
	}
	d.first()
	if d.rows[d.selected].heading {
		// A clean record has nothing to select, so no heading should look selected.
		d.selected = len(d.rows)
	}
	m.picker = d
	return true
}

// historyTime has the year, since the history keeps every timeout and ban the account ever had.
func historyTime(at time.Time) string { return at.In(time.Local).Format("Mon, Jan 2, 2006 · 15:04") }
