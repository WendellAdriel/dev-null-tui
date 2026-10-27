package api

import (
	"context"
	"net/url"
)

func (c *Client) Starred(ctx context.Context, cursor string) (CommitPage, error) {
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return c.commitPage(ctx, "/api/v1/starred", query, cursor)
}
