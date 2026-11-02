package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func TestPastelIconReplacesTheGlyphOnlyInPasteisMode(t *testing.T) {
	m := fixtureApp(t)
	m.images.Enabled = true
	m.account.width, m.account.height = 160, 45
	c := fixtureCommit("pastel")
	c.Author.ID = "another-user"
	c.Pasteis = new(4)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	if slices.Contains(m.visibleMedia(), pastelIconRequest) {
		t.Fatal("pastel icon requested with pastéis mode off")
	}
	m.account.user.PasteisMode = true
	m.account.user.PasteisRemaining = new(7)
	if !slices.Contains(m.visibleMedia(), pastelIconRequest) {
		t.Fatal("pastel icon not requested in pastéis mode")
	}
	m.reflow()
	if !strings.Contains(ansi.Strip(m.View().Content), "◍ 4") {
		t.Fatal("the glyph must stand in until the icon is ready")
	}
	m.images.Clear()
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{pastelIconRequest})
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	m.reflow()
	view := m.View().Content
	if strings.Contains(ansi.Strip(view), "◍") {
		t.Fatal("the glyph stayed after the icon was ready")
	}
	for _, want := range []string{" 4 ", "7 pastéis left today"} {
		found := false
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, string(kitty.Placeholder)) && strings.Contains(ansi.Strip(line), want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no icon beside %q", want)
		}
	}
}
