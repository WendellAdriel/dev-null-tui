package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// The Members dialog follows the web's resources/js/components/chat/org.tsx MembersDialog.

// membersDialog lists an org's members. A maintainer renames the org, adds people, and opens a member's menu to change
// their role or remove them; anyone can leave from their own row.
type membersDialog struct {
	index     int
	menu      bool
	menuFocus int
	renaming  bool
	name      string
	removing  bool
	sending   bool
	err       string
}

type membersResult struct {
	session int
	kind    string
	org     string
	handle  string
	role    api.OrgRole
	name    string
	err     error
}

type memberAction struct {
	label, kind string
	disabled    string
}

// orgMembers is the open org's member list: the thread's when it has loaded, else the inbox row's.
func (m AppModel) orgMembers() []api.Member {
	t := m.remotes.thread
	if t.data.Members != nil {
		return t.data.Members
	}
	return t.conversation.Members
}

func (m AppModel) maintaining() bool {
	return m.remotes.thread.data.Role == string(api.RoleMaintainer)
}

func (m *AppModel) openMembers() {
	if t := m.remotes.thread; t != nil && t.conversation.Kind == "org" {
		m.remotes.members = &membersDialog{}
	}
}

// memberActions is the menu for the focused member, or nothing when the user can't change them.
func (m AppModel) memberActions(member api.Member) []memberAction {
	mine := member.User.ID == m.account.user.ID
	if !m.maintaining() {
		if mine {
			return []memberAction{{label: "Leave org", kind: "leave"}}
		}
		return nil
	}
	if member.Role == string(api.RoleMaintainer) {
		step := memberAction{label: "Make contributor", kind: string(api.RoleContributor)}
		maintainers := 0
		for _, other := range m.orgMembers() {
			if other.Role == string(api.RoleMaintainer) {
				maintainers++
			}
		}
		if maintainers <= 1 {
			step.disabled = "the org needs a maintainer"
		}
		if mine {
			return []memberAction{step, {label: "Leave org", kind: "leave"}}
		}
		return []memberAction{step, {label: "Remove from org", kind: "remove"}}
	}
	if mine {
		return []memberAction{{label: "Leave org", kind: "leave"}}
	}
	return []memberAction{{label: "Make maintainer", kind: string(api.RoleMaintainer)}, {label: "Remove from org", kind: "remove"}}
}

func (m *AppModel) membersKey(pressed string) tea.Cmd {
	d := m.remotes.members
	list := m.orgMembers()
	if d.sending {
		return nil
	}
	switch {
	case d.renaming:
		switch pressed {
		case "esc":
			d.renaming, d.err = false, ""
		case "enter", "ctrl+s":
			return m.saveMembers("rename", list[0], strings.TrimSpace(d.name))
		default:
			d.name = typeInto(d.name, pressed, api.MaxOrgName)
		}
		return nil
	case d.removing:
		switch pressed {
		case "esc":
			d.removing, d.err = false, ""
		case "enter":
			return m.saveMembers("remove", list[d.index], "")
		}
		return nil
	case d.menu:
		actions := m.memberActions(list[d.index])
		switch pressed {
		case "esc":
			d.menu, d.err = false, ""
		case "j", "down", "tab":
			d.menuFocus = (d.menuFocus + 1) % len(actions)
		case "k", "up", "shift+tab":
			d.menuFocus = (d.menuFocus + len(actions) - 1) % len(actions)
		case "enter":
			return m.chooseMemberAction(actions[d.menuFocus])
		}
		return nil
	}
	switch pressed {
	case "esc":
		m.remotes.members = nil
	case "j", "down":
		d.index = min(d.index+1, len(list)-1)
	case "k", "up":
		d.index = max(d.index-1, 0)
	case "home":
		d.index = 0
	case "G", "end":
		d.index = len(list) - 1
	case "enter":
		actions := m.memberActions(list[d.index])
		switch {
		case len(actions) == 1:
			return m.chooseMemberAction(actions[0])
		case len(actions) > 1:
			d.menu, d.menuFocus, d.err = true, 0, ""
		}
	case "u":
		return m.openProfile(list[d.index].User.Handle)
	case "r":
		if m.maintaining() {
			d.renaming, d.err, d.name = true, "", ""
			if name := m.remotes.thread.conversation.Name; name != nil {
				d.name = *name
			}
		}
	case "a":
		if !m.maintaining() {
			return nil
		}
		if len(list) >= api.MaxOrgMembers {
			d.err = fmt.Sprintf("The org is full (%d).", api.MaxOrgMembers)
			return nil
		}
		org := m.remotes.thread.conversation
		org.Members = list
		m.remotes.members = nil
		m.openPeople("add", org)
	}
	return nil
}

func (m *AppModel) chooseMemberAction(action memberAction) tea.Cmd {
	d := m.remotes.members
	if action.disabled != "" {
		return nil
	}
	d.menu = false
	switch action.kind {
	case "leave":
		m.remotes.members = nil
		m.remotes.dialog, m.remotes.dialogFocus, m.remotes.dialogTarget = "block", 0, m.remotes.thread.conversation
		return nil
	case "remove":
		d.removing = true
		return nil
	}
	return m.saveMembers("role", m.orgMembers()[d.index], action.kind)
}

// saveMembers sends a rename, role change, or removal. Value is the new name or role.
func (m *AppModel) saveMembers(kind string, member api.Member, value string) tea.Cmd {
	d := m.remotes.members
	d.sending, d.err = true, ""
	calls, session, life := m.remoteCalls, m.session, m.account.life
	result := membersResult{session: session, kind: kind, org: m.remotes.thread.conversation.ID, handle: member.User.Handle, role: api.OrgRole(value), name: value}
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		switch kind {
		case "rename":
			result.err = calls.rename(ctx, result.org, result.name)
		case "role":
			result.err = calls.setRole(ctx, result.org, result.handle, result.role)
		case "remove":
			result.err = calls.leave(ctx, result.org, result.handle)
		}
		return result
	})
}

func (m *AppModel) acceptMembers(msg membersResult) tea.Cmd {
	if !m.connected || msg.session != m.session {
		return nil
	}
	d := m.remotes.members
	if d != nil {
		d.sending = false
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		failure := managementError(msg.err, map[string]string{"rename": "Could not rename the org. Try again.", "role": "Could not change the role. Try again.", "remove": "Could not remove @" + safeText(msg.handle) + ". Try again."}[msg.kind])
		if d == nil {
			return m.notify("error", failure, "")
		}
		d.err = failure
		return nil
	}
	if d != nil {
		d.renaming, d.removing = false, false
	}
	same := func(member api.Member) bool { return strings.EqualFold(member.User.Handle, msg.handle) }
	change := func(members []api.Member) []api.Member {
		if msg.kind == "remove" {
			return slices.DeleteFunc(slices.Clone(members), same)
		}
		for i := range members {
			if same(members[i]) {
				members[i].Role = string(msg.role)
			}
		}
		return members
	}
	t := m.remotes.thread
	thread := t != nil && t.conversation.ID == msg.org
	switch msg.kind {
	case "rename":
		var name *string
		if msg.name != "" {
			name = &msg.name
		}
		m.updateConversation(msg.org, func(c *api.Conversation) { c.Name = name })
		if thread && t.data.Org != nil {
			t.data.Org.Name = name
		}
	default:
		m.updateConversation(msg.org, func(c *api.Conversation) { c.Members = change(c.Members) })
		if thread && t.data.Members != nil {
			t.data.Members = change(t.data.Members)
		}
		if d != nil && thread {
			d.index = max(0, min(d.index, len(m.orgMembers())-1))
		}
	}
	return nil
}

func (m AppModel) membersModal(pal palette) (string, uiRect, []hitArea) {
	d := m.remotes.members
	list := m.orgMembers()
	rect := centeredRect(max(1, m.account.width), max(1, m.account.height), 64, 24)
	width := max(1, rect.w-4)
	var lines []string
	var areas []hitArea
	top := rect.y + 3
	add := func(line, action string, index int) {
		if action != "" {
			areas = append(areas, hitArea{uiRect{rect.x + 2, top + len(lines), width, 1}, action, index})
		}
		lines = append(lines, line)
	}
	if d.renaming {
		value := text(pal.muted).Render("Name it (optional)")
		if d.name != "" {
			value = text(pal.ink).Render(safeText(d.name))
		}
		add(fit(text(pal.muted).Render("Name ")+value+text(pal.blue).Render("▏"), width), "", 0)
		add("", "", 0)
	}
	menuLines := 0
	if d.menu {
		menuLines = len(m.memberActions(list[d.index]))
	}
	rows := max(1, rect.h-8-len(lines)-menuLines)
	start := max(0, min(d.index-rows+1, len(list)-rows))
	for i := start; i < min(len(list), start+rows); i++ {
		member := list[i]
		you := ""
		if member.User.ID == m.account.user.ID {
			you = text(pal.muted).Render(" (you)")
		}
		name := text(pal.ink).Bold(true).Render(safeText(member.User.Name)) + " " + text(pal.muted).Render("@"+safeText(member.User.Handle)) + you
		// As on the web, maintainers are blue and contributors grey.
		role := text(pal.muted)
		if member.Role == string(api.RoleMaintainer) {
			role = text(pal.blue)
		}
		row := " " + split(fit(name, max(1, width-16)), role.Render(member.Role), width-2) + " "
		if i == d.index && !d.renaming {
			row = withBackground(fit(row, width), pal.soft)
		}
		add(row, "member", i)
		if i == d.index && d.menu {
			for j, action := range m.memberActions(member) {
				label := "   " + action.label
				style := text(pal.ink)
				if action.disabled != "" {
					label += " · " + action.disabled
					style = text(pal.muted)
				}
				line := style.Render(fit(label, width))
				if j == d.menuFocus {
					line = withBackground(lipgloss.NewStyle().Width(width).Render(fit(line, width)), pal.soft)
				}
				add(line, "action", j)
			}
		}
	}
	if d.err != "" {
		add("", "", 0)
		add(text(pal.danger).Render(fit(d.err, width)), "", 0)
	}
	hints := "u profile · enter on you: leave · esc close"
	if m.maintaining() {
		hints = "enter manage · u profile · r rename · a add · esc close"
	}
	if d.renaming {
		hints = "enter save · esc cancel"
	}
	footer := text(pal.muted).Render(fit(hints, width))
	var footerAreas []hitArea
	if d.removing {
		member := list[d.index]
		question := text(pal.ink).Bold(true).Render(fit("Remove @"+safeText(member.User.Handle)+" from "+m.conversationTitle(m.remotes.thread.conversation)+"?", width))
		var buttons string
		buttons, footerAreas = modalFooter([]modalAction{{"Cancel", "esc", "esc"}, {"Remove", "enter", "remove"}}, width, 2, pal)
		footer = question + "\n" + buttons
		for i := range footerAreas {
			footerAreas[i].x += rect.x + 2
			footerAreas[i].y += rect.y + rect.h - 1 - lipgloss.Height(buttons)
		}
	}
	areas = append(append(areas, footerAreas...), hitArea{uiRect{rect.x + rect.w - 3, rect.y + 1, 1, 1}, "esc", 0})
	title := fmt.Sprintf("Members · %d", len(list))
	return panel(title, strings.Join(lines, "\n"), footer, rect, pal, pal.hairline), rect, areas
}

func (m AppModel) membersMouse(x, y int) (AppModel, tea.Cmd) {
	d := m.remotes.members
	_, rect, areas := m.membersModal(colors(m.account.dark))
	if d.sending {
		return m, nil
	}
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		switch area.action {
		case "esc":
			cmd := m.membersKey("esc")
			return m, cmd
		case "remove":
			cmd := m.membersKey("enter")
			return m, cmd
		case "member":
			d.index, d.menu, d.removing, d.renaming = area.index, false, false, false
			cmd := m.membersKey("enter")
			return m, cmd
		case "action":
			d.menuFocus = area.index
			cmd := m.membersKey("enter")
			return m, cmd
		}
		return m, nil
	}
	if !rect.contains(x, y) {
		m.remotes.members = nil
	}
	return m, nil
}
