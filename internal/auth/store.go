package auth

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
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
