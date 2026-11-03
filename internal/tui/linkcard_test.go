package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
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
