package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

// RemoveOrgMember takes someone out of an org. Removing the user themself leaves it, which takes it out of their inbox.
func (c *Client) RemoveOrgMember(ctx context.Context, org, handle string) error {
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

// Merge accepts a pending request, which opens the conversation for both sides.
func (c *Client) Merge(ctx context.Context, handle string) error {
	return c.answerRequest(ctx, handle, "merge")
}

// Drop declines a pending request without telling the sender.
func (c *Client) Drop(ctx context.Context, handle string) error {
	return c.answerRequest(ctx, handle, "drop")
}

func (c *Client) answerRequest(ctx context.Context, handle, answer string) error {
	path, err := Target{Handle: handle}.path()
	if err != nil {
		return err
	}
	return c.transport.NoContent(ctx, http.MethodPost, path+"/"+answer, nil)
}

// CandidatesFor says what a candidate search is for, which changes the reasons a person can't be picked.
type CandidatesFor string

const (
	ForRemote CandidatesFor = "remote"
	ForOrg    CandidatesFor = "org"
	ForShare  CandidatesFor = "share"
)

// The server's limits for an org: a name, its members, a candidate search, and pinned messages.
const (
	MaxOrgName       = 60
	MaxOrgMembers    = 100
	MaxCandidateTerm = 100
	MaxPinned        = 25
)

type CandidateQuery struct {
	Term string
	Org  string
	For  CandidatesFor
}

// Candidate is a person the user can push to or add. Reason says why they can't be picked, and is empty when they can.
type Candidate struct {
	User   User   `json:"user"`
	Reason string `json:"reason"`
}

func (c *Client) Candidates(ctx context.Context, search CandidateQuery) ([]Candidate, error) {
	term := trimInput(search.Term)
	switch {
	case utf8.RuneCountInString(term) > MaxCandidateTerm:
		return nil, fmt.Errorf("searches are limited to %d characters", MaxCandidateTerm)
	case search.Org != "" && !orgID.MatchString(search.Org):
		return nil, errors.New("invalid conversation")
	case search.For != "" && search.For != ForRemote && search.For != ForOrg && search.For != ForShare:
		return nil, errors.New("unknown candidate search")
	}
	query := url.Values{}
	for name, value := range map[string]string{"q": term, "org": search.Org, "for": string(search.For)} {
		if value != "" {
			query.Set(name, value)
		}
	}
	path := "/api/v1/remotes/candidates"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var result struct {
		Candidates []Candidate `json:"candidates"`
	}
	if err := c.transport.GetJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	if result.Candidates == nil {
		return nil, errors.New("server returned invalid candidates")
	}
	for _, candidate := range result.Candidates {
		if !handlePattern.MatchString(candidate.User.Handle) {
			return nil, errors.New("server returned invalid candidates")
		}
	}
	return result.Candidates, nil
}

// CreateOrg starts an org the user maintains, with everyone else as a contributor, and returns its ID.
func (c *Client) CreateOrg(ctx context.Context, name string, members []string) (string, error) {
	title, err := orgName(name)
	if err != nil {
		return "", err
	}
	if err := validMembers(members); err != nil {
		return "", err
	}
	body, _ := json.Marshal(struct {
		Name    *string  `json:"name"`
		Members []string `json:"members"`
	}{title, members})
	var result struct {
		Org struct {
			ID string `json:"id"`
		} `json:"org"`
	}
	if err := c.transport.PostJSON(ctx, "/api/v1/remotes/orgs", body, &result); err != nil {
		return "", err
	}
	if !orgID.MatchString(result.Org.ID) {
		return "", errors.New("server returned an invalid org")
	}
	return result.Org.ID, nil
}

// RenameOrg sets an org's name. An empty name takes it away, so the org goes by its members.
func (c *Client) RenameOrg(ctx context.Context, org, name string) error {
	path, err := Target{Org: org}.path()
	if err != nil {
		return err
	}
	title, err := orgName(name)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]*string{"name": title})
	return c.transport.NoContent(ctx, http.MethodPut, path, body)
}

// AddOrgMembers adds contributors. With shareHistory they can read everything pushed before they joined.
func (c *Client) AddOrgMembers(ctx context.Context, org string, members []string, shareHistory bool) error {
	path, err := Target{Org: org}.path()
	if err != nil {
		return err
	}
	if err := validMembers(members); err != nil {
		return err
	}
	body, _ := json.Marshal(struct {
		Members      []string `json:"members"`
		ShareHistory bool     `json:"share_history"`
	}{members, shareHistory})
	return c.transport.NoContent(ctx, http.MethodPost, path+"/members", body)
}

type OrgRole string

const (
	RoleMaintainer  OrgRole = "maintainer"
	RoleContributor OrgRole = "contributor"
)

func (c *Client) SetOrgRole(ctx context.Context, org, handle string, role OrgRole) error {
	path, err := Target{Org: org}.path()
	if err != nil || !handlePattern.MatchString(handle) || (role != RoleMaintainer && role != RoleContributor) {
		return errors.New("invalid org member")
	}
	body, _ := json.Marshal(map[string]OrgRole{"role": role})
	return c.transport.NoContent(ctx, http.MethodPut, path+"/members/"+handle, body)
}

// MessagePins is an org's pinned messages after a pin or unpin, newest pin first.
type MessagePins struct {
	Pinned           []Message `json:"pinned"`
	ReactionsVersion int64     `json:"reactions_version"`
}

// SetMessagePinned pins a message for everyone in its org, or unpins it.
func (c *Client) SetMessagePinned(ctx context.Context, id int64, pinned bool) (MessagePins, error) {
	if id <= 0 {
		return MessagePins{}, errors.New("invalid message id")
	}
	method := http.MethodDelete
	if pinned {
		method = http.MethodPut
	}
	var result MessagePins
	if err := c.transport.SendJSON(ctx, method, "/api/v1/remotes/messages/"+strconv.FormatInt(id, 10)+"/pin", nil, &result); err != nil {
		return MessagePins{}, err
	}
	if result.Pinned == nil {
		return MessagePins{}, errors.New("server returned invalid pins")
	}
	for _, message := range result.Pinned {
		if !validMessage(message) {
			return MessagePins{}, errors.New("server returned an invalid message")
		}
	}
	return result, nil
}

// orgName trims a name like the server does. An empty name is sent as null.
func orgName(name string) (*string, error) {
	name = trimInput(name)
	if utf8.RuneCountInString(name) > MaxOrgName {
		return nil, fmt.Errorf("org names are limited to %d characters", MaxOrgName)
	}
	if name == "" {
		return nil, nil
	}
	return &name, nil
}

// validMembers mirrors the server's rules: 1 to 99 distinct handles, since the creator or maintainer is already in.
func validMembers(handles []string) error {
	if len(handles) == 0 || len(handles) > MaxOrgMembers-1 {
		return fmt.Errorf("pick from 1 to %d people", MaxOrgMembers-1)
	}
	seen := make(map[string]bool, len(handles))
	for _, handle := range handles {
		key := strings.ToLower(handle)
		if !handlePattern.MatchString(handle) || seen[key] {
			return errors.New("invalid org member")
		}
		seen[key] = true
	}
	return nil
}
