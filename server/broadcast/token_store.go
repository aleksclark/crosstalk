// Package broadcast implements broadcast token management for public
// listener access to CrossTalk sessions.
package broadcast

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// TokenStore is an in-memory implementation of crosstalk.BroadcastTokenStore.
// Tokens are HMAC-signed with a shared secret and stored in a sync.Map for
// concurrent access. Expired tokens are cleaned up periodically.
type TokenStore struct {
	secret string
	ttl    time.Duration
	tokens sync.Map // map[string]*crosstalk.BroadcastToken
	stopCh chan struct{}

	// nowFunc allows tests to override time.Now.
	nowFunc func() time.Time
}

// NewTokenStore creates a new TokenStore with the given HMAC secret and default
// TTL for tokens. It starts a background goroutine that periodically cleans up
// expired tokens. Call Stop() to terminate the cleanup goroutine.
func NewTokenStore(secret string, ttl time.Duration) *TokenStore {
	ts := &TokenStore{
		secret:  secret,
		ttl:     ttl,
		stopCh:  make(chan struct{}),
		nowFunc: time.Now,
	}
	go ts.cleanupLoop()
	return ts
}

// Stop terminates the background cleanup goroutine.
func (ts *TokenStore) Stop() {
	select {
	case <-ts.stopCh:
		// already stopped
	default:
		close(ts.stopCh)
	}
}

// CreateBroadcastToken generates a new broadcast token for the given session.
// The token format is: ctb_{hmac}_{nonce}_{session_id}_{expires_unix}
func (ts *TokenStore) CreateBroadcastToken(sessionID string, ttl time.Duration) (*crosstalk.BroadcastToken, error) {
	if sessionID == "" {
		return nil, errors.New("session ID is required")
	}

	expiresAt := ts.nowFunc().Add(ttl)
	expiresUnix := expiresAt.Unix()

	// Generate a random nonce for uniqueness.
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	// Build the HMAC payload: nonce + session_id + expires_at unix timestamp.
	payload := nonce + sessionID + strconv.FormatInt(expiresUnix, 10)
	mac := ts.computeHMAC(payload)

	token := fmt.Sprintf("ctb_%s_%s_%s_%d", mac, nonce, sessionID, expiresUnix)

	bt := &crosstalk.BroadcastToken{
		Token:     token,
		SessionID: sessionID,
		ExpiresAt: expiresAt.UTC(),
	}

	ts.tokens.Store(token, bt)

	slog.Debug("broadcast token created",
		"session_id", sessionID,
		"expires_at", expiresAt.Format(time.RFC3339),
	)

	return bt, nil
}

// ValidateBroadcastToken validates a broadcast token by checking the HMAC
// signature, expiry, and presence in the store (for revocation support).
func (ts *TokenStore) ValidateBroadcastToken(token string) (*crosstalk.BroadcastToken, error) {
	// Parse the token format: ctb_{hmac}_{nonce}_{session_id}_{expires_unix}
	if !strings.HasPrefix(token, "ctb_") {
		return nil, errors.New("invalid broadcast token format")
	}

	// Split: ["ctb", hmac, nonce, session_id, expires_unix]
	parts := strings.SplitN(token, "_", 5)
	if len(parts) != 5 {
		return nil, errors.New("invalid broadcast token format")
	}

	macHex := parts[1]
	nonce := parts[2]
	sessionID := parts[3]
	expiresStr := parts[4]

	expiresUnix, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return nil, errors.New("invalid broadcast token expiry")
	}

	// Verify HMAC.
	payload := nonce + sessionID + expiresStr
	expectedMAC := ts.computeHMAC(payload)
	if !hmac.Equal([]byte(macHex), []byte(expectedMAC)) {
		return nil, errors.New("invalid broadcast token signature")
	}

	// Check expiry.
	expiresAt := time.Unix(expiresUnix, 0)
	if ts.nowFunc().After(expiresAt) {
		// Also remove from store.
		ts.tokens.Delete(token)
		return nil, errors.New("broadcast token expired")
	}

	// Check store for revocation.
	stored, ok := ts.tokens.Load(token)
	if !ok {
		return nil, errors.New("broadcast token revoked or not found")
	}

	bt := stored.(*crosstalk.BroadcastToken)
	return bt, nil
}

// RevokeBroadcastTokens removes all broadcast tokens for the given session ID.
func (ts *TokenStore) RevokeBroadcastTokens(sessionID string) {
	ts.tokens.Range(func(key, value any) bool {
		bt := value.(*crosstalk.BroadcastToken)
		if bt.SessionID == sessionID {
			ts.tokens.Delete(key)
			slog.Debug("broadcast token revoked",
				"session_id", sessionID,
				"token", key,
			)
		}
		return true
	})
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of the given payload.
func (ts *TokenStore) computeHMAC(payload string) string {
	h := hmac.New(sha256.New, []byte(ts.secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// cleanupLoop periodically removes expired tokens from the store.
func (ts *TokenStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ts.stopCh:
			return
		case <-ticker.C:
			ts.cleanup()
		}
	}
}

// cleanup removes expired tokens from the store.
func (ts *TokenStore) cleanup() {
	now := ts.nowFunc()
	ts.tokens.Range(func(key, value any) bool {
		bt := value.(*crosstalk.BroadcastToken)
		if now.After(bt.ExpiresAt) {
			ts.tokens.Delete(key)
			slog.Debug("expired broadcast token cleaned up", "token", key)
		}
		return true
	})
}
