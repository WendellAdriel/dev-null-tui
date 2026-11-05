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

func TestArchiveNamesIdentifyVersionAndTarget(t *testing.T) {
	seen := map[string]bool{}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name, err := ArchiveName("v1.2.3", goos, arch)
			if err != nil {
				t.Fatal(err)
			}
			extension := ".tar.gz"
			if goos == "windows" {
				extension = ".zip"
			}
			if want := "devnull_v1.2.3_" + goos + "_" + arch + extension; name != want {
				t.Fatalf("archive name %q, want %q", name, want)
			}
			asset, err := AssetName(goos, arch)
			if err != nil {
				t.Fatal(err)
			}
			if seen[name] || seen[asset] {
				t.Fatalf("duplicate release file name for %s/%s", goos, arch)
			}
			seen[name], seen[asset] = true, true
		}
	}
	if _, err := ArchiveName("dev", "linux", "amd64"); err == nil {
		t.Fatal("accepted an unstable archive version")
	}
	if _, err := ArchiveName("v1.2.3", "freebsd", "amd64"); err == nil {
		t.Fatal("accepted an unsupported archive target")
	}
}
