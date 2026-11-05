package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

//go:embed INSTALL.md
var installNotes []byte

// Zip cannot store times before 1980, and a fixed time keeps rebuilt archives identical.
var archiveTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type target struct {
	goos, arch string
}

var targets = []target{
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"linux", "amd64"}, {"linux", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"},
}

func main() {
	version := flag.String("version", release.Version, "version embedded in every binary")
	assets := flag.Bool("release", false, "also write release binaries, archives, and checksums to dist/release")
	flag.Parse()
	if !release.ValidVersion(*version) || flag.NArg() != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: go run ./tools/build [-version vMAJOR.MINOR.PATCH] [-release]")
		os.Exit(1)
	}
	if err := build(*version, "dist", targets, *assets); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(version, dist string, targets []target, assets bool) error {
	var checksums strings.Builder
	for _, t := range targets {
		name := "devnull"
		if t.goos == "windows" {
			name += ".exe"
		}
		output := filepath.Join(dist, "devnull_"+t.goos+"_"+t.arch, name)
		if assets {
			asset, err := release.AssetName(t.goos, t.arch)
			if err != nil {
				return err
			}
			output = filepath.Join(dist, "release", asset)
		}
		// #nosec G204 -- Go and the target matrix are fixed, and the version is validated before building.
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X github.com/WendellAdriel/dev-null-cli/internal/cli.Version="+version, "-o", output, "./cmd/devnull")
		cmd.Env = append(os.Environ(), "GOOS="+t.goos, "GOARCH="+t.arch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s/%s: %w", t.goos, t.arch, err)
		}
		if _, err := fmt.Fprintln(os.Stdout, output); err != nil {
			return err
		}
		if !assets {
			continue
		}
		archive, err := packageBinary(version, t, output)
		if err != nil {
			return err
		}
		for _, path := range []string{output, archive} {
			line, err := checksumLine(path)
			if err != nil {
				return err
			}
			checksums.WriteString(line)
		}
	}
	if assets {
		return os.WriteFile(filepath.Join(dist, "release", "checksums.txt"), []byte(checksums.String()), 0600)
	}
	return nil
}

func packageBinary(version string, t target, binaryPath string) (string, error) {
	name, err := release.ArchiveName(version, t.goos, t.arch)
	if err != nil {
		return "", err
	}
	binary, err := os.ReadFile(binaryPath) // #nosec G304 -- The path comes from the fixed build target matrix.
	if err != nil {
		return "", err
	}
	archive := filepath.Join(filepath.Dir(binaryPath), name)
	if err := writeArchive(archive, t.goos, binary); err != nil {
		return "", fmt.Errorf("package %s/%s: %w", t.goos, t.arch, err)
	}
	if _, err := fmt.Fprintln(os.Stdout, archive); err != nil {
		return "", err
	}
	return archive, nil
}

type archiveEntry struct {
	name string
	mode os.FileMode
	data []byte
}

func writeArchive(path, goos string, binary []byte) error {
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	executable := "devnull"
	if goos == "windows" {
		executable += ".exe"
	}
	entries := []archiveEntry{
		{executable, 0755, binary},
		{"LICENSE", 0644, license},
		{"INSTALL.md", 0644, installNotes},
	}
	var data bytes.Buffer
	if goos == "windows" {
		err = writeZip(&data, entries)
	} else {
		err = writeTarGz(&data, entries)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data.Bytes(), 0600)
}

func writeTarGz(w io.Writer, entries []archiveEntry) error {
	compressed := gzip.NewWriter(w)
	archive := tar.NewWriter(compressed)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: int64(entry.mode), Size: int64(len(entry.data)), ModTime: archiveTime, Format: tar.FormatPAX}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if _, err := archive.Write(entry.data); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return compressed.Close()
}

func writeZip(w io.Writer, entries []archiveEntry) error {
	archive := zip.NewWriter(w)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate, Modified: archiveTime}
		header.SetMode(entry.mode)
		file, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := file.Write(entry.data); err != nil {
			return err
		}
	}
	return archive.Close()
}

func checksumLine(path string) (string, error) {
	file, err := os.Open(path) // #nosec G304 -- The path comes from the fixed build target matrix.
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return fmt.Sprintf("%x  %s\n", hash.Sum(nil), filepath.Base(path)), nil
}
