package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type headlineCalls struct {
	list func(context.Context) ([]api.Headline, error)
	page func(context.Context, string) (api.HeadlinePage, error)
}

func newHeadlineCalls(client *api.Client) headlineCalls {
	return headlineCalls{client.Headlines, client.Headline}
}

// hotTopics are the web's Hot topics, shown in the aside and on the g h screen.
type hotTopics struct {
	list     []api.Headline
	open     bool
	selected int
}

// topicPage is the hot topic shown on the Starred list screen, and whether Esc goes back to the Hot topics screen.
type topicPage struct {
	topic      api.Headline
	fromTopics bool
}

type headlinesResult struct {
	session int
	topics  []api.Headline
	err     error
}

func (m AppModel) fetchHeadlines() tea.Cmd {
	if m.headlineCalls.list == nil {
		return nil
	}
	session, load, base := m.session, m.headlineCalls.list, m.account.life.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		topics, err := load(ctx)
		return headlinesResult{session: session, topics: topics, err: err}
	})
}

func (m *AppModel) acceptHeadlines(msg headlinesResult) {
	if msg.session != m.session {
		return
	}
	// A failed read keeps the topics already shown, as the web keeps its last props.
	if msg.err != nil {
		return
	}
	m.topics.list = msg.topics
	m.topics.selected = min(m.topics.selected, max(0, len(msg.topics)-1))
	m.reflow()
}

func (m AppModel) topicsOnTop() bool {
	return m.topics.open && len(m.threads) == 0 && len(m.profiles) == 0
}

func (m AppModel) topicsStatus() string {
	if len(m.topics.list) == 0 {
		return "No hot topics right now."
	}
	return fmt.Sprintf("%d / %d hot topics", m.topics.selected+1, len(m.topics.list))
}

func (m AppModel) openHotTopics() (AppModel, tea.Cmd) {
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	m.cancelNotifications()
	m.notificationsOpen = false
	m.closeStarred()
	m.closeRemotes()
	m.topics.open = true
	m.resetSequence()
	m.reflow()
	return m, m.fetchHeadlines()
}

// openTopic shows a hot topic's commits on the Starred list screen, headed by its name and recap.
func (m AppModel) openTopic(topic api.Headline, fromTopics bool) (AppModel, tea.Cmd) {
	return m.openList("", &topicPage{topic: topic, fromTopics: fromTopics})
}

func (m AppModel) leaveTopic() (AppModel, tea.Cmd) {
	if m.headline != nil && m.headline.fromTopics {
		selected := m.topics.selected
		next, cmd := m.openHotTopics()
		next.topics.selected = selected
		return next, cmd
	}
	return m.switchLog(m.active)
}

func (m AppModel) topicsKey(pressed string) (AppModel, tea.Cmd, bool) {
	t := &m.topics
	switch pressed {
	case "j", "down":
		t.selected = min(max(0, len(t.list)-1), t.selected+1)
	case "k", "up":
		t.selected = max(0, t.selected-1)
	case "home":
		t.selected = 0
	case "end", "G":
		t.selected = max(0, len(t.list)-1)
	case "esc":
		next, cmd := m.switchLog(m.lastLog)
		return next, cmd, true
	case "ctrl+r":
		return m, m.fetchHeadlines(), true
	case "enter":
		if t.selected < len(t.list) {
			next, cmd := m.openTopic(t.list[t.selected], true)
			return next, cmd, true
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

// topicsLayout draws the Hot topics screen and its click targets, relative to the center column.
func (m AppModel) topicsLayout(width int, p palette) (string, []hitArea) {
	line := text(p.hairline).Render(strings.Repeat("─", width))
	header := text(p.lanes[0]).Render(m.icons.flame) + " " + text(p.ink).Bold(true).Render("Hot topics")
	about := "What people commit about in the last 12 hours, with a short recap."
	lines := []string{"", "  " + header, line, "  " + text(p.muted).Render(fit(about, width-4)), "", line}
	var areas []hitArea
	if len(m.topics.list) == 0 {
		lines = append(lines, "", "  "+text(p.muted).Render(fit("No hot topics right now.", width-4)))
	}
	for i, topic := range m.topics.list {
		marker, style := "  ", text(p.ink)
		if i == m.topics.selected {
			marker, style = text(p.blue).Render("› "), text(p.ink).Bold(true)
		}
		y := len(lines)
		lines = append(lines, "", marker+style.Render(fit(safeText(topic.Name), width-4)))
		recap := "No recap right now."
		if topic.Summary != nil {
			recap = safeText(*topic.Summary)
		}
		for _, row := range strings.Split(ansi.Wrap(recap, width-4, ""), "\n") {
			lines = append(lines, "  "+text(p.muted).Render(row))
		}
		areas = append(areas, hitArea{uiRect{0, y, width, len(lines) - y}, "select", i})
	}
	// The header stays, and the topics below it scroll just enough to show the selected one.
	top, height := 6, m.mainHeight()
	offset := 0
	if m.topics.selected < len(areas) {
		r := areas[m.topics.selected].uiRect
		offset = max(0, min(r.y-top, r.y+r.h-height))
	}
	lines = append(lines[:top:top], lines[top+offset:]...)
	shown := areas[:0]
	for _, area := range areas {
		area.y -= offset
		if area.y >= top {
			shown = append(shown, area)
		}
	}
	areas = shown
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content), areas
}

func (m AppModel) topicsMouse(x, y, width int) (AppModel, tea.Cmd) {
	_, areas := m.topicsLayout(width, m.account.colors())
	for _, area := range areas {
		if area.contains(x, y) {
			m.topics.selected = area.index
			next, cmd, _ := m.topicsKey("enter")
			return next, cmd
		}
	}
	return m, nil
}

// hotTopicsBlock is the aside's Hot topics: a centered heading with the H picker chip, then a card per topic with its
// whole title. Recaps are on the g h screen and in the H picker.
func (m AppModel) hotTopicsBlock(width int, p palette) (string, []hitArea) {
	if len(m.topics.list) == 0 {
		return "", nil
	}
	centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	heading := text(p.lanes[0]).Render(m.icons.flame) + " " + text(p.ink).Bold(true).Render("Hot topics") + "  " + actionChip("Open", "H", p)
	rows := []string{centered.Render(heading)}
	areas := []hitArea{{uiRect{0, 0, width, 1}, "H", 0}}
	card := lipgloss.NewStyle().Width(width).Padding(0, 1).Align(lipgloss.Center).Background(lipgloss.Color(p.wash))
	for i, topic := range m.topics.list {
		rows = append(rows, "")
		top := len(rows)
		rows = append(rows, strings.Split(card.Foreground(lipgloss.Color(p.ink)).Bold(true).Render(safeText(topic.Name)), "\n")...)
		areas = append(areas, hitArea{uiRect{0, top, width, len(rows) - top}, "topic", i})
	}
	return strings.Join(rows, "\n"), areas
}

// openTopicsPicker lists the hot topics for H, like Watch for Live now.
func (m *AppModel) openTopicsPicker() tea.Cmd {
	if len(m.topics.list) == 0 {
		m.notice = "No hot topics right now."
		return nil
	}
	m.pickerGeneration++
	d := &picker{kind: "topics", title: "Hot topics", intro: "Open a topic's commits.", generation: m.pickerGeneration}
	for i, topic := range m.topics.list {
		recap := "No recap right now."
		if topic.Summary != nil {
			recap = safeText(*topic.Summary)
		}
		d.rows = append(d.rows, pickerRow{label: safeText(topic.Name), value: strconv.Itoa(i), detail: recap})
	}
	m.picker = d
	return nil
}
