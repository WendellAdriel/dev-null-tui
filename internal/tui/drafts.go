package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// draftsState is the Drafts screen, opened with g d or the sidebar.
type draftsState struct {
	open     bool
	selected int
	// loading is the draft whose reply or quote target is being read before its composer opens.
	loading string
}

// draftTargetLoaded carries the commit a reply or quote draft answers, read again so the composer shows it as it is now.
type draftTargetLoaded struct {
	session int
	key     string
	commit  api.Commit
	err     error
}

// draftKey names the place a draft belongs to, like the web's stash names: the new commit, or one per reply and quote.
func draftKey(mode string, target api.Commit) string {
	if mode == "" {
		return "commit"
	}
	return mode + ":" + target.ID
}

func (m *AppModel) loadCommitDrafts() {
	m.commitDrafts = m.account.store.CommitDrafts(m.account.user.ID)
}

// canSaveDrafts guards the file: only the connected owner's own draft is written, never one kept across a reconnect.
// A draft whose last submission had an unknown outcome stays in memory, so its duplicate-risk confirmation survives.
func (m AppModel) canSaveDrafts() bool {
	c := m.composer
	return m.connected && c.owner != "" && c.owner == m.account.user.ID && !c.sending && !c.uncertain
}

func (m *AppModel) saveCommitDrafts() tea.Cmd {
	if err := m.account.store.SaveCommitDrafts(m.account.user.ID, m.commitDrafts); err != nil {
		return m.notify("warning", "Draft not saved", "The drafts file could not be written: "+safeText(err.Error()))
	}
	m.commitDrafts = m.account.store.CommitDrafts(m.account.user.ID)
	return nil
}

// stashComposer saves the open draft to disk and clears the composer, which is what Keep means once drafts persist.
func (m *AppModel) stashComposer() tea.Cmd {
	c := m.composer
	draft := auth.CommitDraft{Kind: "commit", Body: c.input.Value(), SavedAt: time.Now()}
	if c.mode != "" {
		draft.Kind, draft.TargetID = c.mode, c.target.ID
		if c.target.Author != nil {
			draft.TargetHandle = c.target.Author.Handle
		}
		draft.TargetExcerpt = draftExcerpt(c.target.Body)
	}
	for _, image := range c.images {
		draft.Images = append(draft.Images, auth.DraftImage{Path: image.Path, Description: image.Description})
	}
	if c.poll != nil {
		draft.Poll = &auth.DraftPoll{Options: append([]string(nil), c.poll.options...), Duration: c.poll.duration}
	}
	if m.commitDrafts == nil {
		m.commitDrafts = map[string]auth.CommitDraft{}
	}
	m.commitDrafts[draftKey(c.mode, c.target)] = draft
	m.clearComposer()
	return m.saveCommitDrafts()
}

// dropDraft deletes the open composer's saved draft, after Discard or a submission.
func (m *AppModel) dropDraft() tea.Cmd {
	if err := m.removeDraft(); err != nil {
		return m.notify("warning", "Draft not deleted", "The drafts file could not be written: "+safeText(err.Error()))
	}
	return nil
}

func (m *AppModel) removeDraft() error {
	key := draftKey(m.composer.mode, m.composer.target)
	if _, ok := m.commitDrafts[key]; !ok || m.composer.owner != m.account.user.ID {
		return nil
	}
	delete(m.commitDrafts, key)
	err := m.account.store.SaveCommitDrafts(m.account.user.ID, m.commitDrafts)
	m.commitDrafts = m.account.store.CommitDrafts(m.account.user.ID)
	return err
}

// restoreDraft fills a fresh composer from its place's saved draft. Images are read again, and ones that are gone or
// no longer valid are dropped with a note, like message drafts.
func (m *AppModel) restoreDraft() {
	c := &m.composer
	draft, ok := m.commitDrafts[draftKey(c.mode, c.target)]
	if !ok || c.hasDraft() {
		return
	}
	c.input.SetValue(draft.Body)
	dropped := 0
	for _, image := range draft.Images {
		file, err := api.ReadImage(image.Path)
		if err != nil || len(c.images) >= api.MaxImages {
			dropped++
			continue
		}
		file.Description = image.Description
		c.images = append(c.images, file)
	}
	if draft.Poll != nil && len(c.images) == 0 {
		c.poll = &pollDraft{options: append([]string(nil), draft.Poll.Options...), duration: draft.Poll.Duration}
	}
	m.attachmentsChanged()
	switch {
	case dropped == 1:
		c.status = "An image in this draft can't be read any more, so it was dropped."
	case dropped > 1:
		c.status = fmt.Sprintf("%d images in this draft can't be read any more, so they were dropped.", dropped)
	}
}

// draftList is the saved drafts, newest first.
func (m AppModel) draftList() []string {
	keys := make([]string, 0, len(m.commitDrafts))
	for key := range m.commitDrafts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := m.commitDrafts[keys[i]], m.commitDrafts[keys[j]]
		if !a.SavedAt.Equal(b.SavedAt) {
			return a.SavedAt.After(b.SavedAt)
		}
		return keys[i] < keys[j]
	})
	return keys
}

func (m AppModel) draftsOnTop() bool {
	return m.drafts.open && len(m.threads) == 0 && len(m.profiles) == 0
}

func (m AppModel) openDrafts() (AppModel, tea.Cmd) {
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	m.cancelNotifications()
	m.notificationsOpen = false
	m.closeStarred()
	m.closeRemotes()
	m.drafts = draftsState{open: true}
	m.resetSequence()
	m.reflow()
	return m, nil
}

func (m AppModel) draftsKey(pressed string) (AppModel, tea.Cmd, bool) {
	keys := m.draftList()
	d := &m.drafts
	switch pressed {
	case "j", "down":
		d.selected = min(max(0, len(keys)-1), d.selected+1)
	case "k", "up":
		d.selected = max(0, d.selected-1)
	case "home":
		d.selected = 0
	case "end", "G":
		d.selected = max(0, len(keys)-1)
	case "esc":
		next, cmd := m.switchLog(m.lastLog)
		return next, cmd, true
	case "enter":
		if d.selected < len(keys) {
			return m, m.reopenDraft(keys[d.selected]), true
		}
	case "D":
		if d.selected < len(keys) {
			m.ask = &ask{kind: "delete-draft", title: "/dev/null · DRAFT", question: "Delete this draft?", detail: "It's removed from this device. This can't be undone.", confirm: "Delete", border: m.account.colors().danger, id: keys[d.selected]}
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

// reopenDraft opens a draft's composer. A reply or quote first reads its target again, since only its id is saved.
func (m *AppModel) reopenDraft(key string) tea.Cmd {
	draft, ok := m.commitDrafts[key]
	if !ok {
		return nil
	}
	if draft.Kind == "commit" || draft.TargetID == "" {
		return m.composeTo("", api.Commit{})
	}
	if m.loadThread == nil || m.drafts.loading != "" {
		return nil
	}
	m.drafts.loading = key
	session, id, load := m.session, draft.TargetID, m.loadThread
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
		defer cancel()
		thread, err := load(ctx, id)
		return draftTargetLoaded{session: session, key: key, commit: thread.Commit, err: err}
	})
}

func (m *AppModel) acceptDraftTarget(msg draftTargetLoaded) tea.Cmd {
	if msg.session != m.session || msg.key != m.drafts.loading {
		return nil
	}
	m.drafts.loading = ""
	draft, ok := m.commitDrafts[msg.key]
	if !ok {
		return nil
	}
	var failure *auth.APIError
	if msg.err != nil || msg.commit.DeletedAt != nil || msg.commit.Author == nil {
		if m.revoked(msg.err) {
			return nil
		}
		detail := "Couldn't load the commit this draft answers. Try again."
		if msg.err == nil || errors.As(msg.err, &failure) && failure.Status == http.StatusNotFound {
			detail = "The commit this draft answers is no longer available. You can delete the draft."
		}
		return m.notify("warning", "Draft not opened", detail)
	}
	return m.composeTo(draft.Kind, msg.commit)
}

func (m *AppModel) deleteDraft(key string) tea.Cmd {
	delete(m.commitDrafts, key)
	m.drafts.selected = max(0, min(m.drafts.selected, len(m.commitDrafts)-1))
	return m.saveCommitDrafts()
}

// draftExcerptLength keeps the saved excerpt to about a line of the widest Drafts screen.
const draftExcerptLength = 120

// draftExcerpt is the target commit's text on one line, cut to draftExcerptLength runes.
func draftExcerpt(body string) string {
	excerpt := []rune(strings.Join(strings.Fields(body), " "))
	if len(excerpt) > draftExcerptLength {
		return string(excerpt[:draftExcerptLength-1]) + "…"
	}
	return string(excerpt)
}

// draftKindLabel names the draft and, when it was kept with one, whose commit it answers.
func draftKindLabel(draft auth.CommitDraft) string {
	to := ""
	if draft.TargetHandle != "" {
		to = " @" + safeText(draft.TargetHandle)
	}
	switch draft.Kind {
	case "reply":
		if to != "" {
			return "Reply to" + to
		}
		return "Reply"
	case "quote":
		if to != "" {
			return "Quote of" + to
		}
		return "Quote"
	}
	return "New commit"
}

func (m AppModel) draftsColumn(width int, p palette) string {
	column, _ := m.draftsLayout(width, p)
	return column
}

// draftsLayout draws the Drafts screen and its click targets, relative to the center column.
func (m AppModel) draftsLayout(width int, p palette) (string, []hitArea) {
	line := text(p.hairline).Render(strings.Repeat("─", width))
	header := split(text(p.ink).Bold(true).Render("Drafts"), text(p.muted).Render(m.icons.lock+" This device"), width-4)
	about := "Unsent commits, replies and quotes you kept, newest first."
	lines := []string{"", "  " + header, line, "  " + text(p.muted).Render(fit(about, width-4)), "", line}
	var areas []hitArea
	keys := m.draftList()
	if len(keys) == 0 {
		lines = append(lines, "", "  "+text(p.muted).Render(fit("No drafts. Keep a draft with Ctrl+K when you close the composer.", width-4)))
	}
	// Each draft takes three rows, four with an excerpt, so the window starts where the selected one stays on screen.
	room := max(1, (m.mainHeight()-len(lines)-4)/4)
	start := max(0, min(m.drafts.selected-room+1, len(keys)-room))
	for i := start; i < len(keys) && i < start+room; i++ {
		draft := m.commitDrafts[keys[i]]
		selected := i == m.drafts.selected
		title := draftKindLabel(draft)
		var extras []string
		if n := len(draft.Images); n > 0 {
			extras = append(extras, plural(n, "image", "images"))
		}
		if draft.Poll != nil {
			extras = append(extras, "poll")
		}
		if len(extras) > 0 {
			title += " · " + strings.Join(extras, ", ")
		}
		age := relativeAge(draft.SavedAt, m.now)
		body := strings.TrimSpace(strings.SplitN(draft.Body, "\n", 2)[0])
		if body == "" {
			body = "(no text)"
		}
		marker, style := "  ", text(p.ink)
		if selected {
			marker, style = text(p.blue).Render("› "), text(p.ink).Bold(true)
		}
		y := len(lines)
		lines = append(lines, marker+split(style.Render(title), text(p.muted).Render(age), width-4))
		if draft.TargetExcerpt != "" {
			lines = append(lines, "  "+text(p.hairline).Render("│ ")+text(p.muted).Italic(true).Render(fit(safeText(draft.TargetExcerpt), width-6)))
		}
		lines = append(lines, "  "+text(p.muted).Render(fit(safeText(body), width-4)))
		areas = append(areas, hitArea{uiRect{0, y, width, len(lines) - y}, "select", i})
		if selected {
			open, del := actionChip("Open", "enter", p), dangerChip("Delete", "D", p)
			if m.drafts.loading == keys[i] {
				open = actionChip("Opening...", "enter", p)
			}
			areas = append(areas, hitArea{uiRect{2, len(lines), lipgloss.Width(open), 1}, "enter", i}, hitArea{uiRect{4 + lipgloss.Width(open), len(lines), lipgloss.Width(del), 1}, "D", i})
			lines = append(lines, "  "+open+"  "+del)
		} else {
			lines = append(lines, "")
		}
	}
	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content), areas
}

func (m AppModel) draftsMouse(x, y, width int) (AppModel, tea.Cmd) {
	_, areas := m.draftsLayout(width, m.account.colors())
	for _, area := range areas {
		if !area.contains(x, y) {
			continue
		}
		if area.action == "select" {
			if m.drafts.selected == area.index {
				next, cmd, _ := m.draftsKey("enter")
				return next, cmd
			}
			m.drafts.selected = area.index
			return m, nil
		}
		m.drafts.selected = area.index
		next, cmd, _ := m.draftsKey(area.action)
		return next, cmd
	}
	return m, nil
}

// draftCount is the sidebar's badge for saved drafts, empty when there are none.
func draftCount(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}
