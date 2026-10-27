package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, command := range batch {
			msgs = append(msgs, runAll(command)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func deliver(m AppModel, msgs []tea.Msg) AppModel {
	for _, msg := range msgs {
		next, _ := m.update(msg)
		m = next.(AppModel)
	}
	return m
}

func replyTo(id, parent, body string) api.Commit {
	c := fixtureCommit(id)
	c.Body, c.ParentID = body, &parent
	c.Author = &api.Author{ID: "other", Name: "Grace Fixture", Handle: "grace", Lane: []byte("2"), AvatarURL: "https://example.test/grace.png"}
	return c
}

func threadApp(t *testing.T) AppModel {
	t.Helper()
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("c")}})
	deleted := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		root := fixtureCommit(id)
		root.Body, root.Replies = "Focused "+id, 2
		root.Author.ID = "fixture"
		parent := replyTo("p2", "p1", "Ancestor body")
		return api.Thread{Commit: root, Ancestors: []api.Commit{{ID: "p1", DeletedAt: &deleted, Replies: 1}, parent}}, nil
	}
	m.loadReplies = func(_ context.Context, id, _ string) (api.CommitPage, error) {
		if id == "r1" {
			child := replyTo("c1", "r1", "Nested child")
			child.Replies = 1
			return api.CommitPage{Data: []api.Commit{child}}, nil
		}
		first := replyTo("r1", id, "First reply")
		first.Replies = 1
		quote := replyTo("q1", "", "Quoting this")
		quote.ParentID, quote.QuotedID = nil, &id
		return api.CommitPage{Data: []api.Commit{first, quote}}, nil
	}
	return m
}

func openThread(t *testing.T, m AppModel) AppModel {
	t.Helper()
	m, cmd := m.handleKey("enter")
	if len(m.threads) == 0 || cmd == nil {
		t.Fatal("enter did not open a thread")
	}
	return deliver(m, runAll(cmd))
}

func TestThreadOpensFromLogAndEscRestoresTheRow(t *testing.T) {
	m := threadApp(t)
	m, _ = m.selectCommit(1)
	offset := m.feeds[0].viewport.YOffset()
	m = openThread(t, m)
	frame := ansi.Strip(m.View().Content)
	for _, want := range []string{"Thread", "Commit deleted", "Ancestor body", "Focused b", "First reply", "Quoting this", "quoted"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("thread lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Quoted commit unavailable") {
		t.Fatal("a quote of the focused commit rendered as unavailable")
	}
	m, _ = m.handleKey("esc")
	if len(m.threads) != 0 || m.feeds[0].selected != 1 || m.feeds[0].viewport.YOffset() != offset {
		t.Fatal("leaving the thread lost the log row")
	}
}

func TestQuoteKeyNeverQuits(t *testing.T) {
	m := threadApp(t)
	m, _ = m.handleKey("q")
	if !m.composer.open || m.composer.mode != "quote" || m.composer.target.ID != "a" || m.account.life.ctx.Err() != nil {
		t.Fatal("q must quote the selected commit")
	}
}

func TestReplyAndQuoteSubmitTheirTargets(t *testing.T) {
	m := openThread(t, threadApp(t))
	var drafts []api.NewCommit
	m.submit = func(_ context.Context, draft api.NewCommit) (api.Commit, error) {
		drafts = append(drafts, draft)
		posted := fixtureCommit("posted" + draft.ParentID + draft.QuotedID)
		posted.Status = "pending"
		if draft.ParentID != "" {
			posted.ParentID = &draft.ParentID
		}
		if draft.QuotedID != "" {
			posted.QuotedID = &draft.QuotedID
		}
		return posted, nil
	}
	m, _ = m.handleKey("j")
	m, _ = clickText(t, m, "Reply")
	if !m.composer.open || m.composer.mode != "reply" || m.composer.target.ID != "r1" || !strings.Contains(ansi.Strip(m.View().Content), "Replying to @grace") {
		t.Fatal("reply control did not target the selected reply")
	}
	m.composer.input.SetValue("A reply")
	m = deliver(m, runAll(m.startSubmission()))
	if len(drafts) != 1 || drafts[0].ParentID != "r1" || drafts[0].QuotedID != "" || len(m.feeds[0].commits) != 3 {
		t.Fatalf("reply payload or log insertion wrong: %+v", drafts)
	}
	if m.threads[0].replies[0].Replies != 2 {
		t.Fatal("reply count did not update")
	}
	m, _ = m.handleKey("k")
	m, _ = m.handleKey("q")
	m.composer.input.SetValue("A quote")
	m = deliver(m, runAll(m.startSubmission()))
	if len(drafts) != 2 || drafts[1].QuotedID != "a" || drafts[1].ParentID != "" || m.feeds[0].commits[0].ID != "posteda" {
		t.Fatalf("quote payload or log insertion wrong: %+v", drafts)
	}
	if last := m.threads[0].replies[len(m.threads[0].replies)-1]; last.ID != "posteda" {
		t.Fatal("quote missing from the thread")
	}
}

func TestKeptDraftReopensWithItsOriginalTarget(t *testing.T) {
	m := openThread(t, threadApp(t))
	m, _ = m.handleKey("r")
	m.composer.input.SetValue("Kept reply")
	m = pressApp(m, "esc")
	m = pressApp(m, "ctrl+k")
	if m.composer.open {
		t.Fatal("keep did not close the composer")
	}
	m, _ = m.handleKey("j")
	m, _ = m.handleKey("q")
	c := m.composer
	if !c.open || c.mode != "reply" || c.target.ID != "a" || c.input.Value() != "Kept reply" || !strings.Contains(c.status, "Finish or discard") {
		t.Fatalf("kept draft not reopened: %+v", c.status)
	}
}

func TestNestedRepliesAreBounded(t *testing.T) {
	m := openThread(t, threadApp(t))
	m, _ = m.handleKey("j")
	m, cmd := m.handleKey("space")
	m = deliver(m, runAll(cmd))
	if !strings.Contains(ansi.Strip(m.View().Content), "Nested child") {
		t.Fatal("space did not show nested replies")
	}
	m, _ = m.handleKey("j")
	if m.threadEntries()[m.threads[0].selected].commit.ID != "c1" {
		t.Fatal("nested reply not selectable")
	}
	if _, cmd := m.handleKey("space"); cmd != nil {
		t.Fatal("nesting deeper than one level fetched inline")
	}
	m, cmd = m.handleKey("enter")
	m = deliver(m, runAll(cmd))
	if len(m.threads) != 2 || m.threads[1].root.ID != "c1" {
		t.Fatal("enter did not open the nested reply's thread")
	}
	m, _ = m.handleKey("esc")
	m, _ = m.handleKey("k")
	m, _ = m.handleKey("space")
	if strings.Contains(ansi.Strip(m.View().Content), "Nested child") {
		t.Fatal("space did not hide nested replies")
	}
}

func TestDeleteOwnCommitConfirmsAndLeavesATombstone(t *testing.T) {
	m := openThread(t, threadApp(t))
	calls := 0
	m.deleteCommit = func(context.Context, string) error { calls++; return nil }
	m, _ = m.handleKey("j")
	if m, _ = m.handleKey("D"); m.deleting.id != "" {
		t.Fatal("offered to delete someone else's commit")
	}
	m, _ = m.handleKey("k")
	m, _ = m.handleKey("D")
	if m.deleting.id != "a" || !strings.Contains(ansi.Strip(m.View().Content), "Delete this commit?") {
		t.Fatal("delete did not ask first")
	}
	m = pressApp(m, "enter")
	if m.deleting.id != "" || calls != 0 {
		t.Fatal("confirmation must default to keeping the commit")
	}
	m, _ = m.handleKey("D")
	m, cmd := clickText(t, m, "Delete commit")
	m = deliver(m, runAll(cmd))
	if calls != 1 || m.threads[0].root.DeletedAt == nil || m.threads[0].root.Author != nil || m.feeds[0].commits[0].ID == "a" {
		t.Fatal("deleted commit kept its content or log row")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Commit deleted") {
		t.Fatal("deleted commit not shown as a tombstone")
	}
}

func TestMissingReplyParentKeepsTheDraft(t *testing.T) {
	m := openThread(t, threadApp(t))
	m.submit = func(context.Context, api.NewCommit) (api.Commit, error) {
		return api.Commit{}, &auth.APIError{Status: http.StatusNotFound}
	}
	m, _ = m.handleKey("r")
	m.composer.input.SetValue("Orphan reply")
	m = deliver(m, runAll(m.startSubmission()))
	c := m.composer
	if !c.open || c.uncertain || c.input.Value() != "Orphan reply" || !strings.Contains(c.status, "no longer available") || len(m.feeds[0].commits) != 3 {
		t.Fatalf("missing parent lost the draft or posted top-level: %q", c.status)
	}
}

func TestImageViewerNavigatesImagesAndRestoresTheRow(t *testing.T) {
	m := threadApp(t)
	pictured := fixtureCommit("pics")
	pictured.Body = "Two pictures"
	pictured.Images = []api.Image{{URL: "https://cdn.example.test/1.png", Description: "First diagram"}, {URL: "https://cdn.example.test/2.png", Description: "Second diagram"}}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), pictured}})
	var copied []string
	m.copyLink = func(_ context.Context, raw string) error { copied = append(copied, raw); return nil }
	m, _ = m.selectCommit(1)
	m, cmd := m.handleKey("v")
	m = deliver(m, runAll(cmd))
	frame := ansi.Strip(m.View().Content)
	for _, want := range []string{"1 / 2", "First diagram", "https://cdn.example.test/1.png", "Two pictures", "First reply", "Close"} {
		if !m.viewer.open || !strings.Contains(frame, want) {
			t.Fatalf("viewer lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Quoted commit unavailable") {
		t.Fatal("a quote of the viewed commit rendered as unavailable")
	}
	m, _ = m.handleKey("right")
	m, _ = m.handleKey("right")
	if m.viewer.index != 1 || !strings.Contains(ansi.Strip(m.View().Content), "2 / 2") {
		t.Fatal("right did not stop at the last image")
	}
	m, _ = m.handleKey("p")
	if strings.Contains(ansi.Strip(m.View().Content), "Two pictures") {
		t.Fatal("panel toggle did not hide the commit")
	}
	m, cmd = m.handleKey("y")
	m = deliver(m, runAll(cmd))
	if len(copied) != 1 || copied[0] != "https://cdn.example.test/2.png" {
		t.Fatalf("copied %v", copied)
	}
	m, _ = m.handleKey("esc")
	if m.viewer.open || m.feeds[0].selected != 1 {
		t.Fatal("closing the viewer lost the log row")
	}
	m = deliver(m, []tea.Msg{toastExpired{id: m.toast.id, session: m.session}})
	m, _ = clickText(t, m, "Second diagram")
	if !m.viewer.open || m.viewer.index != 1 {
		t.Fatal("clicking an image did not open it")
	}
}

func TestThreadSelectionFollowsTheCommitWhenEntriesArriveLate(t *testing.T) {
	m := threadApp(t)
	m, cmd := m.handleKey("enter")
	msgs := runAll(cmd)
	for _, msg := range msgs {
		if _, ok := msg.(repliesResult); ok {
			m = deliver(m, []tea.Msg{msg})
		}
	}
	m, _ = m.handleKey("j")
	for _, msg := range msgs {
		if _, ok := msg.(threadResult); ok {
			m = deliver(m, []tea.Msg{msg})
		}
	}
	if got := m.threadEntries()[m.threads[0].selected].commit.ID; got != "r1" {
		t.Fatalf("late ancestors moved the selection to %q", got)
	}
}

func TestDeleteKeepsTheLogRowAndRunsOnce(t *testing.T) {
	m := threadApp(t)
	mine := fixtureCommit("mine")
	mine.Author.ID = "fixture"
	m = applyPage(m, 0, api.Page{Data: []api.Commit{mine, fixtureCommit("a"), fixtureCommit("b")}})
	m, _ = m.selectCommit(2)
	calls := 0
	m.deleteCommit = func(context.Context, string) error {
		calls++
		return &auth.APIError{Status: http.StatusNotFound}
	}
	m, _ = m.handleKey("D")
	if m.deleting.id != "" {
		t.Fatal("offered to delete someone else's commit")
	}
	m, _ = m.selectCommit(0)
	m, _ = m.handleKey("D")
	cmd := m.deleteKey("ctrl+d")
	if m, _ = m.handleKey("D"); m.deleting.id != "" {
		t.Fatal("offered a second delete while one is in flight")
	}
	m = deliver(m, runAll(cmd))
	if calls != 1 || len(m.feeds[0].commits) != 2 || m.toast.title != "Commit deleted" {
		t.Fatalf("a missing commit must count as deleted: calls=%d toast=%q", calls, m.toast.title)
	}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{mine, fixtureCommit("a"), fixtureCommit("b")}})
	m, _ = m.selectCommit(2)
	m.deleteCommit = func(context.Context, string) error { return nil }
	m.deleting = deleteConfirm{id: "mine"}
	m = deliver(m, runAll(m.deleteKey("ctrl+d")))
	if f := m.feeds[0]; f.commits[f.selected].ID != "b" {
		t.Fatal("deleting a row above the selection moved the selection")
	}
}

func TestNarrowThreadActionsWrapInsideTheirRows(t *testing.T) {
	m := threadApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	m = openThread(t, next.(AppModel))
	th := m.threads[0]
	rendered := strings.Count(th.viewport.GetContent(), "\n") + 1
	if th.ends[len(th.ends)-1]+1 != rendered {
		t.Fatalf("entry rows %v do not match %d rendered lines", th.ends, rendered)
	}
	for _, area := range th.actions {
		if area.action == "D" {
			return
		}
	}
	t.Fatal("wrapped Delete action has no hit area")
}

func TestThreadReloadIgnoresEarlierResponses(t *testing.T) {
	m := openThread(t, threadApp(t))
	m, _ = m.handleKey("j")
	_, stale := m.handleKey("space")
	m, _ = m.handleKey("ctrl+r")
	m = deliver(m, runAll(stale))
	if _, shown := m.threads[0].children["r1"]; shown {
		t.Fatal("a response from before the reload was applied")
	}
}

func TestDeleteModalOwnsTheMouseWheel(t *testing.T) {
	m := openThread(t, threadApp(t))
	m, _ = m.handleKey("D")
	selected := m.threads[0].selected
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m = next.(AppModel); m.threads[0].selected != selected {
		t.Fatal("wheel moved the thread behind the delete modal")
	}
}

func TestViewerPanelScrollsToEveryReply(t *testing.T) {
	m := threadApp(t)
	pictured := fixtureCommit("pics")
	pictured.Images = []api.Image{{URL: "https://cdn.example.test/1.png"}, {URL: "https://cdn.example.test/2.png"}}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{pictured}})
	var replies []api.Commit
	for i := range 30 {
		replies = append(replies, replyTo(fmt.Sprintf("r%d", i), "pics", fmt.Sprintf("Reply number %d", i)))
	}
	m.loadReplies = func(context.Context, string, string) (api.CommitPage, error) {
		return api.CommitPage{Data: replies}, nil
	}
	m, cmd := m.handleKey("v")
	m = deliver(m, runAll(cmd))
	for range 200 {
		m, _ = m.handleKey("j")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Reply number 29") || m.viewer.index != 0 {
		t.Fatal("panel scrolling did not reach the last reply or changed the image")
	}
}

func TestClosedThreadIgnoresLateResponses(t *testing.T) {
	m := threadApp(t)
	m, cmd := m.handleKey("enter")
	m, _ = m.handleKey("esc")
	if m = deliver(m, runAll(cmd)); len(m.threads) != 0 || m.feeds[0].selected != 0 {
		t.Fatal("a closed thread's responses changed the workspace")
	}
}

func graphCell(t *testing.T, lines []string, line, col int) string {
	t.Helper()
	cells := []rune(lines[line])
	if col >= len(cells) {
		return ""
	}
	return string(cells[col])
}

func TestRepliesHangOffTheirParentSpine(t *testing.T) {
	m := openThread(t, threadApp(t))
	m, _ = m.handleKey("j")
	m, cmd := m.handleKey("space")
	m = deliver(m, runAll(cmd))
	th := m.threads[0]
	lines := strings.Split(ansi.Strip(th.viewport.GetContent()), "\n")
	ids := []string{}
	for _, entry := range th.entries() {
		ids = append(ids, entry.commit.ID)
	}
	if strings.Join(ids, ",") != "p1,p2,a,r1,c1,q1" {
		t.Fatalf("unexpected entries %v", ids)
	}
	node := func(i int) int { return th.starts[i] }
	stats := func(i int) int {
		for line := th.starts[i]; line < th.ends[i]; line++ {
			if strings.Contains(lines[line], "☆") {
				return line
			}
		}
		t.Fatalf("entry %d has no stats row", i)
		return 0
	}
	checks := []struct {
		line, col int
		want      string
	}{
		{node(1) - 1, 2, "│"}, {node(2), 2, "●"}, {node(2) + 1, 2, "│"},
		{stats(2), 2, "◉"}, {stats(2) + 1, 2, "┆"},
		{node(3), 2, "├"}, {node(3), 3, "╌"}, {node(3), 5, "●"},
		{node(3) + 1, 2, "┆"}, {stats(3), 5, "◉"}, {stats(3) + 1, 5, "┆"},
		{node(4), 2, "┆"}, {node(4), 5, "╰"}, {node(4), 8, "●"},
		{node(5) - 1, 2, "┆"}, {node(5), 2, "╰"}, {node(5), 5, "●"},
	}
	for _, c := range checks {
		if got := graphCell(t, lines, c.line, c.col); got != c.want {
			t.Fatalf("line %d col %d = %q, want %q:\n%s", c.line, c.col, got, c.want, strings.Join(lines, "\n"))
		}
	}
	for _, col := range []int{2, 5} {
		if got := graphCell(t, lines, node(5)+1, col); got != " " {
			t.Fatalf("a line runs past the last reply at col %d: %q", col, got)
		}
	}
	if !strings.Contains(lines[node(5)], "@grace quoted") || strings.Contains(strings.Join(lines, "\n"), "quoted this commit") {
		t.Fatal("quotes must carry an inline quoted tag")
	}
}

func TestThreadRequestsEveryVisibleAuthorAvatar(t *testing.T) {
	m := threadApp(t)
	m.images.Enabled = true
	m = openThread(t, m)
	want := m.avatarRequest("https://example.test/grace.png", "", 5, 2).URL
	for _, req := range m.visibleMedia() {
		if req.URL == want && req.Cols == 5 && req.Rows == 2 {
			return
		}
	}
	t.Fatal("thread replies do not request their author avatars")
}

func TestClickingACommitOpensItsThread(t *testing.T) {
	m := threadApp(t)
	nav, center, aside := m.columns()
	x := (m.account.width-nav-center-aside-2)/2 + nav + 12
	next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: feedHeaderHeight + m.feeds[0].starts[1] + 1})
	m = deliver(next.(AppModel), runAll(cmd))
	if len(m.threads) != 1 || m.threads[0].root.ID != "b" {
		t.Fatal("clicking a commit did not open its thread")
	}
	th := m.threads[0]
	next, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: threadHeaderHeight + th.starts[3] - th.viewport.YOffset() + 1})
	m = deliver(next.(AppModel), runAll(cmd))
	if len(m.threads) != 1 || m.threadEntries()[m.threads[0].selected].commit.ID != "r1" {
		t.Fatal("the first click on a thread entry must select it")
	}
	th = m.threads[0]
	next, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: threadHeaderHeight + th.starts[3] - th.viewport.YOffset() + 1})
	m = deliver(next.(AppModel), runAll(cmd))
	if len(m.threads) != 2 || m.threads[1].root.ID != "r1" {
		t.Fatal("clicking the selected entry did not open its thread")
	}
}

func TestSpineRunsOnToMoreReplies(t *testing.T) {
	m := openThread(t, threadApp(t))
	th := &m.threads[0]
	th.next = "cursor"
	m.renderThread(th)
	lines := strings.Split(ansi.Strip(th.viewport.GetContent()), "\n")
	last := len(lines) - 1
	if !strings.Contains(lines[last], "More replies") || graphCell(t, lines, last, 2) != "╰" || graphCell(t, lines, th.starts[4], 2) != "├" {
		t.Fatalf("the spine must end at More replies:\n%s", strings.Join(lines, "\n"))
	}
}

func TestViewerPanelDrawsTheReplyGraph(t *testing.T) {
	m := threadApp(t)
	pictured := fixtureCommit("pics")
	pictured.Images = []api.Image{{URL: "https://cdn.example.test/1.png"}}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{pictured}})
	m.loadReplies = func(context.Context, string, string) (api.CommitPage, error) {
		return api.CommitPage{Data: []api.Commit{replyTo("r1", "pics", "First answer"), replyTo("r2", "pics", "Second answer")}}, nil
	}
	m, cmd := m.handleKey("v")
	m = deliver(m, runAll(cmd))
	lines := strings.Split(ansi.Strip(strings.Join(m.viewerPanelLines(m.viewerPanelWidth(), colors(true)), "\n")), "\n")
	find := func(text string) int {
		for i, line := range lines {
			if strings.Contains(line, text) {
				return i
			}
		}
		t.Fatalf("missing %q:\n%s", text, strings.Join(lines, "\n"))
		return 0
	}
	stats, first, second := find("☆"), find("First answer")-1, find("Second answer")-1
	checks := []struct {
		line, col int
		want      string
	}{
		{0, 2, "●"}, {1, 2, "│"}, {stats, 2, "◉"}, {stats + 1, 2, "┆"},
		{first, 2, "├"}, {first, 3, "╌"}, {first + 1, 2, "┆"}, {second, 2, "╰"}, {second + 1, 2, " "},
	}
	for _, c := range checks {
		if got := graphCell(t, lines, c.line, c.col); got != c.want {
			t.Fatalf("line %d col %d = %q, want %q:\n%s", c.line, c.col, got, c.want, strings.Join(lines, "\n"))
		}
	}
	if !strings.Contains(lines[first], "@grace") {
		t.Fatalf("reply header does not follow its elbow:\n%s", strings.Join(lines, "\n"))
	}
	m.images.Enabled = true
	want := map[string]bool{}
	for _, req := range m.visibleMedia() {
		want[fmt.Sprintf("%s %dx%d", req.URL, req.Cols, req.Rows)] = true
	}
	if !want[fmt.Sprintf("%s 2x1", graphics.AvatarURL("https://example.test/grace.png"))] {
		t.Fatalf("viewer replies do not request their author avatars: %v", want)
	}
}
