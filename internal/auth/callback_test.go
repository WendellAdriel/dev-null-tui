package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCallbackDeliveryIsPrivateBoundedAndSingleUse(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	a, _ := NewAttempt(DefaultOrigin, "SHA256:example", "Test device")
	callback, err := ListenCallback(store, a.State)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	link := RedirectURI + "?code=synthetic-code&state=" + a.State
	if err := DeliverCallback(t.Context(), store, RedirectURI+"?code=x&state=../../outside"); err == nil {
		t.Fatal("accepted unsafe state")
	}
	if err := DeliverCallback(t.Context(), store, link); err != nil {
		t.Fatal(err)
	}
	if err := DeliverCallback(t.Context(), store, link); err == nil {
		t.Fatal("delivered twice")
	}
	select {
	case got := <-callback.Links:
		if got != link {
			t.Fatal("callback changed")
		}
	case <-time.After(time.Second):
		t.Fatal("callback never arrived")
	}
	callback.Close()
	if _, err := os.Stat(filepath.Join(store.Dir, a.State+".attempt")); !os.IsNotExist(err) {
		t.Fatal("attempt material not cleaned up")
	}
	if err := DeliverCallback(context.Background(), store, link); err == nil {
		t.Fatal("delivered to ended attempt")
	}
}
