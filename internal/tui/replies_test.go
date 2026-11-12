package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func withPreviews(c api.Commit, replies int, previews ...api.Commit) api.Commit {
	c.Replies = replies
	c.ReplyPreview = &struct {
		Data []api.Commit `json:"data"`
	}{Data: previews}
	return c
}

func TestMoreRepliesRowEndsTheSpineWhenPreviewsDontCoverEveryReply(t *testing.T) {
	m := fixtureApp(t)
	first, second := fixtureCommit("r1"), fixtureCommit("r2")
	first.Body, second.Body = "First preview", "Second preview"
	m = applyPage(m, 0, api.Page{Data: []api.Commit{withPreviews(fixtureCommit("a"), 5, first, second), withPreviews(fixtureCommit("b"), 1, first)}})
	lines := strings.Split(ansi.Strip(m.feeds[0].viewport.GetContent()), "\n")
	more := -1
	for i, line := range lines {
		if strings.Contains(line, "more repl") {
			if more >= 0 {
				t.Fatalf("only the first commit has more replies than previews:\n%s", strings.Join(lines, "\n"))
			}
			more = i
		}
	}
	if more < 0 || !strings.Contains(lines[more], "3 more replies") {
		t.Fatalf("missing the 3 more replies row:\n%s", strings.Join(lines, "\n"))
	}
	if got := graphCell(t, lines, more, 2); got != "╰" {
		t.Fatalf("the more row must end the spine, got %q:\n%s", got, strings.Join(lines, "\n"))
	}
}

func TestPickingRepliesPerPageSavesItAndRereadsTheLogs(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	m = applyPage(m, 1, api.Page{Data: []api.Commit{fixtureCommit("b")}})
	var applied []int
	m.applyRepliesPerPage = func(count int) { applied = append(applied, count) }
	fetched := 0
	m.fetch = func(context.Context, api.Log, string, string) (api.Page, error) {
		fetched++
		return api.Page{Log: api.Following, Data: []api.Commit{fixtureCommit("a")}}, nil
	}
	m.settings.open, m.settings.tab = true, settingsAppearance
	var row settingsRow
	for _, r := range m.settingsRows() {
		if r.action == "replies-per-page" {
			row = r
		}
	}
	if row.title != "Replies: 3" {
		t.Fatalf("Appearance must show the Replies row with the web's default, got %q", row.title)
	}
	m.activateSetting(row.action, "")
	if m.picker == nil || m.picker.kind != "replies-per-page" || len(m.picker.rows) != 7 || m.picker.rows[m.picker.selected].value != "3" {
		t.Fatalf("Replies must open a picker of the web's counts on the current one: %+v", m.picker)
	}
	for m.picker.rows[m.picker.selected].value != "10" {
		m = pressApp(m, "j")
	}
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(AppModel)
	for _, msg := range collect(cmd) {
		updated, _ := m.Update(msg)
		m = updated.(AppModel)
	}
	if m.picker != nil || m.tuiSettings.RepliesPerPage != 10 || m.account.store.TUISettings().RepliesPerPage != 10 {
		t.Fatal("picking 10 must apply and save it")
	}
	if len(applied) != 1 || applied[0] != 10 {
		t.Fatalf("the client must ask for 10 previews, got %v", applied)
	}
	if fetched != 1 || m.feeds[1].loaded {
		t.Fatalf("the open log must be read again and the others on their next visit (fetched %d, latest loaded %v)", fetched, m.feeds[1].loaded)
	}
}

func TestAConfirmedReplyJoinsTheExistingPreviews(t *testing.T) {
	previews := []api.Commit{fixtureCommit("r3"), fixtureCommit("r2"), fixtureCommit("r1")}
	parent := withPreviews(fixtureCommit("a"), 3, previews...)
	addReply(&parent, "a", fixtureCommit("r4"), 3)
	got := []string{}
	for _, c := range parent.ReplyPreview.Data {
		got = append(got, c.ID)
	}
	if parent.Replies != 4 || strings.Join(got, ",") != "r4,r3,r2" {
		t.Fatalf("the reply must come first and keep the newest previews up to the count, got %v (%d replies)", got, parent.Replies)
	}
}

func TestANewRepliesCountRereadsEveryLoadedProfileTab(t *testing.T) {
	m, calls := profileApp(t)
	m, _ = m.selectCommit(1)
	m = press(t, m, "u", "tab", "tab")
	if fmt.Sprint(calls.commits) != "[ada replies=false cursor= ada replies=true cursor=]" {
		t.Fatalf("both tabs must be loaded first: %v", calls.commits)
	}
	m.applyRepliesPerPage = func(int) {}
	m = deliver(m, runAll(m.setRepliesPerPage("10")))
	m = press(t, m, "tab")
	if fmt.Sprint(calls.commits) != "[ada replies=false cursor= ada replies=true cursor= ada replies=false cursor= ada replies=true cursor=]" {
		t.Fatalf("the open tab must be read again now and the other on its next visit: %v", calls.commits)
	}
}

func TestTheSelectedCommitsMoreRepliesRowIsAnEnterChip(t *testing.T) {
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 45})
	m = updated.(AppModel)
	a, b, preview := fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("r1")
	a.Body, b.Body, preview.Body = "Root", "Other", "Reply"
	m = applyPage(m, 0, api.Page{Data: []api.Commit{withPreviews(a, 3, preview), withPreviews(b, 2, preview)}})
	lines := strings.Split(ansi.Strip(m.feeds[0].viewport.GetContent()), "\n")
	var rows []string
	for _, line := range lines {
		if strings.Contains(line, "more repl") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 2 || !strings.Contains(rows[0], "2 more replies  enter") || strings.Contains(rows[1], "enter") {
		t.Fatalf("only the selected commit's more row names the key:\n%s", strings.Join(lines, "\n"))
	}
	m, _ = clickText(t, m, "2 more replies")
	if len(m.threads) != 1 || m.threads[0].root.ID != "a" {
		t.Fatal("clicking the chip must open the commit's thread")
	}
}
