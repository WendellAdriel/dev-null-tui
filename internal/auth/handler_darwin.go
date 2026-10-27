package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

func (h *Handler) macApp() string { return filepath.Join(h.store.Dir, "connection-"+h.state+".app") }

// macOwnerScript prints the bundle identifier that opens devnull:// links, or nothing when no app claims the scheme.
const macOwnerScript = `use framework "AppKit"
set target to current application's NSURL's URLWithString:"devnull://auth/callback"
set owner to current application's NSWorkspace's sharedWorkspace()'s URLForApplicationToOpenURL:target
if owner is missing value then return ""
set identifier to (current application's NSBundle's bundleWithURL:owner)'s bundleIdentifier()
if identifier is missing value then return "unknown"
return identifier as text`

func macOwner(ctx context.Context) (string, error) {
	return tool(ctx, "osascript", "-e", macOwnerScript)
}

// awaitMacOwner polls because LaunchServices can apply a registration after lsregister returns.
func awaitMacOwner(ctx context.Context, want string) error {
	for {
		if owner, err := macOwner(ctx); err == nil && owner == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("macOS did not select the temporary handler")
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (h *Handler) install(ctx context.Context) error {
	owner, err := macOwner(ctx)
	if err != nil || owner != "" {
		return errors.New("protocol handler is owned or cannot be determined")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	source := "use framework \"Foundation\"\non open location theURL\nset task to current application's NSTask's alloc()'s init()\ntask's setLaunchPath:" + strconv.Quote(exe) + "\ntask's setArguments:{\"callback\", theURL}\ntask's launch()\nend open location"
	if _, err := tool(ctx, "osacompile", "-o", h.macApp(), "-e", source); err != nil {
		return err
	}
	if err := os.Chmod(h.macApp(), 0700); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleExecutable</key><string>applet</string><key>CFBundleIdentifier</key><string>sh.dev-null.cli.auth.` + h.state + `</string><key>CFBundlePackageType</key><string>APPL</string><key>LSUIElement</key><true/><key>CFBundleURLTypes</key><array><dict><key>CFBundleURLName</key><string>/dev/null CLI connection</string><key>CFBundleURLSchemes</key><array><string>devnull</string></array></dict></array></dict></plist>`
	if err := os.WriteFile(filepath.Join(h.macApp(), "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
		return err
	}
	if _, err := tool(ctx, "codesign", "--force", "--sign", "-", h.macApp()); err != nil {
		return err
	}
	if _, err := tool(ctx, lsregister, "-f", h.macApp()); err != nil {
		return err
	}
	return awaitMacOwner(ctx, "sh.dev-null.cli.auth."+h.state)
}

func (h *Handler) uninstall(ctx context.Context) error {
	if _, err := os.Lstat(h.macApp()); os.IsNotExist(err) {
		return nil
	}
	if err := protectPath(h.macApp(), true); err != nil {
		return err
	}
	if _, err := tool(ctx, lsregister, "-u", h.macApp()); err != nil {
		return err
	}
	return os.RemoveAll(h.macApp())
}
