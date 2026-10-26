package tui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func clickText(t *testing.T, m AppModel, label string) (AppModel, tea.Cmd) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if pos := strings.Index(line, label); pos >= 0 {
			next, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: ansi.StringWidth(line[:pos]) + 1, Y: y})
			return next.(AppModel), cmd
		}
	}
	t.Fatalf("visible control %q not found", label)
	return m, nil
}

func TestComposerModalPreservesTheReaderViewport(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("c")}})
	m, _ = m.selectCommit(1)
	offset, height, selected := m.feeds[0].viewport.YOffset(), m.feeds[0].viewport.Height(), m.feeds[0].selected
	m = pressApp(m, "c")
	if m.feeds[0].viewport.Height() != height || m.feeds[0].viewport.YOffset() != offset || m.feeds[0].selected != selected {
		t.Fatal("opening modal moved or resized the reader")
	}
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "╭") || !strings.Contains(frame, "Following") || !strings.Contains(frame, "New commit") {
		t.Fatal("composer did not overlay the retained shell")
	}
	m = pressApp(m, "esc")
	if m.composer.open || m.feeds[0].viewport.YOffset() != offset {
		t.Fatal("closing empty modal lost reading position")
	}
}

func TestMouseOwnsComposerControlsAndCannotClickThrough(t *testing.T) {
	m := fixtureApp(t)
	m, _ = clickText(t, m, "c Commit")
	if !m.composer.open {
		t.Fatal("compose prompt did not open modal")
	}
	m.composer.input.SetValue("Synthetic draft")
	selected, active := m.feeds[0].selected, m.active
	next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 1})
	m = next.(AppModel)
	if !m.composer.open || m.active != active || m.feeds[0].selected != selected {
		t.Fatal("outside click escaped modal")
	}
	m, _ = clickText(t, m, "Cancel")
	if m.composer.confirm != "discard" {
		t.Fatal("mouse cancel bypassed draft protection")
	}
	m, _ = clickText(t, m, "Keep draft")
	if m.composer.confirm != "" || m.composer.input.Value() != "Synthetic draft" {
		t.Fatal("keep action lost draft")
	}
	m = pressApp(m, "c")
	m, _ = clickText(t, m, "Cancel")
	m, _ = clickText(t, m, "Discard draft")
	if m.composer.open || m.composer.input.Value() != "" {
		t.Fatal("explicit mouse discard did not close modal")
	}
}

func TestMouseEditorMapsWideCharactersAndDragSelection(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("界abc")
	r := m.composerEditorBounds()
	next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + 2, Y: r.y})
	m = next.(AppModel)
	next, _ = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: r.x + 2, Y: r.y})
	m = next.(AppModel)
	m = pressApp(m, "x")
	if m.composer.input.Value() != "界xabc" {
		t.Fatalf("mouse caret ignored display-cell width: %q", m.composer.input.Value())
	}
	m.composer.input.SetValue("Hello world")
	next, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x, Y: r.y})
	m = next.(AppModel)
	next, _ = m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: r.x + 5, Y: r.y})
	m = next.(AppModel)
	next, _ = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: r.x + 5, Y: r.y})
	m = next.(AppModel)
	if m.composer.input.SelectedText() != "Hello" {
		t.Fatalf("drag selection: %q", m.composer.input.SelectedText())
	}
	m = pressApp(m, "X")
	if m.composer.input.Value() != "X world" {
		t.Fatal("typing did not replace mouse selection")
	}
}

func TestMouseSubmitIsSingleFlightAndShowsModerationToast(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic draft")
	calls := 0
	m.submit = func(context.Context, string) (api.Commit, error) {
		calls++
		c := fixtureCommit("posted")
		c.Status = "pending"
		return c, nil
	}
	m, cmd := clickText(t, m, "Submit")
	if cmd == nil || !m.composer.sending {
		t.Fatal("mouse submit did not start")
	}
	m, duplicate := clickText(t, m, "Sending...")
	if duplicate != nil {
		t.Fatal("second click queued another submission")
	}
	next, _ := m.Update(cmd())
	m = next.(AppModel)
	if calls != 1 || m.composer.open || !m.toast.visible || m.toast.title != "Submitted for review" || !strings.Contains(m.toast.detail, "not public") {
		t.Fatal("submission toast lost single-flight or moderation status")
	}
}

func TestToastExpiryCannotDismissANewerNoticeAndErrorsKeepDraft(t *testing.T) {
	m := fixtureApp(t)
	m.notify("success", "First", "Old result")
	old := toastExpired{id: m.toast.id, session: m.session}
	m.notify("error", "Second", "New result")
	next, _ := m.Update(old)
	m = next.(AppModel)
	if !m.toast.visible || m.toast.title != "Second" {
		t.Fatal("old expiry dismissed a newer toast")
	}
	next, _ = m.Update(toastExpired{id: m.toast.id, session: m.session})
	m = next.(AppModel)
	if m.toast.visible {
		t.Fatal("current toast did not expire")
	}
	m = pressApp(m, "c")
	m.composer.input.SetValue("Retain this draft")
	m.submit = func(context.Context, string) (api.Commit, error) { return api.Commit{}, errors.New("lost connection") }
	cmd := m.startSubmission()
	next, _ = m.Update(cmd())
	m = next.(AppModel)
	if !m.toast.visible || m.toast.kind != "error" || m.composer.input.Value() != "Retain this draft" || !m.composer.uncertain {
		t.Fatal("failure toast lost draft or outcome uncertainty")
	}
	next, _ = m.Update(toastExpired{id: m.toast.id, session: m.session})
	m = next.(AppModel)
	if m.composer.status == "" || m.composer.input.Value() == "" {
		t.Fatal("toast expiry erased persistent recovery context")
	}
}

func TestMouseAccountNavigationAndActionFocus(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 35
	m.reflow()
	m, _ = clickText(t, m, "Account")
	if !m.accountOpen {
		t.Fatal("sidebar account click ignored")
	}
	m, _ = clickText(t, m, "Device details")
	if !m.account.showDeviceDetails {
		t.Fatal("account action click ignored")
	}
}

func TestModalControlsRemainVisibleAtSupportedSizes(t *testing.T) {
	for _, size := range [][2]int{{48, 18}, {60, 24}, {80, 24}, {120, 35}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := fixtureApp(t)
			m.account.width, m.account.height = size[0], size[1]
			m.reflow()
			m = pressApp(m, "c")
			m.composer.input.SetValue("Synthetic draft")
			m, _ = clickText(t, m, "Cancel")
			if m.composer.confirm != "discard" {
				t.Fatal("cancel click missed")
			}
			m, _ = clickText(t, m, "Keep draft")
			if m.composer.confirm != "" {
				t.Fatal("keep click missed")
			}
			m = pressApp(m, "c")
			m, _ = clickText(t, m, "Cancel")
			m, _ = clickText(t, m, "Discard draft")
			if m.composer.open {
				t.Fatal("discard click missed")
			}
		})
	}
}

func TestMouseToastDismissalOwnsItsCells(t *testing.T) {
	m := fixtureApp(t)
	m.notify("success", "Commit submitted", "Synthetic receipt")
	_, r := m.toastView()
	active := m.active
	next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + 2, Y: r.y + 1})
	m = next.(AppModel)
	if !m.toast.visible || m.active != active {
		t.Fatal("toast body click leaked to log tabs")
	}
	next, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + r.w - 3, Y: r.y + 1})
	m = next.(AppModel)
	if m.toast.visible || m.active != active {
		t.Fatal("toast close did not exclusively dismiss it")
	}
}

func TestMouseWheelAndEmojiStayInsideComposer(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue(strings.Repeat("line\n", 30))
	m.updateComposerInput(nil)
	r := m.composerEditorBounds()
	before := m.composer.input.ScrollYOffset()
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 0, Y: 0})
	m = next.(AppModel)
	if m.composer.input.ScrollYOffset() != before {
		t.Fatal("outside wheel moved editor")
	}
	for range 20 {
		next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: r.x, Y: r.y})
		m = next.(AppModel)
	}
	if m.composer.input.ScrollYOffset() >= before {
		t.Fatal("editor wheel did not reveal earlier lines")
	}
	m.composer.input.SetValue(":smi")
	m.refreshEmoji()
	if len(m.composer.emojis) < 2 {
		t.Fatal("missing emoji fixture")
	}
	want := m.composer.emojis[1].value
	next, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + 2, Y: r.y + r.h + 2})
	m = next.(AppModel)
	if m.composer.input.Value() != want {
		t.Fatalf("emoji click inserted %q, want %q", m.composer.input.Value(), want)
	}
}

func TestMouseHelpAndAccountFooter(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 35
	m.reflow()
	m, _ = clickText(t, m, "help")
	if !m.help {
		t.Fatal("help click ignored")
	}
	r := m.helpBounds()
	next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + r.w - 3, Y: r.y + 1})
	m = next.(AppModel)
	if m.help {
		t.Fatal("help close click ignored")
	}
	m = pressApp(m, "a")
	m, _ = clickText(t, m, "esc")
	if m.accountOpen {
		t.Fatal("account back click ignored")
	}
}

func TestCompactAccountButtonsFollowRenderedRows(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 18}, {120, 35}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := fixtureApp(t)
			m.account.width, m.account.height = size[0], size[1]
			m.reflow()
			m = pressApp(m, "a")
			m, _ = clickText(t, m, "Disconnect")
			if m.account.state != "confirm" {
				t.Fatal("disconnect click missed rendered button")
			}
			m, _ = clickText(t, m, "Keep connection")
			if m.account.state != "connected" {
				t.Fatal("wrapped keep button click missed")
			}
		})
	}
}

func TestComposerFooterActionsStayClickableWithEmojiSuggestions(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue(":smi")
	m.refreshEmoji()
	m, _ = clickText(t, m, "Cancel")
	if m.composer.confirm != "discard" || m.composer.input.Value() != ":smi" {
		t.Fatal("emoji suggestions interfered with footer cancel")
	}
}

func TestWrappedManualLinkBackUsesRenderedHeight(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 60, 24
	m.reflow()
	m.manualLink = "https://example.test/commit/12345678-1234-1234-1234-123456789012"
	m, _ = clickText(t, m, "Back to log")
	if m.manualLink != "" {
		t.Fatal("wrapped URL moved Back out of its hitbox")
	}
}

func TestAsideHelpChipAcceptsClick(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 45
	m.reflow()
	m, _ = clickText(t, m, "help")
	if !m.help {
		t.Fatal("aside help control click ignored")
	}
}

func TestRefreshReportsProgressAndOutcomeInToasts(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			m := fixtureApp(t)
			m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
			m, _ = m.selectCommit(1)
			m.fetch = func(context.Context, api.Log, string, string) (api.Page, error) {
				if fail {
					return api.Page{}, errors.New("offline")
				}
				return api.Page{Data: []api.Commit{fixtureCommit("new")}}, nil
			}
			m, cmd := m.handleKey("ctrl+r")
			if !m.toast.visible || m.toast.title != "Refreshing log" || strings.Contains(m.workspaceFooter(120), "Refreshing") {
				t.Fatal("refresh progress belongs in a toast")
			}
			next, _ := m.Update(logResponse(t, cmd))
			m = next.(AppModel)
			want := "success"
			if fail {
				want = "error"
				if m.feeds[0].commits[m.feeds[0].selected].ID != "b" {
					t.Fatal("refresh failure moved selection")
				}
			}
			if !m.toast.visible || m.toast.kind != want {
				t.Fatal("refresh outcome toast missing")
			}
			if strings.Contains(m.workspaceFooter(120), "offline") {
				t.Fatal("refresh failure leaked into footer")
			}
		})
	}
}

func TestToastInteriorHasOneBackground(t *testing.T) {
	for _, dark := range []bool{true, false} {
		m := fixtureApp(t)
		m.account.dark = dark
		m.notify("info", "Refreshing log", "Checking Following for the latest commits.")
		view, r := m.toastView()
		buffer := uv.NewScreenBuffer(r.w, r.h)
		lipgloss.NewLayer(view).Draw(&buffer, buffer.Bounds())
		background := colors(dark).shell
		want := color.NRGBAModel.Convert(lipgloss.Color(background))
		for y := 1; y < r.h-1; y++ {
			for x := 1; x < r.w-1; x++ {
				cell := buffer.CellAt(x, y)
				if cell == nil || cell.Style.Bg == nil || color.NRGBAModel.Convert(cell.Style.Bg) != want {
					t.Fatalf("toast interior background differs at %d,%d", x, y)
				}
			}
		}
	}
}

func TestToastShadowIsOnlyAThinEdge(t *testing.T) {
	m := fixtureApp(t)
	m.notify("success", "Log refreshed", "Following is up to date.")
	view, r := m.toastView()
	base := strings.Repeat(strings.Repeat(".", m.account.width)+"\n", m.account.height-1) + strings.Repeat(".", m.account.width)
	frame := m.overlayToast(base, view, r)
	lines := strings.Split(ansi.Strip(frame), "\n")
	if !strings.Contains(lines[r.y+r.h], "▀") {
		t.Fatal("toast needs a half-cell bottom shadow rather than a solid block")
	}
	if lines[r.y+r.h+1] != strings.Repeat(".", m.account.width) {
		t.Fatal("toast shadow extends too far down")
	}
}

func TestToastsAutoDismissAfterThreeSeconds(t *testing.T) {
	for _, kind := range []string{"info", "success", "error"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			m := fixtureApp(t)
			start := time.Now()
			msg := m.notify(kind, "Notice", "Synthetic message")()
			elapsed := time.Since(start)
			if elapsed < 3*time.Second || elapsed > 5*time.Second {
				t.Fatalf("toast expiry took %s, want 3 seconds", elapsed)
			}
			next, _ := m.Update(msg)
			if next.(AppModel).toast.visible {
				t.Fatal("scheduled expiry did not dismiss toast")
			}
		})
	}
}
