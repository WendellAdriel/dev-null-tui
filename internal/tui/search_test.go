package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type searchLog struct {
	profiles, commits []string
	commitErr         error
	live              *liveLog
}

func searchApp(t *testing.T) (AppModel, *searchLog) {
	t.Helper()
	m, live := liveApp(t)
	log := &searchLog{live: live}
	m.searchCalls = searchCalls{
		profiles: func(_ context.Context, term string) (api.Profiles, error) {
			log.profiles = append(log.profiles, term)
			return api.Profiles{
				Users:         []api.User{{ID: "u-ada", Handle: "ada", Name: "Ada Lovelace", Bio: "Engines"}},
				Organizations: []api.Organization{{Handle: "laravel", Name: "Laravel"}},
			}, nil
		},
		commits: func(_ context.Context, term string, limit int) ([]api.Commit, error) {
			log.commits = append(log.commits, fmt.Sprintf("%s limit=%d", term, limit))
			if log.commitErr != nil {
				return nil, log.commitErr
			}
			c := fixtureCommit("found")
			c.Body = "Caching tips for Laravel"
			return []api.Commit{c}, nil
		},
	}
	return m, log
}

// typeSearch types into the open dialog, then runs what the last keystroke scheduled, as a pause in typing would.
func typeSearch(m AppModel, value string) AppModel {
	var cmd tea.Cmd
	for _, r := range value {
		var next tea.Model
		next, cmd = m.update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(AppModel)
	}
	return settleSearch(m, cmd)
}

func searchKey(m AppModel, name string) AppModel {
	next, cmd := m.update(keyMsg(name))
	return settleSearch(next.(AppModel), cmd)
}

// settleSearch runs a command, delivers its messages, and follows only the search's own debounce and requests, so
// cursor blinks and toast timers don't keep it running.
func settleSearch(m AppModel, cmd tea.Cmd) AppModel {
	for _, msg := range runWithin(cmd, 500*time.Millisecond) {
		next, more := m.update(msg)
		m = next.(AppModel)
		switch msg.(type) {
		case searchTick, searchResult:
			m = settleSearch(m, more)
		default:
			m = deliver(m, runWithin(more, 100*time.Millisecond))
		}
	}
	return m
}

// runWithin runs a command and its batches, dropping whatever takes longer than wait.
func runWithin(cmd tea.Cmd, wait time.Duration) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var msgs []tea.Msg
			for _, command := range batch {
				msgs = append(msgs, runWithin(command, wait)...)
			}
			return msgs
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(wait):
		return nil
	}
}

func TestSearchIntentDecidesWhetherCommitsAreSearched(t *testing.T) {
	for term, want := range map[string]string{
		"@ada":           "developer",
		"taylorotwell":   "either",
		"ada42":          "developer",
		"ada-l":          "developer",
		"Daniel Kim":     "either",
		"caching tips":   "topic",
		"caching":        "either",
		"Laravel":        "developer",
		"verylonghandle": "either",
	} {
		if got := searchIntent(term); got != want {
			t.Errorf("searchIntent(%q) = %q, want %q", term, got, want)
		}
	}
}

func TestCtrlKOpensSearchAndFindsPeopleBeforeCommits(t *testing.T) {
	m, log := searchApp(t)
	m = searchKey(m, "ctrl+k")
	if m.search == nil {
		t.Fatal("Ctrl+K didn't open search")
	}
	if frame := frameText(m); !strings.Contains(frame, "Search developers or commits") || !strings.Contains(frame, "Start typing to search") {
		t.Fatalf("empty dialog:\n%s", frame)
	}
	m = typeSearch(m, "@ada")
	if fmt.Sprint(log.profiles) != "[ada]" || len(log.commits) != 0 {
		t.Fatalf("an @handle searches people only: %+v", log)
	}
	frame := frameText(m)
	for _, want := range []string{"Developers", "Ada Lovelace", "@ada", "Organizations", "Laravel"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("results lack %q:\n%s", want, frame)
		}
	}
	m = searchKey(m, "enter")
	if m.search != nil || len(m.profiles) != 1 || m.profiles[0].handle != "ada" {
		t.Fatalf("Enter didn't open the developer: search=%v profiles=%d", m.search != nil, len(m.profiles))
	}
}

func TestTopicSearchListsCommitsFirstAndSeeAllOpensResults(t *testing.T) {
	m, log := searchApp(t)
	m = searchKey(m, "/")
	m = typeSearch(m, "caching tips")
	if len(log.commits) == 0 || log.commits[len(log.commits)-1] != "caching tips limit=5" {
		t.Fatalf("commit searches: %q", log.commits)
	}
	frame := frameText(m)
	commits, developers := strings.Index(frame, "Most relevant commits"), strings.Index(frame, "Developers")
	if commits < 0 || developers < 0 || commits > developers || !strings.Contains(frame, "See all matching commits") {
		t.Fatalf("a topic lists commits first:\n%s", frame)
	}
	m = searchKey(m, "down")
	m = searchKey(m, "enter")
	if m.search != nil || !m.starredOpen || m.commitSearch != "caching tips" || log.commits[len(log.commits)-1] != "caching tips limit=20" {
		t.Fatalf("See all: open=%v term=%q calls=%q", m.starredOpen, m.commitSearch, log.commits)
	}
	if frame := frameText(m); !strings.Contains(frame, "Most relevant commits") || !strings.Contains(frame, "1 match") || !strings.Contains(frame, "Caching tips for Laravel") {
		t.Fatalf("results screen:\n%s", frame)
	}
	m = searchKey(m, "esc")
	if m.starredOpen || m.commitSearch != "" {
		t.Fatal("Esc didn't leave the results")
	}
}

func TestThrottledCommitSearchSaysSo(t *testing.T) {
	m, log := searchApp(t)
	log.commitErr = &auth.APIError{Status: http.StatusTooManyRequests}
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "caching tips")
	if frame := frameText(m); !strings.Contains(frame, "error: too many searches, give it a few seconds") {
		t.Fatalf("throttle message:\n%s", frame)
	}
	if len(m.tuiSettings.RecentSearches[m.account.user.ID]) != 0 {
		t.Fatal("a throttled search must not be remembered")
	}
}

func TestRecentSearchesKeepTheLastThreePerAccount(t *testing.T) {
	m, _ := searchApp(t)
	for _, term := range []string{"@ada", "@grace", "@ADA", "@linus", "@ken"} {
		m = searchKey(m, "ctrl+k")
		m = typeSearch(m, term)
		m = searchKey(m, "esc")
	}
	recent := m.tuiSettings.RecentSearches[m.account.user.ID]
	if fmt.Sprint(recent) != "[@ken @linus @ADA]" {
		t.Fatalf("recent: %q", recent)
	}
	if !savedSettings(m, func(s auth.TUISettings) bool { return len(s.RecentSearches[m.account.user.ID]) == 3 }) {
		t.Fatal("recent searches weren't saved to tui.json")
	}
	m = searchKey(m, "ctrl+k")
	if frame := frameText(m); !strings.Contains(frame, "Recent searches") || !strings.Contains(frame, "@linus") {
		t.Fatalf("empty dialog lacks recents:\n%s", frame)
	}
	m = searchKey(m, "down")
	m = searchKey(m, "enter")
	if m.search == nil || m.search.field.Value() != "@linus" {
		t.Fatal("picking a recent search should fill the box")
	}
	m = searchKey(m, "esc")
	m = searchKey(m, "ctrl+k")
	m = searchKey(m, "ctrl+l")
	if len(m.tuiSettings.RecentSearches[m.account.user.ID]) != 0 || strings.Contains(frameText(m), "Recent searches") {
		t.Fatal("Ctrl+L didn't clear recent searches")
	}
}

func TestSearchActionsComposeGoLiveAndOpenTheWebForIssues(t *testing.T) {
	m, live := searchApp(t)
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "comm")
	if frame := frameText(m); !strings.Contains(frame, "Actions") || !strings.Contains(frame, "Write a commit") {
		t.Fatalf("actions:\n%s", frame)
	}
	m = searchKey(m, "enter")
	if m.search != nil || !m.composer.open {
		t.Fatal("Write a commit didn't open the composer")
	}
	m = searchKey(m, "esc")
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "/bu")
	m = searchKey(m, "enter")
	if m.search != nil || fmt.Sprint(live.live.opened) != "[https://example.test/]" {
		t.Fatalf("/bug should open the web: %q", live.live.opened)
	}
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "go live")
	m = searchKey(m, "enter")
	if m.live == nil {
		t.Fatal("Go live didn't open its dialog")
	}
}

func TestSearchStaysOutOfTextInputs(t *testing.T) {
	m, _ := searchApp(t)
	m = searchKey(m, "c")
	m = searchKey(m, "ctrl+k")
	if m.search != nil {
		t.Fatal("Ctrl+K opened search over the composer")
	}
	m = searchKey(m, "/")
	if m.search != nil || !strings.Contains(m.composer.input.Value(), "/") {
		t.Fatal("/ should be typed in the composer")
	}
}

func TestTheAsideSearchRowOpensSearch(t *testing.T) {
	m, _ := searchApp(t)
	frame := frameText(m)
	if !strings.Contains(frame, "Search") || !strings.Contains(frame, "ctrl+k") {
		t.Fatalf("aside lacks the search row:\n%s", frame)
	}
	nav, center, aside := m.columns()
	_, areas := m.asideLayout(aside, m.mainHeight(), colors(m.account.dark))
	left := (m.account.width-(nav+center+aside+2))/2 + nav + center + 2
	for _, area := range areas {
		if area.action == "ctrl+k" {
			next, _ := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: left + area.x, Y: area.y})
			if next.search == nil {
				t.Fatal("clicking the search row didn't open search")
			}
			return
		}
	}
	t.Fatal("no clickable search row")
}

func TestHashtagsAreMarkedOutsideCodeAndWords(t *testing.T) {
	commit := api.Commit{Segments: []api.Segment{
		{Type: "text", Text: "Try #laravel and #café, not "},
		{Type: "code", Code: "#code"},
		{Type: "text", Text: " or a#b, x/#y, #123 or #fff"},
	}}
	body := strings.Join(bodyLines(commit, 0, colors(true), statsStyle{}), "\n")
	if got := fmt.Sprint(hashtagMarks.FindAllStringSubmatch(body, -1)); got != "[[\x1b_devnull-tag;%23laravel\x1b\\ %23laravel] [\x1b_devnull-tag;%23caf%C3%A9\x1b\\ %23caf%C3%A9]]" {
		t.Fatalf("hashtags: %q", got)
	}
	if plain := ansi.Strip(body); plain != "Try #laravel and #café, not `#code` or a#b, x/#y, #123 or ■ #fff" {
		t.Fatalf("text changed: %q", plain)
	}
}

func TestClickingAHashtagSearchesIt(t *testing.T) {
	m, _ := searchApp(t)
	m.feeds[m.active].commits[0].Body = "Ship #golang today"
	m.reflow()
	if strings.Contains(m.View().Content, "devnull-tag") {
		t.Fatal("hashtag marks must not reach the terminal")
	}
	for y, line := range strings.Split(frameText(m), "\n") {
		if x := strings.Index(line, "#golang"); x >= 0 {
			x = ansi.StringWidth(line[:x]) + 2
			next, cmd := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
			m = settleSearch(next, cmd)
			if m.search == nil || m.search.field.Value() != "#golang" {
				t.Fatal("clicking the hashtag didn't search it")
			}
			return
		}
	}
	t.Fatalf("no hashtag on screen:\n%s", frameText(m))
}

func TestTheWheelMovesSearchNotTheLogBehindIt(t *testing.T) {
	m, _ := searchApp(t)
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "@ada")
	selected := m.feeds[m.active].selected
	next, _ := m.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 70, Y: 20})
	m = next.(AppModel)
	if m.feeds[m.active].selected != selected || m.search == nil || m.search.index != 1 {
		t.Fatalf("wheel: log %d→%d, search index %v", selected, m.feeds[m.active].selected, m.search)
	}
}

func TestAReopenedSearchIgnoresTheClosedOnesTicks(t *testing.T) {
	m, log := searchApp(t)
	m = searchKey(m, "ctrl+k")
	next, stale := m.update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = searchKey(next.(AppModel), "esc")
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "y")
	settleSearch(m, stale)
	if fmt.Sprint(log.profiles) != "[y]" {
		t.Fatalf("profile searches: %q", log.profiles)
	}
}

func TestRecentSearchesNeverOutgrowWhatTUISettingsLoads(t *testing.T) {
	m, _ := searchApp(t)
	m.tuiSettings.RecentSearches = map[string][]string{}
	for i := range 60 {
		m.tuiSettings.RecentSearches[fmt.Sprint("account-", i)] = []string{strings.Repeat("<", 100), strings.Repeat("&", 100), strings.Repeat(">", 100)}
	}
	m = searchKey(m, "ctrl+k")
	m = typeSearch(m, "@ada")
	m = searchKey(m, "esc")
	if !savedSettings(m, func(s auth.TUISettings) bool { return fmt.Sprint(s.RecentSearches[m.account.user.ID]) == "[@ada]" }) {
		t.Fatal("tui.json didn't load back with this account's recent search")
	}
}
