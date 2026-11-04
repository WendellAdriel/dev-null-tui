package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// The people dialogs follow the web's resources/js/components/chat/org.tsx PeopleDialog and the inbox search in
// inbox-panel.tsx: New remote, New org, and Add people.

// peopleDialog is one of the people dialogs. Mode is "remote", "org", or "add", and org is the org people are added to.
type peopleDialog struct {
	mode       string
	org        api.Conversation
	name       string
	query      string
	field      int
	results    []api.Candidate
	index      int
	picks      []api.User
	share      bool
	loading    bool
	sending    bool
	err        string
	generation int
	cancel     context.CancelFunc
}

// The parts of a people dialog that take keys. The list is the search results under the search field.
const (
	peopleName = iota
	peopleSearch
	peopleList
	peopleShare
)

// peopleSearchDelay is the web's wait after typing before it searches (inbox-panel.tsx).
const peopleSearchDelay = 150 * time.Millisecond

// inboxPeople is how many people the inbox search offers to start a conversation with, as on the web.
const inboxPeople = 5

const peopleRows = 8

type peopleSearchTick struct{ session, generation int }

type peopleSearchResult struct {
	session, generation int
	candidates          []api.Candidate
	err                 error
}

type peopleSaveResult struct {
	session int
	dialog  peopleDialog
	id      string
	err     error
}

func (m *AppModel) openPeople(mode string, org api.Conversation) {
	field := peopleSearch
	if mode == "org" {
		field = peopleName
	}
	m.remotes.people = &peopleDialog{mode: mode, org: org, field: field}
}

func (m *AppModel) closePeople() {
	if p := m.remotes.people; p != nil && p.cancel != nil {
		p.cancel()
	}
	m.remotes.people = nil
}

func (p *peopleDialog) fields() []int {
	switch p.mode {
	case "org":
		return []int{peopleName, peopleSearch}
	case "add":
		return []int{peopleSearch, peopleShare}
	}
	return []int{peopleSearch}
}

// room is how many more people can be picked: 99 besides the creator, or what is left of an org's 100 places.
func (p *peopleDialog) room() int {
	if p.mode == "add" {
		return api.MaxOrgMembers - len(p.org.Members) - len(p.picks)
	}
	return api.MaxOrgMembers - 1 - len(p.picks)
}

func (p *peopleDialog) full() bool { return p.mode == "add" && len(p.org.Members) >= api.MaxOrgMembers }

func (p *peopleDialog) picked(handle string) int {
	return slices.IndexFunc(p.picks, func(u api.User) bool { return strings.EqualFold(u.Handle, handle) })
}

// reason says why a person can't be picked, or is empty when they can.
func (p *peopleDialog) reason(c api.Candidate) string {
	if p.full() {
		return fmt.Sprintf("the org is full (%d)", api.MaxOrgMembers)
	}
	return c.Reason
}

// searchTerm is what the web searches for: the query without spaces around it or a leading @.
func searchTerm(query string) string {
	return strings.TrimLeft(strings.TrimSpace(query), "@")
}

func (m *AppModel) peopleKey(pressed string) tea.Cmd {
	p := m.remotes.people
	if p.sending {
		return nil
	}
	fields := p.fields()
	current := max(0, slices.Index(fields, p.field))
	switch pressed {
	case "esc":
		m.closePeople()
		return nil
	case "tab":
		p.field = fields[(current+1)%len(fields)]
		return nil
	case "shift+tab":
		p.field = fields[(current+len(fields)-1)%len(fields)]
		return nil
	case "ctrl+s":
		return m.savePeople()
	case "down":
		switch {
		case p.field == peopleList:
			p.index = min(p.index+1, len(p.results)-1)
		case p.field == peopleSearch && len(p.results) > 0:
			p.field, p.index = peopleList, 0
		}
		return nil
	case "up":
		if p.field == peopleList {
			if p.index == 0 {
				p.field = peopleSearch
			}
			p.index = max(0, p.index-1)
		}
		return nil
	case "enter":
		switch {
		case p.field == peopleList || (p.mode == "remote" && len(p.results) > 0):
			return m.choosePerson()
		case p.field == peopleShare:
			p.share = !p.share
			return nil
		}
		return m.savePeople()
	case "space":
		switch p.field {
		case peopleList:
			return m.choosePerson()
		case peopleShare:
			p.share = !p.share
			return nil
		}
	}
	switch p.field {
	case peopleShare:
		return nil
	case peopleName:
		p.name = typeInto(p.name, pressed, api.MaxOrgName)
		return nil
	}
	query := typeInto(p.query, pressed, api.MaxCandidateTerm)
	if query == p.query {
		return nil
	}
	p.field, p.query = peopleSearch, query
	return m.searchPeople()
}

// typeInto applies a typed key to a one-line field of at most limit characters.
func typeInto(value, pressed string, limit int) string {
	if pressed == "backspace" {
		runes := []rune(value)
		return string(runes[:max(0, len(runes)-1)])
	}
	if pressed == "space" {
		pressed = " "
	}
	if len([]rune(pressed)) != 1 || len([]rune(value)) >= limit {
		return value
	}
	return value + pressed
}

// searchPeople waits for typing to pause, so a search goes out for the query the user stopped at, not every key.
func (m *AppModel) searchPeople() tea.Cmd {
	p := m.remotes.people
	p.generation++
	p.err = ""
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if searchTerm(p.query) == "" {
		p.results, p.loading, p.index = nil, false, 0
		return nil
	}
	p.loading = true
	session, generation := m.session, p.generation
	return tea.Tick(peopleSearchDelay, func(time.Time) tea.Msg { return peopleSearchTick{session, generation} })
}

func (m *AppModel) acceptPeopleTick(msg peopleSearchTick) tea.Cmd {
	p := m.remotes.people
	if p == nil || msg.session != m.session || msg.generation != p.generation {
		return nil
	}
	query := api.CandidateQuery{Term: searchTerm(p.query), For: api.ForRemote}
	switch p.mode {
	case "org":
		query.For = api.ForOrg
	case "add":
		query.For, query.Org = "", p.org.ID
	}
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	p.cancel = cancel
	search, session, generation := m.remoteCalls.candidates, m.session, p.generation
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		candidates, err := search(ctx, query)
		return peopleSearchResult{session: session, generation: generation, candidates: candidates, err: err}
	})
}

func (m *AppModel) acceptPeopleResult(msg peopleSearchResult) tea.Cmd {
	p := m.remotes.people
	if p == nil || msg.session != m.session || msg.generation != p.generation {
		return nil
	}
	p.loading, p.cancel = false, nil
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		p.err = managementError(msg.err, "Could not search. Try again.")
		return nil
	}
	p.results = msg.candidates
	p.index = max(0, min(p.index, len(p.results)-1))
	if len(p.results) == 0 && p.field == peopleList {
		p.field = peopleSearch
	}
	return nil
}

// choosePerson opens the conversation with the person under the cursor, or picks them for an org.
func (m *AppModel) choosePerson() tea.Cmd {
	p := m.remotes.people
	if p.index >= len(p.results) {
		return nil
	}
	candidate := p.results[p.index]
	if i := p.picked(candidate.User.Handle); i >= 0 {
		p.picks = slices.Delete(p.picks, i, i+1)
		return nil
	}
	if p.reason(candidate) != "" {
		return nil
	}
	if p.mode == "remote" {
		m.closePeople()
		return m.openPerson(candidate.User)
	}
	if p.room() > 0 {
		p.picks = append(p.picks, candidate.User)
	}
	return nil
}

// openPerson opens the conversation with someone. Before anyone pushes there is no conversation yet, so the thread
// goes by their handle until the inbox brings the real one.
func (m *AppModel) openPerson(user api.User) tea.Cmd {
	for _, c := range m.remotes.inbox {
		if c.Kind != "org" && c.Partner != nil && strings.EqualFold(c.Partner.Handle, user.Handle) {
			return m.openRemoteThread(c)
		}
	}
	return m.openRemoteThread(api.Conversation{ID: "@" + user.Handle, Kind: "direct", Partner: &user})
}

func (m *AppModel) savePeople() tea.Cmd {
	p := m.remotes.people
	if p.mode == "remote" || len(p.picks) == 0 || p.full() || p.sending {
		return nil
	}
	p.sending, p.err = true, ""
	handles := make([]string, len(p.picks))
	for i, user := range p.picks {
		handles[i] = user.Handle
	}
	dialog, calls, session, life := *p, m.remoteCalls, m.session, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		result := peopleSaveResult{session: session, dialog: dialog}
		if dialog.mode == "add" {
			result.err = calls.addMembers(ctx, dialog.org.ID, handles, dialog.share)
		} else {
			result.id, result.err = calls.createOrg(ctx, dialog.name, handles)
		}
		return result
	})
}

func (m *AppModel) acceptPeopleSave(msg peopleSaveResult) tea.Cmd {
	if !m.connected || msg.session != m.session {
		return nil
	}
	p := m.remotes.people
	if p != nil {
		p.sending = false
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		failure := managementError(msg.err, "Could not save. Try again.")
		if p == nil {
			return m.notify("error", failure, "")
		}
		p.err = failure
		return nil
	}
	m.closePeople()
	contributors := make([]api.Member, len(msg.dialog.picks))
	for i, user := range msg.dialog.picks {
		contributors[i] = api.Member{User: user, Role: string(api.RoleContributor)}
	}
	if msg.dialog.mode == "add" {
		m.updateConversation(msg.dialog.org.ID, func(c *api.Conversation) { c.Members = append(c.Members, contributors...) })
		if t := m.remotes.thread; t != nil && t.conversation.ID == msg.dialog.org.ID {
			t.data.Members = append(t.data.Members, contributors...)
		}
		return tea.Batch(m.notify("success", fmt.Sprintf("Added %d to %s.", len(contributors), m.conversationTitle(msg.dialog.org)), ""), m.fetchRemoteThread("poll"))
	}
	me := api.Member{User: api.User{ID: m.account.user.ID, Handle: m.account.user.Handle, Name: m.account.user.Name}, Role: string(api.RoleMaintainer)}
	org := api.Conversation{ID: msg.id, Kind: "org", Members: append([]api.Member{me}, contributors...)}
	if name := strings.TrimSpace(msg.dialog.name); name != "" {
		org.Name = &name
	}
	return tea.Batch(m.openRemoteThread(org), m.fetchInbox("poll"))
}

// managementError is the server's reason for refusing a change to an org or a pin, or the fallback.
func managementError(err error, fallback string) string {
	var failure *auth.APIError
	if !errors.As(err, &failure) {
		return fallback
	}
	switch {
	case failure.Status == http.StatusTooManyRequests:
		return retryAfterText(failure.RetryAfter, "Rate limited. Try again later.")
	case failure.Status == http.StatusUnprocessableEntity && len(failure.BodyErrors)+len(failure.FieldErrors) > 0:
		return safeText(strings.Join(append(failure.BodyErrors, failure.FieldErrors...), " "))
	case failure.Message != "":
		return safeText(failure.Message)
	}
	return fallback
}

func (m AppModel) peopleModal(pal palette) (string, uiRect, []hitArea) {
	p := m.remotes.people
	title, subtitle := "New remote", ""
	switch p.mode {
	case "org":
		title, subtitle = "New org", "You maintain it. Everyone you add contributes."
	case "add":
		title, subtitle = "Add people", "They join as contributors."
	}
	rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 60, peopleRows+12)
	width := max(1, rect.w-4)
	var lines []string
	var areas []hitArea
	add := func(line, action string) {
		if action != "" {
			areas = append(areas, hitArea{uiRect{rect.x + 2, rect.y + 3 + len(lines), width, 1}, action, 0})
		}
		lines = append(lines, line)
	}
	if subtitle != "" {
		add(text(pal.muted).Render(fit(subtitle, width)), "")
	}
	field := func(value, placeholder string, focused bool) string {
		if value == "" {
			value = text(pal.muted).Render(placeholder)
		} else {
			value = text(pal.ink).Render(safeText(value))
		}
		if focused {
			value += text(pal.blue).Render("▏")
		}
		return fit(value, width)
	}
	if p.mode == "org" {
		add(field(p.name, "Name it (optional)", p.field == peopleName), "name")
	}
	add(text(pal.muted).Render("/ ")+field(p.query, "Find people by name or @handle", p.field == peopleSearch), "search")
	add("", "")
	term := searchTerm(p.query)
	switch {
	case term == "":
		add(text(pal.muted).Render(" Type a name or handle"), "")
	case p.loading && p.results == nil:
		add(text(pal.muted).Render(" Searching…"), "")
	case len(p.results) == 0 && p.err == "":
		add(text(pal.muted).Render(fit(" fatal: nothing matches '"+safeText(term)+"'", width)), "")
	}
	rows := max(1, rect.h-9-len(lines)+3)
	if p.mode == "add" {
		rows -= 2
	}
	start := max(0, min(p.index-rows+1, len(p.results)-rows))
	for i := start; i < min(len(p.results), start+rows); i++ {
		candidate := p.results[i]
		picked := p.picked(candidate.User.Handle) >= 0
		reason := p.reason(candidate)
		marker := " "
		if p.mode != "remote" {
			marker = "○"
			if picked {
				marker = text(pal.blue).Render("●")
			}
		}
		name, handle := safeText(candidate.User.Name), "@"+safeText(candidate.User.Handle)
		row := " " + marker + " " + text(pal.ink).Bold(true).Render(name) + " " + text(pal.muted).Render(handle)
		if reason != "" && !picked {
			// The reason takes the name's place, so it fits the dialog.
			row = " " + marker + " " + text(pal.muted).Render(handle+" · "+safeText(reason))
		}
		row = fit(row, width)
		if p.field == peopleList && i == p.index {
			row = withBackground(lipgloss.NewStyle().Width(width).Render(fit(row, width)), pal.soft)
		}
		areas = append(areas, hitArea{uiRect{rect.x + 2, rect.y + 3 + len(lines), width, 1}, "pick", i})
		lines = append(lines, row)
	}
	if p.mode == "add" {
		box := "[ ]"
		if p.share {
			box = "[x]"
		}
		share := text(pal.ink).Render(box + " Share history")
		if p.field == peopleShare {
			share = withBackground(share, pal.soft)
		}
		lines = append(lines, "")
		add(fit(share+" "+text(pal.muted).Render("They can read everything pushed before they joined."), width), "share")
	}
	if p.err != "" {
		lines = append(lines, "")
		add(text(pal.danger).Render(fit(p.err, width)), "")
	}
	var footer string
	var footerAreas []hitArea
	if p.mode == "remote" {
		footer = text(pal.muted).Render(fit("type to search · ↑/↓ choose · enter open · esc close", width))
	} else {
		verb := "Create org"
		if p.mode == "add" {
			verb = fmt.Sprintf("Add %d", len(p.picks))
			if p.full() {
				verb = fmt.Sprintf("The org is full (%d)", api.MaxOrgMembers)
			}
		}
		if p.sending {
			verb = "Saving…"
		}
		footer, footerAreas = modalFooter([]modalAction{{"Cancel", "esc", "esc"}, {verb, "ctrl+s", "confirm"}}, width, 0, pal)
		footer = text(pal.muted).Render(fit("space pick · tab next field", width)) + "\n" + footer
		for i := range footerAreas {
			footerAreas[i].x += rect.x + 2
			footerAreas[i].y += rect.y + rect.h - lipgloss.Height(footer)
		}
	}
	areas = append(append(areas, footerAreas...), hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
	return panel(title, strings.Join(lines, "\n"), footer, rect, pal, pal.hairline), rect, areas
}

func (m AppModel) peopleMouse(x, y int) (AppModel, tea.Cmd) {
	p := m.remotes.people
	_, rect, areas := m.peopleModal(colors(m.account.dark))
	if p.sending {
		return m, nil
	}
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		switch area.action {
		case "esc":
			m.closePeople()
		case "confirm":
			return m, m.savePeople()
		case "name":
			p.field = peopleName
		case "search":
			p.field = peopleSearch
		case "share":
			p.field, p.share = peopleShare, !p.share
		case "pick":
			p.field, p.index = peopleList, area.index
			return m, m.choosePerson()
		}
		return m, nil
	}
	if !rect.contains(x, y) {
		m.closePeople()
	}
	return m, nil
}

type inboxPeopleResult struct {
	session    int
	term       string
	candidates []api.Candidate
}

// fetchInboxPeople looks up people to start a conversation with when the Open tab is searched, as the web does.
func (m *AppModel) fetchInboxPeople() tea.Cmd {
	r := &m.remotes
	term := searchTerm(r.term)
	if r.tab != api.Open || term == "" || m.remoteCalls.candidates == nil {
		return nil
	}
	search, session, life, searched := m.remoteCalls.candidates, m.session, m.account.life, r.term
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		// Like the web, a failed lookup leaves the inbox results without people.
		candidates, _ := search(ctx, api.CandidateQuery{Term: term, For: api.ForRemote})
		return inboxPeopleResult{session: session, term: searched, candidates: candidates}
	})
}

func (m *AppModel) acceptInboxPeople(msg inboxPeopleResult) {
	r := &m.remotes
	if msg.session != m.session || !r.open || r.tab != api.Open || r.term != msg.term {
		return
	}
	r.candidates = msg.candidates[:min(len(msg.candidates), inboxPeople)]
}
