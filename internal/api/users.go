package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

var handlePattern = regexp.MustCompile(`^[0-9A-Za-z-]{1,39}$`)

// ValidHandle reports whether a handle has the server's shape, for checks before a request.
func ValidHandle(handle string) bool { return handlePattern.MatchString(handle) }

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
	Live       *LiveStream     `json:"live"`
}

type LiveStream = auth.LiveStream

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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Unlocked    bool   `json:"unlocked"`
	Progress    int    `json:"progress"`
	Target      int    `json:"target"`
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
	// BioProfiles maps each lowercase handle the bio or company mentions to its dev/null profile handle.
	BioProfiles map[string]string `json:"bio_profiles"`
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
	return c.people(ctx, path+"/"+string(list), cursor)
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

// SuggestedUser is a Who to follow suggestion, with how many people the user follows follow them.
type SuggestedUser struct {
	User
	FollowedByFollowing int `json:"followed_by_following_count"`
}

// SuggestionQuery asks for a round of suggestions. Beside a profile or a commit, the server puts the people that
// developer follows, or the thread's participants, first. The server refuses a handle and a commit together.
type SuggestionQuery struct {
	Round            int
	Handle, CommitID string
}

// SuggestedUsers is Who to follow: three people to show, then spares that take a dismissed person's place.
func (c *Client) SuggestedUsers(ctx context.Context, q SuggestionQuery) ([]SuggestedUser, error) {
	query := url.Values{}
	switch {
	case q.Handle != "" && q.CommitID != "":
		return nil, errors.New("suggestions take a handle or a commit, not both")
	case q.Handle != "" && !handlePattern.MatchString(q.Handle):
		return nil, errors.New("invalid handle")
	case q.CommitID != "" && !commitID.MatchString(q.CommitID):
		return nil, errors.New("invalid commit")
	case q.Round < 0:
		return nil, errors.New("invalid round")
	}
	if q.Handle != "" {
		query.Set("handle", q.Handle)
	}
	if q.CommitID != "" {
		query.Set("commit_id", q.CommitID)
	}
	if q.Round > 0 {
		query.Set("round", strconv.Itoa(q.Round))
	}
	path := "/api/v1/me/suggested-users"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var result struct {
		Data []SuggestedUser `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	if result.Data == nil {
		return nil, errors.New("server returned invalid suggestions")
	}
	for _, person := range result.Data {
		if person.ID == "" || !handlePattern.MatchString(person.Handle) {
			return nil, errors.New("server returned an invalid person")
		}
	}
	return result.Data, nil
}

// DismissSuggestion is Not interested: the person is never suggested again, and nothing else changes.
func (c *Client) DismissSuggestion(ctx context.Context, handle string) error {
	path, err := userPath(handle)
	if err != nil {
		return err
	}
	return c.transport.Post(ctx, path+"/dismiss")
}
