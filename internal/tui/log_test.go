package tui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestReferenceNodesAndDayGroupsSurvivePagination(t *testing.T) {
	m := fixtureApp(t)
	first, second := fixtureCommit("a"), fixtureCommit("b")
	second.CreatedAt = second.CreatedAt.Add(-24 * time.Hour)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{first}, Next: "next"})
	m.feeds[0].loading = true
	updated, _ := m.Update(logResult{index: 0, generation: m.feeds[0].generation, mode: "more", page: api.Page{Data: []api.Commit{second}}})
	m = updated.(AppModel)
	f := m.feeds[0]
	lines := strings.Split(ansi.Strip(f.viewport.GetContent()), "\n")
	for _, start := range f.starts {
		if !strings.Contains(lines[start], "●  Ada Fixture") {
			t.Fatalf("author node missing: %q", lines[start])
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Yesterday") {
		t.Fatal("pagination lost the next day group")
	}
	if strings.Contains(strings.Join(lines, "\n"), "│") {
		t.Fatal("unrelated commits must not be joined by reply rails")
	}
	if len(f.starts) != 2 || authorColor(first.Author, colors(true)) != colors(true).lanes[4] {
		t.Fatal("wrong author lane or missing commit offsets")
	}
	if !strings.Contains(ansi.Strip(f.viewport.GetContent()), "EOF") {
		t.Fatal("missing root terminator")
	}
	if authorColor(&api.Author{Lane: []byte(`"#123abc"`)}, colors(true)) != "#123abc" {
		t.Fatal("custom lane lost")
	}
}

func TestReadOnlyPreviewsDoNotLeakDeletedContentOrHTML(t *testing.T) {
	c := fixtureCommit("a")
	c.Segments = []api.Segment{{Type: "text", Text: "Use "}, {Type: "code", Code: "git status"}, {Type: "block", Language: "go", HTML: `<span class="hl-keyword">fmt</span>.Println(&quot;hello&quot;)`}}
	c.QuotedID = new("quote")
	c.Quote = &api.Commit{ID: "hidden", Body: "private body", Author: &api.Author{Name: "private author"}, DeletedAt: new(time.Now())}
	output := ansi.Strip(strings.Join(commitLines(c, 70, colors(true), time.Now()), "\n"))
	for _, unwanted := range []string{"<span", "hl-keyword", "private body", "private author"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("unsafe preview: %s", output)
		}
	}
	if !strings.Contains(output, `fmt.Println("hello")`) || !strings.Contains(output, "`git status`") || !strings.Contains(output, "Quoted commit unavailable") {
		t.Fatalf("preview lost content: %s", output)
	}
	c.DeletedAt = new(time.Now())
	if output := strings.Join(commitLines(c, 70, colors(true), time.Now()), "\n"); strings.Contains(output, "Ada") || strings.Contains(output, "git status") {
		t.Fatal("tombstone leaked author or body")
	}
}

func TestRefreshReturnsToFirstPageAndCountsDoNotInsertRows(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b")}})
	m.feeds[0].selected = 1
	requests := 0
	m.fetch = func(_ context.Context, log api.Log, next, since string) (api.Page, error) {
		requests++
		if next == "" {
			return api.Page{Data: []api.Commit{fixtureCommit("new"), fixtureCommit("a")}, Next: "older", NewCount: 3}, nil
		}
		return api.Page{Data: []api.Commit{fixtureCommit("b"), fixtureCommit("c")}}, nil
	}
	m, cmd := m.handleKey("ctrl+r")
	if cmd == nil {
		t.Fatal("Ctrl+R must refresh")
	}
	if !m.feeds[0].loading {
		t.Fatal("refresh state not retained in returned model")
	}
	if m.feeds[0].commits[m.feeds[0].selected].ID != "b" {
		t.Fatal("refresh cleared content before completion")
	}
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if requests != 1 || m.feeds[0].selected != 0 || m.feeds[0].commits[0].ID != "new" || m.feeds[0].viewport.YOffset() != 0 || m.feeds[0].next != "older" {
		t.Fatal("refresh must load only the first page and select its first commit")
	}
	cmd = m.load("count")
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if len(m.feeds[0].commits) != 2 || m.feeds[0].newCount != 3 {
		t.Fatal("count poll inserted content or lost indicator")
	}
}

func TestRevocationClearsPrivateFeedAndClipboardFailureHasFallback(t *testing.T) {
	m := fixtureApp(t)
	c := fixtureCommit("a")
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	m.copyLink = func(context.Context, string) error { return errors.New("clipboard denied") }
	m, cmd := m.handleKey("y")
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if m.manualLink != c.URL || !strings.Contains(ansi.Strip(m.View().Content), "Clipboard unavailable") {
		t.Fatal("clipboard failure lacks copyable fallback")
	}
	updated, _ = m.Update(logResult{index: 0, generation: m.feeds[0].generation, err: &auth.APIError{Status: 401, Code: "device_not_approved"}})
	m = updated.(AppModel)
	if m.connected || len(m.feeds[0].commits) > 0 || m.manualLink != "" || m.account.state != "failed" {
		t.Fatal("revoked session retained application data")
	}
}

func TestSavedConnectionOpensApplicationAndReadsSignedLogs(t *testing.T) {
	requests := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned application request")
		}
		switch r.URL.Path {
		case "/api/v1/me":
			_, _ = io.WriteString(w, `{"data":{"id":"fixture","username":"fixture","name":"Fixture"}}`)
		default:
			log := strings.TrimPrefix(r.URL.Path, "/api/v1/log/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"log": log, "is_ranked": false, "data": []any{}, "links": map[string]any{"next": nil}})
		}
		requests <- r.URL.Path
	}))
	defer server.Close()
	store := auth.Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	key, _ := auth.GenerateKey()
	if err := store.Save(key, server.URL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	m := NewApp("dark", store, server.URL, ctx)
	defer func() { _ = m.Close() }()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithWindowSize(80, 24))
	finished := make(chan tea.Model, 1)
	go func() {
		final, err := p.Run()
		if err != nil {
			t.Error(err)
		}
		finished <- final
	}()
	for _, want := range []string{"/api/v1/me", "/api/v1/log/following"} {
		select {
		case got := <-requests:
			if got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		case <-ctx.Done():
			t.Fatal("startup did not reach signed application")
		}
	}
	p.Send(tea.KeyPressMsg{Code: tea.KeyTab})
	select {
	case got := <-requests:
		if got != "/api/v1/log/latest" {
			t.Fatal(got)
		}
	case <-ctx.Done():
		t.Fatal("tab switch did not load Latest")
	}
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	final := <-finished
	if app := final.(AppModel); !app.connected || app.active != 1 {
		t.Fatal("single program did not keep application state")
	}
}

func TestNoColorAndReducedColorRetainVisibleFocus(t *testing.T) {
	for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
		m := fixtureApp(t)
		m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
		updated, _ := m.Update(tea.ColorProfileMsg{Profile: profile})
		v := updated.View()
		if !strings.Contains(ansi.Strip(v.Content), "▌ ●  Ada") {
			t.Fatal("focus relies only on color")
		}
		if profile == colorprofile.ASCII && (v.BackgroundColor != nil || v.ForegroundColor != nil) {
			t.Fatal("no-color canvas changed terminal colors")
		}
	}
}

func TestSuccessfulRefreshGoesToTopEvenAfterPendingNavigation(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("c"), fixtureCommit("d")}})
	m.fetch = func(_ context.Context, _ api.Log, next, _ string) (api.Page, error) {
		if next == "" {
			return api.Page{Data: []api.Commit{fixtureCommit("new"), fixtureCommit("a")}, Next: "older"}, nil
		}
		return api.Page{Data: []api.Commit{fixtureCommit("b"), fixtureCommit("c"), fixtureCommit("d")}}, nil
	}
	m, cmd := m.handleKey("ctrl+r")
	if cmd == nil {
		t.Fatal("Ctrl+R must refresh")
	}
	response := cmd()
	m, _ = m.handleKey("j")
	m, _ = m.handleKey("j")
	updated, _ := m.Update(response)
	m = updated.(AppModel)
	f := m.feeds[0]
	if f.commits[f.selected].ID != "new" || f.viewport.YOffset() != 0 {
		t.Fatal("successful explicit refresh must return to the first commit")
	}
}

func TestRefreshFailureKeepsPositionAndLetterRDoesNotRefresh(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a"), fixtureCommit("b"), fixtureCommit("c")}})
	m, _ = m.handleKey("j")
	for _, pressed := range []string{"r", "R"} {
		var cmd tea.Cmd
		m, cmd = m.handleKey(pressed)
		if cmd != nil {
			t.Fatal("letter r must not refresh in either case")
		}
	}
	position := m.feeds[0].viewport.YOffset()
	m.fetch = func(context.Context, api.Log, string, string) (api.Page, error) {
		return api.Page{}, errors.New("offline")
	}
	m, cmd := m.handleKey("ctrl+r")
	if cmd == nil {
		t.Fatal("Ctrl+R must refresh")
	}
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if m.feeds[0].commits[m.feeds[0].selected].ID != "b" || m.feeds[0].viewport.YOffset() != position {
		t.Fatal("failed refresh changed reading position")
	}
}

func TestHiddenAccountVerificationNoticeExpires(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	updated, _ := m.Update(verificationNoticeExpired{generation: m.account.generation})
	m = updated.(AppModel)
	m, _ = m.handleKey("a")
	if strings.Contains(ansi.Strip(m.View().Content), verificationNotice) {
		t.Fatal("hidden account notice did not expire")
	}
	if strings.Contains(ansi.Strip(m.View().Content), "Log screens are not available") {
		t.Fatal("connected account guidance contradicts workspace")
	}
	m.account.state, m.account.approvalRequired = "failed", true
	if strings.Contains(ansi.Strip(m.account.View().Content), "Verify the saved connection before") {
		t.Fatal("revoked account still recommends verification before approval")
	}
}

func TestResizeKeepsSelectedCommitOnScreen(t *testing.T) {
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	m = updated.(AppModel)
	var commits []api.Commit
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		commits = append(commits, fixtureCommit(id))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	for range 4 {
		m, _ = m.handleKey("j")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m = updated.(AppModel)
	f := m.feeds[0]
	if f.commits[f.selected].ID != "e" || f.starts[f.selected] >= f.viewport.YOffset()+f.viewport.Height() || f.ends[f.selected] <= f.viewport.YOffset() {
		t.Fatal("resize moved the selected commit off screen")
	}
}
