package graphics

import (
	"strings"
	"testing"
)

func TestNotificationIconsAreLocalCircularAndIsolatedByKind(t *testing.T) {
	m := New("https://example.test")
	m.Enabled = true
	for _, kind := range []string{"starred", "followed", "approved", "rejected"} {
		req := Request{Icon: kind, Ring: "#123abc", Cols: 5, Rows: 2}
		jobs, _ := m.Plan(t.Context(), []Request{req})
		if len(jobs) != 1 {
			t.Fatalf("no icon job for %s", kind)
		}
		result := m.Fetch(jobs[0])
		if result.Err != nil || !strings.Contains(m.Accept(result), "f=100") {
			t.Fatalf("local icon %s: %v", kind, result.Err)
		}
		im := result.avatar
		if im == nil || im.NRGBAAt(0, 0).A != 0 || im.NRGBAAt(48, 3).A == 0 || im.NRGBAAt(48, 3).B != 0xbc {
			t.Fatal("icon missing transparent corners or lane circle")
		}
		if jobs[0].avatar != nil {
			t.Fatal("different icon reused preceding shape")
		}
	}
	if m.sources.size != 0 {
		t.Fatal("local icons used the download cache")
	}
	if cleanup := m.Clear(); cleanup == "" {
		t.Fatal("local icon resource was not cleaned up")
	}
	for _, req := range []Request{{Icon: "unknown", Cols: 5, Rows: 2}, {Icon: "starred", URL: "https://example.test/image", Cols: 5, Rows: 2}} {
		if jobs, _ := m.Plan(t.Context(), []Request{req}); len(jobs) != 0 {
			t.Fatal("invalid local icon accepted")
		}
	}
}

func TestStatsStarIconsFillTheCellWithoutACircle(t *testing.T) {
	m := New("https://example.test")
	m.Enabled = true
	for kind, filled := range map[string]bool{"stats-star": false, "stats-starred": true} {
		jobs, _ := m.Plan(t.Context(), []Request{{Icon: kind, Ring: "#123abc", Cols: 2, Rows: 1}})
		if len(jobs) != 1 {
			t.Fatalf("no icon job for %s", kind)
		}
		result := m.Fetch(jobs[0])
		im := result.avatar
		if result.Err != nil || im == nil || im.NRGBAAt(0, 0).A != 0 || im.NRGBAAt(48, 90).A != 0 {
			t.Fatalf("%s is not a bare star: %v", kind, result.Err)
		}
		if im.NRGBAAt(48, 6).A == 0 || (im.NRGBAAt(48, 54).A != 0) != filled {
			t.Fatalf("%s has the wrong star shape", kind)
		}
	}
}

func TestFullLogosRenderLocallyAndKeepTheirAspectRatio(t *testing.T) {
	m := New("https://example.test")
	m.Enabled = true
	for _, kind := range []string{"logo-dark", "logo-light"} {
		req := Request{Icon: kind, Cols: 23, Rows: 2}
		jobs, _ := m.Plan(t.Context(), []Request{req})
		if len(jobs) != 1 {
			t.Fatal("logo must render locally")
		}
		result := m.Fetch(jobs[0])
		if result.Err != nil || !strings.Contains(m.Accept(result), "f=100") {
			t.Fatalf("local logo %s: %v", kind, result.Err)
		}
		im, err := prepareLogo(req)
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds().Dx() != 368 || im.Bounds().Dy() != 64 || im.NRGBAAt(0, 0).A != 0 {
			t.Fatal("full logo must fit a transparent cell-sized canvas")
		}
		ink := im.NRGBAAt(135, 30)
		if ink.A < 200 || (ink.R > 128) != (kind == "logo-dark") {
			t.Fatal("wordmark must use the correct theme asset")
		}
		if jobs[0].avatar != nil {
			t.Fatal("theme change must not reuse the old logo")
		}
	}
}

func TestPastelIconKeepsItsOwnColorsOnASquareCanvas(t *testing.T) {
	m := New("https://example.test")
	m.Enabled = true
	jobs, _ := m.Plan(t.Context(), []Request{{Icon: "pastel", Cols: 2, Rows: 1}})
	if len(jobs) != 1 {
		t.Fatal("pastel icon must render locally without a lane color")
	}
	result := m.Fetch(jobs[0])
	if result.Err != nil || !strings.Contains(m.Accept(result), "f=100") {
		t.Fatalf("pastel icon did not render: %v", result.Err)
	}
	im, err := prepareLogo(jobs[0].Request)
	if err != nil || im.Bounds().Dx() != 96 || im.Bounds().Dy() != 96 || im.NRGBAAt(0, 0).A != 0 {
		t.Fatalf("pastel icon must fill a transparent square canvas: %v", err)
	}
	if center := im.NRGBAAt(48, 40); center.A == 0 || center.R <= center.B {
		t.Fatalf("pastel icon lost its colors: %+v", center)
	}
}

func TestBadgeIconsTintTheWebIconsInTheRingColor(t *testing.T) {
	m := New("https://example.test")
	m.Enabled = true
	for _, name := range []string{"terminal", "file-code", "flame", "zap", "server", "box", "shield", "star", "sparkles", "git-merge", "key", "bot-off", "bomb", "dragon-ball", "mushroom-cloud", "award"} {
		jobs, _ := m.Plan(t.Context(), []Request{{Icon: "badge-" + name, Ring: "#123abc", Cols: 2, Rows: 1}})
		if len(jobs) != 1 {
			t.Fatalf("no icon job for %s", name)
		}
		result := m.Fetch(jobs[0])
		im := result.avatar
		if result.Err != nil || im == nil || im.Bounds().Dx() != 96 || im.NRGBAAt(0, 0).A != 0 {
			t.Fatalf("%s: %v", name, result.Err)
		}
		inked := 0
		for y := range 96 {
			for x := range 96 {
				if c := im.NRGBAAt(x, y); c.A > 0 {
					if c.R != 0x12 || c.G != 0x3a || c.B != 0xbc {
						t.Fatalf("%s pixel %d,%d is %v, not the ring color", name, x, y, c)
					}
					inked++
				}
			}
		}
		if inked < 300 {
			t.Fatalf("%s drew only %d pixels", name, inked)
		}
	}
	if jobs, _ := m.Plan(t.Context(), []Request{{Icon: "badge-unknown", Ring: "#123abc", Cols: 2, Rows: 1}}); len(jobs) != 0 {
		t.Fatal("an unknown badge icon was accepted")
	}
}
