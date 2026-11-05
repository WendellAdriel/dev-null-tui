package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// MaxLiveHours mirrors the server's `User::MAX_LIVE_HOURS`, the longest a stream can be shown as live.
const MaxLiveHours = 12

func (c *Client) GoLive(ctx context.Context, link string, hours int) (auth.User, error) {
	link = strings.TrimSpace(link)
	switch {
	case link == "":
		return auth.User{}, errors.New("paste a link to your stream on YouTube or Twitch")
	case utf8.RuneCountInString(link) > 2048:
		return auth.User{}, errors.New("that link is too long")
	case hours < 1 || hours > MaxLiveHours:
		return auth.User{}, errors.New("choose from 1 to 12 hours")
	}
	return c.updateAccount(ctx, "live", struct {
		URL   string `json:"url"`
		Hours int    `json:"hours"`
	}{link, hours})
}

func (c *Client) EndStream(ctx context.Context) (auth.User, error) {
	return c.sendAccount(ctx, http.MethodDelete, "live", nil)
}

// LiveFollowing is the web's Live now aside: a few people you follow who are live.
func (c *Client) LiveFollowing(ctx context.Context) ([]User, error) {
	var response struct {
		Data []User `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/me/live-following", &response); err != nil {
		return nil, err
	}
	for _, person := range response.Data {
		if person.ID == "" || !handlePattern.MatchString(person.Handle) {
			return nil, errors.New("server returned an invalid person")
		}
	}
	return response.Data, nil
}
