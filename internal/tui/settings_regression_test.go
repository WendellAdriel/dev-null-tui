package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestSettingsExitProtectionOwnsTheVisibleModal(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openComposer()
	m.composer.input.SetValue("retained private draft")
	m.keepDraft()
	m.openSettings()
	m.settings.busy = false
	m.beginSettingsDialog("delete-account", "", "")
	m = pressApp(m, "ctrl+c")
	if m.composer.confirm != "exit" || m.settings.dialog != "" {
		t.Fatal("the exit dialog must own drawing, mouse, and keys")
	}
	m = pressApp(m, "esc")
	if m.composer.open || m.composer.confirm != "" || m.settings.dialog != "delete-account" || m.composer.input.Value() != "retained private draft" {
		t.Fatal("Back must restore the settings dialog and retain the draft")
	}
}

func TestSettingsKeepsTheAccountAsideActionUsable(t *testing.T) {
	m := fixtureApp(t)
	m.openSettings()
	m.settings.busy = false
	m = pressApp(m, "a")
	if !m.accountOpen || !m.settings.open {
		t.Fatal("Account must open without losing the settings position")
	}
	m = pressApp(m, "esc")
	if m.accountOpen || !m.settings.open {
		t.Fatal("Esc from account must return to Settings")
	}
}

func TestSafetyListFailureKeepsDisplayedActionsAccurate(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.settings.open, m.settings.tab = true, settingsSafety
	m.settings.lists[0].people = []api.User{{ID: "ada", Handle: "ada", Name: "Ada"}}
	m.settings.lists[1].people = []api.User{{ID: "grace", Handle: "grace", Name: "Grace"}}
	m.sizeSettings()
	m.activateSetting("refresh-blocked", "")
	m.acceptSafetyList(settingsPeopleResult{session: m.session, generation: m.settings.lists[1].generation, list: 1, err: errors.New("offline")})
	visible := ansi.Strip(m.settings.viewport.GetContent())
	if !strings.Contains(visible, "@ada") || !strings.Contains(visible, "@grace") || !strings.Contains(visible, "offline") {
		t.Fatalf("wrong list or missing error: %s", visible)
	}
	for i, row := range m.settingsRows() {
		if row.action == "unblock" {
			m.settings.selected = i
			break
		}
	}
	m.sizeSettings()
	if !strings.Contains(ansi.Strip(m.settings.viewport.View()), "@grace") {
		t.Fatal("selected blocked person must remain visible")
	}
	m.settingsKey("enter")
	if m.settings.dialog != "unblock" || m.settings.handle != "grace" {
		t.Fatal("visible blocked row activated another person")
	}
}

func TestMutedProfilesAndDirectLinksRemainReadable(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openProfile("ada")
	p := m.topProfile()
	p.loaded = true
	p.profile.User = api.User{ID: "ada", Handle: "ada"}
	p.lists[0].loaded = true
	p.lists[0].commits = []api.Commit{fixtureCommit("ada-commit")}
	m.restrictSafetyContent("ada", "mute")
	if len(m.topProfile().lists[0].commits) != 1 {
		t.Fatal("mute must preserve direct author profiles")
	}
	if m.openThread(fixtureCommit("ada-direct")) == nil || len(m.threads) != 1 {
		t.Fatal("mute must preserve direct commit links")
	}
}

func TestLocalBlockFiltersIncomingThreadRootsAndPinnedQuotes(t *testing.T) {
	for _, surface := range []string{"profile", "thread"} {
		t.Run(surface, func(t *testing.T) {
			m := fixtureApp(t)
			m.account.width, m.account.height = 120, 35
			m.restrictSafetyContent("ada", "block")
			root := fixtureCommit("grace-root")
			root.Author.Handle = "grace"
			quoted := fixtureCommit("hidden-ada")
			root.Quote = &quoted
			if surface == "profile" {
				m.openProfile("grace")
				p := m.topProfile()
				next, _ := m.Update(profileResult{session: m.session, generation: p.generation, profile: api.Profile{User: api.User{ID: "grace", Handle: "grace"}, Pinned: &root}})
				m = next.(AppModel)
				if m.topProfile().profile.Pinned.Quote != nil {
					t.Fatal("pinned quote leaked blocked content")
				}
			} else {
				m.openThread(root)
				next, _ := m.Update(threadResult{session: m.session, generation: m.threads[0].generation, thread: api.Thread{Commit: root}})
				m = next.(AppModel)
				if m.threads[0].root.Quote != nil {
					t.Fatal("fresh thread quote leaked blocked content")
				}
			}
		})
	}
}

func TestConfirmedSyncUpdatesCachedOwnProfileFieldsAndCounts(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openProfile("fixture")
	p := m.topProfile()
	p.loaded = true
	p.profile.User = api.User{ID: "fixture", Handle: "fixture", Bio: "old bio", Company: "old company", Following: new(8)}
	u := m.account.user
	u.Bio, u.Company, u.Location, u.Website, u.Following = "new bio", "new company", "Porto", "https://example.test", new(7)
	m.updateOwnIdentity(u)
	got := m.topProfile().profile.User
	if got.Bio != u.Bio || got.Company != u.Company || got.Location != u.Location || got.Website != u.Website || *got.Following != 7 {
		t.Fatal("confirmed profile fields or relationship counts stayed stale")
	}
}

func TestConfirmedDeletionSurvivesConcurrentUnauthorizedRead(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.openComposer()
	m.composer.input.SetValue("retained private draft")
	m.keepDraft()
	m.settings.busy = true
	m.settings.operation = settingsOperation{kind: "delete-account"}
	session, generation := m.session, m.settings.generation
	next, _ := m.Update(notificationCountResult{session: m.session, generation: m.notificationCountGeneration, err: &auth.APIError{Status: 401}})
	m = next.(AppModel)
	m.acceptSettings(settingsResult{session: session, generation: generation, op: settingsOperation{kind: "delete-account"}, signedOut: true})
	if m.connected || m.composer.input.Value() != "" || !strings.Contains(m.account.status, "account was deleted") {
		t.Fatal("concurrent 401 swallowed confirmed deletion")
	}
}
