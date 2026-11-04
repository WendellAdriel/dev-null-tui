package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// membersApp opens the fixture org as role, with the fixture user its only maintainer when role is maintainer. The
// org's changes are recorded, and roleErr decides the next role change's answer.
type members struct {
	log     *remoteCallLog
	roleErr error
}

func membersApp(t *testing.T, role string) (AppModel, *members) {
	t.Helper()
	m, state := peopleApp(t)
	result := &members{log: state.log}
	thread := m.remoteCalls.thread
	m.remoteCalls.thread = func(ctx context.Context, target api.Target, q api.ThreadQuery) (api.RemoteThread, error) {
		data, err := thread(ctx, target, q)
		if target.Org != "" && q.After == 0 {
			data.Role = role
			data.Members = []api.Member{
				{User: api.User{ID: "fixture", Handle: "fixture", Name: "Fixture"}, Role: role},
				{User: api.User{ID: "grace", Handle: "grace", Name: "Grace Hopper"}, Role: "contributor"},
				{User: api.User{ID: "linus", Handle: "linus", Name: "Linus Torvalds"}, Role: "contributor"},
			}
		}
		return data, err
	}
	m.remoteCalls.rename = func(_ context.Context, org, name string) error {
		result.log.add("rename %s %q", org, name)
		return nil
	}
	m.remoteCalls.setRole = func(_ context.Context, org, handle string, role api.OrgRole) error {
		result.log.add("role %s %s %s", org, handle, role)
		return result.roleErr
	}
	m.remoteCalls.addMembers = func(_ context.Context, org string, handles []string, share bool) error {
		result.log.add("add %s %s %t", org, strings.Join(handles, ","), share)
		return nil
	}
	m = settle(t, m, "j", "enter")
	return settle(t, m, "I"), result
}

func TestContributorsSeeTheMembersAndCanOnlyLeave(t *testing.T) {
	m, state := membersApp(t, "contributor")
	frame := frameText(m)
	if m.remotes.members == nil || !strings.Contains(frame, "Members · 3") || !strings.Contains(frame, "Grace Hopper") || !strings.Contains(frame, "contributor") {
		t.Fatalf("members:\n%s", frame)
	}
	if strings.Contains(frame, "rename") || strings.Contains(frame, "a add") {
		t.Fatalf("a contributor sees maintainer controls:\n%s", frame)
	}
	m = settle(t, m, "j", "enter", "r", "a")
	if m.remotes.members.menu || m.remotes.members.renaming || m.remotes.people != nil {
		t.Fatal("a contributor opened a maintainer control")
	}
	m = settle(t, m, "k", "enter")
	if m.remotes.dialog != "block" || !strings.Contains(frameText(m), "Leave") {
		t.Fatalf("leaving from the own row:\n%s", frameText(m))
	}
	if strings.Contains(state.log.String(), "role") {
		t.Fatalf("unexpected calls: %s", state.log)
	}
}

func TestClickingTheMemberCountOpensMembers(t *testing.T) {
	m, _ := membersApp(t, "contributor")
	m, _ = clickText(t, settle(t, m, "esc"), "3 members")
	if m.remotes.members == nil {
		t.Fatal("clicking the member count did not open Members")
	}
	m, _ = clickText(t, settle(t, m, "esc"), "Grace, Linus  org")
	if m.remotes.members != nil {
		t.Fatal("clicking the title opened Members")
	}
}

func TestAMaintainerRenamesTheOrg(t *testing.T) {
	m, state := membersApp(t, "maintainer")
	m = typeKeys(t, settle(t, m, "r"), strings.Repeat("x", 70))
	if got := len([]rune(m.remotes.members.name)); got != api.MaxOrgName {
		t.Fatalf("name kept %d characters", got)
	}
	m.remotes.members.name = "core"
	m = settle(t, m, "enter")
	if !strings.HasSuffix(state.log.String(), `rename `+fixtureOrg+` "core"`) || !strings.Contains(threadText(m), "core") || m.conversationTitle(m.remotes.inbox[1]) != "core" {
		t.Fatalf("rename: %s\n%s", state.log, threadText(m))
	}
	m = settle(t, m, "r")
	m.remotes.members.name = ""
	m = settle(t, m, "enter")
	if !strings.HasSuffix(state.log.String(), `rename `+fixtureOrg+` ""`) || m.conversationTitle(m.remotes.thread.conversation) != "Grace, Linus" {
		t.Fatalf("clearing the name: %s", state.log)
	}
}

func TestTheRoleMenuKeepsAMaintainer(t *testing.T) {
	m, state := membersApp(t, "maintainer")
	m = settle(t, m, "enter")
	if frame := frameText(m); !strings.Contains(frame, "Make contributor") || !strings.Contains(frame, "the org needs a maintainer") {
		t.Fatalf("own menu:\n%s", frame)
	}
	m = settle(t, m, "enter")
	if strings.Contains(state.log.String(), "role") {
		t.Fatalf("the last maintainer stepped down: %s", state.log)
	}
	m = settle(t, m, "esc", "j", "enter", "enter")
	if !strings.HasSuffix(state.log.String(), "role "+fixtureOrg+" grace maintainer") || !strings.Contains(frameText(m), "maintainer") || m.remotes.thread.data.Members[1].Role != "maintainer" {
		t.Fatalf("make maintainer: %s", state.log)
	}
	state.roleErr = &auth.APIError{Status: http.StatusUnprocessableEntity, FieldErrors: []string{"An org needs at least one maintainer."}}
	m = settle(t, m, "enter", "enter")
	if !strings.Contains(frameText(m), "An org needs at least one maintainer.") || m.remotes.thread.data.Members[1].Role != "maintainer" {
		t.Fatalf("refused role:\n%s", frameText(m))
	}
}

func TestRemovingSomeoneAsksFirst(t *testing.T) {
	m, state := membersApp(t, "maintainer")
	m = settle(t, m, "j", "j", "enter", "down", "enter")
	if !strings.Contains(frameText(m), "Remove @linus from") || strings.Contains(state.log.String(), "leave") {
		t.Fatalf("no confirmation:\n%s", frameText(m))
	}
	m = settle(t, m, "enter")
	if !strings.HasSuffix(state.log.String(), "leave "+fixtureOrg+" linus") || !strings.Contains(threadText(m), "org · 2 members") || !strings.Contains(frameText(m), "Members · 2") {
		t.Fatalf("remove: %s\n%s", state.log, threadText(m))
	}
}

func TestAddingPeopleJoinsThemAsContributors(t *testing.T) {
	m, state := membersApp(t, "maintainer")
	m = settle(t, m, "a")
	if p := m.remotes.people; p == nil || p.mode != "add" || !strings.Contains(frameText(m), "They join as contributors.") {
		t.Fatalf("add people:\n%s", frameText(m))
	}
	m = searchNow(t, typeKeys(t, m, "a"))
	if !strings.Contains(state.log.String(), `candidates  `+fixtureOrg+` "a"`) {
		t.Fatalf("search: %s", state.log)
	}
	m = settle(t, m, "down", "space", "tab", "space", "ctrl+s")
	if !strings.Contains(state.log.String(), "add "+fixtureOrg+" ada true") || !strings.Contains(threadText(m), "org · 4 members") {
		t.Fatalf("add: %s", state.log)
	}

	t.Run("a full org takes no one", func(t *testing.T) {
		m, _ := membersApp(t, "maintainer")
		for i := len(m.remotes.thread.data.Members); i < api.MaxOrgMembers; i++ {
			m.remotes.thread.data.Members = append(m.remotes.thread.data.Members, api.Member{User: api.User{ID: fmt.Sprint("m", i), Handle: fmt.Sprint("m", i)}, Role: "contributor"})
		}
		m = settle(t, m, "a")
		if m.remotes.people != nil || !strings.Contains(frameText(m), "The org is full (100)") {
			t.Fatalf("full org:\n%s", frameText(m))
		}
	})
}

func TestMaintainersStandOutInMembers(t *testing.T) {
	m, _ := membersApp(t, "maintainer")
	pal := colors(m.account.dark)
	frame, _, _ := m.membersModal(pal)
	if !strings.Contains(frame, text(pal.blue).Render("maintainer")) || !strings.Contains(frame, text(pal.muted).Render("contributor")) {
		t.Fatalf("role colors:\n%q", frame)
	}
}

func TestUOpensAMembersProfile(t *testing.T) {
	m, _ := membersApp(t, "contributor")
	m = settle(t, m, "j", "u")
	if len(m.profiles) != 1 || m.profiles[0].handle != "grace" {
		t.Fatalf("u opened %d profiles", len(m.profiles))
	}
	m = settle(t, m, "esc")
	if len(m.profiles) != 0 || m.remotes.members == nil || m.remotes.members.index != 1 {
		t.Fatal("closing the profile did not return to Members")
	}
	if !strings.Contains(frameText(m), "u profile") {
		t.Fatalf("no u hint:\n%s", frameText(m))
	}
}
