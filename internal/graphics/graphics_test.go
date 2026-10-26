package graphics

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func picture(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			im.SetNRGBA(x, y, color.NRGBA{R: 180, G: 90, B: 40, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestRoundAvatarAndImageSizing(t *testing.T) {
	out, err := prepare(picture(t), Request{Cols: 4, Rows: 2, Ring: "#ff8a4c"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 96 || out.NRGBAAt(0, 0).A != 0 || out.NRGBAAt(48, 48).A != 255 || out.NRGBAAt(48, 2).R != 255 {
		t.Fatal("avatar must be round with an author lane ring")
	}
	cols, rows := Fit(1600, 900, 40, 12)
	if cols != 40 || rows != 11 {
		t.Fatalf("landscape fit: %dx%d", cols, rows)
	}
	cols, rows = Fit(900, 1600, 40, 12)
	if cols != 14 || rows != 12 {
		t.Fatalf("portrait fit: %dx%d", cols, rows)
	}
	if cols, rows = Fit(0, 100, 40, 12); cols != 0 || rows != 0 {
		t.Fatal("invalid dimensions cannot reserve an image")
	}
}

func TestGraphicsLifecycleAndPlaceholderWidth(t *testing.T) {
	data := picture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials leaked to image request")
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	m := New(server.URL)
	m.Enabled = true
	req := Request{URL: server.URL + "/avatar", Cols: 4, Rows: 2, Ring: "#ff8a4c"}
	jobs, cleanup := m.Plan(t.Context(), []Request{req, req})
	if len(jobs) != 1 || cleanup != "" {
		t.Fatal("image downloads must be deduplicated")
	}
	result := m.Fetch(jobs[0])
	output := m.Accept(result)
	if !strings.Contains(output, "U=1") || !strings.Contains(output, "f=100") || !strings.Contains(output, "q=2") {
		t.Fatal("missing virtual PNG placement")
	}
	row := m.Row(req, 0)
	if ansi.StringWidth(row) != 4 || strings.Count(row, string(kitty.Placeholder)) != 4 {
		t.Fatalf("placeholder cells exceed image bounds: %q", row)
	}
	if !strings.Contains(m.Clear(), "a=d") || m.Row(req, 0) != "" {
		t.Fatal("terminal resources survived cleanup")
	}
	if m.Accept(result) != "" {
		t.Fatal("stale image response reallocated a removed image")
	}
}

func TestUnsafeOrOversizedImagesFallBackWithoutEchoingURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "http://169.254.169.254/private?secret=example")
			w.WriteHeader(302)
			return
		}
		w.Header().Set("Content-Length", "20000000")
		_, _ = io.WriteString(w, "not an image")
	}))
	defer server.Close()
	m := New(server.URL)
	m.Enabled = true
	for _, target := range []string{server.URL + "/redirect", server.URL + "/large", "file:///etc/passwd", "http://127.0.0.1:1/private", "https://user:secret@example.test/image"} {
		jobs, _ := m.Plan(t.Context(), []Request{{URL: target, Cols: 4, Rows: 2}})
		if len(jobs) == 0 {
			continue
		}
		result := m.Fetch(jobs[0])
		if result.Err == nil || strings.Contains(result.Err.Error(), "secret") || m.Accept(result) != "" {
			t.Fatalf("unsafe image failure: %v", result.Err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	jobs, _ := m.Plan(ctx, []Request{{URL: server.URL + "/cancel", Cols: 4, Rows: 2}})
	if len(jobs) > 0 && m.Fetch(jobs[0]).Err == nil {
		t.Fatal("canceled image fetch succeeded")
	}
}

func TestVisibleImageBudgetAndTmuxWrapping(t *testing.T) {
	m := New("https://example.test")
	m.Enabled, m.Tmux = true, true
	var requests []Request
	for i := range 100 {
		requests = append(requests, Request{URL: "https://example.test/" + strings.Repeat("x", i+1), Cols: 4, Rows: 2})
	}
	jobs, _ := m.Plan(t.Context(), requests)
	if len(jobs) > 2 || len(m.entries) > MaxImages {
		t.Fatal("image concurrency or resource budget exceeded")
	}
	for len(jobs) > 0 {
		for _, job := range jobs {
			m.Accept(Result{Job: job, Err: io.EOF})
		}
		jobs, _ = m.Plan(t.Context(), requests)
	}
	if len(m.entries) != MaxImages {
		t.Fatal("visible image budget did not cap the pending request list")
	}
	if !strings.HasPrefix(m.Probe(), "\x1bPtmux;") {
		t.Fatal("tmux graphics must use passthrough")
	}
	if !strings.Contains(m.Clear(), "tmux;") && len(m.entries) != 0 {
		t.Fatal("cleanup did not empty image state")
	}
}

func TestDownloadsStartInVisiblePriorityOrderWithoutAvatarBacklog(t *testing.T) {
	m := New("https://example.test")
	m.Enabled = true
	requests := []Request{{URL: "https://example.test/photo", Cols: 40, Rows: 12}}
	for _, name := range []string{"ada", "lin", "sam", "kenji"} {
		requests = append(requests, Request{URL: "https://example.test/" + name, Cols: 4, Rows: 2, Ring: "#ff8a4c"})
	}
	jobs, _ := m.Plan(t.Context(), requests)
	if len(jobs) != 2 || jobs[0].Request != requests[0] || jobs[1].Request != requests[1] {
		t.Fatalf("expected photo and first avatar without queued goroutines, got %d jobs", len(jobs))
	}
	if next, _ := m.Plan(t.Context(), requests); len(next) != 0 {
		t.Fatal("pending work exceeded the two active downloads")
	}
	m.Accept(Result{Job: jobs[1], Err: io.EOF})
	next, _ := m.Plan(t.Context(), requests)
	if len(next) != 1 || next[0].Request != requests[2] {
		t.Fatal("finishing an image did not start the next visible request")
	}
}

func TestAvatarSizesReuseOneDownloadAndKeepAFreePhotoSlot(t *testing.T) {
	var downloads atomic.Int32
	data := picture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { downloads.Add(1); _, _ = w.Write(data) }))
	defer server.Close()
	m := New(server.URL)
	m.Enabled = true
	small := Request{URL: server.URL + "/avatar", Cols: 2, Rows: 1, Ring: "#ff8a4c"}
	large := small
	large.Cols, large.Rows = 4, 2
	jobs, _ := m.Plan(t.Context(), []Request{small, large})
	if len(jobs) != 1 {
		t.Fatal("two avatar sizes must not occupy both download slots")
	}
	photo := Request{URL: server.URL + "/photo", Cols: 40, Rows: 12}
	next, _ := m.Plan(t.Context(), []Request{photo, small, large})
	if len(next) != 1 || next[0].Request != photo {
		t.Fatal("visible photo waited behind the second avatar size")
	}
	if m.Accept(m.Fetch(jobs[0])) == "" {
		t.Fatal("avatar did not load")
	}
	next, _ = m.Plan(t.Context(), []Request{large})
	if len(next) != 1 || next[0].Request != large {
		t.Fatal("second placement was not scheduled")
	}
	if m.Accept(m.Fetch(next[0])) == "" || m.Row(large, 0) == "" {
		t.Fatal("reused avatar did not render")
	}
	if downloads.Load() != 1 {
		t.Fatalf("avatar variants downloaded %d times", downloads.Load())
	}
	m.Clear()
	jobs, _ = m.Plan(t.Context(), []Request{small})
	m.Accept(m.Fetch(jobs[0]))
	if downloads.Load() != 2 {
		t.Fatal("session cleanup retained avatar pixels")
	}
}
