package tui

import "charm.land/bubbles/v2/viewport"

// roomToJump adds blank lines after the last item so Z can scroll even the last one to the top.
func roomToJump(lines []string, starts []int, height int) []string {
	if len(starts) == 0 {
		return lines
	}
	missing := height - (len(lines) - starts[len(starts)-1])
	for range max(0, missing) {
		lines = append(lines, "")
	}
	return lines
}

// jumpToTop is Z: the focused item of the open list moves to the top of the screen.
func (m *AppModel) jumpToTop() {
	scroll := func(vp *viewport.Model, starts []int, selected int) {
		if selected >= 0 && selected < len(starts) {
			vp.SetYOffset(starts[selected])
		}
	}
	switch {
	case m.threadOnTop():
		t := &m.threads[len(m.threads)-1]
		scroll(&t.viewport, t.starts, t.selected)
	case m.profileOnTop():
		s := m.topProfile()
		scroll(&s.viewport, s.starts, s.lists[s.tab].selected)
	case m.notificationsOpen:
		f := m.notificationFeed()
		scroll(&f.viewport, f.starts, f.selected)
	case m.starredOpen:
		scroll(&m.starred.viewport, m.starred.starts, m.starred.selected)
	case m.directoryOnTop(), m.topicsOnTop(), m.draftsOnTop():
		// These screens have no commits to lift, and the Log under them must stay where it was.
	default:
		f := &m.feeds[m.active]
		scroll(&f.viewport, f.starts, f.selected)
	}
}
