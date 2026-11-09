package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type Preview struct {
	Body     string    `json:"body"`
	Segments []Segment `json:"body_segments"`
}

func (c *Client) Preview(ctx context.Context, body string) (Preview, error) {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{body})
	if err != nil {
		return Preview{}, err
	}
	var result struct {
		Data Preview `json:"data"`
	}
	if err := c.transport.PostJSON(ctx, "/api/v1/commits/preview", payload, &result); err != nil {
		return Preview{}, err
	}
	if result.Data.Body == "" || len(result.Data.Segments) == 0 {
		return Preview{}, errors.New("server returned an invalid preview")
	}
	return result.Data, nil
}

type NewCommit struct {
	Body string
	// Author is the handle of an organization to post as, or empty to post as yourself.
	Author   string
	ParentID string
	QuotedID string
	Poll     *NewPoll
	Images   []ImageFile
}

// Validate applies the server's combination rules: a poll goes alone on a top-level commit, and images make text
// optional.
func (draft NewCommit) Validate() error {
	if draft.Author != "" && !handlePattern.MatchString(draft.Author) {
		return errors.New("invalid organization handle")
	}
	if draft.ParentID != "" && draft.QuotedID != "" {
		return errors.New("a commit cannot both reply and quote")
	}
	if draft.Poll != nil {
		switch {
		case draft.ParentID != "":
			return errors.New("polls can't go on replies")
		case draft.QuotedID != "":
			return errors.New("polls can't go on quotes")
		case len(draft.Images) > 0:
			return errors.New("polls can't go with images")
		}
		if err := ValidatePoll(*draft.Poll); err != nil {
			return err
		}
	}
	if len(draft.Images) > MaxImages {
		return fmt.Errorf("attach at most %d images", MaxImages)
	}
	for _, image := range draft.Images {
		if err := validDescription(image.Description); err != nil {
			return err
		}
	}
	if len(draft.Images) > 0 && NormalizeBody(draft.Body) == "" {
		return nil
	}
	return ValidateBody(draft.Body)
}

// payload is the JSON request body, with scheduledAt set only for a scheduled commit.
func (draft NewCommit) payload(scheduledAt string) ([]byte, error) {
	payload := struct {
		Body         string    `json:"body"`
		ParentID     string    `json:"parent_id,omitempty"`
		QuotedID     string    `json:"quoted_commit_id,omitempty"`
		Author       string    `json:"author,omitempty"`
		Poll         *NewPoll  `json:"poll,omitempty"`
		Descriptions []*string `json:"image_descriptions,omitempty"`
		ScheduledAt  string    `json:"scheduled_at,omitempty"`
	}{Body: draft.Body, ParentID: draft.ParentID, QuotedID: draft.QuotedID, Author: draft.Author, ScheduledAt: scheduledAt}
	if draft.Poll != nil {
		poll := draft.Poll.trimmed()
		payload.Poll = &poll
	}
	for _, image := range draft.Images {
		description := trimInput(image.Description)
		if description == "" {
			payload.Descriptions = append(payload.Descriptions, nil)
			continue
		}
		payload.Descriptions = append(payload.Descriptions, &description)
	}
	return json.Marshal(payload)
}

func (c *Client) CreateCommit(ctx context.Context, draft NewCommit) (Commit, error) {
	if err := draft.Validate(); err != nil {
		return Commit{}, err
	}
	payload, err := draft.payload("")
	if err != nil {
		return Commit{}, err
	}
	var result struct {
		Data Commit `json:"data"`
	}
	if len(draft.Images) > 0 {
		err = c.transport.PostMultipart(ctx, "/api/v1/commits", payload, draft.uploads(), &result)
	} else {
		err = c.transport.PostJSON(ctx, "/api/v1/commits", payload, &result)
	}
	if err != nil {
		return Commit{}, err
	}
	commit := result.Data
	if commit.ID == "" || commit.Author == nil || (commit.Status != "pending" && commit.Status != "approved" && commit.Status != "rejected") {
		return Commit{}, errors.New("server returned an invalid submission response")
	}
	return commit, nil
}
