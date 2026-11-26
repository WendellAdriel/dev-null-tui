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

func TestAMentionOpensWhoItCapturedEvenAfterARename(t *testing.T) {
	const ada, acme = "0199c8a4-0000-7000-8000-000000000001", "0199c8a4-0000-7000-8000-000000000002"
	m, _ := profileApp(t)
	c := fixtureCommit("mentions")
	c.Body = "Thanks @lovelace, @acme and @ghost"
	c.Mentions = []api.Mention{{Handle: "lovelace", Kind: "user", ID: ada}, {Handle: "acme", Kind: "organization", ID: acme}, {Handle: "ghost", Kind: "user"}}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	view := m.View().Content
	if !strings.Contains(view, "/mentions/"+ada) || !strings.Contains(view, "/mentions/organizations/"+acme) || !strings.Contains(view, "/@ghost") {
		t.Fatal("mentions with an id must link to the profile they captured, and the rest to /@handle")
	}
	for _, link := range []string{"https://example.test/mentions/organizations", "https://example.test/mentions/not-an-id"} {
		if _, ok := m.mentionTarget(link); ok {
			t.Fatalf("%s is not a captured mention", link)
		}
	}
	var users, organizations, commits []string
	m.loadMentionedUser = func(_ context.Context, id string) (api.Profile, error) {
		users = append(users, id)
		return fixtureProfile("lovelace-renamed"), nil
	}
	m.loadUserCommits = func(_ context.Context, handle string, _ bool, _ string) (api.CommitPage, error) {
		commits = append(commits, handle)
		return api.CommitPage{}, nil
	}
	m.organizationCalls.mentioned = func(_ context.Context, id string) (api.OrganizationPage, error) {
		organizations = append(organizations, id)
		return api.OrganizationPage{Organization: api.Organization{Handle: "acme-renamed"}}, nil
	}
	m.organizationCalls.commits = func(_ context.Context, handle string, _ bool, _ string) (api.CommitPage, error) {
		commits = append(commits, handle)
		return api.CommitPage{}, nil
	}
	m, cmd := clickText(t, m, "@lovelace")
	m = settleCmd(m, cmd)
	if strings.Join(users, " ") != ada || strings.Join(commits, " ") != "lovelace-renamed" || m.profileHandle() != "lovelace-renamed" {
		t.Fatalf("@lovelace must open its captured user and list that user's commits: %v %v %q", users, commits, m.profileHandle())
	}
	m = pressApp(m, "esc")
	m, cmd = clickText(t, m, "@acme")
	m = settleCmd(m, cmd)
	if strings.Join(organizations, " ") != acme || !m.topProfile().organization || m.profileHandle() != "acme-renamed" {
		t.Fatalf("@acme must open its captured organization: %v %q", organizations, m.profileHandle())
	}
	depth := len(m.profiles)
	m = settleCmd(m, m.openMention(c.Mentions[1]))
	if len(m.profiles) != depth || m.profileHandle() != "acme-renamed" {
		t.Fatalf("a mention of the open organization must not stack it again: %d pages, want %d", len(m.profiles), depth)
	}

	reads := 0
	m.loadModerationHistory = func(context.Context) (api.ModerationHistory, error) {
		reads++
		return api.ModerationHistory{}, nil
	}
	m.loadMentionedUser = func(context.Context, string) (api.Profile, error) { return fixtureProfile(m.account.user.Handle), nil }
	m = settleCmd(m, m.openMention(c.Mentions[0]))
	if !m.ownProfile(m.topProfile()) || reads != 1 {
		t.Fatalf("your own profile opened by a mention must load its moderation history: %d reads", reads)
	}
}

// pickSecond chooses a picker's second row through Update, which routes keys to an open picker.
func pickSecond(m AppModel) AppModel {
	m = pressApp(m, "j")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return settleCmd(next.(AppModel), cmd)
}
