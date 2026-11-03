package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// voteState follows one poll from choosing an option to the confirmation modal. Votes are final, so the modal
// starts on Cancel. pending is the commit whose vote is in flight.
type voteState struct {
	commitID, pending string
	option, focus     int
	confirm           bool
}

type voteResult struct {
	session int
	id      string
	commit  api.Commit
	err     error
}

type voteReloaded struct {
	session int
	commit  api.Commit
	err     error
}

func (m AppModel) canVote(c api.Commit) bool {
	poll := c.Poll
	return m.castVote != nil && m.vote.pending == "" && c.DeletedAt == nil && c.Author != nil && !m.ownCommit(c) &&
		(c.Status == "" || c.Status == "approved") && poll.Valid() && poll.ClosesAt != nil && !poll.Closed && !poll.HasVoted &&
		poll.ClosesAt.After(m.now)
}

// showsResults reports a poll drawn with result bars. Otherwise each option takes one row, which a click can pick.
func showsResults(poll *api.Poll) bool { return poll.TotalVotes != nil && poll.ClosesAt != nil }

// voteOn asks to confirm a vote for option on the selected commit, like o, moving to it, and Enter.
func (m *AppModel) voteOn(option int) bool {
	if !m.startVoting() {
		return false
	}
	m.vote.option, m.vote.confirm = option, true
	m.reflow()
	return true
}

func (m *AppModel) startVoting() bool {
	c, ok := m.focusedEngagementCommit()
	if !ok || !m.canVote(c) {
		return false
	}
	m.vote = voteState{commitID: c.ID}
	m.reflow()
	return true
}

// votingCommit is the poll being voted on, while it is still the selected commit.
func (m AppModel) votingCommit() (api.Commit, bool) {
	c, ok := m.focusedEngagementCommit()
	if !ok || c.ID != m.vote.commitID || !m.canVote(c) {
		return api.Commit{}, false
	}
	return c, true
}

func (m *AppModel) voteKey(pressed string) tea.Cmd {
	c, ok := m.votingCommit()
	if !ok {
		m.vote.commitID, m.vote.confirm = "", false
		m.reflow()
		return nil
	}
	v := &m.vote
	if v.confirm {
		switch pressed {
		case "tab", "shift+tab", "left", "right", "h", "l":
			v.focus = 1 - v.focus
		case "esc":
			v.confirm = false
		case "y":
			return m.sendVote(c)
		case "enter":
			if v.focus == 1 {
				return m.sendVote(c)
			}
			v.confirm = false
		}
		return nil
	}
	switch pressed {
	case "j", "down":
		v.option = min(v.option+1, len(c.Poll.Options)-1)
	case "k", "up":
		v.option = max(v.option-1, 0)
	case "enter", "space":
		v.confirm, v.focus = true, 0
	case "esc":
		v.commitID = ""
	}
	m.reflow()
	return nil
}

func (m *AppModel) sendVote(c api.Commit) tea.Cmd {
	option := c.Poll.Options[m.vote.option].ID
	m.vote = voteState{pending: c.ID}
	m.reflow()
	m.showToast("info", "Voting", "Waiting for the server to confirm.")
	session, cast, life := m.session, m.castVote, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		commit, err := cast(ctx, c.ID, option)
		return voteResult{session: session, id: c.ID, commit: commit, err: err}
	})
}

func (m *AppModel) acceptVote(msg voteResult) tea.Cmd {
	if !m.connected || msg.session != m.session || msg.id != m.vote.pending {
		return nil
	}
	m.vote.pending = ""
	if msg.err == nil {
		m.applyPoll(msg.commit)
		m.reflow()
		return m.notify("success", "Vote recorded", "Your vote is final.")
	}
	m.reflow()
	if m.revoked(msg.err) {
		return nil
	}
	var failure *auth.APIError
	if !errors.As(msg.err, &failure) || failure.Status >= 500 || failure.Status == http.StatusRequestTimeout {
		return tea.Batch(m.notify("warning", "Vote outcome unknown", "Reloading the poll. Your vote was not sent again."), m.reloadPoll(msg.id))
	}
	detail := "The poll is unchanged."
	switch {
	case failure.Status == http.StatusTooManyRequests:
		return m.notify("warning", "Vote not recorded", retryAfterText(failure.RetryAfter, detail))
	case failure.Status == http.StatusNotFound:
		detail = "This commit is no longer available."
	case len(failure.FieldErrors) > 0:
		detail = safeText(strings.Join(failure.FieldErrors, " "))
	}
	if failure.Status == http.StatusUnprocessableEntity {
		return tea.Batch(m.notify("error", "Vote not recorded", detail), m.reloadPoll(msg.id))
	}
	return m.notify("error", "Vote not recorded", detail)
}

func (m *AppModel) reloadPoll(id string) tea.Cmd {
	if m.loadThread == nil {
		return nil
	}
	session, load, life := m.session, m.loadThread, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		thread, err := load(ctx, id)
		return voteReloaded{session: session, commit: thread.Commit, err: err}
	})
}

func (m *AppModel) acceptPollReload(msg voteReloaded) {
	if !m.connected || msg.session != m.session || msg.err != nil || !msg.commit.Poll.Valid() {
		return
	}
	m.applyPoll(msg.commit)
	m.reflow()
}

// applyPoll replaces the poll on every loaded copy of the commit, including quotes and reply previews.
func (m *AppModel) applyPoll(receipt api.Commit) {
	var update func(*api.Commit)
	update = func(c *api.Commit) {
		if c == nil || c.DeletedAt != nil {
			return
		}
		if c.ID == receipt.ID {
			c.Poll = receipt.Poll
		}
		update(c.Quote)
		if c.ReplyPreview != nil {
			for i := range c.ReplyPreview.Data {
				update(&c.ReplyPreview.Data[i])
			}
		}
	}
	m.eachCommit(update)
}

// pollStatus matches the web: time left while open, and the final state once the clock runs out.
func pollStatus(poll *api.Poll, now time.Time) string {
	switch {
	case poll.ClosesAt == nil:
		return "Opens when approved"
	case poll.Closed || !poll.ClosesAt.After(now):
		if poll.TotalVotes == nil {
			return "Poll closed"
		}
		return "Final results"
	}
	minutes := int(math.Ceil(poll.ClosesAt.Sub(now).Minutes()))
	switch {
	case minutes >= 1440:
		return fmt.Sprintf("Closes in %dd", minutes/1440)
	case minutes >= 60:
		return fmt.Sprintf("Closes in %dh", minutes/60)
	}
	return fmt.Sprintf("Closes in %dm", max(minutes, 1))
}

// pollLines draws a poll below the body. focus is the highlighted option while voting, or -1. Result bars keep a
// fixed width so the layout does not move when the counts change.
func pollLines(poll *api.Poll, width int, p palette, now time.Time, focus int) []string {
	if !poll.Valid() {
		return []string{fit(text(p.muted).Render("▤ Poll unavailable"), width)}
	}
	results := showsResults(poll)
	labelWidth := 0
	for _, option := range poll.Options {
		cells := lipgloss.Width(safeText(option.Label))
		if option.Voted {
			cells += 2
		}
		labelWidth = max(labelWidth, cells)
	}
	labelWidth = min(labelWidth+2, max(1, width/2))
	barWidth := width - 2 - labelWidth - 6
	var lines []string
	for i, option := range poll.Options {
		label := safeText(option.Label)
		if !results {
			marker, style := "○ ", text(p.ink)
			if i == focus {
				marker, style = "● ", text(p.blue).Bold(true)
				label = "› " + marker + label
			} else {
				label = "  " + marker + label
			}
			lines = append(lines, fit(style.Render(label), width))
			continue
		}
		share := 0
		if option.Percentage != nil {
			share = max(0, min(100, *option.Percentage))
		}
		if option.Voted {
			label += " ✓"
		}
		row := "  " + lipgloss.NewStyle().Width(labelWidth).MaxWidth(labelWidth).Render(fit(label, labelWidth))
		if barWidth >= 4 {
			filled := (share*barWidth + 50) / 100
			ink := p.muted
			if option.Voted {
				ink = p.blue
			}
			row += text(ink).Render(strings.Repeat("█", filled)) + text(p.hairline).Render(strings.Repeat("░", barWidth-filled))
		}
		row += text(p.muted).Render(fmt.Sprintf(" %4d%%", share))
		lines = append(lines, fit(row, width))
	}
	status := pollStatus(poll, now)
	switch {
	case results:
		noun := "votes"
		if *poll.TotalVotes == 1 {
			noun = "vote"
		}
		status = fmt.Sprintf("%d %s · %s", *poll.TotalVotes, noun, status)
	case poll.ClosesAt != nil && !poll.Closed && poll.ClosesAt.After(now) && !poll.HasVoted:
		status += " · Vote to see results"
	}
	return append(lines, fit("  "+text(p.muted).Render(status), width))
}

func (m AppModel) voteModal(p palette) (string, uiRect, []hitArea) {
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), 60, 11)
	width := max(1, r.w-4)
	label := ""
	if c, ok := m.votingCommit(); ok {
		label = safeText(c.Poll.Options[m.vote.option].Label)
	}
	body := text(p.ink).Bold(true).Render(fit(`Vote for "`+label+`"?`, width)) + "\n\n" + text(p.muted).Render(fit("Votes are final. You can't change or remove it.", width))
	actions := []modalAction{{"Cancel", "esc", "esc"}, {"Vote", "y", "y"}}
	footer, areas := modalFooter(actions, width, m.vote.focus+1, p)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "esc", 0})
	return panel("/dev/null · VOTE", body, footer, r, p, p.warning), r, areas
}
