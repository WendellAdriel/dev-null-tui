package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultOrigin = "https://dev-null.sh"

type User struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Username         string          `json:"username"`
	AvatarURL        string          `json:"avatar_url"`
	Bio              string          `json:"bio"`
	Company          string          `json:"company"`
	Location         string          `json:"location"`
	Commits          *int            `json:"commits_count"`
	Following        *int            `json:"following_count"`
	Followers        *int            `json:"followers_count"`
	Lane             json.RawMessage `json:"lane"`
	PasteisMode      bool            `json:"pasteis_mode"`
	PasteisRemaining *int            `json:"pasteis_remaining"`
}

type APIError struct {
	Status     int
	Code       string
	RetryAfter string
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

func (c *Client) request(ctx context.Context, method, path string, body []byte) (User, error) {
	data, err := c.send(ctx, method, path, body)
	if err != nil {
		return User{}, err
	}
	var result struct {
		Data User `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Data.ID == "" || result.Data.Username == "" {
		return User{}, errors.New("account server returned an invalid identity")
	}
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

func (c *Client) send(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	u, err := c.origin.Parse(path)
	if err != nil || u.Scheme != c.origin.Scheme || u.Host != c.origin.Host || u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/api/v1/") {
		return nil, errors.New("refusing a request outside the configured API origin")
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot create account request")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceString := hex.EncodeToString(nonce)
	message := []byte(strings.Join([]string{timestamp, nonceString, req.Method, req.URL.RequestURI(), string(body)}, "\n"))
	signature, err := c.key.Sign(message)
	if err != nil {
		return nil, errors.New("cannot sign account request")
	}
	req.Header.Set("X-Devnull-Timestamp", timestamp)
	req.Header.Set("X-Devnull-Nonce", nonceString)
	req.Header.Set("X-SSH-Signature", signature)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
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
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &failure)
		return nil, &APIError{Status: response.StatusCode, Code: failure.Code, RetryAfter: response.Header.Get("Retry-After")}
	}
	return data, nil
}
