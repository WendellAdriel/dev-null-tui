package scripts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellInstaller(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows uses install.ps1")
	}
	source, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"linux", "macos", "checksum mismatch", "duplicate checksum", "missing checksum", "download failure", "invalid version", "unsupported platform"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			mockDir := filepath.Join(root, "mocks")
			installDir := filepath.Join(root, "install with spaces")
			for _, dir := range []string{mockDir, installDir} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			binary := []byte("new executable\n")
			write := func(path, content string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			asset := "devnull_linux_amd64"
			platform, arch := "Linux", "x86_64"
			if scenario == "macos" {
				platform, arch, asset = "Darwin", "arm64", "devnull_darwin_arm64"
			}
			if scenario == "unsupported platform" {
				platform = "FreeBSD"
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(binary))
			if scenario == "checksum mismatch" {
				digest = strings.Repeat("0", 64)
			}
			manifest := digest + "  " + asset + "\n"
			if scenario == "duplicate checksum" {
				manifest += manifest
			}
			if scenario == "missing checksum" {
				manifest = digest + "  other\n"
			}
			write(filepath.Join(root, "checksums.txt"), manifest, 0600)
			write(filepath.Join(root, "binary"), string(binary), 0600)
			write(filepath.Join(installDir, "devnull"), "old executable", 0755)
			write(filepath.Join(mockDir, "uname"), "#!/bin/sh\ncase $1 in -s) echo \"$TEST_PLATFORM\";; -m) echo \"$TEST_ARCH\";; esac\n", 0755)
			write(filepath.Join(mockDir, "curl"), `#!/bin/sh
out=
for arg do
    if [ "$out" = next ]; then out=$arg; continue; fi
    if [ "$arg" = --output ]; then out=next; fi
    url=$arg
done
case "$url" in
    */releases/latest) echo 'https://github.com/WendellAdriel/dev-null-tui/releases/tag/v0.1.0';;
    */checksums.txt) cp "$TEST_ROOT/checksums.txt" "$out";;
    */devnull_*)
        if [ "$TEST_SCENARIO" = 'download failure' ]; then exit 22; fi
        cp "$TEST_ROOT/binary" "$out";;
    *) exit 99;;
esac
`, 0755)
			args := []string{"-s", "--"}
			if scenario == "invalid version" {
				args = append(args, "v1/../../bad")
			}
			cmd := exec.Command("sh", args...)
			cmd.Stdin = strings.NewReader(string(source))
			cmd.Env = append(os.Environ(), "PATH="+mockDir+string(os.PathListSeparator)+os.Getenv("PATH"), "INSTALL_DIR="+installDir,
				"TEST_ROOT="+root, "TEST_PLATFORM="+platform, "TEST_ARCH="+arch, "TEST_SCENARIO="+scenario)
			output, err := cmd.CombinedOutput()
			got, readErr := os.ReadFile(filepath.Join(installDir, "devnull")) // #nosec G304 -- This path is a fixture inside t.TempDir.
			if readErr != nil {
				t.Fatal(readErr)
			}
			if scenario == "linux" || scenario == "macos" {
				if err != nil || string(got) != string(binary) {
					t.Fatalf("install failed: %v %s binary=%q", err, output, got)
				}
			} else if err == nil || string(got) != "old executable" {
				t.Fatalf("unsafe failed install: %v %s binary=%q", err, output, got)
			}
			entries, err := os.ReadDir(installDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("installer left temporary files: %v %v", entries, err)
			}
		})
	}
}

func TestPowerShellInstaller(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell installer requires native Windows")
	}
	powershell, err := exec.LookPath("pwsh")
	if err != nil {
		powershell, err = exec.LookPath("powershell")
	}
	if err != nil {
		t.Skip("PowerShell is not installed")
	}
	for _, scenario := range []string{"success", "checksum mismatch", "duplicate checksum", "download failure"} {
		t.Run(scenario, func(t *testing.T) {
			// #nosec G204 -- PowerShell comes from LookPath and all arguments are local test fixtures.
			cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-File", "test-install.ps1", "-WorkDir", t.TempDir(), "-Scenario", scenario)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("PowerShell installer: %v\n%s", err, output)
			}
		})
	}
}
