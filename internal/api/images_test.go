package api

import (
	"bytes"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadImageChecksContentSizeAndDimensions(t *testing.T) {
	dir := t.TempDir()
	good := writePNG(t, dir, "renamed.jpg", 40, 20)
	file, err := ReadImage(good)
	if err != nil || file.Type != "image/png" || file.Name != "renamed.jpg" || file.Width != 40 || file.Height != 20 {
		t.Fatalf("valid image: %+v %v", file, err)
	}
	if _, err := ReadImage(writePNG(t, dir, "wide.png", 6001, 1)); err == nil || !strings.Contains(err.Error(), "6000") {
		t.Fatalf("wide image: %v", err)
	}
	svg := filepath.Join(dir, "logo.png")
	_ = os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0o600)
	if _, err := ReadImage(svg); err == nil || !strings.Contains(err.Error(), "JPEG, PNG, or WebP") {
		t.Fatalf("svg: %v", err)
	}
	big := filepath.Join(dir, "big.png")
	f, _ := os.Create(filepath.Clean(big))
	_ = f.Truncate(MaxImageBytes + 1)
	_ = f.Close()
	if _, err := ReadImage(big); err == nil || !strings.Contains(err.Error(), "10 MB") {
		t.Fatalf("big: %v", err)
	}
	if _, err := ReadImage(dir); err == nil {
		t.Fatal("accepted a directory")
	}
	if _, err := ReadImage(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("accepted a missing file")
	}
}

func TestDroppedPathsParsesQuotedAndEscapedImagePaths(t *testing.T) {
	dir := t.TempDir()
	spaced := writePNG(t, dir, "my shot.png", 1, 1)
	other := writePNG(t, dir, "b.png", 1, 1)
	cases := map[string][]string{
		"'" + spaced + "'":             {spaced},
		fileURL(spaced):                {spaced},
		"\"" + spaced + "\"\n" + other: {spaced, other},
	}
	if runtime.GOOS != "windows" {
		cases[strings.ReplaceAll(spaced, " ", `\ `)+" "+other] = []string{spaced, other}
	}
	for pasted, want := range cases {
		got, ok := DroppedPaths(pasted)
		if !ok || strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: %v %v", pasted, got, ok)
		}
	}
	for _, text := range []string{"hello world", other + " and more", filepath.Join(dir, "missing.png"), ""} {
		if _, ok := DroppedPaths(text); ok {
			t.Errorf("%q read as paths", text)
		}
	}
	t.Chdir(dir)
	if _, ok := DroppedPaths("b.png"); ok {
		t.Error("a relative file name was read as a dropped path")
	}
}

func fileURL(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}

func TestFileURLPathDropsTheSlashBeforeAWindowsDrive(t *testing.T) {
	cases := []struct{ goos, path, want string }{
		{"windows", "/C:/Users/ada/my shot.png", "C:/Users/ada/my shot.png"},
		{"windows", "/server/share.png", "/server/share.png"},
		{"linux", "/C:/odd.png", "/C:/odd.png"},
		{"darwin", "/Users/ada/b.png", "/Users/ada/b.png"},
	}
	for _, c := range cases {
		if got := fileURLPath(c.path, c.goos); got != filepath.FromSlash(c.want) {
			t.Errorf("%s %q: got %q", c.goos, c.path, got)
		}
	}
}
