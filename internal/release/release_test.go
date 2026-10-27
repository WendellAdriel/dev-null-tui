package release

import "testing"

func TestStableVersionsAndNumericOrdering(t *testing.T) {
	for _, version := range []string{"v0.1.0", "v1.10.2", "v999999999999999999999999.0.0"} {
		if !ValidVersion(version) {
			t.Fatalf("rejected stable version %q", version)
		}
	}
	for _, version := range []string{"dev", "0.1.0", "v01.0.0", "v1.0.0-beta", "v1.0.0/../../bad"} {
		if ValidVersion(version) {
			t.Fatalf("accepted invalid version %q", version)
		}
	}
	for _, pair := range [][2]string{{"v0.2.0", "v0.10.0"}, {"v1.9.99", "v2.0.0"}, {"v0.1.2", "v0.1.10"}} {
		if Compare(pair[0], pair[1]) >= 0 || Compare(pair[1], pair[0]) <= 0 || Compare(pair[0], pair[0]) != 0 {
			t.Fatalf("incorrect version order %v", pair)
		}
	}
}
