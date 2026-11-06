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
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/audio"
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
	actions              []hitArea
	next, status         string
	loadingMode          string
	loaded, loading      bool
	generation           int
	// arrivals are the new commits the reader hasn't seen yet, newest first, for the New commits notice.
	arrivals []api.Commit
	cancel   context.CancelFunc
	cursors  map[string]bool
}

type logResult struct {
	index, generation int
	mode              string
	page              api.Page
	err               error
}
type gExpired struct{ generation int }

// clockTick advances the clock once a minute, so ages, live streams, and Live now stay current.
type clockTick struct {
	session int
	at      time.Time
}

// clipboardResult reports a copy. text marks a commit's text rather than a link, with link the commit's own page for
// when the text is too long to show for copying by hand.
type clipboardResult struct {
	session         int
	url, what, link string
	text            bool
	err             error
}

type AppModel struct {
	account                                AccountModel
	messaging                              messagingState
	voice                                  voiceState
	connected, accountOpen, help, pendingG bool
	active, generation, session, lastLog   int
	feeds                                  [3]feed
	fetch                                  func(context.Context, api.Log, string, string) (api.Page, error)
	copyLink                               func(context.Context, string) error
	manualLink, manualPage, notice         string
	manualText                             bool
	focused                                bool
	now                                    time.Time
	images                                 *graphics.Manager
	helpScroll                             int
	graphicsSupported                      bool
	composer                               composer
	submit                                 func(context.Context, api.NewCommit) (api.Commit, error)
	preview                                func(context.Context, string) (api.Preview, error)
	loadThread                             func(context.Context, string) (api.Thread, error)
	loadReplies                            func(context.Context, string, string) (api.CommitPage, error)
	deleteCommit                           func(context.Context, string) error
	threads                                []threadState
	threadGeneration                       int
	viewer                                 viewerState
	deleting                               deleteConfirm
	deletingID                             string
	toast                                  toastNotice
	whatsNew                               whatsNewState
	updates                                Updates
	upgrade                                upgradeState
	tuiSettings                            auth.TUISettings
	engagement                             engagementState
	setStar                                func(context.Context, string, bool) (api.Commit, error)
	givePastel                             func(context.Context, string) (api.Pastel, error)
	vote                                   voteState
	unfolded                               map[string]bool
	imageDir                               string
	castVote                               func(context.Context, string, string) (api.Commit, error)
	notificationsOpen                      bool
	notificationMask                       uint8
	notificationFeeds                      map[string]*notificationFeed
	notificationGeneration                 int
	fetchNotifications                     func(context.Context, string, string) (api.Notifications, error)
	fetchNotificationGroup                 func(context.Context, string, string, string) (api.Notifications, error)
	fetchNotificationCount                 func(context.Context) (int, error)
	unreadNotifications                    int
	notificationCountGeneration            int
	logTicks, badgeTicks                   int
	// logAway and badgeAway mark ticks scheduled at the away pace, which focus restarts.
	logAway, badgeAway         bool
	notificationCountCancel    context.CancelFunc
	deletedNotificationCommits map[string]time.Time
	loadProfile                func(context.Context, string) (api.Profile, error)
	loadUserCommits            func(context.Context, string, bool, string) (api.CommitPage, error)
	loadConnections            func(context.Context, string, api.Connections, string) (api.People, error)
	setFollow                  func(context.Context, string, bool) error
	profiles                   []profileState
	profileGeneration          int
	follow                     followState
	icons                      iconSet
	version                    string
	settings                   settingsState
	settingsAPI                *api.Client
	safetyHidden               map[string]uint8
	starredOpen                bool
	starred                    feed
	starredRemoved             map[string]bool
	loadStarred                func(context.Context, string) (api.CommitPage, error)
	remotes                    remotesState
	remoteCalls                remoteCalls
	organizationCalls          organizationCalls
	claiming                   string
	openURL                    func(context.Context, string) error
	liveCalls                  liveCalls
	live                       *liveDialog
	liveEnding                 bool
	liveNow                    []api.User
	meCall                     func(context.Context) (auth.User, error)
	accountRefresh             accountRefresh
	search                     *searchDialog
	completionCalls            completionCalls
	completionGeneration       int
	searchGeneration           int
	searchCalls                searchCalls
	// commitSearch is set while the Starred list screen shows that search's matching commits instead.
	commitSearch string
}

func NewApp(theme string, store auth.Store, origin string, ctx context.Context) AppModel {
	m := AppModel{account: NewAccount(theme, store, origin, ctx), copyLink: writeClipboard, openURL: auth.OpenBrowser, focused: true, now: time.Now(), notificationMask: 63, icons: nerdIcons}
	m.tuiSettings = store.TUISettings()
	m.images = graphics.New(origin)
	images := m.images
	m.voice.openSound, m.voice.record = audio.OpenDevice, audio.StartRecording
	m.voice.download = func(ctx context.Context, url string) ([]byte, error) {
		return images.Download(ctx, url, audio.MaxUploadBytes)
	}
	m.images.Tmux = os.Getenv("TMUX") != ""
	for i := range m.feeds {
		m.feeds[i].viewport = viewport.New()
		m.feeds[i].cursors = make(map[string]bool)
	}
	return m
}

func (m AppModel) Init() tea.Cmd { return tea.Batch(m.account.Init(), tea.Raw(m.images.Probe())) }

func (m AppModel) WithVersion(version string) AppModel {
	m.version = version
	m.whatsNew.version, m.whatsNew.seen, m.whatsNew.connected = version, m.account.store.LastSeenVersion(), m.account.store.HasConnection()
	return m
}

func (m AppModel) GraphicsCleanup() string { return m.images.Clear() }
func (m AppModel) Close() error {
	m.cancelRecording()
	if m.voice.device != nil {
		m.voice.device.Close()
	}
	if m.engagement.cancel != nil {
		m.engagement.cancel()
	}
	m.cancelNotifications()
	m.closeStarred()
	m.closeRemotes()
	if m.remotes.countCancel != nil {
		m.remotes.countCancel()
	}
	m.invalidateNotificationCount()
	if m.composer.cancel != nil {
		m.composer.cancel()
	}
	for _, f := range m.feeds {
		if f.cancel != nil {
			f.cancel()
		}
	}
	if m.follow.cancel != nil {
		m.follow.cancel()
	}
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	return m.account.Close()
}

func (m *AppModel) resetSequence() { m.pendingG = false; m.generation++ }

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	app := next.(AppModel)
	app.settleArrivals()
	images := app.syncImages()
	return app, tea.Batch(cmd, images)
}

func (m AppModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case settingsResult:
		cmd := m.acceptSettings(msg)
		return m, cmd
	case settingsPeopleResult:
		cmd := m.acceptSafetyList(msg)
		return m, cmd
	case settingsLoadResult:
		cmd := m.acceptSettingsLoad(msg)
		return m, cmd
	case remotesInboxResult:
		return m, m.acceptInbox(msg)
	case remoteThreadResult:
		return m, m.acceptRemoteThread(msg)
	case remotesTick:
		return m, m.acceptRemotesTick(msg)
	case badgeTick:
		return m, m.acceptBadgeTick(msg)
	case logTick:
		return m, m.acceptLogTick(msg)
	case remoteActionResult:
		return m, m.acceptRemoteAction(msg)
	case peopleSearchTick:
		return m, m.acceptPeopleTick(msg)
	case peopleSearchResult:
		return m, m.acceptPeopleResult(msg)
	case pinResult:
		return m, m.acceptPin(msg)
	case membersResult:
		return m, m.acceptMembers(msg)
	case peopleSaveResult:
		return m, m.acceptPeopleSave(msg)
	case inboxPeopleResult:
		m.acceptInboxPeople(msg)
		return m, nil
	case remoteLinkPreviewResult:
		return m, m.acceptLinkPreview(msg)
	case remoteSendResult:
		return m, m.acceptRemoteSend(msg)
	case remotePingResult:
		return m, m.acceptRemotePing(msg)
	case remoteReactionResult:
		return m, m.acceptReaction(msg)
	case reactionUsageResult:
		if msg.session == m.session {
			m.messaging.usage = msg.usage
		}
		return m, nil
	case voiceLoaded:
		return m, m.acceptVoiceLoaded(msg)
	case voiceTick:
		return m, m.acceptVoiceTick(msg)
	case recordingTick:
		return m, m.acceptRecordingTick(msg)
	case volumeSaveTick:
		return m, m.acceptVolumeSave(msg)
	case draftSaveTick:
		m.acceptDraftSave(msg)
		return m, nil
	case remotesCountResult:
		if msg.session == m.session && !m.remotes.open {
			m.remotes.unread = max(0, msg.unread)
		}
		return m, nil
	case starredResult:
		msg.page.Data = m.safeCommits(msg.page.Data)
		cmd := m.acceptStarred(msg)
		return m, cmd
	case engagementResult:
		cmd := m.acceptEngagement(msg)
		return m, cmd
	case voteResult:
		cmd := m.acceptVote(msg)
		return m, cmd
	case voteReloaded:
		m.acceptPollReload(msg)
		return m, nil
	case notificationResult:
		msg.page.Data = m.safeNotifications(msg.page.Data)
		return m, m.acceptNotifications(msg)
	case notificationGroupResult:
		msg.page.Data = m.safeNotifications(msg.page.Data)
		return m, m.acceptNotificationGroup(msg)
	case notificationCountResult:
		if msg.session != m.session || msg.generation != m.notificationCountGeneration || !m.connected {
			return m, nil
		}
		if m.notificationCountCancel != nil {
			m.notificationCountCancel()
			m.notificationCountCancel = nil
		}
		if msg.err != nil {
			m.revoked(msg.err)
			return m, nil
		}
		m.unreadNotifications = msg.count
		return m, nil
	case whatsNewSaved:
		cmd := m.acceptWhatsNewSaved(msg)
		return m, cmd
	case upgradeChecked:
		m.acceptUpgradeChecked(msg)
		return m, nil
	case upgradeInstalled:
		cmd := m.acceptUpgradeInstalled(msg)
		return m, cmd
	case tuiSettingsSaved:
		cmd := m.acceptTUISettingsSaved(msg)
		return m, cmd
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
		msg.thread.Ancestors = m.safeCommits(msg.thread.Ancestors)
		if msg.err == nil {
			commits := m.safeCommitsFor([]api.Commit{msg.thread.Commit}, true)
			if len(commits) == 0 {
				msg.err = &auth.APIError{Status: http.StatusNotFound}
			} else {
				msg.thread.Commit = commits[0]
			}
		}
		cmd := m.acceptThread(msg)
		return m, cmd
	case repliesResult:
		msg.replies.Data = m.safeCommits(msg.replies.Data)
		cmd := m.acceptReplies(msg)
		return m, cmd
	case deleteResult:
		cmd := m.acceptDelete(msg)
		return m, cmd
	case profileResult:
		if msg.profile.Pinned != nil {
			commits := m.safeCommitsFor([]api.Commit{*msg.profile.Pinned}, true)
			if len(commits) == 0 {
				msg.profile.Pinned = nil
			} else {
				msg.profile.Pinned = &commits[0]
			}
		}
		return m, m.acceptProfile(msg)
	case profileListResult:
		msg.commits.Data = m.safeCommitsFor(msg.commits.Data, true)
		return m, m.acceptProfileList(msg)
	case followResult:
		return m, m.acceptFollow(msg)
	case claimResult:
		return m, m.acceptClaim(msg)
	case authorCheckResult:
		return m, m.acceptAuthorCheck(msg)
	case liveResult:
		return m, m.acceptLive(msg)
	case completionTick:
		return m, m.acceptCompletionTick(msg)
	case completionResult:
		return m, m.acceptCompletionResult(msg)
	case searchTick:
		return m, m.acceptSearchTick(msg)
	case searchResult:
		return m, m.acceptSearchResult(msg)
	case liveNowResult:
		return m, m.acceptLiveNow(msg)
	case followedOrganizationsResult:
		return m, m.acceptFollowedOrganizations(msg)
	case pageOpened:
		return m, m.acceptLink(msg)
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
		m.renderStarred()
		m.renderRemoteThread()
		if m.notificationsOpen {
			m.renderNotifications()
		}
		if m.threadOnTop() {
			m.renderThread(&m.threads[len(m.threads)-1])
		}
		if s := m.topProfile(); s != nil {
			m.renderProfile(s)
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
			return m, m.refreshSuggestions()
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
		if !m.connected {
			return m, nil
		}
		polls := m.restartPolls()
		if m.remotesOnTop() {
			m.now = time.Now()
			return m, tea.Batch(polls, m.pollRemotes(), m.markRemoteRead())
		}
		return m, polls
	case tea.WindowSizeMsg, tea.BackgroundColorMsg, tea.ColorProfileMsg:
		m.composer.input.EndSelection()
		m.composer.dragging = false
		updated, cmd := m.account.Update(msg)
		m.account = updated.(AccountModel)
		m.images.Enabled = m.graphicsSupported && m.account.profile == colorprofile.TrueColor
		m.reflow()
		return m, cmd
	case clockTick:
		if msg.session != m.session || !m.connected {
			return m, nil
		}
		if m.settings.signingOut() {
			return m, m.tickClock()
		}
		m.now = msg.at
		for i := range m.feeds {
			m.renderFeed(i)
		}
		m.renderStarred()
		if m.notificationsOpen {
			m.renderNotifications()
		}
		m.renderRemoteThread()
		if m.threadOnTop() {
			m.renderThread(&m.threads[len(m.threads)-1])
		}
		if s := m.topProfile(); s != nil {
			m.renderProfile(s)
		}
		return m, tea.Batch(m.tickClock(), m.refreshAside())
	case meResult:
		m.acceptAccountRefresh(msg)
		return m, nil
	case clipboardResult:
		if msg.session != m.session || !m.connected || m.accountOpen {
			return m, nil
		}
		if msg.err != nil {
			m.closeViewer()
			m.manualLink, m.manualText, m.manualPage = msg.url, msg.text, msg.link
			m.notice = "Clipboard unavailable. Select and copy the " + m.manualNoun() + " below."
			return m, nil
		}
		m.notice = msg.what + " copied"
		if m.viewer.open {
			return m, m.notify("success", msg.what+" copied", safeText(msg.url))
		}
		return m, nil
	case logResult:
		msg.page.Data = m.safeCommits(msg.page.Data)
		msg.page.Suggestions = m.safeCommits(msg.page.Suggestions)
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
			kind := "error"
			f.status = msg.err.Error() + ". Ctrl+R to retry."
			if errors.As(msg.err, &failure) && failure.Status == http.StatusTooManyRequests && failure.RetryAfter != "" {
				kind, f.status = "warning", "Rate limited. Retry after "+safeText(failure.RetryAfter)+". Ctrl+R to retry."
			}
			if msg.mode == "refresh" {
				return m, m.notify(kind, "Refresh failed", f.status)
			}
			return m, nil
		}
		if msg.mode == "count" {
			if msg.index == 2 {
				return m, nil
			}
			if msg.page.NewCount > 0 && msg.index == m.active {
				return m, m.load("new")
			}
			return m, nil
		}
		if msg.mode == "new" {
			if m.acceptNewest(msg.index, msg.page) == 0 {
				return m, nil
			}
			return m, m.refreshAside()
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
		f.next, f.loaded, f.status = msg.page.Next, true, ""
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
			return m, m.notify("success", "Refreshed", logNames[msg.index]+" is up to date.")
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
			wasExit := m.composer.confirm == "exit"
			cmd := m.composerKey(msg)
			if wasExit && m.settings.exitFromSettings && m.composer.confirm == "" && m.account.life.ctx.Err() == nil {
				m.composer.open = false
				m.composer.input.Blur()
				m.settings.dialog, m.settings.exitDialog, m.settings.exitFromSettings = m.settings.exitDialog, "", false
				if m.settings.dialog != "" {
					return m, tea.Batch(cmd, m.settings.input.Focus())
				}
			}
			return m, cmd
		}
		if m.connected && m.search != nil && msg.String() != "ctrl+c" {
			cmd := m.searchKey(msg)
			return m, cmd
		}
		if msg.String() == "ctrl+k" && !m.searchBlocked() {
			cmd := m.openSearch("")
			return m, cmd
		}
		if m.connected && m.live != nil && msg.String() != "ctrl+c" {
			cmd := m.liveKey(msg)
			return m, cmd
		}
		if m.connected && m.deleting.id != "" && msg.String() != "ctrl+c" {
			cmd := m.deleteKey(msg.String())
			return m, cmd
		}
		if menu := m.activeReactions(); menu != nil && msg.String() != "ctrl+c" {
			cmd := m.reactionKey(msg)
			return m, cmd
		}
		if m.activeMessageComposer() != nil && msg.String() != "ctrl+c" {
			cmd := m.messageComposerKey(msg)
			return m, cmd
		}
		if m.connected && !m.accountOpen && m.settings.dialog != "" && msg.String() != "ctrl+c" {
			cmd := m.settingsDialogKey(msg)
			return m, cmd
		}
		if msg.String() == "ctrl+c" {
			if m.composer.hasDraft() {
				if m.settings.open || m.settings.dialog != "" {
					m.settings.exitDialog, m.settings.exitFromSettings = m.settings.dialog, true
					m.settings.dialog = ""
					m.settings.input.Blur()
				}
				m.composer.open = true
				cmd := m.requestDraftAction("exit")
				return m, cmd
			}
			return m, m.quit()
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
	if m.connected && m.search != nil {
		// Pastes and cursor blinks reach the search box.
		return m, m.updateSearchField(msg)
	}
	if m.connected && !m.accountOpen && m.settings.dialog != "" {
		if !m.settings.busy {
			var cmd tea.Cmd
			m.settings.input, cmd = m.settings.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.connected && m.live != nil {
		// Pastes and cursor blinks reach the link field, which is how most people enter a stream link.
		if m.live.ending || m.live.sending || m.live.focus != 0 {
			return m, nil
		}
		before := m.live.field.Value()
		var cmd tea.Cmd
		m.live.field, cmd = m.live.field.Update(msg)
		if m.live.field.Value() != before {
			m.live.status = ""
		}
		return m, cmd
	}
	if mc := m.activeMessageComposer(); mc != nil {
		if paste, ok := msg.(tea.PasteMsg); ok {
			return m, m.messagePaste(paste)
		}
		if mc.browser == nil && !mc.editing && !mc.stripFocus {
			return m, m.updateMessageInput(msg)
		}
	}
	if picker := m.composer.picker; m.connected && m.composer.open && picker != nil {
		if !picker.typed || picker.checking {
			return m, nil
		}
		before := picker.field.Value()
		var cmd tea.Cmd
		picker.field, cmd = picker.field.Update(msg)
		if picker.field.Value() != before {
			picker.status = ""
		}
		return m, cmd
	}
	if m.connected && m.composer.open && !m.composer.sending && !m.composer.previewing && m.composer.confirm == "" {
		if paste, ok := msg.(tea.PasteMsg); ok {
			cmd := m.composerPaste(paste)
			return m, cmd
		}
		if m.composer.focus == 0 && m.composer.browser == nil && m.composer.editing == "" {
			cmd := m.updateComposerInput(msg)
			return m, cmd
		}
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
		m.settingsAPI = client
		m.setStar, m.givePastel, m.castVote = client.SetStar, client.GivePastel, client.Vote
		m.loadStarred = client.Starred
		if m.remoteCalls.inbox == nil {
			m.remoteCalls = newRemoteCalls(client)
		}
		m.startMessaging()
		m.fetchNotifications, m.fetchNotificationGroup, m.fetchNotificationCount = client.Notifications, client.NotificationGroup, client.NotificationCount
		if m.loadThread == nil {
			m.loadThread, m.loadReplies, m.deleteCommit = client.Thread, client.Replies, client.DeleteCommit
		}
		if m.loadProfile == nil {
			m.loadProfile, m.loadUserCommits, m.loadConnections, m.setFollow = client.Profile, client.UserCommits, client.Connections, client.SetFollow
		}
		if m.organizationCalls.load == nil {
			m.organizationCalls = newOrganizationCalls(client)
		}
		if m.liveCalls.goLive == nil {
			m.liveCalls = newLiveCalls(client)
		}
		if m.meCall == nil {
			m.meCall = client.CurrentUser
		}
		if m.searchCalls.profiles == nil {
			m.searchCalls = newSearchCalls(client)
		}
		if m.completionCalls.mentions == nil {
			m.completionCalls = newCompletionCalls(client)
		}
		m.connected, m.accountOpen = true, false
		m.account.workspace = true
		m.session++
		if m.account.user.UnreadNotifications != nil {
			m.unreadNotifications = max(0, *m.account.user.UnreadNotifications)
		}
		if m.account.user.Remotes != nil {
			m.remotes.unread = max(0, m.account.user.Remotes.UnreadCount)
		}
		m.resetSequence()
		m.reflow()
		load := m.load("initial")
		return m, tea.Batch(cmd, load, m.tickClock(), m.scheduleLogCheck(), m.scheduleBadges(), m.startWhatsNew(), m.startUpgradeCheck(), m.fetchLiveNow())
	}
	return m, cmd
}

func (m *AppModel) leaveApplication() {
	// Cleared first, so closing the thread below can't write the drafts back.
	m.messaging = messagingState{}
	m.closeVoice()
	m.settings.generation++
	m.settings = settingsState{generation: m.settings.generation}
	m.settingsAPI = nil
	m.safetyHidden = nil
	if m.engagement.cancel != nil {
		m.engagement.cancel()
	}
	m.engagement = engagementState{generation: m.engagement.generation + 1}
	m.setStar, m.givePastel = nil, nil
	m.closeStarred()
	m.loadStarred = nil
	m.closeRemotes()
	if m.remotes.countCancel != nil {
		m.remotes.countCancel()
	}
	m.remotes, m.remoteCalls = remotesState{generation: m.remotes.generation + 1, tickGeneration: m.remotes.tickGeneration + 1}, remoteCalls{}
	m.cancelNotifications()
	m.invalidateNotificationCount()
	m.notificationsOpen = false
	m.notificationFeeds = nil
	m.deletedNotificationCommits = nil
	m.notificationMask = 63
	m.unreadNotifications = 0
	m.fetchNotifications, m.fetchNotificationGroup, m.fetchNotificationCount = nil, nil, nil
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
	if m.follow.cancel != nil {
		m.follow.cancel()
	}
	m.follow = followState{generation: m.follow.generation + 1}
	m.claiming = ""
	m.live, m.liveEnding, m.liveNow = nil, false, nil
	m.closeSearch()
	// Cleared so a later connection builds them on its own signing key.
	m.organizationCalls, m.liveCalls, m.searchCalls, m.completionCalls = organizationCalls{}, liveCalls{}, searchCalls{}, completionCalls{}
	m.meCall, m.accountRefresh = nil, accountRefresh{}
	m.closeAllProfiles()
	m.closeAllThreads()
	m.closeViewer()
	m.deleting, m.deletingID = deleteConfirm{}, ""
	m.vote, m.unfolded = voteState{}, nil
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

func (m AppModel) tickClock() tea.Cmd {
	session := m.session
	return tea.Tick(time.Minute, func(at time.Time) tea.Msg { return clockTick{session: session, at: at} })
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
	if (mode == "more" && f.next == "") || (backgroundLoad(mode) && m.active == 2) {
		return nil
	}
	if f.loading {
		// A quiet check for new commits gives way to a load the reader asked for.
		if !backgroundLoad(f.loadingMode) || backgroundLoad(mode) {
			return nil
		}
		f.cancel()
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
	var known map[string]bool
	if mode == "new" {
		known = make(map[string]bool, len(f.commits))
		for _, c := range f.commits {
			known[c.ID] = true
		}
	}
	return m.account.life.command(func() tea.Msg {
		defer cancel()
		page, err := fetch(ctx, logs[index], next, since)
		if mode == "new" && err == nil {
			page, err = newestThrough(ctx, fetch, logs[index], page, known)
		}
		return logResult{index: index, generation: generation, mode: mode, page: page, err: err}
	})
}

func (m AppModel) handleKey(pressed string) (AppModel, tea.Cmd) {
	if pressed == "ctrl+c" {
		return m, m.quit()
	}
	if m.manualLink != "" {
		if pressed == "esc" {
			m.manualLink = ""
		}
		return m, nil
	}
	if m.whatsNew.open {
		cmd := m.whatsNewKey(pressed)
		return m, cmd
	}
	if m.upgradeVisible() {
		cmd := m.upgradeKey(pressed)
		return m, cmd
	}
	if m.viewer.open {
		return m.viewerKey(pressed)
	}
	if m.vote.commitID != "" {
		cmd := m.voteKey(pressed)
		return m, cmd
	}
	if pressed == "?" && !m.remotesSearching() {
		m.help = !m.help
		m.helpScroll = 0
		m.resetSequence()
		return m, nil
	}
	// Remotes keeps / for its inbox search, as the web does, and Ctrl+K still opens this one there.
	if pressed == "/" && !m.remotesOnTop() && !m.searchBlocked() {
		return m, m.openSearch("")
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
	if m.settings.open {
		cmd := m.settingsKey(pressed)
		return m, cmd
	}
	if m.remotesOnTop() && (m.remotes.dialog != "" || m.remotes.searching || m.remotesOverlay() || (!m.pendingG && pressed != "g" && pressed != "c" && pressed != "a")) {
		return m.remotesKey(pressed)
	}
	if pressed == "!" {
		cmd := m.beginReport()
		return m, cmd
	}
	if pressed == "o" && !m.pendingG && m.startVoting() {
		return m, nil
	}
	if pressed == "m" && !m.pendingG && m.showMore() {
		return m, nil
	}
	if m.profileOnTop() && (pressed == "M" || pressed == "B") {
		kind := "mute"
		if pressed == "B" {
			kind = "block"
		}
		cmd := m.beginProfileSafety(kind)
		return m, cmd
	}
	if pressed == "L" && !m.pendingG {
		return m, m.toggleLive()
	}
	if m.threadOnTop() && !m.pendingG && pressed != "g" {
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
			if m.threadOnTop() {
				m.selectEntry(&m.threads[len(m.threads)-1], 0)
				return m, nil
			}
			if m.profileOnTop() {
				return m, m.selectProfileEntry(0)
			}
			if m.starredOnTop() {
				cmd := m.selectStarred(0)
				return m, cmd
			}
			if m.remotesOnTop() {
				return m.remotesKey("home")
			}
			return m.selectCommit(0)
		case "l":
			return m.switchLog(m.lastLog)
		case "e":
			return m.switchLog(2)
		case "n":
			return m.openNotifications()
		case "p":
			return m, m.openProfile(m.account.user.Handle)
		case "s":
			return m.openStarred()
		case "r":
			return m.openRemotes()
		case ",":
			cmd := m.openSettings()
			return m, cmd
		}
	}
	if m.profileOnTop() {
		next, cmd, handled := m.profileKey(pressed)
		if handled || (pressed != "c" && pressed != "a" && pressed != "g") {
			return next, cmd
		}
		m = next
	}
	if m.notificationsOpen && pressed != "c" && pressed != "a" && pressed != "g" {
		return m.notificationKey(pressed)
	}
	if m.starredOnTop() && pressed != "c" && pressed != "a" && pressed != "g" {
		return m.starredKey(pressed)
	}
	switch pressed {
	case "c":
		cmd := m.composeTo("", api.Commit{})
		return m, cmd
	case "enter", "r", "q", "v", "D", "s", "p", "u", "y", "Q", "T":
		f := &m.feeds[m.active]
		if len(f.commits) == 0 {
			return m, nil
		}
		selected := f.commits[f.selected]
		if cmd, ok := m.commitShortcut(selected, pressed); ok {
			return m, cmd
		}
		switch pressed {
		case "s":
			return m, m.startEngagement(selected, "star")
		case "p":
			return m, m.startEngagement(selected, "pastel")
		case "r", "q":
			mode := map[string]string{"r": "reply", "q": "quote"}[pressed]
			return m, m.composeTo(mode, selected)
		case "v":
			return m, m.openViewer(selected, 0)
		case "D":
			m.confirmDelete(selected)
			return m, nil
		case "u":
			if selected.DeletedAt != nil || !hasProfile(selected.Author) {
				return m, nil
			}
			return m, m.openAuthor(selected.Author)
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
		expiry := m.notify("info", "Refreshing", "Checking "+m.refreshScope()+".")
		return m, tea.Batch(cmd, m.loadNotificationCount(), m.refreshRemotesCount(), m.refreshAside(), expiry)
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
	if m.threadOnTop() {
		return m.threadMouse(x, msg.Y)
	}
	if m.profileOnTop() {
		return m.profileMouse(x, msg.Y)
	}
	if m.notificationsOpen {
		return m.notificationMouse(x, msg.Y)
	}
	if m.starredOnTop() {
		return m.starredMouse(x, msg.Y)
	}
	if msg.Y == 1 || msg.Y == 2 {
		return m.switchLog(min(2, x/(center/3)))
	}
	f := &m.feeds[m.active]
	if msg.Y < feedHeaderHeight || msg.Y >= m.mainHeight() {
		return m, nil
	}
	if pill, at := m.arrivalsPill(center, colors(m.account.dark)); pill != "" && msg.Y >= feedHeaderHeight && msg.Y < feedHeaderHeight+lipgloss.Height(pill) && x >= at && x < at+lipgloss.Width(pill) {
		f.arrivals = nil
		return m.selectCommit(0)
	}
	row := msg.Y - feedHeaderHeight + f.viewport.YOffset()
	for _, area := range f.actions {
		if area.contains(x, row) {
			return m.mouseKey(area.action)
		}
	}
	for _, picture := range f.pictures {
		if !picture.contains(x, row) {
			continue
		}
		m, _ = m.selectCommit(picture.entry)
		if !picture.poll {
			return m, m.openViewer(f.commits[picture.entry], picture.index)
		}
		if m.voteOn(picture.index) {
			return m, nil
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
	left := m.notificationsOpen || m.starredOpen || m.remotes.open || len(m.profiles) > 0
	if left {
		m.closeStarred()
		m.closeRemotes()
		m.cancelNotifications()
		m.notificationsOpen = false
		m.closeAllProfiles()
		m.closeAllThreads()
		m.closeViewer()
	}
	if index == m.active && !left {
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
	if m.threadOnTop() && m.notice == "" {
		return m.threadStatus()
	}
	if m.pendingG {
		if m.remotesEnabled() {
			return "g ...  l Log  e Trending  n Notifications  r Remotes  p Profile  s Starred  (1 second)"
		}
		return "g ...  l Log  e Trending  n Notifications  p Profile  s Starred  (1 second)"
	}
	if m.profileOnTop() && m.notice == "" {
		return m.profileStatus()
	}
	if m.notice != "" {
		return m.notice
	}
	if m.notificationsOpen {
		return m.notificationStatus()
	}
	if m.starredOpen {
		return m.starredStatus()
	}
	if m.remotes.open {
		return m.remotesStatus()
	}
	if f.status != "" && f.loadingMode != "refresh" {
		return f.status
	}
	// The 15-second checks for new commits run quietly, so the footer doesn't flicker with each one.
	if f.loading && f.loadingMode != "refresh" && f.loadingMode != "count" && f.loadingMode != "new" {
		if !f.loaded {
			return "Reading from /dev/null..."
		}
		switch f.loadingMode {
		case "more":
			return "Loading older commits..."
		}
		return "Updating log..."
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
