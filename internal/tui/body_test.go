package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func bodyOf(segments []api.Segment, width int, style statsStyle) (string, []string) {
	lines := bodyLines(api.Commit{Segments: segments}, width, colors(true), style)
	raw := strings.Join(lines, "\n")
	return raw, strings.Split(ansi.Strip(raw), "\n")
}

func prose(text string) []api.Segment { return []api.Segment{{Type: "text", Text: text}} }

func link(href string) string { return ansi.SetHyperlink(href) }

func TestBodyTurnsKnownShortcodesIntoEmojiOutsideCode(t *testing.T) {
	segments := []api.Segment{{Type: "text", Text: ":rocket: ship :not_an_emoji: :foo:+1: "}, {Type: "code", Code: ":rocket:"}}
	_, plain := bodyOf(segments, 80, statsStyle{glyphs: nerdIcons})
	if want := "🚀 ship :not_an_emoji: :foo👍 `:rocket:`"; plain[0] != want {
		t.Fatalf("got %q, want %q", plain[0], want)
	}
}

func TestBodyDrawsQuotedLinesWithALaneBar(t *testing.T) {
	p := colors(true)
	raw, plain := bodyOf(prose("> a quoted line that wraps around\nplain reply"), 16, statsStyle{glyphs: nerdIcons})
	want := []string{"▎ a quoted line", "▎ that wraps", "▎ around", "plain reply"}
	if strings.Join(plain, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", plain)
	}
	if !strings.Contains(raw, text(p.lanes[5]).Render("▎ ")) {
		t.Fatal("quote bar is not in lane 5")
	}
}

func TestBodyShortensLinksAndKeepsTheWholeURLClickable(t *testing.T) {
	cases := []struct{ body, shown, href string }{
		{"see https://www.example.com/a/very/long/path/that/goes/on.", "see example.com/a/very/long/path/t….", "https://www.example.com/a/very/long/path/that/goes/on"},
		{"(example.com/)", "(example.com)", "https://example.com/"},
		{"go to http://localhost:8000/x", "go to localhost:8000/x", "http://localhost:8000/x"},
	}
	for _, c := range cases {
		raw, plain := bodyOf(prose(c.body), 80, statsStyle{glyphs: nerdIcons})
		if plain[0] != c.shown || !strings.Contains(raw, link(c.href)) {
			t.Fatalf("%q rendered as %q in %q", c.body, plain[0], raw)
		}
	}
	for _, body := range []string{"foo.community", "mail@example.com", "notes.txt", "javascript:alert(1)"} {
		raw, plain := bodyOf(prose(body), 80, statsStyle{glyphs: nerdIcons})
		if strings.Contains(raw, "\x1b]8;") || plain[0] != body {
			t.Fatalf("%q became a link: %q", body, raw)
		}
	}
}

func TestBodyClosesEveryLinkOnTheLineItOpens(t *testing.T) {
	raw, _ := bodyOf(prose("one https://example.com/a-path-much-longer-than-the-row two"), 12, statsStyle{glyphs: nerdIcons})
	for _, line := range strings.Split(raw, "\n") {
		if opens, closes := strings.Count(line, "\x1b]8;;http"), strings.Count(line, ansi.ResetHyperlink()); opens != closes {
			t.Fatalf("unbalanced link in %q", line)
		}
		if ansi.StringWidth(line) > 12 {
			t.Fatalf("line wider than the body: %q", line)
		}
	}
}

func TestBodyHighlightsMentionsButNotEmailsOrPackages(t *testing.T) {
	p := colors(true)
	raw, plain := bodyOf(prose("@ada, me@mail.com @inertiajs/react @vue.js @-no"), 80, statsStyle{glyphs: nerdIcons})
	if plain[0] != "@ada, me@mail.com @inertiajs/react @vue.js @-no" {
		t.Fatalf("mention text changed: %q", plain[0])
	}
	if !strings.Contains(raw, text(p.blue).Bold(true).Render("@ada")) {
		t.Fatal("mention not highlighted")
	}
	if strings.Contains(raw, "\x1b]8;") {
		t.Fatal("a mention linked without a server")
	}
	raw, _ = bodyOf(prose("hi @Ada-L"), 80, statsStyle{glyphs: nerdIcons, origin: "https://dev-null.test"})
	if !strings.Contains(raw, link("https://dev-null.test/@Ada-L")+text(p.blue).Bold(true).Render("@Ada-L")+ansi.ResetHyperlink()) {
		t.Fatalf("mention does not link to the profile: %q", raw)
	}
	mention, _, _ := strings.Cut(text(p.blue).Bold(true).Render("@"), "@")
	if strings.Count(raw, mention) != 1 {
		t.Fatalf("too many mentions in %q", raw)
	}
}

func TestBodyPutsASwatchBeforeHexColors(t *testing.T) {
	raw, plain := bodyOf(prose("#ff8a4c and #123 and #FFF. and #fff-x and #0000"), 80, statsStyle{glyphs: nerdIcons})
	if want := "■ #ff8a4c and #123 and ■ #FFF. and #fff-x and □ #0000"; plain[0] != want {
		t.Fatalf("got %q, want %q", plain[0], want)
	}
	if !strings.Contains(raw, text("#ff8a4c").Render("■")) || !strings.Contains(raw, text("#ffffff").Render("■")) {
		t.Fatal("swatch not drawn in its own color")
	}
	for _, c := range []struct {
		dark  bool
		color string
	}{{false, "#fff"}, {false, "#f8f8f8"}, {true, "#000"}, {true, "#111112"}} {
		p := colors(c.dark)
		raw := strings.Join(bodyLines(api.Commit{Segments: prose(c.color)}, 80, p, statsStyle{glyphs: nerdIcons}), "\n")
		if !strings.Contains(raw, text(p.muted).Render("□")) || strings.Contains(raw, "■") {
			t.Fatalf("%s on the %v background needs an outlined swatch: %q", c.color, c.dark, raw)
		}
	}
	if raw, _ := bodyOf(prose("#fff"), 80, statsStyle{glyphs: nerdIcons}); !strings.Contains(raw, "■") {
		t.Fatal("white must stay filled on the dark background")
	}
}

func TestBodyDrawsTheWordmark(t *testing.T) {
	p := colors(true)
	raw, plain := bodyOf(prose("/dev/null rocks, but not /dev/nullx or a/dev/null"), 80, statsStyle{glyphs: nerdIcons})
	if plain[0] != "/dev/null rocks, but not /dev/nullx or a/dev/null" {
		t.Fatalf("wordmark text changed: %q", plain[0])
	}
	if strings.Count(raw, text(p.lanes[0]).Bold(true).Render("/")) != 1 {
		t.Fatalf("expected one wordmark in %q", raw)
	}
}

func TestBodyShowsDevNullCommitLinksAsHandleAndHash(t *testing.T) {
	url := "https://dev-null.sh/@ada/commits/0190F3A4-1B2C-7D3E-8F40-123456789ABC"
	raw, plain := bodyOf(prose("read "+url+"!"), 80, statsStyle{glyphs: nerdIcons})
	if plain[0] != "read /ada/6789abc!" || !strings.Contains(raw, link(url)) {
		t.Fatalf("got %q", plain[0])
	}
}

func TestBodyShowsForgeLinksAsTheirForgeWritesThem(t *testing.T) {
	cases := map[string]string{
		"https://github.com/laravel":                                          " laravel",
		"https://github.com/orgs/laravel":                                     " laravel",
		"https://github.com/laravel/framework":                                " laravel/framework",
		"https://github.com/laravel/framework/pull/123#discussion_r9":         " laravel/framework#123 (comment)",
		"https://github.com/laravel/framework/issues/7#issuecomment-1":        " laravel/framework#7 (comment)",
		"https://github.com/a/b/blob/main/src/My%20File.php#L10-L20":          " a/b:My File.php:10-20",
		"https://github.com/a/b/tree/main/src/Services":                       " a/b:Services/",
		"https://github.com/a/b/commit/4fa008f1c2d3":                          " a/b@4fa008f",
		"https://github.com/a/b/releases/tag/v1.2.0":                          " a/b@v1.2.0",
		"https://github.com/a/b/compare/4fa008f1c2d3...main":                  " a/b@4fa008f...main",
		"https://gitlab.com/gitlab-org/gitlab/-/merge_requests/42":            " gitlab-org/gitlab!42",
		"https://codeberg.org/forgejo/forgejo/issues/7":                       " forgejo/forgejo#7",
		"https://github.com/features/actions":                                 "github.com/features/actions",
		"https://github.com/a/b/issues/7#top":                                 "github.com/a/b/issues/7#top",
		"https://github.com/a/b?tab=readme":                                   "github.com/a/b?tab=readme",
		"https://github.com/laravel/framework/pull/123/files/very/long/thing": "github.com/laravel/framework/…",
	}
	for url, want := range cases {
		raw, plain := bodyOf(prose(url), 120, statsStyle{glyphs: nerdIcons})
		if plain[0] != want || !strings.Contains(raw, link(url)) {
			t.Errorf("%s: got %q, want %q", url, plain[0], want)
		}
	}
	_, plain := bodyOf(prose("https://gitlab.com/a/b"), 80, statsStyle{glyphs: plainIcons})
	if plain[0] != "⎇ a/b" {
		t.Fatalf("plain icons: got %q", plain[0])
	}
}

func TestBodyDrawsPastelWordsOnlyInPasteisMode(t *testing.T) {
	p := colors(true)
	body := prose("Pastéis for all, pastel! not pastelaria")
	raw, plain := bodyOf(body, 80, statsStyle{glyphs: nerdIcons})
	if strings.Contains(raw, text(p.lanes[0]).Render("pastel")) || plain[0] != "Pastéis for all, pastel! not pastelaria" {
		t.Fatalf("pastéis mode off changed the body: %q", plain[0])
	}
	raw, plain = bodyOf(body, 80, statsStyle{pasteis: true, glyphs: nerdIcons})
	if plain[0] != "Pastéis for all, pastel! not pastelaria" || !strings.Contains(raw, text(p.lanes[0]).Render("Pastéis")) {
		t.Fatalf("the word must stand in until the icon is ready: %q", raw)
	}
	icons := graphics.New("http://127.0.0.1")
	icons.Enabled = true
	jobs, _ := icons.Plan(t.Context(), []graphics.Request{pastelIconRequest})
	icons.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	raw, plain = bodyOf(body, 80, statsStyle{pasteis: true, icons: icons, glyphs: nerdIcons})
	if strings.Count(raw, string(kitty.Placeholder)) != 4 || strings.Contains(plain[0], "Pastéis") || !strings.Contains(plain[0], "pastelaria") {
		t.Fatalf("got %q", plain[0])
	}
}

func TestBodyHighlightsCodeBlocksUnderTheirFilename(t *testing.T) {
	p := colors(true)
	segments := []api.Segment{
		{Type: "text", Text: "Look:\n"},
		{Type: "block", Language: "go", Filename: "main.go", HTML: `<span class="hl-keyword">func</span> main() { <span class="hl-comment">// &lt;hi&gt;</span>`},
		{Type: "text", Text: "\nDone"},
	}
	raw, plain := bodyOf(segments, 80, statsStyle{glyphs: nerdIcons})
	if want := "Look:||main.go|│ func main() { // <hi>||Done"; strings.Join(plain, "|") != want {
		t.Fatalf("got %q", plain)
	}
	if !strings.Contains(raw, text(p.blue).Render("func")) || !strings.Contains(raw, text(p.muted).Italic(true).Render("//")) {
		t.Fatalf("code not highlighted: %q", raw)
	}
	segments[1].Filename = ""
	if _, plain := bodyOf(segments, 80, statsStyle{glyphs: nerdIcons}); plain[2] != "go" {
		t.Fatalf("language header missing: %q", plain)
	}
}
