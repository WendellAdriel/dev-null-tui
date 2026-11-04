package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const newOrgID = "5d6e7f80-1a2b-4c3d-8e9f-0a1b2c3d4e5f"

// people is a Remotes app whose candidate search and org writes are recorded, with createErr deciding the next answer.
type people struct {
	log       *remoteCallLog
	createErr error
}

func peopleApp(t *testing.T) (AppModel, *people) {
	t.Helper()
	m, log := remotesApp(t, auth.RemotePreferences{})
	state := &people{log: log}
	m.remoteCalls.candidates = func(_ context.Context, q api.CandidateQuery) ([]api.Candidate, error) {
		log.add("candidates %s %s %q", q.For, q.Org, q.Term)
		list := []api.Candidate{
			{User: api.User{ID: "ada", Handle: "ada", Name: "Ada Lovelace"}},
			{User: api.User{ID: "bob", Handle: "bob", Name: "Bob Ross"}, Reason: "only takes remotes from people they follow"},
			{User: api.User{ID: "cy", Handle: "cy", Name: "Cy Twombly"}},
		}
		for i := range 4 {
			list = append(list, api.Candidate{User: api.User{ID: fmt.Sprint("p", i), Handle: fmt.Sprint("p", i), Name: fmt.Sprint("Person ", i)}})
		}
		return list, nil
	}
	m.remoteCalls.createOrg = func(_ context.Context, name string, members []string) (string, error) {
		log.add("create %q %s", name, strings.Join(members, ","))
		if state.createErr != nil {
			return "", state.createErr
		}
		return newOrgID, nil
	}
	m, _ = m.handleKey("g")
	return settle(t, m, "r"), state
}

// searchNow ends the search's wait, as its timer would.
func searchNow(t *testing.T, m AppModel) AppModel {
	t.Helper()
	next, cmd := m.update(peopleSearchTick{session: m.session, generation: m.remotes.people.generation})
	return settleCmd(next.(AppModel), cmd)
}

func typeKeys(t *testing.T, m AppModel, value string) AppModel {
	t.Helper()
	for _, r := range value {
		key := string(r)
		if r == ' ' {
			key = "space"
		}
		m = settle(t, m, key)
	}
	return m
}

func TestNewRemoteFindsSomeoneAndOpensTheirConversation(t *testing.T) {
	m, state := peopleApp(t)
	m = settle(t, m, "N")
	if p := m.remotes.people; p == nil || p.mode != "remote" || !strings.Contains(frameText(m), "Type a name or handle") {
		t.Fatalf("New remote did not open:\n%s", frameText(m))
	}
	m = typeKeys(t, m, "c")
	stale := m.remotes.people.generation
	m = typeKeys(t, m, "y")
	next, _ := m.update(peopleSearchTick{session: m.session, generation: stale})
	if m = next.(AppModel); strings.Contains(state.log.String(), "candidates") {
		t.Fatalf("an outdated search went out: %s", state.log)
	}
	m = searchNow(t, m)
	if !strings.HasSuffix(state.log.String(), `candidates remote  "cy"`) {
		t.Fatalf("search: %s", state.log)
	}
	if frame := frameText(m); !strings.Contains(frame, "@bob · only takes remotes from people they follow") || !strings.Contains(frame, "Cy Twombly") {
		t.Fatalf("results:\n%s", frame)
	}
	m = settle(t, m, "down", "down", "enter")
	if m.remotes.people == nil {
		t.Fatal("chose someone who can't be picked")
	}
	m = settle(t, m, "down", "enter")
	thread := m.remotes.thread
	if m.remotes.people != nil || thread == nil || thread.conversation.Partner.Handle != "cy" || !strings.HasSuffix(state.log.String(), "thread cy 0") {
		t.Fatalf("thread not opened: %s", state.log)
	}
}

func TestNewRemoteOpensAnExistingConversation(t *testing.T) {
	m, _ := peopleApp(t)
	m = settle(t, m, "N")
	m = searchNow(t, typeKeys(t, m, "@ada"))
	m = settle(t, m, "down", "enter")
	if thread := m.remotes.thread; thread == nil || thread.conversation.ID != "d1" {
		t.Fatal("did not reuse the conversation with @ada")
	}
}

func TestInboxSearchListsUpToFivePeopleToStartAConversation(t *testing.T) {
	m, state := peopleApp(t)
	m = settle(t, m, "/")
	m = typeKeys(t, m, "a")
	m = settle(t, m, "enter")
	if !strings.Contains(state.log.String(), `candidates remote  "a"`) {
		t.Fatalf("no people search: %s", state.log)
	}
	frame := frameText(m)
	if !strings.Contains(frame, "Start a conversation") || !strings.Contains(frame, "Person 1") || strings.Contains(frame, "Person 2") {
		t.Fatalf("people list:\n%s", frame)
	}
	m = settle(t, m, "G", "enter")
	if thread := m.remotes.thread; thread == nil || thread.conversation.Partner.Handle != "p1" {
		t.Fatal("enter on a person did not open their conversation")
	}
	m = settle(t, m, "esc", "tab")
	if strings.Count(state.log.String(), "candidates") != 1 {
		t.Fatalf("searched people outside the Open tab: %s", state.log)
	}
}

func TestNewOrgPicksPeopleAndOpensTheOrg(t *testing.T) {
	m, state := peopleApp(t)
	m = settle(t, m, "O")
	if frame := frameText(m); !strings.Contains(frame, "New org") || !strings.Contains(frame, "You maintain it. Everyone you add contributes.") {
		t.Fatalf("New org:\n%s", frame)
	}
	m = typeKeys(t, m, " core"+strings.Repeat("x", 70))
	if got := len([]rune(m.remotes.people.name)); got != api.MaxOrgName {
		t.Fatalf("name kept %d characters", got)
	}
	m.remotes.people.name = " core "
	m = settle(t, m, "tab")
	m = searchNow(t, typeKeys(t, m, "a"))
	m = settle(t, m, "down", "space", "down", "space", "down", "space")
	if picks := m.remotes.people.picks; len(picks) != 2 || picks[0].Handle != "ada" || picks[1].Handle != "cy" {
		t.Fatalf("picks %+v", picks)
	}
	m = settle(t, m, "up", "up", "space")
	if picks := m.remotes.people.picks; len(picks) != 1 || picks[0].Handle != "cy" {
		t.Fatalf("space did not unpick @ada: %+v", picks)
	}
	m = settle(t, m, "ctrl+s")
	if !strings.Contains(state.log.String(), `create " core " cy`) {
		t.Fatalf("create: %s", state.log)
	}
	if thread := m.remotes.thread; m.remotes.people != nil || thread == nil || thread.conversation.ID != newOrgID || !strings.Contains(state.log.String(), "thread "+newOrgID) {
		t.Fatalf("the new org did not open: %s", state.log)
	}
}

func TestARefusedOrgKeepsThePicksAndShowsWhy(t *testing.T) {
	m, state := peopleApp(t)
	state.createErr = &auth.APIError{Status: http.StatusUnprocessableEntity, FieldErrors: []string{"@cy only takes remotes from people they follow."}}
	m = settle(t, m, "O", "tab")
	m = searchNow(t, typeKeys(t, m, "c"))
	m = settle(t, m, "down", "down", "down", "space", "ctrl+s")
	p := m.remotes.people
	if p == nil || len(p.picks) != 1 || !strings.Contains(frameText(m), "@cy only takes remotes from people they follow.") {
		t.Fatalf("refusal:\n%s", frameText(m))
	}
	state.createErr = &auth.APIError{Status: http.StatusTooManyRequests, RetryAfter: "30"}
	m = settle(t, m, "ctrl+s")
	if !strings.Contains(frameText(m), "30 seconds") {
		t.Fatalf("rate limit:\n%s", frameText(m))
	}
}

func TestAConversationOpenedByHandleTakesTheInboxRow(t *testing.T) {
	m, _ := peopleApp(t)
	ada := api.User{ID: "ada", Handle: "ada", Name: "Ada Lovelace"}
	m = settleCmd(m, m.openRemoteThread(api.Conversation{ID: "@ada", Kind: "direct", Partner: &ada}))
	m.remotes.focusThread = false
	m = settle(t, m, "ctrl+r")
	if thread := m.remotes.thread; thread == nil || thread.conversation.ID != "d1" {
		t.Fatalf("thread kept %q", m.remotes.thread.conversation.ID)
	}
}
