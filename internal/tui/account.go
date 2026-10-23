package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type accountResult struct {
	generation int
	key        *auth.Key
	origin     string
	user       auth.User
	err        error
	replaceKey bool
}
type flowResult struct {
	generation int
	flow       *auth.Flow
	err        error
}
type linkResult struct {
	generation int
	link       string
	err        error
}
type browserResult struct {
	generation int
	err        error
}
type disconnectedResult struct {
	generation int
	err        error
}

type verificationNoticeExpired struct {
	generation int
}

const verificationNotice = "Connection verified just now"

type accountLife struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
	workMu sync.Mutex
	flows  []*auth.Flow
}

func (l *accountLife) work(work func() tea.Msg) tea.Cmd {
	return l.command(func() tea.Msg { l.workMu.Lock(); defer l.workMu.Unlock(); return work() })
}

func (l *accountLife) command(work func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			return nil
		}
		l.wg.Add(1)
		l.mu.Unlock()
		defer l.wg.Done()
		return work()
	}
}

type AccountModel struct {
	width, height               int
	theme                       string
	dark                        bool
	backgroundKnown             bool
	state                       string
	status                      string
	store                       auth.Store
	origin                      string
	key                         *auth.Key
	user                        auth.User
	flow                        *auth.Flow
	life                        *accountLife
	attemptCtx                  context.Context
	attemptCancel               context.CancelFunc
	generation                  int
	actionFocus                 int
	profile                     colorprofile.Profile
	showDeviceDetails           bool
	workspace, approvalRequired bool
	input                       textinput.Model
	pasting, help, showURL      bool
}

func NewAccount(theme string, store auth.Store, origin string, ctx context.Context) AccountModel {
	lifeCtx, cancel := context.WithCancel(ctx)
	input := textinput.New()
	input.Placeholder = "Paste the complete callback link"
	input.EchoMode, input.EchoCharacter, input.CharLimit = textinput.EchoPassword, '*', 2048
	return AccountModel{width: 80, height: 24, theme: theme, dark: theme != "light", state: "restoring", status: "Checking your saved connection...", store: store, origin: origin, input: input, life: &accountLife{ctx: lifeCtx, cancel: cancel}}
}

func (m AccountModel) Close() error {
	m.life.mu.Lock()
	m.life.closed = true
	m.life.cancel()
	m.life.mu.Unlock()
	m.life.wg.Wait()
	var err error
	for _, flow := range m.life.flows {
		err = errors.Join(err, flow.Close())
	}
	return err
}

func (m AccountModel) Init() tea.Cmd {
	commands := []tea.Cmd{m.life.work(func() tea.Msg {
		if err := auth.RecoverHandlers(m.life.ctx, m.store); err != nil {
			return accountResult{err: errors.New("interrupted browser handler cleanup failed; restart to retry")}
		}
		key, origin, user, err := auth.Restore(m.life.ctx, m.store, m.origin)
		return accountResult{key: key, origin: origin, user: user, err: err}
	})}
	if m.theme == "auto" {
		commands = append(commands, tea.RequestBackgroundColor)
	}
	return tea.Batch(commands...)
}

func (m AccountModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		m.input.SetWidth(max(1, min(m.width-10, 72)))
	case tea.BackgroundColorMsg:
		if m.theme == "auto" && !m.backgroundKnown {
			m.dark = msg.IsDark()
			m.backgroundKnown = true
		}
	case tea.ColorProfileMsg:
		m.profile = msg.Profile
	case verificationNoticeExpired:
		if msg.generation == m.generation && m.status == verificationNotice {
			m.status = ""
		}
	case accountResult:
		if msg.generation != m.generation {
			return m, nil
		}
		m.input.SetValue("")
		m.actionFocus = 0
		m.pasting = false
		if msg.replaceKey || msg.key != nil {
			m.key = msg.key
		}
		if msg.origin != "" {
			m.origin = msg.origin
		}
		m.user = msg.user
		if msg.err != nil {
			m.state, m.status = "failed", msg.err.Error()
			var failure *auth.APIError
			m.approvalRequired = errors.As(msg.err, &failure) && failure.Status == 401
		} else if msg.user.ID != "" {
			m.state, m.status = "connected", verificationNotice
			m.approvalRequired = false
		} else {
			m.state, m.status = "idle", "Connect your existing /dev/null account."
		}
		if m.flow != nil {
			m.flow = nil
			if m.attemptCancel != nil {
				m.attemptCancel()
			}
		}
		if m.state == "connected" {
			generation := m.generation
			return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return verificationNoticeExpired{generation: generation} })
		}
	case flowResult:
		if msg.generation != m.generation {
			if msg.flow != nil {
				flow := msg.flow
				return m, m.life.command(func() tea.Msg { _ = flow.Close(); return nil })
			}
			return m, nil
		}
		if msg.err != nil {
			m.state, m.status = "failed", msg.err.Error()
			return m, nil
		}
		m.flow, m.key, m.state, m.status = msg.flow, msg.flow.Key, "waiting", msg.flow.Notice
		m.actionFocus = 0
		flow, ctx, generation := m.flow, m.attemptCtx, m.generation
		return m, tea.Batch(m.life.command(func() tea.Msg { link, err := flow.Wait(ctx); return linkResult{generation, link, err} }), m.openBrowser())
	case linkResult:
		if msg.generation != m.generation || m.state != "waiting" {
			return m, nil
		}
		if msg.err != nil {
			return m.cancelAttempt("Approval timed out or was canceled. Connect again.")
		}
		return m.redeem(msg.link)
	case browserResult:
		if msg.generation == m.generation && msg.err != nil {
			m.status = msg.err.Error()
		}
	case disconnectedResult:
		if msg.generation != m.generation {
			return m, nil
		}
		if msg.err != nil {
			m.state, m.status = "failed", msg.err.Error()
		} else {
			m.key, m.user, m.state, m.status = nil, auth.User{}, "idle", "Local key deleted. Revoke the device at Settings > Devices on the web."
			m.showDeviceDetails = false
		}
		m.actionFocus = 0
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.life.cancel()
			return m, tea.Quit
		}
		if m.pasting {
			switch msg.String() {
			case "esc":
				m.pasting = false
				m.input.SetValue("")
				m.input.Blur()
				return m, nil
			case "enter":
				return m.redeem(m.input.Value())
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}
		pressed := msg.String()
		if !m.help && !m.showURL {
			actions := m.actions()
			if len(actions) > 0 {
				switch pressed {
				case "tab":
					m.actionFocus = (m.actionFocus + 1) % len(actions)
					return m, nil
				case "shift+tab":
					m.actionFocus = (m.actionFocus + len(actions) - 1) % len(actions)
					return m, nil
				case "enter":
					pressed = actions[m.actionFocus%len(actions)].key
				}
			}
		}
		if pressed == "esc" {
			if m.help || m.showURL {
				m.help, m.showURL = false, false
				return m, nil
			}
			if m.state == "confirm" {
				m.actionFocus = 0
				m.state = "connected"
				if m.user.ID == "" {
					m.state = "failed"
				}
				return m, nil
			}
			if m.state == "waiting" || m.state == "preparing" || m.state == "verifying" {
				return m.cancelAttempt("Connection canceled. You can try again.")
			}
		}
		if msg.String() == "?" {
			m.help = !m.help
			m.showURL = false
			return m, nil
		}
		if m.help {
			return m, nil
		}
		switch pressed {
		case "u":
			if m.state == "waiting" {
				m.showURL = !m.showURL
			}
		case "p":
			if m.state == "waiting" {
				m.pasting = true
				m.showURL = false
				return m, m.input.Focus()
			}
		case "b":
			if m.state == "waiting" {
				return m, m.openBrowser()
			}
		case "enter", "r":
			if m.state == "idle" || m.state == "failed" {
				return m.startAttempt()
			}
		case "delete-key":
			if m.state == "confirm" {
				return m.disconnect()
			}
		case "d":
			if m.state == "connected" || m.state == "failed" {
				m.state = "confirm"
				m.actionFocus = 0
				m.showURL = false
			}
		case "v":
			if m.key != nil && (m.state == "connected" || m.state == "failed") {
				return m.verify()
			}
		case "i":
			if m.state == "connected" && m.key != nil {
				m.showDeviceDetails = !m.showDeviceDetails
			}
		}
	default:
		if m.pasting {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m AccountModel) startAttempt() (tea.Model, tea.Cmd) {
	m.generation++
	m.attemptCtx, m.attemptCancel = context.WithTimeout(m.life.ctx, 10*time.Minute)
	m.state, m.status, m.help, m.showURL = "preparing", "Preparing a private device key and browser return...", false, false
	m.showDeviceDetails = false
	ctx, store, origin, deviceKey, generation := m.attemptCtx, m.store, m.origin, m.key, m.generation
	return m, m.life.work(func() tea.Msg {
		flow, err := auth.Begin(ctx, store, origin, deviceKey)
		if flow != nil {
			m.life.mu.Lock()
			m.life.flows = append(m.life.flows, flow)
			m.life.mu.Unlock()
		}
		return flowResult{generation, flow, err}
	})
}

func (m AccountModel) cancelAttempt(status string) (tea.Model, tea.Cmd) {
	if m.attemptCancel != nil {
		m.attemptCancel()
	}
	m.generation++
	m.input.SetValue("")
	m.pasting, m.showURL, m.state, m.status = false, false, "canceling", status
	generation := m.generation
	m.flow = nil
	m.key = nil
	return m, m.life.work(func() tea.Msg {
		var cleanup error
		m.life.mu.Lock()
		flows := append([]*auth.Flow(nil), m.life.flows...)
		m.life.mu.Unlock()
		for _, flow := range flows {
			cleanup = errors.Join(cleanup, flow.Close())
		}
		key, origin, _ := m.store.Load()
		return accountResult{generation: generation, key: key, origin: origin, replaceKey: true, err: errors.Join(errors.New(status), cleanup)}
	})
}

func (m AccountModel) redeem(raw string) (tea.Model, tea.Cmd) {
	if m.flow == nil {
		return m, nil
	}
	m.pasting, m.showURL, m.state, m.status = false, false, "verifying", "Redeeming approval, then verifying your account..."
	m.input.SetValue("")
	flow, ctx, generation := m.flow, m.attemptCtx, m.generation
	return m, m.life.work(func() tea.Msg {
		user, err := flow.Redeem(ctx, raw)
		err = errors.Join(err, flow.Close())
		key, origin, loadErr := m.store.Load()
		if loadErr != nil && (!os.IsNotExist(loadErr) || err == nil) {
			err = errors.Join(err, loadErr)
		}
		return accountResult{generation: generation, user: user, key: key, origin: origin, replaceKey: true, err: err}
	})
}

func (m AccountModel) openBrowser() tea.Cmd {
	if m.flow == nil {
		return nil
	}
	raw, generation := m.flow.Attempt.URL, m.generation
	return m.life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.life.ctx, 10*time.Second)
		defer cancel()
		return browserResult{generation, auth.OpenBrowser(ctx, raw)}
	})
}

func (m AccountModel) verify() (tea.Model, tea.Cmd) {
	m.generation++
	m.attemptCtx, m.attemptCancel = context.WithCancel(m.life.ctx)
	m.state, m.status = "verifying", "Verifying the saved device with signed GET /api/v1/me..."
	generation, deviceKey, origin := m.generation, m.key, m.origin
	ctx := m.attemptCtx
	return m, m.life.work(func() tea.Msg {
		defer m.attemptCancel()
		client, err := auth.NewClient(origin, deviceKey)
		if err != nil {
			return accountResult{generation: generation, err: err}
		}
		user, err := client.Me(ctx)
		return accountResult{generation: generation, user: user, err: err}
	})
}

func (m AccountModel) disconnect() (tea.Model, tea.Cmd) {
	m.generation++
	m.state, m.status = "disconnecting", "Deleting the local device key..."
	generation := m.generation
	return m, m.life.work(func() tea.Msg { return disconnectedResult{generation, m.store.Disconnect()} })
}

func safeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, ansi.Strip(value))
}

func (m AccountModel) bindings() accountKeys {
	bindings := []key.Binding{key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit"))}
	add := func(keys, label string) {
		bindings = append(bindings, key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, label)))
	}
	if m.help || m.showURL {
		add("esc", "back")
		return accountKeys{bindings}
	}
	if m.pasting {
		add("enter", "submit link")
		add("esc", "back")
		return accountKeys{bindings}
	}
	switch m.state {
	case "idle":
		add("enter", "connect")
	case "failed":
		if m.key != nil && !m.approvalRequired {
			add("r", "new approval")
		} else {
			add("r", "retry")
		}
		add("d", "disconnect")
	case "waiting":
		add("b", "browser")
		add("u", "URL")
		add("p", "paste link")
		add("esc", "cancel")
	case "preparing", "verifying":
		add("esc", "cancel")
	case "connected":
		if m.workspace {
			add("esc", "workspace")
		}
		add("v", "verify")
		add("d", "disconnect")
		if m.key != nil {
			label := "device details"
			if m.showDeviceDetails {
				label = "hide details"
			}
			add("i", label)
		}
	case "confirm":
		add("enter", "choose")
		add("esc", "keep key")
	}
	if len(m.actions()) > 1 {
		add("tab", "select")
		if m.state != "confirm" {
			add("enter", "choose")
		}
	}
	if m.state == "failed" && m.key != nil {
		add("v", "verify")
	}
	add("?", "help")
	return accountKeys{bindings}
}

type accountKeys struct{ bindings []key.Binding }

func (k accountKeys) ShortHelp() []key.Binding  { return k.bindings }
func (k accountKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.bindings} }
