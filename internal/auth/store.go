package auth

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct{ Dir string }

func (s Store) Lock() (*os.File, error) {
	path := filepath.Join(s.Dir, "session.lock")
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile("session.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := protectPath(path, false); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, errors.New("another dev-null-cli account session is running")
	}
	return f, nil
}

func DefaultStore() (Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Store{}, errors.New("cannot locate your user configuration directory")
	}
	return Store{Dir: filepath.Join(dir, "dev-null-cli")}, nil
}

func (s Store) Prepare() error {
	if err := os.MkdirAll(filepath.Dir(s.Dir), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(s.Dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	return protectPath(s.Dir, true)
}

func (s Store) Load() (*Key, string, error) {
	path := filepath.Join(s.Dir, "connection.json")
	if err := protectPath(path, false); err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open("connection.json")
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) > 16384 {
		return nil, "", errors.New("cannot read local connection")
	}
	var record struct {
		Origin     string `json:"origin"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, "", errors.New("local connection is corrupt; disconnect locally before reconnecting")
	}
	if _, err := parseOrigin(record.Origin); err != nil {
		return nil, "", err
	}
	key, err := parseKey([]byte(record.PrivateKey))
	return key, record.Origin, err
}

func (s Store) Save(key *Key, origin string) error {
	if key == nil {
		return errors.New("no device key to save")
	}
	if _, err := parseOrigin(origin); err != nil {
		return err
	}
	data, err := key.encode()
	if err != nil {
		return err
	}
	// The approved local-file policy persists the key only inside protected user storage.
	record, err := json.Marshal(struct { // #nosec G117
		Origin     string `json:"origin"`
		PrivateKey string `json:"private_key"`
	}{origin, string(data)})
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(s.Dir, "connection.json"), record)
}

func (s Store) Disconnect() error {
	if err := os.Remove(filepath.Join(s.Dir, "connection.json")); err != nil && !os.IsNotExist(err) {
		return errors.New("cannot delete the local device key")
	}
	return nil
}

func writePrivate(path string, data []byte) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(filepath.Base(path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := protectPath(path, false); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

const seenVersionFile = "last-seen-version"

func (s Store) HasConnection() bool {
	_, err := os.Lstat(filepath.Join(s.Dir, "connection.json"))
	return err == nil
}

// LastSeenVersion returns the version whose What's new was last closed, or "" when it is missing or unreadable.
func (s Store) LastSeenVersion() string {
	data, ok := s.readState(seenVersionFile, 64)
	if !ok {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SaveSeenVersion replaces the recorded version through a private temporary file, so a failed write keeps the old one.
func (s Store) SaveSeenVersion(version string) error {
	return s.replaceState(seenVersionFile, []byte(version+"\n"))
}

const tuiSettingsFile = "tui.json"

// TUISettings holds local preferences for this client. They stay on this device and survive disconnecting.
type TUISettings struct {
	CheckForUpdates bool      `json:"check_for_updates"`
	LastUpdateCheck time.Time `json:"last_update_check,omitzero"`
	SkippedVersion  string    `json:"skipped_version,omitempty"`
}

// TUISettings reads tui.json, falling back to the defaults when it is missing, oversized, or invalid.
func (s Store) TUISettings() TUISettings {
	defaults := TUISettings{CheckForUpdates: true}
	data, ok := s.readState(tuiSettingsFile, 4096)
	if !ok {
		return defaults
	}
	settings := defaults
	if json.Unmarshal(data, &settings) != nil {
		return defaults
	}
	return settings
}

func (s Store) SaveTUISettings(settings TUISettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.replaceState(tuiSettingsFile, append(data, '\n'))
}

func (s Store) readState(name string, limit int64) ([]byte, bool) {
	if protectPath(filepath.Join(s.Dir, name), false) != nil {
		return nil, false
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, false
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(name)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false
	}
	return data, true
}

func (s Store) replaceState(name string, data []byte) error {
	path, temporary := filepath.Join(s.Dir, name), filepath.Join(s.Dir, name+".tmp")
	_ = os.Remove(temporary)
	if err := writePrivate(temporary, data); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
