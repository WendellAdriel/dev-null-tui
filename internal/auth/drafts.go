package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
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
	if !draftAccount.MatchString(account) {
		return "", errors.New("invalid account for message drafts")
	}
	return "message-drafts-" + account + ".json", nil
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

// DeleteMessageDrafts removes every account's drafts, since a disconnected or revoked machine may not be the user's.
func (s Store) DeleteMessageDrafts() error {
	matches, err := filepath.Glob(filepath.Join(s.Dir, "message-drafts-*"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errors.New("cannot delete the local message drafts")
		}
	}
	return nil
}
