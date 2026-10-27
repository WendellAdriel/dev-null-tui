package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/release"
)

const maxBinarySize = 128 << 20

type updater struct {
	client       *http.Client
	latestURL    string
	downloadBase string
	executable   string
	goos         string
	arch         string
}

func Run(ctx context.Context, current string, output io.Writer) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	u := updater{
		client: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("update redirect must use HTTPS")
			}
			if len(via) >= 10 {
				return errors.New("too many update redirects")
			}
			return nil
		}},
		latestURL:    "https://api.github.com/repos/" + release.Repository + "/releases/latest",
		downloadBase: "https://github.com/" + release.Repository + "/releases/download/",
		executable:   path, goos: runtime.GOOS, arch: runtime.GOARCH,
	}
	return u.run(ctx, current, output)
}

func (u updater) run(ctx context.Context, current string, output io.Writer) error {
	if !release.ValidVersion(current) {
		return errors.New("cannot self-update a development build; install a published release first")
	}
	asset, err := release.AssetName(u.goos, u.arch)
	if err != nil {
		return err
	}
	metadata, err := u.get(ctx, u.latestURL, 1<<20)
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	var latest struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(metadata, &latest); err != nil {
		return fmt.Errorf("read release: %w", err)
	}
	if latest.Draft || latest.Prerelease || !release.ValidVersion(latest.Tag) {
		return errors.New("latest release is not a stable vMAJOR.MINOR.PATCH version")
	}
	if release.Compare(current, latest.Tag) >= 0 {
		_, err := fmt.Fprintln(output, "Already up to date ("+current+").")
		return err
	}
	hasBinary, hasChecksums := false, false
	for _, item := range latest.Assets {
		hasBinary = hasBinary || item.Name == asset
		hasChecksums = hasChecksums || item.Name == "checksums.txt"
	}
	if !hasBinary || !hasChecksums {
		return errors.New("release is missing the platform binary or checksums.txt")
	}
	base := u.downloadBase + latest.Tag + "/"
	checksums, err := u.get(ctx, base+"checksums.txt", 64<<10)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	want, err := checksum(checksums, asset)
	if err != nil {
		return err
	}
	if err := u.install(ctx, base+asset, want); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, "Updated devnull to "+latest.Tag+". Start devnull again to use it.")
	return err
}

func checksum(manifest []byte, asset string) (string, error) {
	want := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != asset {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size || want != "" {
			return "", errors.New("invalid or duplicate release checksum")
		}
		want = strings.ToLower(fields[0])
	}
	if want == "" {
		return "", errors.New("release checksum is missing for " + asset)
	}
	return want, nil
}

func (u updater) request(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "devnull-updater")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GitHub returned HTTP %d (check release availability or API rate limits)", resp.StatusCode)
	}
	return resp, nil
}

func (u updater) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := u.request(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("release response exceeds size limit")
	}
	return body, nil
}

func (u updater) install(ctx context.Context, url, want string) error {
	original, err := os.Stat(u.executable)
	if err != nil {
		return err
	}
	if !original.Mode().IsRegular() {
		return errors.New("installed executable is not a regular file")
	}
	tmp, err := os.CreateTemp(filepath.Dir(u.executable), ".devnull-update-*")
	if err != nil {
		return fmt.Errorf("create update file (install directory must be writable): %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := u.download(ctx, url, tmp, want); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(original.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	now, err := os.Stat(u.executable)
	if err != nil {
		return err
	}
	if !os.SameFile(original, now) {
		return errors.New("installed executable changed during the update; retry")
	}
	if err := replace(tmp.Name(), u.executable); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}

func (u updater) download(ctx context.Context, url string, file *os.File, want string) error {
	resp, err := u.request(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	if n == 0 || n > maxBinarySize {
		return errors.New("release binary is empty or exceeds 128 MiB")
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return errors.New("release binary checksum mismatch; installed binary was not changed")
	}
	return nil
}

func replace(staged, destination string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(staged, destination)
	}
	backup := destination + ".old"
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, destination); err != nil {
		return errors.Join(err, os.Rename(backup, destination))
	}
	return nil
}
