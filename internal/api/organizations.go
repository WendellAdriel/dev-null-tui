package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

type Organization struct {
	Handle     string          `json:"handle"`
	Name       string          `json:"name"`
	AvatarURL  string          `json:"avatar_url"`
	Bio        string          `json:"bio"`
	Location   string          `json:"location"`
	Website    string          `json:"website"`
	GitHubURL  string          `json:"github_url"`
	URL        string          `json:"html_url"`
	Lane       json.RawMessage `json:"lane"`
	IsFollowed *bool           `json:"is_followed"`
	Followers  *int            `json:"followers_count"`
	Admins     []User          `json:"admins"`
}

// OrganizationPage is an organization's page and what the signed user can do on it. SettingsURL is set for members.
type OrganizationPage struct {
	Organization Organization `json:"data"`
	IsFollowing  bool         `json:"is_following"`
	CanClaim     bool         `json:"can_claim"`
	SettingsURL  *string      `json:"settings_url"`
}

type Organizations struct {
	Data []Organization `json:"data"`
	Meta struct {
		NextCursor *string `json:"next_cursor"`
	} `json:"meta"`
	Next string `json:"-"`
}

func organizationPath(handle string) (string, error) {
	if !handlePattern.MatchString(handle) {
		return "", errors.New("invalid handle")
	}
	return "/api/v1/organizations/" + handle, nil
}

func (c *Client) Organization(ctx context.Context, handle string) (OrganizationPage, error) {
	path, err := organizationPath(handle)
	if err != nil {
		return OrganizationPage{}, err
	}
	var page OrganizationPage
	if err := c.transport.GetJSON(ctx, path, &page); err != nil {
		return OrganizationPage{}, err
	}
	if !strings.EqualFold(page.Organization.Handle, handle) {
		return OrganizationPage{}, errors.New("server returned an invalid organization")
	}
	return page, nil
}

// OrganizationCommits reads the Commits tab, or with mentions the commits mentioning the organization.
func (c *Client) OrganizationCommits(ctx context.Context, handle string, mentions bool, cursor string) (CommitPage, error) {
	path, err := organizationPath(handle)
	if err != nil {
		return CommitPage{}, err
	}
	tab := "/commits"
	if mentions {
		tab = "/mentions"
	}
	path += tab
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return c.commitPage(ctx, path, query, cursor)
}

func (c *Client) SetOrganizationFollow(ctx context.Context, handle string, follow bool) error {
	path, err := organizationPath(handle)
	if err != nil {
		return err
	}
	if follow {
		return c.transport.Post(ctx, path+"/follow")
	}
	return c.transport.Delete(ctx, path+"/follow")
}

func (c *Client) ClaimOrganization(ctx context.Context, handle string) error {
	path, err := organizationPath(handle)
	if err != nil {
		return err
	}
	return c.transport.Post(ctx, path+"/claim")
}

// FollowedOrganizations lists the organizations a user follows, newest follow first, as their Following list does.
func (c *Client) FollowedOrganizations(ctx context.Context, handle, cursor string) (Organizations, error) {
	path, err := userPath(handle)
	if err != nil {
		return Organizations{}, err
	}
	path += "/following/organizations"
	if cursor != "" {
		path += "?" + url.Values{"cursor": {cursor}}.Encode()
	}
	var page Organizations
	if err := c.transport.GetJSON(ctx, path, &page); err != nil {
		return Organizations{}, err
	}
	if page.Data == nil {
		return Organizations{}, errors.New("server returned an invalid organization page")
	}
	for _, organization := range page.Data {
		if !handlePattern.MatchString(organization.Handle) {
			return Organizations{}, errors.New("server returned an invalid organization")
		}
	}
	if page.Meta.NextCursor != nil {
		page.Next = *page.Meta.NextCursor
	}
	if page.Next != "" && page.Next == cursor {
		return Organizations{}, errors.New("server repeated a pagination cursor")
	}
	return page, nil
}
