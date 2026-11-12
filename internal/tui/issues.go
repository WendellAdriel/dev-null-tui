package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

// issueTemplates are the web's placeholders for /feature and /bug, from issue-commands.ts.
var issueTemplates = map[string]string{
	"feature": "# Feature title\n\n## Problem\nWhat is missing?\n\n## Proposal\nHow should it work?",
	"bug":     "# Bug title\n\n## Steps\n1. How to reproduce it\n\n## Expected vs. actual\nWhat should happen, and what happens instead?",
}

// typedIssueCommand is the web's typedCommand: a complete command and a space or new line at the start of a draft.
var typedIssueCommand = regexp.MustCompile(`^/(feature|bug)\s`)

type issueResult struct {
	session, generation int
	issue               api.OpenedIssue
	err                 error
}

func isIssueMode(mode string) bool { return mode == "feature" || mode == "bug" }

func (c composer) issue() bool { return isIssueMode(c.mode) }

// startIssueMode turns the composer into a /feature or /bug report, with the web's template as its placeholder.
func (c *composer) startIssueMode(kind string) {
	c.mode, c.target = kind, api.Commit{}
	c.input.Placeholder = issueTemplates[kind]
}

// takeIssueCommand starts a report when a plain new commit begins with /feature or /bug, dropping the command.
func (m *AppModel) takeIssueCommand() {
	c := &m.composer
	if c.mode != "" || c.attachmentCount() > 0 || c.author != nil || c.scheduleAt != nil {
		return
	}
	value := c.input.Value()
	match := typedIssueCommand.FindStringSubmatch(value)
	if match == nil {
		return
	}
	// A saved draft that becomes an issue is removed once the issue opens, or it would rebuild the same report.
	if saved, ok := m.commitDrafts[draftKey("", api.Commit{})]; ok && strings.HasPrefix(saved.Body, match[0]) {
		c.issueDraft = draftKey("", api.Commit{})
	}
	c.startIssueMode(match[1])
	c.input.SetValue(value[len(match[0]):])
	m.sizeComposer()
}

func (m *AppModel) startIssue() tea.Cmd {
	c := &m.composer
	draft := api.NewIssue{Command: "/" + c.mode, Body: c.input.Value(), Environment: m.issueEnvironment()}
	if err := draft.Validate(); err != nil {
		c.status = sentence(err.Error())
		return m.notify("error", "Cannot send", c.status)
	}
	if c.uncertain {
		c.confirm, c.confirmFocus = "retry", 0
		return nil
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.generation++
	c.sending, c.loadingPreview = true, false
	c.suggest.close()
	c.status = "Signing and opening the issue..."
	c.input.Blur()
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	c.cancel = cancel
	session, generation, open := m.session, c.generation, m.openIssue
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		issue, err := open(ctx, draft)
		return issueResult{session: session, generation: generation, issue: issue, err: err}
	})
}

func (m *AppModel) acceptIssue(result issueResult) tea.Cmd {
	c := &m.composer
	if result.session != m.session || result.generation != c.generation || !c.sending {
		return nil
	}
	c.sending, c.cancel = false, nil
	if result.err != nil {
		c.focus, c.previewing = 0, false
		kind := "error"
		c.status, c.uncertain = m.issueError(result.err)
		if c.uncertain {
			kind = "warning"
		}
		if m.reconnectComposer(result.err) {
			return m.notify(kind, "Issue not opened", c.status)
		}
		return tea.Batch(m.focusComposer(), m.notify(kind, "Issue not opened", c.status))
	}
	if c.issueDraft != "" {
		_ = m.removeDraftAt(c.issueDraft)
	}
	m.clearComposer()
	title := fmt.Sprintf("Opened issue #%d", result.issue.Number)
	// Longer than other notices, so there is time to press ctrl+o.
	cmd := m.notifyFor("success", title, "Press ctrl+o or click to open it on GitHub. Thanks for the report.", 10*time.Second)
	m.toast.link = result.issue.URL
	return cmd
}

// issueError explains a failed report, and whether the issue may exist anyway, which needs a check before sending again.
func (m AppModel) issueError(err error) (string, bool) {
	var failure *auth.APIError
	if !errors.As(err, &failure) {
		return "Outcome unknown, so the issue may already exist. Check GitHub before sending again. Your draft is kept.", true
	}
	reason := safeText(failure.Message)
	switch {
	case failure.Code == "github_issue_refused":
		return "GitHub refused the issue. Nothing was opened, so you can send it again. Your draft is kept.", false
	case failure.Code == "github_issue_unanswered", failure.Status >= 500 || failure.Status == http.StatusRequestTimeout:
		return "GitHub didn't answer, so the issue may already exist. Check GitHub before sending again. Your draft is kept.", true
	case failure.Status == http.StatusTooManyRequests && reason != "" && reason != "Too Many Attempts.":
		return reason + " Your draft is kept.", false
	case failure.Status == http.StatusTooManyRequests:
		// The per-minute throttle answers with Laravel's generic message, so the wait comes from Retry-After.
		return retryAfterText(failure.RetryAfter, "Too many reports. Try again in a minute.") + " Your draft is kept.", false
	case failure.Status == http.StatusNotFound:
		return "This server can't open issues. Your draft is kept.", false
	case failure.Code == "timed_out" && reason != "":
		return reason + " Your draft is kept.", false
	}
	return m.composerError(err), false
}

// issueEnvironment is what a terminal knows of the web's environment fields. It ends up in a public issue, so it stays
// coarse.
func (m AppModel) issueEnvironment() map[string]string {
	version := m.version
	if version == "" {
		version = release.Version
	}
	client := "devnull " + version
	if terminal := cmp.Or(os.Getenv("TERM_PROGRAM"), os.Getenv("TERM")); terminal != "" {
		client += " (" + terminal + ")"
	}
	scheme := "light"
	if m.account.isDark() {
		scheme = "dark"
	}
	return map[string]string{
		"browser":      client,
		"os":           runtime.GOOS + "/" + runtime.GOARCH,
		"viewport":     fmt.Sprintf("%dx%d", m.account.width, m.account.height),
		"language":     locale(cmp.Or(os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"))),
		"color_scheme": scheme,
		"page":         m.screenName(),
	}
}

// locale turns a POSIX locale such as pt_PT.UTF-8 into a language tag like the browser's, pt-PT.
func locale(value string) string {
	value, _, _ = strings.Cut(value, ".")
	value, _, _ = strings.Cut(value, "@")
	if value == "C" || value == "POSIX" {
		return ""
	}
	return strings.ReplaceAll(value, "_", "-")
}

// screenName names the screen behind the composer, like the web's page path.
func (m AppModel) screenName() string {
	switch {
	case m.threadOnTop():
		return "thread"
	case m.profileOnTop():
		return "profile"
	case m.notificationsOpen:
		return "notifications"
	case m.remotes.open:
		return "remotes"
	case m.drafts.open:
		return "drafts"
	case m.topics.open:
		return "hot topics"
	case m.headline != nil:
		return "hot topic"
	case m.scheduled != nil:
		return "scheduled"
	case m.commitSearch != "":
		return "search"
	case m.starredOpen:
		return "starred"
	}
	return "log/" + string(logs[m.active])
}
