package release

import (
	"strings"
	"testing"
)

func TestEverySummaryIsNamedForAValidVersion(t *testing.T) {
	versions := SummaryVersions()
	if len(versions) == 0 {
		t.Fatal("no release summaries embedded")
	}
	for _, version := range versions {
		body, ok := Summary(version)
		if !ValidVersion(version) || !ok || strings.TrimSpace(body) == "" {
			t.Fatalf("invalid summary for %q", version)
		}
	}
	if _, ok := Summary("v0.2.0"); !ok {
		t.Fatal("v0.2.0 summary missing")
	}
	if _, ok := Summary("v9.9.9"); ok {
		t.Fatal("unknown version reported a summary")
	}
}
