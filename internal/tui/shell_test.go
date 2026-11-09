package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestTheLeftSidebarCentersYourCountsAndTheAsideDropsYourProfile(t *testing.T) {
	m := fixtureApp(t)
	m.account.user.Name = "Ada Fixture"
	m.account.user.Handle = "ada"
	m.account.user.Bio = "Building small tools.\x1b[2J"
	m.account.user.Company = "Fixture Labs"
	m.account.user.Location = "Porto"
	m.account.user.Commits, m.account.user.Following, m.account.user.Followers = new(31), new(48), new(1)
	m.account.user.PasteisMode, m.account.user.PasteisRemaining = true, new(7)
	nav := strings.Split(ansi.Strip(m.navigation(25, 40, colors(true))), "\n")
	var rows []string
	for _, want := range []string{"31 commits", "48 following", "1 follower", "7 pastéis left today"} {
		for i, line := range nav {
			if !strings.Contains(line, want) {
				continue
			}
			left, right := len(line)-len(strings.TrimLeft(line, " ")), len(line)-len(strings.TrimRight(line, " "))
			if left-right > 1 || right-left > 1 {
				t.Fatalf("%q is not centered: %q", want, line)
			}
			rows = append(rows, fmt.Sprint(i))
		}
	}
	if slices.Sort(rows); len(slices.Compact(rows)) != 4 {
		t.Fatalf("each count goes on its own line, found on rows %v:\n%s", rows, strings.Join(nav, "\n"))
	}
	aside := ansi.Strip(m.aside(40, 40, colors(true)))
	for _, gone := range []string{"Building small tools", "Fixture Labs", "Porto", "@ada", "commits", "pastéis"} {
		if strings.Contains(aside, gone) {
			t.Fatalf("the aside still shows %q", gone)
		}
	}
	for _, gone := range []string{"Building small tools", "Fixture Labs", "Porto", "\x1b[2J"} {
		if strings.Contains(strings.Join(nav, "\n"), gone) {
			t.Fatalf("the sidebar shows %q", gone)
		}
	}
	if short := m.navigation(25, 23, colors(true)); strings.Count(short, "\n") != 22 || !strings.Contains(ansi.Strip(short), "@ada") {
		t.Fatalf("a short sidebar must drop counts, not its identity:\n%s", ansi.Strip(short))
	}
	m.account.user.Commits, m.account.user.Following, m.account.user.Followers = nil, nil, nil
	if plain := ansi.Strip(m.navigation(25, 40, colors(true))); strings.Contains(plain, "commits") || strings.Contains(plain, "follower") {
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
	for _, value := range []string{"●", "Ada Fixture", "@ada", "What did you ship?", " Commit  c "} {
		if !strings.Contains(plain, value) {
			t.Fatalf("workspace lost identity or active reading control %q", value)
		}
	}
	for _, binding := range m.bindings().all {
		if h := binding.Help(); (h.Key == "r" || h.Key == "R") && h.Desc != "reply" {
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
			rows := map[int]int{5: 1, 25: 3}[width]
			if target.h != rows {
				t.Fatalf("commit button at width %d takes %d rows, want %d", width, target.h, rows)
			}
			if blank := strings.TrimSpace(ansi.Strip(lines[target.y-1])); blank != "" || strings.TrimSpace(ansi.Strip(lines[target.y-2])) != "" {
				t.Fatal("commit button needs two blank rows above it")
			}
			line := lines[target.y+target.h/2]
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
