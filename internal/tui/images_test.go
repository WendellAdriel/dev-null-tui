package tui

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
	"github.com/WendellAdriel/dev-null-cli/internal/graphics"
)

func TestGraphicsRequireProbeAndOnlyDownloadVisibleReadOnlyMedia(t *testing.T) {
	var downloads atomic.Int32
	paths := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") != "" || r.Header.Get("Authorization") != "" || r.Method != http.MethodGet {
			t.Error("media request was signed or writable")
		}
		downloads.Add(1)
		paths <- r.URL.Path
		im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		im.SetNRGBA(4, 4, color.NRGBA{R: 255, A: 255})
		if err := png.Encode(w, im); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	m := fixtureApp(t)
	m.account.profile = colorprofile.TrueColor
	m.images = graphics.New(server.URL)
	c := fixtureCommit("picture")
	c.Author.AvatarURL = server.URL + "/avatar.png"
	c.Images = []api.Image{{URL: server.URL + "/image.png", Width: 800, Height: 600, Description: "Synthetic image"}}
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	if m.syncImages() != nil || downloads.Load() != 0 {
		t.Fatal("images must not load before a positive capability response")
	}
	updated, commands := m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.images.ProbeID}, Payload: []byte("OK")})
	m = updated.(AppModel)
	if !m.images.Enabled || commands == nil {
		t.Fatal("positive capability response did not schedule visible media")
	}
	var results []graphics.Result
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, next := range msg {
				run(next)
			}
		case graphics.Result:
			results = append(results, msg)
		}
	}
	run(commands)
	if downloads.Load() != 2 {
		t.Fatalf("expected only the visible avatar and picture, got %d", downloads.Load())
	}
	if first := <-paths; first != "/image.png" {
		t.Fatalf("visible photo was scheduled behind an avatar: %s", first)
	}
	for _, result := range results {
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		updated, _ = m.Update(result)
		m = updated.(AppModel)
	}
	if strings.Count(m.feeds[0].viewport.GetContent(), string(kitty.Placeholder)) < 8 {
		t.Fatal("ready avatars and inline images did not reach the feed")
	}
	m.leaveApplication()
	cleanup := m.syncImages()
	if cleanup == nil {
		t.Fatal("leaving a session did not delete terminal images")
	}
	for _, result := range results {
		updated, _ = m.Update(result)
		m = updated.(AppModel)
	}
	if strings.Contains(m.feeds[0].viewport.GetContent(), string(kitty.Placeholder)) {
		t.Fatal("stale media restored private content after logout")
	}
}

func TestGraphicsProbeKeepsLimitedColorProfilesInTextMode(t *testing.T) {
	m := fixtureApp(t)
	for _, reply := range []uv.KittyGraphicsEvent{{Options: kitty.Options{ID: m.images.ProbeID + 1}, Payload: []byte("OK")}, {Options: kitty.Options{ID: m.images.ProbeID}, Payload: []byte("ENOTSUP")}} {
		updated, _ := m.Update(reply)
		m = updated.(AppModel)
		if m.images.Enabled {
			t.Fatal("unrelated or negative probe enabled graphics")
		}
	}
	updated, _ := m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.images.ProbeID}, Payload: []byte("OK")})
	m = updated.(AppModel)
	for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
		updated, _ = m.Update(tea.ColorProfileMsg{Profile: profile})
		m = updated.(AppModel)
		if m.images.Enabled != (profile == colorprofile.TrueColor) {
			t.Fatal("graphics ignored the renderer's truecolor requirement")
		}
	}
}

func TestGitHubAvatarRequestsUseThumbnailSizes(t *testing.T) {
	m := fixtureApp(t)
	for _, raw := range []string{"https://avatars.githubusercontent.com/u/1", "https://avatars.githubusercontent.com/u/1?v=4"} {
		req := m.avatarRequest(raw, "#ff8a4c", 4, 2)
		if !strings.Contains(req.URL, "s=96") {
			t.Fatal("avatar request still downloads the default full-size image")
		}
	}
	for _, raw := range []string{"https://example.test/avatar?signature=unchanged", "https://avatars.githubusercontent.com.evil.test/u/1", "https://avatars.githubusercontent.com/u/1?s=48"} {
		if m.avatarRequest(raw, "#ff8a4c", 4, 2).URL != raw {
			t.Fatal("avatar optimization changed an unrelated or explicitly sized URL")
		}
	}
	m.images.Enabled = true
	author := &api.Author{AvatarURL: "https://avatars.githubusercontent.com/u/1?v=4", Lane: []byte("0")}
	req := m.avatarRequest(author.AvatarURL, authorColor(author, colors(true)), 2, 1)
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{req})
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	if !strings.Contains(smallAvatar(author, colors(true), m.images), string(kitty.Placeholder)) {
		t.Fatal("small avatars do not use the same thumbnail URL as their downloads")
	}
}

func TestStatsStarUsesCellSizedIconInLogThreadAndViewer(t *testing.T) {
	m := fixtureApp(t)
	m.images.Enabled = true
	c := fixtureCommit("starred")
	c.Starred, c.Stars = new(true), 2
	m = applyPage(m, 0, api.Page{Data: []api.Commit{c}})
	p := colors(m.account.dark)
	req := starIconRequest(true, p.muted)
	if req.Cols != 2 || req.Rows != 1 || !slices.Contains(m.visibleMedia(), req) {
		t.Fatalf("filled star icon not requested: %+v", req)
	}
	m.images.Clear()
	jobs, _ := m.images.Plan(t.Context(), []graphics.Request{req})
	m.images.Accept(graphics.Result{Job: jobs[0], Raw: "ready"})
	m.reflow()
	starred := func(lines []string) bool {
		for _, line := range lines {
			if strings.Contains(line, string(kitty.Placeholder)) && strings.Contains(ansi.Strip(line), " 2 ") {
				return true
			}
		}
		return false
	}
	if !starred(strings.Split(m.feeds[0].viewport.GetContent(), "\n")) {
		t.Fatal("log stats keep the text star")
	}
	m.viewer.commit = c
	if !starred(m.viewerPanelLines(60, p)) {
		t.Fatal("viewer stats keep the text star")
	}
	m.images.Clear()
	if lines, _ := commitLayout(c, "", 60, p, m.now, nil, m.statsStyle()); !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "★ 2") {
		t.Fatal("stats lost the text fallback without graphics")
	}
}
