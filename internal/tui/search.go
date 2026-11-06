package tui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// The web's search-dialog.tsx timings and sizes. Commits wait longer, because each search pays for an embedding.
const (
	peopleSearchDebounce = 120 * time.Millisecond
	commitSearchDebounce = 350 * time.Millisecond
	commitPreviewLimit   = 5
	commitResultsLimit   = 20
	recentSearchLimit    = 3
	topicPeopleLimit     = 2
)

const (
	peopleLane = iota
	commitLane
)

// searchCalls are the search routes, which tests replace.
type searchCalls struct {
	profiles func(context.Context, string) (api.Profiles, error)
	commits  func(context.Context, string, int) ([]api.Commit, error)
}

func newSearchCalls(client *api.Client) searchCalls {
	return searchCalls{client.SearchProfiles, client.SearchCommits}
}

// searchLane is one of the dialog's two searches. found keeps the term its results belong to, so stale results never
// show under a newer term.
type searchLane struct {
	pending    string
	found      searchResult
	loading    bool
	generation int
	cancel     context.CancelFunc
}

type searchDialog struct {
	field textinput.Model
	index int
	lanes [2]searchLane
}

type searchTick struct{ session, lane, generation int }

type searchResult struct {
	session, lane, generation int
	term                      string
	profiles                  api.Profiles
	commits                   []api.Commit
	throttled, done           bool
	err                       error
}

type searchOption struct {
	kind, action, term string
	user               api.User
	organization       api.Organization
	commit             api.Commit
}

var (
	handleCharacters = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	dictionaryWord   = regexp.MustCompile(`^[a-z]{6,}$`)
	capitalizedWord  = regexp.MustCompile(`^\p{Lu}[\p{L}'-]*$`)
)

// searchIntent ports the web's search-intent.ts: whether a term reads like a person, an idea, or either.
func searchIntent(term string) string {
	term = strings.TrimSpace(term)
	words := strings.Fields(term)
	switch {
	case strings.HasPrefix(term, "@"):
		return "developer"
	case len(words) >= 2 && len(words) <= 3 && allCapitalized(words):
		return "either"
	case len(words) > 1:
		return "topic"
	case handleCharacters.MatchString(term) && len(term) <= 12 && !dictionaryWord.MatchString(term):
		return "developer"
	}
	return "either"
}

func allCapitalized(words []string) bool {
	for _, word := range words {
		if !capitalizedWord.MatchString(word) {
			return false
		}
	}
	return true
}

var actionKeywords = map[string][]string{
	"commit":     {"commit", "write a commit", "new commit", "compose"},
	"feature":    {"feature", "request a feature", "feature request"},
	"bug":        {"bug", "report a bug", "bug report"},
	"go-live":    {"go live", "live", "stream", "start stream"},
	"end-stream": {"end stream", "stop stream", "end live", "stop live"},
}

// searchActions ports the web's search-actions.ts. Ending a stream only answers to its own words.
func searchActions(term string, live bool) []string {
	typed := strings.ToLower(strings.Join(strings.Fields(term), " "))
	if strings.HasPrefix(typed, "/") {
		var actions []string
		for _, kind := range []string{"feature", "bug"} {
			if strings.HasPrefix("/"+kind, typed) {
				actions = append(actions, kind)
			}
		}
		return actions
	}
	if utf8.RuneCountInString(typed) < 3 {
		return nil
	}
	stream := "go-live"
	if live {
		stream = "end-stream"
	}
	var actions []string
	for _, action := range []string{"commit", "feature", "bug", stream} {
		for _, keyword := range actionKeywords[action] {
			if strings.HasPrefix(keyword, typed) {
				actions = append(actions, action)
				break
			}
		}
	}
	return actions
}

// searchBlocked keeps Ctrl+K and / away from dialogs and text inputs that own the keyboard.
func (m AppModel) searchBlocked() bool {
	r := m.remotes
	return !m.connected || m.accountOpen || m.search != nil || m.searchCalls.profiles == nil || m.manualLink != "" ||
		m.whatsNew.open || m.upgradeVisible() || m.viewer.open || m.vote.commitID != "" || m.help ||
		m.composer.open || m.composer.confirm != "" || m.live != nil || m.deleting.id != "" ||
		m.settings.dialog != "" || m.settings.busy || m.activeReactions() != nil ||
		(m.remotesOnTop() && (r.dialog != "" || m.remotesOverlay()))
}

func (m *AppModel) openSearch(term string) tea.Cmd {
	if m.searchBlocked() {
		return nil
	}
	field := textinput.New()
	field.Placeholder, field.CharLimit, field.Prompt = "Search developers or commits", api.MaxSearchTerm, ""
	field.SetValue(term)
	m.search = &searchDialog{field: field}
	m.resetSequence()
	return tea.Batch(m.search.field.Focus(), m.searchChanged())
}

func (m *AppModel) closeSearch() {
	if m.search == nil {
		return
	}
	for i := range m.search.lanes {
		if cancel := m.search.lanes[i].cancel; cancel != nil {
			cancel()
		}
	}
	m.search = nil
}

func (d *searchDialog) term() string { return strings.TrimSpace(d.field.Value()) }

func (d *searchDialog) peopleTerm() string { return strings.TrimPrefix(d.term(), "@") }

// commitTerm is empty when the term reads like a handle, or is shorter than the server's three characters.
func (d *searchDialog) commitTerm() string {
	term := d.term()
	if searchIntent(term) == "developer" || utf8.RuneCountInString(term) < api.MinCommitSearch {
		return ""
	}
	return term
}

// searchChanged restarts whichever searches the new term changes, each after its debounce.
func (m *AppModel) searchChanged() tea.Cmd {
	d := m.search
	var cmds []tea.Cmd
	for lane, term := range [2]string{d.peopleTerm(), d.commitTerm()} {
		l := &d.lanes[lane]
		if term == l.pending {
			continue
		}
		if l.cancel != nil {
			l.cancel()
			l.cancel = nil
		}
		// The counter outlives the dialog, so a tick from a closed one never matches a reopened one.
		m.searchGeneration++
		l.generation = m.searchGeneration
		l.pending, l.loading = term, term != ""
		if term == "" {
			continue
		}
		delay := peopleSearchDebounce
		if lane == commitLane {
			delay = commitSearchDebounce
		}
		session, generation := m.session, l.generation
		cmds = append(cmds, tea.Tick(delay, func(time.Time) tea.Msg { return searchTick{session, lane, generation} }))
	}
	return tea.Batch(cmds...)
}

func (m *AppModel) acceptSearchTick(msg searchTick) tea.Cmd {
	if m.search == nil || msg.session != m.session || msg.generation != m.search.lanes[msg.lane].generation {
		return nil
	}
	l := &m.search.lanes[msg.lane]
	if l.cancel != nil {
		l.cancel()
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	l.cancel = cancel
	result := searchResult{session: m.session, lane: msg.lane, generation: msg.generation, term: l.pending}
	profiles, commits := m.searchCalls.profiles, m.searchCalls.commits
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		if result.lane == peopleLane {
			result.profiles, result.err = profiles(ctx, result.term)
		} else {
			result.commits, result.err = commits(ctx, result.term, commitPreviewLimit)
		}
		return result
	})
}

func (m *AppModel) acceptSearchResult(msg searchResult) tea.Cmd {
	if m.search == nil || msg.session != m.session || msg.generation != m.search.lanes[msg.lane].generation {
		return nil
	}
	l := &m.search.lanes[msg.lane]
	l.loading, l.cancel = false, nil
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		// A failed search counts as an empty one, so the dialog never waits on it forever.
		var failure *auth.APIError
		msg.throttled = errors.As(msg.err, &failure) && failure.Status == http.StatusTooManyRequests
		msg.profiles, msg.commits = api.Profiles{}, nil
	}
	msg.profiles.Users = m.safeSearchUsers(msg.profiles.Users)
	msg.commits = m.safeCommits(msg.commits)
	msg.done = true
	l.found = msg
	return m.rememberSettledSearch()
}

// safeSearchUsers leaves out people you blocked or muted on this device, like the rest of the workspace.
func (m AppModel) safeSearchUsers(users []api.User) []api.User {
	kept := users[:0:0]
	for _, user := range users {
		if m.safetyHidden[strings.ToLower(user.Handle)] == 0 {
			kept = append(kept, user)
		}
	}
	return kept
}

func (l searchLane) settled(term string) bool {
	return term == "" || (l.found.done && l.found.term == term)
}

func (l searchLane) results(term string) searchResult {
	if term == "" || !l.settled(term) {
		return searchResult{}
	}
	return l.found
}

// rememberSettledSearch saves the term once both searches answered it, like the web's rememberSearch.
func (m *AppModel) rememberSettledSearch() tea.Cmd {
	d := m.search
	people, commits := d.peopleTerm(), d.commitTerm()
	term := d.term()
	if people == "" || !d.lanes[peopleLane].settled(people) || !d.lanes[commitLane].settled(commits) {
		return nil
	}
	if d.lanes[peopleLane].results(people).throttled || d.lanes[commitLane].results(commits).throttled {
		return nil
	}
	id := m.account.user.ID
	previous := m.tuiSettings.RecentSearches[id]
	recent := []string{term}
	for _, entry := range previous {
		if !strings.EqualFold(entry, term) && len(recent) < recentSearchLimit {
			recent = append(recent, entry)
		}
	}
	if strings.Join(recent, "\n") == strings.Join(previous, "\n") {
		return nil
	}
	m.setRecentSearches(id, recent)
	return m.saveTUISettings(true)
}

func (m *AppModel) setRecentSearches(id string, recent []string) {
	searches := make(map[string][]string, len(m.tuiSettings.RecentSearches)+1)
	for account, terms := range m.tuiSettings.RecentSearches {
		searches[account] = terms
	}
	if len(recent) == 0 {
		delete(searches, id)
	} else {
		searches[id] = recent
	}
	m.tuiSettings.RecentSearches = searches
	// Other accounts' recents give way before tui.json grows past what loads.
	for account := range searches {
		if data, _ := json.Marshal(m.tuiSettings); len(data) < auth.TUISettingsLimit {
			break
		}
		if account != id {
			delete(searches, account)
		}
	}
}

func (m *AppModel) clearRecentSearches() tea.Cmd {
	if len(m.tuiSettings.RecentSearches[m.account.user.ID]) == 0 {
		return nil
	}
	m.setRecentSearches(m.account.user.ID, nil)
	m.search.index = 0
	return m.saveTUISettings(false)
}

// searchOptions lists what the dialog shows, in the web's order: actions, then people, organizations, and commits, or
// commits first for a topic. An empty box, or one that found nothing, shows the recent searches instead.
func (m AppModel) searchOptions() []searchOption {
	d := m.search
	term := d.term()
	var options []searchOption
	if term != "" {
		options = m.searchResults(term)
	}
	if len(options) > 0 || (term != "" && !m.searchFoundNothing()) {
		return options
	}
	for _, recent := range m.tuiSettings.RecentSearches[m.account.user.ID] {
		options = append(options, searchOption{kind: "recent", term: recent})
	}
	return options
}

func (m AppModel) searchResults(term string) []searchOption {
	d := m.search
	intent := searchIntent(term)
	var actions, people, organizations, commits []searchOption
	for _, action := range searchActions(term, m.account.user.Live.On(m.now)) {
		actions = append(actions, searchOption{kind: "action", action: action})
	}
	found := d.lanes[peopleLane].results(d.peopleTerm()).profiles
	users, orgs := found.Users, found.Organizations
	if intent == "topic" {
		users, orgs = users[:min(len(users), topicPeopleLimit)], orgs[:min(len(orgs), topicPeopleLimit)]
	}
	for _, user := range users {
		people = append(people, searchOption{kind: "developer", user: user})
	}
	for _, organization := range orgs {
		organizations = append(organizations, searchOption{kind: "organization", organization: organization})
	}
	for _, commit := range d.lanes[commitLane].results(d.commitTerm()).commits {
		commits = append(commits, searchOption{kind: "commit", commit: commit})
	}
	if d.commitTerm() != "" {
		commits = append(commits, searchOption{kind: "all", term: term})
	}
	if intent == "topic" {
		return concat(actions, commits, people, organizations)
	}
	return concat(actions, people, organizations, commits)
}

func concat(groups ...[]searchOption) []searchOption {
	var options []searchOption
	for _, group := range groups {
		options = append(options, group...)
	}
	return options
}

func (m AppModel) searchLoading() bool {
	d := m.search
	return d.lanes[peopleLane].loading || d.lanes[commitLane].loading
}

func (m AppModel) searchThrottled() bool {
	d := m.search
	return d.lanes[peopleLane].results(d.peopleTerm()).throttled || d.lanes[commitLane].results(d.commitTerm()).throttled
}

// searchFoundNothing is a finished search with no results, not one still waiting for its answer.
func (m AppModel) searchFoundNothing() bool {
	d := m.search
	people, commits := d.peopleTerm(), d.commitTerm()
	if people == "" || m.searchLoading() || !d.lanes[peopleLane].settled(people) || !d.lanes[commitLane].settled(commits) {
		return false
	}
	return len(m.searchResults(d.term())) == 0
}

func (m *AppModel) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	d := m.search
	options := m.searchOptions()
	switch msg.String() {
	case "esc":
		m.closeSearch()
		return nil
	case "ctrl+k":
		return nil
	case "ctrl+l":
		return m.clearRecentSearches()
	case "down", "up", "ctrl+n", "ctrl+p":
		if len(options) > 0 {
			step := 1
			if msg.String() == "up" || msg.String() == "ctrl+p" {
				step = -1
			}
			d.index = (min(d.index, len(options)-1) + step + len(options)) % len(options)
		}
		return nil
	case "enter":
		if len(options) == 0 {
			return nil
		}
		return m.openSearchOption(options[min(d.index, len(options)-1)])
	}
	return m.updateSearchField(msg)
}

func (m *AppModel) updateSearchField(msg tea.Msg) tea.Cmd {
	d := m.search
	before := d.field.Value()
	var cmd tea.Cmd
	d.field, cmd = d.field.Update(msg)
	if d.field.Value() == before {
		return cmd
	}
	d.index = 0
	return tea.Batch(cmd, m.searchChanged())
}

func (m *AppModel) openSearchOption(option searchOption) tea.Cmd {
	if option.kind == "recent" {
		m.search.field.SetValue(option.term)
		m.search.field.CursorEnd()
		m.search.index = 0
		return m.searchChanged()
	}
	m.closeSearch()
	m.settings.open = false
	switch option.kind {
	case "action":
		switch option.action {
		case "commit":
			return m.composeTo("", api.Commit{})
		case "feature", "bug":
			return m.openLink(strings.TrimRight(m.account.origin, "/") + "/")
		}
		return m.toggleLive()
	case "developer":
		return m.openProfile(option.user.Handle)
	case "organization":
		return m.openOrganization(option.organization.Handle)
	case "commit":
		return m.openThread(option.commit)
	}
	next, cmd := m.openCommitSearch(option.term)
	*m = next
	return cmd
}
