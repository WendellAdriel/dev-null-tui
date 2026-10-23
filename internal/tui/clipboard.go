package tui

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func (m AppModel) validCommitURL(raw string) bool {
	u, err := url.Parse(raw)
	origin, originErr := url.Parse(m.account.origin)
	return err == nil && originErr == nil && u.Scheme == origin.Scheme && u.Host == origin.Host && u.User == nil && u.Fragment == "" && !strings.ContainsAny(raw, "\r\n\x1b")
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
