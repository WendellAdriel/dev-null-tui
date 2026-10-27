package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

func main() {
	version := flag.String("version", release.Version, "version embedded in every binary")
	assets := flag.Bool("release", false, "also write release assets and checksums to dist/release")
	flag.Parse()
	if !release.ValidVersion(*version) || flag.NArg() != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: go run ./tools/build [-version vMAJOR.MINOR.PATCH] [-release]")
		os.Exit(1)
	}
	if err := buildAll(*version, *assets); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildAll(version string, assets bool) error {
	var checksums strings.Builder
	for _, targetOS := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "devnull"
			if targetOS == "windows" {
				name += ".exe"
			}
			output := filepath.Join("dist", "devnull_"+targetOS+"_"+arch, name)
			if assets {
				asset, err := release.AssetName(targetOS, arch)
				if err != nil {
					return err
				}
				output = filepath.Join("dist", "release", asset)
			}
			// #nosec G204 -- Go and the target matrix are fixed, and the version is validated before building.
			cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X github.com/WendellAdriel/dev-null-cli/internal/cli.Version="+version, "-o", output, "./cmd/devnull")
			cmd.Env = append(os.Environ(), "GOOS="+targetOS, "GOARCH="+arch, "CGO_ENABLED=0")
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("build %s/%s: %w", targetOS, arch, err)
			}
			if _, err := fmt.Fprintln(os.Stdout, output); err != nil {
				return err
			}
			if assets {
				// #nosec G304 -- The output path comes from the fixed build target matrix.
				file, err := os.Open(output)
				if err != nil {
					return err
				}
				hash := sha256.New()
				_, copyErr := io.Copy(hash, file)
				closeErr := file.Close()
				if copyErr != nil {
					return copyErr
				}
				if closeErr != nil {
					return closeErr
				}
				_, _ = fmt.Fprintf(&checksums, "%x  %s\n", hash.Sum(nil), filepath.Base(output))
			}
		}
	}
	if assets {
		return os.WriteFile(filepath.Join("dist", "release", "checksums.txt"), []byte(checksums.String()), 0600)
	}
	return nil
}
