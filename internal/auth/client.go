package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const DefaultOrigin = "https://dev-null.sh"

type User struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Handle              string             `json:"handle"`
	AvatarURL           string             `json:"avatar_url"`
	Bio                 string             `json:"bio"`
	Company             string             `json:"company"`
	Location            string             `json:"location"`
	Website             string             `json:"website"`
	GitHubURL           string             `json:"github_url"`
	SponsorsURL         string             `json:"sponsors_url"`
	GitHubSyncedAt      string             `json:"github_synced_at"`
	PreferredLane       json.RawMessage    `json:"preferred_lane"`
	AutomaticLane       int                `json:"automatic_lane"`
	Remotes             *RemotePreferences `json:"remotes"`
	Commits             *int               `json:"commits_count"`
	Following           *int               `json:"following_count"`
	Followers           *int               `json:"followers_count"`
	Lane                json.RawMessage    `json:"lane"`
	PasteisMode         bool               `json:"pasteis_mode"`
	PasteisRemaining    *int               `json:"pasteis_remaining"`
	TermsVersion        string             `json:"terms_version"`
	TermsAccepted       bool               `json:"terms_accepted"`
	UnreadNotifications *int               `json:"-"`
}

type RemotePreferences struct {
	From         string `json:"from"`
	PullReceipts bool   `json:"pull_receipts"`
	Everyone     bool   `json:"everyone"`
	Requests     bool   `json:"requests"`
	UnreadCount  int    `json:"unread_count"`
}

type APIError struct {
	Status     int
	Code       string
	RetryAfter string
	// Message is the server's reason for a refusal, such as a Remotes access denial.
	Message    string
	BodyErrors []string
	// FieldErrors holds the poll, image, and vote messages, which are safe to show beside the draft.
	FieldErrors []string
}

func (e *APIError) Error() string {
	switch e.Code {
	case "invalid_grant":
		return "approval expired or was already used; connect again"
	case "invalid_signature":
		return "device signature rejected; check your clock and reconnect"
	case "device_not_approved":
		return "device approval was revoked or is missing; reconnect"
	case "rate_limited":
		return "too many attempts; wait before retrying"
	case "banned":
		return "this account cannot access the API"
	case "commit_rules_required":
		return "your first commit must come from the web after reading the commit rules"
	}
	return fmt.Sprintf("account request failed (HTTP %d)", e.Status)
}

type Client struct {
	origin *url.URL
	key    *Key
	http   *http.Client
}

func parseOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("server must be an HTTPS origin without a path, query, or credentials")
	}
	loopback := u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return nil, errors.New("server requires HTTPS; HTTP is allowed only on loopback for isolated testing")
	}
	u.Path = ""
	return u, nil
}

func NewClient(origin string, key *Key) (*Client, error) {
	u, err := parseOrigin(origin)
	if err != nil {
		return nil, err
	}
	return &Client{origin: u, key: key, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Enroll(ctx context.Context, code, verifier string) (User, error) {
	body, err := json.Marshal(struct {
		Code     string `json:"code"`
		Verifier string `json:"code_verifier"`
	}{code, verifier})
	if err != nil {
		return User{}, err
	}
	return c.request(ctx, http.MethodPost, "/api/v1/devices", body)
}

func (c *Client) Me(ctx context.Context) (User, error) {
	return c.request(ctx, http.MethodGet, "/api/v1/me", nil)
}

func (c *Client) AcceptTerms(ctx context.Context, version string) (User, error) {
	if version == "" {
		return User{}, errors.New("verify the current terms version before accepting")
	}
	body, err := json.Marshal(struct {
		Version string `json:"version"`
	}{version})
	if err != nil {
		return User{}, err
	}
	user, err := c.request(ctx, http.MethodPut, "/api/v1/me/terms", body)
	if err != nil {
		return User{}, err
	}
	if !user.TermsAccepted || user.TermsVersion != version {
		return User{}, errors.New("server did not confirm the displayed terms version; verify the connection")
	}
	return user, nil
}

func (c *Client) request(ctx context.Context, method, path string, body []byte) (User, error) {
	data, err := c.send(ctx, method, path, body)
	if err != nil {
		return User{}, err
	}
	var result struct {
		Data                User `json:"data"`
		UnreadNotifications *int `json:"unread_notifications_count"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Data.ID == "" || result.Data.Handle == "" {
		return User{}, errors.New("account server returned an invalid identity")
	}
	result.Data.UnreadNotifications = result.UnreadNotifications
	return result.Data, nil
}

func (c *Client) GetJSON(ctx context.Context, target string, result any) error {
	data, err := c.send(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return errors.New("server returned an invalid response")
	}
	return nil
}

func (c *Client) PostJSON(ctx context.Context, target string, body []byte, result any) error {
	data, err := c.send(ctx, http.MethodPost, target, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return errors.New("server returned an invalid response")
	}
	return nil
}

func (c *Client) Post(ctx context.Context, target string) error {
	_, err := c.send(ctx, http.MethodPost, target, nil)
	return err
}

func (c *Client) Delete(ctx context.Context, target string) error {
	_, err := c.send(ctx, http.MethodDelete, target, nil)
	return err
}

func (c *Client) DeleteJSON(ctx context.Context, target string, result any) error {
	data, err := c.send(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return errors.New("server returned an invalid response")
	}
	return nil
}

func (c *Client) PutJSON(ctx context.Context, target string, body []byte, result any) error {
	data, err := c.send(ctx, http.MethodPut, target, body)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, result) != nil {
		return errors.New("server returned an invalid response")
	}
	return nil
}

// SendJSON sends a JSON body with any method, for routes like removing a reaction that take a body on DELETE.
func (c *Client) SendJSON(ctx context.Context, method, target string, body []byte, result any) error {
	data, err := c.send(ctx, method, target, body)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, result) != nil {
		return errors.New("server returned an invalid response")
	}
	return nil
}

func (c *Client) NoContent(ctx context.Context, method, target string, body []byte) error {
	_, err := c.sendStatus(ctx, method, target, body, http.StatusNoContent)
	return err
}

// Upload is one image sent beside a multipart payload.
type Upload struct {
	Name, Type string
	Data       []byte
}

// PostMultipart sends the JSON payload as a payload part and each image as images[], in order. The signed message
// carries the comma-separated SHA-256 of the images before the payload, under the images namespace.
func (c *Client) PostMultipart(ctx context.Context, target string, payload []byte, images []Upload, result any) error {
	if len(images) == 0 {
		return errors.New("a multipart request needs at least one image")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	hashes := make([]string, len(images))
	if err := writePart(writer, "payload", "payload.json", "application/json", payload); err != nil {
		return err
	}
	for i, image := range images {
		sum := sha256.Sum256(image.Data)
		hashes[i] = hex.EncodeToString(sum[:])
		if err := writePart(writer, "images[]", image.Name, image.Type, image.Data); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	data, err := c.do(ctx, signedRequest{
		method:      http.MethodPost,
		path:        target,
		body:        body.Bytes(),
		contentType: writer.FormDataContentType(),
		signed:      []string{strings.Join(hashes, ","), string(payload)},
		namespace:   imagesNamespace,
		upload:      true,
	}, 0)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, result) != nil {
		return errors.New("server returned an invalid response")
	}
	return nil
}

func writePart(writer *multipart.Writer, field, name, contentType string, data []byte) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": name}))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

func (c *Client) send(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return c.sendStatus(ctx, method, path, body, 0)
}

func (c *Client) sendStatus(ctx context.Context, method, path string, body []byte, expected int) ([]byte, error) {
	return c.do(ctx, signedRequest{method: method, path: path, body: body, contentType: "application/json", signed: []string{string(body)}, namespace: apiNamespace}, expected)
}

// signedRequest lists the lines signed after the timestamp, nonce, method, and URI, which differ for multipart bodies.
type signedRequest struct {
	method, path, contentType, namespace string
	body                                 []byte
	signed                               []string
	upload                               bool
}

func (c *Client) do(ctx context.Context, request signedRequest, expected int) ([]byte, error) {
	u, err := c.origin.Parse(request.path)
	if err != nil || u.Scheme != c.origin.Scheme || u.Host != c.origin.Host || u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/api/v1/") {
		return nil, errors.New("refusing a request outside the configured API origin")
	}
	req, err := http.NewRequestWithContext(ctx, request.method, u.String(), bytes.NewReader(request.body))
	if err != nil {
		return nil, errors.New("cannot create account request")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceString := hex.EncodeToString(nonce)
	lines := append([]string{timestamp, nonceString, req.Method, req.URL.RequestURI()}, request.signed...)
	signature, err := c.key.SignNamespace(request.namespace, []byte(strings.Join(lines, "\n")))
	if err != nil {
		return nil, errors.New("cannot sign account request")
	}
	req.Header.Set("X-Devnull-Timestamp", timestamp)
	req.Header.Set("X-Devnull-Nonce", nonceString)
	req.Header.Set("X-SSH-Signature", signature)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", request.contentType)
	client := c.http
	if request.upload {
		client = c.uploads()
	}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("cannot reach the server; check your connection and try again")
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("server returned an unreadable response")
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusNoContent {
		var failure struct {
			Code    string              `json:"code"`
			Message string              `json:"message"`
			Errors  map[string][]string `json:"errors"`
		}
		_ = json.Unmarshal(data, &failure)
		return nil, &APIError{Status: response.StatusCode, Code: failure.Code, Message: failure.Message, RetryAfter: response.Header.Get("Retry-After"), BodyErrors: failure.Errors["body"], FieldErrors: fieldErrors(failure.Errors)}
	}
	if expected != 0 && response.StatusCode != expected {
		return nil, errors.New("server did not confirm the operation; verify before retrying")
	}
	return data, nil
}

// UploadTimeout allows a few images up to 10 MB each on a slow link, while other requests keep the short timeout.
const UploadTimeout = 2 * time.Minute

func (c *Client) uploads() *http.Client {
	client := *c.http
	client.Timeout = UploadTimeout
	return &client
}

func fieldErrors(all map[string][]string) []string {
	fields := make([]string, 0, len(all))
	for field := range all {
		name, _, _ := strings.Cut(field, ".")
		switch name {
		case "poll", "images", "image_descriptions", "option", "emoji", "reply_to_id":
			fields = append(fields, field)
		}
	}
	slices.Sort(fields)
	var messages []string
	for _, field := range fields {
		messages = append(messages, all[field]...)
	}
	return messages
}
