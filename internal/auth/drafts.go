package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// MessageDraft is an unsent Remotes message. It is the only message text this client writes to disk, so it holds
// what the user typed and never anything received: the reply target is kept as an id only.
type MessageDraft struct {
	Body      string       `json:"body,omitempty"`
	ReplyToID int64        `json:"reply_to_id,omitempty"`
	Images    []DraftImage `json:"images,omitempty"`
}

// DraftImage is an attached image by path, read again when the draft is restored or sent.
type DraftImage struct {
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

const maxDraftsBytes = 1 << 20

var draftAccount = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

func draftsFile(account string) (string, error) {
	return accountFile("message-drafts-", account)
}

func accountFile(prefix, account string) (string, error) {
	if !draftAccount.MatchString(account) {
		return "", errors.New("invalid account for drafts")
	}
	return prefix + account + ".json", nil
}

// MessageDrafts reads one account's drafts by conversation id, or none when the file is missing or invalid.
func (s Store) MessageDrafts(account string) map[string]MessageDraft {
	name, err := draftsFile(account)
	if err != nil {
		return map[string]MessageDraft{}
	}
	data, ok := s.readState(name, maxDraftsBytes)
	drafts := map[string]MessageDraft{}
	if !ok || json.Unmarshal(data, &drafts) != nil {
		return map[string]MessageDraft{}
	}
	return drafts
}

// SaveMessageDrafts replaces one account's drafts in an owner-only file, and removes the file when none are left.
func (s Store) SaveMessageDrafts(account string, drafts map[string]MessageDraft) error {
	name, err := draftsFile(account)
	if err != nil {
		return err
	}
	if len(drafts) == 0 {
		if err := os.Remove(filepath.Join(s.Dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(drafts)
	if err != nil {
		return err
	}
	return s.replaceState(name, data)
}

// DeleteDrafts removes every account's message and commit drafts, since a disconnected or revoked machine may
// not be the user's.
func (s Store) DeleteDrafts() error {
	for _, pattern := range []string{"message-drafts-*", "commit-drafts-*"} {
		matches, err := filepath.Glob(filepath.Join(s.Dir, pattern))
		if err != nil {
			return err
		}
		for _, path := range matches {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return errors.New("cannot delete the local drafts")
			}
		}
	}
	return nil
}

// CommitDraft is an unsent commit, reply, or quote, kept like a MessageDraft: only what the user wrote, with the reply
// or quote target as an id.
type CommitDraft struct {
	// Kind is commit, reply, or quote.
	Kind     string `json:"kind"`
	TargetID string `json:"target_id,omitempty"`
	// TargetHandle is the replied or quoted commit's author, so Drafts can name them.
	TargetHandle string `json:"target_handle,omitempty"`
	// TargetExcerpt is the start of that commit's text, shown under the draft.
	TargetExcerpt string       `json:"target_excerpt,omitempty"`
	Body          string       `json:"body,omitempty"`
	Images        []DraftImage `json:"images,omitempty"`
	Poll          *DraftPoll   `json:"poll,omitempty"`
	SavedAt       time.Time    `json:"saved_at"`
}

// DraftPoll is a poll being written, with its duration in minutes.
type DraftPoll struct {
	Options  []string `json:"options"`
	Duration int      `json:"duration"`
}

// MaxCommitDrafts bounds the saved commit drafts. Saving more drops the oldest.
const MaxCommitDrafts = 50

// CommitDrafts reads one account's commit drafts by place, or none when the file is missing or invalid.
func (s Store) CommitDrafts(account string) map[string]CommitDraft {
	drafts := map[string]CommitDraft{}
	name, err := accountFile("commit-drafts-", account)
	if err != nil {
		return drafts
	}
	data, ok := s.readState(name, maxDraftsBytes)
	if !ok || json.Unmarshal(data, &drafts) != nil {
		return map[string]CommitDraft{}
	}
	return drafts
}

// SaveCommitDrafts replaces one account's commit drafts, keeping the newest MaxCommitDrafts, and removes the file
// when none are left.
func (s Store) SaveCommitDrafts(account string, drafts map[string]CommitDraft) error {
	name, err := accountFile("commit-drafts-", account)
	if err != nil {
		return err
	}
	if len(drafts) == 0 {
		if err := os.Remove(filepath.Join(s.Dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	kept := drafts
	if len(drafts) > MaxCommitDrafts {
		keys := make([]string, 0, len(drafts))
		for key := range drafts {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return drafts[keys[i]].SavedAt.After(drafts[keys[j]].SavedAt) })
		kept = make(map[string]CommitDraft, MaxCommitDrafts)
		for _, key := range keys[:MaxCommitDrafts] {
			kept[key] = drafts[key]
		}
	}
	data, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	return s.replaceState(name, data)
}
