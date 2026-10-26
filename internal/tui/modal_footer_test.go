package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestComposerHasOneActionFooter(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {48, 18}} {
		m := fixtureApp(t)
		m.account.width, m.account.height = size[0], size[1]
		m = pressApp(m, "c")
		view, _ := m.composerModal(colors(true))
		plain := ansi.Strip(view)
		for _, label := range []string{"Preview", "Submit", "Cancel", "ctrl+p", "ctrl+enter/ctrl+s", "esc"} {
			if strings.Count(plain, label) != 1 {
				t.Fatalf("modal should show %q once: %s", label, plain)
			}
		}
		for _, removed := range []string{"f2", "Ctrl+Enter or Ctrl+S submits", "Tab selects controls"} {
			if strings.Contains(plain, removed) {
				t.Fatalf("redundant modal hint %q", removed)
			}
		}
		m.composer.input.SetValue("Retain this draft")
		m, _ = clickText(t, m, "Cancel")
		if m.composer.confirm != "discard" {
			t.Fatal("footer Cancel click missed")
		}
	}
}

func TestDraftFooterKeepsForLaterAndBackReturnsToEditing(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Retain this draft")
	m = pressApp(m, "esc")
	view, _, _ := m.confirmationModal(colors(true))
	plain := ansi.Strip(view)
	for _, label := range []string{"Keep draft", "ctrl+k", "Discard draft", "ctrl+d", "Back", "esc"} {
		if strings.Count(plain, label) != 1 {
			t.Fatalf("confirmation should show %q once: %s", label, plain)
		}
	}
	if strings.Contains(plain, "f2") {
		t.Fatal("theme shortcut remains in confirmation")
	}
	m, _ = clickText(t, m, "Back")
	if !m.composer.open || m.composer.confirm != "" || m.composer.input.Value() != "Retain this draft" {
		t.Fatal("Back did not return to editor")
	}
	m = pressApp(m, "esc")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(AppModel)
	if m.composer.open || m.composer.confirm != "" || m.composer.input.Value() != "Retain this draft" {
		t.Fatal("Keep draft must close composer and keep text")
	}
	m = pressApp(m, "c")
	if !m.composer.open || m.composer.input.Value() != "Retain this draft" {
		t.Fatal("saved in-memory draft did not reopen")
	}
	m = pressApp(m, "esc")
	next, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(AppModel)
	if m.composer.open || m.composer.input.Value() != "" {
		t.Fatal("Ctrl+D did not discard confirmed draft")
	}
}

func TestDraftShortcutOwnershipAndRetrySafety(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("Not discarded by an editing key")
	m = pressApp(m, "ctrl+d")
	if !m.composer.open || m.composer.confirm != "" || m.composer.input.Value() == "" {
		t.Fatal("editing Ctrl+D discarded draft")
	}
	m.composer.uncertain = true
	m.startSubmission()
	view, _, _ := m.confirmationModal(colors(true))
	if !strings.Contains(ansi.Strip(view), "Submit again") || !strings.Contains(ansi.Strip(view), "duplicate") {
		t.Fatal("retry lost explicit duplicate warning")
	}
	m = pressApp(m, "ctrl+d")
	if m.composer.sending || !m.composer.uncertain {
		t.Fatal("discard shortcut retried uncertain write")
	}
	m = pressApp(m, "esc")
	if m.composer.confirm != "" || !m.composer.open {
		t.Fatal("retry Back did not keep editing")
	}
}
