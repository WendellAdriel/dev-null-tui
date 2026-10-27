package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var handlePattern = regexp.MustCompile(`^[0-9A-Za-z-]{1,39}$`)

type Connections string

const (
	Followers     Connections = "followers"
	FollowingList Connections = "following"
)

type User struct {
	ID         string          `json:"id"`
	Handle     string          `json:"handle"`
	Name       string          `json:"name"`
	AvatarURL  string          `json:"avatar_url"`
	Bio        string          `json:"bio"`
	Company    string          `json:"company"`
	Location   string          `json:"location"`
	Website    string          `json:"website"`
	GitHubURL  string          `json:"github_url"`
	URL        string          `json:"html_url"`
	CreatedAt  time.Time       `json:"created_at"`
	Commits    *int            `json:"commits_count"`
	Followers  *int            `json:"followers_count"`
	Following  *int            `json:"following_count"`
	Lane       json.RawMessage `json:"lane"`
	IsFollowed *bool           `json:"is_followed"`
	FollowsYou *bool           `json:"follows_you"`
}

type Activity struct {
	Scale   string `json:"scale"`
	Buckets []struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	} `json:"buckets"`
}

type Streaks struct {
	Longest int `json:"longest"`
	Current int `json:"current"`
}

type Rank struct {
	Rank     string `json:"rank"`
	Level    int    `json:"level"`
	MinXP    int    `json:"min_xp"`
	Achieved bool   `json:"achieved"`
}

type Badge struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Unlocked bool   `json:"unlocked"`
}

type Achievements struct {
	Level         int     `json:"level"`
	Rank          string  `json:"rank"`
	XP            int     `json:"xp"`
	NextLevelXP   *int    `json:"next_level_xp"`
	LevelProgress float64 `json:"level_progress"`
	Ranks         []Rank  `json:"ranks"`
	Badges        []Badge `json:"badges"`
}

type Profile struct {
	User            User          `json:"data"`
	IsFollowing     *bool         `json:"is_following"`
	FollowsYou      *bool         `json:"follows_you"`
	IsMuting        *bool         `json:"is_muting"`
	PasteisReceived *int          `json:"pasteis_received"`
	Activity        *Activity     `json:"activity"`
	Streaks         *Streaks      `json:"streaks"`
	Achievements    *Achievements `json:"achievements"`
	Pinned          *Commit       `json:"pinned_commit"`
}

type People struct {
	Data []User `json:"data"`
	Meta struct {
		NextCursor *string `json:"next_cursor"`
	} `json:"meta"`
	Next string `json:"-"`
}

func userPath(handle string) (string, error) {
	if !handlePattern.MatchString(handle) {
		return "", errors.New("invalid handle")
	}
	return "/api/v1/users/" + handle, nil
}

func (c *Client) Profile(ctx context.Context, handle string) (Profile, error) {
	path, err := userPath(handle)
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := c.transport.GetJSON(ctx, path, &profile); err != nil {
		return Profile{}, err
	}
	if profile.User.ID == "" || !strings.EqualFold(profile.User.Handle, handle) || (profile.Pinned != nil && !validCommit(*profile.Pinned)) {
		return Profile{}, errors.New("server returned an invalid profile")
	}
	return profile, nil
}

func (c *Client) UserCommits(ctx context.Context, handle string, replies bool, cursor string) (CommitPage, error) {
	path, err := userPath(handle)
	if err != nil {
		return CommitPage{}, err
	}
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if replies {
		query.Set("replies", "1")
	}
	return c.commitPage(ctx, path+"/commits", query, cursor)
}

func (c *Client) Connections(ctx context.Context, handle string, list Connections, cursor string) (People, error) {
	path, err := userPath(handle)
	if err != nil {
		return People{}, err
	}
	if list != Followers && list != FollowingList {
		return People{}, errors.New("unknown connection list")
	}
	path += "/" + string(list)
	if cursor != "" {
		path += "?" + url.Values{"cursor": {cursor}}.Encode()
	}
	var people People
	if err := c.transport.GetJSON(ctx, path, &people); err != nil {
		return People{}, err
	}
	if people.Data == nil {
		return People{}, errors.New("server returned an invalid connection page")
	}
	for _, person := range people.Data {
		if person.ID == "" || !handlePattern.MatchString(person.Handle) {
			return People{}, errors.New("server returned an invalid person")
		}
	}
	if people.Meta.NextCursor != nil {
		people.Next = *people.Meta.NextCursor
	}
	if people.Next != "" && people.Next == cursor {
		return People{}, errors.New("server repeated a pagination cursor")
	}
	return people, nil
}

func (c *Client) SetFollow(ctx context.Context, handle string, follow bool) error {
	path, err := userPath(handle)
	if err != nil {
		return err
	}
	if follow {
		return c.transport.Post(ctx, path+"/follow")
	}
	return c.transport.Delete(ctx, path+"/follow")
}
