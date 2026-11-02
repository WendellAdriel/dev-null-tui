package tui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

type whatsNewState struct {
	checked, open, connected bool
	version, seen, body      string
	scroll                   int
}

type whatsNewSaved struct{ err error }

// whatsNewDecision shows a release summary only after an upgrade. Installs that predate the state file count as
// upgrades when a connection is saved, so only a fresh install records the version without showing it.
func whatsNewDecision(current, seen string, connected bool) (show, record bool) {
	if !release.ValidVersion(current) {
		return false, false
	}
	if release.ValidVersion(seen) && release.Compare(current, seen) <= 0 {
		return false, false
	}
	if !release.ValidVersion(seen) && !connected {
		return false, true
	}
	_, hasSummary := release.Summary(current)
	return hasSummary, !hasSummary
}

func (m *AppModel) startWhatsNew() tea.Cmd {
	w := &m.whatsNew
	if w.checked {
		return nil
	}
	w.checked = true
	show, record := whatsNewDecision(w.version, w.seen, w.connected)
	if show {
		w.body, _ = release.Summary(w.version)
		w.open, w.scroll = true, 0
		return nil
	}
	if record {
		return m.saveSeenVersion()
	}
	return nil
}

func (m AppModel) saveSeenVersion() tea.Cmd {
	store, version := m.account.store, m.whatsNew.version
	return func() tea.Msg { return whatsNewSaved{err: store.SaveSeenVersion(version)} }
}

func (m *AppModel) acceptWhatsNewSaved(msg whatsNewSaved) tea.Cmd {
	if msg.err == nil {
		return nil
	}
	return m.notify("warning", "What's new may show again", "The version could not be saved: "+msg.err.Error())
}

func (m *AppModel) whatsNewKey(pressed string) tea.Cmd {
	w := &m.whatsNew
	switch pressed {
	case "esc", "enter", "q":
		w.open = false
		return m.saveSeenVersion()
	case "j", "down":
		w.scroll++
	case "k", "up":
		w.scroll--
	case "pgdown", "ctrl+d", "space":
		w.scroll += 5
	case "pgup", "ctrl+u":
		w.scroll -= 5
	case "home", "g":
		w.scroll = 0
	case "end", "G":
		w.scroll = m.whatsNewMaxScroll()
	}
	w.scroll = max(0, min(m.whatsNewMaxScroll(), w.scroll))
	return nil
}

func (m AppModel) whatsNewBounds() uiRect {
	return centeredRect(m.account.width, m.account.height, 72, 40)
}

func (m AppModel) whatsNewLines(p palette) []string {
	r := m.whatsNewBounds()
	return strings.Split(renderSummary(m.whatsNew.body, max(1, r.w-4), p), "\n")
}

func (m AppModel) whatsNewMaxScroll() int {
	r := m.whatsNewBounds()
	return max(0, len(m.whatsNewLines(colors(m.account.dark)))-max(1, r.h-6))
}

func (m AppModel) whatsNewModal(p palette) string {
	r := m.whatsNewBounds()
	lines := m.whatsNewLines(p)
	start := min(m.whatsNew.scroll, m.whatsNewMaxScroll())
	content := strings.Join(lines[start:min(len(lines), start+max(1, r.h-6))], "\n")
	return panel("What's new in "+safeText(m.whatsNew.version), content, "↑/↓ scroll · esc close", r, p, p.blue)
}

var summaryInline = regexp.MustCompile("\\*\\*([^*]+)\\*\\*|`([^`]+)`")

// renderSummary draws the markdown subset release summaries use: # and ## headings, - bullets, paragraphs, **bold**,
// and `code`. Any other markup is shown as plain text, and a test keeps the shipped summaries inside this subset.
func renderSummary(markdown string, width int, p palette) string {
	var out []string
	for _, block := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		if heading, ok := strings.CutPrefix(block, "## "); ok {
			out = append(out, wrapSummary(text(p.ink).Bold(true).Render(safeText(heading)), width, "")...)
			continue
		}
		if heading, ok := strings.CutPrefix(block, "# "); ok {
			out = append(out, wrapSummary(text(p.blue).Bold(true).Render(safeText(heading)), width, "")...)
			continue
		}
		var paragraph []string
		for _, line := range strings.Split(block, "\n") {
			if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
				if len(paragraph) > 0 {
					out = append(out, wrapSummary(summaryInlineText(strings.Join(paragraph, " "), p), width, "")...)
					paragraph = nil
				}
				out = append(out, wrapSummary(text(p.muted).Render("• ")+summaryInlineText(item, p), width, "  ")...)
				continue
			}
			paragraph = append(paragraph, strings.TrimSpace(line))
		}
		if len(paragraph) > 0 {
			out = append(out, wrapSummary(summaryInlineText(strings.Join(paragraph, " "), p), width, "")...)
		}
	}
	return strings.Join(out, "\n")
}

func summaryInlineText(line string, p palette) string {
	line = safeText(line)
	return summaryInline.ReplaceAllStringFunc(line, func(match string) string {
		parts := summaryInline.FindStringSubmatch(match)
		if parts[1] != "" {
			return text(p.ink).Bold(true).Render(parts[1])
		}
		return text(p.code).Render(parts[2])
	})
}

func wrapSummary(styled string, width int, indent string) []string {
	lines := strings.Split(ansi.Wrap(styled, max(1, width-len(indent)), ""), "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return lines
}
