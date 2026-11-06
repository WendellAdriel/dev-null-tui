package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const (
	MaxSearchTerm        = 100
	MinCommitSearch      = 3
	MaxCommitSearchLimit = 100
)

var issueNumber = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// Profiles are the people and organizations a search finds, people first, as the web lists them.
type Profiles struct {
	Users         []User         `json:"users"`
	Organizations []Organization `json:"organizations"`
}

// MentionQuery scopes mention suggestions: Org to a Remotes org's members, or Reply or Quote to that commit's thread.
type MentionQuery struct {
	Term, Org, Reply, Quote string
}

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

func searchTerm(term string) (string, error) {
	term = trimInput(term)
	if utf8.RuneCountInString(term) > MaxSearchTerm {
		return "", fmt.Errorf("searches are limited to %d characters", MaxSearchTerm)
	}
	return term, nil
}

func validProfiles(found Profiles) bool {
	if found.Users == nil || found.Organizations == nil {
		return false
	}
	for _, user := range found.Users {
		if !handlePattern.MatchString(user.Handle) {
			return false
		}
	}
	for _, organization := range found.Organizations {
		if !handlePattern.MatchString(organization.Handle) {
			return false
		}
	}
	return true
}

func (c *Client) SearchProfiles(ctx context.Context, term string) (Profiles, error) {
	term, err := searchTerm(term)
	if err != nil {
		return Profiles{}, err
	}
	var found Profiles
	if err := c.transport.GetJSON(ctx, "/api/v1/search/profiles?"+url.Values{"q": {term}}.Encode(), &found); err != nil {
		return Profiles{}, err
	}
	if !validProfiles(found) {
		return Profiles{}, errors.New("server returned invalid profiles")
	}
	return found, nil
}

// SearchCommits finds commits by meaning, closest first. The server may return fewer than limit.
func (c *Client) SearchCommits(ctx context.Context, term string, limit int) ([]Commit, error) {
	term, err := searchTerm(term)
	switch {
	case err != nil:
		return nil, err
	case utf8.RuneCountInString(term) < MinCommitSearch:
		return nil, fmt.Errorf("commit searches need at least %d characters", MinCommitSearch)
	case limit < 1 || limit > MaxCommitSearchLimit:
		return nil, errors.New("invalid commit search limit")
	}
	query := url.Values{"q": {term}, "limit": {strconv.Itoa(limit)}}
	var result struct {
		Data []Commit `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/search/commits?"+query.Encode(), &result); err != nil {
		return nil, err
	}
	if result.Data == nil {
		return nil, errors.New("server returned invalid commits")
	}
	for _, commit := range result.Data {
		if !validCommit(commit) {
			return nil, errors.New("server returned an invalid commit")
		}
	}
	return result.Data, nil
}

func (c *Client) MentionSuggestions(ctx context.Context, search MentionQuery) (Profiles, error) {
	term, err := searchTerm(search.Term)
	switch {
	case err != nil:
		return Profiles{}, err
	case search.Org != "" && (search.Reply != "" || search.Quote != ""), search.Reply != "" && search.Quote != "":
		return Profiles{}, errors.New("mention suggestions take one scope")
	case search.Org != "" && !orgID.MatchString(search.Org):
		return Profiles{}, errors.New("invalid conversation")
	case search.Reply != "" && !commitID.MatchString(search.Reply), search.Quote != "" && !commitID.MatchString(search.Quote):
		return Profiles{}, errors.New("invalid commit id")
	}
	query := url.Values{"q": {term}}
	for name, value := range map[string]string{"org": search.Org, "reply": search.Reply, "quote": search.Quote} {
		if value != "" {
			query.Set(name, value)
		}
	}
	var result struct {
		Users         []User         `json:"data"`
		Organizations []Organization `json:"organizations"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/search/mentions?"+query.Encode(), &result); err != nil {
		return Profiles{}, err
	}
	found := Profiles(result)
	if !validProfiles(found) {
		return Profiles{}, errors.New("server returned invalid suggestions")
	}
	return found, nil
}

// IssueSuggestion returns the /dev/null issue or pull request with that number, or nil when there is none.
func (c *Client) IssueSuggestion(ctx context.Context, number string) (*Issue, error) {
	if !issueNumber.MatchString(number) {
		return nil, errors.New("invalid issue number")
	}
	var result struct {
		Issue *Issue `json:"issue"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/search/issues?"+url.Values{"q": {number}}.Encode(), &result); err != nil {
		return nil, err
	}
	if result.Issue != nil && (strconv.Itoa(result.Issue.Number) != number || result.Issue.URL == "") {
		return nil, errors.New("server returned another issue")
	}
	return result.Issue, nil
}
