package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func quotingCommit(id string) api.Commit {
	quoted := fixtureCommit("quoted")
	quoted.Body = "The quoted commit"
	c := fixtureCommit(id)
	c.QuotedID, c.Quote = &quoted.ID, &quoted
	return c
}

func copiedValues(m *AppModel) *[]string {
	var copied []string
	m.copyLink = func(_ context.Context, raw string) error { copied = append(copied, raw); return nil }
	return &copied
}

func TestQOpensTheQuotedCommitFromTheLog(t *testing.T) {
	m := threadApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{quotingCommit("a"), fixtureCommit("b")}})
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "Open quote") {
		t.Fatalf("the action row lacks Open quote:\n%s", frame)
	}
	m, cmd := m.handleKey("Q")
	m = deliver(m, runAll(cmd))
	if len(m.threads) != 1 || m.threads[0].root.ID != "quoted" {
		t.Fatalf("Q opened %+v, want the quoted commit's thread", m.threads)
	}
	m, cmd = m.handleKey("esc")
	m = deliver(m, runAll(cmd))
	m, _ = m.selectCommit(1)
	if strings.Contains(ansi.Strip(m.View().Content), "Open quote") {
		t.Fatal("a commit without a quote shows Open quote")
	}
	if m, _ = m.handleKey("Q"); len(m.threads) != 0 {
		t.Fatal("Q opened a thread for a commit without a quote")
	}
}

func TestQOpensTheQuotedCommitFromThreadsAndProfiles(t *testing.T) {
	m, _ := profileApp(t)
	m.loadUserCommits = func(context.Context, string, bool, string) (api.CommitPage, error) {
		return api.CommitPage{Data: []api.Commit{quotingCommit("p")}}, nil
	}
	cmd := m.openProfile("grace")
	m = deliver(m, runAll(cmd))
	m, cmd = m.handleKey("Q")
	m = deliver(m, runAll(cmd))
	if len(m.threads) != 1 || m.threads[0].root.ID != "quoted" {
		t.Fatal("Q on a profile did not open the quoted commit")
	}

	m = threadApp(t)
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		if id == "quoted" {
			return api.Thread{Commit: fixtureCommit("quoted")}, nil
		}
		return api.Thread{Commit: quotingCommit(id)}, nil
	}
	m, cmd = m.handleKey("enter")
	m = deliver(m, runAll(cmd))
	m, cmd = m.handleKey("Q")
	m = deliver(m, runAll(cmd))
	if len(m.threads) != 2 || m.threads[1].root.ID != "quoted" {
		t.Fatal("Q in a thread did not open the quoted commit")
	}
}

func TestEveryCardShowsCopyLink(t *testing.T) {
	m := threadApp(t)
	copied := copiedValues(&m)
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "Copy link") {
		t.Fatalf("the log card lacks Copy link:\n%s", frame)
	}
	m, cmd := m.handleKey("enter")
	m = deliver(m, runAll(cmd))
	if !strings.Contains(ansi.Strip(m.View().Content), "Copy link") {
		t.Fatal("the thread card lacks Copy link")
	}
	_, cmd = m.handleKey("y")
	runAll(cmd)
	if len(*copied) != 1 || (*copied)[0] != "https://example.test/commit/a" {
		t.Fatalf("y copied %v", *copied)
	}
}

func TestTCopiesTheTextOfOwnAndOrganizationCommits(t *testing.T) {
	m := threadApp(t)
	copied := copiedValues(&m)
	orgs := []auth.Organization{{ID: "org-1", Handle: "acme"}}
	m.account.user.Organizations = &orgs
	own := fixtureCommit("own")
	own.Body, own.Author.ID = "My own words", "fixture"
	org := fixtureCommit("org")
	org.Body, org.Author = "Words from acme", &api.Author{Type: "organization", ID: "org-1", Handle: "acme", Name: "Acme"}
	other := fixtureCommit("other")
	other.Body = "Someone else's words"
	m = applyPage(m, 0, api.Page{Data: []api.Commit{own, org, other}})

	for i, want := range []string{"My own words", "Words from acme"} {
		m, _ = m.selectCommit(i)
		if !strings.Contains(ansi.Strip(m.View().Content), "Copy text") {
			t.Fatalf("commit %d lacks Copy text", i)
		}
		next, cmd := m.handleKey("T")
		m = deliver(next, runAll(cmd))
		if got := (*copied)[len(*copied)-1]; got != want {
			t.Fatalf("T copied %q, want %q", got, want)
		}
		if m.notice != "Commit text copied" {
			t.Fatalf("notice = %q", m.notice)
		}
	}
	m, _ = m.selectCommit(2)
	if strings.Contains(ansi.Strip(m.View().Content), "Copy text") {
		t.Fatal("someone else's commit shows Copy text")
	}
	before := len(*copied)
	_, cmd := m.handleKey("T")
	if runAll(cmd); len(*copied) != before {
		t.Fatal("T copied someone else's commit")
	}
}

func TestCopyTextFallbackShowsTheTextWithItsLines(t *testing.T) {
	m := threadApp(t)
	m.copyLink = func(context.Context, string) error { return errors.New("no clipboard") }
	own := fixtureCommit("own")
	own.Body, own.Author.ID = "First line\nSecond \x1b[31mline", "fixture"
	m = applyPage(m, 0, api.Page{Data: []api.Commit{own}})
	next, cmd := m.handleKey("T")
	m = deliver(next, runAll(cmd))
	frame := ansi.Strip(m.View().Content)
	for _, want := range []string{"Copy commit text", "Copy this text:", "First line", "Second line"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("fallback lacks %q:\n%s", want, frame)
		}
	}
}
