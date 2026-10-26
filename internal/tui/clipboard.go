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
	session, copyLink, life := m.session, m.copyLink, m.account.life
	return life.command(func() tea.Msg {
		ctx, cancel := context.WithTimeout(life.ctx, 3*time.Second)
		defer cancel()
		return clipboardResult{session: session, url: raw, what: what, err: copyLink(ctx, raw)}
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
