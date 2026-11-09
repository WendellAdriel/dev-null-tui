package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func keepTyped(t *testing.T, m AppModel, body string) AppModel {
	t.Helper()
	m.composer.input.SetValue(body)
	m = pressApp(m, "esc")
	if m.composer.confirm != "discard" {
		t.Fatal("closing a draft must ask first")
	}
	return pressApp(m, "ctrl+k")
}

func TestKeptDraftsAreSavedPerPlaceAndRestoredThere(t *testing.T) {
	m := fixtureApp(t)
	m = keepTyped(t, pressApp(m, "c"), "new commit draft")
	if m.composer.open || m.composer.hasDraft() {
		t.Fatal("Keep must close and clear the composer once the draft is saved")
	}
	saved := m.account.store.CommitDrafts(m.account.user.ID)
	if saved["commit"].Body != "new commit draft" || saved["commit"].Kind != "commit" {
		t.Fatalf("draft not saved to disk: %+v", saved)
	}
	a, b := fixtureCommit("a"), fixtureCommit("b")
	m.composeTo("reply", a)
	if m.composer.input.Value() != "" {
		t.Fatal("a reply must not open the new commit draft")
	}
	m = keepTyped(t, m, "reply to a")
	m.composeTo("reply", b)
	if m.composer.input.Value() != "" {
		t.Fatal("each reply target has its own draft")
	}
	m = pressApp(m, "esc")
	m.composeTo("reply", a)
	if m.composer.input.Value() != "reply to a" || m.composer.target.ID != "a" {
		t.Fatalf("reply draft not restored: %q", m.composer.input.Value())
	}
	m = pressApp(m, "esc")
	m = pressApp(m, "ctrl+d")
	if _, ok := m.account.store.CommitDrafts(m.account.user.ID)["reply:a"]; ok {
		t.Fatal("Discard must delete the saved draft")
	}
	m = pressApp(m, "c")
	if m.composer.input.Value() != "new commit draft" {
		t.Fatal("c must restore the new commit draft")
	}
}

func TestSubmittingADraftDeletesIt(t *testing.T) {
	m := fixtureApp(t)
	m = keepTyped(t, pressApp(m, "c"), "ship it")
	m = pressApp(m, "c")
	m.submit = func(_ context.Context, c api.NewCommit) (api.Commit, error) {
		return api.Commit{ID: "new", Body: c.Body, Status: "approved", Author: &api.Author{Handle: "fixture"}}, nil
	}
	next, _ := m.Update(m.startSubmission()())
	m = next.(AppModel)
	if len(m.account.store.CommitDrafts(m.account.user.ID)) != 0 || len(m.commitDrafts) != 0 {
		t.Fatal("a submitted draft must be deleted")
	}
}

func TestKeepOnExitSavesTheDraftAndQuits(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("keep me")
	m = pressApp(m, "ctrl+c")
	if m.composer.confirm != "exit" {
		t.Fatal("Ctrl+C with a draft must ask first")
	}
	next, cmd := m.Update(keyMsg("ctrl+k"))
	m = next.(AppModel)
	if m.account.store.CommitDrafts(m.account.user.ID)["commit"].Body != "keep me" {
		t.Fatal("Keep on exit must save the draft")
	}
	if cmd == nil {
		t.Fatal("Keep on exit must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Keep on exit must quit")
	}
}

func TestRestoredDraftDropsImagesThatCannotBeRead(t *testing.T) {
	m := fixtureApp(t)
	dir := t.TempDir()
	picture := pngFile(t, dir, "a.png")
	m.commitDrafts = map[string]auth.CommitDraft{"commit": {Kind: "commit", Body: "pics", Images: []auth.DraftImage{{Path: picture, Description: "kept"}, {Path: filepath.Join(dir, "gone.png")}}, Poll: nil}}
	m = pressApp(m, "c")
	if len(m.composer.images) != 1 || m.composer.images[0].Description != "kept" {
		t.Fatalf("readable image not restored: %+v", m.composer.images)
	}
	if !strings.Contains(m.composer.status, "dropped") {
		t.Fatalf("a dropped image must be explained: %q", m.composer.status)
	}
	m.commitDrafts["commit"] = auth.CommitDraft{Kind: "commit", Poll: &auth.DraftPoll{Options: []string{"Go", "Rust"}, Duration: 1440}}
	m.clearComposer()
	m = pressApp(m, "c")
	if m.composer.poll == nil || m.composer.poll.duration != 1440 || strings.Join(m.composer.poll.options, ",") != "Go,Rust" {
		t.Fatal("a saved poll must be restored")
	}
}

func TestDraftsScreenListsReopensAndDeletes(t *testing.T) {
	m := fixtureApp(t)
	m.account.width, m.account.height = 120, 35
	m.reflow()
	m = keepTyped(t, pressApp(m, "c"), "a new idea")
	m.composeTo("quote", fixtureCommit("q1"))
	m = keepTyped(t, m, "quoting this")
	m = keys(m, "g", "d")
	if !m.drafts.open {
		t.Fatal("g d must open Drafts")
	}
	text := frameText(m)
	for _, want := range []string{"Drafts", "Quote of @" + fixtureCommit("q1").Author.Handle, "│ Unicode 界", "quoting this", "New commit", "a new idea"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Drafts screen lacks %q:\n%s", want, text)
		}
	}
	loaded := ""
	m.loadThread = func(_ context.Context, id string) (api.Thread, error) {
		loaded = id
		return api.Thread{Commit: fixtureCommit(id)}, nil
	}
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(AppModel)
	next, _ = m.Update(cmd())
	m = next.(AppModel)
	if loaded != "q1" || !m.composer.open || m.composer.mode != "quote" || m.composer.input.Value() != "quoting this" {
		t.Fatalf("Enter must load the quoted commit and reopen its draft (loaded %q)", loaded)
	}
	m = pressApp(m, "esc")
	m = pressApp(m, "ctrl+k")
	m = keys(m, "g", "d", "D")
	if m.ask == nil {
		t.Fatal("deleting a draft must ask first")
	}
	m = keys(m, "ctrl+y")
	if len(m.commitDrafts) != 1 || len(m.account.store.CommitDrafts(m.account.user.ID)) != 1 {
		t.Fatalf("confirmed delete must remove one draft, left %d", len(m.commitDrafts))
	}
}

func TestKeepAfterAnUnknownOutcomeKeepsTheDuplicateWarning(t *testing.T) {
	m := fixtureApp(t)
	m = pressApp(m, "c")
	m.composer.input.SetValue("maybe sent")
	m.composer.uncertain = true
	m = pressApp(m, "esc")
	m = pressApp(m, "ctrl+k")
	if len(m.account.store.CommitDrafts(m.account.user.ID)) != 0 {
		t.Fatal("a draft with an unknown outcome must not be saved without its warning")
	}
	m = pressApp(m, "c")
	if !m.composer.uncertain || m.composer.input.Value() != "maybe sent" {
		t.Fatal("reopening must keep the duplicate-risk state")
	}
}
