package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func linksApp(t *testing.T, body string) (AppModel, *[]string) {
	t.Helper()
	m, _ := profileApp(t)
	opened := &[]string{}
	m.openURL = func(_ context.Context, raw string) error {
		*opened = append(*opened, raw)
		return nil
	}
	c := fixtureCommit("links")
	c.Body = body
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	return m, opened
}

func TestOpenLinkShortcutOpensTheOnlyLinkOrAsksWhichOne(t *testing.T) {
	m, opened := linksApp(t, "Nothing to open here")
	m = settle(t, m, "x")
	if m.notice != "No links to open." || len(*opened) != 0 {
		t.Fatalf("x without links: %q %v", m.notice, *opened)
	}
	m, opened = linksApp(t, "Read https://example.com/one")
	m = settle(t, m, "x")
	if m.picker != nil || strings.Join(*opened, " ") != "https://example.com/one" {
		t.Fatalf("x with one link: %v", *opened)
	}
	m, opened = linksApp(t, "Read https://example.com/one and https://example.org/two, thanks @ada")
	m = settle(t, m, "x")
	if m.picker == nil || len(m.picker.rows) != 2 || !strings.Contains(frameText(m), "Open a link") {
		t.Fatalf("x with two links and a mention did not ask between the two links:\n%s", frameText(m))
	}
	m = pickSecond(m)
	if m.picker != nil || strings.Join(*opened, " ") != "https://example.org/two" {
		t.Fatalf("picking the second link: %v", *opened)
	}
}

func TestClickingALinkOpensItAndAMentionFallsBackToTheOrganization(t *testing.T) {
	m, opened := linksApp(t, "Read example.com/docs with @laravel")
	m, cmd := clickText(t, m, "example.com/docs")
	settleCmd(m, cmd)
	if strings.Join(*opened, " ") != "https://example.com/docs" {
		t.Fatalf("clicking a link: %v", *opened)
	}
	var organizations []string
	m.loadProfile = func(context.Context, string) (api.Profile, error) {
		return api.Profile{}, &auth.APIError{Status: 404}
	}
	m.organizationCalls.load = func(_ context.Context, handle string) (api.OrganizationPage, error) {
		organizations = append(organizations, handle)
		return api.OrganizationPage{}, nil
	}
	m, cmd = clickText(t, m, "@laravel")
	m = settleCmd(m, cmd)
	if len(m.profiles) != 1 || strings.Join(organizations, " ") != "laravel" {
		t.Fatalf("a mention of an organization did not fall back to it: %v", organizations)
	}
}

// pickSecond chooses a picker's second row through Update, which routes keys to an open picker.
func pickSecond(m AppModel) AppModel {
	m = pressApp(m, "j")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return settleCmd(next.(AppModel), cmd)
}
