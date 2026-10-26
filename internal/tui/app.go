package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

var logs = [3]api.Log{api.Following, api.Latest, api.Trending}
var logNames = [3]string{"Following", "Latest", "Trending"}

type feed struct {
	commits, suggestions []api.Commit
	submitted            []api.Commit
	selected             int
	viewport             viewport.Model
	starts, ends         []int
	pictures             []pictureArea
	next, status         string
	loadingMode          string
	loaded, loading      bool
	newCount, generation int
	cancel               context.CancelFunc
	cursors              map[string]bool
}

type logResult struct {
	index, generation int
	mode              string
	page              api.Page
	err               error
}
type gExpired struct{ generation int }
type pollLogs struct {
	session int
	at      time.Time
}
type clipboardResult struct {
	session   int
	url, what string
	err       error
}

type AppModel struct {
	account                                AccountModel
	connected, accountOpen, help, pendingG bool
	active, generation, session, lastLog   int
	feeds                                  [3]feed
	fetch                                  func(context.Context, api.Log, string, string) (api.Page, error)
	copyLink                               func(context.Context, string) error
	manualLink, notice                     string
	focused                                bool
	now                                    time.Time
	images                                 *graphics.Manager
	helpScroll                             int
	graphicsSupported                      bool
	composer                               composer
	submit                                 func(context.Context, api.NewCommit) (api.Commit, error)
	preview                                func(context.Context, string) (api.Preview, error)
	loadThread                             func(context.Context, string) (api.Thread, error)
	loadReplies                            func(context.Context, string, string) (api.Replies, error)
	deleteCommit                           func(context.Context, string) error
	threads                                []threadState
	threadGeneration                       int
	viewer                                 viewerState
	deleting                               deleteConfirm
	deletingID                             string
	toast                                  toastNotice
}

func NewApp(theme string, store auth.Store, origin string, ctx context.Context) AppModel {
	m := AppModel{account: NewAccount(theme, store, origin, ctx), copyLink: writeClipboard, focused: true, now: time.Now()}
	m.images = graphics.New(origin)
	m.images.Tmux = os.Getenv("TMUX") != ""
	for i := range m.feeds {
		m.feeds[i].viewport = viewport.New()
		m.feeds[i].cursors = make(map[string]bool)
	}
	return m
}

func (m AppModel) Init() tea.Cmd { return tea.Batch(m.account.Init(), tea.Raw(m.images.Probe())) }

func (m AppModel) GraphicsCleanup() string { return m.images.Clear() }
func (m AppModel) Close() error {
	if m.composer.cancel != nil {
		m.composer.cancel()
	}
	for _, f := range m.feeds {
		if f.cancel != nil {
			f.cancel()
		}
	}
	m.closeAllThreads()
	m.closeViewer()
	return m.account.Close()
}

func (m *AppModel) resetSequence() { m.pendingG = false; m.generation++ }

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	app := next.(AppModel)
	images := app.syncImages()
	return app, tea.Batch(cmd, images)
}

func (m AppModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case toastExpired:
		if msg.session == m.session && msg.id == m.toast.id {
			m.toast.visible = false
		}
		return m, nil
	case submissionResult:
		cmd := m.acceptSubmission(msg)
		return m, cmd
	case previewResult:
		cmd := m.acceptPreview(msg)
		return m, cmd
	case threadResult:
		cmd := m.acceptThread(msg)
		return m, cmd
	case repliesResult:
		cmd := m.acceptReplies(msg)
		return m, cmd
	case deleteResult:
		cmd := m.acceptDelete(msg)
		return m, cmd
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == m.images.ProbeID && string(msg.Payload) == "OK" {
			m.graphicsSupported = true
			m.images.Enabled = m.account.profile == colorprofile.TrueColor
			m.reflow()
		}
		return m, nil
	case graphics.Result:
		raw := m.images.Accept(msg)
		m.renderFeed(m.active)
		if len(m.threads) > 0 {
			m.renderThread(&m.threads[len(m.threads)-1])
		}
		if raw != "" {
			return m, tea.Raw(raw)
		}
		return m, nil
	case tea.MouseWheelMsg:
		return m.mouseWheel(msg)
	case tea.MouseClickMsg:
		return m.handleMouse(msg)
	case tea.MouseMotionMsg:
		m.dragComposer(msg.X, msg.Y)
		return m, nil
	case tea.MouseReleaseMsg:
		if m.composer.dragging {
			m.dragComposer(msg.X, msg.Y)
			m.composer.input.EndSelection()
			m.composer.dragging = false
			m.refreshEmoji()
		}
		return m, nil
	case gExpired:
		if msg.generation == m.generation {
			m.pendingG = false
		}
		return m, nil
	case verificationNoticeExpired:
		updated, cmd := m.account.Update(msg)
		m.account = updated.(AccountModel)
		return m, cmd
	case tea.BlurMsg:
		m.composer.input.EndSelection()
		m.composer.dragging = false
		m.focused = false
		m.resetSequence()
		return m, nil
	case tea.FocusMsg:
		m.focused = true
		m.resetSequence()
		return m, nil
	case tea.WindowSizeMsg, tea.BackgroundColorMsg, tea.ColorProfileMsg:
		m.composer.input.EndSelection()
		m.composer.dragging = false
		updated, cmd := m.account.Update(msg)
		m.account = updated.(AccountModel)
		m.images.Enabled = m.graphicsSupported && m.account.profile == colorprofile.TrueColor
		m.reflow()
		return m, cmd
	case pollLogs:
		if msg.session != m.session || !m.connected {
			return m, nil
		}
		m.now = msg.at
		for i := range m.feeds {
			m.renderFeed(i)
		}
		var cmd tea.Cmd
		if m.focused && !m.accountOpen && m.active != 2 && len(m.feeds[m.active].commits) > 0 {
			cmd = m.load("count")
		}
		return m, tea.Batch(cmd, m.poll())
	case clipboardResult:
		if msg.session != m.session || !m.connected || m.accountOpen {
			return m, nil
		}
		if msg.err != nil {
			m.closeViewer()
			m.manualLink = msg.url
			m.notice = "Clipboard unavailable. Select and copy the link below."
			return m, nil
		}
		m.notice = msg.what + " copied"
		if m.viewer.open {
			return m, m.notify("success", msg.what+" copied", safeText(msg.url))
		}
		return m, nil
	case logResult:
		if !m.connected || msg.index < 0 || msg.index >= len(m.feeds) || msg.generation != m.feeds[msg.index].generation {
			return m, nil
		}
		f := &m.feeds[msg.index]
		if f.cancel != nil {
			f.cancel()
			f.cancel = nil
		}
		f.loading = false
		if msg.err != nil {
			f.submitted = nil
			var failure *auth.APIError
			if m.revoked(msg.err) {
				if msg.mode == "refresh" {
					return m, m.notify("error", "Refresh failed", m.account.status)
				}
				return m, nil
			}
			f.status = msg.err.Error() + ". Ctrl+R to retry."
			if errors.As(msg.err, &failure) && failure.Status == http.StatusTooManyRequests && failure.RetryAfter != "" {
				f.status = "Rate limited. Retry after " + safeText(failure.RetryAfter) + ". Ctrl+R to retry."
			}
			if msg.mode == "refresh" {
				return m, m.notify("error", "Refresh failed", f.status)
			}
			return m, nil
		}
		if msg.mode == "count" {
			if msg.index != 2 {
				f.newCount = msg.page.NewCount
			}
			return m, nil
		}
		for i, submitted := range f.submitted {
			for _, current := range msg.page.Data {
				if current.ID == submitted.ID {
					f.submitted[i] = current
					break
				}
			}
		}
		msg.page.Data = uniqueCommits(append(f.submitted, msg.page.Data...))
		f.submitted = nil
		anchor, offset := "", 0
		if len(f.commits) > 0 && msg.mode != "refresh" {
			anchor = f.commits[f.selected].ID
			if f.selected < len(f.starts) {
				offset = f.viewport.YOffset() - f.starts[f.selected]
			}
		}
		if msg.mode == "more" {
			f.commits = uniqueCommits(append(f.commits, msg.page.Data...))
		} else {
			f.commits = uniqueCommits(msg.page.Data)
			f.suggestions = uniqueCommits(msg.page.Suggestions)
			f.cursors = make(map[string]bool)
		}
		f.selected = 0
		for i, c := range f.commits {
			if c.ID == anchor {
				f.selected = i
				break
			}
		}
		f.next, f.loaded, f.status, f.newCount = msg.page.Next, true, "", 0
		if f.cursors[f.next] {
			f.next = ""
			f.status = "Repeated pagination cursor. Ctrl+R to refresh."
		}
		m.renderFeed(msg.index)
		if msg.mode == "refresh" || anchor == "" {
			f.viewport.GotoTop()
		} else if f.selected < len(f.starts) {
			f.viewport.SetYOffset(max(0, f.starts[f.selected]+offset))
		}
		if msg.mode == "refresh" {
			return m, m.notify("success", "Log refreshed", logNames[msg.index]+" is up to date.")
		}
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "f2" {
			m.account.toggleTheme()
			m.resetSequence()
			m.reflow()
			return m, nil
		}
		if (m.connected && m.composer.open) || m.composer.confirm != "" {
			cmd := m.composerKey(msg)
			return m, cmd
		}
		if m.connected && m.deleting.id != "" && msg.String() != "ctrl+c" {
			cmd := m.deleteKey(msg.String())
			return m, cmd
		}
		if msg.String() == "ctrl+c" {
			if m.composer.input.Value() != "" {
				m.composer.open = true
				cmd := m.requestDraftAction("exit")
				return m, cmd
			}
			m.account.life.cancel()
			return m, tea.Quit
		}
		if m.connected && !m.accountOpen {
			next, cmd := m.handleKey(msg.String())
			return next, cmd
		}
		if m.accountOpen && msg.String() == "esc" && m.account.state == "connected" && !m.account.help && !m.account.showURL {
			m.accountOpen = false
			m.resetSequence()
			return m, nil
		}
	}
	if m.connected && m.composer.open && !m.composer.sending && !m.composer.previewing && m.composer.focus == 0 && m.composer.confirm == "" {
		cmd := m.updateComposerInput(msg)
		return m, cmd
	}
	if m.connected && !m.accountOpen {
		return m, nil
	}
	wasConnected := m.account.state == "connected"
	updated, cmd := m.account.Update(msg)
	m.account = updated.(AccountModel)
	if m.connected && m.account.user.ID == "" {
		m.leaveApplication()
	}
	if m.account.state == "connected" && !wasConnected && !m.connected {
		client, err := api.NewClient(m.account.origin, m.account.key)
		if err != nil {
			m.account.state, m.account.status = "failed", err.Error()
			return m, nil
		}
		if m.fetch == nil {
			m.fetch = client.Log
		}
		m.submit, m.preview = client.CreateCommit, client.Preview
		if m.loadThread == nil {
			m.loadThread, m.loadReplies, m.deleteCommit = client.Thread, client.Replies, client.DeleteCommit
		}
		m.connected, m.accountOpen = true, false
		m.account.workspace = true
		m.session++
		m.resetSequence()
		m.reflow()
		load := m.load("initial")
		return m, tea.Batch(cmd, load, m.poll())
	}
	return m, cmd
}

func (m *AppModel) leaveApplication() {
	m.toast.visible = false
	if m.composer.cancel != nil {
		m.composer.cancel()
		m.composer.cancel = nil
	}
	if m.composer.sending {
		m.composer.uncertain = true
		m.composer.status = "Outcome unknown. Check the web before retrying."
	}
	m.composer.open, m.composer.sending, m.composer.loadingPreview = false, false, false
	m.composer.generation++
	m.connected, m.accountOpen, m.help = false, false, false
	m.closeAllThreads()
	m.closeViewer()
	m.deleting, m.deletingID = deleteConfirm{}, ""
	if m.images != nil {
		m.images.Forget()
	}
	m.session++
	m.fetch = nil
	m.manualLink, m.notice = "", ""
	m.resetSequence()
	for i := range m.feeds {
		f := &m.feeds[i]
		if f.cancel != nil {
			f.cancel()
		}
		m.feeds[i] = feed{generation: f.generation + 1, viewport: viewport.New(), cursors: make(map[string]bool)}
	}
	m.account.user = auth.User{}
	m.account.workspace = false
}

func (m AppModel) poll() tea.Cmd {
	session := m.session
	return tea.Tick(time.Minute, func(at time.Time) tea.Msg { return pollLogs{session: session, at: at} })
}

func uniqueCommits(commits []api.Commit) []api.Commit {
	seen := make(map[string]bool, len(commits))
	result := make([]api.Commit, 0, len(commits))
	for _, c := range commits {
		if !seen[c.ID] {
			result = append(result, c)
			seen[c.ID] = true
		}
	}
	return result
}

func (m *AppModel) load(mode string) tea.Cmd {
	f := &m.feeds[m.active]
	if f.loading || (mode == "more" && f.next == "") || (mode == "count" && m.active == 2) {
		return nil
	}
	f.generation++
	f.loading = true
	f.loadingMode = mode
	f.status = ""
	ctx, cancel := context.WithTimeout(m.account.life.ctx, 30*time.Second)
	f.cancel = cancel
	index, generation, fetch := m.active, f.generation, m.fetch
	next, since := "", ""
	if mode == "more" {
		next = f.next
		f.cursors[next] = true
	}
	if mode == "count" && len(f.commits) > 0 {
		since = f.commits[0].ID
	}
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		page, err := fetch(ctx, logs[index], next, since)
		return logResult{index: index, generation: generation, mode: mode, page: page, err: err}
	})
}

func (m AppModel) handleKey(pressed string) (AppModel, tea.Cmd) {
	if pressed == "ctrl+c" {
		m.account.life.cancel()
		return m, tea.Quit
	}
	if m.manualLink != "" {
		if pressed == "esc" {
			m.manualLink = ""
		}
		return m, nil
	}
	if m.viewer.open {
		return m.viewerKey(pressed)
	}
	if pressed == "?" {
		m.help = !m.help
		m.helpScroll = 0
		m.resetSequence()
		return m, nil
	}
	if m.help {
		switch pressed {
		case "esc":
			m.help = false
		case "j", "down":
			m.helpScroll++
		case "k", "up":
			m.helpScroll--
		case "pgdown", "ctrl+d":
			m.helpScroll += 5
		case "pgup", "ctrl+u":
			m.helpScroll -= 5
		case "home":
			m.helpScroll = 0
		case "end", "G":
			m.helpScroll = len(m.bindings().all) + 10
		}
		m.helpScroll = max(0, min(m.helpMaxScroll(), m.helpScroll))
		return m, nil
	}
	if len(m.threads) > 0 {
		next, cmd, handled := m.threadKey(pressed)
		if handled || (pressed != "c" && pressed != "a") {
			return next, cmd
		}
		m = next
	}
	if m.pendingG {
		m.resetSequence()
		switch pressed {
		case "g":
			return m.selectCommit(0)
		case "l":
			return m.switchLog(m.lastLog)
		case "e":
			return m.switchLog(2)
		}
	}
	switch pressed {
	case "c":
		cmd := m.composeTo("", api.Commit{})
		return m, cmd
	case "enter", "r", "q", "v", "D":
		f := &m.feeds[m.active]
		if len(f.commits) == 0 {
			return m, nil
		}
		selected := f.commits[f.selected]
		switch pressed {
		case "r", "q":
			mode := map[string]string{"r": "reply", "q": "quote"}[pressed]
			return m, m.composeTo(mode, selected)
		case "v":
			return m, m.openViewer(selected, 0)
		case "D":
			m.confirmDelete(selected)
			return m, nil
		}
		return m, m.openThread(selected)
	case "g":
		m.pendingG = true
		m.generation++
		generation := m.generation
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return gExpired{generation} })
	case "tab", "right", "]", "l":
		return m.switchLog((m.active + 1) % 3)
	case "shift+tab", "left", "[":
		return m.switchLog((m.active + 2) % 3)
	case "1", "2", "3":
		return m.switchLog(int(pressed[0] - '1'))
	case "a":
		m.accountOpen = true
		m.resetSequence()
		return m, nil
	case "ctrl+r":
		m.notice = ""
		f := &m.feeds[m.active]
		if f.cancel != nil {
			f.cancel()
			f.loading = false
			f.generation++
		}
		cmd := m.load("refresh")
		expiry := m.notify("info", "Refreshing log", "Checking "+logNames[m.active]+" for the latest commits.")
		return m, tea.Batch(cmd, expiry)
	case "j", "down", "k", "up":
		delta := 1
		if pressed == "k" || pressed == "up" {
			delta = -1
		}
		return m.selectCommit(m.feeds[m.active].selected + delta)
	case "home":
		return m.selectCommit(0)
	case "G", "end":
		return m.selectCommit(len(m.feeds[m.active].commits) - 1)
	case "pgdown", "ctrl+d", "pgup", "ctrl+u":
		delta := 5
		if pressed == "pgup" || pressed == "ctrl+u" {
			delta = -5
		}
		return m.selectCommit(m.feeds[m.active].selected + delta)
	case "n":
		cmd := m.load("more")
		return m, cmd
	case "y":
		f := &m.feeds[m.active]
		if len(f.commits) == 0 {
			return m, nil
		}
		return m, m.copyCommitLink(f.commits[f.selected])
	case "esc":
		m.resetSequence()
		m.notice = ""
	}
	return m, nil
}

func (m AppModel) selectCommit(index int) (AppModel, tea.Cmd) {
	f := &m.feeds[m.active]
	if len(f.commits) == 0 {
		return m, nil
	}
	previous := f.selected
	f.selected = max(0, min(len(f.commits)-1, index))
	m.renderFeed(m.active)
	m.revealSelection()
	if f.selected == 0 {
		f.viewport.GotoTop()
	}
	if f.selected > previous && f.selected >= len(f.commits)-2 {
		cmd := m.load("more")
		return m, cmd
	}
	return m, nil
}

func (m AppModel) handleFeedMouse(msg tea.MouseClickMsg) (AppModel, tea.Cmd) {
	if m.composer.open {
		return m, nil
	}
	if !m.connected || m.accountOpen || m.help || m.manualLink != "" || msg.Button != tea.MouseLeft || m.account.width < 30 || m.account.height < 10 {
		return m, nil
	}
	nav, center, aside := m.columns()
	left := (m.account.width - nav - center - aside - 2) / 2
	x := msg.X - left - nav - 1
	if x < 0 || x >= center {
		return m, nil
	}
	if len(m.threads) > 0 {
		return m.threadMouse(x, msg.Y)
	}
	if msg.Y == 1 || msg.Y == 2 {
		return m.switchLog(min(2, x/(center/3)))
	}
	f := &m.feeds[m.active]
	if msg.Y < feedHeaderHeight || msg.Y >= m.mainHeight() {
		return m, nil
	}
	row := msg.Y - feedHeaderHeight + f.viewport.YOffset()
	for _, picture := range f.pictures {
		if picture.contains(x, row) {
			m, _ = m.selectCommit(picture.entry)
			return m, m.openViewer(f.commits[picture.entry], picture.image)
		}
	}
	for i, start := range f.starts {
		if row >= start && row < f.ends[i] {
			m, _ = m.selectCommit(i)
			return m, m.openThread(f.commits[i])
		}
	}
	return m, nil
}

func (m AppModel) switchLog(index int) (AppModel, tea.Cmd) {
	m.resetSequence()
	if index == m.active {
		return m, nil
	}
	if m.toast.kind == "info" {
		m.toast.visible = false
	}
	f := &m.feeds[m.active]
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
		f.loading = false
		f.generation++
	}
	m.active = index
	if index != 2 {
		m.lastLog = index
	}
	m.notice = ""
	m.reflow()
	if !m.feeds[index].loaded || len(m.feeds[index].submitted) > 0 {
		cmd := m.load("initial")
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) revealSelection() {
	f := &m.feeds[m.active]
	if f.selected >= len(f.starts) {
		return
	}
	start, end := f.starts[f.selected], f.ends[f.selected]
	if start < f.viewport.YOffset() || end-start >= f.viewport.Height() {
		f.viewport.SetYOffset(start)
	} else if end > f.viewport.YOffset()+f.viewport.Height() {
		f.viewport.SetYOffset(end - f.viewport.Height())
	}
}

func (m AppModel) feedStatus() string {
	f := m.feeds[m.active]
	if len(m.threads) > 0 && m.notice == "" {
		return m.threadStatus()
	}
	if m.pendingG {
		return "g ...  l Log  e Trending  (1 second)"
	}
	if m.notice != "" {
		return m.notice
	}
	if f.status != "" && f.loadingMode != "refresh" {
		return f.status
	}
	if f.loading && f.loadingMode != "refresh" {
		if !f.loaded {
			return "Reading from /dev/null..."
		}
		switch f.loadingMode {
		case "more":
			return "Loading older commits..."
		case "count":
			return "Checking for new commits..."
		}
		return "Updating log..."
	}
	if f.newCount > 0 {
		return fmt.Sprintf("%d new commits available", f.newCount)
	}
	if len(f.commits) == 0 {
		return "No commits in this log."
	}
	ending := "more available"
	if f.next == "" {
		ending = "end of log"
	}
	return fmt.Sprintf("%d / %d commits  ·  %s", f.selected+1, len(f.commits), ending)
}
