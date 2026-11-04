package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

// Updates checks for and installs newer releases. Restart reports that the new binary can replace this process once
// the app exits.
type Updates struct {
	Check   func(context.Context) (string, error)
	Install func(context.Context) (string, error)
	Restart bool
}

type upgradeState struct {
	checked, open, installing, installed, restart bool
	latest, failure                               string
	choice                                        int
}

type upgradeChecked struct {
	latest string
	err    error
}

type upgradeInstalled struct {
	version string
	err     error
}

type tuiSettingsSaved struct {
	quiet bool
	err   error
}

func (m AppModel) WithUpdates(updates Updates) AppModel {
	m.updates = updates
	return m
}

// RestartRequested reports that an update was installed and the caller should start the new binary.
func (m AppModel) RestartRequested() bool { return m.upgrade.restart }

func upgradeCheckDue(settings auth.TUISettings, current string) bool {
	return settings.CheckForUpdates && release.ValidVersion(current)
}

// startUpgradeCheck looks for a new release once per launch.
func (m *AppModel) startUpgradeCheck() tea.Cmd {
	if m.upgrade.checked || m.updates.Check == nil {
		return nil
	}
	m.upgrade.checked = true
	if !upgradeCheckDue(m.tuiSettings, m.version) {
		return nil
	}
	check, life := m.updates.Check, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 15*time.Second)
		defer cancel()
		latest, err := check(ctx)
		return upgradeChecked{latest: latest, err: err}
	})
}

func (m *AppModel) acceptUpgradeChecked(msg upgradeChecked) {
	if msg.err != nil || msg.latest == "" || msg.latest == m.tuiSettings.SkippedVersion {
		return
	}
	m.upgrade = upgradeState{checked: true, open: true, latest: msg.latest}
}

func (m AppModel) saveTUISettings(quiet bool) tea.Cmd {
	store, settings := m.account.store, m.tuiSettings
	return func() tea.Msg { return tuiSettingsSaved{quiet: quiet, err: store.SaveTUISettings(settings)} }
}

func (m *AppModel) acceptTUISettingsSaved(msg tuiSettingsSaved) tea.Cmd {
	if msg.err == nil || msg.quiet {
		return nil
	}
	return m.notify("warning", "Setting not saved", "tui.json could not be written: "+msg.err.Error())
}

// upgradeVisible keeps the modal behind What's new and away from open text inputs, so a late check result never
// takes keys meant for a draft.
func (m AppModel) upgradeVisible() bool {
	if t := m.remotes.thread; t != nil && (t.composer.focused || t.react != nil || t.composer.recording != nil || t.composer.voiceConfirm) {
		return false
	}
	return m.upgrade.open && !m.whatsNew.open && m.connected && !m.accountOpen && !m.composer.open && m.settings.dialog == "" && !m.vote.confirm
}

func (m *AppModel) upgradeKey(pressed string) tea.Cmd {
	u := &m.upgrade
	if u.installing {
		return nil
	}
	if u.installed {
		if pressed == "esc" || pressed == "enter" || pressed == "q" {
			u.open = false
		}
		return nil
	}
	switch pressed {
	case "j", "down", "tab":
		u.choice = (u.choice + 1) % 3
	case "k", "up", "shift+tab":
		u.choice = (u.choice + 2) % 3
	case "u":
		return m.installUpgrade()
	case "s":
		return m.skipUpgrade()
	case "esc", "q", "l":
		u.open = false
	case "enter", "space":
		switch u.choice {
		case 0:
			return m.installUpgrade()
		case 1:
			u.open = false
		default:
			return m.skipUpgrade()
		}
	}
	return nil
}

func (m *AppModel) installUpgrade() tea.Cmd {
	u := &m.upgrade
	if m.updates.Install == nil {
		u.failure = "this build cannot update itself"
		return nil
	}
	u.installing, u.failure = true, ""
	install, life := m.updates.Install, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 5*time.Minute)
		defer cancel()
		version, err := install(ctx)
		return upgradeInstalled{version: version, err: err}
	})
}

func (m *AppModel) acceptUpgradeInstalled(msg upgradeInstalled) tea.Cmd {
	u := &m.upgrade
	u.installing = false
	if msg.err != nil {
		u.failure = msg.err.Error()
		return nil
	}
	if msg.version == "" {
		u.failure = "no newer release is available"
		return nil
	}
	u.latest = msg.version
	if m.updates.Restart {
		u.restart = true
		return m.quit()
	}
	u.installed = true
	return nil
}

func (m *AppModel) skipUpgrade() tea.Cmd {
	m.upgrade.open = false
	m.tuiSettings.SkippedVersion = m.upgrade.latest
	return m.saveTUISettings(false)
}

func (m AppModel) upgradeBounds() uiRect {
	return centeredRect(m.account.width, m.account.height, 60, 14)
}

// upgradeBody returns the modal content and the line where the choices start, or -1 when none are shown.
func (m AppModel) upgradeBody(p palette) (string, int) {
	u := m.upgrade
	width := max(1, m.upgradeBounds().w-4)
	latest := safeText(u.latest)
	paragraph := func(s string) []string { return strings.Split(ansi.Wrap(s, width, ""), "\n") }
	if u.installing {
		return strings.Join(paragraph("Updating to "+latest+"..."), "\n"), -1
	}
	if u.installed {
		return strings.Join(paragraph(text(p.success).Render("Updated to "+latest+".")+" Start devnull again to use it."), "\n"), -1
	}
	lines := paragraph("devnull " + latest + " is available. You have " + safeText(m.version) + ".")
	if u.failure != "" {
		// Go errors put the cause last, so a long one keeps its final two lines.
		failure := paragraph(safeText(u.failure))
		if len(failure) > 2 {
			failure = failure[len(failure)-2:]
			failure[0] = "…" + ansi.TruncateLeft(failure[0], 1, "")
		}
		lines = append([]string{text(p.danger).Render("Could not update to " + latest + ":")}, failure...)
		lines = append(lines, paragraph("Run devnull update in a terminal, or reinstall.")...)
	}
	lines = append(lines, "")
	start := len(lines)
	labels := []string{"Update and restart", "Later", "Skip this version"}
	if !m.updates.Restart {
		labels[0] = "Update"
	}
	for i, label := range labels {
		if i == u.choice {
			lines = append(lines, text(p.blue).Bold(true).Render("› "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	return strings.Join(lines, "\n"), start
}

func (m AppModel) upgradeModal(p palette) string {
	r := m.upgradeBounds()
	footer := "↑/↓ choose · enter confirm · esc later"
	if m.upgrade.installing {
		footer = "downloading and checking the release"
	} else if m.upgrade.installed {
		footer = "enter close"
	}
	body, _ := m.upgradeBody(p)
	return panel("Update available", body, text(p.muted).Render(fit(footer, max(1, r.w-4))), r, p, p.blue)
}

func (m AppModel) upgradeClick(x, y int) (AppModel, tea.Cmd) {
	r := m.upgradeBounds()
	if (uiRect{r.x + r.w - 3, r.y + 1, 1, 1}).contains(x, y) {
		return m.mouseKey("esc")
	}
	_, start := m.upgradeBody(colors(m.account.dark))
	choice := y - r.y - 3 - start
	if start < 0 || choice < 0 || choice > 2 || x <= r.x || x >= r.x+r.w-1 {
		return m, nil
	}
	m.upgrade.choice = choice
	return m.mouseKey("enter")
}
