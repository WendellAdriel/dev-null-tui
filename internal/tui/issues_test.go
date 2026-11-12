package tui

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// issueApp is a 120x35 workspace whose issues go to the fixture, which answers with each queued error, then opens one.
func issueApp(t *testing.T, failures ...error) (AppModel, *[]api.NewIssue) {
	t.Helper()
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "pt_PT.UTF-8")
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = updated.(AppModel)
	sent := []api.NewIssue{}
	m.openIssue = func(_ context.Context, draft api.NewIssue) (api.OpenedIssue, error) {
		sent = append(sent, draft)
		if len(failures) > 0 {
			err := failures[0]
			failures = failures[1:]
			return api.OpenedIssue{}, err
		}
		return api.OpenedIssue{Number: 1555, URL: "https://github.com/dev-null-sh/dev-null.sh/issues/1555"}, nil
	}
	return m, &sent
}

func submitIssue(m AppModel) AppModel {
	next, cmd := m.Update(keyMsg("ctrl+s"))
	return deliver(next.(AppModel), runAll(cmd))
}

func TestPaletteActionsOpenTheComposerAsAnIssue(t *testing.T) {
	m, sent := issueApp(t)
	m.openSearchOption(searchOption{kind: "action", action: "feature"})
	frame := frameText(m)
	if m.composer.mode != "feature" || !strings.Contains(frame, "Request a feature") || !strings.Contains(frame, "# Feature title") {
		t.Fatalf("the feature action must open the issue composer with its template:\n%s", frame)
	}
	m = pressApp(m, "ctrl+o")
	if m.composer.browser != nil || strings.Contains(frameText(m), "Images (or drop)") {
		t.Fatal("issues are text only")
	}
	m = paste(m, "# Dark mode\n\nPlease.")
	m = submitIssue(m)
	if len(*sent) != 1 {
		t.Fatalf("sent %d issues", len(*sent))
	}
	env := (*sent)[0].Environment
	if (*sent)[0].Command != "/feature" || (*sent)[0].Body != "# Dark mode\n\nPlease." || env["os"] == "" || env["viewport"] != "120x35" || env["language"] != "pt-PT" || env["color_scheme"] != "dark" || !strings.Contains(env["browser"], "ghostty") || env["page"] != "log/following" {
		t.Fatalf("sent %+v", (*sent)[0])
	}
	if m.composer.open || m.composer.hasDraft() || m.toast.title != "Opened issue #1555" {
		t.Fatalf("an opened issue must close the composer and say so, toast %q", m.toast.title)
	}
	var opened string
	m.openURL = func(_ context.Context, raw string) error { opened = raw; return nil }
	m, cmd := clickText(t, m, "Opened issue #1555")
	deliver(m, runAll(cmd))
	if opened != "https://github.com/dev-null-sh/dev-null.sh/issues/1555" {
		t.Fatalf("clicking the toast must open the issue, opened %q", opened)
	}
}

func TestTypingACommandTurnsTheDraftIntoAnIssue(t *testing.T) {
	m, _ := issueApp(t)
	m = pressApp(m, "c")
	m = typeText(m, "/bug ")
	if m.composer.mode != "bug" || m.composer.input.Value() != "" || !strings.Contains(frameText(m), "Report a bug") {
		t.Fatalf("typing /bug must start a bug report, mode %q value %q", m.composer.mode, m.composer.input.Value())
	}
	m = typeText(m, "/feature ")
	if m.composer.mode != "bug" || m.composer.input.Value() != "/feature " {
		t.Fatal("a report keeps its kind once started")
	}
}

func TestIssueErrorsKeepTheDraft(t *testing.T) {
	refused := &auth.APIError{Status: http.StatusBadGateway, Code: "github_issue_refused", Message: "GitHub refused the issue."}
	unanswered := &auth.APIError{Status: http.StatusBadGateway, Code: "github_issue_unanswered", Message: "GitHub didn't answer."}
	limited := &auth.APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Issue limit reached: 10 a day. Try again tomorrow.", RetryAfter: "3600"}
	m, sent := issueApp(t, refused, limited, unanswered)
	m.composeTo("bug", api.Commit{})
	m = typeText(m, "It crashes on start")
	m = submitIssue(m)
	if !m.composer.open || m.composer.input.Value() != "It crashes on start" || m.composer.uncertain || !strings.Contains(m.composer.status, "Nothing was opened") {
		t.Fatalf("a refused issue keeps the draft and can be sent again: %q", m.composer.status)
	}
	m = submitIssue(m)
	if !strings.Contains(m.composer.status, "Issue limit reached") || m.composer.uncertain {
		t.Fatalf("the rate limit must show the server's reason: %q", m.composer.status)
	}
	m = submitIssue(m)
	if !m.composer.uncertain || !strings.Contains(m.composer.status, "may already exist") {
		t.Fatalf("an unanswered issue may exist, so it needs a check: %q", m.composer.status)
	}
	m = submitIssue(m)
	if len(*sent) != 3 || m.composer.confirm != "retry" {
		t.Fatalf("sending again after an unknown outcome must ask first (sent %d, confirm %q)", len(*sent), m.composer.confirm)
	}
}

func TestAnIssueFromARestoredDraftRemovesThatDraft(t *testing.T) {
	m, sent := issueApp(t)
	m.commitDrafts = map[string]auth.CommitDraft{"commit": {Kind: "commit", Body: "/bug the app crashes"}}
	if err := m.account.store.SaveCommitDrafts(m.account.user.ID, m.commitDrafts); err != nil {
		t.Fatal(err)
	}
	m = pressApp(m, "c")
	m = typeText(m, " on start")
	if m.composer.mode != "bug" {
		t.Fatalf("the restored command must start a bug report, mode %q", m.composer.mode)
	}
	m = submitIssue(m)
	if len(*sent) != 1 {
		t.Fatalf("sent %d issues", len(*sent))
	}
	if _, ok := m.account.store.CommitDrafts(m.account.user.ID)["commit"]; ok {
		t.Fatal("the saved draft the issue came from must be removed, or it would rebuild the same report")
	}
}

func TestIssueRetryConfirmationTalksAboutTheIssue(t *testing.T) {
	m, _ := issueApp(t, &auth.APIError{Status: http.StatusBadGateway, Code: "github_issue_unanswered", Message: "GitHub didn't answer."})
	m.composeTo("bug", api.Commit{})
	m = typeText(m, "It crashes on start")
	m = submitIssue(m)
	m = submitIssue(m)
	if frame := frameText(m); !strings.Contains(frame, "may already exist") || strings.Contains(frame, "previous commit") {
		t.Fatalf("the retry confirmation must talk about the issue:\n%s", frame)
	}
}

func TestIssueRateLimitAndMissingRouteExplainThemselves(t *testing.T) {
	m, _ := issueApp(t)
	if got, _ := m.issueError(&auth.APIError{Status: http.StatusTooManyRequests, Message: "Too Many Attempts.", RetryAfter: "30"}); !strings.Contains(got, "30 seconds") || !strings.Contains(got, "draft is kept") {
		t.Fatalf("a per-minute limit must say when to retry: %q", got)
	}
	if got, _ := m.issueError(&auth.APIError{Status: http.StatusNotFound}); strings.Contains(got, "commit") {
		t.Fatalf("a missing issues route must not talk about a commit: %q", got)
	}
}

func TestLeavingTheApplicationForgetsTheIssueAndTopicCalls(t *testing.T) {
	m, _ := issueApp(t)
	m.applyRepliesPerPage = func(int) {}
	m.headlineCalls = headlineCalls{list: func(context.Context) ([]api.Headline, error) { return nil, nil }}
	m.leaveApplication()
	if m.openIssue != nil || m.applyRepliesPerPage != nil || m.headlineCalls.list != nil {
		t.Fatal("a later connection must build these calls on its own signing key")
	}
}

func TestCtrlOOpensTheNewIssueWhileItsNoticeShows(t *testing.T) {
	screens := map[string]func(*AppModel){
		"log":           func(*AppModel) {},
		"remotes":       func(m *AppModel) { m.remotes.open = true },
		"notifications": func(m *AppModel) { m.notificationsOpen = true },
		"hot topics":    func(m *AppModel) { m.topics.open = true },
	}
	for name, show := range screens {
		m, _ := issueApp(t)
		m.composeTo("feature", api.Commit{})
		m = typeText(m, "Dark mode please")
		m = submitIssue(m)
		show(&m)
		if !strings.Contains(frameText(m), "ctrl+o") || !slices.ContainsFunc(m.bindings().all, func(b key.Binding) bool { return b.Help().Key == "ctrl+o" }) {
			t.Fatalf("%s: the notice and the help must name the key:\n%s", name, frameText(m))
		}
		var opened []string
		m.openURL = func(_ context.Context, raw string) error { opened = append(opened, raw); return nil }
		m = pressThrough(m, "ctrl+o")
		if len(opened) != 1 || opened[0] != "https://github.com/dev-null-sh/dev-null.sh/issues/1555" {
			t.Fatalf("%s: ctrl+o must open the issue, opened %v", name, opened)
		}
		updated, _ := m.Update(toastExpired{m.toast.id, m.session})
		m = pressThrough(updated.(AppModel), "ctrl+o")
		if len(opened) != 1 {
			t.Fatalf("%s: ctrl+o must do nothing once the notice is gone", name)
		}
	}
}

// pressThrough presses a key and delivers everything it starts.
func pressThrough(m AppModel, key string) AppModel {
	next, cmd := m.Update(keyMsg(key))
	return deliver(next.(AppModel), runAll(cmd))
}
