package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestComposerKeepsIdentityInHeaderAndCounterInFooter(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}, {48, 18}} {
			for _, mode := range []string{"", "reply", "quote"} {
				m := fixtureApp(t)
				m.account.dark = dark
				m.account.width, m.account.height = size[0], size[1]
				target := fixtureCommit("target")
				target.Body = "Original commit text"
				m.composeTo(mode, target)
				m.composer.input.SetValue("Aligned draft")
				view, r := m.composerModal(colors(dark))
				plain := ansi.Strip(view)
				rows := strings.Split(plain, "\n")
				title := map[string]string{"": "New commit", "reply": "Reply", "quote": "Quote commit"}[mode]
				for _, label := range []string{title, "@fixture", "●"} {
					if !strings.Contains(rows[1], label) {
						t.Fatalf("header lacks %q at %dx%d (%s):\n%s", label, size[0], size[1], mode, plain)
					}
				}
				if strings.Count(plain, "@fixture") != 1 || (mode == "" && strings.Count(plain, "New commit") != 1) {
					t.Fatalf("duplicated composer heading:\n%s", plain)
				}
				editor := m.composerEditorBounds()
				if editor.x != r.x+2 || !strings.HasPrefix(rows[editor.y-r.y], "│ Aligned draft") {
					t.Fatalf("textarea is not aligned with modal content at %dx%d (%s):\n%s", size[0], size[1], mode, plain)
				}
				counter := fmt.Sprintf("13/%d", 4000)
				if strings.Count(plain, counter) != 1 || !strings.HasSuffix(rows[len(rows)-2], counter+" │") {
					t.Fatalf("counter is not right-aligned in the footer:\n%s", plain)
				}
				if mode != "" {
					label := "Replying to @ada"
					if mode == "quote" {
						label = "Quoting @ada"
					}
					if !strings.Contains(rows[3], label) || !strings.Contains(rows[4], target.Body) || editor.y <= r.y+4 {
						t.Fatalf("reply/quote callout is not between the header and editor:\n%s", plain)
					}
				}
			}
		}
	}
}

func TestComposerFooterCounterDoesNotStealActionClicks(t *testing.T) {
	for _, size := range [][2]int{{120, 35}, {80, 24}, {48, 18}} {
		m := fixtureApp(t)
		m.account.width, m.account.height = size[0], size[1]
		m.composeTo("reply", fixtureCommit("target"))
		m.composer.input.SetValue("Retain this draft")
		m, _ = clickText(t, m, "Cancel")
		if m.composer.confirm != "discard" {
			t.Fatal("right-aligned counter broke the Cancel action")
		}
	}
}

func TestComposerFooterHeightStaysStableAcrossCountsAndRequestStates(t *testing.T) {
	for _, width := range []int{48, 58, 60, 80, 120} {
		m := fixtureApp(t)
		m.account.width, m.account.height = width, 24
		m = pressApp(m, "c")
		footer, _ := m.composerFooter()
		height := lipgloss.Height(footer)
		for _, count := range []int{9, 10, 999, 1000, 4000, 4001} {
			m.composer.input.SetValue(strings.Repeat("a", count))
			for _, state := range []string{"editing", "previewing", "sending"} {
				m.composer.previewing, m.composer.sending = state == "previewing", state == "sending"
				footer, _ := m.composerFooter()
				if lipgloss.Height(footer) != height {
					t.Fatalf("footer changed height at width %d with %d characters (%s)", width, count, state)
				}
			}
		}
	}
}
