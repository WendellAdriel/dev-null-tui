package tui

import (
	"net/url"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

const hyperlinkStart = "\x1b]8;"

// hyperlinkTarget is the URI an OSC 8 sequence opens, or empty for the sequence that ends a link.
func hyperlinkTarget(sequence string) string {
	rest := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(sequence, hyperlinkStart), "\x1b\\"), "\x07")
	_, target, _ := strings.Cut(rest, ";")
	return target
}

// linkAt is the hyperlink drawn at column x of a rendered line.
func linkAt(line string, x int) string {
	target, column := "", 0
	var state byte
	for line != "" {
		sequence, width, n, next := ansi.DecodeSequence(line, state, nil)
		state = next
		switch {
		case strings.HasPrefix(sequence, hyperlinkStart):
			target = hyperlinkTarget(sequence)
		case width > 0:
			if x >= column && x < column+width {
				return target
			}
			column += width
		}
		line = line[n:]
	}
	return ""
}

// lineLinks lists the hyperlinks in rendered lines, in order and without repeats.
func lineLinks(lines []string) []string {
	var links []string
	for _, line := range lines {
		var state byte
		for line != "" {
			sequence, _, n, next := ansi.DecodeSequence(line, state, nil)
			state = next
			if strings.HasPrefix(sequence, hyperlinkStart) {
				if target := hyperlinkTarget(sequence); target != "" && !slices.Contains(links, target) {
					links = append(links, target)
				}
			}
			line = line[n:]
		}
	}
	return links
}

// serverPath is the path of a link to this server, or "" for a link elsewhere.
func (m AppModel) serverPath(raw string) string {
	u, err := url.Parse(raw)
	origin, originErr := url.Parse(m.account.origin)
	if err != nil || originErr != nil || !strings.EqualFold(u.Host, origin.Host) {
		return ""
	}
	return u.Path
}

// mentionHandle is the handle a link to a profile on this server names, such as a drawn @mention.
func (m AppModel) mentionHandle(raw string) string {
	handle, ok := strings.CutPrefix(m.serverPath(raw), "/@")
	if !ok || !api.ValidHandle(handle) {
		return ""
	}
	return handle
}

// mentionPath is the web's link to who a mention captured, which still leads to them after a rename.
func mentionPath(mention api.Mention) string {
	if mention.Kind == "organization" {
		return "/mentions/organizations/" + url.PathEscape(mention.ID)
	}
	return "/mentions/" + url.PathEscape(mention.ID)
}

// mentionTarget reads a link drawn by mentionPath back into the mention it opens.
func (m AppModel) mentionTarget(raw string) (api.Mention, bool) {
	path := m.serverPath(raw)
	if id, ok := strings.CutPrefix(path, "/mentions/organizations/"); ok && api.ValidMentionID(id) {
		return api.Mention{Kind: "organization", ID: id}, true
	}
	if id, ok := strings.CutPrefix(path, "/mentions/"); ok && api.ValidMentionID(id) {
		return api.Mention{Kind: "user", ID: id}, true
	}
	return api.Mention{}, false
}

// commitLinks are the web links in a commit's body and link card, leaving out @mentions, which u and the author cover.
func (m AppModel) commitLinks(c api.Commit) []string {
	if c.DeletedAt != nil {
		return nil
	}
	var links []string
	for _, link := range lineLinks(bodyLines(c, 0, m.account.colors(), m.statsStyle())) {
		if _, mention := m.mentionTarget(link); !mention && m.mentionHandle(link) == "" {
			links = append(links, link)
		}
	}
	if c.LinkPreview != nil && c.LinkPreview.URL != "" && !slices.Contains(links, c.LinkPreview.URL) {
		links = append(links, c.LinkPreview.URL)
	}
	return links
}

// openLinks opens the only link at once, or asks which one when there are several.
func (m *AppModel) openLinks(links []string) tea.Cmd {
	switch len(links) {
	case 0:
		m.notice = "No links to open."
		return nil
	case 1:
		return m.openLink(links[0])
	}
	m.pickerGeneration++
	d := &picker{kind: "links", title: "Open a link", generation: m.pickerGeneration}
	for _, link := range links {
		d.rows = append(d.rows, pickerRow{label: shortenURL(link), value: link})
	}
	m.picker = d
	return nil
}

// clickLink opens a clicked link: a profile on this server inside the app, anything else in the browser.
func (m *AppModel) clickLink(link string) tea.Cmd {
	if mention, ok := m.mentionTarget(link); ok {
		return m.openMention(mention)
	}
	if handle := m.mentionHandle(link); handle != "" {
		if m.safetyHidden[strings.ToLower(handle)]&2 != 0 || strings.EqualFold(handle, m.profileHandle()) || m.loadProfile == nil {
			return nil
		}
		s := m.pushProfile(profileState{handle: handle, mention: true})
		return tea.Batch(m.fetchProfile(s), m.fetchProfileList(s, false))
	}
	return m.openLink(link)
}
