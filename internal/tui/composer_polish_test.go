package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func chord(m AppModel, code rune, mod tea.KeyMod) (AppModel, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return next.(AppModel), cmd
}

func TestComposerPreviewAndSubmitShortcutsAreVisibleAndWork(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Synthetic draft")
	m.preview = func(context.Context, string) (api.Preview, error) {
		return api.Preview{Body: "Synthetic draft", Segments: []api.Segment{{Type: "text", Text: "Synthetic draft"}}}, nil
	}
	m, cmd := chord(m, 'p', tea.ModCtrl)
	if cmd == nil || !m.composer.loadingPreview {
		t.Fatal("Ctrl+P did not preview")
	}
	next, _ := m.Update(cmd())
	m = next.(AppModel)
	m, _ = chord(m, 'p', tea.ModCtrl)
	if m.composer.previewing {
		t.Fatal("Ctrl+P did not return to editor")
	}
	for _, key := range []struct {
		code rune
		mod  tea.KeyMod
	}{{tea.KeyEnter, tea.ModCtrl}, {'s', tea.ModCtrl}, {tea.KeyKpEnter, tea.ModCtrl}} {
		m.composer.sending = false
		m, cmd = chord(m, key.code, key.mod)
		if cmd == nil || !m.composer.sending {
			t.Fatalf("submit shortcut not handled: %+v", key)
		}
	}
	for _, hint := range []string{"ctrl+enter", "ctrl+p", "ctrl+s"} {
		if !strings.Contains(ansi.Strip(m.View().Content), hint) {
			t.Fatalf("shortcut %s hidden", hint)
		}
	}
}

func TestThemeTogglePreservesDraftAndOverridesLateBackground(t *testing.T) {
	m := fixtureApp(t)
	m.account.theme = "auto"
	m = pressApp(m, "c")
	m.composer.input.SetValue("Draft :roc")
	before := m.composer.input.Value()
	m, _ = chord(m, tea.KeyF2, 0)
	if m.account.dark || m.account.theme != "light" || m.composer.input.Value() != before || !m.composer.open {
		t.Fatal("theme toggle lost draft or did not apply")
	}
	next, _ := m.Update(tea.BackgroundColorMsg{})
	m = next.(AppModel)
	if m.account.dark {
		t.Fatal("late background detection overwrote explicit theme")
	}
	m, _ = chord(m, tea.KeyF2, 0)
	if !m.account.dark || m.account.theme != "dark" {
		t.Fatal("second toggle did not return to dark")
	}
	account := NewAccount("dark", auth.Store{Dir: t.TempDir()}, "https://example.test", t.Context())
	defer func() { _ = account.Close() }()
	updated, _ := account.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	account = updated.(AccountModel)
	if account.dark || !strings.Contains(ansi.Strip(account.View().Content), "f2") {
		t.Fatal("login theme control missing")
	}
}

func TestEmojiSuggestionsAcceptAtCaretWithoutChangingSurroundingText(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("界 :roc tail")
	m.composer.input.SetCursorColumn(6)
	m = pressApp(m, "x")
	m, _ = chord(m, tea.KeyBackspace, 0)
	if len(m.composer.emojis) < 2 || m.composer.emojis[0].name != "rock" {
		t.Fatal("emoji suggestions missing or incorrectly ranked")
	}
	m, _ = chord(m, tea.KeyDown, 0)
	if !strings.Contains(ansi.Strip(m.View().Content), ":rocket:") {
		t.Fatal("emoji preview not visible")
	}
	m = pressApp(m, "tab")
	if m.composer.input.Value() != "界 🚀 tail" || m.composer.focus != 0 {
		t.Fatalf("selection changed surrounding text or focus: %q", m.composer.input.Value())
	}
	if m.composer.input.Column() != 3 {
		t.Fatal("emoji insertion misplaced caret")
	}
}

func TestEmojiCompletionRespectsCodeAndDismissal(t *testing.T) {
	for _, body := range []string{"`literal :roc", "```go\n:roc", "10:30", "word:roc"} {
		m := fixtureApp(t)
		m = pressApp(m, "c")
		next, _ := m.Update(tea.PasteMsg{Content: body})
		m = next.(AppModel)
		if len(m.composer.emojis) > 0 {
			t.Fatalf("suggestions in non-prose context %q", body)
		}
	}
	m := fixtureApp(t)
	m = pressApp(m, "c")
	next, _ := m.Update(tea.PasteMsg{Content: ":roc"})
	m = next.(AppModel)
	m = pressApp(m, "esc")
	if !m.composer.open || m.composer.confirm != "" || len(m.composer.emojis) != 0 {
		t.Fatal("Escape must dismiss emoji suggestions before canceling")
	}
	m = pressApp(m, "k")
	m = pressApp(m, "e")
	m = pressApp(m, "t")
	m = pressApp(m, ":")
	if m.composer.input.Value() != "🚀" {
		t.Fatal("completed shortcode did not turn into an emoji")
	}
	m.composer.input.SetValue("`literal :rocket")
	m = pressApp(m, ":")
	if m.composer.input.Value() != "`literal :rocket:" {
		t.Fatal("code shortcode was replaced")
	}
}

func TestMovingCaretOverAShortcodeDoesNotChangeTheDraft(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	want := "Literal :rocket: tail"
	m.composer.input.SetValue(want)
	generation := m.composer.generation
	for range 5 {
		m, _ = chord(m, tea.KeyLeft, 0)
	}
	if m.composer.input.Value() != want || m.composer.generation != generation {
		t.Fatal("caret navigation rewrote a shortcode or changed the draft generation")
	}
}

func TestDraftConfirmationUsesBrandAndKeepsActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}} {
		m := fixtureApp(t)
		m.account.width, m.account.height = size[0], size[1]
		m = pressApp(m, "c")
		m.composer.input.SetValue("Synthetic draft")
		m = pressApp(m, "ctrl+c")
		frame := ansi.Strip(m.View().Content)
		for _, value := range []string{"/dev/null", "DRAFT PROTECTION", "Keep draft", "Exit", "ctrl+k", "ctrl+d", "Back"} {
			if !strings.Contains(frame, value) {
				t.Fatalf("confirmation lacks %q at %v", value, size)
			}
		}
		for _, line := range strings.Split(frame, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("confirmation width overflow")
			}
		}
		if len(strings.Split(frame, "\n")) > size[1] {
			t.Fatal("confirmation height overflow")
		}
	}
}

func TestBrandedRetryKeepsTheDuplicateRiskWarning(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Uncertain synthetic draft")
	m.composer.uncertain = true
	m.requestDraftAction("retry")
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "duplicate") || !strings.Contains(frame, "previous commit may exist") {
		t.Fatal("retry confirmation lost its duplicate-risk warning")
	}
	m.account.width, m.account.height = 30, 9
	if !strings.Contains(ansi.Strip(m.View().Content), "duplicate") {
		t.Fatal("small retry confirmation hides duplicate risk")
	}
}
