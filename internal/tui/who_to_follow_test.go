package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type suggestionLog struct {
	queries   []api.SuggestionQuery
	dismissed []string
	followed  []string
}

func suggestionApp(t *testing.T, width int) (AppModel, *suggestionLog) {
	t.Helper()
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = next.(AppModel)
	log := &suggestionLog{}
	two := 2
	people := []api.SuggestedUser{
		{User: api.User{ID: "u1", Handle: "grace", Name: "Grace Hopper", Bio: "Compilers\nand COBOL"}, FollowedByFollowing: 2},
		{User: api.User{ID: "u2", Handle: "linus", Name: "Linus", Followers: &two}},
		{User: api.User{ID: "u3", Handle: "ada", Name: "Ada"}},
		{User: api.User{ID: "u4", Handle: "ken", Name: "Ken Spare"}},
	}
	m.suggestCalls = suggestCalls{
		list: func(_ context.Context, q api.SuggestionQuery) ([]api.SuggestedUser, error) {
			log.queries = append(log.queries, q)
			return people, nil
		},
		dismiss: func(_ context.Context, handle string) error {
			log.dismissed = append(log.dismissed, handle)
			return nil
		},
	}
	m.setFollow = func(_ context.Context, handle string, follow bool) error {
		log.followed = append(log.followed, handle)
		return nil
	}
	var commits []api.Commit
	for i := range 8 {
		commits = append(commits, fixtureCommit(fmt.Sprint("c", i)))
	}
	m = applyPage(m, 0, api.Page{Data: commits})
	m = settleCmd(m, m.fetchSuggestions(false))
	return m, log
}

func TestWhoToFollowSitsInTheLogAfterTheFifthCommitAndTakesKeys(t *testing.T) {
	m, log := suggestionApp(t, 110)
	content := m.feeds[0].viewport.GetContent()
	block := strings.Index(content, "who --should-i-follow")
	if block < 0 || !strings.Contains(content, "Followed by 2 you follow") || !strings.Contains(content, "2 followers") || strings.Contains(content, "Ken Spare") {
		t.Fatalf("the log must show three suggestions with their reasons:\n%s", content)
	}
	if strings.Count(content[:block], "c0") == 0 || !strings.Contains(content[block:], "Ada") {
		t.Fatal("the block must sit between the commits")
	}
	for range whoToFollowAfter {
		m = pressApp(m, "j")
	}
	if m.feeds[0].person != 1 || m.feeds[0].selected != whoToFollowAfter-1 {
		t.Fatalf("j after the fifth commit must reach the first suggestion: person %d selected %d", m.feeds[0].person, m.feeds[0].selected)
	}
	next, cmd := m.Update(keyMsg("f"))
	m = settleCmd(next.(AppModel), cmd)
	if strings.Join(log.followed, ",") != "grace" || !suggestedFollowing(m.shownPeople()[0]) {
		t.Fatalf("f must follow the suggested person, followed %v", log.followed)
	}
	m = pressApp(m, "j")
	next, cmd = m.Update(keyMsg("X"))
	m = settleCmd(next.(AppModel), cmd)
	if strings.Join(log.dismissed, ",") != "linus" || m.shownPeople()[2].Handle != "ken" {
		t.Fatalf("X must dismiss and a spare must take the place: %v %+v", log.dismissed, m.shownPeople())
	}
	if m.composer.open {
		t.Fatal("commit keys must not act on the commit above a suggestion")
	}
	m = pressApp(m, "j")
	m = pressApp(m, "j")
	if m.feeds[0].person != 0 || m.feeds[0].selected != whoToFollowAfter {
		t.Fatal("j past the last suggestion must reach the sixth commit")
	}
	m = pressApp(m, "k")
	if m.feeds[0].person != 3 {
		t.Fatal("k from the sixth commit must return to the last suggestion")
	}
	next, cmd = m.Update(keyMsg("R"))
	_ = settleCmd(next.(AppModel), cmd)
	if last := log.queries[len(log.queries)-1]; last.Round != 1 {
		t.Fatalf("R must ask for the next round, asked %+v", last)
	}
}

func TestWhoToFollowIsAnAsideActionThatOpensAModalLikeTheWeb(t *testing.T) {
	m, log := suggestionApp(t, 140)
	if strings.Contains(m.feeds[0].viewport.GetContent(), "who --should-i-follow") || strings.Contains(frameText(m), "Grace Hopper") {
		t.Fatal("with an aside, suggestions wait for the Who to follow action")
	}
	if !strings.Contains(frameText(m), "Who to follow") {
		t.Fatal("the aside's Actions must offer Who to follow")
	}
	m = pressApp(m, "w")
	text := frameText(m)
	for _, want := range []string{"WHO TO FOLLOW", "● @grace", "# Compilers and COBOL", "Followed by 2 you follow", "Not interested", "Suggest others", "Close  esc"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the modal lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Hide") {
		t.Fatal("the modal must not offer Hide, which belongs in Settings")
	}
	m = pressApp(m, "j")
	next, cmd := m.Update(keyMsg("f"))
	m = settleCmd(next.(AppModel), cmd)
	if strings.Join(log.followed, ",") != "linus" {
		t.Fatalf("f must follow the selected person, followed %v", log.followed)
	}
	next, cmd = m.Update(keyMsg("X"))
	m = settleCmd(next.(AppModel), cmd)
	if strings.Join(log.dismissed, ",") != "linus" || m.whoToFollow.focus != 2 {
		t.Fatalf("X must dismiss the selected person and keep the row: %v focus %d", log.dismissed, m.whoToFollow.focus)
	}
	if m.composer.open || m.threadOnTop() || m.feeds[0].selected != 0 {
		t.Fatal("the modal must own the keyboard")
	}
	m = pressApp(m, "esc")
	if m.whoToFollow.open {
		t.Fatal("esc must close the modal")
	}
	m = pressApp(m, "j")
	if m.feeds[0].selected != 1 {
		t.Fatal("j must move the log again after the modal closes")
	}
}

func TestWhoToFollowModalAsksForTheScreenUnderneath(t *testing.T) {
	m, log := suggestionApp(t, 140)
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		return api.Thread{Commit: fixtureCommit(id)}, nil
	}
	next, cmd := m.Update(keyMsg("enter"))
	m = settleCmd(next.(AppModel), cmd)
	asked := len(log.queries)
	if m.syncSuggestions() != nil || len(log.queries) != asked {
		t.Fatal("a closed modal beside an aside must not ask")
	}
	next, cmd = m.Update(keyMsg("w"))
	m = settleCmd(next.(AppModel), cmd)
	if last := log.queries[len(log.queries)-1]; last.CommitID != "c0" || last.Handle != "" || !m.whoToFollow.open {
		t.Fatalf("a thread's modal must pass its commit, asked %+v", last)
	}
}

func TestWhoToFollowModalGetsCompactToFitTheTerminal(t *testing.T) {
	m, _ := suggestionApp(t, 140)
	m = pressApp(m, "w")
	if !strings.Contains(frameText(m), "# Compilers") {
		t.Fatal("a roomy modal must show bios")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m = next.(AppModel)
	text := frameText(m)
	if strings.Contains(text, "# Compilers") || !strings.Contains(text, "Grace Hopper") || !strings.Contains(text, "Close  esc") {
		t.Fatalf("a small terminal must get a compact modal that still fits:\n%s", text)
	}
}

func TestHidingWhoToFollowIsSavedAndRemovesTheBlock(t *testing.T) {
	m, _ := suggestionApp(t, 110)
	m = settleCmd(m, m.activateSetting("who-to-follow", ""))
	if !m.account.store.TUISettings().HideWhoToFollow {
		t.Fatal("hiding must be saved in tui.json")
	}
	if strings.Contains(m.feeds[0].viewport.GetContent(), "who --should-i-follow") {
		t.Fatal("a hidden block must leave the log")
	}
}

func TestWhoToFollowClickTargetsSurviveALaterDayHeading(t *testing.T) {
	m, _ := suggestionApp(t, 110)
	commits := m.feeds[0].commits
	commits[5].CreatedAt = commits[5].CreatedAt.AddDate(0, 0, -1)
	commits[6].CreatedAt = commits[5].CreatedAt
	commits[7].CreatedAt = commits[7].CreatedAt.AddDate(0, 0, -3)
	m = applyPage(m, 0, api.Page{Data: commits})
	rows := strings.Split(m.feeds[0].viewport.GetContent(), "\n")
	for _, area := range m.feeds[0].actions {
		if area.action == "wtf-round" && !strings.Contains(rows[area.y], "who --should-i-follow") {
			t.Fatalf("the round control points at %q", rows[area.y])
		}
	}
	m, _ = m.suggestionAction("wtf-hide", 0)
	if !m.tuiSettings.HideWhoToFollow {
		t.Fatal("Hide must hide the block")
	}
}

func TestTheAsideFitsItsHeightByDroppingTheVersionThenShortcuts(t *testing.T) {
	m, _ := suggestionApp(t, 140)
	p := m.account.colors()
	view := func(height int) string { return ansi.Strip(m.aside(36, height, p)) }
	if tall := view(60); !strings.Contains(tall, "/dev/null TUI") || !strings.Contains(tall, "Shortcuts") {
		t.Fatalf("a tall aside must show everything:\n%s", tall)
	}
	for _, height := range []int{30, 24, 18} {
		aside := view(height)
		if !strings.Contains(aside, "Account") || !strings.Contains(aside, "Refresh") || !strings.Contains(aside, "Who to follow") {
			t.Fatalf("at %d rows the aside must keep its Actions:\n%s", height, aside)
		}
		if lines := strings.Count(aside, "\n") + 1; lines != height {
			t.Fatalf("the aside must be exactly %d rows, got %d", height, lines)
		}
	}
	if short := view(18); strings.Contains(short, "/dev/null TUI") {
		t.Fatalf("a short aside must drop the version:\n%s", short)
	}
}

func TestAFailedSuggestionRequestLeavesTheModalWithAMessage(t *testing.T) {
	m, _ := suggestionApp(t, 140)
	m.whoToFollow = whoToFollow{}
	m.suggestCalls.list = func(context.Context, api.SuggestionQuery) ([]api.SuggestedUser, error) {
		return nil, errors.New("not found")
	}
	next, cmd := m.Update(keyMsg("w"))
	m = settleCmd(next.(AppModel), cmd)
	if text := frameText(m); strings.Contains(text, "Loading") || !strings.Contains(text, "Couldn't load suggestions") {
		t.Fatalf("a failed request must not leave Loading:\n%s", text)
	}
}

func TestSearchStaysClosedWhileTheWhoToFollowModalIsOpen(t *testing.T) {
	m, _ := suggestionApp(t, 140)
	m = pressApp(m, "w")
	next, _ := m.Update(keyMsg("ctrl+k"))
	if m = next.(AppModel); m.search != nil || !m.whoToFollow.open {
		t.Fatal("Ctrl+K must not open search over the modal")
	}
}

func TestAModalPersonsClickAreaEndsAtTheirLastRow(t *testing.T) {
	m, _ := suggestionApp(t, 140)
	m.images.Enabled = true
	m.whoToFollow.open = true
	_, areas := m.suggestionModalBody(56, 2, m.account.colors())
	for _, area := range areas {
		if area.action != "wtf-open" || area.index != 0 || area.x != 0 {
			continue
		}
		for _, next := range areas {
			if next.action == "wtf-follow" && next.index == 1 && area.contains(0, next.y) {
				t.Fatalf("the first person's area reaches the second person's row %d", next.y)
			}
		}
	}
}
