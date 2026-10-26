package api

import (
	"context"
	"encoding/json"
	"errors"
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

func (c *Client) CreateCommit(ctx context.Context, body string) (Commit, error) {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{body})
	if err != nil {
		return Commit{}, err
	}
	var result struct {
		Data Commit `json:"data"`
	}
	if err := c.transport.PostJSON(ctx, "/api/v1/commits", payload, &result); err != nil {
		return Commit{}, err
	}
	commit := result.Data
	if commit.ID == "" || commit.Author == nil || (commit.Status != "pending" && commit.Status != "approved" && commit.Status != "rejected") {
		return Commit{}, errors.New("server returned an invalid submission response")
	}
	return commit, nil
}
