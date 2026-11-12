package tui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

const topicID = "0199a4c2-7b1e-7c3a-9f00-1234567890ab"

func recap(text string) *string { return &text }

// topicsApp is a 160x45 workspace whose hot topics come from the fixtures, counting each read.
func topicsApp(t *testing.T) (AppModel, *int, *[]string) {
	t.Helper()
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	m = updated.(AppModel)
	lists, pages := 0, []string{}
	m.headlineCalls = headlineCalls{
		list: func(context.Context) ([]api.Headline, error) {
			lists++
			return []api.Headline{
				{ID: topicID, Name: "Laravel 13 ships", Summary: recap("People upgrade their apps.")},
				{ID: topicID, Name: "PHP 9 RFCs"},
				{ID: topicID, Name: "Pest 5", Summary: recap("Faster parallel runs.")},
				{ID: topicID, Name: "FrankenPHP", Summary: recap("Worker mode everywhere.")},
			}, nil
		},
		page: func(_ context.Context, id string) (api.HeadlinePage, error) {
			pages = append(pages, id)
			commit := fixtureCommit("topic-commit")
			commit.Body = "Upgraded to Laravel 13 today"
			return api.HeadlinePage{Data: []api.Commit{commit}, Name: "Laravel 13 ships", Summary: recap("People upgrade their apps.")}, nil
		},
	}
	return settleTopics(m, m.fetchHeadlines()), &lists, &pages
}

func settleTopics(m AppModel, cmd tea.Cmd) AppModel {
	for _, msg := range collect(cmd) {
		updated, next := m.Update(msg)
		m = updated.(AppModel)
		for _, follow := range collect(next) {
			if _, ok := follow.(tea.BatchMsg); ok {
				continue
			}
			updated, _ = m.Update(follow)
			m = updated.(AppModel)
		}
	}
	return m
}

func TestHotTopicsShowInTheAsideAndOpenTheirCommits(t *testing.T) {
	m, _, pages := topicsApp(t)
	frame := frameText(m)
	for _, want := range []string{"Hot topics", "Laravel 13 ships", "PHP 9 RFCs"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the aside is missing %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "People upgrade their apps.") {
		t.Fatalf("the aside shows titles only:\n%s", frame)
	}
	m, cmd := clickText(t, m, "Laravel 13 ships")
	m = settleTopics(m, cmd)
	frame = frameText(m)
	if len(*pages) != 1 || (*pages)[0] != topicID || !m.starredOpen || m.headline == nil {
		t.Fatalf("a click must open the topic's page, read %v", *pages)
	}
	for _, want := range []string{"People upgrade their apps.", "Upgraded to Laravel 13 today"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the topic page is missing %q:\n%s", want, frame)
		}
	}
	m = pressApp(m, "esc")
	if m.starredOpen || m.headline != nil {
		t.Fatal("esc from a topic opened in the aside must return to the log")
	}
}

func TestGoHotTopicsListsTopicsAndEscReturnsToTheList(t *testing.T) {
	m, _, pages := topicsApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(AppModel)
	m = pressApp(m, "g")
	next, cmd := m.Update(keyMsg("h"))
	m = settleTopics(next.(AppModel), cmd)
	frame := frameText(m)
	if !m.topics.open || !strings.Contains(frame, "Hot topics") || !strings.Contains(frame, "People upgrade their apps.") || !strings.Contains(frame, "1 / 4 hot topics") {
		t.Fatalf("g h must open the Hot topics screen:\n%s", frame)
	}
	m = pressApp(m, "j")
	m = pressApp(m, "k")
	next, cmd = m.Update(keyMsg("enter"))
	m = settleTopics(next.(AppModel), cmd)
	if len(*pages) != 1 || m.headline == nil || m.topics.open {
		t.Fatal("enter must open the selected topic")
	}
	m = pressApp(m, "esc")
	if !m.topics.open || m.starredOpen {
		t.Fatal("esc from a topic opened on the Hot topics screen must return to it")
	}
	m = pressApp(m, "esc")
	if m.topics.open {
		t.Fatal("esc must leave the Hot topics screen")
	}
}

func TestHotTopicsRefreshOnTheMinuteOnlyWhileFocusedAndShown(t *testing.T) {
	m, lists, _ := topicsApp(t)
	before := *lists
	m = settleTopics(m, m.refreshAside())
	if *lists != before+1 {
		t.Fatal("the minute refresh must read hot topics while the aside shows them")
	}
	m.focused = false
	m = settleTopics(m, m.refreshAside())
	if *lists != before+1 {
		t.Fatal("an unfocused terminal must not read hot topics")
	}
	m.focused = true
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(AppModel)
	m = settleTopics(m, m.refreshAside())
	if *lists != before+1 {
		t.Fatal("hot topics must not be read while nothing shows them")
	}
}

func TestShortAsideDropsShortcutsBeforeHotTopics(t *testing.T) {
	m, _, _ := topicsApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 25})
	m = updated.(AppModel)
	if frame := frameText(m); !strings.Contains(frame, "FrankenPHP") || strings.Contains(frame, "Shortcuts") {
		t.Fatalf("a short aside must keep the topics and drop the shortcuts:\n%s", frame)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 18})
	m = updated.(AppModel)
	if frame := frameText(m); strings.Contains(frame, "FrankenPHP") {
		t.Fatalf("an aside too short for every topic must drop the block:\n%s", frame)
	}
}

func TestATopicPageIsNotStarred(t *testing.T) {
	m, _, _ := topicsApp(t)
	m, cmd := clickText(t, m, "Laravel 13 ships")
	m = settleTopics(m, cmd)
	frame := frameText(m)
	if !strings.Contains(frame, "1 / 1 commits about this topic") || strings.Contains(frame, "starred commits") {
		t.Fatalf("the topic page's footer must not call its commits starred:\n%s", frame)
	}
	starred := false
	commit := m.starred.commits[0]
	commit.Starred = &starred
	m.engagement = engagementState{id: commit.ID, kind: "star", generation: 1}
	m.acceptEngagement(engagementResult{session: m.session, generation: 1, id: commit.ID, kind: "star", commit: commit})
	if len(m.starred.commits) != 1 {
		t.Fatal("unstarring a commit on a topic page must keep it in the topic's list")
	}
}

func TestTheHotTopicsScreenScrollsToTheSelectedTopic(t *testing.T) {
	m, _, _ := topicsApp(t)
	long := strings.Repeat("A long synthetic recap about what people commit. ", 6)
	m.topics.list = []api.Headline{{ID: topicID, Name: "Topic one", Summary: &long}, {ID: topicID, Name: "Topic two", Summary: &long}, {ID: topicID, Name: "Topic three", Summary: &long}, {ID: topicID, Name: "Topic four", Summary: &long}, {ID: topicID, Name: "Topic five", Summary: &long}}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(AppModel)
	m, _ = m.openHotTopics()
	for range 4 {
		m = pressApp(m, "j")
	}
	if frame := frameText(m); !strings.Contains(frame, "Topic five") {
		t.Fatalf("the selected topic must be on screen:\n%s", frame)
	}
}

func TestGoHotTopicsLeavesSettings(t *testing.T) {
	m, _, _ := topicsApp(t)
	m.settings.open = true
	m = pressApp(m, "g")
	m = pressApp(m, "h")
	if m.settings.open || !m.topics.open {
		t.Fatal("g h must leave Settings for Hot topics")
	}
}

func TestHotTopicsAreCenteredCardsUnderAnOpenChip(t *testing.T) {
	m, _, _ := topicsApp(t)
	m.topics.list[1].Name = "PHP 9 RFCs for property hooks and the new pattern matching syntax"
	block, areas := m.hotTopicsBlock(37, m.account.colors())
	lines := strings.Split(ansi.Strip(block), "\n")
	if !strings.Contains(lines[0], "Hot topics") || !strings.Contains(lines[0], "Open  H") {
		t.Fatalf("the heading must offer H:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(ansi.Strip(block), "People upgrade their apps.") || strings.Contains(ansi.Strip(block), "…") {
		t.Fatalf("cards show whole titles and no recaps:\n%s", ansi.Strip(block))
	}
	if plain := strings.Join(strings.Fields(ansi.Strip(block)), " "); !strings.Contains(plain, "PHP 9 RFCs for property hooks and the new pattern matching syntax") {
		t.Fatalf("a long title must wrap in full:\n%s", ansi.Strip(block))
	}
	for _, want := range []string{"Laravel 13 ships", "FrankenPHP"} {
		found := false
		for _, line := range lines {
			if strings.TrimSpace(line) == want {
				found = true
				left, right := len(line)-len(strings.TrimLeft(line, " ")), len(line)-len(strings.TrimRight(line, " "))
				if left-right > 1 || right-left > 1 {
					t.Fatalf("%q must be centered in its card: %q", want, line)
				}
			}
		}
		if !found {
			t.Fatalf("missing %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	cards := 0
	for _, area := range areas {
		if area.action == "topic" {
			cards++
		}
	}
	if cards != 4 {
		t.Fatalf("each topic must be a clickable card, got %d", cards)
	}
}

func TestHOpensAPickerOfHotTopics(t *testing.T) {
	m, _, pages := topicsApp(t)
	m = pressApp(m, "H")
	if m.picker == nil || m.picker.kind != "topics" || len(m.picker.rows) != 4 || m.picker.rows[0].label != "Laravel 13 ships" {
		t.Fatalf("H must list the hot topics: %+v", m.picker)
	}
	m = pressApp(m, "j")
	next, cmd := m.Update(keyMsg("enter"))
	if chosen := next.(AppModel).headline; chosen == nil || chosen.topic.Name != "PHP 9 RFCs" {
		t.Fatal("enter must open the chosen topic")
	}
	m = settleTopics(next.(AppModel), cmd)
	if m.picker != nil || m.headline == nil || len(*pages) != 1 {
		t.Fatal("enter must open the chosen topic's page")
	}
}

func TestHotTopicsHasASidebarEntryAboveSettings(t *testing.T) {
	m, _, _ := topicsApp(t)
	frame := frameText(m)
	topics, settings := regexp.MustCompile(`Hot topics +g h`).FindStringIndex(frame), strings.Index(frame, "Settings")
	if topics == nil || settings < topics[0] {
		t.Fatalf("the sidebar must list Hot topics above Settings:\n%s", frame)
	}
	m, cmd := clickText(t, m, regexp.MustCompile(`Hot topics +g h`).FindString(frame))
	m = settleTopics(m, cmd)
	if !m.topics.open {
		t.Fatal("clicking the sidebar entry must open the Hot topics screen")
	}
}

func TestHotTopicRecapsShowInFullInThePickerAndTheScreen(t *testing.T) {
	m, _, _ := topicsApp(t)
	long := "Laravel 13 shipped this morning, and people are upgrading their apps, comparing the new queue features, and sharing the breaking changes they ran into along the way. The recap ends here."
	m.topics.list[0].Summary = &long
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = pressApp(updated.(AppModel), "H")
	frame := frameText(m)
	if !strings.Contains(frame, "Laravel 13 shipped this morning,") || !strings.Contains(frame, "The recap ends here.") {
		t.Fatalf("the picker must show the selected topic's whole recap:\n%s", frame)
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Laravel 13 shipped this morning,") && !strings.Contains(lines[i-1], "────────") {
			t.Fatalf("a rule must split the topics from the recap:\n%s", frame)
		}
	}
	m = pressApp(m, "j")
	if frame := frameText(m); !strings.Contains(frame, "No recap right now.") || strings.Contains(frame, "The recap ends here.") {
		t.Fatalf("the picker must follow the selection, and say when a topic has no recap:\n%s", frame)
	}
	m = pressApp(m, "esc")
	m, _ = m.openHotTopics()
	if frame := frameText(m); !strings.Contains(frame, "Laravel 13 shipped this morning,") || !strings.Contains(frame, "ends here.") || !strings.Contains(frame, "No recap right now.") {
		t.Fatalf("the Hot topics screen must show each whole recap, and say when there is none:\n%s", frame)
	}
}
