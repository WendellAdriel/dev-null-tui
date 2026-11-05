package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func TestSidebarIdentityAndVersion(t *testing.T) {
	m := fixtureApp(t).WithVersion("v1.2.3")
	m.account.user.Name = "Fixture Name"
	m.account.user.Handle = "fixture"
	for _, dark := range []bool{false, true} {
		p := colors(dark)
		lines := strings.Split(ansi.Strip(m.navigation(25, 34, p)), "\n")
		name, handle := -1, -1
		for i, line := range lines {
			if strings.Contains(line, "Fixture Name") {
				name = i
			}
			if strings.Contains(line, "@fixture") {
				handle = i
			}
		}
		if name < 0 || handle != name+1 {
			t.Fatalf("name and handle must occupy adjacent rows:\n%s", strings.Join(lines, "\n"))
		}
		if ansi.StringWidth(strings.SplitN(lines[name], "Fixture Name", 2)[0]) != ansi.StringWidth(strings.SplitN(lines[handle], "@fixture", 2)[0]) {
			t.Fatal("name and handle must align beside the avatar")
		}
		for _, width := range []int{36, 40} {
			for _, height := range []int{24, 34} {
				aside := strings.Split(ansi.Strip(m.aside(width, height, p)), "\n")
				at := slices.IndexFunc(aside, func(line string) bool { return strings.Contains(line, "/dev/null TUI") })
				// The version follows the shortcuts after a rule and one blank line, not the aside's bottom.
				if at < 2 || !strings.Contains(aside[at-2], "─") || strings.TrimSpace(aside[at-1]) != "" || strings.Contains(aside[0], "─") {
					t.Fatalf("version placement at %dx%d:\n%s", width, height, strings.Join(aside, "\n"))
				}
				for i, label := range []string{"/dev/null TUI", "v1.2.3"} {
					line := aside[at+i]
					left := strings.Index(line, label)
					right := width - left - len(label)
					if left < 0 || left-right < -1 || left-right > 1 {
						t.Fatalf("footer %q not centered at %dx%d: %q", label, width, height, line)
					}
				}
			}
		}
	}
}

func TestSidebarGraphicsAndCompactFallback(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 35
	m.account.user.AvatarURL = "https://example.com/avatar.png"
	m.images.Enabled = true
	p := colors(m.account.dark)
	logo := graphics.Request{Icon: "logo-dark", Cols: 23, Rows: 2}
	if !m.account.dark {
		logo.Icon = "logo-light"
	}
	avatar := m.avatarRequest(m.account.user.AvatarURL, laneColor(m.account.user.Lane, p), 4, 2)
	media := m.visibleMedia()
	if !slices.Contains(media, logo) || !slices.Contains(media, avatar) {
		t.Fatal("sidebar logo and two-row avatar must be scheduled")
	}
	jobs, _ := m.images.Plan(context.Background(), []graphics.Request{logo})
	if len(jobs) != 1 {
		t.Fatal("local logo was not scheduled")
	}
	result := m.images.Fetch(jobs[0])
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	m.images.Accept(result)
	column := m.navigation(25, 34, p)
	if strings.Count(column, string(kitty.Placeholder)) != 46 || strings.Contains(ansi.Strip(column), "/dev/null") {
		t.Fatal("header must display the full logo without a duplicate text wordmark")
	}
	m.account.width = 80
	if slices.Contains(m.visibleMedia(), logo) {
		t.Fatal("compact sidebar must not schedule a hidden logo")
	}
	m.images.Enabled = false
	for _, width := range []int{5, 25} {
		column := ansi.Strip(m.navigation(width, 23, p))
		if !strings.Contains(column, "//") || strings.Contains(column, "…") {
			t.Fatal("text fallback must fit without truncation markers")
		}
	}
}
