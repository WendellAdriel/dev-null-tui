package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MinPollOptions      = 2
	MaxPollOptions      = 4
	MaxPollOptionLength = 50
)

// PollDurations are the minutes the server accepts, from one hour to seven days.
var PollDurations = []int{60, 1440, 4320, 10080}

// Poll counts, totals, and percentages are null until the user voted, wrote the poll, or it closed.
type Poll struct {
	ID         string       `json:"id"`
	ClosesAt   *time.Time   `json:"closes_at"`
	Closed     bool         `json:"is_closed"`
	HasVoted   bool         `json:"has_voted"`
	TotalVotes *int         `json:"total_votes"`
	Options    []PollOption `json:"options"`
}

type PollOption struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Votes      *int   `json:"votes_count"`
	Voted      bool   `json:"voted"`
	Percentage *int   `json:"percentage"`
}

func (p *Poll) Valid() bool {
	if p == nil || p.ID == "" || len(p.Options) < MinPollOptions || len(p.Options) > MaxPollOptions {
		return false
	}
	for _, option := range p.Options {
		if option.ID == "" {
			return false
		}
	}
	return true
}

type NewPoll struct {
	Options  []string `json:"options"`
	Duration int      `json:"duration"`
}

// trimmed matches the server, which trims every field before validating it.
func (p NewPoll) trimmed() NewPoll {
	options := make([]string, len(p.Options))
	for i, option := range p.Options {
		options[i] = trimInput(option)
	}
	return NewPoll{Options: options, Duration: p.Duration}
}

func ValidatePoll(poll NewPoll) error {
	poll = poll.trimmed()
	if len(poll.Options) < MinPollOptions || len(poll.Options) > MaxPollOptions {
		return fmt.Errorf("a poll needs %d to %d options", MinPollOptions, MaxPollOptions)
	}
	seen := map[string]bool{}
	for i, option := range poll.Options {
		if option == "" {
			return fmt.Errorf("option %d is empty", i+1)
		}
		if utf8.RuneCountInString(option) > MaxPollOptionLength {
			return fmt.Errorf("option %d is longer than %d characters", i+1, MaxPollOptionLength)
		}
		if seen[strings.ToLower(option)] {
			return fmt.Errorf("option %d repeats another option", i+1)
		}
		seen[strings.ToLower(option)] = true
	}
	if !slices.Contains(PollDurations, poll.Duration) {
		return errors.New("choose a poll duration of 1 hour, 1 day, 3 days, or 7 days")
	}
	return nil
}

// Vote is final. The server answers with the commit, so the poll comes back with its results.
func (c *Client) Vote(ctx context.Context, commitID, optionID string) (Commit, error) {
	path, err := commitPath(commitID)
	if err != nil {
		return Commit{}, err
	}
	payload, err := json.Marshal(struct {
		Option string `json:"option"`
	}{optionID})
	if err != nil {
		return Commit{}, err
	}
	var result struct {
		Data Commit `json:"data"`
	}
	if err := c.transport.PostJSON(ctx, path+"/poll/votes", payload, &result); err != nil {
		return Commit{}, err
	}
	commit := result.Data
	if commit.ID != commitID || !validCommit(commit) || !commit.Poll.Valid() || !commit.Poll.HasVoted || !slices.ContainsFunc(commit.Poll.Options, func(o PollOption) bool { return o.ID == optionID && o.Voted }) {
		return Commit{}, errors.New("server returned an invalid vote confirmation")
	}
	return commit, nil
}
