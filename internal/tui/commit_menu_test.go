package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func menuApp(t *testing.T) AppModel {
	t.Helper()
	return applyPage(fixtureApp(t), 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
}

func menuLabels(m AppModel) []string {
	var labels []string
	for _, row := range m.picker.rows {
		labels = append(labels, row.label)
	}
	return labels
}

func TestCommitShowsMainChipsAndKeepsTheRestInTheMoreMenu(t *testing.T) {
	m := fixtureApp(t)
	rows, _ := m.commitActions(fixtureCommit("a"), commitAction{"Thread", "enter"}, 200, colors(true))
	chips := ansi.Strip(strings.Join(rows, "\n"))
	for _, want := range []string{"Thread", "Reply", "Quote", "Translate", "⋯ More"} {
		if !strings.Contains(chips, want) {
			t.Fatalf("visible chips lack %q: %s", want, chips)
		}
	}
	for _, hidden := range []string{"Copy link", "Report", "Open link"} {
		if strings.Contains(chips, hidden) {
			t.Fatalf("%q belongs in the More menu: %s", hidden, chips)
		}
	}
}

func TestCommitMenuChecksTheAuthorAndRunsItsActions(t *testing.T) {
	m := menuApp(t)
	profiles := 0
	following := false
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		profiles++
		return api.Profile{User: api.User{Handle: handle}, IsFollowing: &following, IsMuting: &following}, nil
	}
	var followed string
	m.setFollow = func(_ context.Context, handle string, follow bool) error {
		if follow {
			followed = handle
		}
		return nil
	}
	next, cmd := m.Update(keyMsg("."))
	m = next.(AppModel)
	if m.picker == nil || m.picker.kind != "commit-menu" {
		t.Fatal(". must open the commit menu")
	}
	if got := strings.Join(menuLabels(m), "|"); !strings.Contains(got, "Checking @ada") || strings.Contains(got, "Follow") {
		t.Fatalf("author actions must wait for the author's profile: %s", got)
	}
	m = settleCmd(m, cmd)
	got := strings.Join(menuLabels(m), "|")
	for _, want := range []string{"Copy link", "Follow @ada", "Mute @ada", "Block @ada", "Report commit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("menu lacks %q: %s", want, got)
		}
	}
	if profiles != 1 {
		t.Fatalf("the author's profile must be read once, got %d", profiles)
	}
	for i, row := range m.picker.rows {
		if row.value == "follow" {
			m.picker.selected = i
		}
	}
	next, cmd = m.Update(keyMsg("enter"))
	m = settleCmd(next.(AppModel), cmd)
	if m.picker != nil || followed != "ada" {
		t.Fatalf("Follow must close the menu and follow the author (followed %q)", followed)
	}
	m = pressApp(m, ".")
	m = pressApp(m, "!")
	if m.picker != nil || m.settings.dialog != "report" {
		t.Fatal("a row's key must run it from the menu")
	}
}

func TestSendInRemotesPicksSomeoneConfirmsAndSendsTheLink(t *testing.T) {
	m := menuApp(t)
	m.account.user.Remotes = &auth.RemotePreferences{}
	m.remoteCalls.candidates = func(_ context.Context, q api.CandidateQuery) ([]api.Candidate, error) {
		if q.For != api.ForShare {
			t.Errorf("share search must use for=share, got %q", q.For)
		}
		return []api.Candidate{{User: api.User{ID: "fixture", Handle: "fixture"}}, {User: api.User{ID: "u2", Handle: "grace", Name: "Grace"}}}, nil
	}
	var sent api.NewMessage
	var to api.Target
	m.remoteCalls.push = func(_ context.Context, target api.Target, draft api.NewMessage) (api.Message, error) {
		to, sent = target, draft
		return api.Message{}, nil
	}
	m = pressApp(m, ".")
	for i, row := range m.picker.rows {
		if row.value == "share" {
			m.picker.selected = i
		}
	}
	m = pressApp(m, "enter")
	if !m.sharingCommit() {
		t.Fatal("Send in Remotes must open the people picker")
	}
	m = pressApp(m, "g")
	next, cmd := m.update(peopleSearchTick{session: m.session, generation: m.remotes.people.generation})
	m = settleCmd(next.(AppModel), cmd)
	if len(m.remotes.people.results) != 1 || m.remotes.people.results[0].User.Handle != "grace" {
		t.Fatalf("the picker must leave out the viewer: %+v", m.remotes.people.results)
	}
	if !strings.Contains(frameText(m), "Send commit in Remotes") {
		t.Fatal("the picker must show over the log")
	}
	m = pressApp(m, "enter")
	if m.ask == nil || m.ask.kind != "share-commit" || !strings.Contains(m.ask.question, "@grace") {
		t.Fatal("picking someone must ask before sending")
	}
	next, cmd = m.Update(keyMsg("ctrl+y"))
	m = settleCmd(next.(AppModel), cmd)
	if to.Handle != "grace" || sent.Body != fixtureCommit("a").URL || sent.ClientID == "" {
		t.Fatalf("the commit link must go to @grace, sent %+v to %+v", sent, to)
	}
}
