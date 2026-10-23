package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv("DEV_NULL_TEST_TOOL_DIR"); dir != "" {
		switch filepath.Base(os.Args[0]) {
		case "xdg-mime":
			if owner := os.Getenv("DEV_NULL_TEST_OWNER"); owner != "" {
				fmt.Println(owner)
			} else {
				files, _ := filepath.Glob(filepath.Join(dir, "applications", "*.desktop"))
				if len(files) > 0 {
					fmt.Println(filepath.Base(files[0]))
				}
			}
			os.Exit(0)
		case "update-desktop-database":
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func handlerEnvironment(t *testing.T) Store {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xdg-mime", "update-desktop-database"} {
		if err := os.Symlink(exe, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("DISPLAY", ":synthetic")
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("DEV_NULL_TEST_TOOL_DIR", dir)
	t.Setenv("DEV_NULL_TEST_OWNER", "")
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestTemporaryHandlerDoesNotReplaceAnotherOwner(t *testing.T) {
	store := handlerEnvironment(t)
	t.Setenv("DEV_NULL_TEST_OWNER", "another-app.desktop")
	a, _ := NewAttempt(DefaultOrigin, "SHA256:example", "Test device")
	if _, err := InstallHandler(t.Context(), store, a.State); err == nil {
		t.Fatal("replaced another application")
	}
	files, _ := filepath.Glob(filepath.Join(os.Getenv("XDG_DATA_HOME"), "applications", "*.desktop"))
	if len(files) != 0 {
		t.Fatal("left a handler behind after refusing ownership")
	}
}

func TestHandlerRecoveryLeavesActiveAttemptAndCleansInterruptedAttempt(t *testing.T) {
	store := handlerEnvironment(t)
	a, _ := NewAttempt(DefaultOrigin, "SHA256:example", "Test device")
	callback, err := ListenCallback(store, a.State)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	handler, err := InstallHandler(t.Context(), store, a.State)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handler.Close() }()
	path, _ := handler.desktopPath()
	if err := RecoverHandlers(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("removed an active handler")
	}
	callback.Close()
	if err := RecoverHandlers(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("interrupted handler was not removed")
	}
}

func TestRecoveryCleansOrphanedAttemptMetadata(t *testing.T) {
	store := handlerEnvironment(t)
	a, _ := NewAttempt(DefaultOrigin, "SHA256:example", "Test device")
	callback, err := ListenCallback(store, a.State)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	address, err := readCallbackAddress(store, a.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := callbackRequest(t.Context(), address, "GET", "/alive", ""); err != nil {
		t.Fatal(err)
	}
	if err := callback.server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RecoverHandlers(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(callback.path); !os.IsNotExist(err) {
		t.Fatal("orphaned attempt metadata survived recovery")
	}
}

func TestDesktopArgumentsEscapeFieldCodesAndRejectNewlines(t *testing.T) {
	quoted, err := desktopQuote(`/a path/100%/$hello"`)
	if err != nil || !strings.Contains(quoted, "100%%") || !strings.Contains(quoted, `\\$`) || !strings.Contains(quoted, `\\"`) {
		t.Fatalf("unsafe desktop quoting: %q %v", quoted, err)
	}
	if _, err := desktopQuote("/path\nExec=bad"); err == nil {
		t.Fatal("desktop entry injection allowed")
	}
}
