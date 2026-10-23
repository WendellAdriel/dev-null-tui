package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	if err := buildAll(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildAll() error {
	for _, targetOS := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "devnull"
			if targetOS == "windows" {
				name += ".exe"
			}
			output := filepath.Join("dist", "devnull_"+targetOS+"_"+arch, name)
			// #nosec G204 -- The executable and target matrix are fixed, with no user input.
			cmd := exec.Command("go", "build", "-trimpath", "-o", output, "./cmd/devnull")
			cmd.Env = append(os.Environ(), "GOOS="+targetOS, "GOARCH="+arch, "CGO_ENABLED=0")
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("build %s/%s: %w", targetOS, arch, err)
			}
			if _, err := fmt.Fprintln(os.Stdout, output); err != nil {
				return err
			}
		}
	}
	return nil
}
