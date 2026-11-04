package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStorePreservesKeyAndDisconnectDeletesIt(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	key, _ := GenerateKey()
	if err := store.Save(key, DefaultOrigin); err != nil {
		t.Fatal(err)
	}
	loaded, origin, err := store.Load()
	if err != nil || origin != DefaultOrigin || loaded.Fingerprint() != key.Fingerprint() {
		t.Fatalf("stored connection not recovered: %v", err)
	}
	if err := store.Save(key, "https://other.example"); err == nil {
		t.Fatal("overwrote existing credential")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(store.Dir, "connection.json"))
		if info.Mode().Perm() != 0600 {
			t.Fatal("key permissions not private")
		}
	}
	if err := store.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(); !os.IsNotExist(err) {
		t.Fatalf("credential survived disconnect: %v", err)
	}
}

func TestStoreRefusesUnsafePathsAndCorruptCredentials(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Dir, "connection.json")
	if err := os.WriteFile(path, []byte("not a key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(); err == nil {
		t.Fatal("corrupt credential ignored")
	}
	if err := store.Save(nil, DefaultOrigin); err == nil {
		t.Fatal("corrupt credential overwritten")
	}
	if runtime.GOOS == "windows" {
		return
	}
	// Deliberately expose the test directory to verify rejection of unsafe permissions.
	if err := os.Chmod(store.Dir, 0755); err != nil { // #nosec G302
		t.Fatal(err)
	}
	if err := store.Prepare(); err == nil {
		t.Fatal("unsafe directory accepted")
	}
	// Directories require owner traversal permission as well as read/write permission.
	if err := os.Chmod(store.Dir, 0700); err != nil { // #nosec G302
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(); err == nil {
		t.Fatal("followed credential symlink")
	}
	// The target is a synthetic file created inside this test's private temporary directory.
	if data, _ := os.ReadFile(target); string(data) != "untouched" { // #nosec G304
		t.Fatal("changed symlink target")
	}
}

func TestOnlyOneAccountSessionCanChangeCredentials(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	lock, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if second, err := store.Lock(); err == nil {
		_ = second.Close()
		t.Fatal("overlapping account session allowed")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := store.Lock()
	if err != nil {
		t.Fatal("session did not release its lock")
	}
	_ = again.Close()
}

func TestStoreRemembersTheLastSeenVersionApartFromTheConnection(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	if store.LastSeenVersion() != "" || store.HasConnection() {
		t.Fatal("empty store reported state")
	}
	for _, version := range []string{"v0.2.0", "v0.3.0"} {
		if err := store.SaveSeenVersion(version); err != nil {
			t.Fatal(err)
		}
		if got := store.LastSeenVersion(); got != version {
			t.Fatalf("got %q, want %q", got, version)
		}
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "last-seen-version"), []byte(strings.Repeat("v", 200)), 0600); err != nil {
		t.Fatal(err)
	}
	if store.LastSeenVersion() != "" {
		t.Fatal("oversized state must read as unseen")
	}
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(key, "https://dev-null.sh"); err != nil || !store.HasConnection() {
		t.Fatal("saved connection not detected")
	}
	if err := store.SaveSeenVersion("v0.2.0"); err != nil {
		t.Fatal(err)
	}
	if err := store.Disconnect(); err != nil || store.LastSeenVersion() != "v0.2.0" {
		t.Fatal("disconnecting must keep the last seen version")
	}
}

func TestStoreKeepsTUISettingsWithSafeDefaults(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	if got := store.TUISettings(); !got.CheckForUpdates || got.SkippedVersion != "" || got.PinWarningDismissed {
		t.Fatalf("missing file must default to checking for updates: %+v", got)
	}
	want := TUISettings{CheckForUpdates: false, SkippedVersion: "v0.3.0", PinWarningDismissed: true}
	if err := store.SaveTUISettings(want); err != nil {
		t.Fatal(err)
	}
	if got := store.TUISettings(); got.CheckForUpdates || got.SkippedVersion != "v0.3.0" || !got.PinWarningDismissed {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if err := store.Disconnect(); err != nil || store.TUISettings().SkippedVersion != "v0.3.0" {
		t.Fatal("disconnecting must keep TUI settings")
	}
	for _, corrupt := range []string{"{", `{"check_for_updates":"yes"}`, strings.Repeat(" ", 5000) + "{}"} {
		if err := os.WriteFile(filepath.Join(store.Dir, "tui.json"), []byte(corrupt), 0600); err != nil {
			t.Fatal(err)
		}
		if got := store.TUISettings(); !got.CheckForUpdates || got.SkippedVersion != "" {
			t.Fatalf("corrupt settings %q must read as defaults: %+v", corrupt[:min(len(corrupt), 20)], got)
		}
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "tui.json"), []byte(`{"skipped_version":"v0.4.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := store.TUISettings(); !got.CheckForUpdates || got.SkippedVersion != "v0.4.0" {
		t.Fatalf("missing keys must keep their defaults: %+v", got)
	}
}
