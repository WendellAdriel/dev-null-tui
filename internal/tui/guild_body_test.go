package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
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
