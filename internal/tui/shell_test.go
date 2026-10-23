package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestAccountAsideUsesOnlyReturnedProfileFields(t *testing.T) {
	m := fixtureApp(t)
	m.account.user.Bio = "Building small tools for developers.\x1b[2J"
	m.account.user.Company = "Fixture Labs"
	m.account.user.Location = "Porto"
	m.account.user.Commits = new(31)
	m.account.user.Following = new(48)
	m.account.user.Followers = new(5)
	plain := ansi.Strip(m.aside(34, 42, colors(true)))
	for _, value := range []string{"Building small tools", "Fixture Labs", "Porto", "31", "48", "5", "commits", "following", "followers"} {
		if !strings.Contains(plain, value) {
			t.Fatalf("account aside lost returned field %q", value)
		}
	}
	if strings.Contains(plain, "followingfollowers") {
		t.Fatal("profile count labels run together")
	}
	if !strings.Contains(ansi.Strip(m.aside(36, 42, colors(true))), "5 followers") {
		t.Fatal("profile count must wrap with its label")
	}
	if strings.Contains(plain, "\x1b[2J") {
		t.Fatal("remote profile control sequence survived")
	}
	m.account.user.Commits, m.account.user.Following, m.account.user.Followers = nil, nil, nil
	plain = ansi.Strip(m.aside(34, 42, colors(true)))
	if strings.Contains(plain, "followers") || strings.Contains(plain, "commits") {
		t.Fatal("missing account counts must not become fabricated zeroes")
	}
}

func TestFirstLoadShowsDayMarkerBeforeTheFirstAuthor(t *testing.T) {
	m := fixtureApp(t)
	var commits []api.Commit
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		commits = append(commits, fixtureCommit(id))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	if m.feeds[0].viewport.YOffset() != 0 || !strings.Contains(ansi.Strip(m.View().Content), "Today") {
		t.Fatal("first load skipped its day marker")
	}
}

func TestReaderHasAuthorIdentityAndReadOnlyContext(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	plain := ansi.Strip(m.View().Content)
	for _, value := range []string{"●", "Ada Fixture", "@ada", "ctrl+r", "Read-only"} {
		if !strings.Contains(plain, value) {
			t.Fatalf("workspace lost identity or active reading control %q", value)
		}
	}
	for _, binding := range m.bindings().all {
		if binding.Help().Key == "r" || binding.Help().Key == "R" {
			t.Fatal("refresh must not use a case-sensitive letter shortcut")
		}
	}
	for _, value := range []string{"Commit  c", "Search  /"} {
		if strings.Contains(plain, value) {
			t.Fatalf("obsolete or unfinished control %q", value)
		}
	}
}
