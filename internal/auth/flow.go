package auth

import (
	"context"
	"errors"
	"os"
	"sync"
)

type Flow struct {
	Attempt                     *Attempt
	Key                         *Key
	Notice                      string
	store                       Store
	origin                      string
	callback                    *Callback
	handler                     *Handler
	mu                          sync.Mutex
	submitted, savedNew, closed bool
}

func Begin(ctx context.Context, store Store, origin string, key *Key) (_ *Flow, err error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f := &Flow{store: store, origin: origin, Key: key}
	defer func() {
		if err != nil {
			_ = f.Close()
		}
	}()
	if key == nil {
		f.Key, err = GenerateKey()
		if err != nil {
			return nil, err
		}
		if err = store.Save(f.Key, origin); err != nil {
			return nil, errors.New("cannot save the private device key; no approval was started")
		}
		f.savedNew = true
	}
	f.Attempt, err = NewAttempt(origin, f.Key.Fingerprint(), "dev-null-cli")
	if err != nil {
		return nil, err
	}
	// The browser returns to a loopback listener (RFC 8252, section 7.3) instead of the devnull:// scheme, which
	// another app, such as /dev/null for Mac, may own.
	f.callback, err = ListenCallback(store, f.Attempt.State)
	if err == nil {
		err = f.Attempt.returnTo(f.callback.URI)
	}
	if err != nil {
		f.Notice = "Automatic return unavailable. Paste the full callback link."
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return f, nil
}

func (f *Flow) Wait(ctx context.Context) (string, error) {
	if f.callback == nil {
		<-ctx.Done()
		return "", ctx.Err()
	}
	select {
	case link := <-f.callback.Links:
		return link, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (f *Flow) Redeem(ctx context.Context, raw string) (User, error) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return User{}, errors.New("connection attempt ended")
	}
	code, verifier, err := f.Attempt.grant(raw)
	if err != nil {
		f.mu.Unlock()
		return User{}, err
	}
	f.submitted = true
	f.mu.Unlock()
	client, err := NewClient(f.origin, f.Key)
	if err != nil {
		return User{}, err
	}
	enrolled, err := client.Enroll(ctx, code, verifier)
	if err != nil {
		return User{}, err
	}
	user, err := client.Me(ctx)
	if err != nil {
		return User{}, err
	}
	if user.ID != enrolled.ID {
		return User{}, errors.New("account identity changed during connection; disconnect and try again")
	}
	return user, nil
}

func (f *Flow) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	if f.Attempt != nil {
		f.Attempt.Cancel()
	}
	var cleanup error
	if f.handler != nil {
		cleanup = f.handler.Close()
	}
	if f.callback != nil {
		f.callback.Close()
	}
	if f.savedNew && !f.submitted {
		cleanup = errors.Join(cleanup, f.store.Disconnect())
	}
	return cleanup
}

func Restore(ctx context.Context, store Store, expectedOrigin string) (*Key, string, User, error) {
	key, origin, err := store.Load()
	if os.IsNotExist(err) {
		return nil, "", User{}, nil
	}
	if err != nil {
		return nil, "", User{}, err
	}
	expected, err := parseOrigin(expectedOrigin)
	if err != nil {
		return nil, "", User{}, err
	}
	stored, _ := parseOrigin(origin)
	if expected.String() != stored.String() {
		return nil, "", User{}, errors.New("saved connection belongs to another server; use its --server origin or disconnect locally")
	}
	client, err := NewClient(origin, key)
	if err != nil {
		return key, origin, User{}, err
	}
	user, err := client.Me(ctx)
	return key, origin, user, err
}
