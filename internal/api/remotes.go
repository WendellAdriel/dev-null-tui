package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var orgID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type InboxTab string

const (
	Open    InboxTab = "open"
	Fetched InboxTab = "fetched"
	Stashed InboxTab = "stashed"
)

type MuteDuration string

const (
	MuteHour     MuteDuration = "hour"
	MuteTomorrow MuteDuration = "tomorrow"
	MuteWeek     MuteDuration = "week"
	MuteForever  MuteDuration = "forever"
	MuteOff      MuteDuration = "off"
)

// Target is the other end of a conversation: a partner's handle, or an org's UUID.
type Target struct {
	Handle string
	Org    string
}

func (t Target) path() (string, error) {
	switch {
	case t.Handle != "" && t.Org == "" && handlePattern.MatchString(t.Handle):
		return "/api/v1/remotes/@" + t.Handle, nil
	case t.Org != "" && t.Handle == "" && orgID.MatchString(t.Org):
		return "/api/v1/remotes/orgs/" + t.Org, nil
	}
	return "", errors.New("invalid conversation")
}

type LinkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	SiteName    string `json:"site_name"`
	Pending     bool   `json:"is_pending"`
}

type Mention struct {
	Handle string `json:"handle"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Kind   string `json:"kind"`
}

type Reaction struct {
	UserID string `json:"user_id"`
	Emoji  string `json:"emoji"`
}

type MessageImage struct {
	ID          string `json:"id"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type Voice struct {
	ID         string `json:"id"`
	DurationMS int    `json:"duration_ms"`
	Waveform   []int  `json:"waveform"`
	URL        string `json:"url"`
}

type ReplyTo struct {
	ID          int64  `json:"id"`
	UserID      string `json:"user_id"`
	Body        string `json:"body"`
	ImagesCount int    `json:"images_count"`
	HasVoice    bool   `json:"has_voice"`
}

type Message struct {
	ID          int64          `json:"id"`
	UserID      string         `json:"user_id"`
	ClientID    string         `json:"client_id"`
	Body        string         `json:"body"`
	Segments    []Segment      `json:"body_segments"`
	Mentions    []Mention      `json:"mentions"`
	ReplyToID   *int64         `json:"reply_to_id"`
	ReplyTo     *ReplyTo       `json:"reply_to"`
	LinkPreview *LinkPreview   `json:"link_preview"`
	Images      []MessageImage `json:"images"`
	Voice       *Voice         `json:"voice"`
	Reactions   []Reaction     `json:"reactions"`
	Event       string         `json:"event"`
	SubjectID   string         `json:"subject_id"`
	User        *User          `json:"user"`
	Subject     *User          `json:"subject"`
	PinnedAt    *time.Time     `json:"pinned_at"`
	CreatedAt   time.Time      `json:"created_at"`
}

type Member struct {
	User          User   `json:"user"`
	Role          string `json:"role"`
	ReadThroughID int64  `json:"read_through_id"`
}

type Conversation struct {
	ID                    string     `json:"id"`
	Kind                  string     `json:"kind"`
	Name                  *string    `json:"name"`
	Partner               *User      `json:"partner"`
	Members               []Member   `json:"members"`
	LastMessage           *Message   `json:"last_message"`
	LastMessageAt         *time.Time `json:"last_message_at"`
	Ahead                 int        `json:"ahead"`
	LastIncomingMessageID *int64     `json:"last_incoming_message_id"`
	LastIncomingSender    *string    `json:"last_incoming_sender"`
	PartnerReadThroughID  int64      `json:"partner_read_through_id"`
	PartnerReadAt         *time.Time `json:"partner_read_at"`
	IsArchived            bool       `json:"is_archived"`
	PinnedAt              *time.Time `json:"pinned_at"`
	MutedUntilAt          *time.Time `json:"muted_until_at"`
}

func (c Conversation) Target() Target {
	if c.Kind == "org" {
		return Target{Org: c.ID}
	}
	return Target{Handle: c.Partner.Handle}
}

type Inbox struct {
	Data          []Conversation `json:"data"`
	HasMore       bool           `json:"has_more"`
	UnreadCount   int            `json:"unread_count"`
	RequestsCount int            `json:"requests_count"`
}

type ReactionChange struct {
	ID        int64      `json:"id"`
	Reactions []Reaction `json:"reactions"`
}

type Org struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// RemoteThread is one page of a direct or org conversation. Partner is set for direct threads, Org for org threads.
type RemoteThread struct {
	Partner              *User            `json:"partner"`
	Org                  *Org             `json:"org"`
	Messages             []Message        `json:"messages"`
	Pinned               []Message        `json:"pinned"`
	Members              []Member         `json:"members"`
	Typing               []string         `json:"typing"`
	HasOlder             bool             `json:"has_older"`
	ReactionsVersion     int64            `json:"reactions_version"`
	ReactionChanges      []ReactionChange `json:"reaction_changes"`
	ReadThroughID        int64            `json:"read_through_id"`
	PartnerReadThroughID int64            `json:"partner_read_through_id"`
	PartnerReadAt        *time.Time       `json:"partner_read_at"`
	PartnerTyping        bool             `json:"partner_typing"`
	IsArchived           bool             `json:"is_archived"`
	PinnedAt             *time.Time       `json:"pinned_at"`
	MutedUntilAt         *time.Time       `json:"muted_until_at"`
	Role                 string           `json:"role"`
	Access               string           `json:"access"`
}

// ThreadQuery pages a thread: Before loads older messages, After polls for newer ones, and ReactionsAfter asks for
// reaction changes since a version. Zero leaves a field out.
type ThreadQuery struct {
	Before, After, ReactionsAfter int64
}

func validConversation(c Conversation) bool {
	switch c.Kind {
	case "direct":
		return c.ID != "" && c.Partner != nil && handlePattern.MatchString(c.Partner.Handle)
	case "org":
		return orgID.MatchString(c.ID) && c.Members != nil
	}
	return false
}

func validMessage(m Message) bool {
	return m.ID > 0 && !m.CreatedAt.IsZero()
}

func (c *Client) Inbox(ctx context.Context, tab InboxTab, term, before string) (Inbox, error) {
	if tab != Open && tab != Fetched && tab != Stashed {
		return Inbox{}, errors.New("unknown inbox tab")
	}
	query := url.Values{"tab": {string(tab)}}
	if term = strings.TrimSpace(term); term != "" {
		query.Set("q", term)
	}
	if before != "" {
		query.Set("before", before)
	}
	var inbox Inbox
	if err := c.transport.GetJSON(ctx, "/api/v1/remotes?"+query.Encode(), &inbox); err != nil {
		return Inbox{}, err
	}
	if inbox.Data == nil {
		return Inbox{}, errors.New("server returned an invalid inbox")
	}
	for _, conversation := range inbox.Data {
		if !validConversation(conversation) || (conversation.LastMessage != nil && !validMessage(*conversation.LastMessage)) {
			return Inbox{}, errors.New("server returned an invalid conversation")
		}
	}
	return inbox, nil
}

func (c *Client) RemoteThread(ctx context.Context, target Target, page ThreadQuery) (RemoteThread, error) {
	path, err := target.path()
	if err != nil {
		return RemoteThread{}, err
	}
	query := url.Values{}
	for name, value := range map[string]int64{"before": page.Before, "after": page.After, "reactions_after": page.ReactionsAfter} {
		if value > 0 {
			query.Set(name, strconv.FormatInt(value, 10))
		}
	}
	path += "/messages"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var result struct {
		Data RemoteThread `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, path, &result); err != nil {
		return RemoteThread{}, err
	}
	thread := result.Data
	matches := (target.Handle != "" && thread.Partner != nil && strings.EqualFold(thread.Partner.Handle, target.Handle)) ||
		(target.Org != "" && thread.Org != nil && strings.EqualFold(thread.Org.ID, target.Org))
	if !matches || thread.Messages == nil || !validAccess(thread.Access) {
		return RemoteThread{}, errors.New("server returned an invalid conversation")
	}
	for _, list := range [][]Message{thread.Messages, thread.Pinned} {
		for _, message := range list {
			if !validMessage(message) {
				return RemoteThread{}, errors.New("server returned an invalid message")
			}
		}
	}
	return thread, nil
}

func validAccess(access string) bool {
	switch access {
	case "open", "request", "waiting", "restricted", "closed":
		return true
	}
	return false
}

// MarkRead moves the read receipt forward to a message. The server never moves it back.
func (c *Client) MarkRead(ctx context.Context, target Target, through int64) error {
	path, err := target.path()
	if err != nil || through <= 0 {
		return errors.New("invalid read receipt")
	}
	body, _ := json.Marshal(map[string]int64{"through": through})
	return c.transport.NoContent(ctx, http.MethodPut, path+"/read", body)
}

// SetStashed stashes a conversation, or pops it back to Open.
func (c *Client) SetStashed(ctx context.Context, target Target, stashed bool) error {
	path, err := target.path()
	if err != nil {
		return err
	}
	method := http.MethodDelete
	if stashed {
		method = http.MethodPost
	}
	return c.transport.NoContent(ctx, method, path+"/archive", nil)
}

func (c *Client) SetMute(ctx context.Context, target Target, duration MuteDuration) (*time.Time, error) {
	path, err := target.path()
	if err != nil {
		return nil, err
	}
	switch duration {
	case MuteHour, MuteTomorrow, MuteWeek, MuteForever, MuteOff:
	default:
		return nil, errors.New("unknown mute duration")
	}
	body, _ := json.Marshal(map[string]MuteDuration{"duration": duration})
	var result struct {
		Until *time.Time `json:"muted_until_at"`
	}
	if err := c.transport.PutJSON(ctx, path+"/mute", body, &result); err != nil {
		return nil, err
	}
	return result.Until, nil
}

func (c *Client) SetConversationPin(ctx context.Context, target Target, pinned bool) (*time.Time, error) {
	path, err := target.path()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]bool{"pinned": pinned})
	var result struct {
		PinnedAt *time.Time `json:"pinned_at"`
	}
	if err := c.transport.PutJSON(ctx, path+"/pin", body, &result); err != nil {
		return nil, err
	}
	return result.PinnedAt, nil
}

// LeaveOrg removes the user from an org, which takes it out of their inbox.
func (c *Client) LeaveOrg(ctx context.Context, org, handle string) error {
	path, err := Target{Org: org}.path()
	if err != nil || !handlePattern.MatchString(handle) {
		return errors.New("invalid org member")
	}
	return c.transport.NoContent(ctx, http.MethodDelete, path+"/members/"+handle, nil)
}

func (c *Client) MessageLinkPreview(ctx context.Context, id int64) (*LinkPreview, error) {
	if id <= 0 {
		return nil, errors.New("invalid message id")
	}
	var result struct {
		Preview *LinkPreview `json:"link_preview"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/remotes/messages/"+strconv.FormatInt(id, 10)+"/link-preview", &result); err != nil {
		return nil, err
	}
	return result.Preview, nil
}
