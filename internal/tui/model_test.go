package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestDemoFitsTerminalSizes(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 160, Height: 45}, {Width: 120, Height: 35}, {Width: 80, Height: 24}, {Width: 60, Height: 18}, {Width: 20, Height: 5}, {Width: 0, Height: 0}} {
		for _, theme := range []string{"light", "dark", "auto"} {
			m := New(theme)
			updated, _ := m.Update(size)
			view := updated.View()
			if !view.AltScreen {
				t.Fatal("demo should use the alternate screen")
			}
			if size.Width <= 0 || size.Height <= 0 {
				continue
			}
			if lipgloss.Width(view.Content) > size.Width || lipgloss.Height(view.Content) > size.Height {
				t.Fatalf("%s view exceeds %dx%d: %dx%d", theme, size.Width, size.Height, lipgloss.Width(view.Content), lipgloss.Height(view.Content))
			}
			if size.Width >= 60 && !strings.Contains(view.Content, "DEMO") {
				t.Fatal("demo must be labeled, including compact layouts")
			}
			if size.Width >= 60 && size.Height >= 18 && !strings.Contains(view.Content, "ctrl+c") {
				t.Fatal("demo must keep the exit hint visible at its minimum size")
			}
		}
	}
}

func TestDemoHelpAndQuoteKeyDoNotQuit(t *testing.T) {
	m := New("dark")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Fatal("q must not quit, it is reserved for quoting")
	}
	updated, _ = updated.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if !strings.Contains(updated.View().Content, "Keyboard shortcuts") {
		t.Fatal("? should open shortcut help")
	}
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(updated.View().Content, "Keyboard shortcuts") {
		t.Fatal("Escape should dismiss help")
	}
	_, cmd = updated.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not produce a quit message")
	}
}

func TestDemoProgramStartsAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var output bytes.Buffer
	p := tea.NewProgram(New("dark"), tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&output), tea.WithWindowSize(80, 24))
	finished := make(chan error, 1)
	go func() {
		_, err := p.Run()
		finished <- err
	}()
	p.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if err := <-finished; err != nil {
		t.Fatalf("demo lifecycle: %v", err)
	}
	if !strings.Contains(output.String(), "\x1b[?1049h") || !strings.Contains(output.String(), "\x1b[?1049l") {
		t.Fatal("program must enter and restore the alternate screen")
	}
}
