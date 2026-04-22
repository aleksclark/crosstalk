package pion

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	crosstalk "github.com/anthropics/crosstalk/cli"
)

// AuthClient handles REST authentication with the CrossTalk server.
type AuthClient struct {
	ServerURL  string
	Token      string
	HTTPClient *http.Client
}

// NewAuthClient creates a new authentication client.
func NewAuthClient(serverURL, token string) *AuthClient {
	return &AuthClient{
		ServerURL:  serverURL,
		Token:      token,
		HTTPClient: http.DefaultClient,
	}
}

// RequestWebRTCToken requests a short-lived WebRTC token from the server.
// Uses the API token for authentication via Bearer header.
func (a *AuthClient) RequestWebRTCToken() (string, error) {
	url := a.ServerURL + "/api/webrtc/token"

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating webrtc token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)

	slog.Debug("requesting WebRTC token", "url", url)

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting webrtc token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading webrtc token response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// Parse response
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", &AuthError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("authentication failed (HTTP %d): %s", resp.StatusCode, string(body)),
		}
	default:
		return "", fmt.Errorf("unexpected status %d requesting webrtc token: %s", resp.StatusCode, string(body))
	}

	var tokenResp crosstalk.WebRTCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing webrtc token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("server returned empty webrtc token")
	}

	slog.Info("obtained WebRTC token")
	return tokenResp.Token, nil
}

// AuthError represents an authentication failure that should not be retried.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return e.Message
}

// IsAuthError checks if an error is an authentication error.
func IsAuthError(err error) bool {
	var authErr *AuthError
	return errors.As(err, &authErr)
}
