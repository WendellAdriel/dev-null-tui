package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type callbackAddress struct {
	Port   int
	Secret string
}

type Callback struct {
	Links  chan string
	server *http.Server
	path   string
	once   sync.Once
}

func validState(state string) bool {
	decoded, err := hex.DecodeString(state)
	return err == nil && len(decoded) == 16 && state == strings.ToLower(state)
}

func callbackState(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || strings.Contains(raw, "#") || u.Scheme != "devnull" || u.Host != "auth" || u.Path != "/callback" || u.RawPath != "" || u.Fragment != "" || u.User != nil {
		return "", errors.New("invalid callback link")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["state"]) != 1 || !validState(q.Get("state")) {
		return "", errors.New("invalid callback state")
	}
	return q.Get("state"), nil
}

func ListenCallback(store Store, state string) (*Callback, error) {
	if !validState(state) {
		return nil, errors.New("invalid attempt state")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		_ = listener.Close()
		return nil, err
	}
	address := callbackAddress{Port: listener.Addr().(*net.TCPAddr).Port, Secret: hex.EncodeToString(secret)}
	// This private attempt file authorizes local callback delivery and is removed at cleanup.
	data, err := json.Marshal(address) // #nosec G117
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	path := filepath.Join(store.Dir, state+".attempt")
	if err := writePrivate(path, data); err != nil {
		_ = listener.Close()
		return nil, err
	}
	callback := &Callback{Links: make(chan string, 1), path: path}
	var mu sync.Mutex
	delivered := false
	callback.server = &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, MaxHeaderBytes: 4096, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "127.0.0.1:"+strconv.Itoa(address.Port) || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+address.Secret)) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/alive" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/callback" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, 2049))
		if err != nil || len(data) > 2048 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		got, err := callbackState(string(data))
		if err != nil || got != state {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if delivered {
			w.WriteHeader(http.StatusConflict)
			return
		}
		delivered = true
		callback.Links <- string(data)
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = callback.server.Serve(listener) }()
	return callback, nil
}

func (c *Callback) Close() {
	c.once.Do(func() { _ = c.server.Close(); _ = os.Remove(c.path) })
}

func readCallbackAddress(store Store, state string) (callbackAddress, error) {
	if !validState(state) {
		return callbackAddress{}, errors.New("invalid attempt state")
	}
	path := filepath.Join(store.Dir, state+".attempt")
	if err := protectPath(path, false); err != nil {
		return callbackAddress{}, errors.New("no active private connection attempt")
	}
	root, err := os.OpenRoot(store.Dir)
	if err != nil {
		return callbackAddress{}, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(state + ".attempt")
	if err != nil {
		return callbackAddress{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(data) > 1024 {
		return callbackAddress{}, errors.New("cannot read active attempt")
	}
	var address callbackAddress
	if err := json.Unmarshal(data, &address); err != nil || address.Port < 1 || address.Port > 65535 || len(address.Secret) != 64 {
		return callbackAddress{}, errors.New("invalid active attempt")
	}
	return address, nil
}

func callbackRequest(ctx context.Context, address callbackAddress, method, path, body string) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:"+strconv.Itoa(address.Port)+path, strings.NewReader(body))
	if err != nil {
		return errors.New("cannot deliver callback")
	}
	req.Header.Set("Authorization", "Bearer "+address.Secret)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return errors.New("connection attempt is no longer listening")
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errors.New("callback rejected or already received")
	}
	return nil
}

func DeliverCallback(ctx context.Context, store Store, raw string) error {
	state, err := callbackState(raw)
	if err != nil {
		return err
	}
	address, err := readCallbackAddress(store, state)
	if err != nil {
		return err
	}
	return callbackRequest(ctx, address, http.MethodPost, "/callback", raw)
}
