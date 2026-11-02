package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

func TestWhatsNewShowsOnlyAfterAnUpgrade(t *testing.T) {
	for _, tc := range []struct {
		name, current, seen string
		connected           bool
		show, record        bool
	}{
		{"fresh install", "v0.2.0", "", false, false, true},
		{"upgrade from a version without the feature", "v0.2.0", "", true, true, false},
		{"upgrade", "v0.2.0", "v0.1.2", true, true, false},
		{"same version", "v0.2.0", "v0.2.0", true, false, false},
		{"downgrade", "v0.2.0", "v0.3.0", true, false, false},
		{"corrupt state with a connection", "v0.2.0", "garbage", true, true, false},
		{"release without a summary", "v9.9.9", "v0.1.2", true, false, true},
		{"development build", "dev", "v0.1.2", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			show, record := whatsNewDecision(tc.current, tc.seen, tc.connected)
			if show != tc.show || record != tc.record {
				t.Fatalf("show=%v record=%v", show, record)
			}
		})
	}
}

func upgradedApp(t *testing.T, seen string) (AppModel, auth.Store) {
	t.Helper()
	store := auth.Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	key, _ := auth.GenerateKey()
	if err := store.Save(key, "https://example.test"); err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		if err := store.SaveSeenVersion(seen); err != nil {
			t.Fatal(err)
		}
	}
	m := NewApp("dark", store, "https://example.test", t.Context()).WithVersion("v0.2.0")
	t.Cleanup(func() { _ = m.Close() })
	m.account.width, m.account.height = 120, 35
	next, _ := m.Update(accountResult{key: key, user: auth.User{ID: "fixture", Handle: "fixture", Name: "Fixture"}})
	return next.(AppModel), store
}

func TestWhatsNewOpensOnceAndRecordsTheVersionWhenClosed(t *testing.T) {
	m, store := upgradedApp(t, "v0.1.2")
	if !m.whatsNew.open || !strings.Contains(ansi.Strip(m.View().Content), "What's new in v0.2.0") {
		t.Fatal("upgrade did not show What's new")
	}
	if store.LastSeenVersion() != "v0.1.2" {
		t.Fatal("version recorded before the modal was closed")
	}
	for _, key := range []string{"c", "?", "g"} {
		m = pressApp(m, key)
	}
	if m.composer.open || m.help || m.pendingG {
		t.Fatal("keys leaked past What's new")
	}
	m = pressApp(m, "end")
	if m.whatsNew.scroll != m.whatsNewMaxScroll() {
		t.Fatal("End must reach the last line")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(AppModel)
	if m.whatsNew.open || cmd == nil {
		t.Fatal("Esc must close What's new and record the version")
	}
	next, _ = m.Update(cmd())
	m = next.(AppModel)
	if store.LastSeenVersion() != "v0.2.0" {
		t.Fatal("closing did not record the version")
	}
	again, _ := upgradedApp(t, "v0.2.0")
	if again.whatsNew.open {
		t.Fatal("What's new showed twice for one version")
	}
}

func TestFreshInstallRecordsTheVersionSilently(t *testing.T) {
	store := auth.Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	m := NewApp("dark", store, "https://example.test", t.Context()).WithVersion("v0.2.0")
	t.Cleanup(func() { _ = m.Close() })
	cmd := m.startWhatsNew()
	if m.whatsNew.open || cmd == nil {
		t.Fatal("fresh install must record without showing")
	}
	if _, ok := cmd().(whatsNewSaved); !ok || store.LastSeenVersion() != "v0.2.0" {
		t.Fatal("fresh install did not record the version")
	}
	if m.startWhatsNew() != nil {
		t.Fatal("the check must run once per launch")
	}
}

func TestFailedRecordClosesTheModalWithAWarning(t *testing.T) {
	m, _ := upgradedApp(t, "v0.1.2")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(AppModel)
	next, _ = m.Update(whatsNewSaved{err: errTestWrite})
	m = next.(AppModel)
	if m.whatsNew.open || !m.toast.visible || m.toast.kind != "warning" {
		t.Fatal("a failed write must close quietly with a warning toast")
	}
}

func TestWhatsNewFitsEveryTerminalSize(t *testing.T) {
	body, _ := release.Summary("v0.2.0")
	if strings.ContainsAny(renderSummary(body, 40, colors(true)), "#*`") {
		t.Fatal("v0.2.0 summary uses markdown outside the supported subset")
	}
	for _, dark := range []bool{false, true} {
		for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}} {
			m, _ := upgradedApp(t, "v0.1.2")
			m.account.width, m.account.height, m.account.dark = size[0], size[1], dark
			m.reflow()
			m = pressApp(m, "end")
			plain := ansi.Strip(m.View().Content)
			lines := strings.Split(plain, "\n")
			if len(lines) > size[1] || !strings.Contains(plain, "What's new") {
				t.Fatalf("%dx%d: modal missing or too tall", size[0], size[1])
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%dx%d: line too wide", size[0], size[1])
				}
			}
		}
	}
}

func TestSummaryRendererSupportsOnlyTheDocumentedSubset(t *testing.T) {
	p := colors(true)
	out := ansi.Strip(renderSummary("# Title\n\nSome **bold** and `code` text that wraps across the width.\n\n## Section\n\n- First item\n- Second **item** \x1b[31mred\n", 24, p))
	for _, want := range []string{"Title", "Section", "• First item", "Second item", "bold", "code"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.ContainsAny(out, "#*`\x1b") {
		t.Fatalf("markup or escapes leaked:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatalf("line wider than 24: %q", line)
		}
	}
}

var errTestWrite = errors.New("disk full")
