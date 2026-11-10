package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

const feedHeaderHeight = 8

type appKeys struct{ short, all []key.Binding }

func (k appKeys) ShortHelp() []key.Binding  { return k.short }
func (k appKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.all} }

// bindings adds search to every screen it opens from. Remotes keeps / for its own inbox search.
func (m AppModel) bindings() appKeys {
	keys := m.screenBindings()
	// Help lists what works once it closes.
	closed := m
	closed.help = false
	if closed.searchBlocked() {
		return keys
	}
	shortcut := "ctrl+k / /"
	if m.remotesOnTop() {
		shortcut = "ctrl+k"
	}
	keys.all = append(keys.all, key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp(shortcut, "search")))
	return keys
}

func (m AppModel) screenBindings() appKeys {
	if m.settings.open || m.settings.dialog != "" {
		return m.settingsHelp()
	}
	bind := func(k, label string) key.Binding { return key.NewBinding(key.WithKeys(k), key.WithHelp(k, label)) }
	if m.composer.open {
		keys := []key.Binding{bind("ctrl+enter", "submit"), bind("ctrl+s", "submit fallback"), bind("ctrl+p", "preview / edit"), bind("ctrl+t", "schedule for later"), bind("ctrl+a", "post as"), bind("tab / shift+tab", "focus control"), bind("enter", "new line or activate control"), bind("esc", "back or cancel"), bind("ctrl+c", "request exit")}
		if m.composer.previewing {
			keys = append(keys, bind("↑/↓ / pgup/pgdown", "scroll preview"))
		}
		return appKeys{keys, keys}
	}
	if m.viewer.open {
		keys := []key.Binding{bind("←/→", "previous or next image"), bind("↑/↓ / wheel", "scroll panel"), bind("n", "more replies"), bind("p", "show or hide panel"), bind("y", "copy image link"), bind("esc", "close viewer"), bind("ctrl+c", "quit")}
		return appKeys{keys, keys}
	}
	if m.profileOnTop() {
		return m.profileBindings(bind)
	}
	if len(m.threads) > 0 {
		short := []key.Binding{bind("↑/↓", "move"), bind("r", "reply"), bind("q", "quote")}
		if c, ok := m.focusedEngagementCommit(); ok && hasProfile(c.Author) {
			short = append(short, bind("u", "profile"))
		}
		short = append(short, bind("esc", "back"), bind("?", "help"), bind("ctrl+c", "quit"))
		all := append([]key.Binding(nil), short...)
		all = append(all, bind("enter", "open selected thread"), bind("space", "show or hide replies"), bind("n", "more replies"), bind("v", "view images"), bind("D", "delete own commit"), bind("y", "copy link"), bind(".", "more commit actions"), bind("ctrl+r", "reload thread"), bind("Q", "open quoted commit"), bind("T", "copy own commit text"), bind("c", "compose"), bind("home / G / end", "first or last entry"), bind("pgup/pgdown", "move five entries"), bind("ctrl+u/ctrl+d", "move five entries"), bind("f2", "choose a theme"), bind("click", "select, open selected, act, or view image"))
		all = append(all, m.engagementBindings()...)
		return appKeys{short, all}
	}
	if m.notificationsOpen {
		short := []key.Binding{bind("↑/↓", "move"), bind("space", "expand / collapse group"), bind("enter", "open commit or follower")}
		if entries, selected := m.notificationEntries(), m.notificationFeed().selected; selected < len(entries) && hasProfile(entries[selected].notification.Actor) {
			short = append(short, bind("u", "profile"))
		}
		short = append(short, bind("ctrl+r", "refresh"), bind("esc", "back to log"), bind("?", "help"), bind("ctrl+c", "quit"))
		all := append([]key.Binding(nil), short...)
		all = append(all, bind("1–6", "toggle category"), bind("n", "older group members or notifications"), bind("r / q", "reply / quote commit"), bind("y", "copy commit link"), bind(".", "more commit actions"), bind("v", "view commit images"), bind("Q", "open quoted commit"), bind("T", "copy own commit text"), bind("c", "compose"), bind("a", "account"), bind("g n", "notifications"), bind("g l / g e", "Log / Trending"), bind("g p", "your profile"), bind("home / G / end", "first / last"), bind("pgup/pgdown", "move five entries"), bind("f2", "choose a theme"))
		return appKeys{short, all}
	}
	if m.starredOnTop() {
		return m.starredBindings(bind)
	}
	if m.draftsOnTop() {
		short := []key.Binding{bind("↑/↓", "move"), bind("enter", "open draft"), bind("D", "delete draft"), bind("esc", "back to log"), bind("?", "help"), bind("ctrl+c", "quit")}
		all := append(append([]key.Binding(nil), short...), bind("home / G / end", "first / last"), bind("c", "compose"), bind("g l", "Log"), bind("f2", "choose a theme"))
		return appKeys{short, all}
	}
	if m.remotesOnTop() {
		return m.remotesBindings(bind)
	}
	short := []key.Binding{bind("↑/↓", "move"), bind("tab", "log"), bind("c", "compose"), bind("ctrl+r", "refresh")}
	if len(m.feeds[m.active].commits) > 0 && m.account.width >= 100 {
		short = append(short, bind("y", "copy link"))
	}
	short = append(short, bind("f2", "choose a theme"), bind("?", "help"), bind("ctrl+c", "quit"))
	all := append([]key.Binding(nil), short...)
	all = append(all, bind("g n", "notifications"), bind("g p", "your profile"), bind("g s", "Starred"), bind("L", "go live / end your stream"))
	all = append(all, bind("enter", "open thread"), bind("u", "author profile"), bind("r", "reply"), bind("q", "quote"), bind("v", "view images"), bind("D", "delete own commit"), bind(".", "more commit actions"), bind("a", "account"), bind("Q", "open quoted commit"), bind("T", "copy own commit text"), bind("1/2/3", "choose log"), bind("w", "Who to follow"), bind("f / X / R", "on a suggested person: follow, not interested, others"), bind("g l", "return to log"), bind("g e", "Trending"), bind("g g / home", "first commit"), bind("G / end", "last commit"), bind("pgup/pgdown", "move five commits"), bind("ctrl+u/ctrl+d", "move five commits"), bind("] / l / right", "next log"), bind("[ / shift+tab", "previous log"), bind("wheel", "move selection"), bind("click", "open thread or view image"), bind("esc", "close overlay"))
	if len(m.feeds[m.active].commits) > 0 && m.account.width < 100 {
		all = append(all, bind("y", "copy link"))
	}
	if m.feeds[m.active].next != "" {
		all = append(all, bind("n", "older commits"))
	}
	all = append(all, m.engagementBindings()...)
	all = append(all, bind("g ,", "settings"))
	return appKeys{short, all}
}

func (m AppModel) engagementBindings() []key.Binding {
	bindings := []key.Binding{key.NewBinding(key.WithKeys("g t"), key.WithHelp("g t", "scheduled commits")), key.NewBinding(key.WithKeys("g d"), key.WithHelp("g d", "drafts"))}
	if m.undo.id != "" {
		bindings = append(bindings, key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "undo the delete")))
	}
	if len(m.liveNow) > 0 {
		bindings = append(bindings, key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "watch someone live now")))
	}
	c, ok := m.focusedEngagementCommit()
	if !ok {
		return bindings
	}
	if len(m.commitLinks(c)) > 0 {
		bindings = append(bindings, key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "open a link in the commit")))
	}
	if strings.TrimSpace(c.Body) != "" && c.DeletedAt == nil {
		bindings = append(bindings, key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "translate / show original")))
	}
	if c.Stars > 0 && c.DeletedAt == nil {
		bindings = append(bindings, key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "who starred it")))
	}
	if m.canPinCommit(c) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "pin / unpin on your profile")))
	}
	if m.canStar(c) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "star / unstar selected commit")))
	}
	if m.canGivePastel(c) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "give selected commit a pastél")))
	}
	if folded(c, m.statsStyle()) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "show the whole commit")))
	}
	if m.canVote(c) {
		bindings = append(bindings, key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "vote on the poll")))
	}
	if c.Author != nil && !m.ownCommit(c) && c.DeletedAt == nil && (c.Status == "" || c.Status == "approved") {
		bindings = append(bindings, key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "report commit")))
	}
	return bindings
}

// actionChip is the one look for an action you can click or press: its label on the hairline, then its key in blue.
func actionChip(label, key string, p palette) string {
	return chipStyled(label, key, p.ink, p)
}

// dangerChip is actionChip for a destructive action, such as Block.
func dangerChip(label, key string, p palette) string {
	return chipStyled(label, key, p.danger, p)
}

func chipStyled(label, key, ink string, p palette) string {
	labelStyle, keyStyle := chipStyles(ink, p)
	return labelStyle.Render(" "+label+" ") + keyStyle.Render(" "+key+" ")
}

// wideChip is actionChip stretched to width, with the label on the left and the key at the right edge.
func wideChip(label, key string, width int, p palette) string {
	labelStyle, keyStyle := chipStyles(p.ink, p)
	keyPart := keyStyle.Render(" " + key + " ")
	room := max(1, width-lipgloss.Width(keyPart))
	label = " " + fit(label, max(0, room-2))
	return labelStyle.Render(label+strings.Repeat(" ", max(0, room-ansi.StringWidth(label)))) + keyPart
}

func chipStyles(ink string, p palette) (label, key lipgloss.Style) {
	label = lipgloss.NewStyle().Foreground(lipgloss.Color(ink)).Background(lipgloss.Color(p.hairline))
	key = lipgloss.NewStyle().Foreground(lipgloss.Color(p.blue)).Background(lipgloss.Color(p.soft)).Bold(true)
	return label, key
}

// keyChip names a key in shortcut lists and hints. Actions use actionChip.
func keyChip(value string, p palette) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.wash)).Render(" " + value + " ")
}

func filledButton(label, shortcut string, width int, background, foreground string) string {
	content := label
	if shortcut != "" {
		content += strings.Repeat(" ", max(1, width-2-ansi.StringWidth(label)-len(shortcut))) + shortcut
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(background)).Foreground(lipgloss.Color(foreground)).Bold(true).Width(width).Padding(0, 1).Align(lipgloss.Center).Render(content)
}

func (m AppModel) View() tea.View {
	v := m.workspaceView()
	p := m.account.colors()
	v.Content = m.haunt(v.Content, p)
	if m.viewer.open && m.connected && !m.accountOpen && m.account.width >= 30 && m.account.height >= 10 {
		v.Content, _ = m.viewerLayout(p)
	}
	if m.deleting.id != "" && m.connected && !m.accountOpen {
		content, r, _ := m.deleteModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.picker != nil && m.connected && !m.accountOpen {
		content, r, _ := m.listPickerModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.whoToFollow.open && m.connected && !m.accountOpen {
		content, r, _ := m.suggestionModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.ask != nil && m.connected && !m.accountOpen {
		content, r, _ := m.askModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.live != nil && m.connected && !m.accountOpen {
		content, r, _ := m.liveModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.remotes.dialog != "" && m.remotesOnTop() && m.connected && !m.accountOpen {
		content, r, _ := m.remotesModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.remotes.members != nil && m.remotes.thread != nil && m.remotesOnTop() && m.connected && !m.accountOpen {
		content, r, _ := m.membersModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.remotes.thread != nil && m.remotesOnTop() && m.connected && !m.accountOpen {
		var content string
		var r uiRect
		switch {
		case m.remotes.pinWarning != nil:
			content, r, _ = m.pinWarningModal(p)
		case m.remotes.pinList != nil:
			content, r, _ = m.pinListModal(p)
		}
		if content != "" {
			v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
		}
	}
	if m.remotes.people != nil && (m.remotesOnTop() || m.sharingCommit()) && m.connected && !m.accountOpen {
		content, r, _ := m.peopleModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if menu := m.activeReactions(); menu != nil && menu.picking {
		content, r, _ := m.reactionModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.vote.confirm && m.connected && !m.accountOpen {
		content, r, _ := m.voteModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.achievements != nil && m.connected && !m.accountOpen {
		content, r, _ := m.achievementsModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.search != nil && m.connected && !m.accountOpen {
		content, r, _ := m.searchModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.composer.open && m.connected && !m.accountOpen {
		content, r := m.composerModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.composer.confirm != "" {
		content, r, _ := m.confirmationModal(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.connected && !m.accountOpen && m.settings.dialog != "" {
		content, r, _ := m.settingsDialog(p)
		v.Content = overlay(v.Content, content, r, p, m.account.width, m.account.height)
	}
	if m.toast.visible && m.composer.confirm == "" && m.settings.dialog == "" && m.account.width >= 20 && m.account.height >= 8 {
		content, r := m.toastView()
		v.Content = m.overlayToast(v.Content, content, r)
	}
	if m.connected && !m.accountOpen && m.account.width > 0 && m.account.height > 0 {
		// Overlays and the viewer bypass canvas, so repaint whatever they reset.
		v.Content = withBackground(v.Content, p.shell)
	}
	v.Content = hashtagMarker.ReplaceAllString(v.Content, "")
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m AppModel) workspaceView() tea.View {
	if !m.connected || m.accountOpen {
		v := m.account.View()
		v.ReportFocus = true
		return v
	}
	w, h := m.account.width, m.account.height
	if w <= 0 || h <= 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}
	p := m.account.colors()
	var page string
	if w < 30 || h < 10 {
		page = "/dev/null needs a bigger terminal."
	} else {
		nav, center, aside := m.columns()
		total := nav + center + aside + 2
		main := m.feedColumn(center, p)
		if m.starredOpen && m.manualLink == "" {
			main = m.starredColumn(center, p)
		}
		if m.remotes.open && m.manualLink == "" {
			main = m.remotesColumn(m.remotesWidth(), p)
		}
		if m.notificationsOpen && m.manualLink == "" {
			main = m.notificationColumn(center, p)
		}
		if m.drafts.open && m.manualLink == "" {
			main = m.draftsColumn(center, p)
		}
		if m.threadOnTop() && m.manualLink == "" {
			main = m.threadColumn(center, p)
		}
		if m.profileOnTop() && m.manualLink == "" {
			main = m.profileColumn(center, p)
		}
		if m.settings.open {
			main = m.settingsColumn(center, p)
		}
		rail := text(p.hairline).Render(strings.TrimSuffix(strings.Repeat("│\n", m.mainHeight()), "\n"))
		main = lipgloss.JoinHorizontal(lipgloss.Top, m.navigation(nav, m.mainHeight(), p), rail, main, rail)
		if aside > 0 && !m.remotesWide() {
			main = lipgloss.JoinHorizontal(lipgloss.Top, main, m.aside(aside, m.mainHeight(), p))
		}
		footer, _ := m.workspaceFooter(w, p)
		page = lipgloss.NewStyle().MarginLeft((w-total)/2).Render(main) + "\n" + footer
	}
	page = canvas(page, p, w, h)
	if m.help && w >= 30 && h >= 10 {
		modal := m.helpModal(p)
		page = overlay(page, modal, m.helpBounds(), p, w, h)
	}
	if m.whatsNew.open && m.connected && !m.accountOpen && w >= 30 && h >= 10 {
		page = overlay(page, m.whatsNewModal(p), m.whatsNewBounds(), p, w, h)
	}
	if m.upgradeVisible() && w >= 30 && h >= 10 {
		page = overlay(page, m.upgradeModal(p), m.upgradeBounds(), p, w, h)
	}
	v := tea.NewView(page)
	v.AltScreen, v.ReportFocus = true, true
	v.MouseMode = tea.MouseModeCellMotion
	if m.account.profile >= colorprofile.ANSI && m.account.paintsBackground() {
		v.BackgroundColor = lipgloss.Color(p.shell)
		v.ForegroundColor = lipgloss.Color(p.ink)
	}
	return v
}

func (m AppModel) mainHeight() int {
	return max(1, m.account.height-1)
}

func logoRequest(dark bool, event string) graphics.Request {
	kind := "logo-light"
	if dark {
		kind = "logo-dark"
	}
	if event == "christmas" {
		kind += "-santa"
	}
	return graphics.Request{Icon: kind, Cols: 23, Rows: 2}
}

func (m AppModel) brand(p palette) string {
	req := logoRequest(m.account.isDark(), m.account.event)
	if top, bottom := m.images.Row(req, 0), m.images.Row(req, 1); top != "" && bottom != "" {
		return top + "\n" + bottom
	}
	return text(p.lanes[0]).Bold(true).Render("//") + "  " + text(p.ink).Bold(true).Render("/dev/null")
}

// workspaceFooter is the open screen's status. Refresh lives in the aside's Actions, so the footer doesn't repeat it.
func (m AppModel) workspaceFooter(width int, p palette) (string, []hitArea) {
	return statusLine(m.statusParts(), width, p)
}

func (m AppModel) feedTabs(width int, p palette) string {
	var labels, underline strings.Builder
	for i, name := range logNames {
		cellWidth := width / 3
		if i == 2 {
			cellWidth = width - 2*(width/3)
		}
		label := name
		if i == m.active {
			label = text(p.ink).Bold(true).Render(label)
		} else {
			label = text(p.muted).Render(label)
		}
		labels.WriteString(lipgloss.NewStyle().Width(cellWidth).Align(lipgloss.Center).Render(label))
		line := strings.Repeat(" ", cellWidth)
		if i == m.active {
			mark := min(len(name)+2, cellWidth)
			pad := (cellWidth - mark) / 2
			line = strings.Repeat(" ", pad) + text(p.blue).Render(strings.Repeat("━", mark)) + strings.Repeat(" ", cellWidth-pad-mark)
		}
		underline.WriteString(line)
	}
	return labels.String() + "\n" + underline.String()
}

func (m AppModel) feedColumn(width int, p palette) string {
	height := m.mainHeight()
	var content string
	switch {
	case m.manualLink != "":
		content, _ = m.manualLinkView(width, p)
	default:
		f := m.feeds[m.active]
		user := m.account.user
		avatar := m.images.Row(m.avatarRequest(user.AvatarURL, laneColor(user.Lane, p), 2, 1), 0)
		if avatar == "" {
			avatar = text(laneColor(user.Lane, p)).Render("◯ ")
		}
		reader := "  " + avatar + " " + text(p.muted).Render("What did you ship?")
		refresh := actionChip("Commit", "c", p)
		reader = fit(reader, max(1, width-lipgloss.Width(refresh)-2)) + strings.Repeat(" ", max(1, width-lipgloss.Width(reader)-lipgloss.Width(refresh)-1)) + refresh
		hairline := text(p.hairline).Render(strings.Repeat("─", width))
		content = "\n" + m.feedTabs(width, p) + "\n" + hairline + "\n\n" + reader + "\n\n" + hairline + "\n"
		if !f.loaded {
			title, detail := "Reading your log...", "Fetching commits with your signed connection."
			if !f.loading && f.status != "" {
				title, detail = "Couldn't load this log", f.status
			}
			placeholder := "\n\n" + text(p.ink).Bold(true).Render(title) + "\n\n" + text(p.muted).Render(detail)
			content += lipgloss.NewStyle().Width(max(1, width-12)).MarginLeft(6).Render(placeholder)
		} else {
			feed := m.feedViewport(p)
			// The New commits notice floats over the first row, under the tabs, like the web's sticky button.
			if pill, at := m.arrivalsPill(width, p); pill != "" {
				rows := strings.Split(feed, "\n")
				for i, row := range strings.Split(pill, "\n") {
					if i < len(rows) {
						rows[i] = ansi.Cut(rows[i], 0, at) + row + ansi.Cut(rows[i], at+lipgloss.Width(row), width)
					}
				}
				feed = strings.Join(rows, "\n")
			}
			content += feed
		}
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(content)
}

func (m AppModel) navigation(width, height int, p palette) string {
	column, _ := m.navigationLayout(width, height, p)
	return column
}

func (m AppModel) navigationLayout(width, height int, p palette) (string, []hitArea) {
	inner := width - 2
	// Rows follow the web sidebar: muted icon and label, with the open screen in blue on a soft fill.
	navRow := func(icon, label, badge, hint string, active bool) string {
		color := p.muted
		if active {
			color = p.blue
		}
		style := text(color).Bold(active)
		if width < 12 {
			if badge != "" {
				icon += text(p.blue).Render("•")
			}
			line := lipgloss.NewStyle().Width(inner).Align(lipgloss.Center).Render(style.Render(icon))
			if active {
				return withBackground(line, p.soft)
			}
			return line
		}
		if badge != "" {
			label += " " + text(p.blue).Bold(true).Render(badge)
		}
		// A narrow sidebar drops the icon first, then the hint, so an unread badge never hides the shortcut.
		left := style.Render(icon+"  ") + style.Render(label)
		if lipgloss.Width(left)+len(hint)+3 > inner {
			left = style.Render(label)
		}
		if lipgloss.Width(left)+len(hint)+3 > inner {
			hint = ""
		}
		left = fit(left, max(1, inner-len(hint)-3))
		line := " " + left + strings.Repeat(" ", max(1, inner-lipgloss.Width(left)-len(hint)-2)) + text(color).Faint(!active).Render(hint) + " "
		if active {
			return withBackground(fit(line, inner), p.soft)
		}
		return fit(line, inner)
	}
	logo := lipgloss.PlaceHorizontal(width, lipgloss.Center, m.brand(p))
	if width < 12 {
		logo = " //"
	}
	logo = lipgloss.NewStyle().Height(2).Render(logo)
	badge := countBadge(m.unreadNotifications)
	ownProfile := !m.settings.open && m.profileOnTop() && strings.EqualFold(m.profileHandle(), m.account.user.Handle)
	idle := !m.settings.open && !m.profileOnTop()
	type navEntry struct {
		icon, label, badge, hint, action string
		active                           bool
	}
	entries := []navEntry{
		{m.icons.log, "Log", "", "g l", "log", idle && !m.notificationsOpen && !m.starredOpen && !m.remotes.open && !m.drafts.open},
		{m.icons.notifications, "Notifications", badge, "g n", "notifications", idle && m.notificationsOpen},
	}
	// The web's sidebar shows Remotes only to accounts that have them, between Notifications and Profile.
	if m.remotesEnabled() {
		entries = append(entries, navEntry{m.icons.remotes, "Remotes", countBadge(m.remotes.unread), "g r", "remotes", idle && m.remotes.open})
	}
	entries = append(entries, navEntry{m.icons.profile, "Profile", "", "g p", "profile", ownProfile}, navEntry{m.icons.starred, "Starred", "", "g s", "starred", idle && m.starredOpen && m.commitSearch == "" && m.scheduled == nil}, navEntry{m.icons.scheduled, "Scheduled", "", "g t", "scheduled", idle && m.scheduled != nil}, navEntry{m.icons.drafts, "Drafts", draftCount(len(m.commitDrafts)), "g d", "drafts", idle && m.drafts.open}, navEntry{m.icons.settings, "Settings", "", "g ,", "settings", m.settings.open})
	top := "\n" + logo
	var areas []hitArea
	for i, entry := range entries {
		top += "\n\n " + navRow(entry.icon, entry.label, entry.badge, entry.hint, entry.active)
		areas = append(areas, hitArea{uiRect{1, 4 + 2*i, inner, 1}, entry.action, 0})
	}
	compose, shortcut := m.icons.commit+"  Commit", "c"
	if width < 12 {
		compose, shortcut = m.icons.commit, ""
	}
	button := filledButton(compose, shortcut, inner, p.blue, p.shell)
	// A blank row sets the button apart from Settings, and it grows a row of fill above and below when there is room.
	if width >= 12 && lipgloss.Height(top)+2+3+1+2 <= height {
		fill := lipgloss.NewStyle().Background(lipgloss.Color(p.blue)).Width(inner).Render("")
		button = fill + "\n" + button + "\n" + fill
	}
	buttonY := lipgloss.Height(top) + 2
	buttonX := (width - lipgloss.Width(button)) / 2
	top += "\n\n\n" + lipgloss.PlaceHorizontal(width, lipgloss.Center, button)
	areas = append(areas, hitArea{uiRect{buttonX, buttonY, lipgloss.Width(button), lipgloss.Height(button)}, "c", 0})
	user := m.account.user
	lane := laneColor(user.Lane, p)
	avatar := lipgloss.NewStyle().Width(4).Height(2).Render(text(lane).Render("●"))
	req := m.avatarRequest(user.AvatarURL, liveRing(user.Live, lane, p, m.now), 4, 2)
	if top, bottom := m.images.Row(req, 0), m.images.Row(req, 1); top != "" && bottom != "" {
		avatar = top + "\n" + bottom
	}
	bottom := lipgloss.PlaceHorizontal(width, lipgloss.Center, avatar)
	if width >= 12 {
		name := safeText(user.Name)
		if name == "" {
			name = safeText(user.Handle)
		}
		identity := text(p.ink).Bold(true).Render(fit(name, inner-5)) + "\n" + text(p.muted).Render(fit("@"+safeText(user.Handle), inner-5))
		bottom = lipgloss.NewStyle().PaddingLeft(1).Render(lipgloss.JoinHorizontal(lipgloss.Top, avatar, " ", identity))
		// Your counts sit above your identity, one per line, dropping from the end when the column is short.
		stats := m.accountStats(width, p)
		for len(stats) > 0 && lipgloss.Height(top)+len(stats)+1+lipgloss.Height(bottom) >= height {
			stats = stats[:len(stats)-1]
		}
		if len(stats) > 0 {
			bottom = strings.Join(stats, "\n") + "\n\n" + bottom
		}
	}
	gap := max(1, height-lipgloss.Height(top)-lipgloss.Height(bottom))
	content := top + strings.Repeat("\n", gap) + bottom + "\n"
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).MaxWidth(width).Render(content), areas
}

// accountStats are the sidebar's centered lines for your counts, pastéis, and live stream. A count the server
// didn't return is left out rather than shown as zero.
func (m AppModel) accountStats(width int, p palette) []string {
	u := m.account.user
	centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	var stats []string
	for _, stat := range []struct {
		value     *int
		one, many string
	}{{u.Commits, "commit", "commits"}, {u.Following, "following", "following"}, {u.Followers, "follower", "followers"}} {
		if stat.value != nil {
			label := strings.SplitN(plural(*stat.value, stat.one, stat.many), " ", 2)
			stats = append(stats, centered.Render(text(p.ink).Bold(true).Render(label[0])+text(p.muted).Render(" "+label[1])))
		}
	}
	if u.PasteisMode && u.PasteisRemaining != nil {
		label := fmt.Sprintf("%d pastéis left today", *u.PasteisRemaining)
		stats = append(stats, lipgloss.PlaceHorizontal(width, lipgloss.Center, pastelLabel(label, min(width, lipgloss.Width(pastelIcon(p, m.images))+1+len([]rune(label))), p, m.images)))
	}
	if u.Live.On(m.now) {
		stats = append(stats, centered.Render(liveLabel(u.Live, p)), centered.Render(text(p.muted).Render(fit("until "+liveUntil(u.Live), width))))
	}
	return stats
}

func (m AppModel) manualLinkView(width int, p palette) (string, hitArea) {
	noun := m.manualNoun()
	head := "  " + text(p.ink).Bold(true).Render("Copy commit "+noun) + "\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n\n  "
	block := lipgloss.NewStyle().Width(width - 4).MarginLeft(2)
	content := head + "Clipboard unavailable. Copy this " + noun + ":\n\n" + block.Render(safeLines(m.manualLink))
	// The column doesn't scroll, so text taller than it points to the commit's page instead, with Back still shown.
	if m.manualText && lipgloss.Height(content)+2 > m.mainHeight() {
		content = head + block.Render("Clipboard unavailable, and this commit is too long to show here. Copy it from its page:")
		if m.manualPage != "" {
			content += "\n\n" + block.Render(m.manualPage)
		}
	}
	back := "  " + actionChip("Back to log", "esc", p)
	area := hitArea{uiRect{2, lipgloss.Height(content) + 1, min(width-2, lipgloss.Width(back)-2), 1}, "esc", 0}
	return content + "\n\n" + back, area
}

func (m AppModel) aside(width, height int, p palette) string {
	column, _ := m.asideLayout(width, height, p)
	return column
}

// asideFit says how much of the aside to draw: how many shortcuts (-1 for all), and whether the version shows.
type asideFit struct {
	shortcuts int
	version   bool
}

// asideLayout fits the aside to its height. It drops the version first, then shortcuts from the bottom, because the
// aside doesn't scroll and anything below the bottom is cut off.
func (m AppModel) asideLayout(width, height int, p palette) (string, []hitArea) {
	try := func(layout asideFit) (string, []hitArea, int, int) {
		content, areas, shortcuts := m.asideAt(width, layout, p)
		return content, areas, strings.Count(content, "\n") + 1, shortcuts
	}
	layout := asideFit{shortcuts: -1, version: true}
	content, areas, rows, _ := try(layout)
	if rows > height {
		layout.version = false
		var shortcuts int
		content, areas, rows, shortcuts = try(layout)
		if rows > height {
			// The section's rule and heading go too when fewer than two shortcuts fit.
			layout.shortcuts = shortcuts - (rows - height)
			if layout.shortcuts < 2 {
				layout.shortcuts = 0
			}
			content, areas, _, _ = try(layout)
		}
	}
	return m.asideBox(content, width, height), areas
}

func (m AppModel) asideBox(content string, width, height int) string {
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).MaxWidth(width).PaddingLeft(2).PaddingRight(1).Render(content)
}

// asideAt draws the aside at one fit, returning how many shortcuts exist.
func (m AppModel) asideAt(width int, layout asideFit, p palette) (string, []hitArea, int) {
	var areas []hitArea
	inner := width - 3
	centered := lipgloss.NewStyle().Width(inner).Align(lipgloss.Center)
	content := "\n"
	line := text(p.hairline).Render(strings.Repeat("─", inner))
	search := searchRow(inner, p)
	areas = append(areas, hitArea{uiRect{2, 0, inner, lipgloss.Height(search)}, "ctrl+k", 0})
	controls := search + "\n"
	if greeting := m.newYearGreeting(inner, p); greeting != "" {
		controls += "\n" + greeting + "\n"
	}
	live, watch := m.liveNowBlock(inner, p)
	if live != "" {
		top := strings.Count(controls, "\n") + 1
		for _, area := range watch {
			area.x += 2
			area.y += top
			areas = append(areas, area)
		}
		controls += line + "\n" + live + "\n"
	}
	// A rule separates Actions from the blocks above it, and a blank line from the search box.
	if live != "" {
		controls += line + "\n"
	} else {
		controls += "\n"
	}
	controls += text(p.ink).Bold(true).Width(inner).Align(lipgloss.Center).Render("Actions") + "\n"
	buttons := []struct{ label, key string }{{"Account", "a"}, {m.liveButton(), "L"}}
	if !m.suggestionsHidden() && m.suggestCalls.list != nil {
		buttons = append(buttons, struct{ label, key string }{"Who to follow", "w"})
	}
	buttons = append(buttons, struct{ label, key string }{"Theme", "f2"}, struct{ label, key string }{"Refresh", "ctrl+r"})
	for i, action := range buttons {
		if i > 0 {
			controls += "\n"
		}
		areas = append(areas, hitArea{uiRect{2, strings.Count(controls, "\n"), inner, 1}, action.key, 0})
		controls += wideChip(action.label, action.key, inner, p)
	}
	workspace := m
	workspace.composer.open = false
	var shortcuts []key.Binding
	for _, binding := range workspace.bindings().short {
		if h := binding.Help(); h.Key != "c" && h.Key != "ctrl+r" && h.Key != "a" && h.Key != "f2" {
			shortcuts = append(shortcuts, binding)
		}
	}
	shown := shortcuts
	if layout.shortcuts >= 0 {
		shown = shortcuts[:min(layout.shortcuts, len(shortcuts))]
	}
	if len(shown) > 0 {
		controls += "\n" + line + "\n" + text(p.ink).Bold(true).Width(inner).Align(lipgloss.Center).Render("Shortcuts")
	}
	controls += "\n"
	for _, binding := range shown {
		h := binding.Help()
		// A pair of arrows moves down or forward, like the other pair chips.
		action := map[string]string{"↑/↓": "down", "←/→": "right"}[h.Key]
		if action == "" {
			action = h.Key
		}
		areas = append(areas, hitArea{uiRect{2, strings.Count(controls, "\n"), inner, 1}, action, 0})
		controls += wideChip(h.Desc, h.Key, inner, p) + "\n"
	}
	version := m.version
	if version == "" {
		version = release.Version
	}
	if layout.version {
		controls += line + "\n\n" + centered.Foreground(lipgloss.Color(p.muted)).Render("/dev/null TUI\n"+fit(safeText(version), inner))
	}
	for i := range areas {
		areas[i].y += strings.Count(content, "\n")
	}
	content += strings.TrimRight(controls, "\n")
	return content, areas, len(shortcuts)
}

func (m AppModel) feedViewport(p palette) string {
	f := m.feeds[m.active]
	view := f.viewport.View()
	height, total, width := f.viewport.Height(), f.viewport.TotalLineCount(), f.viewport.Width()
	if total <= height || height <= 2 || width <= 1 {
		return view
	}
	lines := strings.Split(view, "\n")
	thumb := max(1, height*height/total)
	top := f.viewport.YOffset() * (height - thumb) / max(1, total-height)
	for i := top; i < min(len(lines), top+thumb); i++ {
		lines[i] = ansi.Cut(lines[i], 0, width-1) + text(p.hairline).Render("▐")
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) helpLines(width int, p palette) []string {
	lines := []string{text(p.muted).Render("Log, thread, and composer controls."), ""}
	for _, binding := range m.bindings().all {
		h := binding.Help()
		chip := keyChip(h.Key, p)
		label := fit(h.Desc, max(1, width-lipgloss.Width(chip)-1))
		lines = append(lines, label+strings.Repeat(" ", max(1, width-lipgloss.Width(chip)-lipgloss.Width(label)))+chip)
	}
	return lines
}

// scrollFooter is the Scroll and Close chips of a scrolling panel such as help, with their hit areas on screen.
func scrollFooter(r uiRect, p palette) (string, []hitArea) {
	footer, areas := hintRow([]statusPart{hintChip("Scroll", "↑/↓", "down"), closeChip("Close")}, max(1, r.w-4), p)
	return footer, panelFooterAreas(areas, footer, r)
}

func (m AppModel) helpMaxScroll() int {
	r := m.helpBounds()
	return max(0, len(m.helpLines(max(1, r.w-4), m.account.colors()))-max(1, r.h-6))
}

func (m AppModel) helpBounds() uiRect { return centeredRect(m.account.width, m.account.height, 62, 48) }

func (m AppModel) helpModal(p palette) string {
	r := m.helpBounds()
	lines := m.helpLines(max(1, r.w-4), p)
	start := min(m.helpScroll, m.helpMaxScroll())
	content := strings.Join(lines[start:min(len(lines), start+max(1, r.h-6))], "\n")
	footer, _ := scrollFooter(r, p)
	return panel("Keyboard shortcuts", content, footer, r, p, p.blue)
}

func countBadge(count int) string {
	switch {
	case count > 99:
		return "99+"
	case count > 0:
		return fmt.Sprint(count)
	}
	return ""
}

// remotesWide reports Remotes drawn across the center column and the aside.
func (m AppModel) remotesWide() bool {
	return m.remotesOnTop() && m.manualLink == "" && !m.settings.open
}
