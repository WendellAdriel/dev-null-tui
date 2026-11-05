package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

func TestReleaseArchiveRunsHostBinaryAndMatchesChecksums(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the executable")
	}
	t.Chdir(filepath.Join("..", ".."))
	output := t.TempDir()
	host := target{runtime.GOOS, runtime.GOARCH}
	if err := build("v9.8.7", output, []target{host}, true); err != nil {
		t.Fatal(err)
	}
	archive, err := release.ArchiveName("v9.8.7", host.goos, host.arch)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := release.AssetName(host.goos, host.arch)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(output, "release")
	checksums := readChecksums(t, filepath.Join(dir, "checksums.txt"))
	if len(checksums) != 2 || checksums[archive] == "" || checksums[asset] == "" {
		t.Fatalf("checksums %v, want the archive and raw binary", checksums)
	}
	for name, sum := range checksums {
		data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- The name comes from the test's own checksums file.
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != sum {
			t.Fatalf("%s checksum %s, want %s", name, sum, got)
		}
	}

	executable := "devnull"
	if host.goos == "windows" {
		executable += ".exe"
	}
	files := readArchive(t, filepath.Join(dir, archive))
	if len(files) != 3 || files["LICENSE"].data == nil || files["INSTALL.md"].data == nil || files[executable].data == nil {
		t.Fatalf("archive holds %v, want %s, LICENSE, and INSTALL.md", keys(files), executable)
	}
	extracted := filepath.Join(t.TempDir(), executable)
	if err := os.WriteFile(extracted, files[executable].data, 0700); err != nil { // #nosec G306 -- The extracted test binary must be executable.
		t.Fatal(err)
	}
	version, err := exec.Command(extracted, "--version").Output() // #nosec G204 -- Runs the binary this test just built.
	if err != nil || !strings.Contains(string(version), "v9.8.7") {
		t.Fatalf("extracted --version printed %q: %v", version, err)
	}
	if help, err := exec.Command(extracted, "--help").Output(); err != nil || len(help) == 0 { // #nosec G204 -- Runs the binary this test just built.
		t.Fatalf("extracted --help printed %q: %v", help, err)
	}

	before, err := os.ReadFile(filepath.Join(dir, archive)) // #nosec G304 -- The path is inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if err := build("v9.8.7", output, []target{host}, true); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, archive)) // #nosec G304 -- The path is inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rebuilding the same revision produced a different archive")
	}
}

func TestArchiveEntriesUseTheTargetExecutableName(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	for _, goos := range []string{"darwin", "linux", "windows"} {
		name, err := release.ArchiveName("v9.8.7", goos, "amd64")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), name)
		if err := writeArchive(path, goos, []byte("binary")); err != nil {
			t.Fatal(err)
		}
		want := "devnull"
		if goos == "windows" {
			want += ".exe"
		}
		files := readArchive(t, path)
		if string(files[want].data) != "binary" {
			t.Fatalf("%s archive holds %v, want %s", goos, keys(files), want)
		}
		if mode := files[want].mode.Perm(); mode != 0755 {
			t.Fatalf("%s archive stores %s with mode %o, want 755", goos, want, mode)
		}
	}
}

func readChecksums(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path) // #nosec G304 -- The path is inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	sums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		sum, name, ok := strings.Cut(scanner.Text(), "  ")
		if !ok {
			t.Fatalf("malformed checksum line %q", scanner.Text())
		}
		sums[name] = sum
	}
	return sums
}

type archivedFile struct {
	data []byte
	mode os.FileMode
}

func readArchive(t *testing.T, path string) map[string]archivedFile {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- The path is inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]archivedFile{}
	if strings.HasSuffix(path, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range reader.File {
			content, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(content)
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name] = archivedFile{body, entry.Mode()}
		}
		return files
	}
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = archivedFile{body, header.FileInfo().Mode()}
	}
}

func keys(files map[string]archivedFile) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}
