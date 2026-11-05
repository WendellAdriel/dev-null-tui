package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func orgCommit(id string) api.Commit {
	c := fixtureCommit(id)
	c.Author = &api.Author{Type: "organization", ID: "org", Handle: "acme", Name: "Acme", Lane: []byte("3")}
	return c
}

func shortHelp(m AppModel) string {
	var keys []string
	for _, b := range m.bindings().short {
		keys = append(keys, b.Help().Key+" "+b.Help().Desc)
	}
	return strings.Join(keys, " · ")
}

type organizationLog struct {
	calls    []string
	page     api.OrganizationPage
	claimErr error
	opened   []string
}

func (l *organizationLog) add(format string, args ...any) {
	l.calls = append(l.calls, fmt.Sprintf(format, args...))
}

func (l *organizationLog) String() string { return strings.Join(l.calls, "|") }

func fixtureOrganization(handle string) api.OrganizationPage {
	followers := 12
	return api.OrganizationPage{
		Organization: api.Organization{Handle: handle, Name: "Acme Inc", Bio: "We build rockets", Location: "Porto", Website: "https://acme.dev", GitHubURL: "https://github.com/" + handle, Followers: &followers, Admins: []api.User{{ID: "id-taylor", Handle: "taylor"}}},
	}
}

func organizationApp(t *testing.T) (AppModel, *organizationLog) {
	t.Helper()
	m, _ := profileApp(t)
	log := &organizationLog{page: fixtureOrganization("acme")}
	m.organizationCalls = organizationCalls{
		load: func(_ context.Context, handle string) (api.OrganizationPage, error) {
			log.add("load %s", handle)
			page := log.page
			page.Organization.Handle = handle
			return page, nil
		},
		commits: func(_ context.Context, handle string, mentions bool, cursor string) (api.CommitPage, error) {
			log.add("commits %s mentions=%v cursor=%s", handle, mentions, cursor)
			c := fixtureCommit(handle + "-commit")
			c.Body = "Commit by " + handle
			if mentions {
				c.Body = "Mention of @" + handle
			}
			return api.CommitPage{Data: []api.Commit{c}}, nil
		},
		follow: func(_ context.Context, handle string, follow bool) error {
			log.add("follow %s %v", handle, follow)
			return nil
		},
		claim: func(_ context.Context, handle string) error {
			log.add("claim %s", handle)
			return log.claimErr
		},
		followed: func(_ context.Context, handle, cursor string) (api.Organizations, error) {
			log.add("followed %s cursor=%s", handle, cursor)
			yes := true
			page := api.Organizations{Data: []api.Organization{{Handle: "acme", Name: "Acme Inc", Bio: "We build rockets", IsFollowed: &yes}}}
			if cursor == "" {
				page.Next = "more"
			}
			return page, nil
		},
	}
	m.openURL = func(_ context.Context, raw string) error {
		log.opened = append(log.opened, raw)
		return nil
	}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{orgCommit("o1"), fixtureCommit("u1")}})
	return m, log
}

func TestOrganizationAuthorsOpenTheOrganizationPage(t *testing.T) {
	m, log := organizationApp(t)
	if content := ansi.Strip(m.feeds[0].viewport.GetContent()); !strings.Contains(content, "Profile  u ") {
		t.Fatalf("organization commit lacks a profile chip:\n%s", content)
	}
	m = press(t, m, "u")
	if len(m.profiles) != 1 || !m.profiles[0].organization || log.String() != "load acme|commits acme mentions=false cursor=" {
		t.Fatalf("opening the organization: %s", log)
	}
	frame := profileText(m)
	for _, want := range []string{"Acme Inc", "@acme", "Organization", "We build rockets", "Porto", "acme.dev", "github.com/acme", "12 followers", "Admins @taylor", "Follow  f", "Commits", "Mentions", "Commit by acme"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("organization page lacks %q:\n%s", want, frame)
		}
	}
	for _, unwanted := range []string{"Claim", "Settings", "Mute", "Commit activity"} {
		if strings.Contains(frame, unwanted) {
			t.Fatalf("organization page shows %q:\n%s", unwanted, frame)
		}
	}
	m = press(t, m, "tab")
	if !strings.HasSuffix(log.String(), "commits acme mentions=true cursor=") || !strings.Contains(profileText(m), "Mention of @acme") {
		t.Fatalf("mentions tab: %s", log)
	}
	m = press(t, m, "esc")
	if len(m.profiles) != 0 || m.feeds[0].selected != 0 {
		t.Fatal("leaving the organization lost the log row")
	}
}

func TestOrganizationAuthorsOpenFromThreadsStarredAndNotifications(t *testing.T) {
	m, log := organizationApp(t)
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) { return api.Thread{Commit: orgCommit(id)}, nil }
	m.loadReplies = func(context.Context, string, string) (api.CommitPage, error) { return api.CommitPage{}, nil }
	m = press(t, m, "enter")
	if !strings.Contains(frameText(m), "Profile") || !strings.Contains(shortHelp(m), "profile") {
		t.Fatalf("organization thread entry lacks a profile: %s", shortHelp(m))
	}
	m = press(t, m, "u")
	if len(m.profiles) != 1 || !m.profiles[0].organization || !strings.Contains(log.String(), "load acme") {
		t.Fatalf("thread author: %s", log)
	}

	starred, _ := starredApp(t)
	starred.organizationCalls = m.organizationCalls
	starred.loadStarred = func(context.Context, string) (api.CommitPage, error) {
		return api.CommitPage{Data: []api.Commit{orgCommit("s1")}}, nil
	}
	starred, _ = starred.handleKey("g")
	starred = press(t, starred, "s", "u")
	if len(starred.profiles) != 1 || !starred.profiles[0].organization {
		t.Fatal("starred organization author did not open its page")
	}

	inbox := inboxFixture(t)
	inbox.organizationCalls = m.organizationCalls
	n := fixtureNotification("n1")
	n.Actor = orgCommit("x").Author
	inbox = press(t, inboxPage(inbox, n), "u")
	if len(inbox.profiles) != 1 || !inbox.profiles[0].organization {
		t.Fatal("organization actor did not open its page")
	}
}

func TestFollowingAnOrganizationMovesItsFollowersAndYourFollowing(t *testing.T) {
	m, log := organizationApp(t)
	following := 4
	m.account.user.Following = &following
	m = press(t, m, "u", "f")
	if !strings.HasSuffix(log.String(), "follow acme true") || !m.profiles[0].page.IsFollowing || *m.profiles[0].page.Organization.Followers != 13 || *m.account.user.Following != 5 {
		t.Fatalf("follow: %s %+v", log, m.profiles[0].page)
	}
	if frame := profileText(m); !strings.Contains(frame, "Following  f") || !strings.Contains(frame, "13 followers") || m.toast.title != "Following" {
		t.Fatalf("followed page:\n%s", frame)
	}
	m = press(t, m, "f")
	if !strings.HasSuffix(log.String(), "follow acme false") || m.profiles[0].page.IsFollowing || *m.account.user.Following != 4 {
		t.Fatalf("unfollow: %s", log)
	}
}

func TestClaimingAnUnclaimedOrganization(t *testing.T) {
	m, log := organizationApp(t)
	log.page.CanClaim, log.page.Organization.Admins = true, nil
	m = settle(t, m, "u")
	if frame := profileText(m); !strings.Contains(frame, "Nobody has claimed @acme yet") || !strings.Contains(frame, "Claim  C") {
		t.Fatalf("unclaimed page:\n%s", frame)
	}
	log.claimErr = &auth.APIError{Status: http.StatusForbidden, Message: "Make your membership of @acme public on GitHub, then claim it again."}
	m = settle(t, m, "C")
	if !strings.HasSuffix(log.String(), "claim acme|load acme") || m.toast.title != "Couldn't claim @acme" || !strings.Contains(m.toast.detail, "public on GitHub") {
		t.Fatalf("refused claim: %s %q", log, m.toast.detail)
	}
	log.claimErr = nil
	log.page.CanClaim, log.page.Organization.Admins = false, []api.User{{ID: "id-fixture", Handle: "fixture"}}
	settings := "https://example.test/@acme/settings"
	log.page.SettingsURL = &settings
	m = settle(t, m, "C")
	if !strings.HasSuffix(log.String(), "claim acme|load acme") || m.toast.title != "Claimed @acme" {
		t.Fatalf("claim: %s %q", log, m.toast.title)
	}
	frame := profileText(m)
	if strings.Contains(frame, "Nobody has claimed") || !strings.Contains(frame, "Settings  S") || !strings.Contains(frame, "Admins @fixture") {
		t.Fatalf("claimed page:\n%s", frame)
	}
	m = settle(t, m, "S")
	if strings.Join(log.opened, " ") != settings {
		t.Fatalf("settings opened %v", log.opened)
	}
}

func TestAMissingOrganizationSaysSo(t *testing.T) {
	m, _ := organizationApp(t)
	m.organizationCalls.load = func(context.Context, string) (api.OrganizationPage, error) {
		return api.OrganizationPage{}, &auth.APIError{Status: http.StatusNotFound}
	}
	m = press(t, m, "u")
	if frame := profileText(m); !strings.Contains(frame, "This organization doesn't exist.") {
		t.Fatalf("missing organization:\n%s", frame)
	}
}

func TestTheFollowingListShowsOrganizationsAbovePeople(t *testing.T) {
	m, log := organizationApp(t)
	m, _ = m.selectCommit(1)
	m = press(t, m, "u", "O")
	frame := profileText(m)
	if !strings.Contains(log.String(), "followed ada cursor=") || !strings.Contains(frame, "Organizations") || !strings.Contains(frame, "More organizations  o") {
		t.Fatalf("following list: %s\n%s", log, frame)
	}
	if strings.Index(frame, "Acme Inc") > strings.Index(frame, "Linus") {
		t.Fatalf("organizations are not above the people:\n%s", frame)
	}
	m = press(t, m, "o")
	if !strings.HasSuffix(log.String(), "followed ada cursor=more") || strings.Contains(profileText(m), "More organizations") {
		t.Fatalf("more organizations: %s", log)
	}
	m = press(t, m, "f")
	if !strings.HasSuffix(log.String(), "follow acme false") {
		t.Fatalf("unfollow from the list: %s", log)
	}
	m = press(t, m, "j", "j", "k", "k", "enter")
	if len(m.profiles) != 3 || !m.profiles[2].organization {
		t.Fatalf("enter on an organization row did not open it: %s", log)
	}
	m = press(t, m, "esc", "F")
	if strings.Contains(profileText(m), "Organizations") {
		t.Fatal("the Followers list shows organizations")
	}
}

func TestMuteAndBlockDoNothingOnAnOrganizationPage(t *testing.T) {
	m, _ := organizationApp(t)
	m = press(t, m, "u", "M", "B")
	if m.settings.dialog != "" {
		t.Fatalf("an organization page opened %q", m.settings.dialog)
	}
}

func TestASelectedPersonKeepsItsRowAsOrganizationsLoad(t *testing.T) {
	m, _ := organizationApp(t)
	m, _ = m.selectCommit(1)
	m = press(t, m, "u", "O", "j")
	s := m.profiles[len(m.profiles)-1]
	if l := s.lists[s.tab]; l.selected != len(l.organizations) {
		t.Fatalf("expected the first person, got row %d", l.selected)
	}
	m = press(t, m, "o")
	s = m.profiles[len(m.profiles)-1]
	l := s.lists[s.tab]
	if l.selected != len(l.organizations) || l.people[l.selected-len(l.organizations)].Handle != "linus" {
		t.Fatalf("selection moved off the person: %d of %d organizations", l.selected, len(l.organizations))
	}
}

func TestAnEmptyMentionsTabWrapsItsMessage(t *testing.T) {
	m, _ := organizationApp(t)
	m.organizationCalls.commits = func(context.Context, string, bool, string) (api.CommitPage, error) {
		return api.CommitPage{}, nil
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(t, next.(AppModel), "u", "tab")
	s := m.profiles[0]
	content := ansi.Strip(s.viewport.GetContent())
	if !strings.Contains(strings.Join(strings.Fields(content), " "), "Nobody has mentioned @acme yet. Mention it in a commit and it shows up here.") {
		t.Fatalf("the empty mentions message is missing:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if ansi.StringWidth(line) > s.viewport.Width() {
			t.Fatalf("%q is wider than the %d-column page", line, s.viewport.Width())
		}
	}
}
