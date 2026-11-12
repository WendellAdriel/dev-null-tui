package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"unicode/utf8"
)

// MinIssueBody mirrors the server's `CreateIssueRequest::MIN_BODY_LENGTH`: an issue needs enough to act on.
const MinIssueBody = 3

// maxEnvironmentValue is the server's limit on each environment field.
const maxEnvironmentValue = 100

// NewIssue is a /feature or /bug report. Environment holds what the client knows, and empty fields are left out.
type NewIssue struct {
	Command     string
	Body        string
	Environment map[string]string
}

// OpenedIssue is the GitHub issue the server opened.
type OpenedIssue struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

func (i NewIssue) Validate() error {
	if i.Command != "/feature" && i.Command != "/bug" {
		return errors.New("issues are /feature or /bug")
	}
	count := utf8.RuneCountInString(NormalizeBody(i.Body))
	switch {
	case count < MinIssueBody:
		return fmt.Errorf("write at least %d characters", MinIssueBody)
	case count > MaxBodyLength:
		return fmt.Errorf("issues are limited to %d characters", MaxBodyLength)
	}
	for _, value := range i.Environment {
		if utf8.RuneCountInString(value) > maxEnvironmentValue {
			return errors.New("environment details are too long")
		}
	}
	return nil
}

// OpenIssue opens a GitHub issue as the web's /feature and /bug commands do.
func (c *Client) OpenIssue(ctx context.Context, draft NewIssue) (OpenedIssue, error) {
	if err := draft.Validate(); err != nil {
		return OpenedIssue{}, err
	}
	environment := maps.Clone(draft.Environment)
	maps.DeleteFunc(environment, func(_, value string) bool { return value == "" })
	payload, err := json.Marshal(struct {
		Command     string            `json:"command"`
		Body        string            `json:"body"`
		Environment map[string]string `json:"environment,omitempty"`
	}{draft.Command, draft.Body, environment})
	if err != nil {
		return OpenedIssue{}, err
	}
	var issue OpenedIssue
	if err := c.transport.PostJSON(ctx, "/api/v1/issues", payload, &issue); err != nil {
		return OpenedIssue{}, err
	}
	if issue.Number < 1 || issue.URL == "" {
		return OpenedIssue{}, errors.New("server returned an invalid issue")
	}
	return issue, nil
}
