package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// openRepliesPerPage is the Appearance tab's Replies row: the web's reply preview counts.
func (m *AppModel) openRepliesPerPage() {
	d := &picker{kind: "replies-per-page", title: "/dev/null · REPLIES", intro: "Reply previews under each commit in lists, like the web. Saved in tui.json."}
	for i, count := range auth.RepliesPerPageCounts {
		d.rows = append(d.rows, pickerRow{label: strconv.Itoa(count), value: strconv.Itoa(count)})
		if count == m.tuiSettings.RepliesPerPage {
			d.selected = i
		}
	}
	m.picker = d
}

// setRepliesPerPage saves a reply preview count and reads the open lists again with it, as the web reloads its pages.
func (m *AppModel) setRepliesPerPage(value string) tea.Cmd {
	count, err := strconv.Atoi(value)
	if err != nil || count == m.tuiSettings.RepliesPerPage {
		return nil
	}
	m.tuiSettings.RepliesPerPage = count
	if m.applyRepliesPerPage != nil {
		m.applyRepliesPerPage(count)
	}
	m.sizeSettings()
	cmds := []tea.Cmd{m.saveTUISettings(false)}
	for i := range m.feeds {
		if i != m.active {
			m.feeds[i].loaded = false
		}
	}
	if m.feeds[m.active].loaded {
		cmds = append(cmds, m.load("refresh"))
	}
	if m.starredOpen && m.scheduled == nil {
		m.starredRemoved = make(map[string]bool)
		cmds = append(cmds, m.fetchStarred("refresh"))
	}
	for i := range m.profiles {
		s := &m.profiles[i]
		// Other loaded tabs read again on their next visit, like the other logs.
		for tab := range s.lists {
			if !s.people && tab != s.tab && s.lists[tab].loaded {
				s.lists[tab] = profileList{cursors: make(map[string]bool)}
			}
		}
		cmds = append(cmds, m.reloadProfile(s))
	}
	return tea.Batch(cmds...)
}
