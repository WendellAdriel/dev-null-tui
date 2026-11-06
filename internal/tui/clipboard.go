package tui

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

func (m AppModel) validCommitURL(raw string) bool {
	u, err := url.Parse(raw)
	origin, originErr := url.Parse(m.account.origin)
	return err == nil && originErr == nil && u.Scheme == origin.Scheme && u.Host == origin.Host && u.User == nil && u.Fragment == "" && !strings.ContainsAny(raw, "\r\n\x1b")
}

func validImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == "" && !strings.ContainsAny(raw, "\r\n\x1b")
}

func (m *AppModel) copyCommitLink(commit api.Commit) tea.Cmd {
	if !m.validCommitURL(commit.URL) {
		m.notice = "The selected commit has no safe link to copy"
		return nil
	}
	return m.copyText(commit.URL, "Commit link")
}

func (m *AppModel) copyText(raw, what string) tea.Cmd {
	return m.copyToClipboard(clipboardResult{url: raw, what: what})
}

func (m *AppModel) copyCommitText(commit api.Commit) tea.Cmd {
	result := clipboardResult{url: commit.Body, what: "Commit text", text: true}
	if m.validCommitURL(commit.URL) {
		result.link = commit.URL
	}
	return m.copyToClipboard(result)
}

func (m *AppModel) copyToClipboard(result clipboardResult) tea.Cmd {
	copyLink, life := m.copyLink, m.account.life
	result.session = m.session
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 3*time.Second)
		defer cancel()
		result.err = copyLink(ctx, result.url)
		return result
	})
}

func writeClipboard(ctx context.Context, value string) error {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbcopy"}}
	case "windows":
		candidates = [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$input | Set-Clipboard"}}
	default:
		candidates = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, args := range candidates {
		path, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}
		// #nosec G204: Executable and arguments come only from the fixed platform allowlist.
		cmd := exec.CommandContext(ctx, path, args[1:]...)
		cmd.Stdin = strings.NewReader(value)
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return errors.New("clipboard integration unavailable")
}

// manualNoun names what the clipboard fallback shows, since commit text is not a link.
func (m AppModel) manualNoun() string {
	if m.manualText {
		return "text"
	}
	return "link"
}

// safeLines cleans each line like safeText but keeps the line breaks of copied commit text.
func safeLines(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = safeText(line)
	}
	return strings.Join(lines, "\n")
}
