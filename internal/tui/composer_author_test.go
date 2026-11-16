package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func postAsApp(t *testing.T, organizations []auth.Organization) (AppModel, *[]api.NewCommit) {
	t.Helper()
	m, _ := organizationApp(t)
	m.account.user.Organizations = organizations
	var sent []api.NewCommit
	m.submit = func(_ context.Context, draft api.NewCommit) (api.Commit, error) {
		sent = append(sent, draft)
		c := fixtureCommit("new")
		c.Status = "approved"
		return c, nil
	}
	return settle(t, m, "c"), &sent
}

// compose sends keys through Update, which gives the open composer its keys first.
func compose(t *testing.T, m AppModel, keys ...string) AppModel {
	t.Helper()
	for _, name := range keys {
		msg := tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
		switch {
		case name == "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case name == "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case name == "home":
			msg = tea.KeyPressMsg{Code: tea.KeyHome}
		case name == "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case name == "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case name == "left":
			msg = tea.KeyPressMsg{Code: tea.KeyLeft}
		case name == "right":
			msg = tea.KeyPressMsg{Code: tea.KeyRight}
		case strings.HasPrefix(name, "ctrl+"):
			msg = tea.KeyPressMsg{Code: rune(name[5]), Mod: tea.ModCtrl}
		}
		next, cmd := m.Update(msg)
		m = settleCmd(next.(AppModel), cmd)
	}
	return m
}

func TestPostAsPicksAnOrganizationFromTheAccount(t *testing.T) {
	m, sent := postAsApp(t, []auth.Organization{{ID: "o1", Role: "admin", Handle: "laravel", Name: "Laravel"}})
	m = typeText(m, "Laravel 13 is out")
	m = compose(t, m, "ctrl+a")
	if frame := frameText(m); m.composer.picker == nil || !strings.Contains(frame, "Post as") || !strings.Contains(frame, "@fixture") || !strings.Contains(frame, "@laravel") {
		t.Fatalf("picker:\n%s", frame)
	}
	if m.composer.input.Value() != "Laravel 13 is out" {
		t.Fatalf("ctrl+a changed the text: %q", m.composer.input.Value())
	}
	m = compose(t, m, "j", "enter")
	if m.composer.picker != nil || m.composer.author == nil || m.composer.author.Handle != "laravel" || !strings.Contains(frameText(m), "Laravel @laravel") {
		t.Fatalf("chosen author:\n%s", frameText(m))
	}
	m = compose(t, m, "ctrl+s")
	if len(*sent) != 1 || (*sent)[0].Author != "laravel" {
		t.Fatalf("sent %+v", *sent)
	}
}

func TestPostAsCanSwitchBackToYourself(t *testing.T) {
	m, sent := postAsApp(t, []auth.Organization{{Handle: "laravel"}})
	m, _ = clickText(t, compose(t, m, "ctrl+a", "j", "enter"), "@laravel")
	if m.composer.picker.index != 1 {
		t.Fatalf("picker does not start on the current author: %d", m.composer.picker.index)
	}
	m = typeText(compose(t, m, "k", "enter"), "hello")
	m = compose(t, m, "ctrl+s")
	if m.composer.author != nil || len(*sent) != 1 || (*sent)[0].Author != "" {
		t.Fatalf("sent %+v", *sent)
	}
}

func TestPostAsSaysWhenYouHaveNoOrganizations(t *testing.T) {
	for name, organizations := range map[string][]auth.Organization{"none": {}, "a server without the list": nil} {
		m, _ := postAsApp(t, organizations)
		m = compose(t, m, "ctrl+a")
		if m.composer.picker != nil || !strings.Contains(m.composer.status, "member of any organization") {
			t.Fatalf("%s: %q", name, m.composer.status)
		}
	}
}

func TestHomeStillMovesToTheLineStart(t *testing.T) {
	m, _ := postAsApp(t, []auth.Organization{})
	m = typeText(compose(t, typeText(m, "world"), "home"), "hello ")
	if m.composer.input.Value() != "hello world" {
		t.Fatalf("home: %q", m.composer.input.Value())
	}
}

func TestClicksWaitWhilePostAsIsOpen(t *testing.T) {
	m, sent := postAsApp(t, []auth.Organization{{Handle: "laravel"}})
	m = compose(t, typeText(m, "hello"), "ctrl+a", "j")
	for _, area := range m.composerControls() {
		next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: area.x, Y: area.y})
		m = next.(AppModel)
	}
	if m.composer.picker == nil || m.composer.author != nil || len(*sent) != 0 {
		t.Fatalf("a footer click acted through the picker: %+v %v", m.composer.author, *sent)
	}
}

func TestPostAsChipShowsOnlyWithOrganizations(t *testing.T) {
	m, _ := postAsApp(t, []auth.Organization{{ID: "org-acme", Handle: "acme", Name: "Acme"}})
	// The chip is the strip's last, which a narrow composer drops first.
	wide, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	next, cmd := clickText(t, wide.(AppModel), "Post as  ctrl+a")
	if m = settleCmd(next, cmd); m.composer.picker == nil {
		t.Fatal("the Post as chip did not open Post as")
	}
	m, _ = postAsApp(t, nil)
	if strings.Contains(frameText(m), "Post as") {
		t.Fatal("Post as offered without organizations")
	}
}
