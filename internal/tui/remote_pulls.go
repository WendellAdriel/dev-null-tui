package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

// orgPulls mirrors the web's pullsOf: the members other than you and the message's author who pulled it, out of
// everyone who could.
func orgPulls(members []api.Member, message api.Message, viewer string) (pulled []api.User, total int) {
	for _, member := range members {
		if member.User.ID == viewer || member.User.ID == message.UserID {
			continue
		}
		total++
		if member.ReadThroughID >= message.ID {
			pulled = append(pulled, member.User)
		}
	}
	return pulled, total
}

// pulledBy follows the web's PulledBy: everyone once more than three members all pulled, otherwise up to five
// members and a count of the rest.
func pulledBy(pulled []api.User, total int) (shown []api.User, rest int, everyone bool) {
	if len(pulled) > 0 && len(pulled) == total && total > 3 {
		return nil, 0, true
	}
	shown = pulled[:min(5, len(pulled))]
	return shown, len(pulled) - len(shown), false
}

// pulledByFaces draws pulledBy as 2x1 avatars and "+N", counting the avatars that don't fit the width with the rest.
func (m AppModel) pulledByFaces(pulled []api.User, total, width int, p palette) (string, []graphics.Request) {
	shown, rest, everyone := pulledBy(pulled, total)
	if everyone {
		if width < lipgloss.Width("everyone") {
			return "", nil
		}
		return text(p.muted).Render("everyone"), nil
	}
	if len(pulled) == 0 {
		return "", nil
	}
	for n := len(shown); n >= 0; n-- {
		var faces []string
		var requests []graphics.Request
		for i := range shown[:n] {
			req, lines := m.userAvatar(&shown[i], 2, 1, p)
			faces, requests = append(faces, lines[0]), append(requests, req)
		}
		if hidden := rest + len(shown) - n; hidden > 0 {
			faces = append(faces, text(p.muted).Render(fmt.Sprintf("+%d", hidden)))
		}
		if row := strings.Join(faces, " "); lipgloss.Width(row) <= width {
			return row, requests
		}
	}
	return "", nil
}

// orgStatusIndex is the message whose pull status an org thread shows: the latest one that isn't an event, unless a
// push is still pending.
func orgStatusIndex(messages []api.Message, pending int) int {
	if pending > 0 {
		return -1
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Event == "" {
			return i
		}
	}
	return -1
}
