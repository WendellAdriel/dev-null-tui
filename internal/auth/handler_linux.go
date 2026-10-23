package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (h *Handler) desktopPath() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("XDG data directory must be absolute")
	}
	return filepath.Join(dir, "applications", "dev-null-cli-"+h.state+".desktop"), nil
}

func desktopQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("executable path cannot contain control characters")
	}
	value = strings.NewReplacer("\\", "\\\\\\\\", "\"", "\\\\\"", "`", "\\\\`", "$", "\\\\$", "%", "%%").Replace(value)
	return "\"" + value + "\"", nil
}

func (h *Handler) install(ctx context.Context) error {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return errors.New("no desktop session")
	}
	owner, err := tool(ctx, "xdg-mime", "query", "default", "x-scheme-handler/devnull")
	if err != nil || owner != "" {
		return errors.New("protocol handler is owned or cannot be determined")
	}
	path, err := h.desktopPath()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	quoted, err := desktopQuote(exe)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	entry := "[Desktop Entry]\nType=Application\nName=/dev/null CLI connection\nExec=" + quoted + " callback %u\nNoDisplay=true\nMimeType=x-scheme-handler/devnull;\n"
	if err := writePrivate(path, []byte(entry)); err != nil {
		return err
	}
	// Register an available application, without changing the user's default association files.
	if _, err := tool(ctx, "update-desktop-database", filepath.Dir(path)); err != nil {
		return err
	}
	owner, err = tool(ctx, "xdg-mime", "query", "default", "x-scheme-handler/devnull")
	if err != nil || owner != filepath.Base(path) {
		return errors.New("desktop did not select the temporary handler")
	}
	return nil
}

func (h *Handler) uninstall(ctx context.Context) error {
	path, err := h.desktopPath()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	if err := protectPath(path, false); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	_, err = tool(ctx, "update-desktop-database", filepath.Dir(path))
	return err
}
