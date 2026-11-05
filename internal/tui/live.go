package tui

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const defaultLiveHours, liveNowRows = 2, 3

// liveCalls are the live stream routes, which tests replace.
type liveCalls struct {
	goLive    func(context.Context, string, int) (auth.User, error)
	end       func(context.Context) (auth.User, error)
	following func(context.Context) ([]api.User, error)
}

func newLiveCalls(client *api.Client) liveCalls {
	return liveCalls{client.GoLive, client.EndStream, client.LiveFollowing}
}

// liveDialog is the web's Go live form: a stream link and how many hours to show it, with focus 1 on the hours.
type liveDialog struct {
	field        textinput.Model
	hours, focus int
	// ending asks to confirm End stream, so one stray L can't stop a broadcast.
	ending  bool
	sending bool
	status  string
}

type liveResult struct {
	session int
	ending  bool
	user    auth.User
	err     error
}

type liveNowResult struct {
	session int
	people  []api.User
	err     error
}

func (m *AppModel) toggleLive() tea.Cmd {
	if m.liveCalls.goLive == nil || m.live != nil {
		return nil
	}
	if m.account.user.Live.On(m.now) {
		m.live = &liveDialog{ending: true}
		return nil
	}
	field := textinput.New()
	field.Placeholder, field.CharLimit = "https://www.twitch.tv/you", 2048
	m.live = &liveDialog{field: field, hours: defaultLiveHours}
	return m.live.field.Focus()
}

func (m *AppModel) liveKey(msg tea.KeyPressMsg) tea.Cmd {
	d := m.live
	pressed := msg.String()
	switch {
	case pressed == "esc":
		if !d.sending {
			m.live = nil
		}
		return nil
	case d.ending:
		if pressed == "enter" {
			m.live = nil
			return m.endStream()
		}
		return nil
	case pressed == "enter":
		return m.goLive()
	case d.sending:
		return nil
	case pressed == "tab" || pressed == "shift+tab" || pressed == "up" || pressed == "down":
		d.focus = 1 - d.focus
		if d.focus == 0 {
			return d.field.Focus()
		}
		d.field.Blur()
		return nil
	case d.focus == 1:
		switch pressed {
		case "left", "h", "-":
			d.hours = max(1, d.hours-1)
		case "right", "l", "+":
			d.hours = min(api.MaxLiveHours, d.hours+1)
		}
		return nil
	}
	var cmd tea.Cmd
	d.field, cmd = d.field.Update(msg)
	d.status = ""
	return cmd
}

func (m *AppModel) goLive() tea.Cmd {
	d := m.live
	if d.sending {
		return nil
	}
	link := strings.TrimSpace(d.field.Value())
	if link == "" {
		d.status = "Paste a link to your stream on YouTube or Twitch."
		return nil
	}
	d.sending, d.status = true, "Going live..."
	session, hours, start := m.session, d.hours, m.liveCalls.goLive
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		user, err := start(ctx, link, hours)
		return liveResult{session: session, user: user, err: err}
	})
}

func (m *AppModel) endStream() tea.Cmd {
	if m.liveEnding || m.liveCalls.end == nil {
		return nil
	}
	m.liveEnding = true
	session, end := m.session, m.liveCalls.end
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	return tea.Batch(m.notify("info", "Ending your stream", "Waiting for the server to confirm."), m.account.life.command(func() tea.Msg {
		defer cancel()
		user, err := end(ctx)
		return liveResult{session: session, ending: true, user: user, err: err}
	}))
}

func (m *AppModel) acceptLive(msg liveResult) tea.Cmd {
	if msg.session != m.session {
		return nil
	}
	if msg.ending {
		m.liveEnding = false
	}
	if msg.err != nil {
		if m.revoked(msg.err) {
			return nil
		}
		if msg.ending {
			return m.notify("error", "Couldn't end your stream", safeText(msg.err.Error())+".")
		}
		if m.live != nil {
			m.live.sending, m.live.status = false, liveError(msg.err)
		}
		return nil
	}
	m.applyAccount(msg.user)
	m.reflow()
	if msg.ending {
		return m.notify("success", "Stream ended", "Your avatar no longer shows you as live.")
	}
	m.live = nil
	return m.notify("success", "You're live", "Your avatar and commits show you as live until "+liveUntil(msg.user.Live)+".")
}

// liveError is the server's reason, such as an unsupported platform, or the client's own check.
func liveError(err error) string {
	var failure *auth.APIError
	if errors.As(err, &failure) && len(failure.FieldErrors) > 0 {
		return safeText(strings.Join(failure.FieldErrors, " "))
	}
	message := []rune(safeText(err.Error()))
	if len(message) == 0 {
		return "Couldn't go live."
	}
	message[0] = unicode.ToUpper(message[0])
	return "Couldn't go live: " + string(message) + "."
}

// profileLive is the stream a profile shows. Your own comes from your account, which Go live and End stream update.
func (m AppModel) profileLive(s *profileState) *auth.LiveStream {
	if strings.EqualFold(s.profile.User.Handle, m.account.user.Handle) {
		return m.account.user.Live
	}
	return s.profile.User.Live
}

func liveUntil(live *auth.LiveStream) string {
	if live == nil {
		return "it ends"
	}
	return live.EndsAt.In(time.Local).Format("15:04")
}

func (m AppModel) fetchLiveNow() tea.Cmd {
	if m.liveCalls.following == nil {
		return nil
	}
	session, load, base := m.session, m.liveCalls.following, m.account.life.ctx
	return m.account.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		people, err := load(ctx)
		return liveNowResult{session: session, people: people, err: err}
	})
}

func (m *AppModel) acceptLiveNow(msg liveNowResult) tea.Cmd {
	if msg.session != m.session {
		return nil
	}
	if msg.err != nil {
		// Older servers don't have the route, so the aside simply shows nobody live.
		if m.revoked(msg.err) {
			return nil
		}
		m.liveNow = nil
		return nil
	}
	m.liveNow = m.liveNow[:0]
	for _, person := range msg.people {
		if person.Live.On(m.now) && len(m.liveNow) < liveNowRows {
			m.liveNow = append(m.liveNow, person)
		}
	}
	return nil
}

// watchLive opens a live person's stream, from their profile or the Live now aside.
func (m *AppModel) watchLive(live *auth.LiveStream) tea.Cmd {
	if !live.On(m.now) {
		return nil
	}
	return m.openLink(live.URL)
}
