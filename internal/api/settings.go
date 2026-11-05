package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

var laneHex = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func (c *Client) CurrentUser(ctx context.Context) (auth.User, error) { return c.transport.Me(ctx) }

func (c *Client) updateAccount(ctx context.Context, route string, body any) (auth.User, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return auth.User{}, err
	}
	return c.sendAccount(ctx, http.MethodPut, route, data)
}

func (c *Client) sendAccount(ctx context.Context, method, route string, data []byte) (auth.User, error) {
	var response struct {
		Data   auth.User `json:"data"`
		Unread *int      `json:"unread_notifications_count"`
	}
	if err := c.transport.SendJSON(ctx, method, "/api/v1/me/"+route, data, &response); err != nil {
		return auth.User{}, err
	}
	if response.Data.ID == "" || !handlePattern.MatchString(response.Data.Handle) {
		return auth.User{}, errors.New("server returned an invalid account")
	}
	response.Data.UnreadNotifications = response.Unread
	return response.Data, nil
}

func (c *Client) SetLane(ctx context.Context, lane *int, color string) (auth.User, error) {
	if (lane != nil && (*lane < 0 || *lane > 5 || color != "")) || (color != "" && !laneHex.MatchString(color)) {
		return auth.User{}, errors.New("choose Automatic, a lane from 0 to 5, or a lowercase #rrggbb color")
	}
	return c.updateAccount(ctx, "lane", struct {
		Lane  *int   `json:"lane"`
		Color string `json:"color,omitempty"`
	}{lane, color})
}

func (c *Client) SyncProfile(ctx context.Context) (auth.User, error) {
	return c.updateAccount(ctx, "github", struct{}{})
}
func (c *Client) SetPasteis(ctx context.Context, enabled bool) (auth.User, error) {
	return c.updateAccount(ctx, "pasteis-mode", struct {
		Enabled bool `json:"enabled"`
	}{enabled})
}
func (c *Client) SetRemotes(ctx context.Context, enabled bool) (auth.User, error) {
	return c.updateAccount(ctx, "remotes", struct {
		Enabled bool `json:"enabled"`
	}{enabled})
}
func (c *Client) SetRemotePreferences(ctx context.Context, from string, receipts bool) (auth.User, error) {
	if from != "following" && from != "everyone" {
		return auth.User{}, errors.New("invalid Remotes audience")
	}
	return c.updateAccount(ctx, "remote-preferences", struct {
		From     string `json:"from"`
		Receipts bool   `json:"pull_receipts"`
	}{from, receipts})
}

func (c *Client) DeleteAccount(ctx context.Context) error {
	return c.transport.NoContent(ctx, http.MethodDelete, "/api/v1/me", nil)
}

func (c *Client) SafetyList(ctx context.Context, kind, cursor string) (People, error) {
	if kind != "mute" && kind != "block" {
		return People{}, errors.New("invalid safety list")
	}
	path := "/api/v1/me/" + map[string]string{"mute": "muted", "block": "blocked"}[kind]
	if cursor != "" {
		path += "?" + url.Values{"cursor": {cursor}}.Encode()
	}
	var page People
	if err := c.transport.GetJSON(ctx, path, &page); err != nil {
		return People{}, err
	}
	if page.Data == nil {
		return People{}, errors.New("server returned an invalid safety list")
	}
	for _, u := range page.Data {
		if u.ID == "" || !handlePattern.MatchString(u.Handle) {
			return People{}, errors.New("server returned an invalid person")
		}
	}
	if page.Meta.NextCursor != nil {
		page.Next = *page.Meta.NextCursor
	}
	if page.Next != "" && page.Next == cursor {
		return People{}, errors.New("server repeated a pagination cursor")
	}
	return page, nil
}

func (c *Client) SetSafety(ctx context.Context, handle, kind string, enabled bool) (User, error) {
	path, err := userPath(handle)
	if err != nil {
		return User{}, err
	}
	if kind != "mute" && kind != "block" {
		return User{}, errors.New("invalid safety action")
	}
	var response struct {
		User     User  `json:"data"`
		Muting   *bool `json:"is_muting"`
		Blocking *bool `json:"is_blocking"`
	}
	if enabled {
		err = c.transport.PostJSON(ctx, path+"/"+kind, nil, &response)
	} else {
		err = c.transport.DeleteJSON(ctx, path+"/"+kind, &response)
	}
	if err != nil {
		return User{}, err
	}
	confirmed := response.Muting
	if kind == "block" {
		confirmed = response.Blocking
	}
	if response.User.ID == "" || !strings.EqualFold(response.User.Handle, handle) || confirmed == nil || *confirmed != enabled {
		return User{}, errors.New("server did not confirm the safety change; refresh before retrying")
	}
	return response.User, nil
}

var ReportReasons = []struct{ Value, Label string }{{"spam", "Spam or misleading links"}, {"harassment", "Harassment or hate"}, {"unsafe", "Harmful or private information"}, {"other", "Something else"}}

func (c *Client) Report(ctx context.Context, id, reason, details string) error {
	valid := false
	for _, r := range ReportReasons {
		valid = valid || r.Value == reason
	}
	if !valid || utf8.RuneCountInString(details) > 500 {
		return errors.New("choose a report reason and at most 500 characters of details")
	}
	path, err := commitPath(id)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		Reason  string `json:"reason"`
		Details string `json:"details,omitempty"`
	}{reason, details})
	if err != nil {
		return err
	}
	return c.transport.NoContent(ctx, http.MethodPost, path+"/reports", body)
}

var deviceID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type AccountSettings struct {
	Timezone             string     `json:"timezone"`
	ShowLocalTime        bool       `json:"show_local_time"`
	TranslationLanguage  string     `json:"translation_language"`
	AutomaticTranslation bool       `json:"automatic_translation"`
	TranslationLanguages []Language `json:"translation_languages"`
}

type Language struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
}

type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AppName     string `json:"app_name"`
	Fingerprint string `json:"fingerprint"`
	ApprovedAt  string `json:"approved_at"`
	LastUsedAt  string `json:"last_used_at"`
}

func (c *Client) AccountSettings(ctx context.Context) (AccountSettings, error) {
	var response struct {
		Data *AccountSettings `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/me/settings", &response); err != nil {
		return AccountSettings{}, err
	}
	if response.Data == nil {
		return AccountSettings{}, errors.New("server returned invalid settings")
	}
	return *response.Data, nil
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (c *Client) SetTimezone(ctx context.Context, zone string, showLocalTime bool) (auth.User, error) {
	return c.updateAccount(ctx, "timezone", struct {
		Timezone      *string `json:"timezone"`
		ShowLocalTime bool    `json:"show_local_time"`
	}{nullable(zone), zone != "" && showLocalTime})
}

func (c *Client) SetAutomaticTranslation(ctx context.Context, enabled bool) (auth.User, error) {
	return c.updateAccount(ctx, "automatic-translation", struct {
		Enabled bool `json:"enabled"`
	}{enabled})
}

func (c *Client) SetTranslationLanguage(ctx context.Context, code string) (auth.User, error) {
	return c.updateAccount(ctx, "translation-language", struct {
		Language *string `json:"translation_language"`
	}{nullable(code)})
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var response struct {
		Data []Device `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/me/devices", &response); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return nil, errors.New("server returned an invalid device list")
	}
	for _, d := range response.Data {
		if !deviceID.MatchString(d.ID) || d.Fingerprint == "" {
			return nil, errors.New("server returned an invalid device")
		}
	}
	return response.Data, nil
}

func (c *Client) RevokeDevice(ctx context.Context, id string) error {
	if !deviceID.MatchString(id) {
		return errors.New("invalid device")
	}
	return c.transport.NoContent(ctx, http.MethodDelete, "/api/v1/me/devices/"+id, nil)
}
