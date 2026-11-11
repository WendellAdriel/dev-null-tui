package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type settingsRow struct{ title, detail, action, value string }

func (m AppModel) settingsRows() []settingsRow {
	u := m.account.user
	on := func(v bool) string {
		if v {
			return "On"
		}
		return "Off"
	}
	switch m.settings.tab {
	case settingsRegionTab:
		return m.regionRows(on)
	case settingsDevices:
		return m.deviceRows()
	case settingsAppearance:
		event := eventLabel(m.tuiSettings.EventTheme)
		if m.tuiSettings.EventTheme == "auto" {
			if m.account.event != "" {
				event += " (" + themeNamed(m.account.event).label + " now)"
			}
		}
		return []settingsRow{
			{"Event theme: " + event, "Halloween (Oct 29 to Nov 1), Christmas (Dec 23 to 26) and New Year (Dec 30 to Jan 2) colors, like the web. Each shows its dark or light colors to match your theme. Auto follows the dates, Default never shows them. Saved in tui.json.", "event-theme", ""},
			{"Who to follow: " + map[bool]string{true: "Hidden", false: "Shown"}[m.tuiSettings.HideWhoToFollow], "The Who to follow action (w) in the aside, and suggested people in the Following and Latest logs after the fifth commit when the aside has no room. Saved in tui.json.", "who-to-follow", ""},
		}
	case settingsTUI:
		theme := m.account.lookupTheme(m.account.themeName).label
		if m.account.event != "" && m.account.event != m.account.themeName {
			theme += " (" + themeNamed(m.account.event).label + " event theme on now)"
		}
		return []settingsRow{
			{"Theme: " + theme, "The colors throughout the app. F2 opens this list anywhere. An active event theme overrides this choice. Saved on this device in tui.json.", "theme-name", ""},
			{"Check for updates: " + on(m.tuiSettings.CheckForUpdates), "Look for a new release each time devnull starts. Saved on this device in tui.json.", "update-check", ""},
			{"Desktop notifications: " + on(m.tuiSettings.DesktopNotifications), "Tell your desktop about new notifications and Remotes messages while the terminal isn't focused. Muted conversations and the open thread stay quiet. Saved on this device in tui.json.", "desktop-notifications", ""},
		}
	case settingsSafety:
		var rows []settingsRow
		for i, kind := range []string{"muted", "blocked"} {
			title, detail, verb, action := "Muted", "Their commits stay out of your logs. They aren't notified.", "Unmute", "unmute"
			if i == 1 {
				title, detail, verb, action = "Blocked", "Blocking ends follows in both directions. Unblocking does not restore them.", "Unblock", "unblock"
			}
			f := m.settings.lists[i]
			if f.loading {
				detail += "\n\nLoading " + kind + "..."
			} else if f.status != "" {
				detail += "\n\n" + safeText(f.status)
			} else if f.loaded && len(f.people) == 0 {
				detail += "\n\nYou haven't " + kind + " anyone."
			}
			rows = append(rows, settingsRow{title, detail, "", ""}, settingsRow{"Refresh " + kind, "Fetch the latest " + kind + " list.", "refresh-" + kind, ""})
			for _, person := range f.people {
				rows = append(rows, settingsRow{safeText(person.Name) + "  @" + safeText(person.Handle), verb + " this person. Confirmation required.", action, person.Handle})
			}
			if f.next != "" {
				rows = append(rows, settingsRow{"Load more " + kind, "Older entries in this list.", "more-" + kind, ""})
			}
		}
		return rows
	default:
		choice, color := savedSettingsLane(u.PreferredLane)
		lane := settingsLaneNames[choice]
		if color != "" {
			lane += " (" + color + ")"
		}
		rows := []settingsRow{
			{"Sync from GitHub", "Name, handle, bio, company, location and website are mirrored from GitHub.", "github", ""},
			{"View profile", "Open your profile in this client.", "profile", ""},
			{"Color: " + lane, "Automatic, six palette lanes, or a custom #rrggbb color.", "lane", ""},
			{"Pastéis mode: " + on(u.PasteisMode), "Ten to give each day. Turning this off hides giving controls and counters.", "pasteis", ""},
			{"Remotes: " + on(u.Remotes != nil), "When off, nobody can message you. Existing conversations are kept.", "remotes", ""},
		}
		if u.Remotes != nil {
			rows = append(rows, settingsRow{"Who can push: " + u.Remotes.From, "Everyone is available only when the server allows it.", "audience", ""}, settingsRow{"Share pull receipts: " + on(u.Remotes.PullReceipts), "Tell others when you have read their messages.", "receipts", ""})
		}
		return append(rows, settingsRow{"Delete account", "Permanently delete your /dev/null account and its data. Your GitHub account is unchanged.", "delete-account", ""})
	}
}

func (m *AppModel) sizeSettings() {
	s := &m.settings
	inputWidth := max(1, min(60, m.account.width-12))
	if s.dialog == "lane" {
		inputWidth = max(1, inputWidth-8)
	}
	s.input.SetWidth(inputWidth)
	if !s.open {
		return
	}
	_, width, _ := m.columns()
	s.viewport.SetWidth(max(1, width-4))
	s.viewport.SetHeight(max(1, m.mainHeight()-7))
	rows := m.settingsRows()
	if s.selected >= 0 && s.selected < len(s.rows) {
		anchor := s.rows[s.selected]
		if anchor.action != "" {
			for i, row := range rows {
				if row.action == anchor.action && row.value == anchor.value {
					s.selected = i
					break
				}
			}
		}
	}
	s.rows = rows
	s.selected = max(0, min(s.selected, len(rows)-1))
	for s.selected < len(rows)-1 && rows[s.selected].action == "" {
		s.selected++
	}
	p := m.account.colors()
	var lines []string
	if s.tab == settingsProfile {
		u := m.account.user
		lines = append(lines, text(p.ink).Bold(true).Render(safeText(u.Name)), text(p.muted).Render("@"+safeText(u.Handle)), "")
		for _, f := range []struct{ label, value string }{{"Bio", u.Bio}, {"Company", u.Company}, {"Location", u.Location}, {"Website", u.Website}, {"Sponsors", u.SponsorsURL}, {"Last synced", u.GitHubSyncedAt}} {
			if f.value != "" {
				lines = append(lines, strings.Split(ansi.Wrap(f.label+": "+safeText(f.value), s.viewport.Width(), ""), "\n")...)
			}
		}
		lines = append(lines, "", text(p.hairline).Render(strings.Repeat("─", s.viewport.Width())), "")
	}
	s.starts = nil
	for i, row := range rows {
		s.starts = append(s.starts, len(lines))
		prefix := "  "
		style := text(p.ink)
		if row.action == "" {
			prefix = ""
			style = style.Bold(true)
		} else if i == s.selected {
			prefix = "› "
			style = style.Foreground(lipgloss.Color(p.blue)).Bold(true).Background(lipgloss.Color(p.soft))
		}
		lines = append(lines, style.Render(fit(prefix+row.title, s.viewport.Width())))
		lines = append(lines, strings.Split(ansi.Wrap(text(p.muted).Render(row.detail), s.viewport.Width(), ""), "\n")...)
		lines = append(lines, "", text(p.hairline).Render(strings.Repeat("─", s.viewport.Width())), "")
	}
	s.viewport.SetContent(strings.Join(lines, "\n"))
	if s.selected < len(s.starts) {
		start := s.starts[s.selected]
		if start < s.viewport.YOffset() || start >= s.viewport.YOffset()+s.viewport.Height()-1 {
			s.viewport.SetYOffset(start)
		}
	}
}

func (m AppModel) settingsColumn(width int, p palette) string {
	tabs := settingsTabs
	for i := range tabs {
		if i == m.settings.tab {
			tabs[i] = text(p.blue).Bold(true).Render(tabs[i])
		} else {
			tabs[i] = text(p.muted).Render(tabs[i])
		}
	}
	header := "  " + text(p.ink).Bold(true).Render("Settings") + "\n\n" + ansi.Truncate("  "+strings.Join(tabs[:], "   "), width, "…") + "\n" + text(p.hairline).Render(strings.Repeat("─", width)) + "\n"
	body := lipgloss.NewStyle().Padding(0, 2).Render(m.settings.viewport.View())
	status := fit(safeText(m.settings.status), max(1, width-4))
	content := header + body + "\n  " + text(p.muted).Render(status)
	return lipgloss.NewStyle().Width(width).Height(m.mainHeight()).MaxWidth(width).MaxHeight(m.mainHeight()).Render(content)
}

func (m AppModel) settingsDialog(p palette) (string, uiRect, []hitArea) {
	s := m.settings
	r := centeredRect(m.account.width, m.account.height, 72, 24)
	w := max(1, r.w-4)
	var title, body string
	var bodyAreas []hitArea
	switch s.dialog {
	case "delete-account":
		title = "Delete account permanently"
		body = "This removes your /dev/null account, commits, replies and relationships. Threads you started and their replies are deleted. Some replies remain as anonymous tombstones. Your GitHub account is unchanged.\n\nType your exact handle to confirm:\n" + s.input.View()
		if m.account.height < 24 {
			body = "Permanently delete /dev/null and its data.\nYour GitHub account is unchanged.\n\nType your exact handle to confirm:\n" + s.input.View()
		}
	case "timezone":
		title = "Timezone"
		body = "Times use this timezone, and others see it when they hover your commits. Leave it empty to follow this device without sharing it.\n\n" + s.input.View()
	case "translation-language":
		title = "Translation language"
		body = m.settingsLanguagePicker(p)
	case "revoke-device":
		title = "Revoke device"
		body = "Revoke this device? The app on it signs out."
		for _, d := range s.devices {
			if d.ID == s.targetID {
				body = "Revoke " + safeText(deviceLabel(d)) + "? The app on it signs out."
				if m.account.key != nil && d.Fingerprint == m.account.key.Fingerprint() {
					body = "This is the device you are using. Revoking it signs this client out and removes its saved key. You will need to approve it again on the web."
				}
			}
		}
	case "lane":
		title = "Your color"
		body, bodyAreas = m.settingsLanePicker(p, w)
	case "report":
		title = "Report commit"
		body = "Private report · choose a reason (↑/↓):\n"
		for i, reason := range api.ReportReasons {
			marker := "  "
			if s.choice == i {
				marker = "› "
			}
			body += marker + reason.Label + "\n"
		}
		body += "\n" + s.input.View()
	default:
		title = map[string]string{"mute": "Mute", "unmute": "Unmute", "block": "Block", "unblock": "Unblock"}[s.dialog] + " @" + safeText(s.handle)
		body = "Mute keeps their commits and replies out of your logs and threads without notifying them.\n\nBlock also ends follows in both directions. Unblocking does not restore them."
	}
	body = ansi.Wrap(body, w, "")
	if s.status != "" {
		body += "\n\n" + text(p.warning).Render(safeText(s.status))
	}
	if s.busy {
		body += "\n\nWorking..."
	}
	if m.account.width < 60 || m.account.height < 18 {
		body = "This action needs a terminal of at least 60×18. Esc to cancel."
		bodyAreas = nil
	}
	label := "Confirm"
	if s.dialog == "lane" {
		label = "Save color"
	}
	if s.dialog == "report" {
		label = "Send report"
	}
	if s.dialog == "delete-account" {
		label = "Delete permanently"
	}
	if s.dialog == "timezone" || s.dialog == "translation-language" {
		label = "Save"
	}
	if s.dialog == "revoke-device" {
		label = "Revoke"
	}
	footer, areas := modalFooter([]modalAction{{"Cancel", "Esc", "esc"}, {label, "Ctrl+S", "ctrl+s"}}, w, s.focus+1, p)
	for i := range areas {
		areas[i].x += r.x + 2
		areas[i].y += r.y + r.h - 1 - lipgloss.Height(footer)
	}
	for i := range bodyAreas {
		bodyAreas[i].x += r.x + 2
		bodyAreas[i].y += r.y + 3
	}
	border := p.blue
	switch s.dialog {
	case "delete-account", "revoke-device", "block":
		border = p.danger
	case "mute", "report":
		border = p.warning
	}
	return panel(title, body, footer, r, p, border), r, append(bodyAreas, areas...)
}

func (m AppModel) regionRows(on func(bool) string) []settingsRow {
	s := m.settings
	if !s.regionLoad.loaded {
		detail := "Loading your timezone and translation settings..."
		if !s.regionLoad.loading {
			detail = safeText(s.regionLoad.status)
		}
		return []settingsRow{{"Region", detail, "", ""}, {"Refresh", "Fetch your timezone and translation settings again.", "refresh-region", ""}}
	}
	r := s.region
	zone := "Not set"
	if r.Timezone != "" {
		zone = safeText(r.Timezone)
	}
	rows := []settingsRow{{"Timezone: " + zone, "Others see your timezone when they hover your commits. Unset follows this device without sharing it.", "timezone", ""}}
	if r.Timezone != "" {
		rows = append(rows, settingsRow{"Show local time on profile: " + on(r.ShowLocalTime), "Anyone who opens your profile sees the current time in your timezone.", "local-time", ""})
	}
	language := "Follow device language"
	for _, l := range r.TranslationLanguages {
		if l.Code == r.TranslationLanguage && l.Code != "" {
			language = safeText(languageLabel(l))
		}
	}
	return append(rows,
		settingsRow{"Translation language: " + language, "The language for translated commits and replies.", "translation-language", ""},
		settingsRow{"Automatically show translations: " + on(r.AutomaticTranslation), "Turn off to read originals first and choose Translate when you need it.", "automatic-translation", ""},
	)
}

func languageLabel(l api.Language) string {
	if l.Code == "" || l.NativeName == "" || strings.EqualFold(l.NativeName, l.Name) {
		return l.Name
	}
	return l.Name + " (" + l.NativeName + ")"
}

func (m AppModel) deviceRows() []settingsRow {
	s := m.settings
	detail := "Each one can use /dev/null as you. Revoke any you don't use or don't recognize."
	switch {
	case s.devicesLoad.loading:
		detail += "\n\nLoading devices..."
	case s.devicesLoad.status != "":
		detail += "\n\n" + safeText(s.devicesLoad.status)
	case s.devicesLoad.loaded && len(s.devices) == 0:
		detail += "\n\nNo approved devices."
	}
	rows := []settingsRow{{"Approved devices", detail, "", ""}, {"Refresh devices", "Fetch the latest device list.", "refresh-devices", ""}}
	for _, d := range s.devices {
		title := safeText(deviceLabel(d))
		if m.account.key != nil && d.Fingerprint == m.account.key.Fingerprint() {
			title += " · This device"
		}
		used := "never"
		if d.LastUsedAt != "" {
			used = deviceDate(d.LastUsedAt)
		}
		rows = append(rows, settingsRow{title, "Approved " + deviceDate(d.ApprovedAt) + " · Last used " + used + "\n" + safeText(d.Fingerprint) + "\nRevoke this device. Confirmation required.", "revoke-device", d.ID})
	}
	return rows
}

func deviceLabel(d api.Device) string {
	switch {
	case d.Name != "" && d.AppName != "":
		return d.Name + " · " + d.AppName
	case d.Name != "":
		return d.Name
	case d.AppName != "":
		return d.AppName
	}
	return "Unnamed device"
}

func deviceDate(value string) string {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return safeText(value)
	}
	return at.UTC().Format("Jan 2, 2006 15:04 UTC")
}

func (m AppModel) settingsLanguagePicker(p palette) string {
	s := m.settings
	languages := m.settingsLanguages()
	visible := max(3, min(8, m.account.height-16))
	first := max(0, min(s.choice-visible/2, len(languages)-visible))
	var rows []string
	for i := first; i < min(len(languages), first+visible); i++ {
		label := "  " + safeText(languageLabel(languages[i]))
		if i == s.choice {
			label = withBackground(text(p.ink).Bold(true).Render("› "+safeText(languageLabel(languages[i]))), p.soft)
		}
		rows = append(rows, label)
	}
	if len(languages) == 0 {
		rows = append(rows, text(p.muted).Render("No language matches this search."))
	}
	return "Search: " + s.input.View() + "\n\n" + strings.Join(rows, "\n") + "\n\n" + text(p.muted).Render("Type to search · ↑/↓ to choose")
}

func (m AppModel) settingsLanePicker(p palette, width int) (string, []hitArea) {
	var rows []string
	var areas []hitArea
	row := ""
	for i, name := range settingsLaneNames {
		marker := "  "
		if m.settings.choice == i {
			marker = "› "
		}
		label := marker + name
		if i > 0 && i < 7 {
			label = marker + text(p.lanes[i-1]).Render("● ") + name
		}
		if m.settings.choice == i {
			label = withBackground(text(p.ink).Bold(true).Render(label), p.soft)
		}
		if row != "" && lipgloss.Width(row)+2+lipgloss.Width(label) > width {
			rows = append(rows, row)
			row = ""
		}
		if row != "" {
			row += "  "
		}
		areas = append(areas, hitArea{uiRect{lipgloss.Width(row), len(rows), lipgloss.Width(label), 1}, "lane-choice", i})
		row += label
	}
	rows = append(rows, row)
	areas = append(areas, hitArea{uiRect{8, len(rows) + 2, max(1, width-8), 1}, "lane-input", 7})
	body := strings.Join(rows, "\n") + "\n\n" + text(p.muted).Render("←/→ or ↑/↓ to select · click a color") + "\nCustom: " + m.settings.input.View()
	return body, areas
}

func (m AppModel) settingsHelp() appKeys {
	bind := func(k, label string) key.Binding { return key.NewBinding(key.WithKeys(k), key.WithHelp(k, label)) }
	bindings := []key.Binding{bind("←/→", "settings tab"), bind("↑/↓", "choose control"), bind("enter", "activate"), bind("ctrl+r", "refresh account"), bind("a", "account connection"), bind("esc", "back"), bind("f2", "choose a theme"), bind("?", "help"), bind("ctrl+c", "quit")}
	if m.settings.dialog != "" {
		bindings = []key.Binding{bind("↑/↓", "choose option"), bind("tab", "Cancel / Confirm"), bind("ctrl+s", "confirm"), bind("esc", "cancel")}
		if m.settings.dialog == "lane" {
			bindings[0] = bind("arrows / click", "choose color")
		}
	}
	return appKeys{bindings, bindings}
}
