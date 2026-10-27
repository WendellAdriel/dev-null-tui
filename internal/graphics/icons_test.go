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
