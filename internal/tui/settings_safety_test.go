package tui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestSafetyTabShowsMutedAndBlockedInOrder(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 160, 45
	m.settings.open, m.settings.tab = true, settingsSafety
	m.settings.lists[0] = settingsPeople{loaded: true, people: []api.User{{ID: "ada", Handle: "ada", Name: "Ada"}}}
	m.settings.lists[1] = settingsPeople{loaded: true, people: []api.User{{ID: "grace", Handle: "grace", Name: "Grace"}}}
	m.sizeSettings()
	content := ansi.Strip(m.settings.viewport.View())
	if !strings.Contains(content, "@ada") || !strings.Contains(content, "@grace") || strings.Index(content, "Muted") >= strings.Index(content, "Blocked") || strings.Contains(content, "switch to") {
		t.Fatalf("both safety sections must be visible in order:\n%s", content)
	}
	m.account.width, m.account.height = 60, 18
	m.sizeSettings()
	for range len(m.settingsRows()) {
		if m.settingsRows()[m.settings.selected].action == "unblock" {
			break
		}
		m.settingsKey("down")
	}
	if !strings.Contains(ansi.Strip(m.settings.viewport.View()), "@grace") {
		t.Fatal("compact keyboard navigation must scroll to Blocked")
	}
}

func TestOpeningSafetyLoadsBothListsAndPaginatesThemIndependently(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if strings.HasSuffix(r.URL.Path, "muted") {
			_, _ = io.WriteString(w, `{"data":[{"id":"ada","handle":"ada"}],"meta":{"next_cursor":"older-muted"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"grace","handle":"grace"}],"meta":{"next_cursor":"older-blocked"}}`)
	}))
	defer server.Close()
	m := fixtureApp(t)
	m.settingsAPI, _ = api.NewClient(server.URL, m.account.key)
	m.settings.open, m.settings.tab = true, settingsAppearance
	cmd := m.settingsKey("right")
	if cmd == nil || !m.settings.lists[0].loading || !m.settings.lists[1].loading {
		t.Fatal("opening Safety must load both lists")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("both safety reads must be scheduled")
	}
	for _, read := range batch {
		m.acceptSafetyList(read().(settingsPeopleResult))
	}
	if len(paths) != 2 || paths[0] != "/api/v1/me/muted" || paths[1] != "/api/v1/me/blocked" {
		t.Fatal(paths)
	}
	cmd = m.activateSetting("more-muted", "")
	if cmd == nil || !m.settings.lists[0].loading || m.settings.lists[1].loading {
		t.Fatal("muted pagination must not reload Blocked")
	}
	m.acceptSafetyList(cmd().(settingsPeopleResult))
	if paths[2] != "/api/v1/me/muted?cursor=older-muted" || len(m.settings.lists[1].people) != 1 {
		t.Fatal(paths)
	}
	cmd = m.activateSetting("refresh-blocked", "")
	if cmd == nil || m.settings.lists[0].loading || !m.settings.lists[1].loading {
		t.Fatal("blocked refresh must not reload Muted")
	}
	m.acceptSafetyList(cmd().(settingsPeopleResult))
	if paths[3] != "/api/v1/me/blocked" {
		t.Fatal(paths)
	}
}

func TestSafetyPaginationKeepsSelectionInTheOtherSection(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.settings.open, m.settings.tab = true, settingsSafety
	m.settings.lists[0] = settingsPeople{loaded: true, people: []api.User{{ID: "ada", Handle: "ada"}}, cursors: map[string]bool{}}
	m.settings.lists[1] = settingsPeople{loaded: true, people: []api.User{{ID: "grace", Handle: "grace"}}}
	m.sizeSettings()
	for i, row := range m.settingsRows() {
		if row.action == "unblock" {
			m.settings.selected = i
			break
		}
	}
	m.sizeSettings()
	m.settings.lists[0].loading = true
	m.settings.lists[0].generation = 5
	m.acceptSafetyList(settingsPeopleResult{session: m.session, generation: 5, list: 0, cursor: "older", page: api.People{Data: []api.User{{ID: "bob", Handle: "bob"}}}})
	row := m.settingsRows()[m.settings.selected]
	if row.action != "unblock" || row.value != "grace" {
		t.Fatal("new muted entries moved selection away from the blocked person")
	}
}
