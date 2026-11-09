package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type liveLog struct {
	calls, opened []string
	err           error
	people        []api.User
}

func stream(m AppModel, platform string, left time.Duration) *auth.LiveStream {
	return &auth.LiveStream{Platform: platform, URL: "https://www.twitch.tv/" + platform + "-stream", ThumbnailURL: "https://static.example.test/" + platform + ".jpg", EndsAt: m.now.Add(left)}
}

func liveApp(t *testing.T) (AppModel, *liveLog) {
	t.Helper()
	m, _ := profileApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = next.(AppModel)
	log := &liveLog{}
	m.liveCalls = liveCalls{
		goLive: func(_ context.Context, link string, hours int) (auth.User, error) {
			log.calls = append(log.calls, "go "+link+" "+plural(hours, "hour", "hours"))
			if log.err != nil {
				return auth.User{}, log.err
			}
			u := m.account.user
			u.Live = stream(m, "twitch", time.Duration(hours)*time.Hour)
			return u, nil
		},
		end: func(context.Context) (auth.User, error) {
			log.calls = append(log.calls, "end")
			u := m.account.user
			u.Live = nil
			return u, nil
		},
		following: func(context.Context) ([]api.User, error) {
			log.calls = append(log.calls, "following")
			return log.people, nil
		},
	}
	m.openURL = func(_ context.Context, raw string) error {
		log.opened = append(log.opened, raw)
		return nil
	}
	return m, log
}

func TestLiveAuthorsShowALiveTagAndARedRing(t *testing.T) {
	m, _ := liveApp(t)
	live, ended := fixtureCommit("live"), fixtureCommit("ended")
	live.Author.Live = stream(m, "twitch", time.Hour)
	ended.Author = &api.Author{Name: "Linus", Handle: "linus", Live: stream(m, "youtube", -time.Minute)}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{live, ended}})
	content := ansi.Strip(m.feeds[0].viewport.GetContent())
	if !strings.Contains(content, "@ada LIVE") || strings.Contains(content, "@linus LIVE") {
		t.Fatalf("live tags:\n%s", content)
	}
	p := colors(m.account.dark)
	if authorRequest(live.Author, p, m.now, 4, 2).Ring != p.danger || authorRequest(ended.Author, p, m.now, 4, 2).Ring == p.danger {
		t.Fatal("only a live author gets the red ring")
	}
}

func TestTheGoLiveDialogKeepsItsFieldsOnASmallTerminal(t *testing.T) {
	m, log := liveApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m = compose(t, next.(AppModel), "L")
	log.err = &auth.APIError{Status: 422, FieldErrors: []string{"Only YouTube and Twitch streams can go live."}}
	next, _ = m.Update(tea.PasteMsg{Content: "https://example.test/stream"})
	m = compose(t, next.(AppModel), "enter")
	frame := frameText(m)
	for _, want := range []string{"Stream link", "Show me as live for", "2 hours", "Only YouTube and Twitch", "Go live"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q at 60x18:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Share your") {
		t.Fatalf("the explanation should give way at 60x18:\n%s", frame)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	if frame := frameText(next.(AppModel)); !strings.Contains(frame, "Share your") {
		t.Fatalf("the explanation shows when it fits:\n%s", frame)
	}
}

func TestGoingLiveAndEndingTheStream(t *testing.T) {
	m, log := liveApp(t)
	m = compose(t, m, "L")
	if m.live == nil || m.live.hours != 2 || !strings.Contains(frameText(m), "Stream link") || !strings.Contains(frameText(m), "2 hours") {
		t.Fatalf("go live dialog:\n%s", frameText(m))
	}
	m = compose(t, m, "enter")
	if len(log.calls) != 0 || !strings.Contains(m.live.status, "Paste a link") {
		t.Fatalf("empty link: %v %q", log.calls, m.live.status)
	}
	next, _ := m.Update(tea.PasteMsg{Content: "https://www.twitch.tv/fixture"})
	m = compose(t, next.(AppModel), "tab", "right", "left", "right", "right")
	log.err = &auth.APIError{Status: 422, FieldErrors: []string{"Only YouTube and Twitch streams can go live."}}
	m = compose(t, m, "enter")
	if strings.Join(log.calls, "|") != "go https://www.twitch.tv/fixture 4 hours" || m.live == nil || m.live.status != "Only YouTube and Twitch streams can go live." {
		t.Fatalf("refused: %v %+v", log.calls, m.live)
	}
	log.err = nil
	m = compose(t, m, "enter")
	if m.live != nil || !m.account.user.Live.On(m.now) || m.toast.title != "You're live" {
		t.Fatalf("live: %+v %q", m.account.user.Live, m.toast.title)
	}
	if frame := frameText(m); !strings.Contains(frame, "End stream") || !strings.Contains(frame, "LIVE on Twitch") {
		t.Fatalf("aside while live:\n%s", frame)
	}
	m = settle(t, m, "g", "p")
	if frame := profileText(m); !strings.Contains(frame, "End stream  L") || !strings.Contains(frame, "LIVE on Twitch · until") || !strings.Contains(frame, "Watch  W") {
		t.Fatalf("own profile while live:\n%s", frame)
	}
	m, _ = clickText(t, m, "End stream  L")
	if m.live == nil || !m.live.ending || !strings.Contains(frameText(m), "End your stream?") {
		t.Fatalf("end stream asks first:\n%s", frameText(m))
	}
	m = compose(t, m, "esc")
	if m.live != nil || log.calls[len(log.calls)-1] == "end" || !m.account.user.Live.On(m.now) {
		t.Fatalf("keep streaming: %v", log.calls)
	}
	m = compose(t, m, "L", "enter")
	if log.calls[len(log.calls)-1] != "end" || m.account.user.Live != nil || m.toast.title != "Stream ended" {
		t.Fatalf("end stream: %v", log.calls)
	}
	if frame := profileText(m); !strings.Contains(frame, "Go live  L") || strings.Contains(frame, "LIVE on") {
		t.Fatalf("own profile after the stream:\n%s", frame)
	}
}

func TestWatchingAStreamFromAProfileAndLiveNow(t *testing.T) {
	m, log := liveApp(t)
	load := m.loadProfile
	m.loadProfile = func(ctx context.Context, handle string) (api.Profile, error) {
		profile, err := load(ctx, handle)
		profile.User.Live = stream(m, "youtube", time.Hour)
		return profile, err
	}
	m = settle(t, m, "u", "W")
	if strings.Join(log.opened, " ") != "https://www.twitch.tv/youtube-stream" {
		t.Fatalf("watch from the profile: %v", log.opened)
	}
	m = settle(t, m, "esc")
	log.people = []api.User{
		{ID: "id-linus", Handle: "linus", Name: "Linus", Live: stream(m, "twitch", time.Hour)},
		{ID: "id-grace", Handle: "grace", Name: "Grace", Live: stream(m, "youtube", -time.Hour)},
	}
	m = settleCmd(m, m.fetchLiveNow())
	frame := frameText(m)
	if !strings.Contains(frame, "Live now") || !strings.Contains(frame, "@linus · Twitch") || strings.Contains(frame, "@grace") {
		t.Fatalf("live now aside:\n%s", frame)
	}
	m, cmd := clickText(t, m, "@linus · Twitch")
	settleCmd(m, cmd)
	if log.opened[len(log.opened)-1] != "https://www.twitch.tv/twitch-stream" {
		t.Fatalf("watch from live now: %v", log.opened)
	}
}

func TestABrowserFailureNamesThePageToOpen(t *testing.T) {
	m, _ := liveApp(t)
	m.openURL = func(context.Context, string) error {
		return errors.New("browser could not open; use the displayed authorization URL")
	}
	m = settleCmd(m, m.openLink("https://www.twitch.tv/linus"))
	if m.toast.title != "Couldn't open the browser" || m.toast.detail != "Open https://www.twitch.tv/linus yourself." {
		t.Fatalf("toast: %q %q", m.toast.title, m.toast.detail)
	}
}

func TestTheEmptyLinkMessageStaysThroughCursorBlinks(t *testing.T) {
	m, _ := liveApp(t)
	m = compose(t, m, "L", "enter")
	next, _ := m.Update(idleMsg{})
	m = next.(AppModel)
	if !strings.Contains(m.live.status, "Paste a link") {
		t.Fatalf("status: %q", m.live.status)
	}
}

func TestGoingLiveReachesYourQuotedCommits(t *testing.T) {
	m, _ := liveApp(t)
	c := fixtureCommit("q")
	quoted := fixtureCommit("mine")
	quoted.Author.ID, quoted.Author.Handle = "fixture", "fixture"
	c.Quote = &quoted
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	m = compose(t, m, "L")
	next, _ := m.Update(tea.PasteMsg{Content: "https://www.twitch.tv/fixture"})
	m = compose(t, next.(AppModel), "enter")
	if !m.feeds[0].commits[0].Quote.Author.Live.On(m.now) {
		t.Fatal("your quoted commit doesn't show you as live")
	}
}

func TestAStreamEndsOnTheMinuteTickOnAnOpenProfile(t *testing.T) {
	m, _ := liveApp(t)
	load := m.loadProfile
	m.loadProfile = func(ctx context.Context, handle string) (api.Profile, error) {
		profile, err := load(ctx, handle)
		profile.User.Live = stream(m, "twitch", 30*time.Second)
		return profile, err
	}
	m = settle(t, m, "u")
	if !strings.Contains(profileText(m), "LIVE on Twitch") {
		t.Fatalf("live profile:\n%s", profileText(m))
	}
	next, _ := m.Update(clockTick{session: m.session, at: m.now.Add(time.Minute)})
	m = next.(AppModel)
	if strings.Contains(profileText(m), "LIVE on") || strings.Contains(profileText(m), "Watch") {
		t.Fatalf("an ended stream still shows:\n%s", profileText(m))
	}
}

// TestGoLiveIsNotDrawnLikeADestructiveAction keeps red for deletes and blocks, so Go live and its dialogs use blue.
func TestGoLiveIsNotDrawnLikeADestructiveAction(t *testing.T) {
	m, _ := liveApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = compose(t, next.(AppModel), "L")
	p := colors(m.account.dark)
	var r, g, b int
	if _, err := fmt.Sscanf(p.danger, "#%02x%02x%02x", &r, &g, &b); err != nil {
		t.Fatal(err)
	}
	red := fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
	dialog, _, _ := m.liveModal(p)
	m.live.ending = true
	ending, _, _ := m.liveModal(p)
	for name, view := range map[string]string{"Go live": dialog, "End stream": ending, "profile button": m.liveAction(p)} {
		if strings.Contains(view, red) {
			t.Errorf("%s is drawn in the destructive red: %q", name, view)
		}
	}
}

func TestWatchShortcutOpensTheOnlyStreamOrAsksWhichOne(t *testing.T) {
	m, log := liveApp(t)
	m = settle(t, m, "W")
	if m.notice != "Nobody you follow is live now." || len(log.opened) != 0 {
		t.Fatalf("W with nobody live: %q %v", m.notice, log.opened)
	}
	log.people = []api.User{{ID: "id-linus", Handle: "linus", Name: "Linus", Live: stream(m, "twitch", time.Hour)}}
	m = settleCmd(m, m.fetchLiveNow())
	m = settle(t, m, "W")
	if m.picker != nil || strings.Join(log.opened, " ") != "https://www.twitch.tv/twitch-stream" {
		t.Fatalf("W with one stream: picker %v, opened %v", m.picker != nil, log.opened)
	}
	log.people = append(log.people, api.User{ID: "id-grace", Handle: "grace", Name: "Grace", Live: stream(m, "youtube", time.Hour)})
	m = settleCmd(m, m.fetchLiveNow())
	m = settle(t, m, "W")
	if m.picker == nil || m.picker.kind != "live" || !strings.Contains(frameText(m), "@grace") {
		t.Fatalf("W with two streams did not ask which one:\n%s", frameText(m))
	}
	m = pickSecond(m)
	if m.picker != nil || log.opened[len(log.opened)-1] != "https://www.twitch.tv/youtube-stream" {
		t.Fatalf("picking the second stream: %v", log.opened)
	}
}
