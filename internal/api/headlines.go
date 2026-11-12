package api

import (
	"context"
	"errors"
	"regexp"
)

var headlineID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Headline is a hot topic. Summary is its recap, or nil while there's no fresh one that is safe to show.
type Headline struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Summary *string `json:"summary"`
}

// HeadlinePage is a hot topic's page: the past week's commits about it, all at once, newest first.
type HeadlinePage struct {
	Data    []Commit `json:"data"`
	Name    string   `json:"name"`
	Summary *string  `json:"summary"`
}

// Headlines reads the hot topics the web's aside shows.
func (c *Client) Headlines(ctx context.Context) ([]Headline, error) {
	var response struct {
		Data []Headline `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/headlines", &response); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return nil, errors.New("server returned invalid hot topics")
	}
	for _, topic := range response.Data {
		if !headlineID.MatchString(topic.ID) || topic.Name == "" {
			return nil, errors.New("server returned an invalid hot topic")
		}
	}
	return response.Data, nil
}

// Headline reads one hot topic's page.
func (c *Client) Headline(ctx context.Context, id string) (HeadlinePage, error) {
	if !headlineID.MatchString(id) {
		return HeadlinePage{}, errors.New("invalid hot topic")
	}
	var page HeadlinePage
	if err := c.transport.GetJSON(ctx, "/api/v1/headlines/"+id, &page); err != nil {
		return HeadlinePage{}, err
	}
	if page.Data == nil || page.Name == "" {
		return HeadlinePage{}, errors.New("server returned an invalid hot topic")
	}
	for _, commit := range page.Data {
		if !validCommit(commit) {
			return HeadlinePage{}, errors.New("server returned an invalid commit")
		}
	}
	return page, nil
}
