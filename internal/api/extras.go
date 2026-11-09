package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
)

// RelatedLimit is the server's RelatedCommitsQuery::LIMIT.
const RelatedLimit = 3

func (c *Client) RelatedCommits(ctx context.Context, id string) ([]Commit, error) {
	path, err := commitPath(id)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []Commit `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, path+"/related", &result); err != nil {
		return nil, err
	}
	if result.Data == nil || len(result.Data) > RelatedLimit {
		return nil, errors.New("server returned invalid related commits")
	}
	for _, commit := range result.Data {
		if commit.ID == id || commit.Author == nil || !validCommit(commit) {
			return nil, errors.New("server returned an invalid commit")
		}
	}
	return result.Data, nil
}

func (c *Client) Stargazers(ctx context.Context, id, cursor string) (People, error) {
	path, err := commitPath(id)
	if err != nil {
		return People{}, err
	}
	return c.people(ctx, path+"/stars", cursor)
}

func (c *Client) people(ctx context.Context, path, cursor string) (People, error) {
	if cursor != "" {
		path += "?" + url.Values{"cursor": {cursor}}.Encode()
	}
	var people People
	if err := c.transport.GetJSON(ctx, path, &people); err != nil {
		return People{}, err
	}
	if people.Data == nil {
		return People{}, errors.New("server returned an invalid people page")
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

func (c *Client) SetPin(ctx context.Context, id string, pinned bool) (Commit, error) {
	path, err := commitPath(id)
	if err != nil {
		return Commit{}, err
	}
	var result struct {
		Commit Commit `json:"data"`
	}
	if pinned {
		err = c.transport.PutJSON(ctx, path+"/pin", nil, &result)
	} else {
		err = c.transport.DeleteJSON(ctx, path+"/pin", &result)
	}
	if err != nil {
		return Commit{}, err
	}
	if result.Commit.ID != id || !validCommit(result.Commit) || result.Commit.Pinned == nil || *result.Commit.Pinned != pinned {
		return Commit{}, errors.New("server returned an invalid pin confirmation")
	}
	return result.Commit, nil
}

func (c *Client) RestoreCommit(ctx context.Context, id string) (Commit, error) {
	path, err := commitPath(id)
	if err != nil {
		return Commit{}, err
	}
	var result struct {
		Commit Commit `json:"data"`
	}
	if err := c.transport.PostJSON(ctx, path+"/restoration", nil, &result); err != nil {
		return Commit{}, err
	}
	if result.Commit.ID != id || result.Commit.DeletedAt != nil || result.Commit.Author == nil {
		return Commit{}, errors.New("server returned an invalid restored commit")
	}
	return result.Commit, nil
}

// ErrNoTranslation means the server offers no translation, as for a deleted, rejected, or empty commit.
var ErrNoTranslation = errors.New("this commit can't be translated")

type Translation struct {
	Language       string `json:"language"`
	SourceLanguage string `json:"source_language"`
	SourceName     string `json:"source_name"`
	Available      bool   `json:"available"`
	// Translated is nil while the server is still translating.
	Translated *struct {
		Segments []Segment `json:"body_segments"`
		Preview  []Segment `json:"preview_segments"`
	} `json:"translated"`
}

func (c *Client) Translate(ctx context.Context, id, language string) (Translation, error) {
	path, err := commitPath(id)
	if err != nil {
		return Translation{}, err
	}
	payload, err := json.Marshal(struct {
		Language string `json:"language"`
	}{language})
	if err != nil {
		return Translation{}, err
	}
	var result struct {
		Translation *Translation `json:"translation"`
	}
	if err := c.transport.PostJSON(ctx, path+"/translation", payload, &result); err != nil {
		return Translation{}, err
	}
	t := result.Translation
	if t == nil {
		return Translation{}, ErrNoTranslation
	}
	if t.Language != language || (t.Translated != nil && len(t.Translated.Segments) == 0) {
		return Translation{}, errors.New("server returned an invalid translation")
	}
	return *t, nil
}
