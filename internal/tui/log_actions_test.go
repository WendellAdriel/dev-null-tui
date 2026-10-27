package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func logActionApp(t *testing.T) AppModel {
	t.Helper()
	m := threadApp(t)
	first, second := fixtureCommit("a"), fixtureCommit("b")
	first.Body, second.Body = "First commit", "Second commit"
	first.Author.ID = m.account.user.ID
	first.Images = []api.Image{{URL: "https://example.test/image.png", Width: 96, Height: 64, Description: "Synthetic image"}}
	return applyPage(m, 0, api.Page{Data: []api.Commit{first, second}})
}

func TestLogShowsActionsOnlyForTheSelectedCommit(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}} {
			m := logActionApp(t)
			m.account.dark = theme == "dark"
			next, _ := m.Update(size)
			m = next.(AppModel)
			content := ansi.Strip(m.feeds[0].viewport.GetContent())
			for _, label := range []string{"Thread", "Reply", "Quote", "View", "Delete"} {
				if strings.Count(content, label) != 1 {
					t.Fatalf("expected one %s action at %dx%d (%s):\n%s", label, size.Width, size.Height, theme, content)
				}
			}
			m, _ = m.handleKey("j")
			f := m.feeds[0]
			content = ansi.Strip(f.viewport.GetContent())
			if strings.Contains(content, "View") || strings.Contains(content, "Delete") || strings.Contains(content, "Replies") {
				t.Fatal("selected text commit exposed unavailable actions")
			}
			rows := strings.Split(content, "\n")
			for _, label := range []string{"Thread", "Reply", "Quote"} {
				if strings.Count(content, label) != 1 || !strings.Contains(strings.Join(rows[f.starts[1]:f.ends[1]], "\n"), label) {
					t.Fatalf("%s action did not follow selection", label)
				}
			}
		}
	}
}

func TestLogActionsAndKeysTargetTheSelectedCommit(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		for _, action := range []struct{ label, key string }{{"Thread", "enter"}, {"Reply", "r"}, {"Quote", "q"}, {"View", "v"}, {"Delete", "D"}} {
			m := logActionApp(t)
			id := "a"
			var cmd tea.Cmd
			if mouse {
				m.feeds[0].commits[1].Images = m.feeds[0].commits[0].Images
				m.feeds[0].commits[1].Author.ID = m.account.user.ID
				m, _ = m.selectCommit(1)
				id = "b"
				m, cmd = clickText(t, m, action.label)
			} else {
				m, cmd = m.handleKey(action.key)
			}
			switch action.key {
			case "enter":
				m = deliver(m, runAll(cmd))
				if len(m.threads) != 1 || m.threads[0].root.ID != id {
					t.Fatal("log thread action targeted the wrong commit")
				}
			case "r", "q":
				if !m.composer.open || m.composer.target.ID != id || (action.key == "r" && m.composer.mode != "reply") || (action.key == "q" && m.composer.mode != "quote") || len(m.threads) != 0 {
					t.Fatal("log composer action opened a thread or lost its target")
				}
			case "v":
				if !m.viewer.open || m.viewer.commit.ID != id || len(m.threads) != 0 {
					t.Fatal("log image action did not open the selected commit's image")
				}
			case "D":
				if m.deleting.id != id || len(m.threads) != 0 {
					t.Fatal("log delete action did not open confirmation")
				}
			}
		}
	}
}
