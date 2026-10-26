package api

import (
	"context"
	"errors"
	"net/url"
	"regexp"
)

var commitID = regexp.MustCompile(`^[0-9A-Za-z-]{1,64}$`)

type Thread struct {
	Commit    Commit   `json:"data"`
	Ancestors []Commit `json:"ancestors"`
}

type Replies struct {
	Data []Commit `json:"data"`
	Meta struct {
		NextCursor *string `json:"next_cursor"`
	} `json:"meta"`
	Next string `json:"-"`
}

func commitPath(id string) (string, error) {
	if !commitID.MatchString(id) {
		return "", errors.New("invalid commit id")
	}
	return "/api/v1/commits/" + id, nil
}

func validCommit(commit Commit) bool {
	return commit.ID != "" && (commit.DeletedAt != nil || commit.Author != nil)
}

func (c *Client) Thread(ctx context.Context, id string) (Thread, error) {
	path, err := commitPath(id)
	if err != nil {
		return Thread{}, err
	}
	var thread Thread
	if err := c.transport.GetJSON(ctx, path, &thread); err != nil {
		return Thread{}, err
	}
	if thread.Commit.ID != id || !validCommit(thread.Commit) {
		return Thread{}, errors.New("server returned an invalid commit")
	}
	for _, ancestor := range thread.Ancestors {
		if !validCommit(ancestor) {
			return Thread{}, errors.New("server returned an invalid commit")
		}
	}
	return thread, nil
}

func (c *Client) Replies(ctx context.Context, id, cursor string) (Replies, error) {
	path, err := commitPath(id)
	if err != nil {
		return Replies{}, err
	}
	path += "/replies"
	if cursor != "" {
		path += "?cursor=" + url.QueryEscape(cursor)
	}
	var replies Replies
	if err := c.transport.GetJSON(ctx, path, &replies); err != nil {
		return Replies{}, err
	}
	if replies.Data == nil {
		return Replies{}, errors.New("server returned invalid replies")
	}
	for _, reply := range replies.Data {
		if !validCommit(reply) {
			return Replies{}, errors.New("server returned an invalid commit")
		}
	}
	if replies.Meta.NextCursor != nil {
		replies.Next = *replies.Meta.NextCursor
	}
	if replies.Next != "" && replies.Next == cursor {
		return Replies{}, errors.New("server repeated a pagination cursor")
	}
	return replies, nil
}

func (c *Client) DeleteCommit(ctx context.Context, id string) error {
	path, err := commitPath(id)
	if err != nil {
		return err
	}
	return c.transport.Delete(ctx, path)
}
