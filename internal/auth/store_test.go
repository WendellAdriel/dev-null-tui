package auth

import (
	"os"
	"path/filepath"
	"runtime"
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
