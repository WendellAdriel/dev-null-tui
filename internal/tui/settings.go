package tui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type settingsState struct {
	open, busy, uncertain                    bool
	tab, selected, generation, focus, choice int
	dialog, handle, targetID, status         string
	input                                    textinput.Model
	viewport                                 viewport.Model
	starts                                   []int
	rows                                     []settingsRow
	lists                                    [2]settingsPeople
	region                                   api.AccountSettings
	devices                                  []api.Device
	regionLoad, devicesLoad                  settingsLoad
	operation                                settingsOperation
	exitDialog                               string
	exitFromSettings                         bool
}

const (
	settingsProfile = iota
	settingsRegionTab
	settingsAppearance
	settingsSafety
	settingsDevices
	settingsTUI
)

var settingsTabs = [...]string{"Profile", "Region", "Appearance", "Safety", "Devices", "TUI"}

type settingsLoad struct {
	loaded, loading bool
	status          string
	generation      int
}

type settingsLoadResult struct {
	session, generation, tab int
	region                   api.AccountSettings
	devices                  []api.Device
	err                      error
}

// signingOut reports a pending write that revokes this client's key, so unrelated 401s must wait for its result.
func (s settingsState) signingOut() bool {
	return s.busy && (s.operation.kind == "delete-account" || (s.operation.kind == "revoke-device" && s.operation.current))
}

var settingsLaneNames = [...]string{"Automatic", "Orange", "Green", "Violet", "Teal", "Rose", "Ochre", "Custom"}

func savedSettingsLane(raw json.RawMessage) (int, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, ""
	}
	var lane int
	if json.Unmarshal(raw, &lane) == nil && lane >= 0 && lane < 6 {
		return lane + 1, ""
	}
	var color string
	if json.Unmarshal(raw, &color) == nil && validLaneColor(color) {
		return 7, color
	}
	return 0, ""
}

func (m *AppModel) chooseSettingsLane(choice int) tea.Cmd {
	if m.settings.busy || choice < 0 || choice >= len(settingsLaneNames) {
		return nil
	}
	m.settings.choice, m.settings.status = choice, ""
	if choice == 7 {
		return m.settings.input.Focus()
	}
	m.settings.input.Blur()
	return nil
}

type settingsPeople struct {
	people          []api.User
	next, status    string
	loaded, loading bool
	generation      int
	cursors         map[string]bool
}

type settingsOperation struct {
	kind, handle, id, color, from, reason, details, value string
	lane                                                  *int
	enabled, receipts, current                            bool
}

type settingsResult struct {
	session, generation int
	op                  settingsOperation
	user                auth.User
	person              api.User
	err, cleanupErr     error
	signedOut           bool
}

type settingsPeopleResult struct {
	session, generation, list int
	cursor                    string
	page                      api.People
	err                       error
}

func (m *AppModel) openSettings() tea.Cmd {
	if m.settings.busy {
		return nil
	}
	m.settings.open = true
	m.settings.dialog = ""
	m.settings.selected = 0
	m.settings.viewport = viewport.New()
	m.manualLink = ""
	m.resetSequence()
	m.sizeSettings()
	return tea.Batch(m.startSettings(settingsOperation{kind: "refresh"}), m.loadSettingsTab(true))
}

func (m *AppModel) startSettings(op settingsOperation) tea.Cmd {
	s := &m.settings
	if !m.connected || m.settingsAPI == nil || s.busy || (s.uncertain && op.kind != "refresh") {
		return nil
	}
	if op.kind != "refresh" && (m.composer.sending || m.deletingID != "" || m.follow.cancel != nil || m.engagement.cancel != nil) {
		s.status = "Wait for the current action to finish."
		return nil
	}
	s.generation++
	s.busy, s.operation = true, op
	s.status = ""
	session, generation, client, store := m.session, s.generation, m.settingsAPI, m.account.store
	work := m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
		defer cancel()
		r := settingsResult{session: session, generation: generation, op: op}
		switch op.kind {
		case "refresh":
			r.user, r.err = client.CurrentUser(ctx)
		case "lane":
			r.user, r.err = client.SetLane(ctx, op.lane, op.color)
		case "github":
			r.user, r.err = client.SyncProfile(ctx)
		case "pasteis":
			r.user, r.err = client.SetPasteis(ctx, op.enabled)
		case "remotes":
			r.user, r.err = client.SetRemotes(ctx, op.enabled)
		case "remote-preferences":
			r.user, r.err = client.SetRemotePreferences(ctx, op.from, op.receipts)
		case "mute", "block":
			r.person, r.err = client.SetSafety(ctx, op.handle, op.kind, op.enabled)
		case "report":
			r.err = client.Report(ctx, op.id, op.reason, op.details)
		case "timezone":
			r.user, r.err = client.SetTimezone(ctx, op.value, op.enabled)
		case "automatic-translation":
			r.user, r.err = client.SetAutomaticTranslation(ctx, op.enabled)
		case "translation-language":
			r.user, r.err = client.SetTranslationLanguage(ctx, op.value)
		case "revoke-device":
			r.err = client.RevokeDevice(ctx, op.id)
			if r.err == nil && op.current {
				r.signedOut = true
				r.cleanupErr = store.Disconnect()
			}
		case "delete-account":
			r.err = client.DeleteAccount(ctx)
			if r.err == nil {
				r.signedOut = true
				r.cleanupErr = store.Disconnect()
			}
		}
		return r
	})
	progress := map[string]string{"github": "Syncing GitHub profile", "pasteis": "Updating pastéis mode", "remotes": "Updating Remotes", "remote-preferences": "Updating Remotes preferences", "automatic-translation": "Updating automatic translation"}[op.kind]
	if progress != "" {
		return tea.Batch(work, m.notify("info", progress, "Waiting for server confirmation."))
	}
	return work
}

func (m *AppModel) acceptSettings(r settingsResult) tea.Cmd {
	s := &m.settings
	if !m.connected || r.session != m.session || r.generation != s.generation || !s.busy {
		return nil
	}
	s.busy = false
	if r.err == nil && r.user.ID != "" && r.user.ID != m.account.user.ID {
		r.err = errors.New("server returned another account; refresh before retrying")
	}
	if r.err != nil {
		if m.revoked(r.err) {
			return nil
		}
		var failure *auth.APIError
		definite := errors.As(r.err, &failure) && failure.Status >= 400 && failure.Status < 500
		s.status = r.err.Error()
		if failure != nil && failure.Status == http.StatusTooManyRequests {
			s.status = retryAfterText(failure.RetryAfter, s.status)
		}
		if !definite && r.op.kind != "refresh" {
			s.uncertain = true
			s.status = "Outcome unknown. Refresh Settings before another write. Check the web before retrying deletion or a report."
			if r.op.kind == "mute" || r.op.kind == "block" {
				m.restrictSafetyContent(r.op.handle, r.op.kind)
			}
		}
		return m.notify("error", "Settings action failed", s.status)
	}
	if r.signedOut {
		m.leaveApplication()
		m.composer = composer{generation: m.composer.generation + 1}
		m.account.key = nil
		done, status := "Account deleted", "Your /dev/null account was deleted. Your GitHub account is unchanged."
		if r.op.kind == "revoke-device" {
			done, status = "Device revoked", "This device was revoked. Connect again to approve a new one."
		}
		m.account.state, m.account.status = "idle", status
		if r.cleanupErr != nil {
			m.account.state, m.account.status = "failed", done+", but the local key could not be removed. Disconnect locally before reconnecting."
			m.account.approvalRequired = true
		}
		return nil
	}
	if r.user.ID != "" {
		m.applyAccount(r.user)
		if r.user.TermsVersion != "" && !r.user.TermsAccepted {
			m.account.state, m.accountOpen = "terms", true
			s.open = false
		}
	}
	if r.op.kind == "mute" || r.op.kind == "block" {
		if r.op.enabled {
			m.restrictSafetyContent(r.op.handle, r.op.kind)
		} else {
			bit := uint8(1)
			if r.op.kind == "block" {
				bit = 2
			}
			if m.safetyHidden != nil {
				m.safetyHidden[strings.ToLower(r.op.handle)] &^= bit
			}
		}
		for i := range s.lists {
			s.lists[i] = settingsPeople{generation: s.lists[i].generation + 1}
		}
		for i := range m.profiles {
			p := &m.profiles[i]
			if strings.EqualFold(p.handle, r.op.handle) && r.op.kind == "mute" {
				p.profile.IsMuting = new(r.op.enabled)
			}
		}
	}
	m.applySettingsChange(r.op)
	s.status, s.uncertain, s.dialog = "", false, ""
	title := "Saved."
	if r.op.kind == "refresh" {
		title = "Account refreshed."
	}
	m.reflow()
	var list tea.Cmd
	if s.open && s.tab == settingsSafety {
		list = m.loadSafetyLists(false)
	}
	notice := m.notify("success", title, "")
	var refresh tea.Cmd
	if r.op.kind == "mute" || r.op.kind == "block" {
		m.invalidateNotificationCount()
		refresh = m.startSettings(settingsOperation{kind: "refresh"})
	}
	return tea.Batch(list, notice, refresh)
}

// applyAccount takes a returned CurrentUserResource, from a settings save or a live stream change.
func (m *AppModel) applyAccount(u auth.User) {
	m.account.user = u
	if u.UnreadNotifications != nil {
		m.invalidateNotificationCount()
		m.unreadNotifications = *u.UnreadNotifications
	}
	m.updateOwnIdentity(u)
}

func (m *AppModel) updateOwnIdentity(u auth.User) {
	var update func(*api.Commit)
	update = func(c *api.Commit) {
		if c == nil {
			return
		}
		if c.Author != nil && c.Author.ID == u.ID {
			c.Author.Name, c.Author.Handle, c.Author.AvatarURL, c.Author.Lane, c.Author.Live = u.Name, u.Handle, u.AvatarURL, u.Lane, u.Live
		}
		update(c.Quote)
		if c.ReplyPreview != nil {
			for i := range c.ReplyPreview.Data {
				update(&c.ReplyPreview.Data[i])
			}
		}
	}
	m.eachCommit(update)
	for i := range m.profiles {
		p := &m.profiles[i]
		if p.profile.User.ID == u.ID {
			person := &p.profile.User
			person.Name, person.Handle, person.AvatarURL, person.Lane = u.Name, u.Handle, u.AvatarURL, u.Lane
			person.Bio, person.Company, person.Location, person.Website, person.GitHubURL = u.Bio, u.Company, u.Location, u.Website, u.GitHubURL
			if u.Commits != nil {
				person.Commits = u.Commits
			}
			if u.Followers != nil {
				person.Followers = u.Followers
			}
			if u.Following != nil {
				person.Following = u.Following
			}
			p.handle = u.Handle
			p.cancel()
			m.profileGeneration++
			p.generation = m.profileGeneration
			p.ctx, p.cancel = context.WithCancel(m.account.life.ctx)
			for j := range p.lists {
				p.lists[j].generation++
				p.lists[j].loading = false
			}
		}
	}
}

func (m *AppModel) loadSafetyLists(refresh bool) tea.Cmd {
	var commands []tea.Cmd
	for i := range m.settings.lists {
		if refresh || !m.settings.lists[i].loaded {
			commands = append(commands, m.loadSafety(i, false))
		}
	}
	return tea.Batch(commands...)
}

func (m *AppModel) loadSafety(list int, more bool) tea.Cmd {
	s := &m.settings
	f := &s.lists[list]
	if f.loading || m.settingsAPI == nil || (more && f.next == "") {
		return nil
	}
	if !more {
		f.cursors = map[string]bool{}
	}
	cursor := ""
	if more {
		cursor = f.next
		f.cursors[cursor] = true
	}
	f.generation++
	f.loading, f.status = true, ""
	m.sizeSettings()
	session, generation, client := m.session, f.generation, m.settingsAPI
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 20*time.Second)
		defer cancel()
		page, err := client.SafetyList(ctx, []string{"mute", "block"}[list], cursor)
		return settingsPeopleResult{session, generation, list, cursor, page, err}
	})
}

func (m *AppModel) acceptSafetyList(r settingsPeopleResult) tea.Cmd {
	if !m.connected || r.session != m.session || r.list < 0 || r.list > 1 {
		return nil
	}
	f := &m.settings.lists[r.list]
	if r.generation != f.generation || !f.loading {
		return nil
	}
	f.loading = false
	if r.err != nil {
		if !m.revoked(r.err) {
			f.status = r.err.Error()
		}
		m.sizeSettings()
		return nil
	}
	if r.cursor == "" {
		f.people = nil
	}
	for _, u := range r.page.Data {
		found := false
		for _, old := range f.people {
			found = found || old.ID == u.ID
		}
		if !found {
			f.people = append(f.people, u)
		}
	}
	f.next, f.loaded = r.page.Next, true
	if f.cursors[f.next] {
		f.next, f.status = "", "Repeated pagination cursor. Refresh to retry."
	}
	m.sizeSettings()
	return nil
}

func (m *AppModel) selectSettingsTab(tab int) tea.Cmd {
	s := &m.settings
	s.tab, s.selected = tab, 0
	s.viewport.GotoTop()
	m.sizeSettings()
	return m.loadSettingsTab(false)
}

func (m *AppModel) loadSettingsTab(refresh bool) tea.Cmd {
	s := &m.settings
	load := &s.regionLoad
	switch s.tab {
	case settingsSafety:
		return m.loadSafetyLists(refresh)
	case settingsDevices:
		load = &s.devicesLoad
	case settingsRegionTab:
	default:
		return nil
	}
	if m.settingsAPI == nil || load.loading || (load.loaded && !refresh) {
		return nil
	}
	load.generation++
	load.loading, load.status = true, ""
	m.sizeSettings()
	session, generation, tab, client := m.session, load.generation, s.tab, m.settingsAPI
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 20*time.Second)
		defer cancel()
		r := settingsLoadResult{session: session, generation: generation, tab: tab}
		if tab == settingsDevices {
			r.devices, r.err = client.Devices(ctx)
		} else {
			r.region, r.err = client.AccountSettings(ctx)
		}
		return r
	})
}

func (m *AppModel) acceptSettingsLoad(r settingsLoadResult) tea.Cmd {
	s := &m.settings
	load := &s.regionLoad
	if r.tab == settingsDevices {
		load = &s.devicesLoad
	}
	if !m.connected || r.session != m.session || r.generation != load.generation || !load.loading {
		return nil
	}
	load.loading = false
	if r.err != nil {
		if !m.revoked(r.err) {
			load.status = r.err.Error()
			m.sizeSettings()
		}
		return nil
	}
	if !load.loaded && s.tab == r.tab {
		s.selected = 0
	}
	load.loaded = true
	if r.tab == settingsDevices {
		s.devices = r.devices
	} else {
		s.region = r.region
	}
	m.sizeSettings()
	return nil
}

// applySettingsChange mirrors a confirmed write locally, because these routes do not return the changed values.
func (m *AppModel) applySettingsChange(op settingsOperation) {
	s := &m.settings
	switch op.kind {
	case "timezone":
		s.region.Timezone, s.region.ShowLocalTime = op.value, op.value != "" && op.enabled
	case "automatic-translation":
		s.region.AutomaticTranslation = op.enabled
	case "translation-language":
		s.region.TranslationLanguage = op.value
	case "revoke-device":
		var devices []api.Device
		for _, d := range s.devices {
			if d.ID != op.id {
				devices = append(devices, d)
			}
		}
		s.devices = devices
	}
}

func (m *AppModel) beginSettingsDialog(kind, handle, id string) tea.Cmd {
	if m.settings.busy {
		return nil
	}
	s := &m.settings
	s.dialog, s.handle, s.targetID, s.focus, s.choice, s.status = kind, handle, id, 0, 0, ""
	s.input = textinput.New()
	s.input.SetVirtualCursor(true)
	s.input.CharLimit = 500
	s.input.SetWidth(max(1, min(60, m.account.width-12)))
	if kind == "lane" {
		s.input.CharLimit = 7
		s.input.Placeholder = "#rrggbb"
		choice, color := savedSettingsLane(m.account.user.PreferredLane)
		s.choice = choice
		s.input.SetValue(color)
		m.sizeSettings()
		return m.chooseSettingsLane(s.choice)
	}
	if kind == "delete-account" {
		s.input.CharLimit = 39
		s.input.Placeholder = m.account.user.Handle
	}
	if kind == "timezone" {
		s.input.CharLimit = 64
		s.input.Placeholder = "Region/City, such as Europe/Lisbon"
		s.input.SetValue(s.region.Timezone)
		s.input.CursorEnd()
	}
	if kind == "translation-language" {
		s.input.CharLimit = 40
		s.input.Placeholder = "Search by English or native name"
		for i, language := range m.settingsLanguages() {
			if language.Code == s.region.TranslationLanguage {
				s.choice = i
			}
		}
	}
	if kind == "report" {
		s.input.Placeholder = "Optional details (500 characters)"
	}
	if kind == "timezone" || kind == "translation-language" {
		// Preference dialogs are reversible, so Enter after typing saves instead of discarding the input.
		s.focus = 1
	}
	if kind == "delete-account" || kind == "report" || kind == "timezone" || kind == "translation-language" {
		return s.input.Focus()
	}
	return nil
}

func (m *AppModel) settingsDialogKey(msg tea.KeyPressMsg) tea.Cmd {
	s := &m.settings
	if s.busy {
		return nil
	}
	if msg.String() == "esc" {
		s.dialog = ""
		s.input.Blur()
		return nil
	}
	if m.account.width < 60 || m.account.height < 18 {
		return nil
	}
	switch msg.String() {
	case "tab", "shift+tab":
		s.focus = 1 - s.focus
		return nil
	case "ctrl+s":
		return m.submitSettingsDialog()
	case "enter":
		if s.focus == 0 {
			s.dialog = ""
			return nil
		}
		return m.submitSettingsDialog()
	case "up", "down", "left", "right":
		count := 0
		if s.dialog == "lane" && (msg.String() == "up" || msg.String() == "down" || s.choice != 7) {
			count = 8
		}
		if s.dialog == "report" && (msg.String() == "up" || msg.String() == "down") {
			count = len(api.ReportReasons)
		}
		if s.dialog == "translation-language" && (msg.String() == "up" || msg.String() == "down") {
			count = len(m.settingsLanguages())
		}
		if count > 0 {
			delta := 1
			if msg.String() == "up" || msg.String() == "left" {
				delta = -1
			}
			s.choice = (s.choice + delta + count) % count
			if s.dialog == "lane" {
				return m.chooseSettingsLane(s.choice)
			}
			return nil
		}
	}
	if s.dialog == "lane" && s.choice != 7 {
		return nil
	}
	query := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.dialog == "translation-language" && s.input.Value() != query {
		s.choice = 0
	}
	return cmd
}

// settingsLanguages lists "follow this device" first, then server languages matching the dialog's search.
func (m AppModel) settingsLanguages() []api.Language {
	query := strings.ToLower(strings.TrimSpace(m.settings.input.Value()))
	languages := []api.Language{{Name: "Follow device language"}}
	if query != "" {
		languages = nil
	}
	for _, l := range m.settings.region.TranslationLanguages {
		if query == "" || strings.Contains(strings.ToLower(l.Name+" "+l.NativeName+" "+l.Code), query) {
			languages = append(languages, l)
		}
	}
	return languages
}

func (m *AppModel) submitSettingsDialog() tea.Cmd {
	s := &m.settings
	if m.account.width < 60 || m.account.height < 18 || s.busy {
		return nil
	}
	op := settingsOperation{kind: s.dialog, handle: s.handle, id: s.targetID}
	switch s.dialog {
	case "lane":
		if s.choice > 0 && s.choice < 7 {
			op.lane = new(s.choice - 1)
		}
		if s.choice == 7 {
			op.color = s.input.Value()
			if !validLaneColor(op.color) {
				s.status = "Enter a lowercase color such as #ff8a4c."
				return nil
			}
		}
	case "report":
		op.reason, op.details = api.ReportReasons[s.choice].Value, s.input.Value()
	case "delete-account":
		if s.input.Value() != m.account.user.Handle {
			s.status = "Type your exact handle to confirm permanent account deletion."
			return nil
		}
	case "timezone":
		op.value, op.enabled = strings.TrimSpace(s.input.Value()), s.region.ShowLocalTime
	case "translation-language":
		languages := m.settingsLanguages()
		if s.choice >= len(languages) {
			s.status = "No language matches this search."
			return nil
		}
		op.value = languages[s.choice].Code
	case "revoke-device":
		for _, d := range s.devices {
			op.current = op.current || (d.ID == op.id && m.account.key != nil && d.Fingerprint == m.account.key.Fingerprint())
		}
	case "mute", "block":
		op.enabled = true
	case "unmute", "unblock":
		op.kind, op.enabled = strings.TrimPrefix(s.dialog, "un"), false
	default:
		return nil
	}
	return m.startSettings(op)
}

func validLaneColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, r := range value[1:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (m *AppModel) settingsKey(pressed string) tea.Cmd {
	s := &m.settings
	if s.busy {
		return nil
	}
	switch pressed {
	case "a":
		m.accountOpen = true
		m.resetSequence()
		return nil
	case "esc":
		s.open = false
		m.resetSequence()
		return nil
	case "left", "right", "tab", "shift+tab":
		delta := 1
		if pressed == "left" || pressed == "shift+tab" {
			delta = -1
		}
		return m.selectSettingsTab((s.tab + delta + len(settingsTabs)) % len(settingsTabs))
	case "j", "down":
		m.moveSettingsSelection(1)
	case "k", "up":
		m.moveSettingsSelection(-1)
	case "pgdown", "ctrl+d":
		m.moveSettingsSelection(5)
	case "pgup", "ctrl+u":
		m.moveSettingsSelection(-5)
	case "ctrl+r":
		return tea.Batch(m.startSettings(settingsOperation{kind: "refresh"}), m.loadSettingsTab(true))
	case "enter", "space":
		rows := m.settingsRows()
		if s.selected >= 0 && s.selected < len(rows) {
			return m.activateSetting(rows[s.selected].action, rows[s.selected].value)
		}
	}
	m.sizeSettings()
	return nil
}

func (m *AppModel) moveSettingsSelection(delta int) {
	rows := m.settingsRows()
	step := 1
	if delta < 0 {
		step = -1
	}
	for range max(delta, -delta) {
		next := m.settings.selected + step
		for next >= 0 && next < len(rows) && rows[next].action == "" {
			next += step
		}
		if next < 0 || next >= len(rows) {
			break
		}
		m.settings.selected = next
	}
}

func (m *AppModel) activateSetting(action, value string) tea.Cmd {
	s := &m.settings
	u := m.account.user
	switch action {
	case "github":
		return m.startSettings(settingsOperation{kind: action})
	case "lane", "delete-account", "timezone", "translation-language":
		return m.beginSettingsDialog(action, "", "")
	case "local-time":
		return m.startSettings(settingsOperation{kind: "timezone", value: s.region.Timezone, enabled: !s.region.ShowLocalTime})
	case "automatic-translation":
		return m.startSettings(settingsOperation{kind: action, enabled: !s.region.AutomaticTranslation})
	case "revoke-device":
		return m.beginSettingsDialog(action, "", value)
	case "refresh-region", "refresh-devices":
		return m.loadSettingsTab(true)
	case "pasteis":
		return m.startSettings(settingsOperation{kind: action, enabled: !u.PasteisMode})
	case "remotes":
		return m.startSettings(settingsOperation{kind: action, enabled: u.Remotes == nil})
	case "receipts", "audience":
		if u.Remotes == nil {
			return nil
		}
		from, receipts := u.Remotes.From, u.Remotes.PullReceipts
		if action == "receipts" {
			receipts = !receipts
		} else if u.Remotes.Everyone {
			if from == "following" {
				from = "everyone"
			} else {
				from = "following"
			}
		} else {
			return nil
		}
		return m.startSettings(settingsOperation{kind: "remote-preferences", from: from, receipts: receipts})
	case "theme":
		m.account.toggleTheme()
		m.reflow()
	case "update-check":
		m.tuiSettings.CheckForUpdates = !m.tuiSettings.CheckForUpdates
		m.sizeSettings()
		return m.saveTUISettings(false)
	case "more-muted":
		return m.loadSafety(0, true)
	case "more-blocked":
		return m.loadSafety(1, true)
	case "refresh-muted":
		return m.loadSafety(0, false)
	case "refresh-blocked":
		return m.loadSafety(1, false)
	case "unmute", "unblock":
		return m.beginSettingsDialog(action, value, "")
	case "profile":
		s.open = false
		return m.openProfile(u.Handle)
	}
	return nil
}

func (m *AppModel) beginReport() tea.Cmd {
	c, ok := m.focusedEngagementCommit()
	if !ok || c.Author == nil || c.DeletedAt != nil || c.Author.ID == m.account.user.ID || (c.Status != "" && c.Status != "approved") {
		return nil
	}
	return m.beginSettingsDialog("report", "", c.ID)
}

func (m *AppModel) beginProfileSafety(kind string) tea.Cmd {
	p := m.topProfile()
	if p == nil || p.people || p.organization || !p.loaded || p.profile.User.ID == m.account.user.ID {
		return nil
	}
	if kind == "mute" && p.profile.IsMuting != nil && *p.profile.IsMuting {
		kind = "unmute"
	}
	return m.beginSettingsDialog(kind, p.handle, "")
}

func (m *AppModel) restrictSafetyContent(handle, kind string) {
	if m.safetyHidden == nil {
		m.safetyHidden = map[string]uint8{}
	}
	bit := uint8(1)
	if kind == "block" {
		bit = 2
	}
	m.safetyHidden[strings.ToLower(handle)] |= bit
	for i := range m.feeds {
		f := &m.feeds[i]
		if f.cancel != nil {
			f.cancel()
			f.cancel = nil
		}
		f.generation++
		f.loading = false
		anchor := ""
		if f.selected < len(f.commits) {
			anchor = f.commits[f.selected].ID
		}
		f.commits = m.safeCommits(f.commits)
		f.suggestions = m.safeCommits(f.suggestions)
		f.submitted = m.safeCommits(f.submitted)
		f.selected = max(0, min(f.selected, len(f.commits)-1))
		for j, c := range f.commits {
			if c.ID == anchor {
				f.selected = j
			}
		}
	}
	if m.starred.cancel != nil {
		m.starred.cancel()
		m.starred.cancel = nil
	}
	m.starred.generation++
	m.starred.loading = false
	m.starred.commits = m.safeCommits(m.starred.commits)
	m.starred.selected = max(0, min(m.starred.selected, len(m.starred.commits)-1))
	m.cancelNotifications()
	for _, f := range m.notificationFeeds {
		f.items = m.safeNotifications(f.items)
		f.selected = max(0, min(f.selected, len(f.items)-1))
		for _, g := range f.groups {
			g.items = m.safeNotifications(g.items)
		}
	}
	for i := range m.threads {
		t := &m.threads[i]
		if len(m.safeCommitsFor([]api.Commit{t.root}, true)) == 0 {
			for j := i; j < len(m.threads); j++ {
				m.threads[j].cancel()
			}
			m.threads = m.threads[:i]
			break
		}
		t.cancel()
		m.threadGeneration++
		t.generation, t.loading = m.threadGeneration, false
		t.ctx, t.cancel = context.WithCancel(m.account.life.ctx)
		t.root = m.safeCommitsFor([]api.Commit{t.root}, true)[0]
		t.ancestors = m.safeCommits(t.ancestors)
		t.replies = m.safeCommits(t.replies)
		for key, children := range t.children {
			t.children[key] = m.safeCommits(children)
		}
		t.selected = max(0, min(t.selected, len(t.entries())-1))
	}
	for i := 0; i < len(m.profiles); i++ {
		p := &m.profiles[i]
		if p.below > len(m.threads) || (kind == "block" && strings.EqualFold(p.handle, handle)) {
			for j := i; j < len(m.profiles); j++ {
				m.profiles[j].cancel()
			}
			m.profiles = m.profiles[:i]
			break
		}
		p.cancel()
		m.profileGeneration++
		p.generation = m.profileGeneration
		p.ctx, p.cancel = context.WithCancel(m.account.life.ctx)
		if strings.EqualFold(p.handle, handle) {
			p.profile.IsMuting = new(true)
		}
		if p.profile.Pinned != nil {
			commits := m.safeCommitsFor([]api.Commit{*p.profile.Pinned}, true)
			if len(commits) == 0 {
				p.profile.Pinned = nil
			} else {
				p.profile.Pinned = &commits[0]
			}
		}
		for l := range p.lists {
			list := &p.lists[l]
			list.loading = false
			list.generation++
			list.commits = m.safeCommitsFor(list.commits, true)
			if kind == "block" {
				var people []api.User
				for _, u := range list.people {
					if !strings.EqualFold(u.Handle, handle) {
						people = append(people, u)
					}
				}
				list.people = people
			}
			count := len(list.commits)
			if p.people {
				count = len(list.organizations) + len(list.people)
			}
			list.selected = max(0, min(list.selected, count-1))
		}
	}
	m.closeViewer()
	if c := m.composer.target; kind == "block" && c.Author != nil && strings.EqualFold(c.Author.Handle, handle) {
		m.invalidatePreview()
		m.composer.target = api.Commit{ID: c.ID, DeletedAt: &m.now}
		m.composer.status = "The reply or quote target is unavailable. Your draft text is kept; discard it to start a different commit."
	}
	m.images.Forget()
	if kind == "block" {
		m.account.user.Followers, m.account.user.Following = nil, nil
	}
	m.reflow()
}

func (m AppModel) safeCommits(commits []api.Commit) []api.Commit {
	return m.safeCommitsFor(commits, false)
}

func (m AppModel) safeCommitsFor(commits []api.Commit, direct bool) []api.Commit {
	var result []api.Commit
	mask := uint8(3)
	if direct {
		mask = 2
	}
	for _, c := range commits {
		if c.Author != nil && m.safetyHidden[strings.ToLower(c.Author.Handle)]&mask != 0 {
			continue
		}
		if c.Quote != nil {
			quotes := m.safeCommitsFor([]api.Commit{*c.Quote}, direct)
			if len(quotes) == 0 {
				c.Quote = nil
			} else {
				c.Quote = &quotes[0]
			}
		}
		if c.ReplyPreview != nil {
			preview := *c.ReplyPreview
			preview.Data = m.safeCommits(preview.Data)
			c.ReplyPreview = &preview
		}
		result = append(result, c)
	}
	return result
}

func (m AppModel) safeNotifications(items []api.Notification) []api.Notification {
	var result []api.Notification
	for _, n := range items {
		hidden := n.Actor != nil && m.safetyHidden[strings.ToLower(n.Actor.Handle)] != 0
		for _, a := range n.Actors {
			hidden = hidden || m.safetyHidden[strings.ToLower(a.Handle)] != 0
		}
		if hidden {
			continue
		}
		if n.Commit != nil {
			commits := m.safeCommits([]api.Commit{*n.Commit})
			if len(commits) == 0 {
				continue
			}
			n.Commit = &commits[0]
		}
		result = append(result, n)
	}
	return result
}
