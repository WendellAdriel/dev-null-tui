package auth

import (
	"context"
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const schemeKey = `Software\Classes\devnull`

func (h *Handler) install(_ context.Context) error {
	for _, location := range []struct {
		root registry.Key
		path string
	}{{registry.CLASSES_ROOT, "devnull"}, {registry.CURRENT_USER, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\devnull\UserChoice`}} {
		key, err := registry.OpenKey(location.root, location.path, registry.READ)
		if err == nil {
			_ = key.Close()
			return errors.New("protocol handler already exists")
		}
		if !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	exe, err := os.Executable()
	if err != nil || strings.ContainsRune(exe, '"') {
		return errors.New("cannot resolve executable path")
	}
	key, existed, err := registry.CreateKey(registry.CURRENT_USER, schemeKey, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer func() { _ = key.Close() }()
	if existed {
		return errors.New("protocol handler already exists")
	}
	if err := key.SetStringValue("DevNullCLIHandler", h.state); err != nil {
		return err
	}
	if err := key.SetStringValue("", "URL:/dev/null CLI connection"); err != nil {
		return err
	}
	if err := key.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	command, _, err := registry.CreateKey(key, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = command.Close() }()
	return command.SetStringValue("", `"`+exe+`" callback "%1"`)
}

func (h *Handler) uninstall(_ context.Context) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, schemeKey, registry.READ)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	owner, _, readErr := key.GetStringValue("DevNullCLIHandler")
	_ = key.Close()
	if readErr != nil || owner != h.state {
		return nil
	}
	for _, sub := range []string{`\shell\open\command`, `\shell\open`, `\shell`, ""} {
		if err := registry.DeleteKey(registry.CURRENT_USER, schemeKey+sub); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	return nil
}
