package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestBareEmojiAskForTheColorPresentation(t *testing.T) {
	cases := map[string]string{
		"ship it ⚡":     "ship it ⚡️",
		"⚡️":            "⚡️",
		"⚡︎":            "⚡︎",
		"✋\U0001f3fd":   "✋\U0001f3fd",
		"no emoji here": "no emoji here",
		"☕⭐":            "☕️⭐️",
	}
	for in, want := range cases {
		if got := emojiPresentation(in); got != want {
			t.Errorf("emojiPresentation(%q) = %q, want %q", in, got, want)
		}
		if ansi.StringWidth(emojiPresentation(in)) != ansi.StringWidth(in) {
			t.Errorf("%q changed width", in)
		}
	}
	lines := bodyLines(api.Commit{Body: "fast :zap: and ⚡"}, 40, colors(true), statsStyle{})
	if got := strings.Count(strings.Join(lines, ""), "⚡️"); got != 2 {
		t.Fatalf("the body drew %d color ⚡, want 2: %q", got, lines)
	}
}
