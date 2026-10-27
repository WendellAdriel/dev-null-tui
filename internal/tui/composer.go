package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type composer struct {
	input                                                textarea.Model
	open, sending, previewing, loadingPreview, uncertain bool
	focus, generation, log                               int
	previewOffset                                        int
	status, confirm, owner, mode                         string
	target                                               api.Commit
	confirmFocus                                         int
	dragging                                             bool
	preview                                              api.Preview
	cancel                                               context.CancelFunc
	emojis                                               []emojiSuggestion
	emojiIndex                                           int
	emojiQuery, emojiPrefix, emojiDismissed              string
}

type submissionResult struct {
	session, generation int
	commit              api.Commit
	err                 error
}

type previewResult struct {
	session, generation int
	body                string
	preview             api.Preview
	err                 error
}

func (m *AppModel) openComposer() tea.Cmd {
	if m.composer.owner == "" {
		input := textarea.New()
		input.Prompt, input.Placeholder = "", "What did you ship?"
		input.ShowLineNumbers = false
		input.CharLimit = 0
		input.MaxHeight, input.MaxWidth = 0, 0
		m.composer = composer{input: input, owner: m.account.user.ID, log: m.active, generation: m.composer.generation + 1}
	}
	m.composer.open = true
	m.composer.focus = 0
	m.resetSequence()
	m.reflow()
	if m.composer.owner != m.account.user.ID {
		m.composer.input.Blur()
		m.composer.status = "Reconnect as the original account to edit this draft, or cancel to discard it."
		return nil
	}
	return m.focusComposer()
}

func (m *AppModel) composeTo(mode string, target api.Commit) tea.Cmd {
	if mode != "" && (target.DeletedAt != nil || target.Author == nil) {
		return nil
	}
	c := &m.composer
	if c.owner != "" && c.input.Value() != "" {
		cmd := m.openComposer()
		if m.composer.mode != mode || m.composer.target.ID != target.ID {
			m.composer.status = "Finish or discard this draft before starting another."
		}
		return cmd
	}
	cmd := m.openComposer()
	m.composer.mode, m.composer.target = mode, target
	m.sizeComposer()
	return cmd
}

func (m *AppModel) focusComposer() tea.Cmd {
	c := &m.composer
	if c.owner != m.account.user.ID || c.focus != 0 || c.previewing || c.sending {
		c.input.Blur()
		return nil
	}
	return c.input.Focus()
}

func (m *AppModel) clearComposer() {
	if m.composer.cancel != nil {
		m.composer.cancel()
	}
	m.composer = composer{generation: m.composer.generation + 1}
	m.reflow()
}

func (m *AppModel) requestDraftAction(action string) tea.Cmd {
	c := &m.composer
	if c.input.Value() == "" && !c.sending {
		m.clearComposer()
		if action == "exit" {
			m.account.life.cancel()
			return tea.Quit
		}
		return nil
	}
	c.confirm, c.confirmFocus = action, 0
	c.input.Blur()
	return nil
}

func (m *AppModel) composerKey(msg tea.KeyPressMsg) tea.Cmd {
	c := &m.composer
	pressed := msg.String()
	if c.confirm != "" {
		switch pressed {
		case "tab", "right":
			c.confirmFocus = (c.confirmFocus + 1) % 3
		case "shift+tab", "left":
			c.confirmFocus = (c.confirmFocus + 2) % 3
		case "esc":
			c.confirm = ""
			c.confirmFocus = 0
			return m.focusComposer()
		case "ctrl+k":
			return m.keepDraft()
		case "ctrl+d":
			if c.confirm != "retry" {
				return m.confirmDraftAction()
			}
		case "ctrl+s":
			if c.confirm == "retry" {
				return m.confirmDraftAction()
			}
		case "enter":
			switch c.confirmFocus {
			case 0:
				return m.keepDraft()
			case 1:
				return m.confirmDraftAction()
			case 2:
				c.confirm = ""
				c.confirmFocus = 0
				return m.focusComposer()
			}
		}
		return nil
	}
	if pressed == "ctrl+c" {
		return m.requestDraftAction("exit")
	}
	if pressed == "esc" {
		if len(c.emojis) > 0 {
			c.emojiDismissed = c.emojiPrefix
			c.emojis = nil
			return nil
		}
		if c.previewing {
			c.previewing = false
			c.focus = 0
			return m.focusComposer()
		}
		if c.sending {
			c.status = "Submission in progress. Wait for its result before closing."
			return nil
		}
		return m.requestDraftAction("discard")
	}
	if c.sending {
		return nil
	}
	if c.owner != m.account.user.ID {
		return m.requestDraftAction("discard")
	}
	if len(c.emojis) > 0 && c.focus == 0 {
		switch pressed {
		case "down":
			c.emojiIndex = (c.emojiIndex + 1) % len(c.emojis)
			return nil
		case "up":
			c.emojiIndex = (c.emojiIndex + len(c.emojis) - 1) % len(c.emojis)
			return nil
		case "tab", "enter":
			m.acceptEmoji()
			return nil
		}
	}
	if c.previewing {
		switch pressed {
		case "j", "down":
			c.previewOffset++
			return nil
		case "k", "up":
			c.previewOffset = max(0, c.previewOffset-1)
			return nil
		case "pgdown":
			c.previewOffset += c.input.Height()
			return nil
		case "pgup":
			c.previewOffset = max(0, c.previewOffset-c.input.Height())
			return nil
		}
	}
	switch pressed {
	case "ctrl+enter", "ctrl+kpenter", "ctrl+s":
		return m.startSubmission()
	case "ctrl+p", "ctrl+shift+p":
		return m.startPreview()
	case "tab", "shift+tab":
		delta := 1
		if pressed == "shift+tab" {
			delta = 3
		}
		c.focus = (c.focus + delta) % 4
		c.emojis = nil
		c.input.Blur()
		if c.focus == 0 && !c.previewing {
			return m.focusComposer()
		}
		return nil
	case "enter":
		switch c.focus {
		case 1:
			return m.startPreview()
		case 2:
			return m.startSubmission()
		case 3:
			return m.requestDraftAction("discard")
		}
	}
	if c.focus != 0 || c.previewing {
		return nil
	}
	return m.updateComposerInput(msg)
}

func (m *AppModel) keepDraft() tea.Cmd {
	c := &m.composer
	if c.loadingPreview {
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
		c.generation++
		c.loadingPreview = false
		c.status = ""
	}
	c.open, c.dragging = false, false
	c.confirm, c.confirmFocus = "", 0
	c.emojis = nil
	c.input.EndSelection()
	c.input.Blur()
	m.reflow()
	return nil
}

func (m *AppModel) confirmDraftAction() tea.Cmd {
	c := &m.composer
	action := c.confirm
	c.confirm, c.confirmFocus = "", 0
	if action == "retry" {
		c.uncertain = false
		return m.startSubmission()
	}
	m.clearComposer()
	if action == "exit" {
		m.account.life.cancel()
		return tea.Quit
	}
	return nil
}

func (m *AppModel) updateComposerInput(msg tea.Msg) tea.Cmd {
	c := &m.composer
	if c.owner != m.account.user.ID {
		return nil
	}
	before := c.input.Value()
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	if c.input.Value() != before {
		if !c.input.HasSelection() {
			name, closed := emojiAtCaret(c.beforeCaret())
			if value, ok := emojiCatalog[name]; closed && ok {
				c.replaceEmoji(name, value, true)
			}
		}
		m.invalidatePreview()
	}
	m.refreshEmoji()
	return cmd
}

func (m *AppModel) invalidatePreview() {
	c := &m.composer
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.generation++
	c.loadingPreview, c.previewing = false, false
	c.preview = api.Preview{}
	c.status = ""
}

func (m *AppModel) startSubmission() tea.Cmd {
	c := &m.composer
	if c.sending || !c.open || c.owner != m.account.user.ID {
		return nil
	}
	if err := api.ValidateBody(c.input.Value()); err != nil {
		c.status = err.Error()
		return m.notify("error", "Cannot submit", c.status)
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
	c.emojis = nil
	c.status = "Signing and submitting..."
	c.input.Blur()
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	c.cancel = cancel
	draft := api.NewCommit{Body: c.input.Value()}
	switch c.mode {
	case "reply":
		draft.ParentID = c.target.ID
	case "quote":
		draft.QuotedID = c.target.ID
	}
	session, generation, submit := m.session, c.generation, m.submit
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		commit, err := submit(ctx, draft)
		return submissionResult{session: session, generation: generation, commit: commit, err: err}
	})
}

func (m *AppModel) startPreview() tea.Cmd {
	c := &m.composer
	if c.sending || c.loadingPreview || c.owner != m.account.user.ID {
		return nil
	}
	if c.previewing {
		c.previewing = false
		c.focus = 0
		return m.focusComposer()
	}
	if err := api.ValidateBody(c.input.Value()); err != nil {
		c.status = err.Error()
		return m.notify("error", "Cannot preview", c.status)
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.generation++
	c.loadingPreview = true
	c.emojis = nil
	c.status = "Preparing server preview..."
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	c.cancel = cancel
	session, generation, body, preview := m.session, c.generation, c.input.Value(), m.preview
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		result, err := preview(ctx, body)
		return previewResult{session, generation, body, result, err}
	})
}

func (m *AppModel) acceptPreview(result previewResult) tea.Cmd {
	c := &m.composer
	if result.session != m.session || result.generation != c.generation || !c.open || !c.loadingPreview || result.body != c.input.Value() {
		return nil
	}
	c.loadingPreview, c.cancel = false, nil
	if result.err != nil {
		c.status = m.composerError(result.err)
		m.reconnectComposer(result.err)
		return m.notify("error", "Preview failed", c.status)
	}
	c.preview, c.previewing, c.previewOffset = result.preview, true, 0
	c.status = "Server preview. Nothing has been published."
	c.input.Blur()
	return nil
}

func (m *AppModel) acceptSubmission(result submissionResult) tea.Cmd {
	c := &m.composer
	if result.session != m.session || result.generation != c.generation || !c.sending {
		return nil
	}
	c.sending, c.cancel = false, nil
	if result.err != nil {
		c.focus, c.previewing = 0, false
		var failure *auth.APIError
		c.uncertain = !errors.As(result.err, &failure) || failure.Status >= 500 || failure.Status == http.StatusRequestTimeout
		kind := "error"
		c.status = m.composerError(result.err)
		if c.uncertain {
			kind, c.status = "warning", "Outcome unknown. Check the web before retrying to avoid duplicates. Your draft is kept."
		}
		if m.reconnectComposer(result.err) {
			return m.notify(kind, "Submission failed", c.status)
		}
		return tea.Batch(m.focusComposer(), m.notify(kind, "Submission failed", c.status))
	}
	commit := result.commit
	if commit.Status == "rejected" {
		c.focus, c.previewing = 0, false
		c.status = "Rejected. Not public. Your draft is kept for editing."
		return tea.Batch(m.focusComposer(), m.notify("error", "Commit rejected", c.status))
	}
	m.addToThreads(c.mode, c.target.ID, commit)
	index := c.log
	if c.mode != "reply" && index != 2 {
		f := &m.feeds[index]
		if f.loading && f.loadingMode == "initial" {
			f.submitted = append([]api.Commit{commit}, f.submitted...)
		} else {
			if f.cancel != nil {
				f.cancel()
				f.cancel = nil
			}
			f.generation++
			f.loading = false
		}
		f.loaded = true
		f.commits = uniqueCommits(append([]api.Commit{commit}, f.commits...))
		f.selected = 0
		m.renderFeed(index)
		f.viewport.GotoTop()
	}
	m.notice = "Submitted (" + commit.Status + ")."
	if c.mode == "reply" {
		m.notice = "Reply submitted (" + commit.Status + ")."
	}
	if commit.Status == "pending" {
		m.notice += " Awaiting review, not public yet."
	}
	if index == 2 && c.mode != "reply" {
		m.notice += " Trending stays ranked. Refresh to check for it."
	}
	m.clearComposer()
	if commit.Status == "pending" {
		return m.notify("success", "Submitted for review", "Your commit is awaiting review, not public yet.")
	}
	return m.notify("success", "Commit submitted", m.notice)
}

func (m *AppModel) reconnectComposer(err error) bool {
	var failure *auth.APIError
	if !errors.As(err, &failure) || failure.Status != http.StatusUnauthorized {
		return false
	}
	m.leaveApplication()
	m.account.state, m.account.status = "failed", "Device approval is invalid. Reconnect to recover your in-memory draft."
	m.account.approvalRequired = true
	return true
}

func (m AppModel) composerError(err error) string {
	var failure *auth.APIError
	if errors.As(err, &failure) {
		if failure.Code == "commit_rules_required" {
			return "First commit required on the web. Read and accept the commit rules at " + m.account.origin + "/. Your draft is kept."
		}
		if failure.Status == http.StatusUnauthorized {
			return "Device approval is invalid. Your draft is kept. Reconnect before submitting."
		}
		if failure.Status == http.StatusNotFound {
			return "That commit is no longer available. Your draft is kept."
		}
		if failure.Status == http.StatusUnprocessableEntity && len(failure.BodyErrors) > 0 {
			return safeText(strings.Join(failure.BodyErrors, " "))
		}
		if failure.Status == http.StatusTooManyRequests {
			return fmt.Sprintf("Rate limited. Wait before submitting again. Retry after %s. Draft kept.", safeText(failure.RetryAfter))
		}
	}
	return safeText(err.Error()) + ". Draft kept."
}
