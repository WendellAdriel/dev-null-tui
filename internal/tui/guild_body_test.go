package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func TestGuildHashtagsDrawAsChipsAndOtherTagsStay(t *testing.T) {
	commit := api.Commit{
		Segments: []api.Segment{
			{Type: "text", Text: "Queues in #Laravel are underrated, unlike #golang. "},
			{Type: "code", Code: "#laravel"},
		},
		GuildMentions: map[string]api.GuildBadge{"laravel": {Handle: "laravel", Name: "Laravel", Emoji: "🟥"}},
	}
	body := strings.Join(bodyLines(commit, 0, colors(true), statsStyle{}), "\n")
	if plain := ansi.Strip(body); plain != "Queues in 🟥 Laravel are underrated, unlike #golang. `#laravel`" {
		t.Fatalf("text: %q", plain)
	}
	line := body
	at := ansi.StringWidth(ansi.Strip(line[:strings.Index(line, "Laravel")]))
	if got := guildAt(line, at); got != "laravel" {
		t.Fatalf("the chip opens %q", got)
	}
	if hashtagAt(line, at) != "" {
		t.Fatal("a guild chip must not also search its hashtag")
	}
	if hashtagMarker.ReplaceAllString(body, "") == body || strings.Contains(hashtagMarker.ReplaceAllString(body, ""), "devnull-guild") {
		t.Fatal("guild marks must not reach the terminal")
	}
}

func TestBoardPostsAndJoinedGuildCommitsSayWhichGuild(t *testing.T) {
	post := fixtureCommit("c1")
	post.Guild = &api.GuildBadge{Handle: "cafe", Name: "Café", Emoji: "☕"}
	post.GuildHandle = "cafe"
	lines := commitLines(post, 70, colors(true), fixtureCommit("x").CreatedAt)
	if !strings.Contains(ansi.Strip(lines[1]), "posted in ☕ Café · not shown in general logs") {
		t.Fatalf("board post:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	}
	at := strings.Index(ansi.Strip(lines[1]), "Café")
	if guildAt(lines[1], ansi.StringWidth(ansi.Strip(lines[1])[:at])) != "cafe" {
		t.Fatal("the guild line must open the board")
	}

	joined := post
	joined.JoinedGuild = "cafe"
	if line := ansi.Strip(commitLines(joined, 70, colors(true), post.CreatedAt)[1]); !strings.Contains(line, "because you joined ☕ Café") || strings.Contains(line, "posted in") {
		t.Fatalf("Following: %q", line)
	}

}

func TestGuildChipsDropTerminalEscapesFromGuildNames(t *testing.T) {
	commit := api.Commit{
		Segments:      []api.Segment{{Type: "text", Text: "Join #evil"}},
		GuildMentions: map[string]api.GuildBadge{"evil": {Handle: "evil", Name: "Evil\x1b]52;c;aGk=\x07", Emoji: "😈"}},
	}
	body := strings.Join(bodyLines(commit, 0, colors(true), statsStyle{}), "\n")
	if strings.Contains(body, "]52;") || strings.Contains(body, "\x07") {
		t.Fatalf("a guild name reached the terminal raw: %q", body)
	}
}

func TestGuildChipsDrawTheUploadedIconOnceItIsReady(t *testing.T) {
	brew := api.GuildBadge{Handle: "brew", Name: "Café", Emoji: "☕", IconURL: "https://example.test/guilds/brew/icon"}
	commit := fixtureCommit("c1")
	commit.Segments = []api.Segment{{Type: "text", Text: "Brewing in #brew today"}}
	commit.GuildMentions = map[string]api.GuildBadge{"brew": brew}
	commit.Guild, commit.GuildHandle = &brew, "brew"
	icons := graphics.New("http://127.0.0.1")
	icons.Enabled = true
	draw := func() []string { return commitLines(commit, 70, colors(true), commit.CreatedAt, icons) }
	if text := ansi.Strip(strings.Join(draw(), "\n")); !strings.Contains(text, "posted in ☕ Café") || !strings.Contains(text, "Brewing in ☕ Café today") {
		t.Fatalf("the emoji must stand in until the icon is ready:\n%s", text)
	}
	jobs, _ := icons.Plan(t.Context(), []graphics.Request{guildChipRequest(brew)})
	icons.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	lines := draw()
	for _, line := range lines[1:3] {
		if strings.Count(line, string(kitty.Placeholder)) != 2 || strings.Contains(line, "☕") {
			t.Fatalf("the chip must draw the icon in two cells: %q", ansi.Strip(line))
		}
	}
	if plain := ansi.Strip(lines[2]); !strings.HasPrefix(plain, "Brewing in "+string(kitty.Placeholder)) || !strings.HasSuffix(plain, " Café today") {
		t.Fatalf("the icon must take the emoji's place: %q", ansi.Strip(lines[2]))
	}
	at := strings.Index(ansi.Strip(lines[2]), string(kitty.Placeholder))
	if guildAt(lines[2], ansi.StringWidth(ansi.Strip(lines[2])[:at])) != "brew" {
		t.Fatal("clicking the icon must open the board")
	}
	long := api.Commit{Segments: []api.Segment{{Type: "text", Text: "Join #brew today please"}}, GuildMentions: map[string]api.GuildBadge{"brew": brew}}
	long.GuildMentions["brew"] = api.GuildBadge{Handle: "brew", Name: "Laravel Brasil Community", Emoji: "☕", IconURL: brew.IconURL}
	widths := func(style statsStyle) []int {
		var out []int
		for _, line := range bodyLines(long, 14, colors(true), style) {
			out = append(out, ansi.StringWidth(line))
		}
		return out
	}
	if before, after := widths(statsStyle{}), widths(statsStyle{icons: icons}); !slices.Equal(before, after) {
		t.Fatalf("the icon must wrap as the emoji does: %v, then %v", before, after)
	}
	icons.Enabled = false
	if text := ansi.Strip(strings.Join(draw(), "\n")); !strings.Contains(text, "Brewing in ☕ Café today") {
		t.Fatalf("with images off the emoji stays:\n%s", text)
	}
}

func TestGuildIconsInVisibleCommitsLoadInTheSmallLane(t *testing.T) {
	m, _ := guildApp(t, laravelBoard(false, true, false))
	m.images.Enabled = true
	brew := api.GuildBadge{Handle: "brew", Name: "Café", Emoji: "☕", IconURL: "https://example.test/guilds/brew/icon"}
	commit := fixtureCommit("c1")
	commit.Body = "Brewing in #brew"
	commit.GuildMentions = map[string]api.GuildBadge{"brew": brew}
	quoted := fixtureCommit("q1")
	php := api.GuildBadge{Handle: "php", Name: "PHP", Emoji: "🐘", IconURL: "https://example.test/guilds/php/icon"}
	quoted.Body, quoted.GuildMentions = "Read #php", map[string]api.GuildBadge{"php": php}
	commit.QuotedID, commit.Quote = &quoted.ID, &quoted
	m = applyPage(m, 0, api.Page{Data: []api.Commit{commit}})
	if media := m.visibleMedia(); !slices.Contains(media, guildChipRequest(brew)) || !slices.Contains(media, guildChipRequest(php)) {
		t.Fatal("the guild icons a visible commit and its quote draw must be requested")
	}
}
