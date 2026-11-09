package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// scheduledList is the private Scheduled commits screen, drawn in the Starred list screen's place.
type scheduledList struct {
	items           []api.ScheduledCommit
	page            int
	more, loading   bool
	loaded          bool
	status          string
	selected        int
	generation      int
	busy            string
	viewport        viewport.Model
	starts, ends    []int
	actions         []hitArea
	cancelRequested context.CancelFunc
}

type scheduledResult struct {
	session, generation int
	page                api.ScheduledPage
	more                bool
	err                 error
}

type scheduledActionResult struct {
	session   int
	id, kind  string
	scheduled api.ScheduledCommit
	err       error
}

func (m AppModel) openScheduled() (AppModel, tea.Cmd) {
	if m.commitCalls.scheduled == nil {
		return m, nil
	}
	next, cmd := m.openList("")
	if !next.starredOpen {
		return next, cmd
	}
	if next.starred.cancel != nil {
		next.starred.cancel()
		next.starred.cancel, next.starred.loading = nil, false
	}
	next.starred.generation++
	next.scheduled = &scheduledList{page: 1, viewport: viewport.New()}
	next.reflow()
	return next, next.fetchScheduled(false)
}

func (m *AppModel) fetchScheduled(more bool) tea.Cmd {
	s := m.scheduled
	if s == nil || s.loading || (more && !s.more) {
		return nil
	}
	page := 1
	if more {
		page = s.page + 1
	}
	s.generation++
	s.loading, s.status = true, ""
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	s.cancelRequested = cancel
	session, generation, load := m.session, s.generation, m.commitCalls.scheduled
	m.renderScheduled()
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		result, err := load(ctx, page)
		return scheduledResult{session: session, generation: generation, page: result, more: more, err: err}
	})
}

func (m *AppModel) acceptScheduled(msg scheduledResult) tea.Cmd {
	s := m.scheduled
	if s == nil || msg.session != m.session || msg.generation != s.generation {
		return nil
	}
	s.loading, s.cancelRequested = false, nil
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		s.status = "Couldn't load scheduled commits: " + safeText(msg.err.Error()) + ". Ctrl+R to retry."
		m.renderScheduled()
		return nil
	}
	if !msg.more {
		s.items, s.selected = nil, 0
	}
	more := msg.page.More()
	for _, item := range msg.page.Data {
		// Committed rows sort after waiting ones, so the first one ends the list.
		if item.Status != "scheduled" {
			more = false
			continue
		}
		if !slices.ContainsFunc(s.items, func(have api.ScheduledCommit) bool { return have.ID == item.ID }) {
			s.items = append(s.items, item)
		}
	}
	s.page, s.more, s.loaded = max(1, msg.page.Meta.CurrentPage), more, true
	m.renderScheduled()
	if !msg.more {
		s.viewport.GotoTop()
	}
	return nil
}

func (m AppModel) selectedScheduled() (api.ScheduledCommit, bool) {
	s := m.scheduled
	if s == nil || s.selected >= len(s.items) {
		return api.ScheduledCommit{}, false
	}
	return s.items[s.selected], true
}

func (m AppModel) scheduledKey(pressed string) (AppModel, tea.Cmd) {
	s := m.scheduled
	move := func(index int) {
		s.selected = max(0, min(len(s.items)-1, index))
		m.renderScheduled()
		m.revealScheduledSelection()
	}
	switch pressed {
	case "j", "down":
		move(s.selected + 1)
		if s.selected >= len(s.items)-2 {
			return m, m.fetchScheduled(true)
		}
	case "k", "up":
		move(s.selected - 1)
	case "home":
		move(0)
	case "G", "end":
		move(len(s.items) - 1)
	case "pgdown", "ctrl+d":
		move(s.selected + 5)
	case "pgup", "ctrl+u":
		move(s.selected - 5)
	case "n":
		return m, m.fetchScheduled(true)
	case "ctrl+r":
		return m, m.fetchScheduled(false)
	case "enter":
		if item, ok := m.selectedScheduled(); ok {
			return m, m.submitScheduledNow(item.ID)
		}
	case "D":
		if item, ok := m.selectedScheduled(); ok && item.Status == "scheduled" && s.busy == "" {
			m.ask = &ask{kind: "cancel-scheduled", title: "/dev/null · CANCEL", question: "Cancel this scheduled commit?", detail: "It won't be submitted. This can't be undone.", confirm: "Cancel commit", border: m.account.colors().danger, id: item.ID}
		}
	case "esc":
		return m.switchLog(m.active)
	}
	return m, nil
}

func (m *AppModel) scheduledAction(id, kind string) tea.Cmd {
	s := m.scheduled
	if s == nil || s.busy != "" {
		return nil
	}
	i := -1
	for j, item := range s.items {
		if item.ID == id {
			i = j
		}
	}
	if i < 0 || s.items[i].Status != "scheduled" {
		return nil
	}
	s.busy = id
	m.renderScheduled()
	session, calls, life := m.session, m.commitCalls, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		result := scheduledActionResult{session: session, id: id, kind: kind}
		if kind == "cancel" {
			result.err = calls.cancelLater(ctx, id)
			return result
		}
		result.scheduled, result.err = calls.submitNow(ctx, id)
		return result
	})
}

func (m *AppModel) submitScheduledNow(id string) tea.Cmd { return m.scheduledAction(id, "submit") }

func (m *AppModel) cancelScheduled(id string) tea.Cmd { return m.scheduledAction(id, "cancel") }

func (m *AppModel) acceptScheduledAction(msg scheduledActionResult) tea.Cmd {
	s := m.scheduled
	if msg.session != m.session || s == nil || s.busy != msg.id {
		return nil
	}
	s.busy = ""
	defer m.renderScheduled()
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		var failure *auth.APIError
		if errors.As(msg.err, &failure) && failure.Status == http.StatusConflict {
			return tea.Batch(m.fetchScheduled(false), m.notify("warning", "Already handled", "This commit was already submitted or canceled. Refreshing the list."))
		}
		if failure != nil && len(failure.BodyErrors)+len(failure.FieldErrors) > 0 {
			return m.notify("error", "Couldn't submit it", safeText(strings.Join(append(failure.BodyErrors, failure.FieldErrors...), " ")))
		}
		return m.notify("error", "Action failed", safeText(msg.err.Error())+". Ctrl+R to check its status.")
	}
	for i, item := range s.items {
		if item.ID != msg.id {
			continue
		}
		// The list pages by number, so an action shifts later pages; reload them from the first rather than skip a row.
		var reload tea.Cmd
		if s.more {
			reload = m.fetchScheduled(false)
		}
		s.items = append(s.items[:i], s.items[i+1:]...)
		s.selected = max(0, min(s.selected, len(s.items)-1))
		if msg.kind == "cancel" {
			return tea.Batch(reload, m.notify("success", "Canceled", "The scheduled commit won't be submitted."))
		}
		return tea.Batch(reload, m.notify("success", "Submitted for review", "It goes through the usual review process."))
	}
	return nil
}

// scheduledStatus is the web's status label: Waiting for submission once due, else the commit's moderation status.
func scheduledStatus(item api.ScheduledCommit, now time.Time) (string, string) {
	switch item.Status {
	case "scheduled":
		if !item.ScheduledAt.After(now) {
			return "Waiting for submission", "warning"
		}
		return "Scheduled", "blue"
	case "pending":
		return "Submitted for review", "muted"
	case "approved":
		return "Approved", "success"
	case "rejected":
		return "Rejected", "danger"
	}
	return "Deleted", "muted"
}

func scheduleTime(at time.Time) string { return at.In(time.Local).Format("Mon, Jan 2 · 15:04") }

func (m *AppModel) renderScheduled() {
	s := m.scheduled
	if s == nil {
		return
	}
	p := m.account.colors()
	width := max(1, s.viewport.Width())
	inner := max(1, width-4)
	ink := map[string]string{"blue": p.blue, "warning": p.warning, "muted": p.muted, "success": p.success, "danger": p.danger}
	var lines []string
	s.starts, s.ends, s.actions = nil, nil, nil
	for i, item := range s.items {
		s.starts = append(s.starts, len(lines))
		label, color := scheduledStatus(item, m.now)
		meta := text(ink[color]).Bold(true).Render(label) + text(p.muted).Render(" · "+scheduleTime(item.ScheduledAt))
		if item.Author != nil && item.Author.Organization() {
			meta += text(p.muted).Render(" · As @" + safeText(item.Author.Handle))
		}
		block := []string{"  " + fit(meta, inner)}
		if item.Body != nil {
			body := bodyLines(api.Commit{ID: "scheduled-" + item.ID, Body: *item.Body, Segments: item.Segments}, inner, p, m.statsStyle())
			for _, line := range body {
				block = append(block, "  "+line)
			}
		}
		for _, option := range item.PollOptions {
			block = append(block, "  "+text(p.muted).Render(fit("◯ "+safeText(option), inner)))
		}
		if item.ImageCount > 0 {
			block = append(block, "  "+text(p.muted).Render(plural(item.ImageCount, "image", "images")))
		}
		help := ""
		switch {
		case item.Status == "scheduled" && !item.ScheduledAt.After(m.now):
			help = "Waiting for the scheduler. If submission fails, it is retried automatically."
		case item.Status == "scheduled":
			help = "Private until submitted. You can commit it now or cancel it before submission."
		}
		if help != "" {
			for _, line := range strings.Split(lipgloss.NewStyle().Width(inner).Render(help), "\n") {
				block = append(block, "  "+text(p.muted).Render(line))
			}
		}
		if item.Status == "scheduled" && i == s.selected {
			now := "Commit now"
			if s.busy == item.ID {
				now = "Working…"
			}
			chips, areas := actionChips([]commitAction{{now, "enter"}, {"Cancel", "D"}}, inner, p)
			for _, area := range areas {
				area.x += 2
				area.y += len(lines) + len(block)
				s.actions = append(s.actions, area)
			}
			for _, row := range chips {
				block = append(block, "  "+row)
			}
		}
		if i == s.selected {
			for j := range block {
				block[j] = withBackground(lipgloss.NewStyle().Width(width).Render(block[j]), p.soft)
			}
		}
		lines = append(lines, block...)
		s.ends = append(s.ends, len(lines))
		lines = append(lines, text(p.hairline).Render(strings.Repeat("─", width)))
	}
	add := func(value string) { lines = append(lines, "", "  "+text(p.muted).Render(fit(value, inner))) }
	switch {
	case s.status != "":
		add(s.status)
	case s.loading:
		add("Reading scheduled commits from /dev/null...")
	case s.loaded && len(s.items) == 0:
		add("No scheduled commits")
		add("Press Ctrl+T in the composer to submit later.")
	case s.more:
		lines = append(lines, "", lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(actionChip("More", "n", p)), "")
	}
	s.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *AppModel) revealScheduledSelection() {
	s := m.scheduled
	if s == nil || s.selected >= len(s.starts) {
		return
	}
	start, end := s.starts[s.selected], s.ends[s.selected]
	if start < s.viewport.YOffset() || end-start >= s.viewport.Height() {
		s.viewport.SetYOffset(start)
	} else if end > s.viewport.YOffset()+s.viewport.Height() {
		s.viewport.SetYOffset(end - s.viewport.Height())
	}
}

func (m AppModel) scheduledColumn(width int, p palette) string {
	line := text(p.hairline).Render(strings.Repeat("─", width))
	header := split(text(p.ink).Bold(true).Render("Scheduled commits"), text(p.muted).Render(m.icons.lock+" Private"), width-4)
	zone, _ := time.Now().In(time.Local).Zone()
	about := "Only you can see these. Times in " + time.Local.String() + " (" + zone + ")."
	if time.Local.String() == "Local" {
		about = "Only you can see these. Times in " + zone + "."
	}
	content := "\n  " + header + "\n" + line + "\n  " + text(p.muted).Render(fit(about, width-4)) + "\n\n" + line + "\n" + m.scheduled.viewport.View()
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) scheduledMouse(x, y int) (AppModel, tea.Cmd) {
	s := m.scheduled
	if y < starredHeaderHeight || y >= m.mainHeight() {
		return m, nil
	}
	row := y - starredHeaderHeight + s.viewport.YOffset()
	for _, area := range s.actions {
		if area.contains(x, row) {
			return m.scheduledKey(area.action)
		}
	}
	for i, start := range s.starts {
		if row >= start && row < s.ends[i] {
			s.selected = i
			m.renderScheduled()
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) scheduledBindings(bind func(string, string) key.Binding) appKeys {
	short := []key.Binding{bind("j/k", "move"), bind("enter", "commit now"), bind("D", "cancel"), bind("ctrl+r", "refresh"), bind("esc", "back to log"), bind("?", "help"), bind("ctrl+c", "quit")}
	all := append(append([]key.Binding(nil), short...), bind("n", "next page"), bind("home / G / end", "first / last"), bind("g t", "reopen Scheduled"), bind("g l / g e", "Log / Trending"), bind("g s", "Starred"), bind("c", "compose (ctrl+t schedules)"), bind("a", "account"))
	return appKeys{short, all}
}

func (m AppModel) scheduledStatusLine() string {
	s := m.scheduled
	if s.status != "" {
		return s.status
	}
	if s.loading {
		return "Reading scheduled commits from /dev/null..."
	}
	if len(s.items) == 0 {
		return "No scheduled commits."
	}
	return fmt.Sprintf("%d / %d scheduled commits", s.selected+1, len(s.items))
}
