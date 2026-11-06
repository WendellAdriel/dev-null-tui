package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func achievementsProfile(handle string) api.Profile {
	profile := fixtureProfile(handle)
	profile.Achievements.Badges = []api.Badge{
		{ID: "init", Name: "init", Description: "Piped your first commit to /dev/null.", Icon: "terminal", Unlocked: true, Progress: 1, Target: 1},
		{ID: "uptime", Name: "uptime", Description: "Maintained a 7-day commit streak without a reboot.", Icon: "flame", Progress: 6, Target: 7},
		{ID: "stdout", Name: "stdout", Description: "Reached 50 commits piped to /dev/null.", Icon: "box", Unlocked: true, Progress: 50, Target: 50},
		{ID: "luddite", Name: "luddite", Description: "Unfollowed Tom.", Icon: "bot-off", Progress: 0, Target: 1},
	}
	return profile
}

func achievementsApp(t *testing.T) AppModel {
	t.Helper()
	m, _ := profileApp(t)
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) { return achievementsProfile(handle), nil }
	return deliver(m, runAll(m.openProfile("grace")))
}

func TestTheShelfShowsBadgeIconsOrTheirGlyphs(t *testing.T) {
	m := achievementsApp(t)
	content := ansi.Strip(m.topProfile().viewport.GetContent())
	for _, want := range []string{badgeGlyphs["award"] + " Lv. 5 · wheel", badgeGlyphs["terminal"] + " init", badgeGlyphs["box"] + " stdout"} {
		if !strings.Contains(content, want) {
			t.Fatalf("the shelf lacks %q:\n%s", want, content)
		}
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "init") {
			continue
		}
		if !strings.HasSuffix(strings.TrimSpace(line), "stdout     A") {
			t.Fatalf("the badge row doesn't end with its A shortcut: %q", line)
		}
		if !strings.HasPrefix(strings.TrimSpace(lines[i-1]), "▄▄▄") || !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "▀▀▀") {
			t.Fatalf("the badge chips lack their half-block padding:\n%s\n%s\n%s", lines[i-1], line, lines[i+1])
		}
	}
	m.topProfile().profile.Achievements.Badges = append(m.topProfile().profile.Achievements.Badges, achievementsProfile("x").Achievements.Badges...)
	for i := range 8 {
		m.topProfile().profile.Achievements.Badges = append(m.topProfile().profile.Achievements.Badges, api.Badge{ID: fmt.Sprint(i), Name: fmt.Sprintf("badge %d", i), Unlocked: true})
	}
	m.renderProfile(m.topProfile())
	for _, line := range strings.Split(ansi.Strip(m.topProfile().viewport.GetContent()), "\n") {
		if strings.Contains(line, "init") && (!strings.Contains(line, " more   A") || lipgloss.Width(line) > m.topProfile().viewport.Width()) {
			t.Fatalf("a full badge row lacks +N more and its A shortcut, or overflows: %q", line)
		}
	}

	m.images.Enabled = true
	p := colors(m.account.dark)
	req := badgeRequest("terminal", p.ink)
	if req != (graphics.Request{Icon: "badge-terminal", Ring: p.ink, Cols: 2, Rows: 1}) {
		t.Fatalf("badge request = %+v", req)
	}
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{req})
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	m.renderProfile(m.topProfile())
	for _, line := range strings.Split(m.topProfile().viewport.GetContent(), "\n") {
		if strings.Contains(ansi.Strip(line), "init") && strings.Contains(line, string(kitty.Placeholder)) {
			return
		}
	}
	t.Fatal("the init badge kept its glyph once its icon was ready")
}

func TestAOpensAllAchievementsWithEveryBadge(t *testing.T) {
	m := achievementsApp(t)
	m, _ = m.handleKey("A")
	if m.achievements == nil {
		t.Fatal("A did not open All achievements")
	}
	frame := ansi.Strip(m.View().Content)
	for _, want := range []string{"All achievements", "init", "uptime (6/7)", "stdout", "luddite"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the popup lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "luddite (0/1)") {
		t.Fatal("a one-step badge shows its progress")
	}
	if !strings.Contains(frame, "init ✓") || !strings.Contains(frame, "Piped your first commit") {
		t.Fatalf("the first badge's details are missing:\n%s", frame)
	}
	m, _ = m.mouseKey("right")
	frame = ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "uptime (6/7)") || !strings.Contains(frame, "7-day commit streak") {
		t.Fatalf("moving right did not show uptime's details:\n%s", frame)
	}
	m, _ = m.mouseKey("esc")
	if m.achievements != nil || m.topProfile() == nil {
		t.Fatal("esc did not close only the popup")
	}
}

func TestClicksOpenAllAchievementsAndSelectABadge(t *testing.T) {
	m := achievementsApp(t)
	s := m.topProfile()
	var shelf *hitArea
	for i, area := range s.actions {
		if area.action == "A" {
			shelf = &s.actions[i]
		}
	}
	if shelf == nil {
		t.Fatal("the badge row has no click target")
	}
	s.viewport.SetYOffset(max(0, shelf.y-2))
	m, _ = m.profileMouse(shelf.x, shelf.y+threadHeaderHeight-s.viewport.YOffset())
	if m.achievements == nil {
		t.Fatal("clicking the badge row did not open All achievements")
	}
	_, _, areas := m.achievementsModal(colors(m.account.dark))
	for _, area := range areas {
		if area.action == "badge" && area.index == 3 {
			m, _ = m.achievementsMouse(area.x, area.y)
			if m.achievements.selected != 3 || !strings.Contains(ansi.Strip(m.View().Content), "Unfollowed Tom.") {
				t.Fatal("clicking luddite did not select it")
			}
			return
		}
	}
	t.Fatal("luddite has no click target")
}

func TestOutsideClicksCloseAllAchievements(t *testing.T) {
	m := achievementsApp(t)
	m, _ = m.handleKey("A")
	if m, _ = m.achievementsMouse(0, 0); m.achievements != nil {
		t.Fatal("a click outside the popup left it open")
	}
}

func TestTheWheelMovesThePopupSelectionNotThePage(t *testing.T) {
	m := achievementsApp(t)
	m, _ = m.handleKey("A")
	offset := m.topProfile().viewport.YOffset()
	next, _ := m.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(AppModel)
	if m.achievements == nil || m.achievements.selected != 1 || m.topProfile().viewport.YOffset() != offset {
		t.Fatalf("the wheel scrolled the page to %d instead of moving the selection", m.topProfile().viewport.YOffset())
	}
}

func TestThePopupKeepsTheDescriptionAt80x24(t *testing.T) {
	m := achievementsApp(t)
	s := m.topProfile()
	var badges []api.Badge
	for _, name := range []string{"init", "syntax", "uptime", "daemon", "sysadmin", "stdout", "kernel space", "stargazer", "superstar", "pipe", "authorized_keys", "sweet tooth", "luddite", "shell shocked", "super-saiyan", "whoopsie"} {
		badges = append(badges, api.Badge{ID: name, Name: name, Icon: "terminal", Description: "Description of " + name + ".", Progress: 85, Target: 100})
	}
	s.profile.Achievements.Badges = badges
	m.account.width, m.account.height = 80, 24
	m, _ = m.handleKey("A")
	m, _ = m.mouseKey("G")
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "Description of whoopsie.") || !strings.Contains(frame, "whoopsie (85/100)") || !strings.Contains(frame, "↑ more") {
		t.Fatalf("the description or the selected chip is cut off at 80x24:\n%s", frame)
	}
	m, _ = m.mouseKey("g")
	if frame = ansi.Strip(m.View().Content); !strings.Contains(frame, "init (85/100)") || !strings.Contains(frame, "↓ more") {
		t.Fatalf("the chip rows did not scroll back to the first badge:\n%s", frame)
	}
}

func TestShelfIconsAreRequestedOnlyWhileVisible(t *testing.T) {
	m := achievementsApp(t)
	s := m.topProfile()
	p := colors(m.account.dark)
	icon := badgeRequest("box", p.ink)
	var line int
	for _, media := range s.media {
		if media.req == icon {
			line = media.line
		}
	}
	if line < s.viewport.Height() {
		t.Fatalf("the fixture shelf must start below the first screen: line %d, view %d", line, s.viewport.Height())
	}
	s.viewport.SetYOffset(0)
	if slices.Contains(m.profileAvatars(s, p), icon) {
		t.Fatal("a shelf icon below the view is requested")
	}
	s.viewport.SetYOffset(line)
	if !slices.Contains(m.profileAvatars(s, p), icon) {
		t.Fatal("a visible shelf icon is not requested")
	}
}

func TestAllAchievementsNeedsAchievements(t *testing.T) {
	m, _ := profileApp(t)
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		profile := fixtureProfile(handle)
		profile.Achievements = nil
		return profile, nil
	}
	m = deliver(m, runAll(m.openProfile("grace")))
	if m, _ = m.handleKey("A"); m.achievements != nil {
		t.Fatal("A opened All achievements on a profile without them")
	}
	m, _ = organizationApp(t)
	m = press(t, m, "u")
	if m, _ = m.handleKey("A"); !m.profiles[0].organization || m.achievements != nil {
		t.Fatal("A opened All achievements on an organization page")
	}
}
