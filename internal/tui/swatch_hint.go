package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// swatchHint is a hashtag in a draft that could also be a hex color, which color(...) would draw as a swatch.
type swatchHint struct {
	start int
	hex   string
}

func (h swatchHint) markup() string { return "color(" + h.hex + ")" }

var (
	hexHashtag = regexp.MustCompile(`(?i)^#(?:[0-9a-f]{8}|[0-9a-f]{6}|[0-9a-f]{3,4})$`)
	// typingColor is a draft that ends inside color(, where the hashtag is already becoming a swatch.
	typingColor = regexp.MustCompile(`(?i)color\((?:#[0-9a-f]*)?$`)
)

// swatchHintFor finds the last hashtag outside code and links that is also a hex color, as the web's
// ambiguousColorInDraft does.
func swatchHintFor(draft string) (swatchHint, bool) {
	prose := []byte(draft)
	blank := func(start, end int) {
		for i := start; i < end; i++ {
			prose[i] = ' '
		}
	}
	for _, span := range typedCode.FindAllStringIndex(draft, -1) {
		blank(span[0], span[1])
	}
	for _, match := range findURLs(string(prose)) {
		if link := withoutTrailingPunctuation(string(prose[match[0]:match[1]])); hrefOK(link) {
			blank(match[0], match[0]+len(link))
		}
	}
	text := string(prose)
	var hint swatchHint
	found := false
	for end := 0; ; {
		start, stop, ok := nextHashtag(text, end, func(i int) string { return text[:i] })
		if !ok {
			break
		}
		if hexHashtag.MatchString(text[start:stop]) && !typedColorBefore(text[:start]) {
			hint, found = swatchHint{start: start, hex: text[start:stop]}, true
		}
		end = stop
	}
	return hint, found
}

// typedColorBefore reports prose ending in "color(" that isn't part of a longer word.
func typedColorBefore(prose string) bool {
	loc := typingColor.FindStringIndex(prose)
	if loc == nil {
		return false
	}
	before := lastRune(prose[:loc[0]])
	return loc[0] == 0 || (!isWordRune(before) && before != '#')
}

// swatchHintRow is the hint under a draft: the markup to use, and a chip to apply it while the draft still fits.
func swatchHintRow(input textarea.Model, fits func(string) bool, width int, p palette) (string, []hitArea) {
	draft := input.Value()
	hint, ok := swatchHintFor(draft)
	if !ok {
		return "", nil
	}
	parts := []statusPart{hintText("For a swatch, use " + hint.markup() + ".")}
	if fits(draft[:hint.start] + hint.markup() + draft[hint.start+len(hint.hex):]) {
		parts = append(parts, hintChip("Use as color", "ctrl+y", "ctrl+y"))
	}
	return hintRow(parts, width, p)
}

func hrefOK(link string) bool {
	_, ok := hrefFor(link)
	return ok
}

func fitsCommit(body string) bool {
	return len([]rune(body)) <= api.MaxRawBodyLength && api.BodyCharacterCount(body) <= api.MaxBodyLength
}

func fitsMessage(body string) bool { return api.MessageLength(body) <= api.MaxMessageLength }

// applySwatchHint wraps the hinted hashtag in color(...) and leaves the caret after it, unless the draft would no
// longer fit.
func applySwatchHint(input *textarea.Model, fits func(string) bool) bool {
	draft := input.Value()
	hint, ok := swatchHintFor(draft)
	if !ok {
		return false
	}
	after := draft[hint.start+len(hint.hex):]
	replaced := draft[:hint.start] + hint.markup() + after
	if !fits(replaced) {
		return false
	}
	input.SetValue(replaced)
	before := draft[:hint.start] + hint.markup()
	row := strings.Count(before, "\n")
	// CursorDown walks wrapped rows too, which also scrolls the view to the caret. Each step is one row at most.
	input.MoveToBegin()
	for steps := 0; input.Line() < row && steps < len(replaced); steps++ {
		input.CursorDown()
	}
	input.SetCursorColumn(utf8.RuneCountInString(before[strings.LastIndex(before, "\n")+1:]))
	return true
}
