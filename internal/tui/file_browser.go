package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type browserEntry struct {
	name string
	dir  bool
}

// fileBrowser picks an image inside the composer. It lists folders first, then files with an image extension.
// The picked file is still checked by content before it is attached.
type fileBrowser struct {
	dir, filter, reason string
	all, entries        []browserEntry
	selected            int
}

func (b *fileBrowser) load(dir string) error {
	items, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var folders, files []browserEntry
	for _, item := range items {
		name := item.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.IsDir() {
			folders = append(folders, browserEntry{name + "/", true})
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".jpg", ".jpeg", ".png", ".webp":
			files = append(files, browserEntry{name, false})
		}
	}
	byName := func(a, b browserEntry) int { return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)) }
	slices.SortFunc(folders, byName)
	slices.SortFunc(files, byName)
	b.dir, b.all, b.filter, b.reason = dir, append(folders, files...), "", ""
	b.applyFilter()
	return nil
}

func (b *fileBrowser) applyFilter() {
	b.entries, b.selected = nil, 0
	query := strings.ToLower(b.filter)
	for _, entry := range b.all {
		if strings.Contains(strings.ToLower(entry.name), query) {
			b.entries = append(b.entries, entry)
		}
	}
}

func (m *AppModel) openBrowser() tea.Cmd {
	c := &m.composer
	if c.poll != nil {
		c.status = "Remove the poll to add images."
		return nil
	}
	if len(c.images) >= api.MaxImages {
		c.status = imageLimitReason
		return nil
	}
	b := &fileBrowser{}
	dir := m.imageDir
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	if err := b.load(dir); err != nil {
		c.status = "That folder cannot be read."
		return nil
	}
	c.browser, c.emojis = b, nil
	c.input.Blur()
	return nil
}

func (m *AppModel) closeBrowser() tea.Cmd {
	m.imageDir = m.composer.browser.dir
	m.composer.browser = nil
	return m.focusComposer()
}

func (m *AppModel) browserKey(msg tea.KeyPressMsg) tea.Cmd {
	b := m.composer.browser
	switch msg.String() {
	case "esc":
		return m.closeBrowser()
	case "up":
		b.selected = max(0, b.selected-1)
	case "down":
		b.selected = max(0, min(len(b.entries)-1, b.selected+1))
	case "backspace":
		if b.filter != "" {
			runes := []rune(b.filter)
			b.filter = string(runes[:len(runes)-1])
			b.applyFilter()
		} else if parent := filepath.Dir(b.dir); parent != b.dir {
			m.enterFolder(parent)
		}
	case "enter":
		if len(b.entries) == 0 {
			return nil
		}
		entry := b.entries[b.selected]
		path := filepath.Join(b.dir, strings.TrimSuffix(entry.name, "/"))
		if entry.dir {
			m.enterFolder(path)
			return nil
		}
		if reason := m.attachImages([]string{path}); reason != "" {
			b.reason = reason
			return nil
		}
		return m.closeBrowser()
	default:
		m.filterBrowser(msg.Text)
	}
	return nil
}

func (m *AppModel) enterFolder(dir string) {
	b := m.composer.browser
	if err := b.load(dir); err != nil {
		b.reason = filepath.Base(dir) + " cannot be read."
	}
}

func (m *AppModel) filterBrowser(typed string) {
	typed = ansi.Strip(typed)
	if typed == "" {
		return
	}
	b := m.composer.browser
	b.filter += typed
	b.applyFilter()
}

func (m AppModel) browserView(width, height int, p palette) string {
	b := m.composer.browser
	rows := []string{
		text(p.ink).Bold(true).Render(fit("Add an image · "+safeText(b.dir), width)),
		text(p.muted).Render(fit("Filter: "+safeText(b.filter)+"▏  ↑/↓ move · Enter open · Backspace up · Esc close", width)),
	}
	if b.reason != "" {
		rows = append(rows, text(p.danger).Render(fit(safeText(b.reason), width)))
	}
	visible := max(1, height-len(rows))
	if len(b.entries) == 0 {
		rows = append(rows, text(p.muted).Render(fit("No folders or JPEG, PNG, or WebP images here.", width)))
	}
	start := max(0, min(b.selected-visible/2, len(b.entries)-visible))
	for i := start; i < min(len(b.entries), start+visible); i++ {
		name := safeText(b.entries[i].name)
		row := "  " + name
		if i == b.selected {
			row = text(p.ink).Bold(true).Background(lipgloss.Color(p.soft)).Render(fit("› "+name, width))
		} else if b.entries[i].dir {
			row = text(p.blue).Render(row)
		}
		rows = append(rows, fit(row, width))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(rows, "\n"))
}
