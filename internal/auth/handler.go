package auth

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type Handler struct {
	store Store
	state string
}

func InstallHandler(ctx context.Context, store Store, state string) (*Handler, error) {
	if !validState(state) {
		return nil, errors.New("invalid attempt state")
	}
	h := &Handler{store: store, state: state}
	if err := writePrivate(h.receipt(), []byte("dev-null-cli")); err != nil {
		return nil, err
	}
	if err := h.install(ctx); err != nil {
		_ = h.Close()
		return nil, errors.New("automatic browser return unavailable; paste the complete callback link instead")
	}
	return h, nil
}

func (h *Handler) receipt() string { return filepath.Join(h.store.Dir, h.state+".handler") }

func (h *Handler) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.uninstall(ctx); err != nil {
		return errors.New("temporary browser handler cleanup failed; restart the account screen to retry cleanup")
	}
	if err := os.Remove(h.receipt()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func RecoverHandlers(ctx context.Context, store Store) error {
	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		state := strings.TrimSuffix(entry.Name(), ".handler")
		if state == entry.Name() {
			state = strings.TrimSuffix(entry.Name(), ".attempt")
		}
		if state == entry.Name() || !validState(state) {
			continue
		}
		if seen[state] {
			continue
		}
		seen[state] = true
		if err := protectPath(filepath.Join(store.Dir, entry.Name()), false); err != nil {
			return err
		}
		address, err := readCallbackAddress(store, state)
		if err == nil {
			conn, dialErr := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", portString(address.Port)))
			if dialErr == nil {
				_ = conn.Close()
				continue
			}
			if !errors.Is(dialErr, syscall.ECONNREFUSED) {
				continue
			}
		}
		if err := (&Handler{store: store, state: state}).Close(); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(store.Dir, state+".attempt")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func tool(ctx context.Context, name string, args ...string) (string, error) {
	switch name {
	case "xdg-open", "xdg-mime", "update-desktop-database", "open", "osacompile", "osascript", "codesign", "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "rundll32.exe":
	default:
		return "", errors.New("unsupported platform utility")
	}
	// Only the fixed platform utilities above may run; arguments never pass through a shell.
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204
	cmd.Stdin = nil
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func OpenBrowser(ctx context.Context, raw string) error {
	u, err := parseBrowserURL(raw)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		_, err = tool(ctx, "open", u)
	case "windows":
		_, err = tool(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", u)
	default:
		_, err = tool(ctx, "xdg-open", u)
	}
	if err != nil {
		return errors.New("browser could not open; use the displayed authorization URL")
	}
	return nil
}
