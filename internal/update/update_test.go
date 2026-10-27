package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateVerifiesDownloadBeforeReplacingExecutable(t *testing.T) {
	for _, failure := range []string{"", "checksum mismatch", "missing checksum", "duplicate checksum", "download failure", "invalid version", "prerelease", "missing asset"} {
		t.Run(failure, func(t *testing.T) {
			binary := []byte("new executable")
			asset := "devnull_linux_amd64"
			digest := fmt.Sprintf("%x", sha256.Sum256(binary))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/latest":
					tag, prerelease, name := "v0.2.0", false, asset
					if failure == "invalid version" {
						tag = "../../bad"
					}
					if failure == "prerelease" {
						prerelease = true
					}
					if failure == "missing asset" {
						name = "other"
					}
					_, _ = fmt.Fprintf(w, `{"tag_name":%q,"prerelease":%t,"assets":[{"name":%q},{"name":"checksums.txt"}]}`, tag, prerelease, name)
				case "/download/v0.2.0/checksums.txt":
					if failure == "missing checksum" {
						_, _ = fmt.Fprintln(w, digest+"  other")
						return
					}
					if failure == "checksum mismatch" {
						digest = strings.Repeat("0", 64)
					}
					_, _ = fmt.Fprintln(w, digest+"  "+asset)
					if failure == "duplicate checksum" {
						_, _ = fmt.Fprintln(w, digest+"  "+asset)
					}
				case "/download/v0.2.0/" + asset:
					if failure == "download failure" {
						http.Error(w, "failed", http.StatusBadGateway)
						return
					}
					_, _ = w.Write(binary)
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "devnull")
			// #nosec G306 -- The fixture needs executable permissions to verify they survive replacement.
			if err := os.WriteFile(path, []byte("original executable"), 0700); err != nil {
				t.Fatal(err)
			}
			u := updater{client: server.Client(), latestURL: server.URL + "/latest", downloadBase: server.URL + "/download/", executable: path, goos: "linux", arch: "amd64"}
			var output bytes.Buffer
			err := u.run(context.Background(), "v0.1.0", &output)
			got, readErr := os.ReadFile(path) // #nosec G304 -- This path is a fixture inside t.TempDir.
			if readErr != nil {
				t.Fatal(readErr)
			}
			if failure != "" {
				if err == nil || string(got) != "original executable" {
					t.Fatalf("failed update changed executable: error=%v binary=%q", err, got)
				}
			} else if err != nil || !bytes.Equal(got, binary) || !strings.Contains(output.String(), "v0.2.0") {
				t.Fatalf("update: error=%v binary=%q output=%q", err, got, &output)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			wantEntries := 1
			if runtime.GOOS == "windows" && failure == "" {
				wantEntries = 2
			}
			if err != nil || len(entries) != wantEntries {
				t.Fatalf("left temporary downloads: %v %v", entries, err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
				t.Fatalf("executable permissions changed: %v", info.Mode())
			}
		})
	}
}

func TestUpdateDoesNotDowngradeOrDownloadCurrentVersion(t *testing.T) {
	for _, current := range []string{"v0.2.0", "v0.10.0", "v1.0.0", "dev"} {
		t.Run(current, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/latest" {
					t.Errorf("unnecessary download %s", r.URL)
				}
				_, _ = fmt.Fprint(w, `{"tag_name":"v0.2.0"}`)
			}))
			defer server.Close()
			u := updater{client: server.Client(), latestURL: server.URL + "/latest", goos: "linux", arch: "amd64"}
			err := u.run(context.Background(), current, &bytes.Buffer{})
			if (current == "dev") != (err != nil) {
				t.Fatalf("current=%s error=%v", current, err)
			}
		})
	}
}

func TestUpdateRejectsUnavailableReleaseAndUnsupportedPlatform(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	for _, goos := range []string{"linux", "freebsd"} {
		u := updater{client: server.Client(), latestURL: server.URL, goos: goos, arch: "amd64"}
		if err := u.run(context.Background(), "v0.1.0", &bytes.Buffer{}); err == nil {
			t.Fatal("expected update error")
		}
	}
}

func TestReleaseResponseLimitsAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, "12345") }))
	defer server.Close()
	u := updater{client: server.Client()}
	if _, err := u.get(context.Background(), server.URL, 4); err == nil {
		t.Fatal("accepted oversized release response")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := u.get(ctx, server.URL, 100); err == nil {
		t.Fatal("ignored update cancellation")
	}
}

func TestReplaceRunningExecutable(t *testing.T) {
	if os.Getenv("DEVNULL_REPLACE_HELPER") == "1" {
		destination, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		staged, err := os.CreateTemp(filepath.Dir(destination), ".replacement-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := staged.WriteString("replacement"); err != nil {
			t.Fatal(err)
		}
		if err := staged.Close(); err != nil {
			t.Fatal(err)
		}
		if err := replace(staged.Name(), destination); err != nil {
			t.Fatal(err)
		}
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(self) // #nosec G304 -- The source is this test process's executable.
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "update-test.exe")
	// #nosec G306,G703 -- This executable copy stays inside t.TempDir to test running-binary replacement.
	if err := os.WriteFile(path, binary, 0700); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- This executes an isolated copy of the current test binary.
	cmd := exec.Command(path, "-test.run=^TestReplaceRunningExecutable$")
	cmd.Env = append(os.Environ(), "DEVNULL_REPLACE_HELPER=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running executable replacement: %v\n%s", err, output)
	}
	got, err := os.ReadFile(path) // #nosec G304 -- The path is the isolated child fixture in t.TempDir.
	if err != nil || string(got) != "replacement" {
		t.Fatalf("running executable was not replaced: %v %q", err, got)
	}
}
