package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// The commit menu follows the web's resources/js/components/commit-secondary-actions.tsx: the actions a commit
// shows only on demand, opened with the ⋯ More chip or the . key.

// commitMenuAuthor carries the commit author's profile, read when the menu opens because a commit doesn't say
// whether you follow or mute its author.
type commitMenuAuthor struct {
	session int
	handle  string
	profile api.Profile
	err     error
}

type commitShared struct {
	session int
	handle  string
	err     error
}

// menuAuthor is the handle the menu offers Follow, Mute and Block for: another person, never an organization.
func (m AppModel) menuAuthor(c api.Commit) string {
	a := c.Author
	if c.DeletedAt != nil || a == nil || a.Organization() || a.ID == m.account.user.ID || strings.EqualFold(a.Handle, m.account.user.Handle) || m.loadProfile == nil {
		return ""
	}
	return a.Handle
}

// canShareCommit is the web's canSend: Remotes on and an approved commit with a link to send.
func (m AppModel) canShareCommit(c api.Commit) bool {
	return m.account.user.Remotes != nil && (c.Status == "" || c.Status == "approved") && c.DeletedAt == nil && c.Author != nil &&
		m.validCommitURL(c.URL) && m.remoteCalls.push != nil && m.remoteCalls.candidates != nil
}

func (m AppModel) commitMenuRows(c api.Commit, author *api.Profile, loading bool) []pickerRow {
	rows := []pickerRow{{label: "Copy link", key: "y", value: "y"}}
	if m.canCopyText(c) {
		rows = append(rows, pickerRow{label: "Copy text", key: "T", value: "T"})
	}
	if len(m.commitLinks(c)) > 0 {
		rows = append(rows, pickerRow{label: "Open link", key: "x", value: "x"})
	}
	if m.canShareCommit(c) {
		rows = append(rows, pickerRow{label: "Send in Remotes", value: "share"})
	}
	if c.Stars > 0 && m.ownCommit(c) && !m.organizationSettingsKey() {
		rows = append(rows, pickerRow{label: "Starred by", key: "S", value: "S"})
	}
	if m.canPinCommit(c) {
		label := "Pin"
		if c.Pinned != nil && *c.Pinned {
			label = "Unpin"
		}
		rows = append(rows, pickerRow{label: label, key: "P", value: "P"})
	}
	if handle := m.menuAuthor(c); handle != "" {
		at := "@" + safeText(handle)
		switch {
		case loading:
			rows = append(rows, pickerRow{label: "Checking " + at + "…"})
		case author != nil:
			if author.IsFollowing != nil {
				label := "Follow " + at
				if *author.IsFollowing {
					label = "Unfollow " + at
				}
				rows = append(rows, pickerRow{label: label, value: "follow"})
			}
			label := "Mute " + at
			if author.IsMuting != nil && *author.IsMuting {
				label = "Unmute " + at
			}
			rows = append(rows, pickerRow{label: label, value: "mute"}, pickerRow{label: "Block " + at, value: "block", danger: true})
		}
	}
	if m.ownCommit(c) {
		rows = append(rows, pickerRow{label: "Delete commit", key: "D", value: "D", danger: true})
	} else if c.Author != nil && (c.Status == "" || c.Status == "approved") {
		rows = append(rows, pickerRow{label: "Report commit", key: "!", value: "!", danger: true})
	}
	return rows
}

func (m *AppModel) openCommitMenu(c api.Commit) tea.Cmd {
	if c.DeletedAt != nil || c.Author == nil {
		return nil
	}
	handle := m.menuAuthor(c)
	m.picker = &picker{kind: "commit-menu", title: "/dev/null · COMMIT", commit: c, loading: handle != ""}
	m.picker.rows = m.commitMenuRows(c, nil, m.picker.loading)
	if handle == "" {
		return nil
	}
	session, load, life := m.session, m.loadProfile, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		profile, err := load(ctx, handle)
		return commitMenuAuthor{session: session, handle: handle, profile: profile, err: err}
	})
}

func (m *AppModel) acceptCommitMenuAuthor(msg commitMenuAuthor) {
	d := m.picker
	if d == nil || d.kind != "commit-menu" || msg.session != m.session || !strings.EqualFold(m.menuAuthor(d.commit), msg.handle) {
		return
	}
	if msg.err != nil && m.revoked(msg.err) {
		return
	}
	d.loading = false
	var author *api.Profile
	if msg.err == nil {
		author = &msg.profile
	} else {
		d.status = "Couldn't check @" + safeText(msg.handle) + ", so Follow, Mute and Block are unavailable."
	}
	chosen := ""
	if d.selected < len(d.rows) {
		chosen = d.rows[d.selected].label
	}
	d.author, d.rows = author, m.commitMenuRows(d.commit, author, false)
	d.selected = min(d.selected, len(d.rows)-1)
	for i, row := range d.rows {
		if row.label == chosen {
			d.selected = i
		}
	}
}

func (m AppModel) chooseCommitMenu(row pickerRow) (AppModel, tea.Cmd) {
	if row.value == "" {
		return m, nil
	}
	c, author := m.picker.commit, m.picker.author
	m.picker = nil
	var cmd tea.Cmd
	switch row.value {
	case "y":
		cmd = m.copyCommitLink(c)
	case "T":
		cmd = m.copyCommitText(c)
	case "x":
		cmd = m.openLinks(m.commitLinks(c))
	case "S":
		cmd = m.openStargazers(c)
	case "P":
		cmd = m.startPin(c)
	case "share":
		m.openShare(c)
	case "follow":
		cmd = m.startFollow(c.Author.Handle, author.IsFollowing)
	case "mute":
		kind := "mute"
		if author.IsMuting != nil && *author.IsMuting {
			kind = "unmute"
		}
		cmd = m.beginSettingsDialog(kind, c.Author.Handle, "")
	case "block":
		cmd = m.beginSettingsDialog("block", c.Author.Handle, "")
	case "!":
		cmd = m.beginSettingsDialog("report", "", c.ID)
	case "D":
		m.confirmDelete(c)
	}
	return m, tea.Batch(m.syncImages(), cmd)
}

// sharingCommit reports the Send in Remotes picker, which opens over any screen, unlike the other people dialogs.
func (m AppModel) sharingCommit() bool {
	return m.remotes.people != nil && m.remotes.people.mode == "share"
}

func (m *AppModel) openShare(c api.Commit) {
	if !m.canShareCommit(c) {
		return
	}
	m.closePeople()
	m.remotes.people = &peopleDialog{mode: "share", field: peopleSearch, commit: c}
}

func (m *AppModel) confirmShare(c api.Commit, person api.User) {
	m.closePeople()
	m.ask = &ask{kind: "share-commit", title: "/dev/null · REMOTES", question: "Send commit to @" + safeText(person.Handle) + "?",
		detail: "Sends a link to this commit in Remotes: " + firstLine(c.Body), confirm: "Send", border: m.account.colors().blue, commit: c, id: person.Handle}
}

func firstLine(body string) string {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(body), "\n", 2)[0])
	if line == "" {
		return "(no text)"
	}
	return safeText(line)
}

// shareCommit sends the commit's permalink as a direct message, as the web does.
func (m *AppModel) shareCommit(c api.Commit, handle string) tea.Cmd {
	if !m.canShareCommit(c) {
		return nil
	}
	session, push, life := m.session, m.remoteCalls.push, m.account.life
	draft := api.NewMessage{Body: c.URL, ClientID: newClientID()}
	m.notice = "Sending to @" + safeText(handle) + "..."
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 30*time.Second)
		defer cancel()
		_, err := push(ctx, api.Target{Handle: handle}, draft)
		return commitShared{session: session, handle: handle, err: err}
	})
}

func (m *AppModel) acceptCommitShared(msg commitShared) tea.Cmd {
	if !m.connected || msg.session != m.session {
		return nil
	}
	m.notice = ""
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		return m.notify("error", "Not sent", managementError(msg.err, "Couldn't send the commit. Try again."))
	}
	return m.notify("success", "Sent", "The commit link is in your remote with @"+safeText(msg.handle)+".")
}
