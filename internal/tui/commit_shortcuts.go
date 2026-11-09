package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// commitShortcut handles the keys every commit list shares: y copies the link, Q opens the quoted commit, and
// T copies the text of a commit the account wrote or can post as.
func (m *AppModel) commitShortcut(c api.Commit, pressed string) (tea.Cmd, bool) {
	switch pressed {
	case "y":
		return m.copyCommitLink(c), true
	case "Q":
		return m.openQuote(c), true
	case "T":
		if !m.canCopyText(c) {
			return nil, true
		}
		return m.copyCommitText(c), true
	case "S":
		return m.openStargazers(c), true
	case "P":
		return m.startPin(c), true
	case "t":
		return m.toggleTranslation(c), true
	case "x":
		return m.openLinks(m.commitLinks(c)), true
	case ".":
		return m.openCommitMenu(c), true
	}
	return nil, false
}

func (m *AppModel) openQuote(c api.Commit) tea.Cmd {
	if c.DeletedAt != nil || c.QuotedID == nil && c.Quote == nil {
		return nil
	}
	if q := c.Quote; q == nil || q.DeletedAt != nil || q.Author == nil {
		m.notice = "The quoted commit is no longer available"
		return nil
	}
	return m.openThread(*c.Quote)
}

func (m AppModel) canCopyText(c api.Commit) bool {
	if c.DeletedAt != nil || c.Author == nil || strings.TrimSpace(c.Body) == "" {
		return false
	}
	if m.ownCommit(c) {
		return true
	}
	if !c.Author.Organization() || m.account.user.Organizations == nil {
		return false
	}
	for _, o := range *m.account.user.Organizations {
		if o.ID != "" && c.Author.ID != "" {
			if o.ID == c.Author.ID {
				return true
			}
			continue
		}
		if strings.EqualFold(o.Handle, c.Author.Handle) {
			return true
		}
	}
	return false
}
