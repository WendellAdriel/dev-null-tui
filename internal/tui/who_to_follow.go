package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// Who to follow follows the web's resources/js/components/who-to-follow.tsx: a modal opened from the aside's Actions or
// w, and in windows without an aside, a block after the fifth commit in the Following and Latest logs.

type suggestCalls struct {
	list    func(context.Context, api.SuggestionQuery) ([]api.SuggestedUser, error)
	dismiss func(context.Context, string) error
}

type whoToFollow struct {
	// people holds the shown suggestions first, then the spares that take a dismissed person's place.
	people []api.SuggestedUser
	round  int
	// key names the screen the list was asked for: a profile's handle, a thread's commit, or neither.
	key        string
	asked      bool
	loaded     bool
	dismissed  int
	generation int
	// failed marks a list request that failed, so the modal stops showing Loading.
	failed bool
	// open shows the Who to follow modal, and focus is its selected person, counted from one.
	open  bool
	focus int
}

const (
	shownSuggestions = 3
	whoToFollowAfter = 5
	// suggestionDelay waits for the screen to settle, so moving through threads asks once.
	suggestionDelay = 400 * time.Millisecond
)

type suggestionTick struct{ session, generation int }

type suggestionResult struct {
	session, generation int
	topUp               bool
	people              []api.SuggestedUser
	err                 error
}

type suggestionDismissed struct {
	session int
	person  api.SuggestedUser
	at      int
	err     error
}

func (m AppModel) suggestionsHidden() bool { return m.tuiSettings.HideWhoToFollow }

// suggestionContext is what the web passes beside a profile or a commit.
func (m AppModel) suggestionContext() (api.SuggestionQuery, string) {
	if m.threadOnTop() {
		id := m.threads[len(m.threads)-1].root.ID
		return api.SuggestionQuery{CommitID: id}, "commit:" + id
	}
	if p := m.topProfile(); p != nil && m.profileOnTop() && !p.people && !p.organization {
		return api.SuggestionQuery{Handle: p.handle}, "user:" + strings.ToLower(p.handle)
	}
	return api.SuggestionQuery{}, ""
}

// syncSuggestions asks again, after a pause, when the screen the list belongs to changes.
func (m *AppModel) syncSuggestions() tea.Cmd {
	w := &m.whoToFollow
	if !m.connected || m.accountOpen || m.suggestionsHidden() || m.suggestCalls.list == nil {
		return nil
	}
	// Ask only where the list can be seen: the modal, or the Following and Latest logs on top without an aside.
	_, _, aside := m.columns()
	inLog := aside == 0 && m.active < 2 && len(m.threads) == 0 && len(m.profiles) == 0 && !m.notificationsOpen && !m.starredOpen && !m.remotes.open && !m.drafts.open
	if !w.open && !inLog {
		return nil
	}
	_, key := m.suggestionContext()
	if w.asked && key == w.key {
		return nil
	}
	w.key, w.asked = key, true
	w.generation++
	session, generation := m.session, w.generation
	return tea.Tick(suggestionDelay, func(time.Time) tea.Msg { return suggestionTick{session, generation} })
}

func (m *AppModel) acceptSuggestionTick(msg suggestionTick) tea.Cmd {
	if msg.session != m.session || msg.generation != m.whoToFollow.generation {
		return nil
	}
	return m.fetchSuggestions(false)
}

func (m *AppModel) fetchSuggestions(topUp bool) tea.Cmd {
	if m.suggestCalls.list == nil {
		return nil
	}
	w := &m.whoToFollow
	if !topUp {
		w.generation++
	}
	query, _ := m.suggestionContext()
	query.Round = w.round
	session, generation, list, life := m.session, w.generation, m.suggestCalls.list, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		people, err := list(ctx, query)
		return suggestionResult{session: session, generation: generation, topUp: topUp, people: people, err: err}
	})
}

func (m *AppModel) acceptSuggestions(msg suggestionResult) {
	w := &m.whoToFollow
	if msg.session != m.session || msg.generation != w.generation {
		return
	}
	if msg.err != nil {
		// The list keeps what it showed. Older servers without the route simply show no suggestions.
		w.failed = !m.revoked(msg.err)
		return
	}
	w.loaded, w.failed = true, false
	if !msg.topUp {
		w.people = msg.people
	} else {
		for _, person := range msg.people {
			if !slices.ContainsFunc(w.people, func(p api.SuggestedUser) bool { return p.ID == person.ID }) {
				w.people = append(w.people, person)
			}
		}
	}
	m.reflow()
}

func (m AppModel) shownPeople() []api.SuggestedUser {
	people := m.whoToFollow.people
	return people[:min(shownSuggestions, len(people))]
}

// suggestionsEmptied is the web's note after the last suggestion was followed or dismissed.
func (m AppModel) suggestionsEmptied() bool {
	w := m.whoToFollow
	return w.loaded && len(w.people) == 0 && w.dismissed > 0
}

func (m AppModel) suggestionsShown() bool {
	return m.connected && !m.suggestionsHidden() && (len(m.whoToFollow.people) > 0 || m.suggestionsEmptied())
}

// suggestionsInLog places the block in the Following and Latest logs when the aside has no room for it.
func (m AppModel) suggestionsInLog(index int) bool {
	_, _, aside := m.columns()
	return aside == 0 && index < 2 && m.suggestionsShown() && len(m.feeds[index].commits) > whoToFollowAfter
}

func suggestionReason(u api.SuggestedUser) string {
	switch {
	case u.FollowedByFollowing > 0:
		return fmt.Sprintf("Followed by %d you follow", u.FollowedByFollowing)
	case u.Followers != nil && *u.Followers == 1:
		return "1 follower"
	case u.Followers != nil && *u.Followers > 0:
		return fmt.Sprintf("%d followers", *u.Followers)
	}
	return ""
}

func suggestedFollowing(u api.SuggestedUser) bool { return u.IsFollowed != nil && *u.IsFollowed }

func (m *AppModel) followSuggestion(i int) tea.Cmd {
	people := m.shownPeople()
	if i < 0 || i >= len(people) {
		return nil
	}
	following := suggestedFollowing(people[i])
	return m.startFollow(people[i].Handle, &following)
}

// dismissSuggestion takes the person out at once, so a spare fills the place, and puts them back if it fails.
func (m *AppModel) dismissSuggestion(i int) tea.Cmd {
	w := &m.whoToFollow
	if i < 0 || i >= min(shownSuggestions, len(w.people)) || m.suggestCalls.dismiss == nil {
		return nil
	}
	person := w.people[i]
	w.people = slices.Delete(slices.Clone(w.people), i, i+1)
	w.dismissed++
	m.reflow()
	session, dismiss, life := m.session, m.suggestCalls.dismiss, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		return suggestionDismissed{session: session, person: person, at: i, err: dismiss(ctx, person.Handle)}
	})
}

func (m *AppModel) acceptSuggestionDismissed(msg suggestionDismissed) tea.Cmd {
	w := &m.whoToFollow
	if msg.session != m.session {
		return nil
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		w.people = slices.Insert(slices.Clone(w.people), min(msg.at, len(w.people)), msg.person)
		w.dismissed--
		m.reflow()
		return m.notify("warning", "Not dismissed", "@"+safeText(msg.person.Handle)+" may be suggested again. Try again.")
	}
	if len(w.people) <= shownSuggestions {
		return m.fetchSuggestions(true)
	}
	return nil
}

func (m *AppModel) nextSuggestionRound() tea.Cmd {
	m.whoToFollow.round++
	return m.fetchSuggestions(false)
}

func (m *AppModel) reconcileSuggestionFollow(handle string, follow bool) {
	for i := range m.whoToFollow.people {
		if person := &m.whoToFollow.people[i]; strings.EqualFold(person.Handle, handle) {
			person.IsFollowed = &follow
		}
	}
}

func (m *AppModel) toggleWhoToFollow() tea.Cmd {
	m.tuiSettings.HideWhoToFollow = !m.tuiSettings.HideWhoToFollow
	m.whoToFollow.asked = false
	for i := range m.feeds {
		m.feeds[i].person = 0
	}
	m.sizeSettings()
	m.reflow()
	return tea.Batch(m.saveTUISettings(false), m.syncSuggestions())
}

// moveLog steps the log's cursor, through the suggested people when the block sits after the fifth commit.
func (m AppModel) moveLog(delta int) (AppModel, tea.Cmd) {
	f := &m.feeds[m.active]
	people := len(m.shownPeople())
	if !m.suggestionsInLog(m.active) {
		people = 0
	}
	above := whoToFollowAfter - 1
	settle := func() (AppModel, tea.Cmd) {
		m.renderFeed(m.active)
		m.revealPerson()
		return m, nil
	}
	switch {
	case people == 0:
	case delta > 0 && f.person == 0 && f.selected == above:
		f.person = 1
		return settle()
	case delta > 0 && f.person > 0 && f.person < people:
		f.person++
		return settle()
	case delta > 0 && f.person == people:
		return m.selectCommit(above + 1)
	case delta < 0 && f.person > 1:
		f.person--
		return settle()
	case delta < 0 && f.person == 1:
		return m.selectCommit(above)
	case delta < 0 && f.person == 0 && f.selected == above+1:
		f.person, f.selected = people, above
		return settle()
	}
	return m.selectCommit(f.selected + delta)
}

func (m *AppModel) revealPerson() {
	f := &m.feeds[m.active]
	if f.person == 0 || f.person > len(f.personStarts) {
		return
	}
	start, end := f.personStarts[f.person-1], f.personEnds[f.person-1]
	if start < f.viewport.YOffset() {
		f.viewport.SetYOffset(start)
	} else if end > f.viewport.YOffset()+f.viewport.Height() {
		f.viewport.SetYOffset(end - f.viewport.Height())
	}
}

// suggestionKey runs a suggested person's keys while the log's cursor is on them, and keeps commit keys off the
// commit above.
func (m AppModel) suggestionKey(pressed string) (AppModel, tea.Cmd, bool) {
	i := m.feeds[m.active].person - 1
	switch pressed {
	case "enter", "u":
		people := m.shownPeople()
		if i < len(people) {
			return m, m.openProfile(people[i].Handle), true
		}
	case "f":
		return m, m.followSuggestion(i), true
	case "X":
		cmd := m.dismissSuggestion(i)
		f := &m.feeds[m.active]
		f.person = min(f.person, len(m.shownPeople()))
		if f.person == 0 {
			f.selected = whoToFollowAfter - 1
		}
		m.renderFeed(m.active)
		return m, cmd, true
	case "R":
		return m, m.nextSuggestionRound(), true
	case "r", "q", "v", "D", "s", "p", "y", "Q", "T", "S", "P", "t", "x", ".", "o", "m", "space":
		return m, nil, true
	}
	return m, nil, false
}

// suggestionAction runs a click on the log block or the modal.
func (m AppModel) suggestionAction(action string, index int) (AppModel, tea.Cmd) {
	people := m.shownPeople()
	switch action {
	case "wtf-round":
		return m, m.nextSuggestionRound()
	case "wtf-hide":
		m.whoToFollow.open = false
		return m, m.toggleWhoToFollow()
	case "wtf-close":
		m.whoToFollow.open = false
		return m, m.syncImages()
	case "wtf-select":
		f := &m.feeds[m.active]
		if f.person == index+1 {
			return m.suggestionAction("wtf-open", index)
		}
		f.person, f.selected = index+1, whoToFollowAfter-1
		m.renderFeed(m.active)
		return m, nil
	}
	if index >= len(people) {
		return m, nil
	}
	switch action {
	case "wtf-open":
		return m, m.openProfile(people[index].Handle)
	case "wtf-follow":
		return m, m.followSuggestion(index)
	case "wtf-dismiss":
		return m, m.dismissSuggestion(index)
	case "wtf-avatar":
		// The modal routes keys before the viewer, so it closes first.
		m.whoToFollow.open = false
		person := people[index].User
		return m, tea.Batch(m.openAvatar("@"+safeText(person.Handle), person.AvatarURL), m.syncImages())
	}
	return m, nil
}

// suggestionLogBlock draws the block that sits after the fifth commit, with click targets relative to its first line.
func (m AppModel) suggestionLogBlock(width, cursor int, p palette) (lines []string, areas []hitArea, starts, ends []int) {
	inner := max(1, width-5)
	round, hide := actionChip("Suggest others", "R", p), chip("⋯ Hide", p.muted, p)
	controls := round + "  " + hide
	title := text(p.muted).Render("$ who --should-i-follow")
	lines = append(lines, "", "     "+split(title, controls, inner))
	right := 5 + inner - lipgloss.Width(controls)
	areas = append(areas, hitArea{uiRect{right, 1, lipgloss.Width(round), 1}, "wtf-round", 0},
		hitArea{uiRect{5 + inner - lipgloss.Width(hide), 1, lipgloss.Width(hide), 1}, "wtf-hide", 0})
	if m.suggestionsEmptied() {
		lines = append(lines, "     "+text(p.muted).Render(fit("fatal: no one left to suggest", inner)), "")
		return lines, areas, nil, nil
	}
	for i, person := range m.shownPeople() {
		lines = append(lines, "")
		start := len(lines)
		chosen := cursor == i+1
		name := text(p.ink).Bold(true).Render(safeText(person.Name)) + " " + text(p.muted).Render("@"+safeText(person.Handle))
		dot := text(laneColor(person.Lane, p)).Render("●")
		row := []string{"     " + fit(dot+" "+name, inner)}
		if reason := suggestionReason(person); reason != "" {
			row = append(row, "       "+text(p.muted).Render(fit(reason, inner-2)))
		} else if bio := strings.TrimSpace(strings.ReplaceAll(person.Bio, "\n", " ")); bio != "" {
			row = append(row, "       "+text(p.muted).Render(fit("# "+safeText(bio), inner-2)))
		}
		areas = append(areas, hitArea{uiRect{0, start, width, len(row)}, "wtf-select", i})
		if chosen {
			label := "Follow"
			if suggestedFollowing(person) {
				label = "Unfollow"
			}
			chips, chipAreas := actionChips([]commitAction{{"Profile", "enter"}, {label, "f"}, {"Not interested", "X"}}, inner-2, p)
			actions := map[string]string{"enter": "wtf-open", "f": "wtf-follow", "X": "wtf-dismiss"}
			for _, area := range chipAreas {
				area.x += 7
				area.y += start + len(row)
				area.action, area.index = actions[area.action], i
				areas = append(areas, area)
			}
			for _, chip := range chips {
				row = append(row, "       "+chip)
			}
		}
		for _, line := range row {
			if chosen {
				line = text(p.blue).Render("▌") + ansi.Cut(line, 1, width)
				line = withBackground(lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line), p.wash)
			}
			lines = append(lines, line)
		}
		starts, ends = append(starts, start), append(ends, len(lines))
	}
	lines = append(lines, "", text(p.hairline).Render(strings.Repeat("─", width)))
	return lines, areas, starts, ends
}

// modalCompactest is the most compact Who to follow modal: 1 drops bios, 2 the blank rows, 3 and 4 show two people,
// then one.
const modalCompactest = 4

// modalPeople is how many suggested people the modal shows at a compactness.
func (m AppModel) modalPeople(compact int) []api.SuggestedUser {
	people := m.shownPeople()
	if compact >= 3 {
		people = people[:min(len(people), shownSuggestions-(compact-2))]
	}
	return people
}

// openWhoToFollow opens the modal from the aside's Actions or w, asking for suggestions for the screen underneath.
func (m *AppModel) openWhoToFollow() tea.Cmd {
	w := &m.whoToFollow
	if !m.connected || m.suggestionsHidden() || m.suggestCalls.list == nil {
		return nil
	}
	w.open, w.focus, w.failed = true, 1, false
	_, key := m.suggestionContext()
	if w.asked && w.loaded && key == w.key {
		return m.syncImages()
	}
	w.key, w.asked = key, true
	return tea.Batch(m.fetchSuggestions(false), m.syncImages())
}

// suggestionModal draws the web's Who to follow card in a modal: avatar, name, lane dot and handle, bio as a comment,
// and the reason, with follow and dismiss icons. It gets more compact until it fits the terminal.
func (m AppModel) suggestionModal(p palette) (string, uiRect, []hitArea) {
	width := modalWidth(m.account.width)
	body, areas := m.suggestionModalBody(max(1, width-4), m.modalCompact(), p)
	footer, footerAreas := hintRow([]statusPart{hintChip("Choose", "↑/↓", "down"), statusChip("Profile", "enter"), closeChip("Close")}, max(1, width-4), p)
	r := centeredRect(max(1, m.account.width), max(1, m.account.height), width, lipgloss.Height(body)+6)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + 3
	}
	areas = append(areas, panelFooterAreas(footerAreas, footer, r)...)
	areas = append(areas, hitArea{uiRect{r.x + r.w - 3, r.y + 1, 1, 1}, "wtf-close", 0})
	return panel("/dev/null · WHO TO FOLLOW", body, footer, r, p, p.blue), r, areas
}

func (m AppModel) suggestionModalBody(inner, compact int, p palette) (string, []hitArea) {
	round := actionChip("Suggest others", "R", p)
	title := text(p.muted).Render(fit("Suggested people", max(1, inner-lipgloss.Width(round)-1)))
	w := m.whoToFollow
	switch {
	case m.suggestionsEmptied():
		return strings.Join([]string{title, "", text(p.muted).Render(fit("fatal: no one left to suggest", inner))}, "\n"), nil
	case w.failed && !w.loaded:
		return strings.Join([]string{title, "", text(p.muted).Render(fit("Couldn't load suggestions. Close and press w to try again.", inner))}, "\n"), nil
	case !w.loaded:
		return strings.Join([]string{title, "", text(p.muted).Render("Loading…")}, "\n"), nil
	case len(m.shownPeople()) == 0:
		return strings.Join([]string{title, "", text(p.muted).Render("No suggestions right now.")}, "\n"), nil
	}
	lines := []string{split(title, round, inner)}
	areas := []hitArea{{uiRect{inner - lipgloss.Width(round), 0, lipgloss.Width(round), 1}, "wtf-round", 0}}
	if compact < 2 {
		lines = append(lines, "")
	}
	for i, person := range m.modalPeople(compact) {
		row, rowAreas := m.suggestionModalRow(person, i, w.focus == i+1, inner, compact, p)
		for _, area := range rowAreas {
			area.y += len(lines)
			areas = append(areas, area)
		}
		lines = append(lines, row...)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n"), areas
}
func (m AppModel) suggestionModalRow(person api.SuggestedUser, i int, chosen bool, inner, compact int, p palette) ([]string, []hitArea) {
	follow := m.icons.follow
	if suggestedFollowing(person) {
		follow = "✓"
	}
	followChip, dismiss := chip(follow, p.blue, p), chip("×", p.muted, p)
	buttons := followChip + " " + dismiss
	indent := 0
	var avatar []string
	if m.images.Enabled {
		_, avatar = m.userAvatar(&person.User, 4, 2, p)
		indent = 5
	}
	width := max(1, inner-indent)
	name := text(p.ink).Bold(true).Render(fit(safeText(person.Name), max(1, width-lipgloss.Width(buttons)-1)))
	handle := text(laneColor(person.Lane, p)).Render("●") + " " + text(p.muted).Render(fit("@"+safeText(person.Handle), max(1, width-2)))
	body := []string{split(name, buttons, width), handle}
	if bio := strings.TrimSpace(strings.Join(strings.Fields(person.Bio), " ")); bio != "" && compact < 1 {
		body = append(body, text(p.hairline).Render("# ")+text(p.muted).Render(fit(safeText(bio), max(1, width-2))))
	}
	if reason := suggestionReason(person); reason != "" {
		body = append(body, text(p.muted).Render(fit(reason, width)))
	}
	areas := []hitArea{
		{uiRect{0, 1, inner, len(body) - 1}, "wtf-open", i},
		{uiRect{indent, 0, inner - indent - lipgloss.Width(buttons), 1}, "wtf-open", i},
		{uiRect{inner - lipgloss.Width(buttons), 0, lipgloss.Width(followChip), 1}, "wtf-follow", i},
		{uiRect{inner - lipgloss.Width(dismiss), 0, lipgloss.Width(dismiss), 1}, "wtf-dismiss", i},
	}
	if chosen {
		label := "Follow"
		if suggestedFollowing(person) {
			label = "Unfollow"
		}
		chips, chipAreas := actionChips([]commitAction{{"Profile", "enter"}, {label, "f"}, {"Not interested", "X"}}, width, p)
		actions := map[string]string{"enter": "wtf-open", "f": "wtf-follow", "X": "wtf-dismiss"}
		for _, area := range chipAreas {
			area.x += indent
			area.y += len(body)
			area.action, area.index = actions[area.action], i
			areas = append(areas, area)
		}
		body = append(body, chips...)
	}
	if indent > 0 {
		areas = append(areas, hitArea{uiRect{0, 0, 4, 2}, "wtf-avatar", i})
	}
	lines := make([]string, 0, len(body)+1)
	for n, line := range body {
		left := strings.Repeat(" ", indent)
		if n < len(avatar) {
			left = avatar[n] + " "
		}
		line = left + line
		if chosen {
			line = withBackground(lipgloss.NewStyle().Width(inner).MaxWidth(inner).Render(line), p.wash)
		}
		lines = append(lines, line)
	}
	if compact < 2 {
		lines = append(lines, "")
	}
	return lines, areas
}

// suggestionModalKey runs the modal's keys. It owns the keyboard while open.
func (m AppModel) suggestionModalKey(pressed string) (AppModel, tea.Cmd) {
	w := &m.whoToFollow
	people := m.modalPeople(m.modalCompact())
	if len(people) > 0 {
		w.focus = max(1, min(w.focus, len(people)))
	}
	i := w.focus - 1
	switch pressed {
	case "esc", "w", "q":
		w.open = false
		return m, m.syncImages()
	case "R":
		w.focus = 1
		return m, m.nextSuggestionRound()
	}
	if len(people) == 0 {
		return m, nil
	}
	switch pressed {
	case "j", "down":
		w.focus = min(len(people), w.focus+1)
	case "k", "up":
		w.focus = max(1, w.focus-1)
	case "enter", "u":
		w.open = false
		return m, m.openProfile(people[i].Handle)
	case "f":
		return m, m.followSuggestion(i)
	case "X":
		cmd := m.dismissSuggestion(i)
		m.whoToFollow.focus = max(1, min(m.whoToFollow.focus, len(m.shownPeople())))
		return m, cmd
	}
	return m, nil
}

func modalWidth(terminal int) int { return min(64, terminal-4) }

// modalCompact is the compactness the modal draws at for the terminal's height.
func (m AppModel) modalCompact() int {
	width := modalWidth(m.account.width)
	for compact := 0; compact < modalCompactest; compact++ {
		if body, _ := m.suggestionModalBody(max(1, width-4), compact, m.account.colors()); lipgloss.Height(body)+6 <= m.account.height-4 {
			return compact
		}
	}
	return modalCompactest
}

func (m AppModel) suggestionModalMouse(x, y int) (AppModel, tea.Cmd) {
	_, r, areas := m.suggestionModal(m.account.colors())
	for i := len(areas) - 1; i >= 0; i-- {
		if !areas[i].contains(x, y) {
			continue
		}
		if pressed, ok := keyAction(areas[i].action); ok {
			return m.mouseKey(pressed)
		}
		return m.suggestionAction(areas[i].action, areas[i].index)
	}
	if !r.contains(x, y) {
		m.whoToFollow.open = false
		return m, m.syncImages()
	}
	return m, nil
}

// suggestionModalMedia asks for the modal's avatars while it is open.
func (m AppModel) suggestionModalMedia(p palette) []graphics.Request {
	if !m.whoToFollow.open {
		return nil
	}
	var requests []graphics.Request
	for _, person := range m.shownPeople() {
		req, _ := m.userAvatar(&person.User, 4, 2, p)
		requests = append(requests, req)
	}
	return requests
}
