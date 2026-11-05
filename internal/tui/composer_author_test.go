package tui

import (
	"context"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func postAsApp(t *testing.T, organizations *[]auth.Organization, canPost bool) (AppModel, *[]api.NewCommit) {
	t.Helper()
	m, _ := organizationApp(t)
	m.account.user.Organizations, m.account.user.CanPostAsOrganization = organizations, &canPost
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
	m, sent := postAsApp(t, &[]auth.Organization{{ID: "o1", Role: "admin", Handle: "laravel", Name: "Laravel"}}, true)
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
	m, sent := postAsApp(t, &[]auth.Organization{{Handle: "laravel"}}, true)
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

func TestPostAsSaysWhyItIsUnavailable(t *testing.T) {
	off, _ := postAsApp(t, &[]auth.Organization{{Handle: "laravel"}}, false)
	off = compose(t, off, "ctrl+a")
	if off.composer.picker != nil || !strings.Contains(off.composer.status, "isn't available yet") {
		t.Fatalf("feature off: %q", off.composer.status)
	}
	none, _ := postAsApp(t, &[]auth.Organization{}, true)
	none = compose(t, none, "ctrl+a")
	if none.composer.picker != nil || !strings.Contains(none.composer.status, "member of any organization") {
		t.Fatalf("no organizations: %q", none.composer.status)
	}
}

func TestPostAsChecksATypedHandleOnAnOlderServer(t *testing.T) {
	m, sent := postAsApp(t, nil, false)
	m.account.user.CanPostAsOrganization = nil
	settings := "https://example.test/@acme/settings"
	pages := map[string]api.OrganizationPage{
		"acme":    {Organization: api.Organization{Handle: "Acme", Name: "Acme Inc"}, SettingsURL: &settings},
		"pestphp": {Organization: api.Organization{Handle: "pestphp"}},
	}
	var checked []string
	m.organizationCalls.load = func(_ context.Context, handle string) (api.OrganizationPage, error) {
		checked = append(checked, handle)
		page, ok := pages[handle]
		if !ok {
			return api.OrganizationPage{}, &auth.APIError{Status: http.StatusNotFound}
		}
		return page, nil
	}
	m = compose(t, m, "ctrl+a")
	if m.composer.picker == nil || !m.composer.picker.typed || !strings.Contains(frameText(m), "organization you're a member of") {
		t.Fatalf("typed picker:\n%s", frameText(m))
	}
	next, _ := m.Update(tea.PasteMsg{Content: "pestphp"})
	m = compose(t, next.(AppModel), "enter")
	if m.composer.author != nil || m.composer.picker == nil || !strings.Contains(m.composer.picker.status, "only post as an organization you are a member of") {
		t.Fatalf("not a member: %+v", m.composer.picker)
	}
	for range len("pestphp") {
		m = compose(t, m, "backspace")
	}
	m = compose(t, typeText(m, "nope"), "enter")
	if !strings.Contains(m.composer.picker.status, "@nope isn't an organization on /dev/null") {
		t.Fatalf("missing: %q", m.composer.picker.status)
	}
	for range len("nope") {
		m = compose(t, m, "backspace")
	}
	m = compose(t, typeText(m, "@acme"), "enter")
	if m.composer.picker != nil || m.composer.author == nil || m.composer.author.Handle != "Acme" || strings.Join(checked, " ") != "pestphp nope acme" {
		t.Fatalf("member: %+v %v", m.composer.author, checked)
	}
	m = compose(t, typeText(m, "Ship it"), "ctrl+s")
	if len(*sent) != 1 || (*sent)[0].Author != "Acme" {
		t.Fatalf("sent %+v", *sent)
	}
}

func TestPostAsIgnoresTheCheckOfAnEarlierHandle(t *testing.T) {
	m, _ := postAsApp(t, nil, false)
	settings := "https://example.test/@acme/settings"
	m.organizationCalls.load = func(_ context.Context, handle string) (api.OrganizationPage, error) {
		return api.OrganizationPage{Organization: api.Organization{Handle: handle}, SettingsURL: &settings}, nil
	}
	m = compose(t, m, "ctrl+a")
	m = typeText(m, "foo")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(AppModel)
	stale := authorCheckResult{session: m.session, generation: m.composer.picker.generation, handle: "foo", page: api.OrganizationPage{Organization: api.Organization{Handle: "foo"}, SettingsURL: &settings}}
	m = compose(t, m, "esc", "ctrl+a")
	for range len("foo") {
		m = compose(t, m, "backspace")
	}
	m = typeText(m, "bar")
	next, _ = m.Update(stale)
	m = next.(AppModel)
	if m.composer.author != nil || m.composer.picker == nil {
		t.Fatalf("an earlier check chose the author: %+v", m.composer.author)
	}
}

func TestHomeStillMovesToTheLineStart(t *testing.T) {
	m, _ := postAsApp(t, &[]auth.Organization{}, true)
	m = typeText(compose(t, typeText(m, "world"), "home"), "hello ")
	if m.composer.input.Value() != "hello world" {
		t.Fatalf("home: %q", m.composer.input.Value())
	}
}

func TestClicksWaitWhilePostAsIsOpen(t *testing.T) {
	m, sent := postAsApp(t, &[]auth.Organization{{Handle: "laravel"}}, true)
	m = compose(t, typeText(m, "hello"), "ctrl+a", "j")
	for _, area := range m.composerControls() {
		next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: area.x, Y: area.y})
		m = next.(AppModel)
	}
	if m.composer.picker == nil || m.composer.author != nil || len(*sent) != 0 {
		t.Fatalf("a footer click acted through the picker: %+v %v", m.composer.author, *sent)
	}
}

type idleMsg struct{}

func TestAnInvalidHandleMessageStaysThroughCursorBlinks(t *testing.T) {
	m, _ := postAsApp(t, nil, false)
	m.account.user.CanPostAsOrganization = nil
	m = compose(t, typeText(compose(t, m, "ctrl+a"), "bad_handle"), "enter")
	next, _ := m.Update(idleMsg{})
	m = next.(AppModel)
	if m.composer.picker.status != "That isn't a valid handle." {
		t.Fatalf("status: %q", m.composer.picker.status)
	}
}
