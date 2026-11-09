package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const (
	// MaxScheduleDays is how far ahead the server accepts a scheduled time.
	MaxScheduleDays = 30
	scheduledFormat = "2006-01-02T15:04:05-07:00"
)

var scheduledID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ScheduledCommit struct {
	ID          string     `json:"id"`
	Author      *Author    `json:"author"`
	Body        *string    `json:"body"`
	Segments    []Segment  `json:"body_segments"`
	Preview     []Segment  `json:"preview_segments"`
	PollOptions []string   `json:"poll_options"`
	ImageCount  int        `json:"image_count"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	SubmittedAt *time.Time `json:"submitted_at"`
	// Status is scheduled, deleted, or the submitted commit's moderation status.
	Status string `json:"status"`
}

type ScheduledPage struct {
	Data []ScheduledCommit `json:"data"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
}

// More reports whether a later page exists.
func (p ScheduledPage) More() bool { return p.Meta.CurrentPage < p.Meta.LastPage }

func validScheduled(s ScheduledCommit) bool {
	if !scheduledID.MatchString(s.ID) || s.ScheduledAt.IsZero() || s.ImageCount < 0 {
		return false
	}
	switch s.Status {
	case "scheduled":
		return s.SubmittedAt == nil
	case "deleted", "pending", "approved", "rejected":
		return true
	}
	return false
}

func (c *Client) ScheduledCommits(ctx context.Context, page int) (ScheduledPage, error) {
	target := "/api/v1/scheduled-commits"
	if page > 1 {
		target += "?" + url.Values{"page": {strconv.Itoa(page)}}.Encode()
	}
	var result ScheduledPage
	if err := c.transport.GetJSON(ctx, target, &result); err != nil {
		return ScheduledPage{}, err
	}
	if result.Data == nil {
		return ScheduledPage{}, errors.New("server returned an invalid scheduled list")
	}
	for _, s := range result.Data {
		if !validScheduled(s) {
			return ScheduledPage{}, errors.New("server returned an invalid scheduled commit")
		}
	}
	return result, nil
}

// ValidScheduleTime applies the server's window: after now and at most MaxScheduleDays ahead.
func ValidScheduleTime(at, now time.Time) error {
	if !at.After(now) {
		return errors.New("choose a time in the future")
	}
	if at.After(now.AddDate(0, 0, MaxScheduleDays)) {
		return fmt.Errorf("choose a time within %d days", MaxScheduleDays)
	}
	return nil
}

// ScheduleCommit queues a top-level commit to be submitted at the given time.
func (c *Client) ScheduleCommit(ctx context.Context, draft NewCommit, at time.Time) (ScheduledCommit, error) {
	if draft.ParentID != "" || draft.QuotedID != "" {
		return ScheduledCommit{}, errors.New("replies and quotes can't be scheduled")
	}
	if err := draft.Validate(); err != nil {
		return ScheduledCommit{}, err
	}
	if err := ValidScheduleTime(at, time.Now()); err != nil {
		return ScheduledCommit{}, err
	}
	payload, err := draft.payload(at.UTC().Format(scheduledFormat))
	if err != nil {
		return ScheduledCommit{}, err
	}
	var result struct {
		Data ScheduledCommit `json:"data"`
	}
	if len(draft.Images) > 0 {
		err = c.transport.PostMultipart(ctx, "/api/v1/scheduled-commits", payload, draft.uploads(), &result)
	} else {
		err = c.transport.PostJSON(ctx, "/api/v1/scheduled-commits", payload, &result)
	}
	if err != nil {
		return ScheduledCommit{}, err
	}
	if !validScheduled(result.Data) || result.Data.Status != "scheduled" {
		return ScheduledCommit{}, errors.New("server returned an invalid scheduled commit")
	}
	return result.Data, nil
}

func scheduledPath(id string) (string, error) {
	if !scheduledID.MatchString(id) {
		return "", errors.New("invalid scheduled commit id")
	}
	return "/api/v1/scheduled-commits/" + id, nil
}

func (c *Client) CancelScheduled(ctx context.Context, id string) error {
	path, err := scheduledPath(id)
	if err != nil {
		return err
	}
	return c.transport.Delete(ctx, path)
}

func (c *Client) SubmitScheduled(ctx context.Context, id string) (ScheduledCommit, error) {
	path, err := scheduledPath(id)
	if err != nil {
		return ScheduledCommit{}, err
	}
	var result struct {
		Data ScheduledCommit `json:"data"`
	}
	if err := c.transport.PostJSON(ctx, path+"/submission", nil, &result); err != nil {
		return ScheduledCommit{}, err
	}
	if result.Data.ID != id || !validScheduled(result.Data) || result.Data.SubmittedAt == nil {
		return ScheduledCommit{}, errors.New("server returned an invalid submission")
	}
	return result.Data, nil
}

// uploads lists the draft's images in order, for multipart requests.
func (draft NewCommit) uploads() []auth.Upload {
	uploads := make([]auth.Upload, len(draft.Images))
	for i, image := range draft.Images {
		uploads[i] = image.Upload
	}
	return uploads
}
