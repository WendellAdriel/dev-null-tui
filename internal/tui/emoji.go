package tui

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

//go:embed emoji.json
var emojiJSON []byte

var emojiCatalog = func() map[string]string {
	var catalog map[string]string
	if err := json.Unmarshal(emojiJSON, &catalog); err != nil {
		panic("invalid embedded emoji catalog")
	}
	return catalog
}()

var emojiToken = regexp.MustCompile(`:([A-Za-z0-9_+-]{2,})(:?)$`)
var typedCode = regexp.MustCompile("(?m)^`{3,}[^`\r\n]*\n(?:[\\s\\S]*?\n)?`{3,}[ \\t]*$|`[^`\r\n]+`|^```[\\s\\S]*$|`[^`\r\n]*$")

type emojiSuggestion struct{ name, value string }

func searchEmoji(query string) []emojiSuggestion {
	query = strings.ToLower(query)
	var matches []emojiSuggestion
	for name, value := range emojiCatalog {
		if strings.Contains(name, query) {
			matches = append(matches, emojiSuggestion{name, value})
		}
	}
	rank := func(name string) int {
		if strings.HasPrefix(name, query) {
			return 0
		}
		for _, word := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' }) {
			if strings.HasPrefix(word, query) {
				return 1
			}
		}
		return 2
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i].name, matches[j].name
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	return matches[:min(8, len(matches))]
}

// textBeforeCaret is the editor's text up to the caret, where a typed shortcode ends.
func textBeforeCaret(input textarea.Model) string {
	lines := strings.Split(input.Value(), "\n")
	row := min(input.Line(), len(lines)-1)
	line := []rune(lines[row])
	lines[row] = string(line[:min(input.Column(), len(line))])
	return strings.Join(lines[:row+1], "\n")
}

func emojiAtCaret(prefix string) (string, bool) {
	start := 0
	for _, span := range typedCode.FindAllStringIndex(prefix, -1) {
		if span[1] == len(prefix) {
			return "", false
		}
		start = span[1]
	}
	prose := prefix[start:]
	match := emojiToken.FindStringSubmatchIndex(prose)
	if match == nil {
		return "", false
	}
	if match[0] > 0 {
		previous, _ := utf8.DecodeLastRuneInString(prose[:match[0]])
		if unicode.IsLetter(previous) || unicode.IsNumber(previous) || previous == '_' || previous == ':' {
			return "", false
		}
	}
	return prose[match[2]:match[3]], match[4] != match[5]
}

// replaceShortcode swaps the shortcode before the caret for its emoji.
func replaceShortcode(input *textarea.Model, name, value string, closed bool) {
	length := len(name) + 1
	if closed {
		length++
	}
	for range length {
		*input, _ = input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	input.InsertString(value)
}

// refreshSuggestions updates the composer's suggestions: emoji, and the mentions and issues it looks up.
func (m *AppModel) refreshSuggestions() tea.Cmd {
	c := &m.composer
	if c.focus != 0 || c.owner != m.account.user.ID || c.sending || c.previewing || c.loadingPreview {
		c.suggest.close()
		return nil
	}
	return m.refreshCompletion(&c.suggest, c.input, "commit", m.composerMentionScope())
}

func (m *AppModel) acceptSuggestion() tea.Cmd {
	c := &m.composer
	acceptCompletion(&c.suggest, &c.input)
	m.invalidatePreview()
	return m.refreshSuggestions()
}

// bareEmoji are the catalog's single BMP codepoints. Terminal fonts often have plain text glyphs for them, so a
// terminal draws ⚡ as a thin symbol unless U+FE0F asks for the color emoji.
var bareEmoji = func() map[rune]bool {
	bare := map[rune]bool{}
	for _, value := range emojiCatalog {
		if r, size := utf8.DecodeRuneInString(value); size == len(value) && r <= 0xffff {
			bare[r] = true
		}
	}
	return bare
}()

// emojiPresentation adds U+FE0F after a bare emoji, unless a selector, skin tone, joiner or keycap already follows
// it. The emoji keeps its two-cell width.
func emojiPresentation(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return bareEmoji[r] }) {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		b.WriteRune(r)
		if !bareEmoji[r] {
			continue
		}
		if i+1 < len(runes) {
			next := runes[i+1]
			if next == '\ufe0f' || next == '\ufe0e' || next == '\u200d' || next == '\u20e3' || next >= 0x1f3fb && next <= 0x1f3ff {
				continue
			}
		}
		b.WriteRune('\ufe0f')
	}
	return b.String()
}
