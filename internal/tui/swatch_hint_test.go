package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestTheSwatchHintOffersTheLastHashtagThatCouldBeAColor(t *testing.T) {
	for draft, want := range map[string]string{
		"Love #fade":                     "#fade",
		"#fade and #beef":                "#beef",
		"#beef then #design":             "#beef",
		"#Cafe12 is warm":                "#Cafe12",
		"`#fade` in code":                "",
		"```\n#fade":                     "",
		"open https://example.com/#fade": "",
		"type color(#fade":               "",
		"a#fade or &#fade":               "",
		"#123 is an issue":               "",
		"#fades is a word":               "",
		"color(#fade) already":           "",
	} {
		hint, ok := swatchHintFor(draft)
		if got := map[bool]string{true: hint.hex}[ok]; got != want {
			t.Errorf("%q: got %q, want %q", draft, got, want)
		}
	}
}

func TestUsingTheSwatchHintWrapsTheHashtagAndPutsTheCaretAfterIt(t *testing.T) {
	input := textarea.New()
	input.SetValue("Love #fade today\nand more")
	if !applySwatchHint(&input, fitsCommit) || input.Value() != "Love color(#fade) today\nand more" {
		t.Fatalf("applying the hint: %q", input.Value())
	}
	input.InsertString("|")
	if !strings.HasPrefix(input.Value(), "Love color(#fade)| today") {
		t.Fatalf("the caret must follow the markup: %q", input.Value())
	}
	lines := textarea.New()
	lines.SetWidth(20)
	lines.CharLimit = 0
	lines.SetValue("#fade\n" + strings.Repeat("a long wrapped line of text\n", 100) + "end")
	lines.MoveToEnd()
	if !applySwatchHint(&lines, fitsCommit) || lines.Line() != 0 || lines.Column() != len("color(#fade)") {
		t.Fatalf("the caret must land after the markup on the first line: row %d, column %d", lines.Line(), lines.Column())
	}
	nested := textarea.New()
	nested.SetWidth(12)
	nested.SetValue("one line\nwrapped words and then #fade more")
	if !applySwatchHint(&nested, fitsCommit) || nested.Line() != 1 || nested.Column() != len("wrapped words and then color(#fade)") {
		t.Fatalf("the caret must follow markup inside a wrapped line: row %d, column %d", nested.Line(), nested.Column())
	}
	long := textarea.New()
	long.CharLimit = 0
	long.SetValue(strings.Repeat("a", api.MaxBodyLength-6) + " #fade")
	if applySwatchHint(&long, fitsCommit) || strings.Contains(long.Value(), "color(") {
		t.Fatal("the hint must not push a draft past the body limit")
	}
}

func TestTheComposerOffersTheSwatchHintAndAppliesItByKeyOrClick(t *testing.T) {
	m := typeText(composerApp(t), " in #fade")
	if !strings.Contains(frameText(m), "For a swatch, use color(#fade).") {
		t.Fatalf("the composer must offer the swatch:\n%s", frameText(m))
	}
	m = keys(m, "ctrl+y")
	if m.composer.input.Value() != "Draft text in color(#fade)" || strings.Contains(frameText(m), "For a swatch") {
		t.Fatalf("ctrl+y must apply the hint: %q", m.composer.input.Value())
	}
	m = typeText(m, " and #beef")
	m, cmd := clickText(t, m, "Use as color")
	m = settleCmd(m, cmd)
	if m.composer.input.Value() != "Draft text in color(#fade) and color(#beef)" {
		t.Fatalf("clicking Use as color must apply the hint: %q", m.composer.input.Value())
	}
}

func TestTheRemotesComposerOffersTheSwatchHintToo(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter", "w")
	m = typeText(m, "paint it #fade")
	if !strings.Contains(frameText(m), "For a swatch, use color(#fade).") {
		t.Fatalf("the message composer must offer the swatch:\n%s", frameText(m))
	}
	m = keys(m, "ctrl+y")
	if got := m.remotes.thread.composer.input.Value(); got != "paint it color(#fade)" {
		t.Fatalf("ctrl+y must apply the hint in Remotes: %q", got)
	}
}
