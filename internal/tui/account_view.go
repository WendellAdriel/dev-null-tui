package tui

import (
	"net/url"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

type accountAction struct {
	key, label, compact string
}

func (m AccountModel) actions() []accountAction {
	switch m.state {
	case "idle":
		return []accountAction{{"enter", "Connect in browser", "Connect in browser"}}
	case "waiting":
		return []accountAction{{"b", "Open browser", "Browser"}, {"p", "Paste callback", "Paste link"}, {"u", "Show URL", "URL"}}
	case "failed":
		if m.approvalRequired {
			return []accountAction{{"r", "New browser approval", "New approval"}, {"d", "Clear local connection", "Clear connection"}}
		}
		if m.key != nil {
			return []accountAction{{"v", "Verify saved connection", "Verify saved connection"}, {"r", "New browser approval", "New approval"}}
		}
		return []accountAction{{"r", "Try again", "Try again"}, {"d", "Clear local connection", "Clear connection"}}
	case "connected":
		actions := []accountAction{{"v", "Verify connection", "Verify"}, {"d", "Disconnect", "Disconnect"}}
		if m.key != nil {
			label, compact := "Device details", "Details"
			if m.showDeviceDetails {
				label, compact = "Hide device details", "Hide details"
			}
			actions = append(actions, accountAction{"i", label, compact})
		}
		return actions
	case "confirm":
		return []accountAction{{"esc", "Keep connection", "Keep connection"}, {"delete-key", "Delete local key", "Delete key"}}
	}
	return nil
}

func (m AccountModel) View() tea.View {
	p := colors(m.dark)
	page, _ := m.accountScreen(p)
	frame := lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink)).Background(lipgloss.Color(p.shell)).Width(max(0, m.width)).Height(max(0, m.height)).MaxWidth(max(0, m.width)).MaxHeight(max(0, m.height))
	view := tea.NewView(frame.Render(page))
	view.AltScreen = true
	if m.profile >= colorprofile.ANSI && (m.theme != "auto" || m.backgroundKnown) {
		view.BackgroundColor = lipgloss.Color(p.shell)
		view.ForegroundColor = lipgloss.Color(p.ink)
	}
	return view
}

func (m AccountModel) accountScreen(p palette) (string, []hitArea) {
	var page string
	var areas []hitArea
	if m.width >= 60 && m.height >= 18 {
		width := min(m.width-8, 100)
		wide := width >= 92 && m.height >= 26 && !m.help && !m.showURL && !m.pasting
		mainWidth := width
		if wide {
			mainWidth -= 28
		}
		head := m.accountMasthead(width, p)
		body, buttons := m.accountBodyLayout(mainWidth, p)
		bodyX, bodyY := 0, 0
		if wide {
			bodyX = 28
			body = lipgloss.JoinHorizontal(lipgloss.Top, m.accountJourney(p), strings.Repeat(" ", 5), body)
		} else if !m.help && !m.showURL && !m.pasting {
			bodyY = lipgloss.Height(m.accountSteps(p)) + 1
			body = m.accountSteps(p) + "\n\n" + body
		}
		footer, footerAreas := m.accountFooterLayout(width, p)
		gap := "\n\n"
		gapRows := 2
		if m.height <= 24 {
			gap = "\n"
			gapRows = 0
		}
		maxBody := max(1, m.height-lipgloss.Height(head)-lipgloss.Height(footer)-gapRows)
		body = lipgloss.NewStyle().MaxHeight(maxBody).Render(body)
		page = head + gap + body + gap + footer
		page = lipgloss.NewStyle().Width(width).Render(page)
		top := min(6, max(0, (m.height-lipgloss.Height(page))/3))
		for _, area := range footerAreas {
			area.x += (m.width - width) / 2
			area.y += top + lipgloss.Height(page) - lipgloss.Height(footer)
			if area.y < m.height {
				areas = append(areas, area)
			}
		}
		for _, area := range buttons {
			area.x += (m.width-width)/2 + bodyX
			area.y += top + lipgloss.Height(head) + len(gap) + bodyY - 1
			if area.y < top+lipgloss.Height(head)+len(gap)-1+maxBody {
				areas = append(areas, area)
			}
		}
		page = lipgloss.NewStyle().MarginLeft((m.width - width) / 2).MarginTop(top).Render(page)
	} else if m.width > 0 && m.height > 0 {
		page = "ACCOUNT\nResize to at least 60x18.\nf2 " + m.nextTheme() + " theme · Ctrl+C to quit."
	}
	return page, areas
}

func (m AccountModel) accountMasthead(width int, p palette) string {
	brand := text(p.lanes[0]).Bold(true).Render("//") + "  " + text(p.ink).Bold(true).Render("/dev/null")
	label := text(p.muted).Render("ACCOUNT CONNECTION")
	if width < 92 {
		server, _ := url.Parse(m.origin)
		label = text(p.muted).Render(safeText(server.Host))
	}
	space := max(2, width-lipgloss.Width(brand)-lipgloss.Width(label))
	return brand + strings.Repeat(" ", space) + label + "\n" + text(p.hairline).Render(strings.Repeat("─", width))
}

func (m AccountModel) connectionStep() int {
	switch m.state {
	case "connected", "confirm", "disconnecting":
		return 3
	case "verifying":
		return 2
	case "waiting", "failed", "canceling":
		if m.key != nil {
			return 1
		}
	}
	return 0
}

func (m AccountModel) accountJourney(p palette) string {
	step := m.connectionStep()
	var lines []string
	for i, label := range []string{"Device identity", "Browser approval", "Account verified"} {
		ink, node := p.muted, "○"
		if i < step {
			ink, node = p.ink, "●"
		} else if i == step {
			ink, node = p.blue, "●"
		}
		lines = append(lines, text(ink).Render(node+"  "+label))
		if i < 2 {
			lines = append(lines, text(p.hairline).Render("│"), text(p.hairline).Render("│"))
		}
	}
	u, _ := url.Parse(m.origin)
	lines = append(lines, "", text(p.muted).Render("SERVER"), text(p.ink).Render(safeText(u.Host)), "", text(p.muted).Render("Ed25519 / SSHSIG"))
	return lipgloss.NewStyle().Width(23).MaxWidth(23).Render(strings.Join(lines, "\n"))
}

func (m AccountModel) accountSteps(p palette) string {
	step := m.connectionStep()
	var labels []string
	for i, label := range []string{"1 KEY", "2 APPROVE", "3 VERIFY"} {
		ink := p.muted
		if i < step {
			ink = p.ink
		} else if i == step {
			ink = p.blue
		}
		labels = append(labels, text(ink).Render(label))
	}
	return strings.Join(labels, text(p.hairline).Render("  /  "))
}

func (m AccountModel) accountBodyLayout(width int, p palette) (string, []hitArea) {
	compact := m.height <= 24
	title, lead := "Connect your account", "Approve this terminal in your browser."
	detail := "Sign in through GitHub, then approve the device on /dev/null."
	switch m.state {
	case "restoring":
		title, lead, detail = "Checking saved connection", "Verifying this terminal's device key.", "No browser approval is needed if the saved key is still approved."
	case "preparing":
		title, lead, detail = "Preparing connection", "Creating a private device identity.", "Your browser will open when the return handler is ready."
	case "waiting":
		title, lead, detail = "Waiting for browser approval", "Compare the fingerprint before you approve.", "If automatic return fails, paste the complete callback link."
	case "verifying":
		title, lead, detail = "Verifying connection", "Checking your signed account identity.", "The saved key is checked with GET /api/v1/me."
	case "canceling":
		title, lead, detail = "Canceling connection", "Stopping pending work and cleaning up.", "Your existing device key is kept unless you disconnect."
	case "disconnecting":
		title, lead, detail = "Disconnecting locally", "Deleting this terminal's private key.", "Server approval is unchanged."
	case "connected":
		title, lead, detail = "Account connected", safeText(m.user.Name)+"  @"+safeText(m.user.Username), "This terminal can sign requests as your account."
		if m.workspace {
			detail += " Press Esc to return to your workspace."
		}
	case "failed":
		title = "Connection needs attention"
		if m.approvalRequired {
			lead, detail = "This device needs browser approval.", "Start a new browser approval to reconnect your account."
		} else if m.key != nil {
			lead, detail = "Your device key is still saved.", "Browser approval may have succeeded. Verify the saved connection before starting a new approval."
		} else {
			lead, detail = "No account connection was established.", "Try connecting again. No account credentials are shown or logged."
		}
	case "confirm":
		title, lead, detail = "Delete this terminal's key?", "Local disconnect does not revoke server approval.", "Revoke the device separately at "+m.origin+"/settings/devices"
	}
	if m.help {
		return m.accountHelp(width, p), nil
	}
	if m.pasting {
		input := m.input
		input.SetStyles(textinput.DefaultStyles(m.dark))
		input.SetWidth(max(1, width-4))
		return text(p.ink).Bold(true).Render("Paste callback link") + "\n\n" + text(p.muted).Render("Use the full devnull://auth/callback link.") + "\n\n" + input.View() + "\n\n" + text(p.muted).Render("Masked input. Nothing is logged."), nil
	}
	if m.showURL && m.flow != nil {
		return text(p.ink).Bold(true).Render("Open this authorization URL") + "\n\n" + lipgloss.NewStyle().Width(width).Render(m.flow.Attempt.URL) + "\n\n" + text(p.muted).Render("Return here after approval. Never share the callback."), nil
	}
	var sections []string
	sections = append(sections, text(p.ink).Bold(true).Render(title))
	if !compact || m.state == "confirm" || m.state == "connected" {
		sections = append(sections, lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color(p.muted)).Render(lead))
	}
	if m.status != "" && m.state == "failed" {
		sections = append(sections, lipgloss.NewStyle().Width(width).MaxHeight(3).Foreground(lipgloss.Color(p.lanes[0])).Render(safeText(m.status)))
	}
	if m.state == "connected" && m.status == verificationNotice {
		sections = append(sections, text(p.blue).Render(m.status))
	}
	if m.key != nil && (m.state == "waiting" || (m.state == "connected" && m.showDeviceDetails) || (!compact && m.state == "failed")) {
		sections = append(sections, text(p.muted).Render("DEVICE FINGERPRINT")+"\n"+text(p.ink).Render(m.key.Fingerprint()))
	}
	if !compact {
		sections = append(sections, lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color(p.muted)).Render(detail))
	}
	if m.status != "" && m.state != "failed" && (m.state == "waiting" || m.state == "idle") {
		status := safeText(m.status)
		if compact && m.state == "waiting" {
			status = "Automatic return unavailable. Use Paste link."
			if strings.Contains(m.status, "browser could not open") {
				status = "Browser did not open. Choose URL."
			}
		}
		sections = append(sections, lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color(p.muted)).Render(status))
	}
	separator := "\n\n"
	if m.height < 28 {
		separator = "\n"
	}
	body := strings.Join(sections, separator)
	actions, areas := m.accountActionsLayout(width, p)
	if actions != "" {
		for i := range areas {
			areas[i].y += lipgloss.Height(body) + len(separator) - 1
		}
		body += separator + actions
	}
	return lipgloss.NewStyle().Width(width).Render(body), areas
}

func (m AccountModel) accountActionsLayout(width int, p palette) (string, []hitArea) {
	actions := m.actions()
	var areas []hitArea
	var rows []string
	line := ""
	for i, action := range actions {
		label := action.label
		if m.height <= 24 {
			label = action.compact
		}
		hint := action.key
		if hint == "delete-key" {
			hint = "tab"
			if i == m.actionFocus%len(actions) {
				hint = "enter"
			}
		}
		plain := " " + hint + "  " + label + " "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(p.ink))
		if i == m.actionFocus%len(actions) {
			style = style.Foreground(lipgloss.Color(p.shell)).Background(lipgloss.Color(p.blue)).Bold(true)
		}
		button := style.Render(plain)
		if line != "" && lipgloss.Width(line)+2+lipgloss.Width(button) > width {
			rows = append(rows, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		areas = append(areas, hitArea{uiRect{lipgloss.Width(line), len(rows), min(width-lipgloss.Width(line), lipgloss.Width(button)), 1}, "account", i})
		line += button
	}
	if line != "" {
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n"), areas
}

func (m AccountModel) accountFooterLayout(width int, p palette) (string, []hitArea) {
	var hints []struct{ label, action string }
	for _, binding := range m.bindings().bindings {
		hint := binding.Help()
		if hint.Key == "tab" || hint.Key == "enter" || hint.Key == "esc" || hint.Key == "?" || hint.Key == "ctrl+c" || hint.Key == "f2" || (hint.Key == "d" && m.state == "failed") {
			hints = append(hints, struct{ label, action string }{hint.Key + " " + hint.Desc, hint.Key})
		}
	}
	var rows []string
	var areas []hitArea
	row := ""
	for _, hint := range hints {
		if row != "" && lipgloss.Width(row)+3+lipgloss.Width(hint.label) > width {
			rows = append(rows, row)
			row = ""
		}
		if row != "" {
			row += "   "
		}
		areas = append(areas, hitArea{uiRect{lipgloss.Width(row), 1 + len(rows), min(width-lipgloss.Width(row), lipgloss.Width(hint.label)), 1}, hint.action, 0})
		row += hint.label
	}
	if row != "" {
		rows = append(rows, row)
	}
	footer := text(p.hairline).Render(strings.Repeat("─", width)) + "\n" + text(p.muted).Render(strings.Join(rows, "\n"))
	return footer, areas
}

func (m AccountModel) accountHelp(width int, p palette) string {
	active := m
	active.help = false
	var rows []string
	for _, binding := range active.bindings().bindings {
		hint := binding.Help()
		rows = append(rows, text(p.blue).Width(9).Render(hint.Key)+text(p.ink).Render(hint.Desc))
	}
	return text(p.ink).Bold(true).Render("Connection shortcuts") + "\n\n" + strings.Join(rows, "\n") + "\n\n" + lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color(p.muted)).Render("Typing owns the callback field. Esc leaves input first.")
}
