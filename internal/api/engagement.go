package api

import (
	"context"
	"errors"
)

type Pastel struct {
	Commit    Commit `json:"data"`
	Remaining *int   `json:"pasteis_remaining"`
}

func (c *Client) SetStar(ctx context.Context, id string, starred bool) (Commit, error) {
	path, err := commitPath(id)
	if err != nil {
		return Commit{}, err
	}
	var result struct {
		Commit Commit `json:"data"`
	}
	if starred {
		err = c.transport.PostJSON(ctx, path+"/star", nil, &result)
	} else {
		err = c.transport.DeleteJSON(ctx, path+"/star", &result)
	}
	if err != nil {
		return Commit{}, err
	}
	if result.Commit.ID != id || !validCommit(result.Commit) || result.Commit.Starred == nil || *result.Commit.Starred != starred || result.Commit.Stars < 0 {
		return Commit{}, errors.New("server returned an invalid star confirmation")
	}
	return result.Commit, nil
}

func (c *Client) GivePastel(ctx context.Context, id string) (Pastel, error) {
	path, err := commitPath(id)
	if err != nil {
		return Pastel{}, err
	}
	var result Pastel
	if err := c.transport.PostJSON(ctx, path+"/pasteis", nil, &result); err != nil {
		return Pastel{}, err
	}
	if result.Commit.ID != id || !validCommit(result.Commit) || result.Commit.Pasteis == nil || *result.Commit.Pasteis < 0 || result.Remaining == nil || *result.Remaining < 0 {
		return Pastel{}, errors.New("server returned an invalid pastel confirmation")
	}
	return result, nil
}
