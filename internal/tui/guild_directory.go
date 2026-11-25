package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// guildDirectory is the Guilds screen: public guilds, biggest first, a page of 24 at a time.
type guildDirectory struct {
	open, loading, searching bool
	query, status            string
	input                    textinput.Model
	guilds                   []api.Guild
	page, lastPage, total    int
	selected, generation     int
}

type directoryResult struct {
	session, generation int
	guilds              api.Guilds
	err                 error
}

func (m AppModel) directoryOnTop() bool {
	return m.directory.open && len(m.threads) == 0 && len(m.profiles) == 0
}

func (m AppModel) openDirectory() (AppModel, tea.Cmd) {
	if m.guildCalls.directory == nil {
		return m, nil
	}
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	m.cancelNotifications()
	m.notificationsOpen = false
	m.closeStarred()
	m.closeRemotes()
	m.directory = guildDirectory{open: true, page: 1, generation: m.directory.generation + 1}
	m.resetSequence()
	m.reflow()
	return m, m.fetchDirectory(1)
}

func (m *AppModel) fetchDirectory(page int) tea.Cmd {
	d := &m.directory
	d.generation++
	d.loading, d.status = true, ""
	session, generation, query, load := m.session, d.generation, d.query, m.guildCalls.directory
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
		defer cancel()
		guilds, err := load(ctx, query, page)
		return directoryResult{session: session, generation: generation, guilds: guilds, err: err}
	})
}

func (m *AppModel) acceptDirectory(msg directoryResult) {
	d := &m.directory
	if msg.session != m.session || msg.generation != d.generation || !d.open {
		return
	}
	d.loading = false
	if msg.err != nil {
		if !m.revoked(msg.err) {
			d.status = m.ghostly(msg.err, "Couldn't load guilds: "+safeText(msg.err.Error())+". Ctrl+R to retry.")
		}
		return
	}
	d.guilds, d.page, d.lastPage, d.total, d.selected = msg.guilds.Data, msg.guilds.Page, msg.guilds.LastPage, msg.guilds.Total, 0
}

// directoryUpdated mirrors a join, leave or mute from a board on the directory's card.
func (m *AppModel) directoryUpdated(a guildAction) {
	for i := range m.directory.guilds {
		g := &m.directory.guilds[i]
		if g.Handle == a.handle {
			applyGuildAction(g, a)
		}
	}
}

func (m AppModel) directoryKey(pressed string) (AppModel, tea.Cmd, bool) {
	d := &m.directory
	if d.searching {
		switch pressed {
		case "esc":
			d.searching = false
			d.input.Blur()
		case "enter":
			d.searching, d.query = false, strings.TrimSpace(d.input.Value())
			d.input.Blur()
			return m, m.fetchDirectory(1), true
		}
		return m, nil, true
	}
	last := len(d.guilds) - 1
	columns := m.directoryColumns()
	switch pressed {
	case "j", "down":
		d.selected = min(max(0, last), d.selected+columns)
	case "k", "up":
		d.selected = max(0, d.selected-columns)
	case "l", "right":
		d.selected = min(max(0, last), d.selected+1)
	case "h", "left":
		d.selected = max(0, d.selected-1)
	case "home":
		d.selected = 0
	case "end", "G":
		d.selected = max(0, last)
	case "/":
		d.searching = true
		d.input = textinput.New()
		d.input.SetVirtualCursor(true)
		d.input.Placeholder = "Search names, #handles, and descriptions."
		d.input.CharLimit = 100
		d.input.SetValue(d.query)
		d.input.CursorEnd()
		return m, d.input.Focus(), true
	case "n", "pgdown":
		if d.page < d.lastPage && !d.loading {
			return m, m.fetchDirectory(d.page + 1), true
		}
	case "p", "pgup":
		if d.page > 1 && !d.loading {
			return m, m.fetchDirectory(d.page - 1), true
		}
	case "ctrl+r":
		return m, m.fetchDirectory(d.page), true
	case "C":
		return m, m.openGuildForm(nil), true
	case "esc":
		if d.query != "" {
			d.query = ""
			return m, m.fetchDirectory(1), true
		}
		next, cmd := m.switchLog(m.lastLog)
		return next, cmd, true
	case "enter", "J", "M":
		if d.selected > last {
			return m, nil, true
		}
		g := d.guilds[d.selected]
		switch pressed {
		case "enter":
			return m, m.openGuild(g.Handle), true
		case "J":
			return m, m.startGuildAction(guildAction{kind: "membership", handle: g.Handle, on: !g.Joined}), true
		case "M":
			if !g.Joined {
				return m, m.startGuildAction(guildAction{kind: "mute", handle: g.Handle, on: !g.Muted}), true
			}
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

// directoryInput takes typing while the search box has focus.
func (m *AppModel) directoryInput(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	m.directory.input, cmd = m.directory.input.Update(msg)
	return cmd
}

// directoryColumns is the web's grid: one card per row, two from 120 terminal columns when the center has room.
func (m AppModel) directoryColumns() int {
	if _, center, _ := m.columns(); m.account.width >= 120 && center >= 64 {
		return 2
	}
	return 1
}

func (m AppModel) directoryColumn(width int, p palette) string {
	column, _ := m.directoryLayout(width, p)
	return column
}

// directoryLayout draws the Guilds screen and its click targets, relative to the center column.
func (m AppModel) directoryLayout(width int, p palette) (string, []hitArea) {
	d := m.directory
	line := text(p.hairline).Render(strings.Repeat("─", width))
	inner := max(1, width-4)
	lines := []string{"", "  " + text(p.ink).Bold(true).Render("Guilds"), "  " + text(p.muted).Render(fit("Public groups, biggest first. Join to see their commits in Following.", inner)), line}
	var areas []hitArea
	search := text(p.muted).Render("/ ") + text(p.muted).Render("Search names, #handles, and descriptions.")
	if d.searching {
		search = text(p.blue).Render("/ ") + d.input.View()
	} else if d.query != "" {
		search = text(p.blue).Render("/ ") + text(p.ink).Render(safeText(d.query)) + text(p.muted).Render("  esc clears")
	}
	areas = append(areas, hitArea{uiRect{2, len(lines), inner, 1}, "/", -1})
	lines = append(lines, "  "+fit(search, inner))
	create := actionChip("Create a guild", "C", p)
	areas = append(areas, hitArea{uiRect{2 + inner - lipgloss.Width(create), len(lines), lipgloss.Width(create), 1}, "C", -1})
	lines = append(lines, "  "+split(text(p.muted).Render("Start a public group."), create, inner))
	pages := ""
	if d.lastPage > 1 {
		pages = fmt.Sprintf("‹ p  Page %d of %d  n ›", d.page, d.lastPage)
	}
	lines = append(lines, line, "  "+split(text(p.ink).Bold(true).Render(plural(d.total, "guild", "guilds")), text(p.muted).Render(pages), inner), "")
	switch {
	case d.status != "":
		lines = append(lines, "  "+text(p.muted).Render(fit(d.status, inner)))
	case d.loading && len(d.guilds) == 0:
		lines = append(lines, "  "+text(p.muted).Render("Reading from /dev/null..."))
	case len(d.guilds) == 0 && d.query != "":
		lines = append(lines, "  "+text(p.muted).Render(fit("No guilds match "+safeText(d.query)+".", inner)))
	case len(d.guilds) == 0:
		lines = append(lines, "  "+text(p.muted).Render("No guilds yet. Create the first one."))
	}
	columns := m.directoryColumns()
	cardWidth := (inner - 2*(columns-1)) / columns
	// Each card takes five rows and a gap, so the window starts where the selected card stays on screen.
	room := max(1, (m.mainHeight()-len(lines)-1)/6)
	firstRow := max(0, min(d.selected/columns-room+1, (len(d.guilds)+columns-1)/columns-room))
	for row := firstRow; row < firstRow+room && row*columns < len(d.guilds); row++ {
		cards := make([][]string, 0, columns)
		for column := range columns {
			i := row*columns + column
			if i >= len(d.guilds) {
				break
			}
			x := 2 + column*(cardWidth+2)
			card, cardAreas := m.guildCard(d.guilds[i], i == d.selected, cardWidth, p)
			for _, area := range cardAreas {
				area.x += x
				area.y += len(lines)
				area.index = i
				areas = append(areas, area)
			}
			areas = append(areas, hitArea{uiRect{x, len(lines), cardWidth, len(card)}, "select", i})
			cards = append(cards, card)
		}
		for r := range cards[0] {
			row := "  "
			for c, card := range cards {
				if c > 0 {
					row += "  "
				}
				row += lipgloss.NewStyle().Width(cardWidth).MaxWidth(cardWidth).Render(card[r])
			}
			lines = append(lines, row)
		}
		lines = append(lines, "")
	}
	content := lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(strings.Join(lines, "\n"))
	return content, areas
}

// guildCard is one directory card: icon, name and handle, two lines of description, then members and the buttons.
func (m AppModel) guildCard(g api.Guild, selected bool, width int, p palette) ([]string, []hitArea) {
	icon := m.guildIcon(badgeOf(g), 4, 2)
	text1 := 4 + 2
	nameStyle := text(p.ink).Bold(true)
	if selected {
		nameStyle = text(p.blue).Bold(true)
	}
	rows := []string{
		icon[0] + "  " + fit(nameStyle.Render(safeText(g.Name)), max(1, width-text1)),
		icon[1] + "  " + fit(text(p.muted).Render("#"+safeText(g.Handle)), max(1, width-text1)),
	}
	description := strings.Split(ansi.Wrap(strings.ReplaceAll(remoteBody(g.Description), "\n", " "), max(1, width), ""), "\n")
	for i := range 2 {
		value := ""
		if i < len(description) {
			value = description[i]
			if i == 1 && len(description) > 2 {
				value = fit(value+" …", width)
			}
		}
		rows = append(rows, text(p.ink).Render(fit(value, width)))
	}
	buttons, keys := membershipButtons(g, p)
	actions := strings.Join(buttons, " ")
	var areas []hitArea
	if selected {
		x := width - lipgloss.Width(actions)
		for i, button := range buttons {
			areas = append(areas, hitArea{uiRect{x, 4, lipgloss.Width(button), 1}, keys[i], 0})
			x += lipgloss.Width(button) + 1
		}
	} else {
		actions = ""
	}
	rows = append(rows, split(text(p.muted).Render(plural(g.Members, "member", "members")), actions, width))
	if selected {
		for i, row := range rows {
			rows[i] = withBackground(lipgloss.NewStyle().Width(width).Render(row), p.soft)
		}
	}
	return rows, areas
}

func (m AppModel) directoryStatus() []statusPart {
	d := m.directory
	if d.searching {
		return []statusPart{plainStatus("Search guilds"), statusChip("Search", "enter"), statusChip("Cancel", "esc")}
	}
	if d.status != "" {
		return plainStatusLine(d.status)
	}
	if d.loading {
		return plainStatusLine("Reading from /dev/null...")
	}
	parts := []statusPart{plainStatus(fmt.Sprintf("Page %d of %d", max(1, d.page), max(1, d.lastPage))), statusChip("Open", "enter"), statusChip("Join", "J"), statusChip("Search", "/"), statusChip("Create", "C")}
	if d.lastPage > 1 {
		parts = append(parts, statusChip("Next", "n"), statusChip("Previous", "p"))
	}
	return parts
}

func (m AppModel) directoryBindings(bind func(string, string) key.Binding) appKeys {
	short := []key.Binding{bind("↑/↓/←/→", "move"), bind("enter", "open board"), bind("J", "join / leave"), bind("/", "search"), bind("esc", "back to log"), bind("?", "help"), bind("ctrl+c", "quit")}
	all := append(append([]key.Binding(nil), short...), bind("M", "mute / unmute"), bind("C", "create a guild"), bind("n / p", "next / previous page"), bind("ctrl+r", "reload"), bind("home / G / end", "first / last"), bind("c", "compose"), bind("g l", "Log"), bind("f2", "choose a theme"))
	return appKeys{short, all}
}

func (m AppModel) directoryMouse(x, y, width int) (AppModel, tea.Cmd) {
	_, areas := m.directoryLayout(width, m.account.colors())
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if area.action == "select" {
			if m.directory.selected == area.index {
				return m, m.openGuild(m.directory.guilds[area.index].Handle)
			}
			m.directory.selected = area.index
			return m, nil
		}
		// Search and Create sit above the cards and leave the selection alone.
		if area.index >= 0 {
			m.directory.selected = area.index
		}
		next, cmd, _ := m.directoryKey(area.action)
		return next, cmd
	}
	return m, nil
}
