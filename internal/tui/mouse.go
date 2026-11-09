package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m AppModel) mouseKey(name string) (AppModel, tea.Cmd) {
	key := tea.KeyPressMsg{}
	switch name {
	case "enter":
		key.Code = tea.KeyEnter
	case "tab":
		key.Code = tea.KeyTab
	case "space":
		key.Code = tea.KeySpace
	case "down":
		key.Code = tea.KeyDown
	case "left":
		key.Code = tea.KeyLeft
	case "right":
		key.Code = tea.KeyRight
	case "esc":
		key.Code = tea.KeyEscape
	case "f2":
		key.Code = tea.KeyF2
	case "ctrl+r":
		key.Code = 'r'
		key.Mod = tea.ModCtrl
	case "ctrl+c":
		key.Code = 'c'
		key.Mod = tea.ModCtrl
	case "ctrl+p":
		key.Code = 'p'
		key.Mod = tea.ModCtrl
	case "ctrl+k":
		key.Code = 'k'
		key.Mod = tea.ModCtrl
	case "ctrl+l":
		key.Code = 'l'
		key.Mod = tea.ModCtrl
	case "ctrl+d":
		key.Code = 'd'
		key.Mod = tea.ModCtrl
	case "ctrl+s":
		key.Code = 's'
		key.Mod = tea.ModCtrl
	case "ctrl+enter":
		key.Code = tea.KeyEnter
		key.Mod = tea.ModCtrl
	default:
		if len([]rune(name)) != 1 {
			return m, nil
		}
		key.Code = []rune(name)[0]
		key.Text = name
	}
	next, cmd := m.update(key)
	return next.(AppModel), cmd
}

func (m AppModel) handleMouse(msg tea.MouseClickMsg) (AppModel, tea.Cmd) {
	if msg.Button != tea.MouseLeft || msg.X < 0 || msg.Y < 0 || msg.X >= m.account.width || msg.Y >= m.account.height {
		return m, nil
	}
	if m.connected && !m.accountOpen && m.settings.dialog != "" {
		_, r, areas := m.settingsDialog(m.account.colors())
		if (uiRect{r.x + r.w - 3, r.y + 1, 1, 1}).contains(msg.X, msg.Y) {
			return m.mouseKey("esc")
		}
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				if area.action == "lane-choice" || area.action == "lane-input" {
					cmd := m.chooseSettingsLane(area.index)
					return m, cmd
				}
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.settings.open && m.settings.busy {
		return m, nil
	}
	if m.toast.visible && m.composer.confirm == "" && m.account.width >= 20 && m.account.height >= 8 {
		_, r := m.toastView()
		if r.contains(msg.X, msg.Y) {
			if msg.Y == r.y+1 && msg.X >= r.x+r.w-4 {
				m.toast.visible = false
				return m, nil
			}
			if m.undo.id != "" && m.toast.id == m.undo.toast {
				return m, m.startUndo()
			}
			return m, nil
		}
	}
	if m.composer.confirm != "" {
		_, _, areas := m.confirmationModal(m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.composer.open && m.connected && !m.accountOpen {
		return m.composerMouse(msg)
	}
	if m.connected && !m.accountOpen && m.search != nil {
		return m.searchMouse(msg.X, msg.Y)
	}
	if m.connected && !m.accountOpen && m.achievements != nil {
		return m.achievementsMouse(msg.X, msg.Y)
	}
	if m.connected && m.vote.confirm {
		_, _, areas := m.voteModal(m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.connected && m.remotes.dialog != "" && m.remotesOnTop() {
		return m.remotesDialogMouse(msg.X, msg.Y)
	}
	if m.connected && m.remotes.members != nil && m.remotes.thread != nil && m.remotesOnTop() {
		return m.membersMouse(msg.X, msg.Y)
	}
	if m.connected && m.remotes.thread != nil && (m.remotes.pinWarning != nil || m.remotes.pinList != nil) && m.remotesOnTop() {
		return m.pinModalMouse(msg.X, msg.Y)
	}
	if m.connected && m.remotes.people != nil && m.remotesOnTop() {
		return m.peopleMouse(msg.X, msg.Y)
	}
	if menu := m.activeReactions(); menu != nil && menu.picking {
		return m.reactionModalMouse(msg.X, msg.Y)
	}
	if m.connected && m.live != nil {
		_, _, areas := m.liveModal(m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.connected && !m.accountOpen && m.ask != nil {
		return m.askMouse(msg.X, msg.Y)
	}
	if m.connected && !m.accountOpen && m.picker != nil {
		return m.listPickerMouse(msg.X, msg.Y)
	}
	if m.connected && !m.accountOpen && m.whoToFollow.open {
		return m.suggestionModalMouse(msg.X, msg.Y)
	}
	if m.connected && !m.accountOpen && m.sharingCommit() {
		return m.peopleMouse(msg.X, msg.Y)
	}
	if m.connected && m.deleting.id != "" {
		_, _, areas := m.deleteModal(m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.vote.commitID != "" {
		m.vote.commitID = ""
		m.reflow()
	}
	if m.connected && !m.accountOpen && m.viewer.open {
		return m.viewerMouse(msg.X, msg.Y)
	}
	if !m.connected || m.accountOpen {
		_, areas := m.account.accountScreen(m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				if area.action == "account" {
					m.account.actionFocus = area.index
					return m.mouseKey("enter")
				}
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.whatsNew.open {
		r := m.whatsNewBounds()
		if (uiRect{r.x + r.w - 3, r.y + 1, 1, 1}).contains(msg.X, msg.Y) {
			return m.mouseKey("esc")
		}
		return m, nil
	}
	if m.upgradeVisible() {
		return m.upgradeClick(msg.X, msg.Y)
	}
	if m.help {
		r := m.helpBounds()
		if (uiRect{r.x + r.w - 3, r.y + 1, 1, 1}).contains(msg.X, msg.Y) {
			return m.mouseKey("esc")
		}
		return m, nil
	}
	if m.account.width < 30 || m.account.height < 10 {
		return m, nil
	}
	if lines := strings.Split(m.workspaceView().Content, "\n"); msg.Y < len(lines) && !m.searchBlocked() {
		if tag := hashtagAt(lines[msg.Y], msg.X); tag != "" {
			cmd := m.openSearch(tag)
			return m, cmd
		}
		if link := linkAt(lines[msg.Y], msg.X); link != "" {
			return m, m.clickLink(link)
		}
		if id := starsAt(lines[msg.Y], msg.X); id != "" {
			return m, m.openStargazersFor(id)
		}
	}
	nav, center, aside := m.columns()
	left := (m.account.width - nav - center - aside - 2) / 2
	if (uiRect{left, 0, nav, m.mainHeight()}).contains(msg.X, msg.Y) {
		_, areas := m.navigationLayout(nav, m.mainHeight(), m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X-left, msg.Y) {
				m.settings.open = false
				if area.action == "log" {
					return m.switchLog(m.lastLog)
				}
				if area.action == "notifications" {
					return m.openNotifications()
				}
				if area.action == "profile" {
					cmd := m.openProfile(m.account.user.Handle)
					return m, cmd
				}
				if area.action == "starred" {
					return m.openStarred()
				}
				if area.action == "scheduled" {
					return m.openScheduled()
				}
				if area.action == "drafts" {
					return m.openDrafts()
				}
				if area.action == "remotes" {
					return m.openRemotes()
				}
				if area.action == "settings" {
					cmd := m.openSettings()
					return m, cmd
				}
				m.settings.open = false
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.remotesWide() {
		if x := msg.X - left - nav - 1; x >= 0 && x < m.remotesWidth() {
			return m.remotesMouse(x, msg.Y)
		}
		return m, nil
	}
	if aside > 0 && (uiRect{left + nav + center + 2, 0, aside, m.mainHeight()}).contains(msg.X, msg.Y) {
		_, areas := m.asideLayout(aside, m.mainHeight(), m.account.colors())
		for _, area := range areas {
			if area.contains(msg.X-left-nav-center-2, msg.Y) {
				if area.action == "watch" {
					return m, m.watchLive(m.liveNow[area.index].Live)
				}
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.manualLink != "" {
		_, back := m.manualLinkView(center, m.account.colors())
		if back.contains(msg.X-left-nav-1, msg.Y) {
			return m.mouseKey("esc")
		}
		return m, nil
	}
	if m.draftsOnTop() && !m.settings.open {
		if x := msg.X - left - nav - 1; x >= 0 && x < center {
			return m.draftsMouse(x, msg.Y, center)
		}
		return m, nil
	}
	if m.settings.open {
		if m.settings.busy {
			return m, nil
		}
		x := msg.X - left - nav - 1
		if x >= 0 && x < center {
			if msg.Y == 2 {
				start := 2
				for i, name := range settingsTabs {
					if x >= start && x < start+len([]rune(name)) {
						cmd := m.selectSettingsTab(i)
						return m, cmd
					}
					start += len([]rune(name)) + 3
				}
			}
			row := msg.Y - 4 + m.settings.viewport.YOffset()
			for i, start := range m.settings.starts {
				if row == start {
					if m.settingsRows()[i].action == "" {
						return m, nil
					}
					m.settings.selected = i
					cmd := m.settingsKey("enter")
					return m, cmd
				}
			}
		}
		return m, nil
	}
	if !m.notificationsOpen && !m.starredOpen && len(m.threads) == 0 && len(m.profiles) == 0 && (uiRect{left + nav + 1, 5, center, 1}).contains(msg.X, msg.Y) {
		return m.mouseKey("c")
	}
	return m.handleFeedMouse(msg)
}

func (m AppModel) composerMouse(msg tea.MouseClickMsg) (AppModel, tea.Cmd) {
	r := m.composerBounds()
	if (uiRect{r.x + r.w - 3, r.y + 1, 1, 1}).contains(msg.X, msg.Y) {
		if m.composer.sending {
			m.composer.status = "Submission in progress. Wait for its result before closing."
			return m, nil
		}
		cmd := m.requestDraftAction("discard")
		return m, cmd
	}
	// The browser, the attachment field, and Post as own the keyboard until Enter or Esc, so clicks wait for them to close.
	if !m.composerUsable() || m.composer.browser != nil || m.composer.editing != "" || m.composer.picker != nil {
		return m, nil
	}
	for _, area := range m.composerControls() {
		if area.contains(msg.X, msg.Y) {
			if m.composer.sending {
				return m, nil
			}
			m.composer.input.EndSelection()
			m.composer.dragging = false
			m.composer.suggest.close()
			m.composer.focus = area.index
			m.composer.input.Blur()
			return m.mouseKey("enter")
		}
	}
	if m.composer.sending || m.composer.owner != m.account.user.ID {
		return m, nil
	}
	if m.composerIdentityArea().contains(msg.X, msg.Y) {
		cmd := m.openAuthorPicker()
		return m, cmd
	}
	editor := m.composerEditorBounds()
	if s := &m.composer.suggest; s.count() > 0 && (uiRect{editor.x, editor.y + editor.h, editor.w, m.composerStatusRows()}).contains(msg.X, msg.Y) {
		index := max(0, min(s.index-1, s.count()-3)) + msg.Y - editor.y - editor.h
		if index < s.count() {
			s.index = index
			cmd := m.acceptSuggestion()
			return m, cmd
		}
		return m, nil
	}
	if editor.contains(msg.X, msg.Y) && !m.composer.previewing {
		m.composer.focus = 0
		m.composer.input.BeginSelection(msg.X-editor.x, msg.Y-editor.y)
		m.composer.dragging = true
		return m, tea.Batch(m.refreshSuggestions(), m.focusComposer())
	}
	return m, nil
}

func (m *AppModel) dragComposer(x, y int) {
	c := &m.composer
	if !c.dragging || !c.open || c.sending || c.previewing || c.confirm != "" || c.owner != m.account.user.ID {
		return
	}
	r := m.composerEditorBounds()
	c.input.ExtendSelection(x-r.x, y-r.y)
	c.suggest.close()
}

func (m AppModel) mouseWheel(msg tea.MouseWheelMsg) (AppModel, tea.Cmd) {
	if m.composer.confirm != "" || m.deleting.id != "" || m.live != nil || m.vote.confirm || m.settings.dialog != "" || m.ask != nil {
		return m, nil
	}
	if m.picker != nil && m.connected && !m.accountOpen {
		pressed := "down"
		if msg.Button == tea.MouseWheelUp {
			pressed = "up"
		}
		return m.listPickerKey(pressed)
	}
	if m.whoToFollow.open && m.connected && !m.accountOpen {
		pressed := "down"
		if msg.Button == tea.MouseWheelUp {
			pressed = "up"
		}
		return m.suggestionModalKey(pressed)
	}
	code := tea.KeyDown
	if msg.Button == tea.MouseWheelUp {
		code = tea.KeyUp
	} else if msg.Button != tea.MouseWheelDown {
		return m, nil
	}
	if m.search != nil && m.connected && !m.accountOpen && !m.composer.open {
		cmd := m.searchKey(tea.KeyPressMsg{Code: code})
		return m, cmd
	}
	if m.achievements != nil && m.connected && !m.accountOpen {
		pressed := "down"
		if code == tea.KeyUp {
			pressed = "up"
		}
		return m.achievementsKey(pressed)
	}
	if m.composer.open && m.connected && !m.accountOpen {
		if !m.composerUsable() || m.composer.sending || !m.composerBounds().contains(msg.X, msg.Y) {
			return m, nil
		}
		if !m.composer.previewing && !m.composerEditorBounds().contains(msg.X, msg.Y) && m.composer.suggest.count() == 0 {
			return m, nil
		}
		if !m.composer.previewing && m.composer.suggest.count() == 0 {
			m.composer.focus = 0
			m.composer.input.EndSelection()
			if code == tea.KeyUp {
				m.composer.input.CursorUp()
			} else {
				m.composer.input.CursorDown()
			}
			return m, m.focusComposer()
		}
		cmd := m.composerKey(tea.KeyPressMsg{Code: code})
		return m, cmd
	}
	if m.connected && !m.accountOpen {
		if code == tea.KeyUp {
			return m.handleKey("up")
		}
		return m.handleKey("down")
	}
	return m, nil
}
