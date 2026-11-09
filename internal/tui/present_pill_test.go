package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func TestBackToPresentPillCountsNewMessagesAndGJumps(t *testing.T) {
	m, _ := messagingApp(t)
	m = openDirect(t, m)
	var history []api.Message
	for i := range 40 {
		history = append(history, remoteMessage(int64(100+i), "ada", fmt.Sprint("line ", i), m.now))
	}
	th := m.remotes.thread
	m.acceptRemoteThread(remoteThreadResult{session: m.session, generation: th.generation, thread: api.RemoteThread{Access: "open", Messages: history}})
	if strings.Contains(threadText(m), "Back to present") {
		t.Fatal("the pill must hide at the newest message")
	}
	m.remotes.thread.viewport.GotoTop()
	if !strings.Contains(threadText(m), "↓ Back to present") {
		t.Fatalf("scrolled up, the pill must show:\n%s", threadText(m))
	}
	arrivals := []api.Message{remoteMessage(200, "ada", "new one", m.now), remoteMessage(201, "ada", "and another", m.now)}
	m.acceptRemoteThread(remoteThreadResult{session: m.session, generation: m.remotes.thread.generation, mode: "poll", thread: api.RemoteThread{Access: "open", Messages: arrivals}})
	if !strings.Contains(threadText(m), "↓ 2 new") {
		t.Fatalf("messages that arrive while scrolled up must be counted:\n%s", threadText(m))
	}
	m.remotes.focusThread = true
	next, _ := m.remotesKey("G")
	m = next
	if !m.remotes.thread.viewport.AtBottom() || m.remotes.thread.unseen != 0 || strings.Contains(threadText(m), "↓ ") {
		t.Fatal("G must jump to the newest message and clear the count")
	}
}
