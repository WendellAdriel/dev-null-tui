package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m AppModel) mouseKey(name string) (AppModel, tea.Cmd) {
	key := tea.KeyPressMsg{}
	switch name {
	case "enter":
		key.Code = tea.KeyEnter
	case "tab":
		key.Code = tea.KeyTab
	case "down":
		key.Code = tea.KeyDown
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
	if m.toast.visible && m.composer.confirm == "" && m.account.width >= 20 && m.account.height >= 8 {
		_, r := m.toastView()
		if r.contains(msg.X, msg.Y) {
			if msg.Y == r.y+1 && msg.X >= r.x+r.w-4 {
				m.toast.visible = false
			}
			return m, nil
		}
	}
	if m.composer.confirm != "" {
		_, _, areas := m.confirmationModal(colors(m.account.dark))
		for _, area := range areas {
			if area.contains(msg.X, msg.Y) {
				m.composer.confirmAction = area.index == 1
				return m.mouseKey("enter")
			}
		}
		return m, nil
	}
	if m.composer.open && m.connected && !m.accountOpen {
		return m.composerMouse(msg)
	}
	if !m.connected || m.accountOpen {
		_, areas := m.account.accountScreen(colors(m.account.dark))
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
	nav, center, aside := m.columns()
	left := (m.account.width - nav - center - aside - 2) / 2
	if (uiRect{left, 0, nav, m.mainHeight()}).contains(msg.X, msg.Y) {
		_, areas := m.navigationLayout(nav, m.mainHeight(), colors(m.account.dark))
		for _, area := range areas {
			if area.contains(msg.X-left, msg.Y) {
				if area.action == "log" {
					return m.switchLog(m.lastLog)
				}
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if aside > 0 && (uiRect{left + nav + center + 2, 0, aside, m.mainHeight()}).contains(msg.X, msg.Y) {
		_, areas := m.asideLayout(aside, m.mainHeight(), colors(m.account.dark))
		for _, area := range areas {
			if area.contains(msg.X-left-nav-center-2, msg.Y) {
				return m.mouseKey(area.action)
			}
		}
		return m, nil
	}
	if m.manualLink != "" {
		_, back := m.manualLinkView(center, colors(m.account.dark))
		if back.contains(msg.X-left-nav-1, msg.Y) {
			return m.mouseKey("esc")
		}
		return m, nil
	}
	if (uiRect{left + nav + 1, 5, center, 1}).contains(msg.X, msg.Y) {
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
	if !m.composerUsable() {
		return m, nil
	}
	for _, area := range m.composerControls() {
		if area.contains(msg.X, msg.Y) {
			if m.composer.sending {
				return m, nil
			}
			m.composer.input.EndSelection()
			m.composer.dragging = false
			m.composer.emojis = nil
			m.composer.focus = area.index
			m.composer.input.Blur()
			return m.mouseKey("enter")
		}
	}
	if m.composer.sending || m.composer.owner != m.account.user.ID {
		return m, nil
	}
	for _, key := range []string{"ctrl+enter", "ctrl+p", "f2", "esc"} {
		footer := m.footer(max(1, r.w-4), colors(m.account.dark))
		if x := strings.Index(footer, key); x >= 0 && (uiRect{r.x + 2 + ansi.StringWidth(footer[:x]), r.y + r.h - 2, len(key), 1}).contains(msg.X, msg.Y) {
			return m.mouseKey(key)
		}
	}
	editor := m.composerEditorBounds()
	if len(m.composer.emojis) > 0 && (uiRect{editor.x, editor.y + editor.h + 1, editor.w, 3}).contains(msg.X, msg.Y) {
		index := max(0, min(m.composer.emojiIndex-1, len(m.composer.emojis)-3)) + msg.Y - editor.y - editor.h - 1
		if index < len(m.composer.emojis) {
			m.composer.emojiIndex = index
			m.acceptEmoji()
		}
		return m, nil
	}
	if editor.contains(msg.X, msg.Y) && !m.composer.previewing {
		m.composer.focus = 0
		m.composer.input.BeginSelection(msg.X-editor.x, msg.Y-editor.y)
		m.composer.dragging = true
		m.refreshEmoji()
		return m, m.focusComposer()
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
	c.emojis = nil
}

func (m AppModel) mouseWheel(msg tea.MouseWheelMsg) (AppModel, tea.Cmd) {
	if m.composer.confirm != "" {
		return m, nil
	}
	code := tea.KeyDown
	if msg.Button == tea.MouseWheelUp {
		code = tea.KeyUp
	} else if msg.Button != tea.MouseWheelDown {
		return m, nil
	}
	if m.composer.open && m.connected && !m.accountOpen {
		if !m.composerUsable() || m.composer.sending || !m.composerBounds().contains(msg.X, msg.Y) {
			return m, nil
		}
		if !m.composer.previewing && !m.composerEditorBounds().contains(msg.X, msg.Y) && len(m.composer.emojis) == 0 {
			return m, nil
		}
		if !m.composer.previewing && len(m.composer.emojis) == 0 {
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
