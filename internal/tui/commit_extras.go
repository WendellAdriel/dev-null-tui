package tui

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type commitPinCheck struct {
	session int
	commit  api.Commit
	pinned  *api.Commit
	err     error
}

type commitPinResult struct {
	session int
	id      string
	pinned  bool
	commit  api.Commit
	err     error
}

func (m AppModel) canPinCommit(c api.Commit) bool {
	return m.ownCommit(c) && c.ParentID == nil && c.Status != "rejected"
}

// startPin unpins a pinned commit at once, and checks the profile first so replacing a pin asks.
func (m *AppModel) startPin(c api.Commit) tea.Cmd {
	if !m.canPinCommit(c) || m.pinning != "" || m.commitCalls.pin == nil {
		return nil
	}
	if c.Pinned != nil && *c.Pinned {
		return m.savePin(c, false)
	}
	if m.loadProfile == nil {
		return m.savePin(c, true)
	}
	m.pinning = c.ID
	session, handle, load, life := m.session, m.account.user.Handle, m.loadProfile, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 15*time.Second)
		defer cancel()
		profile, err := load(ctx, handle)
		return commitPinCheck{session: session, commit: c, pinned: profile.Pinned, err: err}
	})
}

func (m *AppModel) acceptCommitPinCheck(msg commitPinCheck) tea.Cmd {
	if msg.session != m.session || m.pinning != msg.commit.ID {
		return nil
	}
	m.pinning = ""
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		return m.notify("error", "Pin failed", "Could not check your pinned commit. Try again.")
	}
	if msg.pinned == nil || msg.pinned.ID == msg.commit.ID {
		return m.savePin(msg.commit, true)
	}
	// Another dialog or the account screen would hide the question, so ask on the next P instead.
	if m.accountOpen || m.ask != nil || m.picker != nil || m.deleting.id != "" {
		return m.notify("info", "Pin not changed", "Press P again to replace your pinned commit.")
	}
	detail := "Your pinned commit goes back to your commits."
	if msg.pinned.Hash != "" {
		detail = "Commit " + safeText(msg.pinned.Hash) + " goes back to your commits."
	}
	m.ask = &ask{kind: "pin", title: "/dev/null · PIN", question: "Replace your pinned commit?", detail: detail, confirm: "Pin", border: m.account.colors().warning, focus: 1, commit: msg.commit}
	return nil
}

func (m *AppModel) savePin(c api.Commit, pinned bool) tea.Cmd {
	if m.commitCalls.pin == nil {
		return nil
	}
	m.pinning = c.ID
	session, set, life := m.session, m.commitCalls.pin, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		commit, err := set(ctx, c.ID, pinned)
		return commitPinResult{session: session, id: c.ID, pinned: pinned, commit: commit, err: err}
	})
}

func (m *AppModel) acceptCommitPin(msg commitPinResult) tea.Cmd {
	if msg.session != m.session || msg.id != m.pinning {
		return nil
	}
	m.pinning = ""
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		var failure *auth.APIError
		switch {
		case errors.As(msg.err, &failure) && failure.Status == http.StatusTooManyRequests:
			return m.notify("warning", "Slow down", retryAfterText(failure.RetryAfter, "Try pinning again in a minute."))
		case failure != nil && failure.Status == http.StatusForbidden:
			return m.notify("error", "Can't pin this commit", "Only your own top-level commits can be pinned.")
		}
		return m.notify("error", "Could not update your pinned commit", safeText(msg.err.Error()))
	}
	m.applyPin(msg.commit, msg.pinned)
	if msg.pinned {
		return m.notify("success", "Pinned", "Commit pinned to your profile.")
	}
	return m.notify("success", "Unpinned", "Commit unpinned.")
}

// applyPin moves the pin on every loaded copy, and swaps it on your loaded profiles like the web's withPinChanges.
func (m *AppModel) applyPin(receipt api.Commit, pinned bool) {
	own := m.account.user.ID
	m.eachCommit(func(c *api.Commit) {
		if c == nil || c.DeletedAt != nil || c.Author == nil || c.Author.ID != own {
			return
		}
		if c.ID == receipt.ID {
			c.Pinned = new(pinned)
		} else if pinned && c.Pinned != nil && *c.Pinned {
			c.Pinned = new(false)
		}
	})
	for i := range m.profiles {
		s := &m.profiles[i]
		if s.people || s.organization || !strings.EqualFold(s.handle, m.account.user.Handle) {
			continue
		}
		list := &s.lists[0]
		previous := s.profile.Pinned
		if pinned {
			list.commits = slices.DeleteFunc(list.commits, func(c api.Commit) bool { return c.ID == receipt.ID })
			s.profile.Pinned = &receipt
		} else if previous != nil && previous.ID == receipt.ID {
			s.profile.Pinned = nil
			previous = &receipt
		}
		if previous != nil && (s.profile.Pinned == nil || previous.ID != s.profile.Pinned.ID) {
			list.commits = insertByDate(list.commits, *previous, list.next != "")
		}
		list.selected = max(0, min(list.selected, len(s.entriesOf(0))-1))
		m.renderProfile(s)
	}
	m.reflow()
}

// insertByDate puts a commit back into a newest-first list, leaving it out when it belongs on a page not loaded yet.
func insertByDate(commits []api.Commit, c api.Commit, more bool) []api.Commit {
	if slices.ContainsFunc(commits, func(o api.Commit) bool { return o.ID == c.ID }) {
		return commits
	}
	at := slices.IndexFunc(commits, func(o api.Commit) bool { return o.CreatedAt.Before(c.CreatedAt) })
	if at < 0 {
		if more {
			return commits
		}
		at = len(commits)
	}
	return slices.Insert(commits, at, c)
}

type stargazersResult struct {
	session, generation int
	people              api.People
	err                 error
}

// openStargazersFor opens Starred by for a commit drawn anywhere, from a click on its star count.
func (m *AppModel) openStargazersFor(id string) tea.Cmd {
	var found *api.Commit
	m.eachCommit(func(c *api.Commit) {
		if found == nil && c != nil && c.ID == id {
			copy := *c
			found = &copy
		}
	})
	if found == nil {
		return nil
	}
	return m.openStargazers(*found)
}

func (m *AppModel) openStargazers(c api.Commit) tea.Cmd {
	// The server shows stargazers only to the author, as the web does.
	if !m.ownCommit(c) || c.Stars == 0 || m.commitCalls.stargazers == nil {
		return nil
	}
	intro := "1 person starred this commit."
	if c.Stars != 1 {
		intro = strconv.Itoa(c.Stars) + " people starred this commit."
	}
	m.pickerGeneration++
	m.picker = &picker{kind: "stars", title: "Starred by", intro: intro, commitID: c.ID, generation: m.pickerGeneration}
	return m.loadStargazers(true)
}

func (m *AppModel) loadStargazers(first bool) tea.Cmd {
	d := m.picker
	if d == nil || d.kind != "stars" || d.loading || (!first && d.next == "") {
		return nil
	}
	d.loading, d.status = true, ""
	session, generation, id, cursor, load, life := m.session, d.generation, d.commitID, d.next, m.commitCalls.stargazers, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		people, err := load(ctx, id, cursor)
		return stargazersResult{session: session, generation: generation, people: people, err: err}
	})
}

func (m *AppModel) acceptStargazers(msg stargazersResult) tea.Cmd {
	d := m.picker
	if msg.session != m.session || d == nil || d.generation != msg.generation {
		return nil
	}
	d.loading = false
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		d.status = "fatal: could not load stars"
		return nil
	}
	for _, person := range msg.people.Data {
		if m.safetyHidden[strings.ToLower(person.Handle)] != 0 {
			continue
		}
		d.rows = append(d.rows, pickerRow{person: &person, label: person.Name, note: "@" + person.Handle})
	}
	d.next = msg.people.Next
	if len(d.rows) == 0 && d.next == "" {
		d.status = "No stars yet."
	}
	return m.syncImages()
}

type translationResult struct {
	session, attempt int
	id               string
	translation      api.Translation
	err              error
}

// translation is one commit's translated body, shown in place of the original until toggled off.
type translation struct {
	source   string
	segments []api.Segment
	preview  []api.Segment
	shown    bool
	pending  bool
}

const translationAttempts = 3

// translationLanguage is the Region tab's language, else the locale's when the server offers it, else English.
func translationLanguage(settings api.AccountSettings) string {
	if settings.TranslationLanguage != "" {
		return settings.TranslationLanguage
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		code, _, _ := strings.Cut(strings.ToLower(os.Getenv(name)), "_")
		code, _, _ = strings.Cut(code, ".")
		if code == "" || code == "c" || code == "posix" {
			continue
		}
		if slices.ContainsFunc(settings.TranslationLanguages, func(l api.Language) bool { return l.Code == code }) {
			return code
		}
		break
	}
	return "en"
}

// toggleTranslation translates a commit, or switches an existing translation on and off.
func (m *AppModel) toggleTranslation(c api.Commit) tea.Cmd {
	if c.DeletedAt != nil || strings.TrimSpace(c.Body) == "" || m.commitCalls.translate == nil {
		return nil
	}
	if t, ok := m.translations[c.ID]; ok {
		if t.pending {
			return nil
		}
		if t.segments != nil {
			t.shown = !t.shown
			m.translations[c.ID] = t
			m.reflow()
			return nil
		}
	}
	if m.translations == nil {
		m.translations = make(map[string]translation)
	}
	m.translations[c.ID] = translation{pending: true}
	m.reflow()
	return m.requestTranslation(c.ID, 1, 0)
}

func (m *AppModel) requestTranslation(id string, attempt int, wait time.Duration) tea.Cmd {
	var known *api.AccountSettings
	if m.settings.regionLoad.loaded {
		known = &m.settings.region
	}
	session, translate, settings, life := m.session, m.commitCalls.translate, m.commitCalls.settings, m.account.life
	return life.command(func() tea.Msg {
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-life.ctx.Done():
				return nil
			}
		}
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		region := known
		if region == nil && settings != nil {
			if loaded, err := settings(ctx); err == nil {
				region = &loaded
			}
		}
		language := "en"
		if region != nil {
			language = translationLanguage(*region)
		}
		t, err := translate(ctx, id, language)
		return translationResult{session: session, attempt: attempt, id: id, translation: t, err: err}
	})
}

func (m *AppModel) acceptTranslation(msg translationResult) tea.Cmd {
	t, ok := m.translations[msg.id]
	if msg.session != m.session || !ok || !t.pending {
		return nil
	}
	if msg.err == nil && msg.translation.Available && msg.translation.Translated == nil && msg.attempt < translationAttempts {
		return m.requestTranslation(msg.id, msg.attempt+1, 3*time.Second)
	}
	delete(m.translations, msg.id)
	defer m.reflow()
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		var failure *auth.APIError
		switch {
		case errors.Is(msg.err, api.ErrNoTranslation), errors.As(msg.err, &failure) && failure.Status == http.StatusNotFound:
			return m.notify("info", "No translation", "This commit can't be translated.")
		case failure != nil && failure.Status == http.StatusTooManyRequests:
			return m.notify("warning", "Slow down", retryAfterText(failure.RetryAfter, "You can translate 10 commits a minute."))
		case failure != nil && failure.Message != "":
			return m.notify("error", "Translation failed", safeText(failure.Message))
		}
		return m.notify("error", "Translation failed", "Could not translate this commit. Try again.")
	}
	tr := msg.translation
	if !tr.Available {
		return m.notify("info", "Already in your language", "This commit is already in your translation language.")
	}
	if tr.Translated == nil {
		return m.notify("warning", "Translation pending", "The server is still translating. Press t again in a moment.")
	}
	source := tr.SourceName
	if source == "" {
		source = tr.SourceLanguage
	}
	m.translations[msg.id] = translation{source: source, segments: tr.Translated.Segments, preview: tr.Translated.Preview, shown: true}
	return nil
}

// translatedBody swaps in a commit's shown translation, with the notice line drawn above its body.
func translatedBody(c api.Commit, style statsStyle) (api.Commit, string) {
	t, ok := style.translations[c.ID]
	if !ok {
		return c, ""
	}
	if t.pending {
		return c, "Translating…"
	}
	if !t.shown {
		return c, ""
	}
	c.Segments, c.Preview = t.segments, t.preview
	notice := "Translated"
	if t.source != "" {
		notice += " from " + safeText(t.source)
	}
	return c, notice + " · t shows the original"
}

type relatedResult struct {
	session, generation int
	id                  string
	commits             []api.Commit
	err                 error
}

func (m *AppModel) loadRelated(t *threadState) tea.Cmd {
	if m.commitCalls.related == nil || t.root.DeletedAt != nil {
		return nil
	}
	session, generation, id, load, ctx := m.session, t.generation, t.root.ID, m.commitCalls.related, t.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		commits, err := load(ctx, id)
		return relatedResult{session: session, generation: generation, id: id, commits: commits, err: err}
	})
}

func (m *AppModel) acceptRelated(msg relatedResult) tea.Cmd {
	if msg.session != m.session || msg.err != nil {
		return nil
	}
	for i := range m.threads {
		t := &m.threads[i]
		if t.generation != msg.generation || t.root.ID != msg.id {
			continue
		}
		t.related = append([]api.Commit{}, m.safeCommits(msg.commits)...)
		m.renderThread(t)
		return m.syncImages()
	}
	return nil
}
