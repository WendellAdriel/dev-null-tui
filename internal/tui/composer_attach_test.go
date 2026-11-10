package tui

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/auth"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func keyMsg(name string) tea.KeyPressMsg {
	codes := map[string]rune{"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "delete": tea.KeyDelete, "backspace": tea.KeyBackspace, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight}
	mod := tea.KeyMod(0)
	if rest, ok := strings.CutPrefix(name, "alt+"); ok {
		mod, name = tea.ModAlt, rest
	}
	if rest, ok := strings.CutPrefix(name, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(rest)[0], Mod: tea.ModCtrl}
	}
	if name == "shift+tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if code, ok := codes[name]; ok {
		return tea.KeyPressMsg{Code: code, Mod: mod}
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name, Mod: mod}
}

func keys(m AppModel, names ...string) AppModel {
	for _, name := range names {
		next, _ := m.Update(keyMsg(name))
		m = next.(AppModel)
	}
	return m
}

func typeText(m AppModel, value string) AppModel {
	for _, r := range value {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(AppModel)
	}
	return m
}

func paste(m AppModel, value string) AppModel {
	next, _ := m.Update(tea.PasteMsg{Content: value})
	return next.(AppModel)
}

func pngFile(t *testing.T, dir, name string) string {
	t.Helper()
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 32, 16)))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func attachDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_ = os.Mkdir(filepath.Join(dir, "shots"), 0o700)
	pngFile(t, dir, "chart.png")
	pngFile(t, filepath.Join(dir, "shots"), "inner.png")
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "logo.svg"), []byte("<svg/>"), 0o600)
	return dir
}

func composerApp(t *testing.T) AppModel {
	t.Helper()
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = next.(AppModel)
	return typeText(keys(m, "c"), "Draft text")
}

func TestFileBrowserListsFoldersAndImagesAndAttachesAPick(t *testing.T) {
	m := composerApp(t)
	dir := attachDir(t)
	m.imageDir = dir
	m = keys(m, "ctrl+o")
	if m.composer.browser == nil || !m.composer.open {
		t.Fatal("ctrl+o did not open the file browser")
	}
	var names []string
	for _, entry := range m.composer.browser.entries {
		names = append(names, entry.name)
	}
	if strings.Join(names, ",") != "shots/,chart.png" {
		t.Fatalf("browser entries %v", names)
	}
	if !strings.Contains(frameText(m), "shots/") || m.upgradeVisible() {
		t.Fatalf("browser not drawn inside the composer:\n%s", frameText(m))
	}
	m = keys(m, "enter")
	if m.composer.browser.dir != filepath.Join(dir, "shots") {
		t.Fatal("enter did not open the folder")
	}
	m = keys(m, "backspace")
	m = typeText(m, "ch art")
	if m.composer.browser.filter != "ch art" {
		t.Fatalf("typed filter %q", m.composer.browser.filter)
	}
	m = keys(m, "backspace", "backspace", "backspace", "backspace")
	m = typeText(m, "a")
	if len(m.composer.browser.entries) != 1 {
		t.Fatal("typing did not filter")
	}
	m = keys(m, "enter")
	if m.composer.browser != nil || len(m.composer.images) != 1 || m.composer.images[0].Name != "chart.png" || m.composer.input.Value() != "Draft text" {
		t.Fatalf("pick not attached: %+v", m.composer.images)
	}
	if m.imageDir != dir {
		t.Fatal("browser did not remember its folder")
	}
	if !strings.Contains(frameText(m), "chart.png · 32×16") {
		t.Fatalf("strip missing:\n%s", frameText(m))
	}
}

func TestDroppedPathsAttachImagesAndRejectOthersWithAReason(t *testing.T) {
	m := composerApp(t)
	if !strings.Contains(frameText(m), "Images (or drop)  ctrl+o") {
		t.Fatalf("composer hides the drop hint:\n%s", frameText(m))
	}
	dir := attachDir(t)
	m = paste(m, "'"+filepath.Join(dir, "chart.png")+"'")
	if len(m.composer.images) != 1 || m.composer.input.Value() != "Draft text" {
		t.Fatalf("drop not attached: %q %d", m.composer.input.Value(), len(m.composer.images))
	}
	m = paste(m, filepath.Join(dir, "logo.svg"))
	if len(m.composer.images) != 1 || !strings.Contains(m.composer.status, "logo.svg is not a JPEG, PNG, or WebP image") || m.composer.input.Value() != "Draft text" {
		t.Fatalf("svg: %q %q", m.composer.status, m.composer.input.Value())
	}
	m = paste(m, " more words")
	if m.composer.input.Value() != "Draft text more words" {
		t.Fatalf("text paste became a path: %q", m.composer.input.Value())
	}
	for range 3 {
		m = paste(m, filepath.Join(dir, "chart.png"))
	}
	if len(m.composer.images) != api.MaxImages || !strings.Contains(m.composer.status, "at most 3") {
		t.Fatalf("limit: %d %q", len(m.composer.images), m.composer.status)
	}
}

func TestPollAndImagesStayExclusiveWithAReason(t *testing.T) {
	m := composerApp(t)
	dir := attachDir(t)
	m = paste(m, filepath.Join(dir, "chart.png"))
	m = keys(m, "ctrl+l")
	if m.composer.poll != nil || !strings.Contains(m.composer.status, "Polls can't go with images") {
		t.Fatalf("poll beside images: %q", m.composer.status)
	}
	m = composerApp(t)
	m = keys(m, "ctrl+l", "esc")
	m = keys(m, "ctrl+o")
	if m.composer.browser != nil || !strings.Contains(m.composer.status, "Remove the poll to add images") {
		t.Fatalf("images beside poll: %q", m.composer.status)
	}
	for mode, noun := range map[string]string{"reply": "replies", "quote": "quotes"} {
		m := fixtureApp(t)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
		m = next.(AppModel)
		m.composeTo(mode, fixtureCommit("target"))
		m = keys(m, "ctrl+l")
		if m.composer.poll != nil || !strings.Contains(m.composer.status, "Polls can't go on "+noun) {
			t.Fatalf("%s poll: %q", mode, m.composer.status)
		}
		if !strings.Contains(frameText(m), "Poll") {
			t.Fatalf("%s hides the disabled poll hint", mode)
		}
	}
}

func TestPollEditorKeys(t *testing.T) {
	m := composerApp(t)
	m = keys(m, "ctrl+l")
	if m.composer.poll == nil || len(m.composer.poll.options) != 2 || m.composer.editing != "option" {
		t.Fatal("ctrl+l did not start a poll on its first option")
	}
	m = typeText(m, "Tabs")
	m = keys(m, "enter", "down", "enter")
	m = typeText(m, "Spaces")
	m = keys(m, "enter", "ctrl+n")
	if len(m.composer.poll.options) != 3 || m.composer.editing != "option" {
		t.Fatal("ctrl+n did not add an option")
	}
	m = typeText(m, "Both")
	m = keys(m, "enter", "right")
	if m.composer.poll.duration != 2 || !strings.Contains(frameText(m), "3 days") {
		t.Fatalf("duration did not change:\n%s", frameText(m))
	}
	m = keys(m, "delete")
	if strings.Join(m.composer.poll.options, ",") != "Tabs,Spaces" {
		t.Fatalf("delete removed the wrong option: %v", m.composer.poll.options)
	}
	m = keys(m, "delete")
	if len(m.composer.poll.options) != 2 || !strings.Contains(m.composer.status, "at least 2") {
		t.Fatal("poll dropped below two options")
	}
	var sent api.NewCommit
	m.submit = func(_ context.Context, draft api.NewCommit) (api.Commit, error) {
		sent = draft
		c := fixtureCommit("new")
		c.Author.ID, c.Status = "fixture", "pending"
		return c, nil
	}
	next, cmd := m.Update(keyMsg("ctrl+s"))
	m = deliver(next.(AppModel), runAll(cmd))
	if sent.Poll == nil || strings.Join(sent.Poll.Options, ",") != "Tabs,Spaces" || sent.Poll.Duration != 4320 || sent.Body != "Draft text" {
		t.Fatalf("submitted %+v", sent)
	}
	m = composerApp(t)
	m = keys(m, "ctrl+l", "esc", "ctrl+x")
	if m.composer.poll != nil {
		t.Fatal("ctrl+x did not remove the poll")
	}
}

func TestImageStripEditsDescriptionsAndReorders(t *testing.T) {
	m := composerApp(t)
	dir := attachDir(t)
	m = paste(m, filepath.Join(dir, "chart.png")+" "+filepath.Join(dir, "shots", "inner.png"))
	m = keys(m, "tab")
	if m.composer.focus != composerStrip {
		t.Fatal("tab did not reach the attachment strip")
	}
	m = keys(m, "enter")
	m = typeText(m, "A chart")
	m = keys(m, "enter", "alt+down")
	if m.composer.images[1].Name != "chart.png" || m.composer.images[1].Description != "A chart" || m.composer.strip != 1 {
		t.Fatalf("reorder: %+v", m.composer.images)
	}
	m = keys(m, "delete")
	if len(m.composer.images) != 1 || m.composer.images[0].Name != "inner.png" {
		t.Fatal("delete removed the wrong image")
	}
}

func TestAttachmentsSurviveRefusalsAndUnknownOutcomes(t *testing.T) {
	m := composerApp(t)
	dir := attachDir(t)
	m = paste(m, filepath.Join(dir, "chart.png"))
	failures := []error{&auth.APIError{Status: 422, FieldErrors: []string{"The images.0 field must be smaller."}}, errors.New("connection dropped")}
	for _, failure := range failures {
		m.submit = func(context.Context, api.NewCommit) (api.Commit, error) { return api.Commit{}, failure }
		next, cmd := m.Update(keyMsg("ctrl+s"))
		m = deliver(next.(AppModel), runAll(cmd))
		if len(m.composer.images) != 1 || m.composer.input.Value() != "Draft text" {
			t.Fatalf("%v lost the draft", failure)
		}
	}
	if !m.composer.uncertain {
		t.Fatal("unknown outcome not marked")
	}
}

func TestImagesAllowAnEmptyBodyAndCountAsADraft(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = keys(next.(AppModel), "c")
	m = paste(m, filepath.Join(attachDir(t), "chart.png"))
	m = keys(m, "esc")
	if m.composer.confirm != "discard" {
		t.Fatal("an image-only draft closed without protection")
	}
	m = keys(m, "esc")
	var sent api.NewCommit
	var deadline time.Time
	m.submit = func(ctx context.Context, draft api.NewCommit) (api.Commit, error) {
		sent = draft
		deadline, _ = ctx.Deadline()
		c := fixtureCommit("new")
		c.Author.ID, c.Status = "fixture", "pending"
		return c, nil
	}
	next, cmd := m.Update(keyMsg("ctrl+s"))
	m = deliver(next.(AppModel), runAll(cmd))
	if len(sent.Images) != 1 || sent.Body != "" || m.composer.open {
		t.Fatalf("image-only submit: %+v open=%v status=%q", sent, m.composer.open, m.composer.status)
	}
	if time.Until(deadline) < auth.UploadTimeout-time.Second {
		t.Fatalf("an upload gets only %v before the composer gives up", time.Until(deadline))
	}
}

func TestCompactComposerKeepsThreeEditorRowsWithAttachments(t *testing.T) {
	m := fixtureApp(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m = keys(next.(AppModel), "c")
	dir := attachDir(t)
	chart := filepath.Join(dir, "chart.png")
	m = paste(m, chart+" "+chart+" "+chart)
	if m.composer.input.Height() < 3 {
		t.Fatalf("editor shrank to %d rows", m.composer.input.Height())
	}
	if !strings.Contains(frameText(m), "more") {
		t.Fatalf("compact strip lacks the more line:\n%s", frameText(m))
	}
}

func TestPreviewSendsOnlyTheBodyAndDrawsAttachmentsLocally(t *testing.T) {
	m := composerApp(t)
	var bodies []string
	m.preview = func(_ context.Context, body string) (api.Preview, error) {
		bodies = append(bodies, body)
		return api.Preview{Body: body}, nil
	}
	m = keys(m, "ctrl+l")
	m = typeText(m, "Tabs")
	m = keys(m, "enter", "down", "enter")
	m = typeText(m, "Spaces")
	m = keys(m, "enter")
	next, cmd := m.Update(keyMsg("ctrl+p"))
	m = deliver(next.(AppModel), runAll(cmd))
	frame := frameText(m)
	if len(bodies) != 1 || bodies[0] != "Draft text" || !m.composer.previewing {
		t.Fatalf("preview sent %q", bodies)
	}
	for _, want := range []string{"Tabs", "Spaces", "Runs 1 day once approved"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("preview lacks %q:\n%s", want, frame)
		}
	}
	m = fixtureApp(t)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = keys(next.(AppModel), "c")
	m.preview = func(context.Context, string) (api.Preview, error) {
		t.Fatal("an image-only preview called the server")
		return api.Preview{}, nil
	}
	m = paste(m, filepath.Join(attachDir(t), "chart.png"))
	m.images.Enabled = true
	workspace := m.visibleMedia()
	next, cmd = m.Update(keyMsg("ctrl+p"))
	m = deliver(next.(AppModel), runAll(cmd))
	if !m.composer.previewing || !strings.Contains(frameText(m), "Local preview") {
		t.Fatalf("image-only preview:\n%s", frameText(m))
	}
	found := false
	for _, req := range m.visibleMedia() {
		found = found || req.Path == m.composer.images[0].Path
	}
	if !found {
		t.Fatal("preview did not request the local thumbnail")
	}
	for _, req := range workspace {
		if req.Path == "" && !slices.Contains(m.visibleMedia(), req) {
			t.Fatalf("preview dropped the workspace's %+v", req)
		}
	}
}

func TestAttachedImagesShowThumbnailsInsteadOfNames(t *testing.T) {
	m := composerApp(t)
	m.images.Enabled = true
	dir := attachDir(t)
	m = paste(m, filepath.Join(dir, "chart.png")+" "+filepath.Join(dir, "shots", "inner.png"))
	frame := frameText(m)
	if strings.Contains(frame, "chart.png") || strings.Contains(frame, "inner.png") {
		t.Fatalf("strip still lists file names:\n%s", frame)
	}
	if !strings.Contains(frame, "Image 1 · 32×16 · No description") {
		t.Fatalf("strip lacks the selected image's details:\n%s", frame)
	}
	var thumbs []graphics.Request
	for _, req := range m.visibleMedia() {
		if req.Path != "" {
			thumbs = append(thumbs, req)
		}
	}
	if len(thumbs) != 2 || thumbs[0].Path != m.composer.images[0].Path || thumbs[0].Rows > stripThumbRows || thumbs[0].Cols > stripThumbCols {
		t.Fatalf("strip thumbnails %+v", thumbs)
	}
	m = keys(m, "tab", "down")
	if !strings.Contains(frameText(m), "Image 2 · 32×16") {
		t.Fatalf("moving in the strip did not change the details:\n%s", frameText(m))
	}
	m.images.Enabled = false
	if !strings.Contains(frameText(m), "chart.png · 32×16") {
		t.Fatal("terminals without graphics lost the text list")
	}
}

func TestFileBrowserShowsAThumbnailOfTheSelectedImage(t *testing.T) {
	m := composerApp(t)
	m.images.Enabled = true
	dir := attachDir(t)
	m.imageDir = dir
	m = keys(m, "ctrl+o")
	thumbnail := func(m AppModel) (graphics.Request, bool) {
		for _, req := range m.visibleMedia() {
			if req.Path != "" {
				return req, true
			}
		}
		return graphics.Request{}, false
	}
	if _, ok := thumbnail(m); ok {
		t.Fatal("a folder row requested a thumbnail")
	}
	m = keys(m, "down")
	req, ok := thumbnail(m)
	if !ok || req.Path != filepath.Join(dir, "chart.png") {
		t.Fatalf("selected image not previewed: %+v", req)
	}
	if !strings.Contains(frameText(m), "32×16") {
		t.Fatalf("preview pane lacks the image size:\n%s", frameText(m))
	}
}
