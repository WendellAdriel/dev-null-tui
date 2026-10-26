package tui

import (
	"fmt"
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

func TestReaderHasAuthorIdentityAndComposerEntry(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	plain := ansi.Strip(m.View().Content)
	for _, value := range []string{"●", "Ada Fixture", "@ada", "What did you ship?", "c Commit"} {
		if !strings.Contains(plain, value) {
			t.Fatalf("workspace lost identity or active reading control %q", value)
		}
	}
	for _, binding := range m.bindings().all {
		if binding.Help().Key == "r" || binding.Help().Key == "R" {
			t.Fatal("refresh must not use a case-sensitive letter shortcut")
		}
	}
	for _, value := range []string{"Search  /"} {
		if strings.Contains(plain, value) {
			t.Fatalf("obsolete or unfinished control %q", value)
		}
	}
}

func TestWorkspaceActionsAndShortcutsHaveOneHome(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 35
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	p := colors(true)
	nav := ansi.Strip(m.navigation(25, m.mainHeight(), p))
	if strings.Contains(nav, "Account") || strings.Contains(nav, "Refresh") || !strings.Contains(nav, "Commit") {
		t.Fatal("left sidebar must contain a commit button, not account or refresh")
	}
	aside := ansi.Strip(m.aside(40, m.mainHeight(), p))
	if strings.Contains(aside, "New commit") {
		t.Fatal("aside still duplicates composing")
	}
	sections := strings.SplitN(aside, "Shortcuts", 2)
	if len(sections) != 2 || !strings.Contains(sections[0], "Actions") || !strings.Contains(sections[0], "Account") || !strings.Contains(sections[0], "Refresh") {
		t.Fatal("aside actions missing above shortcuts")
	}
	for _, removed := range []string{"compose", "refresh", "account", "New commit"} {
		if strings.Contains(sections[1], removed) {
			t.Fatalf("duplicate shortcut %q", removed)
		}
	}
	for _, key := range []string{"j/k", "tab", "y", "f2", "?", "ctrl+c"} {
		if !strings.Contains(sections[1], key) {
			t.Fatalf("missing sidebar shortcut %q", key)
		}
	}
	footer := ansi.Strip(strings.Split(m.View().Content, "\n")[m.mainHeight()])
	for _, key := range []string{"f2", "? help", "ctrl+c", "ctrl+r", "compose", "tab"} {
		if strings.Contains(footer, key) {
			t.Fatalf("footer still has shortcut %q", key)
		}
	}
	m.notice = "Synthetic request failed. Reading position kept."
	if !strings.Contains(ansi.Strip(m.View().Content), m.notice) {
		t.Fatal("status footer lost feedback")
	}
}

func TestRelocatedButtonsAndShortcutsAcceptMouse(t *testing.T) {
	for _, width := range []int{124, 160} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := fixtureApp(t)
			m.account.width, m.account.height = width, 35
			m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
			m, _ = clickText(t, m, "Account")
			if !m.accountOpen {
				t.Fatal("relocated account button missed")
			}
			m = pressApp(m, "esc")
			m, cmd := clickText(t, m, "Refresh")
			if cmd == nil || !m.feeds[m.active].loading {
				t.Fatal("relocated refresh button missed")
			}
			m, _ = clickText(t, m, "light theme")
			if m.account.dark {
				t.Fatal("sidebar theme shortcut missed")
			}
			m, _ = clickText(t, m, "help")
			if !m.help {
				t.Fatal("sidebar help shortcut missed")
			}
			m = pressApp(m, "esc")
			m, _ = clickText(t, m, "Commit")
			if !m.composer.open {
				t.Fatal("left commit button missed")
			}
		})
	}
}

func TestAsideKeepsActionsAndShortcutsVisibleWithLongProfile(t *testing.T) {
	m := fixtureApp(t)
	m.account.user.Bio = strings.Repeat("Long profile text. ", 30)
	m.account.user.Company = "Fixture Company"
	m.account.user.Location = "Porto"
	m.account.user.Commits = new(31)
	for _, height := range []int{24, 35} {
		plain := ansi.Strip(m.aside(36, height, colors(true)))
		for _, label := range []string{"Actions", "Account", "Refresh", "Shortcuts", "f2", "?", "ctrl+c"} {
			if !strings.Contains(plain, label) {
				t.Fatalf("sidebar at %d rows hid %q behind profile", height, label)
			}
		}
	}
}

func TestSidebarButtonsAreCompactFilledAndCentered(t *testing.T) {
	m := fixtureApp(t)
	p := colors(true)
	for _, width := range []int{5, 25} {
		column, targets := m.navigationLayout(width, 34, p)
		lines := strings.Split(column, "\n")
		for _, target := range targets {
			if target.action != "c" {
				continue
			}
			if target.h != 1 {
				t.Fatal("commit button must occupy one row")
			}
			line := lines[target.y]
			plain := ansi.Strip(line)
			if !strings.Contains(line, "48;2;") {
				t.Fatal("commit button is not filled")
			}
			if target.x != width-target.x-target.w || ansi.StringWidth(plain) != width {
				t.Fatal("commit button not centered within sidebar")
			}
		}
	}
	aside, areas := m.asideLayout(40, 34, p)
	lines := strings.Split(aside, "\n")
	for _, area := range areas {
		if area.action != "a" && area.action != "ctrl+r" {
			continue
		}
		if area.h != 1 {
			t.Fatal("action buttons must occupy one row")
		}
		for _, line := range lines[area.y : area.y+area.h] {
			if !strings.Contains(line, "48;2;") {
				t.Fatal("action button border is not filled")
			}
		}
	}
}
