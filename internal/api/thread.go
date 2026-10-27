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

type CommitPage struct {
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

func (c *Client) Replies(ctx context.Context, id, cursor string) (CommitPage, error) {
	path, err := commitPath(id)
	if err != nil {
		return CommitPage{}, err
	}
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return c.commitPage(ctx, path+"/replies", query, cursor)
}

func (c *Client) commitPage(ctx context.Context, path string, query url.Values, cursor string) (CommitPage, error) {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var page CommitPage
	if err := c.transport.GetJSON(ctx, path, &page); err != nil {
		return CommitPage{}, err
	}
	if page.Data == nil {
		return CommitPage{}, errors.New("server returned an invalid commit page")
	}
	for _, commit := range page.Data {
		if !validCommit(commit) {
			return CommitPage{}, errors.New("server returned an invalid commit")
		}
	}
	if page.Meta.NextCursor != nil {
		page.Next = *page.Meta.NextCursor
	}
	if page.Next != "" && page.Next == cursor {
		return CommitPage{}, errors.New("server repeated a pagination cursor")
	}
	return page, nil
}

func (c *Client) DeleteCommit(ctx context.Context, id string) error {
	path, err := commitPath(id)
	if err != nil {
		return err
	}
	return c.transport.Delete(ctx, path)
}
