package tui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func engagementFixture(t *testing.T) AppModel {
	m := fixtureApp(t)
	c := fixtureCommit("engagement")
	c.Author.ID = "another-user"
	c.Starred = new(false)
	c.Pasteis = new(2)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	m.account.user.PasteisMode = true
	m.account.user.PasteisRemaining = new(2)
	m.setStar = func(context.Context, string, bool) (api.Commit, error) {
		r := c
		r.Starred = new(true)
		r.Stars = 1
		return r, nil
	}
	m.givePastel = func(context.Context, string) (api.Pastel, error) {
		r := c
		r.Pasteis = new(3)
		return api.Pastel{Commit: r, Remaining: new(1)}, nil
	}
	return m
}

func TestActiveLogAndThreadEngagementKeysAndQuota(t *testing.T) {
	m := engagementFixture(t)
	rows, _ := m.commitActions(m.feeds[0].commits[0], commitAction{}, 80, colors(true))
	text := ansi.Strip(strings.Join(rows, "\n"))
	text = strings.Join(strings.Fields(text), " ")
	if !strings.Contains(text, "Star s") || !strings.Contains(text, "Pastél p") {
		t.Fatalf("missing actions: %s", text)
	}
	m, cmd := m.handleKey("s")
	if cmd == nil {
		t.Fatal("star key inactive")
	}
	if _, duplicate := m.handleKey("s"); duplicate != nil {
		t.Fatal("star sent twice")
	}
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if c := m.feeds[0].commits[0]; c.Starred == nil || !*c.Starred || c.Stars != 1 {
		t.Fatal("star confirmation not reconciled")
	}
	if m.toast.kind != "success" || m.toast.title != "Star" || m.toast.detail != "You starred the commit" {
		t.Fatalf("star toast: %+v", m.toast)
	}
	m, _ = m.handleKey("enter")
	m, cmd, _ = m.threadKey("p")
	if cmd == nil {
		t.Fatal("thread pastel shortcut inactive")
	}
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if *m.account.user.PasteisRemaining != 1 || *m.threads[0].root.Pasteis != 3 || *m.feeds[0].commits[0].Pasteis != 3 {
		t.Fatal("pastel counts not reconciled across log and thread")
	}
	if m.toast.title != "Pastél awarded" || m.toast.detail != "You awarded a pastél" {
		t.Fatalf("pastel toast: %+v", m.toast)
	}
	m.account.user.PasteisRemaining = new(0)
	rows, _ = m.commitActions(m.threads[0].root, commitAction{}, 80, colors(true))
	if strings.Contains(ansi.Strip(strings.Join(rows, "\n")), "Pastél") {
		t.Fatal("exhausted quota exposes award action")
	}
}

func TestEngagementFailureLeavesCountsAndBlocksUncertainPastelRetry(t *testing.T) {
	m := engagementFixture(t)
	m.setStar = func(context.Context, string, bool) (api.Commit, error) { return api.Commit{}, errors.New("offline") }
	m, cmd := m.handleKey("s")
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if m.feeds[0].commits[0].Stars != 0 || *m.feeds[0].commits[0].Starred || m.toast.kind != "error" {
		t.Fatal("failed star changed counts or was not an error")
	}
	m.givePastel = func(context.Context, string) (api.Pastel, error) { return api.Pastel{}, errors.New("unknown delivery") }
	m, cmd = m.handleKey("p")
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if m.account.user.PasteisRemaining != nil || *m.feeds[0].commits[0].Pasteis != 2 || m.toast.kind != "warning" {
		t.Fatal("uncertain award guessed quota or count, or was not a warning")
	}
	_, cmd = m.handleKey("p")
	if cmd != nil {
		t.Fatal("uncertain pastel replayed")
	}
	c := m.feeds[0].commits[0]
	c.Author.ID = m.account.user.ID
	if m.canStar(c) || m.canGivePastel(c) {
		t.Fatal("own commit exposes engagement")
	}
}

func TestUnstarToastNamesTheAction(t *testing.T) {
	m := engagementFixture(t)
	c := m.feeds[0].commits[0]
	c.Starred = new(true)
	m.feeds[0].commits[0] = c
	m.setStar = func(context.Context, string, bool) (api.Commit, error) {
		r := c
		r.Starred = new(false)
		return r, nil
	}
	m, cmd := m.handleKey("s")
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if m.toast.title != "Unstar" || m.toast.detail != "You unstarred the commit" {
		t.Fatalf("unstar toast: %+v", m.toast)
	}
}

func TestExhaustedPasteisWarn(t *testing.T) {
	m := engagementFixture(t)
	m.givePastel = func(context.Context, string) (api.Pastel, error) {
		return api.Pastel{}, &auth.APIError{Status: http.StatusUnprocessableEntity, Code: "pasteis_exhausted"}
	}
	m, cmd := m.handleKey("p")
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if m.toast.kind != "warning" || *m.account.user.PasteisRemaining != 0 {
		t.Fatalf("exhausted pastéis toast: %+v", m.toast)
	}
}
