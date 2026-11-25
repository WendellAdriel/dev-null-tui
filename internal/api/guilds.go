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
	"unicode/utf8"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

// guildHandlePattern is the server's hashtag rule: a letter, then letters, marks, digits or underscores.
var guildHandlePattern = regexp.MustCompile(`^\pL[\pL\pM\pN_]*$`)

var notHashtag = regexp.MustCompile(`[^\pL\pM\pN_]+`)

// GuildBadge is the short form of a guild that commits and hashtags carry.
type GuildBadge struct {
	Handle  string `json:"handle"`
	Name    string `json:"name"`
	Emoji   string `json:"emoji"`
	IconURL string `json:"icon_url"`
}

type Guild struct {
	Handle      string `json:"handle"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Emoji       string `json:"emoji"`
	IconURL     string `json:"icon_url"`
	Members     int    `json:"member_count"`
	Joined      bool   `json:"is_joined"`
	Muted       bool   `json:"is_muted"`
	// Paused is a guild without a moderator: its board is read-only until it has one again.
	Paused bool   `json:"is_paused"`
	URL    string `json:"html_url"`
}

// GuildMentions maps handles to the guilds a commit's hashtags name.
type GuildMentions map[string]GuildBadge

// UnmarshalJSON accepts the empty list PHP sends for a commit that names no guild.
func (g *GuildMentions) UnmarshalJSON(data []byte) error {
	var list []json.RawMessage
	if json.Unmarshal(data, &list) == nil {
		*g = nil
		return nil
	}
	return json.Unmarshal(data, (*map[string]GuildBadge)(g))
}

// pages is Laravel's page meta, shared by the directory, a board and its members.
type pages struct {
	Meta struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
		Total       int `json:"total"`
	} `json:"meta"`
	Links struct {
		Next *string `json:"next"`
	} `json:"links"`
	Page, LastPage, Total int `json:"-"`
}

// settle reads the page numbers. A board uses a simple paginator with no last page, so its next link says whether
// another page follows.
func (p *pages) settle() {
	p.Page, p.LastPage, p.Total = max(1, p.Meta.CurrentPage), max(1, p.Meta.LastPage, p.Meta.CurrentPage), p.Meta.Total
	if p.Links.Next != nil && p.LastPage <= p.Page {
		p.LastPage = p.Page + 1
	}
}

type Guilds struct {
	Data []Guild `json:"data"`
	pages
}

type GuildBoard struct {
	Data        []Commit `json:"data"`
	Guild       Guild    `json:"guild"`
	Pinned      *Commit  `json:"pinned_commit"`
	CanModerate bool     `json:"can_moderate"`
	pages
}

type GuildMembers struct {
	Data []User `json:"data"`
	// ModeratorID is empty while the guild is paused.
	ModeratorID string `json:"moderator_id"`
	pages
}

// JoinedGuild is one of the joined, unmuted guilds the navigation lists, with the commits missed since the last visit.
type JoinedGuild struct {
	GuildBadge
	Missed   int        `json:"missed_commits"`
	ReadFrom *time.Time `json:"read_from"`
	URL      string     `json:"html_url"`
}

// GuildForm creates or edits a guild. The icon, when set, replaces the emoji; RemoveIcon only applies to edits.
type GuildForm struct {
	Name, Description, Emoji string
	Icon                     *auth.Upload
	RemoveIcon               bool
}

// GuildSaved is a create or edit: the published guild, or the server's message while it waits for an admin.
type GuildSaved struct {
	Guild   *Guild
	Message string
}

// GuildHashtag is the handle a display name gives a guild, as the server derives it.
func GuildHashtag(name string) string {
	return strings.ToLower(notHashtag.ReplaceAllString(name, ""))
}

// ValidGuildHandle applies the server's hashtag rule, so a handle is always safe in a path.
func ValidGuildHandle(handle string) bool {
	return utf8.RuneCountInString(handle) <= 60 && guildHandlePattern.MatchString(handle)
}

func guildPath(handle string) (string, error) {
	if !ValidGuildHandle(handle) {
		return "", errors.New("invalid guild handle")
	}
	return "/api/v1/guilds/" + url.PathEscape(handle), nil
}

func withPage(path string, query url.Values, page int) string {
	if page > 1 {
		query.Set("page", strconv.Itoa(page))
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return path
}

// Guilds lists the directory, biggest first, 24 to a page. A query searches names, handles and descriptions.
func (c *Client) Guilds(ctx context.Context, query string, page int) (Guilds, error) {
	values := url.Values{}
	if query = strings.TrimSpace(query); query != "" {
		values.Set("q", query)
	}
	var result Guilds
	if err := c.transport.GetJSON(ctx, withPage("/api/v1/guilds", values, page), &result); err != nil {
		return Guilds{}, err
	}
	if result.Data == nil {
		return Guilds{}, errors.New("server returned an invalid guild list")
	}
	for _, guild := range result.Data {
		if !ValidGuildHandle(guild.Handle) {
			return Guilds{}, errors.New("server returned an invalid guild")
		}
	}
	result.settle()
	return result, nil
}

// GuildBoard reads a guild's board: its commits newest first, the pinned commit, and whether you moderate it.
func (c *Client) GuildBoard(ctx context.Context, handle string, page int) (GuildBoard, error) {
	path, err := guildPath(handle)
	if err != nil {
		return GuildBoard{}, err
	}
	var board GuildBoard
	if err := c.transport.GetJSON(ctx, withPage(path, c.withReplies(url.Values{}), page), &board); err != nil {
		return GuildBoard{}, err
	}
	if board.Data == nil || board.Guild.Handle != handle {
		return GuildBoard{}, errors.New("server returned an invalid guild board")
	}
	for _, commit := range board.Data {
		if !validCommit(commit) {
			return GuildBoard{}, errors.New("server returned an invalid commit")
		}
	}
	if board.Pinned != nil && !validCommit(*board.Pinned) {
		return GuildBoard{}, errors.New("server returned an invalid pinned commit")
	}
	board.settle()
	return board, nil
}

// GuildMembers lists a guild's members, the moderator first, 50 to a page.
func (c *Client) GuildMembers(ctx context.Context, handle string, page int) (GuildMembers, error) {
	path, err := guildPath(handle)
	if err != nil {
		return GuildMembers{}, err
	}
	var members GuildMembers
	if err := c.transport.GetJSON(ctx, withPage(path+"/members", url.Values{}, page), &members); err != nil {
		return GuildMembers{}, err
	}
	if members.Data == nil {
		return GuildMembers{}, errors.New("server returned an invalid member list")
	}
	for _, member := range members.Data {
		if !handlePattern.MatchString(member.Handle) {
			return GuildMembers{}, errors.New("server returned an invalid member")
		}
	}
	members.settle()
	return members, nil
}

// JoinedGuilds lists the joined guilds you didn't mute, in the web's order: most missed commits first.
func (c *Client) JoinedGuilds(ctx context.Context) ([]JoinedGuild, error) {
	var result struct {
		Data []JoinedGuild `json:"data"`
	}
	if err := c.transport.GetJSON(ctx, "/api/v1/me/guilds", &result); err != nil {
		return nil, err
	}
	if result.Data == nil {
		return nil, errors.New("server returned an invalid guild list")
	}
	for _, guild := range result.Data {
		if !ValidGuildHandle(guild.Handle) {
			return nil, errors.New("server returned an invalid guild")
		}
	}
	return result.Data, nil
}

// toggle sends POST to turn a guild setting on and DELETE to turn it off, as membership, mute and pin do.
func (c *Client) toggle(ctx context.Context, path string, on bool) error {
	method := http.MethodDelete
	if on {
		method = http.MethodPost
	}
	return c.transport.NoContent(ctx, method, path, nil)
}

// SetGuildMembership joins or leaves a guild. Joining also clears a mute.
func (c *Client) SetGuildMembership(ctx context.Context, handle string, join bool) error {
	path, err := guildPath(handle)
	if err != nil {
		return err
	}
	return c.toggle(ctx, path+"/membership", join)
}

func (c *Client) SetGuildMute(ctx context.Context, handle string, mute bool) error {
	path, err := guildPath(handle)
	if err != nil {
		return err
	}
	return c.toggle(ctx, path+"/mute", mute)
}

// ReadGuild clears the guild's missed commits, as opening its board on the web does.
func (c *Client) ReadGuild(ctx context.Context, handle string) error {
	path, err := guildPath(handle)
	if err != nil {
		return err
	}
	return c.transport.NoContent(ctx, http.MethodPut, path+"/read", nil)
}

func (c *Client) ReportGuild(ctx context.Context, handle, details string) error {
	path, err := guildPath(handle)
	if err != nil {
		return err
	}
	if details = strings.TrimSpace(details); details == "" {
		return errors.New("describe what looks wrong")
	}
	body, err := json.Marshal(map[string]string{"details": details})
	if err != nil {
		return err
	}
	return c.transport.NoContent(ctx, http.MethodPost, path+"/reports", body)
}

// SetGuildPin pins a commit to the board or unpins it. Only the moderator can.
func (c *Client) SetGuildPin(ctx context.Context, handle, id string, pin bool) error {
	path, err := guildPath(handle)
	if err != nil {
		return err
	}
	if !commitID.MatchString(id) {
		return errors.New("invalid commit id")
	}
	method := http.MethodDelete
	if pin {
		method = http.MethodPut
	}
	return c.transport.NoContent(ctx, method, path+"/commits/"+id+"/pin", nil)
}

func (c *Client) CreateGuild(ctx context.Context, form GuildForm) (GuildSaved, error) {
	return c.saveGuild(ctx, http.MethodPost, "/api/v1/guilds", form)
}

func (c *Client) UpdateGuild(ctx context.Context, handle string, form GuildForm) (GuildSaved, error) {
	path, err := guildPath(handle)
	if err != nil {
		return GuildSaved{}, err
	}
	return c.saveGuild(ctx, http.MethodPut, path, form)
}

// saveGuild sends the form as JSON, or as multipart with the icon. A 201 or 200 carries the guild, and a 202 the
// message saying it waits for review.
func (c *Client) saveGuild(ctx context.Context, method, path string, form GuildForm) (GuildSaved, error) {
	fields := map[string]any{"name": strings.TrimSpace(form.Name), "description": strings.TrimSpace(form.Description), "emoji": form.Emoji}
	if method == http.MethodPut {
		fields["remove_icon"] = form.RemoveIcon
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return GuildSaved{}, err
	}
	var result struct {
		Data    *Guild `json:"data"`
		Message string `json:"message"`
	}
	if form.Icon != nil {
		err = c.transport.SendMultipart(ctx, method, path, payload, []auth.Upload{*form.Icon}, &result)
	} else {
		err = c.transport.SendJSON(ctx, method, path, payload, &result)
	}
	if err != nil {
		return GuildSaved{}, err
	}
	if result.Data != nil && ValidGuildHandle(result.Data.Handle) {
		return GuildSaved{Guild: result.Data}, nil
	}
	if result.Message != "" {
		return GuildSaved{Message: result.Message}, nil
	}
	return GuildSaved{}, errors.New("server returned an invalid guild response")
}
