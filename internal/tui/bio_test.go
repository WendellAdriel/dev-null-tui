package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestBioTagsBecomeChipsThatOpenTheProfileOrGitHub(t *testing.T) {
	m, calls := profileApp(t)
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		calls.profiles = append(calls.profiles, handle)
		profile := fixtureProfile(handle)
		if handle == "ada" {
			profile.User.Bio = "Building @Laravel\r\npreviously @vercel #php, mail me@example.com or use @inertiajs/react"
			profile.User.Company = "@laravel"
			profile.BioProfiles = map[string]string{"laravel": "laravel"}
		}
		return profile, nil
	}
	m = press(t, m, "u")
	page := m.profiles[0].viewport.GetContent()
	for _, want := range []string{
		ansi.SetHyperlink(strings.TrimRight(m.account.origin, "/") + "/@laravel"),
		ansi.SetHyperlink("https://github.com/vercel"),
		hashtagStart + "%23php",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("bio lacks %q:\n%q", want, page)
		}
	}
	for _, plain := range []string{"github.com/example", "github.com/inertiajs", "github.com/me"} {
		if strings.Contains(page, plain) {
			t.Fatalf("bio linked %q, which the web leaves as text", plain)
		}
	}
	text := ansi.Strip(page)
	if !strings.Contains(text, "Building ◆  @Laravel ") || !strings.Contains(text, "previously ◆  @vercel  #php") {
		t.Fatalf("bio chips or line break missing:\n%s", text)
	}
	m, _ = clickText(t, m, "@Laravel")
	if len(m.profiles) != 2 || m.profiles[1].handle != "laravel" || !m.profiles[1].mention {
		t.Fatalf("clicking a dev/null chip did not open its profile: %d profiles", len(m.profiles))
	}
}
