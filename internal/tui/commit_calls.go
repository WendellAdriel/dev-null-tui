package tui

import (
	"context"
	"time"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

// commitCalls are the commit extras and scheduling routes, which tests replace.
type commitCalls struct {
	related     func(context.Context, string) ([]api.Commit, error)
	stargazers  func(context.Context, string, string) (api.People, error)
	pin         func(context.Context, string, bool) (api.Commit, error)
	restore     func(context.Context, string) (api.Commit, error)
	translate   func(context.Context, string, string) (api.Translation, error)
	settings    func(context.Context) (api.AccountSettings, error)
	schedule    func(context.Context, api.NewCommit, time.Time) (api.ScheduledCommit, error)
	scheduled   func(context.Context, int) (api.ScheduledPage, error)
	cancelLater func(context.Context, string) error
	submitNow   func(context.Context, string) (api.ScheduledCommit, error)
}

func newCommitCalls(client *api.Client) commitCalls {
	return commitCalls{client.RelatedCommits, client.Stargazers, client.SetPin, client.RestoreCommit, client.Translate, client.AccountSettings, client.ScheduleCommit, client.ScheduledCommits, client.CancelScheduled, client.SubmitScheduled}
}
