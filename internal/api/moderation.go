package api

import (
	"context"
	"errors"
	"time"
)

// ModerationHistory is your own record of timeouts and bans, newest first, which only you and moderators see.
type ModerationHistory struct {
	TimeoutsCount int                 `json:"timeouts_count"`
	BansCount     int                 `json:"bans_count"`
	Timeouts      []ModerationTimeout `json:"timeouts"`
	Bans          []ModerationBan     `json:"bans"`
}

type ModerationTimeout struct {
	ID        string    `json:"id"`
	Reason    string    `json:"reason"`
	Duration  string    `json:"duration"`
	StartedAt time.Time `json:"started_at"`
	EndsAt    time.Time `json:"ends_at"`
}

type ModerationBan struct {
	ID       string    `json:"id"`
	Reason   string    `json:"reason"`
	BannedAt time.Time `json:"banned_at"`
}

func (c *Client) ModerationHistory(ctx context.Context) (ModerationHistory, error) {
	var response struct {
		Data *ModerationHistory `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/me/moderation-history", &response); err != nil {
		return ModerationHistory{}, err
	}
	if response.Data == nil {
		return ModerationHistory{}, errors.New("server returned no moderation history")
	}
	return *response.Data, nil
}
