package api

import (
	"io"
	"net/http"
	"testing"
)

func TestSettingsUseSignedContractsAndDecodeCurrentAccount(t *testing.T) {
	want := []string{`{"lane":null}`, `{"lane":3}`, `{"lane":null,"color":"#123abc"}`, `{}`, `{"enabled":true}`, `{"enabled":false}`, `{"from":"following","pull_receipts":true}`}
	paths := []string{"lane", "lane", "lane", "github", "pasteis-mode", "remotes", "remote-preferences"}
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if calls >= len(want) || r.Method != "PUT" || r.URL.Path != "/api/v1/me/"+paths[calls] || string(body) != want[calls] || r.Header.Get("X-SSH-Signature") == "" {
			t.Fatalf("unexpected settings request: %s %s %s", r.Method, r.URL, body)
		}
		calls++
		_, _ = io.WriteString(w, `{"data":{"id":"me","handle":"fixture","preferred_lane":"#123abc","automatic_lane":2,"remotes":{"from":"following","pull_receipts":true,"everyone":false}},"unread_notifications_count":4}`)
	})
	ctx := t.Context()
	for _, lane := range []*int{nil, new(3)} {
		if _, err := c.SetLane(ctx, lane, ""); err != nil {
			t.Fatal(err)
		}
	}
	user, err := c.SetLane(ctx, nil, "#123abc")
	if err != nil || user.Remotes == nil || user.Remotes.From != "following" || user.UnreadNotifications == nil || *user.UnreadNotifications != 4 {
		t.Fatalf("account: %+v %v", user, err)
	}
	if _, err := c.SyncProfile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetPasteis(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetRemotes(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetRemotePreferences(ctx, "following", true); err != nil {
		t.Fatal(err)
	}
	for _, lane := range []int{-1, 6} {
		if _, err := c.SetLane(ctx, &lane, ""); err == nil {
			t.Fatal("invalid lane accepted")
		}
	}
	for _, color := range []string{"#ABCDEF", "red", "#123"} {
		if _, err := c.SetLane(ctx, nil, color); err == nil {
			t.Fatal("invalid color accepted")
		}
	}
	if _, err := c.SetLane(ctx, new(1), "#123abc"); err == nil {
		t.Fatal("conflicting lane and color accepted")
	}
	if _, err := c.SetRemotePreferences(ctx, "unknown", false); err == nil {
		t.Fatal("invalid remote preference accepted")
	}
	if calls != len(want) {
		t.Fatal("validation sent unexpected requests")
	}
}

func TestSafetyRequiresConfirmedRelationshipAndValidPages(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned safety request")
		}
		if r.Method == "GET" {
			_, _ = io.WriteString(w, `{"data":[{"id":"ada","handle":"ada"}],"meta":{"next_cursor":"next"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"ada","handle":"ada"},"is_muting":true,"is_blocking":true}`)
	})
	for _, kind := range []string{"mute", "block"} {
		if _, err := c.SetSafety(t.Context(), "ada", kind, true); err != nil {
			t.Fatal(err)
		}
		if _, err := c.SetSafety(t.Context(), "ada", kind, false); err == nil {
			t.Fatal("unconfirmed relationship accepted")
		}
		page, err := c.SafetyList(t.Context(), kind, "")
		if err != nil || len(page.Data) != 1 || page.Next != "next" {
			t.Fatal(page, err)
		}
		if _, err := c.SafetyList(t.Context(), kind, "next"); err == nil {
			t.Fatal("repeated cursor accepted")
		}
	}
}

func TestReportsAndAccountDeletionRequireNoContentAndDoNotRetry(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned write")
		}
		if r.URL.Path == "/api/v1/me" && r.Method != "DELETE" {
			t.Error("wrong delete method")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Report(t.Context(), "commit", "spam", "details"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteAccount(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Report(t.Context(), "commit", "unknown", ""); err == nil {
		t.Fatal("unknown reason accepted")
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	wrong := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{}`) })
	if err := wrong.DeleteAccount(t.Context()); err == nil {
		t.Fatal("200 response treated as confirmed deletion")
	}
}

func TestRegionalSettingsReadAndWriteSignedContracts(t *testing.T) {
	want := []string{`{"timezone":"Europe/Lisbon","show_local_time":true}`, `{"timezone":null,"show_local_time":false}`, `{"enabled":false}`, `{"translation_language":"pt"}`, `{"translation_language":null}`}
	paths := []string{"timezone", "timezone", "automatic-translation", "translation-language", "translation-language"}
	calls := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned request")
		}
		if r.Method == "GET" {
			_, _ = io.WriteString(w, `{"data":{"timezone":null,"show_local_time":false,"translation_language":"pt","automatic_translation":true,"translation_languages":[{"code":"pt","name":"Portuguese","native_name":"português"}],"is_admin":false}}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if calls >= len(want) || r.Method != "PUT" || r.URL.Path != "/api/v1/me/"+paths[calls] || string(body) != want[calls] {
			t.Fatalf("unexpected request: %s %s %s", r.Method, r.URL, body)
		}
		calls++
		_, _ = io.WriteString(w, `{"data":{"id":"me","handle":"fixture"}}`)
	})
	ctx := t.Context()
	settings, err := c.AccountSettings(ctx)
	if err != nil || settings.Timezone != "" || settings.TranslationLanguage != "pt" || !settings.AutomaticTranslation || len(settings.TranslationLanguages) != 1 || settings.TranslationLanguages[0].NativeName != "português" {
		t.Fatalf("settings: %+v %v", settings, err)
	}
	if _, err := c.SetTimezone(ctx, "Europe/Lisbon", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetTimezone(ctx, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetAutomaticTranslation(ctx, false); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"pt", ""} {
		if _, err := c.SetTranslationLanguage(ctx, code); err != nil {
			t.Fatal(err)
		}
	}
	if calls != len(want) {
		t.Fatal(calls)
	}
}

func TestDevicesListAndRevokeRequireValidResponses(t *testing.T) {
	const id = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	deletes := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SSH-Signature") == "" {
			t.Error("unsigned request")
		}
		if r.Method == "DELETE" {
			if r.URL.Path != "/api/v1/me/devices/"+id {
				t.Errorf("unexpected revoke path %s", r.URL.Path)
			}
			deletes++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"`+id+`","name":"laptop","app_name":"/dev/null TUI","fingerprint":"SHA256:abc","approved_at":"2026-11-01T10:00:00Z","last_used_at":null}]}`)
	})
	devices, err := c.Devices(t.Context())
	if err != nil || len(devices) != 1 || devices[0].Fingerprint != "SHA256:abc" || devices[0].LastUsedAt != "" {
		t.Fatalf("devices: %+v %v", devices, err)
	}
	if err := c.RevokeDevice(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeDevice(t.Context(), "../me"); err == nil {
		t.Fatal("invalid device ID accepted")
	}
	if deletes != 1 {
		t.Fatal(deletes)
	}
	invalid := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"","fingerprint":""}]}`)
	})
	if _, err := invalid.Devices(t.Context()); err == nil {
		t.Fatal("invalid device accepted")
	}
}
