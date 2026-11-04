package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMessageDraftsAreKeptPerAccountAndPrivately(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	if got := store.MessageDrafts("u-1"); len(got) != 0 {
		t.Fatalf("missing file must read as no drafts: %+v", got)
	}
	drafts := map[string]MessageDraft{
		"d1":   {Body: "lunch?", ReplyToID: 7, Images: []DraftImage{{Path: "/tmp/a.png", Description: "a chart"}}},
		"org1": {Body: "standup"},
	}
	if err := store.SaveMessageDrafts("u-1", drafts); err != nil {
		t.Fatal(err)
	}
	got := store.MessageDrafts("u-1")
	if len(got) != 2 || got["d1"].Body != "lunch?" || got["d1"].ReplyToID != 7 || got["d1"].Images[0].Description != "a chart" {
		t.Fatalf("drafts not recovered: %+v", got)
	}
	if other := store.MessageDrafts("u-2"); len(other) != 0 {
		t.Fatalf("another account read the drafts: %+v", other)
	}
	if runtime.GOOS != "windows" {
		matches, _ := filepath.Glob(filepath.Join(store.Dir, "message-drafts-*.json"))
		info, err := os.Stat(matches[0])
		if len(matches) != 1 || err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("drafts file %v is not private: %v", matches, err)
		}
	}
	if err := store.SaveMessageDrafts("u-1", map[string]MessageDraft{}); err != nil {
		t.Fatal(err)
	}
	if matches, _ := filepath.Glob(filepath.Join(store.Dir, "message-drafts-*.json")); len(matches) != 0 {
		t.Fatalf("saving no drafts must remove the file: %v", matches)
	}
}

func TestDisconnectDeletesEveryAccountsMessageDrafts(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{"u-1", "u-2"} {
		if err := store.SaveMessageDrafts(account, map[string]MessageDraft{"d1": {Body: "private"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if matches, _ := filepath.Glob(filepath.Join(store.Dir, "message-drafts-*")); len(matches) != 0 {
		t.Fatalf("drafts survived disconnecting: %v", matches)
	}
}

func TestMessageDraftsRefuseUnsafeAccountsAndCorruptFiles(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "dev-null-cli")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessageDrafts("../escape", map[string]MessageDraft{"d1": {Body: "x"}}); err == nil {
		t.Fatal("saved drafts under an unsafe account id")
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "message-drafts-u-1.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := store.MessageDrafts("u-1"); len(got) != 0 {
		t.Fatalf("corrupt drafts must read as none: %+v", got)
	}
}
