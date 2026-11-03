package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
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

func TestOrganizationAuthorsHaveNoProfileActionUntilPhase12(t *testing.T) {
	m, calls := profileApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{orgCommit("o1"), fixtureCommit("u1")}})
	if content := ansi.Strip(m.feeds[0].viewport.GetContent()); strings.Contains(content, "Profile") {
		t.Fatalf("organization commit shows a profile chip:\n%s", content)
	}
	m = press(t, m, "u")
	if len(calls.profiles) != 0 || len(m.profiles) != 0 {
		t.Fatalf("organization author opened users/%v", calls.profiles)
	}
	m, _ = m.selectCommit(1)
	m = press(t, m, "u")
	if len(calls.profiles) != 1 || calls.profiles[0] != "ada" {
		t.Fatalf("user author did not open a profile: %v", calls.profiles)
	}
}

func TestOrganizationThreadEntriesHideProfileChipAndHint(t *testing.T) {
	m, calls := profileApp(t)
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) { return api.Thread{Commit: orgCommit(id)}, nil }
	m.loadReplies = func(context.Context, string, string) (api.CommitPage, error) { return api.CommitPage{}, nil }
	m = press(t, m, "enter")
	if len(m.threads) != 1 {
		t.Fatal("thread did not open")
	}
	if strings.Contains(frameText(m), "Profile") || strings.Contains(shortHelp(m), "profile") {
		t.Fatalf("organization thread entry offers a profile: %s", shortHelp(m))
	}
	m = press(t, m, "u")
	if len(calls.profiles) != 0 {
		t.Fatalf("organization author opened users/%v", calls.profiles)
	}
}

func TestOrganizationStarredEntriesHideProfileHint(t *testing.T) {
	m, _ := starredApp(t)
	m.loadStarred = func(context.Context, string) (api.CommitPage, error) {
		return api.CommitPage{Data: []api.Commit{orgCommit("s1")}}, nil
	}
	m, _ = m.handleKey("g")
	m = press(t, m, "s")
	if !m.starredOpen || strings.Contains(shortHelp(m), "profile") || strings.Contains(ansi.Strip(m.starred.viewport.GetContent()), "Profile") {
		t.Fatalf("organization starred entry offers a profile: %s", shortHelp(m))
	}
	m = press(t, m, "u")
	if len(m.profiles) != 0 {
		t.Fatal("organization author opened a profile")
	}
}

func TestOrganizationNotificationActorsHideProfileChipAndHint(t *testing.T) {
	m := inboxFixture(t)
	opened := 0
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		opened++
		return fixtureProfile(handle), nil
	}
	n := fixtureNotification("n1")
	n.Actor = orgCommit("x").Author
	m = inboxPage(m, n)
	if strings.Contains(ansi.Strip(m.notificationFeed().viewport.GetContent()), "Profile") || strings.Contains(shortHelp(m), "profile") {
		t.Fatalf("organization actor offers a profile: %s", shortHelp(m))
	}
	m = press(t, m, "u")
	if opened != 0 || len(m.profiles) != 0 {
		t.Fatal("organization actor opened a profile")
	}
}
