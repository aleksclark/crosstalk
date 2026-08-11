// Package httpapi is a narrow net/http adapter for ct-play REST calls.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 1 << 20 // 1 MiB

// Client talks to the CrossTalk REST API.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

// New creates a Client for baseURL (trailing slash stripped).
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Session is a session row from GET /api/sessions.
type Session struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Channel is a channel row from GET /api/sessions/{id}/channels.
type Channel struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

// MediaTicket is the response from POST /api/webrtc/token.
type MediaTicket struct {
	Token             string    `json:"token"`
	ExpiresAt         time.Time `json:"expires_at"`
	SessionID         string    `json:"session_id"`
	Role              string    `json:"role"`
	ProduceChannelIDs []string  `json:"produce_channel_ids"`
	ListenChannelIDs  []string  `json:"listen_channel_ids"`
	OwnerGeneration   uint64    `json:"owner_generation"`
}

// APIError is a non-2xx HTTP response.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// Login authenticates with username/password and returns the access token only.
func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	if err := c.requireBase(); err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", fmt.Errorf("encode login: %w", err)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/auth/login", "", body, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("login response missing access token")
	}
	return out.AccessToken, nil
}

// ListSessions returns sessions visible to the authenticated user.
func (c *Client) ListSessions(ctx context.Context, accessToken string) ([]Session, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	var out struct {
		Data []Session `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/sessions", accessToken, nil, &out); err != nil {
		return nil, err
	}
	if out.Data == nil {
		out.Data = []Session{}
	}
	return out.Data, nil
}

// ListChannels returns channels for a session.
func (c *Client) ListChannels(ctx context.Context, accessToken, sessionID string) ([]Channel, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	path := "/api/sessions/" + url.PathEscape(sessionID) + "/channels"
	var out struct {
		Data []Channel `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, accessToken, nil, &out); err != nil {
		return nil, err
	}
	if out.Data == nil {
		out.Data = []Channel{}
	}
	return out.Data, nil
}

// IssueMediaTicket mints a one-time media ticket scoped to produceChannelIDs.
func (c *Client) IssueMediaTicket(ctx context.Context, accessToken, sessionID string, produceChannelIDs []string) (*MediaTicket, error) {
	if err := c.requireBase(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if produceChannelIDs == nil {
		produceChannelIDs = []string{}
	}
	body, err := json.Marshal(map[string]any{
		"session_id": sessionID,
		"produce":    produceChannelIDs,
		"listen":     []string{},
	})
	if err != nil {
		return nil, fmt.Errorf("encode media ticket request: %w", err)
	}
	var out MediaTicket
	if err := c.doJSON(ctx, http.MethodPost, "/api/webrtc/token", accessToken, body, &out); err != nil {
		return nil, err
	}
	if out.Token == "" {
		return nil, fmt.Errorf("media ticket response missing token")
	}
	if out.ProduceChannelIDs == nil {
		out.ProduceChannelIDs = []string{}
	}
	if out.ListenChannelIDs == nil {
		out.ListenChannelIDs = []string{}
	}
	return &out, nil
}

func (c *Client) requireBase() error {
	if c == nil || c.BaseURL == "" {
		return fmt.Errorf("host is required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("host must be an absolute http(s) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("host scheme must be http or https")
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path, bearer string, body []byte, dest any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(raw) > maxBody {
		return fmt.Errorf("response body exceeds %d bytes", maxBody)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    safeErrorMessage(raw),
		}
	}
	if dest == nil {
		return nil
	}
	if len(raw) == 0 {
		return fmt.Errorf("empty response body")
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func safeErrorMessage(raw []byte) string {
	var envelope struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
		Error  struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		switch {
		case envelope.Detail != "":
			return envelope.Detail
		case envelope.Error.Message != "":
			return envelope.Error.Message
		case envelope.Message != "":
			return envelope.Message
		case envelope.Title != "":
			return envelope.Title
		}
	}
	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		return ""
	}
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	// Never echo obvious JWT-shaped material.
	if strings.Count(msg, ".") >= 2 && len(msg) > 40 {
		return "request rejected"
	}
	return msg
}
