package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleasePublicationRequiresExplicitApprovalAndCleanPushedMain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release.sh requires a POSIX shell")
	}
	source, err := os.ReadFile("release.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"prepare", "publish", "dirty", "wrong remote", "unpushed", "existing tag", "remote tag", "existing release", "changed during build", "declined"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			scriptsDir, mockDir := filepath.Join(root, "scripts"), filepath.Join(root, "mocks")
			for _, dir := range []string{scriptsDir, mockDir} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			// #nosec G703 -- This copies the maintained script into an isolated temporary fixture.
			if err := os.WriteFile(filepath.Join(scriptsDir, "release.sh"), source, 0600); err != nil {
				t.Fatal(err)
			}
			mock := `#!/bin/sh
name=${0##*/}
printf '%s %s\n' "$name" "$*" >> "$TEST_ROOT/calls"
if [ "$name" = go ]; then touch "$TEST_ROOT/built"; exit 0; fi
if [ "$name" = gh ]; then
    case "$*" in 'release view '*)
        if [ "$TEST_SCENARIO" = 'existing release' ]; then exit 0; else exit 1; fi;;
    esac
    exit 0
fi
case "$*" in
    'status --porcelain')
        if [ "$TEST_SCENARIO" = dirty ] || { [ "$TEST_SCENARIO" = 'changed during build' ] && [ -f "$TEST_ROOT/built" ]; }; then echo ' M README.md'; fi;;
    'branch --show-current') echo main;;
    'remote get-url origin')
        if [ "$TEST_SCENARIO" = 'wrong remote' ]; then echo https://github.com/other/repo.git
        else echo https://github.com/WendellAdriel/dev-null-tui.git; fi;;
    'ls-remote --exit-code origin refs/heads/main') echo 'abc refs/heads/main';;
    'rev-parse HEAD')
        if [ "$TEST_SCENARIO" = unpushed ]; then echo def; else echo abc; fi;;
    'show-ref --verify --quiet refs/tags/'*)
        if [ "$TEST_SCENARIO" = 'existing tag' ]; then exit 0; else exit 1; fi;;
    'ls-remote origin refs/tags/'*)
        if [ "$TEST_SCENARIO" = 'remote tag' ]; then echo 'abc refs/tags/v0.1.0'; fi;;
    'tag -a '*|'push origin refs/tags/'*) exit 0;;
    *) exit 99;;
esac
`
			for _, name := range []string{"git", "gh", "go"} {
				path := filepath.Join(mockDir, name)
				if err := os.WriteFile(path, []byte(mock), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0700); err != nil { // #nosec G302 -- The temporary command stub must be executable.
					t.Fatal(err)
				}
			}
			args := []string{filepath.Join(scriptsDir, "release.sh"), "v0.1.0"}
			if scenario != "prepare" {
				args = append(args, "--publish")
			}
			// #nosec G204 -- Only the isolated release-script fixture and fixed test arguments are executed.
			cmd := exec.Command("sh", args...)
			cmd.Env = append(os.Environ(), "PATH="+mockDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_ROOT="+root, "TEST_SCENARIO="+scenario)
			confirmation := "v0.1.0\n"
			if scenario == "declined" {
				confirmation = "no\n"
			}
			cmd.Stdin = strings.NewReader(confirmation)
			output, err := cmd.CombinedOutput()
			calls, readErr := os.ReadFile(filepath.Join(root, "calls")) // #nosec G304 -- This log is inside t.TempDir.
			if readErr != nil {
				t.Fatal(readErr)
			}
			published := strings.Contains(string(calls), "git tag -a") || strings.Contains(string(calls), "git push") || strings.Contains(string(calls), "gh release create")
			if scenario == "publish" {
				if err != nil || !published || !strings.Contains(string(calls), "--draft=false --latest") {
					t.Fatalf("mock publication failed: %v %s\n%s", err, output, calls)
				}
			} else if published {
				t.Fatalf("published without approval: %s", calls)
			} else if scenario == "prepare" {
				if err != nil || strings.Contains(string(calls), "git ") || strings.Contains(string(calls), "gh ") {
					t.Fatalf("preparation touched GitHub or Git: %v %s\n%s", err, output, calls)
				}
			} else if err == nil {
				t.Fatalf("accepted unsafe release state %q: %s", scenario, output)
			}
		})
	}
}
