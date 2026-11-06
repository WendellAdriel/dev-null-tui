package tui

import (
	"fmt"
	"strings"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
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

// pulledByText is the web's PulledBy as text: "everyone" once more than three members all pulled, otherwise up to
// five handles and a count of the rest.
func pulledByText(pulled []api.User, total int) string {
	if len(pulled) == 0 {
		return ""
	}
	if len(pulled) == total && total > 3 {
		return "everyone"
	}
	handles := make([]string, 0, 5)
	for _, user := range pulled[:min(5, len(pulled))] {
		handles = append(handles, "@"+safeText(user.Handle))
	}
	text := strings.Join(handles, ", ")
	if len(pulled) > 5 {
		text += fmt.Sprintf(" +%d", len(pulled)-5)
	}
	return text
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
