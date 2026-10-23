package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

type Log string

const (
	Following Log = "following"
	Latest    Log = "latest"
	Trending  Log = "trending"
)

type Author struct {
	ID        string          `json:"id"`
	Username  string          `json:"username"`
	Name      string          `json:"name"`
	Lane      json.RawMessage `json:"lane"`
	AvatarURL string          `json:"avatar_url"`
}

type Segment struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Code     string `json:"code"`
	Language string `json:"language"`
	HTML     string `json:"html"`
}

type Commit struct {
	ID           string          `json:"id"`
	Hash         string          `json:"short_hash"`
	Body         string          `json:"body"`
	URL          string          `json:"html_url"`
	CreatedAt    time.Time       `json:"created_at"`
	DeletedAt    *time.Time      `json:"deleted_at"`
	Author       *Author         `json:"author"`
	Status       string          `json:"status"`
	Stars        int             `json:"stars_count"`
	Replies      int             `json:"replies_count"`
	Segments     []Segment       `json:"body_segments"`
	Preview      []Segment       `json:"preview_segments"`
	QuotedID     *string         `json:"quoted_commit_id"`
	Quote        *Commit         `json:"quoted_commit"`
	Images       []Image         `json:"images"`
	Poll         json.RawMessage `json:"poll"`
	ReplyPreview *struct {
		Data []Commit `json:"data"`
	} `json:"reply_preview"`
}

type Image struct {
	URL         string `json:"url"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Description string `json:"description"`
}

type Page struct {
	Data        []Commit `json:"data"`
	Suggestions []Commit `json:"suggestions"`
	Log         Log      `json:"log"`
	Ranked      bool     `json:"is_ranked"`
	NewCount    int      `json:"new_commits_count"`
	Links       struct {
		Next *string `json:"next"`
	} `json:"links"`
	Next string `json:"-"`
}

type Client struct {
	transport *auth.Client
	origin    *url.URL
}

func NewClient(origin string, key *auth.Key) (*Client, error) {
	transport, err := auth.NewClient(origin, key)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	return &Client{transport: transport, origin: u}, nil
}

func (c *Client) pageURL(log Log, target string) (string, error) {
	if log != Following && log != Latest && log != Trending {
		return "", errors.New("unknown log")
	}
	path := "/api/v1/log/" + string(log)
	if target == "" {
		return path, nil
	}
	u, err := c.origin.Parse(target)
	if err != nil || u.Scheme != c.origin.Scheme || u.Host != c.origin.Host || u.User != nil || u.Fragment != "" || u.Path != path || u.RawPath != "" {
		return "", errors.New("refusing an unsafe pagination link")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query["cursor"]) != 1 || query.Get("cursor") == "" {
		return "", errors.New("invalid pagination cursor")
	}
	for name := range query {
		if name != "cursor" && name != "since" {
			return "", errors.New("invalid pagination parameters")
		}
	}
	return u.RequestURI(), nil
}

func (c *Client) Log(ctx context.Context, log Log, next, since string) (Page, error) {
	target, err := c.pageURL(log, next)
	if err != nil {
		return Page{}, err
	}
	if next == "" && since != "" && log != Trending {
		target += "?since=" + url.QueryEscape(since)
	}
	var page Page
	if err := c.transport.GetJSON(ctx, target, &page); err != nil {
		return Page{}, err
	}
	if page.Log != log || page.Ranked != (log == Trending) || page.Data == nil {
		return Page{}, errors.New("server returned an invalid log")
	}
	for _, commit := range append(append([]Commit(nil), page.Data...), page.Suggestions...) {
		if commit.ID == "" || (commit.DeletedAt == nil && commit.Author == nil) {
			return Page{}, errors.New("server returned an invalid commit")
		}
	}
	if page.Links.Next != nil && *page.Links.Next != "" {
		page.Next, err = c.pageURL(log, *page.Links.Next)
		if err != nil {
			return Page{}, err
		}
		if page.Next == target {
			return Page{}, errors.New("server repeated a pagination cursor")
		}
	}
	if log == Trending {
		page.NewCount = 0
	}
	return page, nil
}
