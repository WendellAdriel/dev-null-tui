package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestShowMoreUnfoldsALongCommit(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(AppModel)
	long := fixtureCommit("long")
	long.Body = "The start of a long commit and the hidden ending."
	long.Segments = []api.Segment{{Type: "text", Text: long.Body}}
	long.Preview = []api.Segment{{Type: "text", Text: "The start of a long commit…"}}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{long, fixtureCommit("b")}})
	feed := func(m AppModel) string { return ansi.Strip(m.feeds[0].viewport.GetContent()) }
	if strings.Contains(feed(m), "hidden ending") || !strings.Contains(feed(m), "Show more") {
		t.Fatalf("long commit not folded with a Show more action:\n%s", feed(m))
	}
	m, _ = m.handleKey("m")
	if !strings.Contains(feed(m), "hidden ending") || strings.Contains(feed(m), "Show more") {
		t.Fatalf("m did not unfold the commit:\n%s", feed(m))
	}
	m, _ = m.handleKey("j")
	if !strings.Contains(feed(m), "hidden ending") {
		t.Fatal("moving away folded the commit again")
	}
}

func TestReplyingFromTheLogShowsTheReplyUnderItsParent(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyPage(next.(AppModel), 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
	m = press(t, m, "r")
	m.submit = func(_ context.Context, draft api.NewCommit) (api.Commit, error) {
		c := fixtureCommit("new-reply")
		c.Body, c.ParentID = draft.Body, &draft.ParentID
		return c, nil
	}
	m.composer.input.SetValue("A synthetic reply")
	m = deliver(m, runAll(m.startSubmission()))
	parent := m.feeds[0].commits[0]
	if parent.Replies != 1 || parent.ReplyPreview == nil || len(parent.ReplyPreview.Data) != 1 || parent.ReplyPreview.Data[0].ID != "new-reply" {
		t.Fatalf("parent not updated: %d %+v", parent.Replies, parent.ReplyPreview)
	}
	if !strings.Contains(ansi.Strip(m.feeds[0].viewport.GetContent()), "A synthetic reply") {
		t.Fatal("the log does not show the new reply")
	}
}

func TestThreadShowsItsOpenedCommitWhole(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(AppModel)
	long := func(id, ending string) api.Commit {
		c := fixtureCommit(id)
		c.Body = "The start of a long commit and " + ending + "."
		c.Segments = []api.Segment{{Type: "text", Text: c.Body}}
		c.Preview = []api.Segment{{Type: "text", Text: "The start of a long commit…"}}
		return c
	}
	root, ancestor := long("root", "the opened ending"), long("ancestor", "the ancestor ending")
	m.openThread(root)
	next, _ = m.Update(threadResult{session: m.session, generation: m.threads[0].generation, thread: api.Thread{Commit: root, Ancestors: []api.Commit{ancestor}}})
	m = next.(AppModel)
	thread := ansi.Strip(m.threads[0].viewport.GetContent())
	if !strings.Contains(thread, "the opened ending") || strings.Contains(thread, "Show more") {
		t.Fatalf("the opened commit is not shown whole:\n%s", thread)
	}
	if strings.Contains(thread, "the ancestor ending") {
		t.Fatalf("the ancestor is not folded:\n%s", thread)
	}
}
