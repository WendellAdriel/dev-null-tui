package tui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func openPoll(closes time.Time) *api.Poll {
	return &api.Poll{ID: "poll", ClosesAt: &closes, Options: []api.PollOption{{ID: "o1", Label: "Tabs"}, {ID: "o2", Label: "Spaces"}}}
}

func votedPoll(closes time.Time) *api.Poll {
	poll := openPoll(closes)
	poll.HasVoted, poll.TotalVotes = true, new(3)
	poll.Options[0].Votes, poll.Options[0].Percentage = new(1), new(33)
	poll.Options[1].Votes, poll.Options[1].Percentage, poll.Options[1].Voted = new(2), new(67), true
	return poll
}

func pollApp(t *testing.T) AppModel {
	t.Helper()
	m := fixtureApp(t)
	c := fixtureCommit("p1")
	c.Body, c.Author.ID = "Pick one", "other"
	c.Poll = openPoll(m.now.Add(50 * time.Hour))
	plain := fixtureCommit("p2")
	plain.Author.ID = "other"
	plain.Poll = openPoll(m.now.Add(time.Hour))
	page := api.Page{Data: []api.Commit{c, plain}}
	m = applyPage(m, 0, page)
	return applyPage(m, 1, page)
}

func feedText(m AppModel) string { return ansi.Strip(m.feeds[m.active].viewport.GetContent()) }

func TestPollStatesFollowTheWeb(t *testing.T) {
	m := pollApp(t)
	content := feedText(m)
	for _, want := range []string{"Tabs", "Spaces", "Closes in 2d", "Vote to see results", "Vote  o"} {
		if !strings.Contains(content, want) {
			t.Fatalf("open poll lacks %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "%") {
		t.Fatal("hidden results show percentages")
	}
	cases := map[string]func(*api.Commit){
		"Opens when approved": func(c *api.Commit) { c.Status, c.Author.ID, c.Poll.ClosesAt = "pending", "fixture", nil },
		"Final results":       func(c *api.Commit) { c.Poll = votedPoll(m.now.Add(-time.Hour)); c.Poll.Closed = true },
		"3 votes":             func(c *api.Commit) { c.Poll = votedPoll(m.now.Add(time.Hour)) },
	}
	for want, change := range cases {
		m := pollApp(t)
		change(&m.feeds[0].commits[0])
		m.reflow()
		f := m.feeds[0]
		content := strings.Join(strings.Split(feedText(m), "\n")[f.starts[0]:f.ends[0]], "\n")
		if !strings.Contains(content, want) || strings.Contains(content, "Vote  o") || strings.Contains(content, "Vote to see results") {
			t.Fatalf("%s state:\n%s", want, content)
		}
		if want != "Opens when approved" && (!strings.Contains(content, "67%") || !strings.Contains(content, "✓")) {
			t.Fatalf("results lack percentages or the user's vote:\n%s", content)
		}
		if p := press(t, m, "o"); p.vote.commitID != "" {
			t.Fatalf("%s poll accepted a vote", want)
		}
	}
}

func TestVoteNeedsConfirmationAndUpdatesEveryCopy(t *testing.T) {
	m := pollApp(t)
	var sent []string
	m.castVote = func(_ context.Context, id, option string) (api.Commit, error) {
		sent = append(sent, id+":"+option)
		c := m.feeds[0].commits[0]
		c.Poll = votedPoll(m.now.Add(50 * time.Hour))
		return c, nil
	}
	m = press(t, m, "o", "j", "enter")
	if !m.vote.confirm || m.vote.focus != 0 || !strings.Contains(frameText(m), "Votes are final") || !strings.Contains(frameText(m), `"Spaces"`) {
		t.Fatalf("vote modal missing or not on Cancel:\n%s", frameText(m))
	}
	m = press(t, m, "enter")
	if len(sent) != 0 || m.vote.confirm || m.vote.commitID == "" {
		t.Fatal("Cancel sent a vote or left the poll")
	}
	m = press(t, m, "enter", "y")
	if len(sent) != 1 || sent[0] != "p1:o2" || m.vote.commitID != "" {
		t.Fatalf("vote sent %v", sent)
	}
	for _, f := range m.feeds[:2] {
		if !f.commits[0].Poll.HasVoted || f.commits[1].Poll.HasVoted {
			t.Fatal("vote did not update only the voted poll in every log")
		}
	}
	if !strings.Contains(feedText(m), "67%") || m.toast.kind != "success" {
		t.Fatalf("results or toast missing:\n%s", feedText(m))
	}
}

func TestVoteRefusalKeepsThePollAndUnknownOutcomeReloads(t *testing.T) {
	m := pollApp(t)
	m.castVote = func(context.Context, string, string) (api.Commit, error) {
		return api.Commit{}, &auth.APIError{Status: http.StatusUnprocessableEntity, FieldErrors: []string{"This poll has closed."}}
	}
	refusedReloads := 0
	m.loadThread = func(context.Context, string) (api.Thread, error) {
		refusedReloads++
		return api.Thread{}, errors.New("offline")
	}
	m = voteAndFollow(press(t, m, "o", "enter"))
	if m.feeds[0].commits[0].Poll.HasVoted || m.toast.kind != "error" || !strings.Contains(m.toast.detail, "This poll has closed.") {
		t.Fatalf("refusal: %+v", m.toast)
	}
	if refusedReloads != 1 {
		t.Fatalf("a refused vote reloaded the poll %d times", refusedReloads)
	}
	reloaded := 0
	m.castVote = func(context.Context, string, string) (api.Commit, error) {
		return api.Commit{}, errors.New("connection dropped")
	}
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		reloaded++
		c := m.feeds[0].commits[0]
		c.Poll = votedPoll(m.now.Add(time.Hour))
		return api.Thread{Commit: c}, nil
	}
	m = voteAndFollow(press(t, m, "o", "enter"))
	if reloaded != 1 || !m.feeds[0].commits[0].Poll.HasVoted || m.toast.kind != "warning" {
		t.Fatalf("unknown outcome: reloaded=%d toast=%+v", reloaded, m.toast)
	}
}

func TestEscLeavesThePollWithoutVoting(t *testing.T) {
	m := pollApp(t)
	m.castVote = func(context.Context, string, string) (api.Commit, error) {
		t.Fatal("esc sent a vote")
		return api.Commit{}, nil
	}
	m = press(t, m, "o", "enter", "esc")
	if m.vote.confirm || m.vote.commitID == "" {
		t.Fatal("esc in the modal did not return to the options")
	}
	m = press(t, m, "esc")
	if m.vote.commitID != "" || len(m.threads) != 0 {
		t.Fatal("esc did not leave the poll")
	}
}

// quickMsgs runs a command's batch concurrently and keeps the messages that arrive at once, skipping timers such as
// toast expiry.
// voteAndFollow confirms the vote and delivers the commands its result starts, skipping the slow toast timers.
func voteAndFollow(m AppModel) AppModel {
	next, cmd := m.handleKey("y")
	for _, msg := range runAll(cmd) {
		updated, follow := next.update(msg)
		next = deliver(updated.(AppModel), quickMsgs(follow))
	}
	return next
}

func quickMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	results := make(chan []tea.Msg, 8)
	msg := cmd()
	commands, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	for _, command := range commands {
		go func() { results <- runAll(command) }()
	}
	var msgs []tea.Msg
	timeout := time.After(200 * time.Millisecond)
	for range commands {
		select {
		case got := <-results:
			msgs = append(msgs, got...)
		case <-timeout:
			return msgs
		}
	}
	return msgs
}

// clickNth clicks the nth visible occurrence of label, counting from zero.
func clickNth(t *testing.T, m AppModel, label string, nth int) AppModel {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		for offset := 0; ; {
			pos := strings.Index(line[offset:], label)
			if pos < 0 {
				break
			}
			if nth == 0 {
				next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: ansi.StringWidth(line[:offset+pos]) + 1, Y: y})
				return deliver(next.(AppModel), quickMsgs(cmd))
			}
			nth, offset = nth-1, offset+pos+len(label)
		}
	}
	t.Fatalf("visible %q number %d not found", label, nth)
	return m
}

func TestClickingAPollOptionAsksToConfirmThatVote(t *testing.T) {
	sized, _ := pollApp(t).Update(tea.WindowSizeMsg{Width: 120, Height: 45})
	m := sized.(AppModel)
	voted := false
	m.castVote = func(context.Context, string, string) (api.Commit, error) {
		voted = true
		return api.Commit{}, nil
	}
	m = clickNth(t, m, "Spaces", 1)
	if m.feeds[0].selected != 1 || !m.vote.confirm || m.vote.commitID != "p2" || m.vote.option != 1 || m.vote.focus != 0 || voted {
		t.Fatalf("click on the second poll's Spaces: selected=%d vote=%+v voted=%v", m.feeds[0].selected, m.vote, voted)
	}
	m = press(t, m, "esc")
	m.feeds[0].commits[0].Poll = votedPoll(m.now.Add(time.Hour))
	m.reflow()
	m = clickNth(t, m, "Tabs", 0)
	if m.vote.confirm || voted {
		t.Fatalf("clicking a poll with results started a vote: %+v", m.vote)
	}
}

func TestClickingAPollOptionInAThreadAsksToConfirmThatVote(t *testing.T) {
	m := pollApp(t)
	m.castVote = func(context.Context, string, string) (api.Commit, error) { return api.Commit{}, nil }
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		return api.Thread{Commit: m.feeds[0].commits[0]}, nil
	}
	m = press(t, m, "enter")
	if !m.threadOnTop() {
		t.Fatal("enter did not open the thread")
	}
	m = clickNth(t, m, "Tabs", 0)
	if !m.vote.confirm || m.vote.commitID != "p1" || m.vote.option != 0 {
		t.Fatalf("thread click: %+v", m.vote)
	}
}
