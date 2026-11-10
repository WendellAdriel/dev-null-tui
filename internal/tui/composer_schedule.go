package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// schedulePicker is the composer's Submit at picker: a day within the server's window, an hour, and a minute in
// steps of five, all in local time.
type schedulePicker struct {
	day, hour, minute int
	field             int
	status            string
}

type scheduleResult struct {
	session, generation int
	at                  time.Time
	scheduled           api.ScheduledCommit
	err                 error
}

// at is the picked time in now's zone, and whether it exists, since a daylight saving jump skips some.
func (s schedulePicker) at(now time.Time) (time.Time, bool) {
	zone := now.Location()
	at := time.Date(now.Year(), now.Month(), now.Day()+s.day, s.hour, s.minute, 0, 0, zone)
	return at, at.Hour() == s.hour && at.Minute() == s.minute
}

// newSchedulePicker starts at the chosen time, or like the web at the next full hour today, else 09:00 tomorrow.
func newSchedulePicker(chosen *time.Time, now time.Time) *schedulePicker {
	zone := now.Location()
	if chosen != nil {
		at := chosen.In(zone)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
		day := int(time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, zone).Sub(today).Hours()/24 + 0.5)
		return &schedulePicker{day: day, hour: at.Hour(), minute: at.Minute()}
	}
	if now.Hour() < 23 {
		return &schedulePicker{hour: now.Hour() + 1}
	}
	return &schedulePicker{day: 1, hour: 9}
}

func (m *AppModel) openSchedulePicker() tea.Cmd {
	c := &m.composer
	if c.previewing || c.sending || c.owner != m.account.user.ID || m.commitCalls.schedule == nil {
		return nil
	}
	if c.mode != "" {
		c.status = "Replies and quotes can't be scheduled."
		return nil
	}
	c.schedule = newSchedulePicker(c.scheduleAt, time.Now())
	c.input.Blur()
	return nil
}

func (m *AppModel) scheduleKey(msg tea.KeyPressMsg) tea.Cmd {
	c := &m.composer
	s := c.schedule
	limits := [3]int{api.MaxScheduleDays, 23, 55}
	steps := [3]int{1, 1, 5}
	value := [3]*int{&s.day, &s.hour, &s.minute}[s.field]
	s.status = ""
	switch msg.String() {
	case "esc":
		c.schedule = nil
		return m.focusComposer()
	case "tab", "right", "l":
		s.field = (s.field + 1) % 3
	case "shift+tab", "left", "h":
		s.field = (s.field + 2) % 3
	case "up", "k", "+":
		*value = min(limits[s.field], *value+steps[s.field])
	case "down", "j", "-":
		*value = max(0, *value-steps[s.field])
	case "ctrl+d":
		c.scheduleAt, c.schedule = nil, nil
		return m.focusComposer()
	case "enter":
		now := time.Now()
		at, exists := s.at(now)
		if !exists {
			s.status = "This time does not exist in your timezone. Choose another time."
			return nil
		}
		if err := api.ValidScheduleTime(at, now); err != nil {
			s.status = sentence(err.Error())
			return nil
		}
		c.scheduleAt, c.schedule = &at, nil
		return m.focusComposer()
	}
	return nil
}

func zoneLabel(at time.Time) string {
	name, _ := at.Zone()
	if zone := time.Local.String(); zone != "Local" && zone != name {
		return zone + " (" + name + ")"
	}
	return name
}

// scheduleView draws the schedule picker. Its hit areas are the hint chips.
func (m AppModel) scheduleView(width, height int, p palette) (string, []hitArea) {
	s := m.composer.schedule
	now := time.Now()
	at, _ := s.at(now)
	rows := []string{text(p.ink).Bold(true).Render("Submit at"), ""}
	fields := []string{at.Format("Mon, Jan 2"), fmt.Sprintf("%02d", s.hour), fmt.Sprintf("%02d", s.minute)}
	var cells []string
	for i, value := range fields {
		cell := " " + value + " "
		if i == s.field {
			cell = text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render("‹" + value + "›")
		} else {
			cell = text(p.muted).Render(cell)
		}
		cells = append(cells, cell)
	}
	rows = append(rows, cells[0]+"   "+cells[1]+text(p.muted).Render(":")+cells[2])
	// The status and keys come before the longer help, so a short terminal clips the help instead.
	if s.status != "" {
		rows = append(rows, text(p.warning).Render(fit(s.status, width)))
	}
	hints := []statusPart{hintChip("Field", "←/→", "right"), hintChip("Earlier", "↓", "down"), hintChip("Later", "↑", "up"), statusChip("Set", "enter")}
	if m.composer.scheduleAt != nil {
		hints = append(hints, statusChip("Don't schedule", "ctrl+d"))
	}
	hint, areas := hintRow(append(hints, closeChip("Back")), width, p)
	for i := range areas {
		areas[i].y = len(rows)
	}
	rows = append(rows, hint, "")
	help := zoneLabel(at) + ". Within " + fmt.Sprint(api.MaxScheduleDays) + " days. Private until submitted, then submitted for review, not published automatically."
	for _, line := range strings.Split(ansi.Wrap(help, width, ""), "\n") {
		rows = append(rows, text(p.muted).Render(line))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n")), areas
}

// scheduleNotice is the composer's line about a chosen time, above its status.
func (m AppModel) scheduleNotice() string {
	at := m.composer.scheduleAt
	if at == nil {
		return ""
	}
	return "Scheduled for " + scheduleTime(*at) + " " + zoneLabel(at.In(time.Local)) + ". Ctrl+T to change."
}

func (m *AppModel) startSchedule(draft api.NewCommit) tea.Cmd {
	c := &m.composer
	at := *c.scheduleAt
	if err := api.ValidScheduleTime(at, time.Now()); err != nil {
		c.sending = false
		c.status = sentence(err.Error()) + " Press Ctrl+T to pick another time."
		return tea.Batch(m.focusComposer(), m.notify("error", "Cannot schedule", c.status))
	}
	session, generation, schedule := m.session, c.generation, m.commitCalls.schedule
	timeout := 30 * time.Second
	if len(draft.Images) > 0 {
		timeout = auth.UploadTimeout
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, timeout)
	c.cancel = cancel
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		scheduled, err := schedule(ctx, draft, at)
		return scheduleResult{session: session, generation: generation, at: at, scheduled: scheduled, err: err}
	})
}

func (m *AppModel) acceptSchedule(result scheduleResult) tea.Cmd {
	c := &m.composer
	if result.session != m.session || result.generation != c.generation || !c.sending {
		return nil
	}
	if result.err != nil {
		return m.acceptSubmission(submissionResult{session: result.session, generation: result.generation, err: result.err})
	}
	c.sending, c.cancel = false, nil
	m.clearComposer()
	m.notice = "Scheduled for " + scheduleTime(result.at) + ". g t lists your scheduled commits."
	if m.scheduled != nil {
		return tea.Batch(m.fetchScheduled(false), m.notify("success", "Commit scheduled", m.notice))
	}
	return m.notify("success", "Commit scheduled", m.notice)
}
