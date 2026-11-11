package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestLinkCardShowsSiteTitleAndDescription(t *testing.T) {
	p := colors(true)
	card := strings.Join(linkCardLines(&api.LinkPreview{URL: "https://laravel.com/docs", Title: "Laravel docs", Description: "The PHP framework", SiteName: "Laravel"}, 60, p, nil), "\n")
	plain := ansi.Strip(card)
	for _, want := range []string{"Laravel", "Laravel docs", "The PHP framework", "╭", "╰"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("card misses %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(card, ansi.SetHyperlink("https://laravel.com/docs")) {
		t.Fatal("the card title does not link to the page")
	}
	host := ansi.Strip(strings.Join(linkCardLines(&api.LinkPreview{URL: "https://go.dev/blog", Title: "Go"}, 60, p, nil), "\n"))
	if !strings.Contains(host, "go.dev") {
		t.Fatalf("a card without a site name does not fall back to the host:\n%s", host)
	}
	if host := linkHost("https://evil\u202e.com/x"); strings.ContainsRune(host, '\u202e') {
		t.Fatalf("the host keeps a format character: %q", host)
	}
}

func TestLinkCardWaitsWhilePendingAndHidesWithoutATitle(t *testing.T) {
	p := colors(true)
	if pending := ansi.Strip(strings.Join(linkCardLines(&api.LinkPreview{URL: "https://go.dev", Pending: true}, 60, p, nil), "\n")); !strings.Contains(pending, "Loading preview") {
		t.Fatalf("pending card: %q", pending)
	}
	if lines := linkCardLines(&api.LinkPreview{URL: "https://go.dev"}, 60, p, nil); lines != nil {
		t.Fatalf("an untitled card is drawn: %q", lines)
	}
	if lines := linkCardLines(nil, 60, p, nil); lines != nil {
		t.Fatal("a missing card is drawn")
	}
}

func TestCommitsShowTheirLinkCardInTheLog(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	c := fixtureCommit("card")
	c.LinkPreview = &api.LinkPreview{URL: "https://laravel.com", Title: "Laravel", SiteName: "laravel.com"}
	m = applyPage(next.(AppModel), 0, api.Page{Data: []api.Commit{c}})
	if feed := ansi.Strip(m.feeds[0].viewport.GetContent()); !strings.Contains(feed, "Laravel") || !strings.Contains(feed, "laravel.com") {
		t.Fatalf("the log does not show the link card:\n%s", feed)
	}
	quoting, quoted := fixtureCommit("quoting"), fixtureCommit("quoted")
	quoted.LinkPreview = &api.LinkPreview{URL: "https://go.dev", Title: "The Go Programming Language", SiteName: "go.dev"}
	quoting.QuotedID, quoting.Quote = &quoted.ID, &quoted
	m = applyPage(m, 0, api.Page{Data: []api.Commit{quoting}})
	if feed := ansi.Strip(m.feeds[0].viewport.GetContent()); !strings.Contains(feed, "The Go Programming Language") {
		t.Fatalf("the quote does not show its link card:\n%s", feed)
	}
}

func TestClickingAnywhereOnALinkCardOpensItsLink(t *testing.T) {
	m, opened := linksApp(t, "Docs https://laravel.com/docs")
	c := fixtureCommit("card")
	c.Body = "Docs https://laravel.com/docs"
	c.LinkPreview = &api.LinkPreview{URL: "https://laravel.com/docs", Title: "Laravel docs", Description: "The PHP framework", SiteName: "Laravel"}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	for _, part := range []string{"The PHP framework", "Laravel docs", "Laravel  "} {
		*opened = nil
		next, cmd := clickText(t, m, part)
		settleCmd(next, cmd)
		if strings.Join(*opened, " ") != "https://laravel.com/docs" {
			t.Fatalf("clicking the card's %q opened %v", part, *opened)
		}
	}
}

func TestClickingARemotesLinkCardOpensItsLink(t *testing.T) {
	m, _ := remotesApp(t, auth.RemotePreferences{})
	var opened []string
	m.openURL = func(_ context.Context, raw string) error {
		opened = append(opened, raw)
		return nil
	}
	m, _ = m.handleKey("g")
	m = settle(t, m, "r", "enter")
	message := remoteMessage(5, "ada", "https://go.dev", m.now)
	message.LinkPreview = &api.LinkPreview{URL: "https://go.dev", Title: "The Go Programming Language", Description: "Build simple, secure, scalable systems", SiteName: "go.dev"}
	m.remotes.thread.messages = []api.Message{message}
	m.renderRemoteThread()
	next, cmd := clickText(t, m, "Build simple")
	settleCmd(next, cmd)
	if strings.Join(opened, " ") != "https://go.dev" {
		t.Fatalf("clicking the Remotes card opened %v", opened)
	}
}
