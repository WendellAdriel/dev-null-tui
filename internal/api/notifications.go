package api

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

type Notification struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	SubjectID   string     `json:"subject_id"`
	Reason      string     `json:"reason"`
	ReadAt      *time.Time `json:"read_at"`
	CreatedAt   time.Time  `json:"created_at"`
	Actor       *Author    `json:"actor"`
	Actors      []Author   `json:"actors"`
	GroupID     string     `json:"group_id"`
	EventCount  int        `json:"event_count"`
	ActorsCount int        `json:"actors_count"`
	UnreadCount int        `json:"unread_count"`
	Commit      *Commit    `json:"commit"`
}

func (n Notification) NewSince(boundary time.Time) bool {
	return n.ReadAt == nil || !n.ReadAt.Before(boundary)
}

type Notifications struct {
	Data   []Notification `json:"data"`
	ReadAt time.Time      `json:"read_at"`
	Meta   struct {
		NextCursor *string `json:"next_cursor"`
	} `json:"meta"`
	Next string `json:"-"`
}

func validNotificationFilter(filter string) bool {
	if filter == "all" || filter == "none" {
		return true
	}
	for _, category := range strings.Split(filter, ",") {
		switch category {
		case "replies", "quotes", "stars", "mentions", "follows", "moderation":
		default:
			return false
		}
	}
	return true
}

func (c *Client) Notifications(ctx context.Context, filter, cursor string) (Notifications, error) {
	return c.notifications(ctx, "/api/v1/notifications", filter, cursor, true)
}

func (c *Client) NotificationGroup(ctx context.Context, id, filter, cursor string) (Notifications, error) {
	if !commitID.MatchString(id) {
		return Notifications{}, errors.New("invalid notification id")
	}
	return c.notifications(ctx, "/api/v1/notifications/"+id+"/group", filter, cursor, false)
}

func (c *Client) notifications(ctx context.Context, path, filter, cursor string, inbox bool) (Notifications, error) {
	if !validNotificationFilter(filter) {
		return Notifications{}, errors.New("invalid notification filter")
	}
	query := url.Values{"type": {filter}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	var page Notifications
	if err := c.transport.GetJSON(ctx, path+"?"+query.Encode(), &page); err != nil {
		return Notifications{}, err
	}
	if page.Data == nil || (inbox && page.ReadAt.IsZero()) {
		return Notifications{}, errors.New("server returned an invalid notification page")
	}
	for _, n := range page.Data {
		if n.ID == "" || n.GroupID == "" || n.Actor == nil || n.Actor.Handle == "" || n.CreatedAt.IsZero() || (n.Commit != nil && !validCommit(*n.Commit)) {
			return Notifications{}, errors.New("server returned an invalid notification")
		}
	}
	if page.Meta.NextCursor != nil {
		page.Next = *page.Meta.NextCursor
	}
	if page.Next != "" && page.Next == cursor {
		return Notifications{}, errors.New("server repeated a pagination cursor")
	}
	return page, nil
}

func (c *Client) NotificationCount(ctx context.Context) (int, error) {
	var result struct {
		Count *int `json:"unread_notifications_count"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/me", &result); err != nil {
		return 0, err
	}
	if result.Count == nil || *result.Count < 0 {
		return 0, errors.New("server returned no notification count")
	}
	return *result.Count, nil
}
