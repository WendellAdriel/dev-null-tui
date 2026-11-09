package tui

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func ownCommit(id string) api.Commit {
	c := fixtureCommit(id)
	c.Author.ID, c.Author.Handle = "fixture", "fixture"
	c.Pinned = new(false)
	return c
}

func TestPinAsksBeforeReplacingAnotherPinAndMovesThePin(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{ownCommit("a"), ownCommit("b")}})
	old := ownCommit("old")
	old.Pinned = new(true)
	m.loadProfile = func(context.Context, string) (api.Profile, error) { return api.Profile{Pinned: &old}, nil }
	var sent []bool
	m.commitCalls.pin = func(_ context.Context, id string, pinned bool) (api.Commit, error) {
		sent = append(sent, pinned)
		c := ownCommit(id)
		c.Pinned = new(pinned)
		return c, nil
	}
	m, cmd := m.handleKey("P")
	m = deliver(m, runAll(cmd))
	if m.ask == nil || len(sent) != 0 || !strings.Contains(ansi.Strip(m.View().Content), "Replace your pinned commit?") {
		t.Fatal("replacing a pin did not ask first")
	}
	m, cmd = m.askKey("enter")
	m = deliver(m, runAll(cmd))
	if len(sent) != 1 || !sent[0] || !*m.feeds[0].commits[0].Pinned {
		t.Fatalf("pin not saved: %v", sent)
	}
	m, cmd = m.handleKey("P")
	m = deliver(m, runAll(cmd))
	if len(sent) != 2 || sent[1] || *m.feeds[0].commits[0].Pinned {
		t.Fatalf("unpin did not run at once: %v", sent)
	}
}

func TestPinIsOnlyOfferedOnYourOwnTopLevelCommits(t *testing.T) {
	m := fixtureApp(t)
	reply := ownCommit("r")
	parent := "a"
	reply.ParentID = &parent
	if m.canPinCommit(fixtureCommit("x")) || m.canPinCommit(reply) || !m.canPinCommit(ownCommit("a")) {
		t.Fatal("pin offered on the wrong commit")
	}
}

func TestStarredByPagesAndOpensAProfile(t *testing.T) {
	m := fixtureApp(t)
	starred := fixtureCommit("a")
	starred.Stars = 3
	starred.Author.ID = m.account.user.ID
	m = applyPage(m, 0, api.Page{Data: []api.Commit{starred}})
	var cursors []string
	m.commitCalls.stargazers = func(_ context.Context, _, cursor string) (api.People, error) {
		cursors = append(cursors, cursor)
		if cursor == "" {
			return api.People{Data: []api.User{{ID: "1", Handle: "grace", Name: "Grace"}, {ID: "2", Handle: "linus", Name: "Linus"}}, Next: "c2"}, nil
		}
		return api.People{Data: []api.User{{ID: "3", Handle: "ada", Name: "Ada"}}}, nil
	}
	opened := ""
	m.loadProfile = func(_ context.Context, handle string) (api.Profile, error) {
		opened = handle
		return api.Profile{User: api.User{ID: "u", Handle: handle}}, nil
	}
	m, cmd := m.handleKey("S")
	m = deliver(m, runAll(cmd))
	frame := ansi.Strip(m.View().Content)
	if m.picker == nil || !strings.Contains(frame, "Starred by") || !strings.Contains(frame, "@linus") || !strings.Contains(frame, "3 people starred") {
		t.Fatalf("Starred by dialog missing:\n%s", frame)
	}
	m, cmd = m.listPickerKey("n")
	m = deliver(m, runAll(cmd))
	if len(m.picker.rows) != 3 || m.picker.next != "" || strings.Join(cursors, ",") != ",c2" {
		t.Fatalf("paging: %v %d", cursors, len(m.picker.rows))
	}
	m, _ = m.listPickerKey("j")
	m, cmd = m.listPickerKey("enter")
	m = deliver(m, runAll(cmd))
	if m.picker != nil || opened != "linus" {
		t.Fatalf("enter opened %q", opened)
	}
}

func TestTranslateRetriesPendingThenTogglesTheOriginal(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	m.commitCalls.settings = func(context.Context) (api.AccountSettings, error) {
		return api.AccountSettings{TranslationLanguage: "pt"}, nil
	}
	calls := 0
	m.commitCalls.translate = func(_ context.Context, _, language string) (api.Translation, error) {
		calls++
		if language != "pt" {
			t.Errorf("language %q", language)
		}
		tr := api.Translation{Language: language, SourceName: "English", Available: true}
		if calls > 1 {
			tr.Translated = &struct {
				Segments []api.Segment `json:"body_segments"`
				Preview  []api.Segment `json:"preview_segments"`
			}{Segments: []api.Segment{{Type: "text", Text: "Olá mundo traduzido"}}}
		}
		return tr, nil
	}
	m, cmd := m.handleKey("t")
	if !strings.Contains(ansi.Strip(m.View().Content), "Translating…") {
		t.Fatal("pending translation not shown")
	}
	msgs := runAll(cmd)
	next, retry := m.update(msgs[0])
	m = next.(AppModel)
	m = deliver(m, runAll(retry))
	frame := ansi.Strip(m.View().Content)
	if calls != 2 || !strings.Contains(frame, "Olá mundo traduzido") || !strings.Contains(frame, "Translated from English") {
		t.Fatalf("translation not shown after %d calls:\n%s", calls, frame)
	}
	m, _ = m.handleKey("t")
	if strings.Contains(ansi.Strip(m.View().Content), "Olá mundo traduzido") || calls != 2 {
		t.Fatal("t did not switch back to the original without a request")
	}
}

func TestTranslationLanguageFallsBackToTheLocaleThenEnglish(t *testing.T) {
	langs := []api.Language{{Code: "pt"}, {Code: "de"}}
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "pt_PT.UTF-8")
	if got := translationLanguage(api.AccountSettings{TranslationLanguages: langs}); got != "pt" {
		t.Fatalf("locale: %q", got)
	}
	t.Setenv("LANG", "ja_JP.UTF-8")
	if got := translationLanguage(api.AccountSettings{TranslationLanguages: langs}); got != "en" {
		t.Fatalf("unsupported locale: %q", got)
	}
	if got := translationLanguage(api.AccountSettings{TranslationLanguage: "de"}); got != "de" {
		t.Fatalf("setting: %q", got)
	}
}

func TestRelatedCommitsShowBelowTheThreadAndOpen(t *testing.T) {
	m := threadApp(t)
	related := fixtureCommit("rel")
	related.Body = "A related idea"
	m.commitCalls.related = func(context.Context, string) ([]api.Commit, error) { return []api.Commit{related}, nil }
	m = openThread(t, m)
	m = deliver(m, runAll(m.loadRelated(&m.threads[0])))
	m, _ = m.handleKey("G")
	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, "Related commits") || !strings.Contains(frame, "A related idea") {
		t.Fatalf("related commits missing:\n%s", frame)
	}
	m.commitCalls.related = func(context.Context, string) ([]api.Commit, error) { return nil, nil }
	m, cmd := m.handleKey("enter")
	m = deliver(m, runAll(cmd))
	if len(m.threads) != 2 || m.threads[1].root.ID != "rel" {
		t.Fatal("enter did not open the related commit")
	}
}

func TestScheduledScreenSubmitsCancelsAndRefreshesOnConflict(t *testing.T) {
	m := fixtureApp(t)
	const first, second = "0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a11", "0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a12"
	body := "Ship it later"
	item := func(id string) api.ScheduledCommit {
		return api.ScheduledCommit{ID: id, Body: &body, Status: "scheduled", ScheduledAt: m.now.Add(time.Hour), Author: &api.Author{Type: "organization", Handle: "laravel"}}
	}
	loads := 0
	m.commitCalls.scheduled = func(context.Context, int) (api.ScheduledPage, error) {
		loads++
		done := item("0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a13")
		done.Status = "approved"
		return api.ScheduledPage{Data: []api.ScheduledCommit{item(first), item(second), done}}, nil
	}
	m.commitCalls.submitNow = func(_ context.Context, id string) (api.ScheduledCommit, error) {
		s := item(id)
		s.Status, s.Body, s.SubmittedAt = "pending", nil, &m.now
		return s, nil
	}
	m.commitCalls.cancelLater = func(context.Context, string) error {
		return &auth.APIError{Status: http.StatusConflict}
	}
	m, _ = m.handleKey("g")
	m, cmd := m.handleKey("t")
	m = deliver(m, runAll(cmd))
	frame := ansi.Strip(m.View().Content)
	for _, want := range []string{"Scheduled commits", "Private", "Ship it later", "As @laravel", "Commit now"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q:\n%s", want, frame)
		}
	}
	if len(m.scheduled.items) != 2 || strings.Contains(frame, "Approved") {
		t.Fatalf("already committed rows should be hidden:\n%s", frame)
	}
	m, cmd = m.handleKey("enter")
	m = deliver(m, runAll(cmd))
	if len(m.scheduled.items) != 1 || m.scheduled.items[0].ID != second || m.toast.title != "Submitted for review" {
		t.Fatalf("commit now should drop the row: %d %q", len(m.scheduled.items), m.toast.title)
	}
	m, _ = m.handleKey("D")
	if m.ask == nil {
		t.Fatal("cancel did not ask")
	}
	m, cmd = m.askKey("ctrl+y")
	next, refresh := m.update(runAll(cmd)[0])
	m = next.(AppModel)
	if m.toast.title != "Already handled" {
		t.Fatalf("conflict toast: %q", m.toast.title)
	}
	for _, msg := range runAll(refresh) {
		if result, ok := msg.(scheduledResult); ok {
			m = deliver(m, []tea.Msg{result})
		}
	}
	if loads != 2 {
		t.Fatalf("conflict did not refresh: %d loads", loads)
	}
}

func TestComposerSchedulesATopLevelCommitAsAnOrganization(t *testing.T) {
	m := fixtureApp(t)
	var sentAt time.Time
	var sent api.NewCommit
	m.commitCalls.schedule = func(_ context.Context, draft api.NewCommit, at time.Time) (api.ScheduledCommit, error) {
		sent, sentAt = draft, at
		return api.ScheduledCommit{Status: "scheduled", ScheduledAt: at}, nil
	}
	submitted := false
	m.submit = func(context.Context, api.NewCommit) (api.Commit, error) { submitted = true; return api.Commit{}, nil }
	m, _ = m.handleKey("c")
	m.composer.input.SetValue("Ship it tomorrow")
	m.composer.author = &auth.Organization{Handle: "laravel"}
	m = pressApp(m, "ctrl+t")
	if m.composer.schedule == nil || !strings.Contains(ansi.Strip(m.View().Content), "Submit at") {
		t.Fatal("ctrl+t did not open the picker")
	}
	m = pressApp(m, "tab")
	m = pressApp(m, "up")
	m = pressApp(m, "enter")
	if m.composer.scheduleAt == nil || !strings.Contains(ansi.Strip(m.View().Content), "Schedule") {
		t.Fatal("picked time not kept")
	}
	m = deliver(m, runAll(m.startSubmission()))
	if submitted || sent.Author != "laravel" || sent.Body != "Ship it tomorrow" || !sentAt.After(time.Now()) {
		t.Fatalf("scheduled wrong: %+v %v submitted=%v", sent, sentAt, submitted)
	}
	if m.composer.open || !strings.Contains(m.notice, "g t") {
		t.Fatal("composer not cleared after scheduling")
	}
}

func TestSchedulePickerRefusesPastAndSkippedTimes(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 30, 0, 0, time.Local)
	s := schedulePicker{hour: 13}
	at, ok := s.at(now)
	if !ok || at.After(now) {
		t.Fatalf("today 13:00 should exist and be past: %v", at)
	}
	if picker := newSchedulePicker(nil, now); picker.day != 0 || picker.hour != 15 {
		t.Fatalf("default should be the next hour: %+v", picker)
	}
	if picker := newSchedulePicker(nil, time.Date(2026, 11, 9, 23, 10, 0, 0, time.Local)); picker.day != 1 || picker.hour != 9 {
		t.Fatalf("late default should be 09:00 tomorrow: %+v", picker)
	}
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Skip("no zone database:", err)
	}
	local := time.Local
	time.Local = lisbon
	defer func() { time.Local = local }()
	// Lisbon skips 01:00 to 02:00 on 28 March 2027.
	eve := time.Date(2027, 3, 27, 12, 0, 0, 0, lisbon)
	if _, ok := (schedulePicker{day: 1, hour: 1, minute: 30}).at(eve); ok {
		t.Fatal("01:30 on the daylight saving night does not exist in Lisbon")
	}
	if _, ok := (schedulePicker{day: 1, hour: 2, minute: 30}).at(eve); !ok {
		t.Fatal("02:30 after the jump exists")
	}
}

func TestRelatedCommitsFollowWritesAndDeletes(t *testing.T) {
	m := threadApp(t)
	related := ownCommit("rel")
	m.commitCalls.related = func(context.Context, string) ([]api.Commit, error) { return []api.Commit{related}, nil }
	m = openThread(t, m)
	m = deliver(m, runAll(m.loadRelated(&m.threads[0])))
	starred := false
	m.eachCommit(func(c *api.Commit) {
		if c != nil && c.ID == "rel" {
			starred = true
			c.Stars = 7
		}
	})
	if !starred || m.threads[0].related[0].Stars != 7 {
		t.Fatal("shared commit updates skip related commits")
	}
	next, _ := m.update(deleteResult{session: m.session, id: "rel", sentAt: m.now})
	m = next.(AppModel)
	if m.threads[0].related[0].DeletedAt == nil {
		t.Fatal("a deleted related commit stays visible")
	}
}

func TestTranslationAlreadyInYourLanguageAsksOnce(t *testing.T) {
	m := fixtureApp(t)
	m = applyPage(m, 0, api.Page{Data: []api.Commit{fixtureCommit("a")}})
	m.commitCalls.settings = func(context.Context) (api.AccountSettings, error) {
		return api.AccountSettings{TranslationLanguage: "en"}, nil
	}
	calls := 0
	m.commitCalls.translate = func(_ context.Context, _, language string) (api.Translation, error) {
		calls++
		return api.Translation{Language: language, SourceLanguage: language}, nil
	}
	m, cmd := m.handleKey("t")
	msgs := runAll(cmd)
	next, retry := m.update(msgs[0])
	m = next.(AppModel)
	if m.toast.title != "Already in your language" {
		t.Fatalf("toast %q", m.toast.title)
	}
	for _, msg := range runAll(retry) {
		if _, again := msg.(translationResult); again {
			t.Fatal("a translation that isn't needed was retried")
		}
	}
	if calls != 1 || m.toast.title != "Already in your language" {
		t.Fatalf("calls %d, toast %q", calls, m.toast.title)
	}
}

func TestStarredByIsOnlyOfferedOnYourOwnCommits(t *testing.T) {
	m := fixtureApp(t)
	others := fixtureCommit("a")
	others.Stars = 3
	m = applyPage(m, 0, api.Page{Data: []api.Commit{others}})
	asked := 0
	m.commitCalls.stargazers = func(context.Context, string, string) (api.People, error) {
		asked++
		return api.People{}, nil
	}
	if strings.Contains(ansi.Strip(m.View().Content), "Starred by") {
		t.Fatal("the server answers 404 to anyone but the author, so the chip must not show")
	}
	m, cmd := m.handleKey("S")
	m = deliver(m, runAll(cmd))
	m, cmd = clickText(t, m, "☆ 3")
	m = settleCmd(m, cmd)
	if asked != 0 || m.picker != nil {
		t.Fatalf("Starred by opened on someone else's commit: %d", asked)
	}
}

func TestOrganizationPagesKeepSettingsOnS(t *testing.T) {
	m, log := organizationApp(t)
	m.organizationCalls.commits = func(_ context.Context, handle string, _ bool, _ string) (api.CommitPage, error) {
		c := orgCommit(handle + "-commit")
		c.Stars = 3
		return api.CommitPage{Data: []api.Commit{c}}, nil
	}
	m = press(t, m, "u")
	settings := "https://dev-null.test/orgs/acme/settings"
	m.topProfile().page.SettingsURL = &settings
	m.renderProfile(m.topProfile())
	if strings.Contains(profileText(m), "Starred by") {
		t.Fatal("org commits show the org as author, so Starred by is not offered")
	}
	m = settle(t, m, "S")
	if strings.Join(log.opened, " ") != settings {
		t.Fatalf("S should open the org settings: %v", log.opened)
	}
}

func TestLeavingTheAccountClearsPinTranslationAndDialogs(t *testing.T) {
	m := fixtureApp(t)
	m.pinning = "a"
	m.translations = map[string]translation{"a": {pending: true}}
	m.ask = &ask{kind: "pin"}
	m.picker = &picker{kind: "links"}
	m.leaveApplication()
	if m.pinning != "" || m.translations != nil || m.ask != nil || m.picker != nil {
		t.Fatal("state from the old account survives")
	}
}

func TestAFailedPinCheckDoesNotReplaceThePin(t *testing.T) {
	m := fixtureApp(t)
	c := ownCommit("a")
	saved := 0
	m.commitCalls.pin = func(context.Context, string, bool) (api.Commit, error) {
		saved++
		return c, nil
	}
	m.pinning = c.ID
	m.acceptCommitPinCheck(commitPinCheck{session: m.session, commit: c, err: errors.New("offline")})
	if saved != 0 || m.toast.title != "Pin failed" || m.pinning != "" {
		t.Fatalf("saved %d, toast %q", saved, m.toast.title)
	}
}

func TestScheduledPagingSurvivesACancelWithoutRepeatsOrGaps(t *testing.T) {
	m := fixtureApp(t)
	ids := []string{"0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a01", "0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a02", "0b0e6c43-7d0e-4a8e-9a39-6a3a1e8f0a03"}
	server := slices.Clone(ids)
	var pages []int
	m.commitCalls.scheduled = func(_ context.Context, page int) (api.ScheduledPage, error) {
		pages = append(pages, page)
		result := api.ScheduledPage{}
		from := (page - 1) * 2
		for _, id := range server[min(from, len(server)):min(from+2, len(server))] {
			result.Data = append(result.Data, api.ScheduledCommit{ID: id, Status: "scheduled", ScheduledAt: m.now.Add(time.Hour)})
		}
		result.Meta.CurrentPage, result.Meta.LastPage = page, (len(server)+1)/2
		return result, nil
	}
	m.commitCalls.cancelLater = func(_ context.Context, id string) error {
		server = slices.DeleteFunc(server, func(have string) bool { return have == id })
		return nil
	}
	m, _ = m.handleKey("g")
	m, cmd := m.handleKey("t")
	m = settleCmd(m, cmd)
	m, _ = m.handleKey("D")
	m, cmd = m.askKey("ctrl+y")
	m = settleCmd(m, cmd)
	m = settleCmd(m, m.fetchScheduled(true))
	var got []string
	for _, item := range m.scheduled.items {
		got = append(got, item.ID)
	}
	if !slices.Equal(got, server) {
		t.Fatalf("after a cancel the list shows %v, the server has %v (pages %v)", got, server, pages)
	}
}

func TestScheduledOpensFromTheSidebarAndTheComposerHintsCtrlT(t *testing.T) {
	m := fixtureApp(t)
	opened := 0
	m.commitCalls.scheduled = func(context.Context, int) (api.ScheduledPage, error) {
		opened++
		return api.ScheduledPage{}, nil
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 45})
	m = next.(AppModel)
	m, cmd := clickText(t, m, "Scheduled")
	m = settleCmd(m, cmd)
	if opened != 1 || m.scheduled == nil {
		t.Fatalf("the Scheduled sidebar item should open the list: %d", opened)
	}
	m.closeStarred()
	m, _ = m.handleKey("g")
	m, cmd = m.handleKey("t")
	m = settleCmd(m, cmd)
	if opened != 2 {
		t.Fatalf("g t should open Scheduled: %d", opened)
	}
	m.closeStarred()
	m, cmd = m.handleKey("c")
	m = deliver(m, runAll(cmd))
	if !strings.Contains(ansi.Strip(m.View().Content), "Ctrl+T schedule") {
		t.Fatalf("the composer should hint Ctrl+T:\n%s", ansi.Strip(m.View().Content))
	}
}
