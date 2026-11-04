package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// MaxMessageLength is the server's Message::MAX_LENGTH, counted in characters after trimming.
const MaxMessageLength = 2000

// NewMessage is a message to push. ClientID stays the same across retries, so the server answers a resend with the
// message it already created instead of a duplicate.
type NewMessage struct {
	Body      string
	ClientID  string
	ReplyToID int64
	Images    []ImageFile
}

// EmojiUse is how often the user reacted with an emoji, which ranks the quick reactions and the picker.
type EmojiUse struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

func MessageLength(body string) int { return utf8.RuneCountInString(trimInput(body)) }

func (draft NewMessage) Validate() error {
	if draft.ClientID == "" {
		return errors.New("a message needs a client id")
	}
	if len(draft.Images) > MaxImages {
		return fmt.Errorf("attach at most %d images", MaxImages)
	}
	for _, image := range draft.Images {
		if err := validDescription(image.Description); err != nil {
			return err
		}
	}
	length := MessageLength(draft.Body)
	if length == 0 && len(draft.Images) == 0 {
		return errors.New("write a message first")
	}
	if length > MaxMessageLength {
		return fmt.Errorf("messages are limited to %d characters", MaxMessageLength)
	}
	return nil
}

func (draft NewMessage) payload() ([]byte, error) {
	payload := struct {
		Body         string    `json:"body"`
		ClientID     string    `json:"client_id"`
		ReplyToID    int64     `json:"reply_to_id,omitempty"`
		Descriptions []*string `json:"image_descriptions,omitempty"`
	}{Body: trimInput(draft.Body), ClientID: draft.ClientID, ReplyToID: draft.ReplyToID}
	for _, image := range draft.Images {
		description := trimInput(image.Description)
		if description == "" {
			payload.Descriptions = append(payload.Descriptions, nil)
			continue
		}
		payload.Descriptions = append(payload.Descriptions, &description)
	}
	return json.Marshal(payload)
}

// PushMessage sends a message, with its images as multipart. It leaves out link_preview_url, so the server gives the
// first link its card.
func (c *Client) PushMessage(ctx context.Context, target Target, draft NewMessage) (Message, error) {
	path, err := target.path()
	if err != nil {
		return Message{}, err
	}
	if err := draft.Validate(); err != nil {
		return Message{}, err
	}
	payload, err := draft.payload()
	if err != nil {
		return Message{}, err
	}
	var result struct {
		Data Message `json:"data"`
	}
	if len(draft.Images) > 0 {
		uploads := make([]auth.Upload, len(draft.Images))
		for i, image := range draft.Images {
			uploads[i] = image.Upload
		}
		err = c.transport.PostMultipart(ctx, path+"/messages", payload, uploads, &result)
	} else {
		err = c.transport.PostJSON(ctx, path+"/messages", payload, &result)
	}
	if err != nil {
		return Message{}, err
	}
	if !validMessage(result.Data) {
		return Message{}, errors.New("server returned an invalid message")
	}
	return result.Data, nil
}

// SendTyping tells the other side the user is typing for the next few seconds.
func (c *Client) SendTyping(ctx context.Context, target Target) error {
	path, err := target.path()
	if err != nil {
		return err
	}
	return c.transport.NoContent(ctx, http.MethodPut, path+"/typing", nil)
}

// Ping sends a ping to a conversation that has started, and returns the ping's event message.
func (c *Client) Ping(ctx context.Context, target Target) (Message, error) {
	path, err := target.path()
	if err != nil {
		return Message{}, err
	}
	var result struct {
		Data Message `json:"data"`
	}
	if err := c.transport.PostJSON(ctx, path+"/ping", nil, &result); err != nil {
		return Message{}, err
	}
	if !validMessage(result.Data) {
		return Message{}, errors.New("server returned an invalid message")
	}
	return result.Data, nil
}

// SetReaction adds or removes one of the user's reactions and returns the message's reactions afterwards.
func (c *Client) SetReaction(ctx context.Context, id int64, emoji string, on bool) ([]Reaction, error) {
	if id <= 0 || emoji == "" {
		return nil, errors.New("invalid reaction")
	}
	method := http.MethodDelete
	if on {
		method = http.MethodPost
	}
	body, _ := json.Marshal(map[string]string{"emoji": emoji})
	var result struct {
		Reactions []Reaction `json:"reactions"`
	}
	if err := c.transport.SendJSON(ctx, method, "/api/v1/remotes/messages/"+strconv.FormatInt(id, 10)+"/reactions", body, &result); err != nil {
		return nil, err
	}
	if result.Reactions == nil {
		return nil, errors.New("server returned invalid reactions")
	}
	return result.Reactions, nil
}

func (c *Client) ReactionUsage(ctx context.Context) ([]EmojiUse, error) {
	var result struct {
		Usage []EmojiUse `json:"usage"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/remotes/reactions", &result); err != nil {
		return nil, err
	}
	return result.Usage, nil
}
