package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type completionLog struct{ mentions, issues []string }

func withCompletion(m AppModel) (AppModel, *completionLog) {
	log := &completionLog{}
	m.completionCalls = completionCalls{
		mentions: func(_ context.Context, q api.MentionQuery) (api.Profiles, error) {
			log.mentions = append(log.mentions, fmt.Sprintf("q=%s org=%s reply=%s quote=%s", q.Term, q.Org, q.Reply, q.Quote))
			return api.Profiles{
				Users:         []api.User{{Handle: "ada", Name: "Ada Lovelace"}, {Handle: "adam", Name: "Adam"}},
				Organizations: []api.Organization{{Handle: "adalabs", Name: "Ada Labs"}},
			}, nil
		},
		issue: func(_ context.Context, number string) (*api.Issue, error) {
			log.issues = append(log.issues, number)
			if number != "42" {
				return nil, nil
			}
			return &api.Issue{Number: 42, Title: "Dark mode", URL: "https://github.com/nunomaduro/dev-null.sh/issues/42"}, nil
		},
	}
	return m, log
}

// typeComposer types into the composer, then runs what the last keystroke scheduled, as a pause in typing would.
func typeComposer(m AppModel, value string) AppModel {
	var cmd tea.Cmd
	for _, r := range value {
		var next tea.Model
		next, cmd = m.update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(AppModel)
	}
	return settleCompletion(m, cmd)
}

func settleCompletion(m AppModel, cmd tea.Cmd) AppModel {
	for _, msg := range runWithin(cmd, 300_000_000) {
		switch msg.(type) {
		case completionTick, completionResult:
			next, more := m.update(msg)
			m = settleCompletion(next.(AppModel), more)
		}
	}
	return m
}

func composerKey(m AppModel, name string) AppModel {
	next, cmd := m.update(keyMsg(name))
	return settleCompletion(next.(AppModel), cmd)
}

func TestMentionsAreSuggestedAndInsertedWithASpace(t *testing.T) {
	m, log := withCompletion(fixtureApp(t))
	m = pressApp(m, "c")
	m = typeComposer(m, "hi @ad")
	if fmt.Sprint(log.mentions) != "[q=ad org= reply= quote=]" {
		t.Fatalf("lookups: %q", log.mentions)
	}
	frame := frameText(m)
	for _, want := range []string{"> @ada Ada Lovelace", "@adam Adam", "@adalabs Ada Labs · org"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("suggestions lack %q:\n%s", want, frame)
		}
	}
	m = composerKey(m, "down")
	m = composerKey(m, "enter")
	if got := m.composer.input.Value(); got != "hi @adam " {
		t.Fatalf("body: %q", got)
	}
	m = typeComposer(m, "and @ad")
	if len(log.mentions) != 1 {
		t.Fatalf("a repeated query must come from the cache: %q", log.mentions)
	}
	m = composerKey(m, "esc")
	if !m.composer.open || m.composer.suggest.count() != 0 {
		t.Fatal("Esc must dismiss suggestions before closing the composer")
	}
}

func TestRepliesAndQuotesSuggestTheirThreadFirst(t *testing.T) {
	m, log := withCompletion(threadApp(t))
	m = press(t, m, "r")
	m = typeComposer(m, "@")
	m = composerKey(m, "esc")
	m = composerKey(m, "esc")
	m = pressApp(m, "ctrl+d")
	m = press(t, m, "q")
	typeComposer(m, "@g")
	want := "[q= org= reply=a quote= q=g org= reply= quote=a]"
	if fmt.Sprint(log.mentions) != want {
		t.Fatalf("lookups: %q, want %s", log.mentions, want)
	}
}

func TestIssueNumbersBecomeLinksButColorsAndCodeStayQuiet(t *testing.T) {
	m, log := withCompletion(fixtureApp(t))
	m = pressApp(m, "c")
	m = typeComposer(m, "see #42")
	if fmt.Sprint(log.issues) != "[42]" || !strings.Contains(frameText(m), "#42 Dark mode") {
		t.Fatalf("issue lookup: %q\n%s", log.issues, frameText(m))
	}
	m = composerKey(m, "tab")
	if got := m.composer.input.Value(); got != "see https://github.com/nunomaduro/dev-null.sh/issues/42 " {
		t.Fatalf("body: %q", got)
	}
	for _, body := range []string{"#333333", "`#42", "x#42", "#tag"} {
		m, log := withCompletion(fixtureApp(t))
		m = pressApp(m, "c")
		typeComposer(m, body)
		if len(log.issues) != 0 {
			t.Fatalf("%q looked up an issue", body)
		}
	}
	m, log = withCompletion(fixtureApp(t))
	m = pressApp(m, "c")
	m = typeComposer(m, "#7")
	if fmt.Sprint(log.issues) != "[7]" || m.composer.suggest.count() != 0 {
		t.Fatal("an unknown issue shows nothing")
	}
}

func TestOrgMessagesSuggestOnlyItsMembers(t *testing.T) {
	m, _ := messagingApp(t)
	m, log := withCompletion(m)
	m = openOrg(t, m)
	m = send(m, typed("w")...)
	m = typeComposer(m, "@a")
	if len(log.mentions) != 1 || !strings.Contains(log.mentions[0], "org="+m.remotes.thread.conversation.ID) {
		t.Fatalf("lookups: %q", log.mentions)
	}
	if !strings.Contains(threadText(m), "@ada Ada Lovelace") {
		t.Fatalf("suggestions:\n%s", threadText(m))
	}
	m = composerKey(m, "enter")
	if got := m.remotes.thread.composer.input.Value(); got != "@ada " {
		t.Fatalf("message: %q", got)
	}
}
