package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func pressApp(m AppModel, key string) AppModel {
	msg := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	if key == "esc" {
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	if key == "tab" {
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	}
	if key == "enter" {
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	if key == "ctrl+c" {
		msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	if strings.HasPrefix(key, "ctrl+") && len(key) == 6 {
		msg = tea.KeyPressMsg{Code: rune(key[5]), Mod: tea.ModCtrl}
	}
	next, _ := m.Update(msg)
	return next.(AppModel)
}

func TestComposerOwnsTypingPasteAndProtectsDraft(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	if !m.composer.open {
		t.Fatal("c did not focus composer")
	}
	for _, key := range []string{"j", "g", "e", "a", "?", "q"} {
		m = pressApp(m, key)
	}
	next, _ := m.Update(tea.PasteMsg{Content: " gl?j\nSynthetic paste"})
	m = next.(AppModel)
	want := "jgea?q gl?j\nSynthetic paste"
	if m.composer.input.Value() != want || m.active != 0 || m.accountOpen || m.help || m.pendingG {
		t.Fatalf("input escaped composer: %q", m.composer.input.Value())
	}
	m = pressApp(m, "esc")
	if m.composer.confirm != "discard" || m.composer.input.Value() != want {
		t.Fatal("cancel lost draft")
	}
	m = pressApp(m, "enter")
	if m.composer.open || m.composer.confirm != "" || m.composer.input.Value() != want {
		t.Fatal("confirmation must default to keep")
	}
	m = pressApp(m, "ctrl+c")
	if m.composer.confirm != "exit" || m.composer.input.Value() != want {
		t.Fatal("exit did not protect draft")
	}
}

func TestSubmissionSingleFlightAndUncertainRecovery(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic draft")
	calls := 0
	m.submit = func(context.Context, api.NewCommit) (api.Commit, error) {
		calls++
		return api.Commit{}, errors.New("transport lost")
	}
	cmd := m.startSubmission()
	if cmd == nil || !m.composer.sending || m.startSubmission() != nil {
		t.Fatal("duplicate submit not blocked")
	}
	result := cmd().(submissionResult)
	next, cmd := m.Update(result)
	m = next.(AppModel)
	if calls != 1 || m.composer.sending || !m.composer.uncertain || m.composer.input.Value() != "Synthetic draft" {
		t.Fatal("uncertain result lost draft or retried")
	}
	if retry := m.startSubmission(); retry != nil || m.composer.confirm != "retry" {
		t.Fatal("uncertain retry needs explicit duplicate warning")
	}
	_ = cmd
}

func TestSubmissionModerationAndFeedIsolation(t *testing.T) {
	for _, status := range []string{"pending", "approved", "rejected"} {
		t.Run(status, func(t *testing.T) {
			m := fixtureApp(t)
			m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
			m = applyPage(m, 1, api.Page{Data: []api.Commit{fixtureCommit("other"), fixtureCommit("anchor")}})
			m.feeds[1].selected = 1
			m = pressApp(m, "c")
			m.composer.input.SetValue("Synthetic draft")
			commit := fixtureCommit("new")
			commit.Status = status
			m.submit = func(context.Context, api.NewCommit) (api.Commit, error) { return commit, nil }
			cmd := m.startSubmission()
			result := cmd().(submissionResult)
			stale := result
			stale.session--
			next, _ := m.Update(stale)
			m = next.(AppModel)
			if !m.composer.sending {
				t.Fatal("stale submission accepted")
			}
			next, _ = m.Update(result)
			m = next.(AppModel)
			if m.feeds[1].selected != 1 || len(m.feeds[1].commits) != 2 {
				t.Fatal("unrelated log changed")
			}
			if status == "rejected" {
				if !m.composer.open || m.composer.input.Value() == "" || len(m.feeds[0].commits) != 2 {
					t.Fatal("rejection lost draft or inserted content")
				}
			} else if m.composer.open || m.feeds[0].commits[0].Status != status || !strings.Contains(m.notice, status) {
				t.Fatal("success lost moderation state")
			}
		})
	}
}

func TestComposerValidationPreviewAndLayout(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic draft")
	m.preview = func(context.Context, string) (api.Preview, error) {
		return api.Preview{Body: "Preview", Segments: []api.Segment{{Type: "text", Text: "Safe preview"}}}, nil
	}
	cmd := m.startPreview()
	next, _ := m.Update(cmd())
	m = next.(AppModel)
	if !m.composer.previewing || m.composer.input.Value() != "Synthetic draft" {
		t.Fatal("preview changed draft")
	}
	m.composer.sending = true
	next, _ = m.Update(submissionResult{session: m.session, generation: m.composer.generation, err: &auth.APIError{Status: 422, BodyErrors: []string{"Body rejected\x1b[2J"}}})
	m = next.(AppModel)
	if m.composer.uncertain || m.composer.input.Value() == "" || strings.Contains(m.composer.status, "\x1b") {
		t.Fatal("validation error mishandled")
	}
	for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}, {30, 10}} {
		next, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(AppModel)
		frame := m.View().Content
		for _, line := range strings.Split(frame, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow at %v", size)
			}
		}
		if size[1] >= 18 && !strings.Contains(ansi.Strip(frame), "Submit") {
			t.Fatalf("submit hidden at %v", size)
		}
	}
}

func TestComposerServerFailuresKeepDraftAndExplainRecovery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		want      string
		uncertain bool
	}{
		{"rules", &auth.APIError{Status: 403, Code: "commit_rules_required"}, "First commit required on the web", false},
		{"rate", &auth.APIError{Status: 429, RetryAfter: "30"}, "Rate limited", false},
		{"server", &auth.APIError{Status: 503}, "Outcome unknown", true},
		{"bad response", errors.New("server returned an invalid submission response"), "Outcome unknown", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := fixtureApp(t)
			m = pressApp(m, "c")
			m.composer.input.SetValue("Synthetic private draft")
			m.submit = func(context.Context, api.NewCommit) (api.Commit, error) { return api.Commit{}, tc.err }
			cmd := m.startSubmission()
			next, _ := m.Update(cmd())
			m = next.(AppModel)
			if !strings.Contains(m.composer.status, tc.want) || m.composer.uncertain != tc.uncertain || m.composer.input.Value() != "Synthetic private draft" {
				t.Fatalf("recovery: %+v", m.composer)
			}
		})
	}
}

func TestRevokedSubmissionRetainsDraftAndProtectsExitOnLogin(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic retained draft")
	m.submit = func(context.Context, api.NewCommit) (api.Commit, error) {
		return api.Commit{}, &auth.APIError{Status: 401}
	}
	cmd := m.startSubmission()
	next, _ := m.Update(cmd())
	m = next.(AppModel)
	if m.connected || m.composer.input.Value() == "" {
		t.Fatal("revocation lost draft or kept invalid connection")
	}
	m = pressApp(m, "ctrl+c")
	if !strings.Contains(m.View().Content, "Exit and lose") {
		t.Fatal("exit confirmation hidden by login")
	}
	m = pressApp(m, "enter")
	if m.composer.confirm != "" || m.composer.input.Value() == "" {
		t.Fatal("default confirmation discarded draft")
	}
}

func TestComposerModifierShortcutsAndSendingInputOwnership(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic draft")
	m.submit = func(context.Context, api.NewCommit) (api.Commit, error) { return api.Commit{}, errors.New("offline") }
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	m = next.(AppModel)
	if cmd == nil || !m.composer.sending {
		t.Fatal("distinguished Ctrl+Enter did not submit")
	}
	m = pressApp(m, "a")
	next, _ = m.Update(tea.PasteMsg{Content: "must not alter submitted draft"})
	m = next.(AppModel)
	if m.composer.input.Value() != "Synthetic draft" || m.accountOpen {
		t.Fatal("in-flight input changed draft or navigated")
	}
	m = pressApp(m, "esc")
	if m.composer.confirm != "" || !m.composer.sending {
		t.Fatal("escape canceled uncertain in-flight write")
	}
	m = pressApp(m, "ctrl+c")
	if m.composer.confirm != "exit" {
		t.Fatal("in-flight exit unprotected")
	}
}

func TestComposerPreviewStaleResultsAndCancelConfirmation(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Old synthetic draft")
	m.preview = func(context.Context, string) (api.Preview, error) {
		return api.Preview{Body: "old", Segments: []api.Segment{{Type: "text", Text: "old"}}}, nil
	}
	cmd := m.startPreview()
	m = pressApp(m, "x")
	next, _ := m.Update(cmd())
	m = next.(AppModel)
	if m.composer.previewing || m.composer.loadingPreview {
		t.Fatal("stale preview replaced edited draft")
	}
	m = pressApp(m, "esc")
	m = pressApp(m, "tab")
	m = pressApp(m, "enter")
	if m.composer.open || m.composer.input.Value() != "" {
		t.Fatal("explicit discard did not clear draft")
	}
}

func TestReconnectionDoesNotExposeOrMutateAnotherOwnersDraft(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Original private draft")
	next, _ := m.Update(logResult{index: 0, generation: m.feeds[0].generation, err: &auth.APIError{Status: 401}})
	m = next.(AppModel)
	next, _ = m.Update(accountResult{generation: m.account.generation, user: auth.User{ID: "other", Handle: "other"}})
	m = next.(AppModel)
	m = pressApp(m, "c")
	if m.composer.input.Focused() {
		t.Error("another account focused the retained editor")
	}
	next, _ = m.Update(tea.PasteMsg{Content: "other account paste"})
	m = next.(AppModel)
	m = pressApp(m, "j")
	if m.composer.input.Value() != "Original private draft" || strings.Contains(m.View().Content, "Original private draft") {
		t.Fatal("another account changed or saw retained draft")
	}
	m = pressApp(m, "enter")
	if m.composer.input.Focused() {
		t.Fatal("keeping another owner's draft focused its editor")
	}
}

func TestSmallTerminalStillShowsDraftConfirmations(t *testing.T) {
	for _, action := range []string{"discard", "exit"} {
		for _, size := range [][2]int{{80, 17}, {30, 9}, {25, 8}} {
			m := fixtureApp(t)
			m = pressApp(m, "c")
			m.composer.input.SetValue("Keep me")
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(AppModel)
			key := "esc"
			if action == "exit" {
				key = "ctrl+c"
			}
			m = pressApp(m, key)
			frame := ansi.Strip(m.View().Content)
			if !strings.Contains(frame, "Keep draft") {
				t.Fatalf("%s confirmation hidden at %v: %s", action, size, frame)
			}
			for _, line := range strings.Split(frame, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("confirmation overflow")
				}
			}
			if len(strings.Split(frame, "\n")) > size[1] {
				t.Fatal("confirmation height overflow")
			}
			m = pressApp(m, "enter")
			if m.composer.input.Value() != "Keep me" || m.composer.confirm != "" {
				t.Fatal("default confirmation discarded draft")
			}
		}
	}
}

func TestPreviewRevocationRecoversDraftAndRejectsOldResult(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Retained draft")
	m.preview = func(context.Context, string) (api.Preview, error) { return api.Preview{}, &auth.APIError{Status: 401} }
	cmd := m.startPreview()
	old := cmd().(previewResult)
	next, _ := m.Update(old)
	m = next.(AppModel)
	if m.connected || m.composer.input.Value() != "Retained draft" {
		t.Fatal("preview revocation did not keep draft for reconnection")
	}
	key, _ := auth.GenerateKey()
	next, _ = m.Update(accountResult{generation: m.account.generation, key: key, user: auth.User{ID: "fixture", Handle: "fixture"}})
	m = next.(AppModel)
	m = pressApp(m, "c")
	old.err = nil
	old.preview = api.Preview{Body: "stale"}
	next, _ = m.Update(old)
	m = next.(AppModel)
	if !m.connected || m.composer.input.Value() != "Retained draft" || m.composer.previewing {
		t.Fatal("reconnection lost draft or accepted old preview")
	}
}

func TestSuccessfulSubmissionSurvivesOlderInitialLogResult(t *testing.T) {
	for _, returned := range []bool{false, true} {
		m := fixtureApp(t)
		m = applyPage(m, 1, api.Page{Data: []api.Commit{fixtureCommit("other"), fixtureCommit("anchor")}, Next: "other-cursor"})
		m.feeds[1].selected = 1
		old := logResult{index: 0, generation: m.feeds[0].generation, mode: "initial", page: api.Page{Data: []api.Commit{fixtureCommit("old")}, Next: "older-cursor"}}
		m = pressApp(m, "c")
		m.composer.input.SetValue("Synthetic new")
		m.submit = func(context.Context, api.NewCommit) (api.Commit, error) {
			c := fixtureCommit("new")
			c.Status = "pending"
			return c, nil
		}
		cmd := m.startSubmission()
		next, _ := m.Update(cmd())
		m = next.(AppModel)
		if len(m.feeds[0].commits) == 0 || !m.feeds[0].loaded {
			t.Fatal("submitted receipt not visible during initial load")
		}
		if returned {
			old.page.Data = append([]api.Commit{fixtureCommit("new")}, old.page.Data...)
		}
		next, _ = m.Update(old)
		m = next.(AppModel)
		if len(m.feeds[0].commits) != 2 || m.feeds[0].commits[0].ID != "new" || m.feeds[0].next != "older-cursor" {
			t.Fatal("initial result lost submission or pagination")
		}
		if returned && m.feeds[0].commits[0].Status != "approved" {
			t.Fatal("local receipt overwrote fresher server moderation")
		}
		if m.feeds[1].selected != 1 || m.feeds[1].next != "other-cursor" {
			t.Fatal("unrelated log changed")
		}
	}
}

func TestReturningToSubmittedLogResumesItsCanceledInitialLoad(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 1, api.Page{Data: []api.Commit{fixtureCommit("other")}})
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic new")
	m.submit = func(context.Context, api.NewCommit) (api.Commit, error) {
		c := fixtureCommit("new")
		c.Status = "pending"
		return c, nil
	}
	cmd := m.startSubmission()
	next, _ := m.Update(cmd())
	m = next.(AppModel)
	m, _ = m.switchLog(1)
	m, cmd = m.switchLog(0)
	if cmd == nil || !m.feeds[0].loading || m.feeds[0].commits[0].ID != "new" {
		t.Fatal("returning did not resume initial load alongside submitted receipt")
	}
}
