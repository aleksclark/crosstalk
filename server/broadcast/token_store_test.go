package broadcast

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenStore_CreateAndValidate(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	bt, err := ts.CreateBroadcastToken("session-123", 15*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, bt)

	// Token should have correct format.
	assert.True(t, strings.HasPrefix(bt.Token, "ctb_"), "token should start with ctb_ prefix")
	assert.Equal(t, "session-123", bt.SessionID)
	assert.False(t, bt.ExpiresAt.IsZero())

	// Validate the token — round-trip should work.
	validated, err := ts.ValidateBroadcastToken(bt.Token)
	require.NoError(t, err)
	require.NotNil(t, validated)
	assert.Equal(t, bt.SessionID, validated.SessionID)
	assert.Equal(t, bt.Token, validated.Token)
}

func TestTokenStore_InvalidToken(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no prefix", "abc_def_ghi_123"},
		{"wrong prefix", "ct_something_session_123"},
		{"garbage", "complete-garbage"},
		{"too few parts", "ctb_hmac_nonce_session"},
		{"tampered hmac", "ctb_deadbeef_0011223344556677_session-123_9999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ts.ValidateBroadcastToken(tt.token)
			assert.Error(t, err)
		})
	}
}

func TestTokenStore_ExpiredToken(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	// Override time to create a token that is already expired.
	past := time.Now().Add(-1 * time.Hour)
	ts.nowFunc = func() time.Time { return past }

	bt, err := ts.CreateBroadcastToken("session-456", 30*time.Minute)
	require.NoError(t, err)

	// Reset time to now — token should be expired.
	ts.nowFunc = time.Now

	_, err = ts.ValidateBroadcastToken(bt.Token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestTokenStore_Revocation(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	bt1, err := ts.CreateBroadcastToken("session-A", 15*time.Minute)
	require.NoError(t, err)

	bt2, err := ts.CreateBroadcastToken("session-A", 15*time.Minute)
	require.NoError(t, err)

	bt3, err := ts.CreateBroadcastToken("session-B", 15*time.Minute)
	require.NoError(t, err)

	// Both tokens for session-A should be valid.
	_, err = ts.ValidateBroadcastToken(bt1.Token)
	require.NoError(t, err)
	_, err = ts.ValidateBroadcastToken(bt2.Token)
	require.NoError(t, err)

	// Revoke all tokens for session-A.
	ts.RevokeBroadcastTokens("session-A")

	// Session-A tokens should fail.
	_, err = ts.ValidateBroadcastToken(bt1.Token)
	assert.Error(t, err)
	_, err = ts.ValidateBroadcastToken(bt2.Token)
	assert.Error(t, err)

	// Session-B token should still work.
	_, err = ts.ValidateBroadcastToken(bt3.Token)
	require.NoError(t, err)
}

func TestTokenStore_TokenFormat(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	bt, err := ts.CreateBroadcastToken("session-XYZ", 15*time.Minute)
	require.NoError(t, err)

	// Token format: ctb_{hmac}_{nonce}_{session_id}_{expires_unix}
	parts := strings.SplitN(bt.Token, "_", 5)
	require.Len(t, parts, 5)
	assert.Equal(t, "ctb", parts[0])
	assert.NotEmpty(t, parts[1], "HMAC should not be empty")
	assert.NotEmpty(t, parts[2], "nonce should not be empty")
	assert.Equal(t, "session-XYZ", parts[3])
	assert.NotEmpty(t, parts[4], "expires unix should not be empty")
}

func TestTokenStore_DifferentSecrets(t *testing.T) {
	ts1 := NewTokenStore("secret-1", 15*time.Minute)
	defer ts1.Stop()

	ts2 := NewTokenStore("secret-2", 15*time.Minute)
	defer ts2.Stop()

	bt, err := ts1.CreateBroadcastToken("session-123", 15*time.Minute)
	require.NoError(t, err)

	// Store the token in ts2 so it passes the store lookup,
	// but HMAC should still fail because the secret differs.
	// Actually, ValidateBroadcastToken checks HMAC first, then store.
	_, err = ts2.ValidateBroadcastToken(bt.Token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestTokenStore_EmptySessionID(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	_, err := ts.CreateBroadcastToken("", 15*time.Minute)
	assert.Error(t, err)
}

func TestTokenStore_Cleanup(t *testing.T) {
	ts := NewTokenStore("test-secret", 15*time.Minute)
	defer ts.Stop()

	// Create token in the past.
	past := time.Now().Add(-1 * time.Hour)
	ts.nowFunc = func() time.Time { return past }
	bt, err := ts.CreateBroadcastToken("session-old", 30*time.Minute)
	require.NoError(t, err)

	// Reset to now and run cleanup.
	ts.nowFunc = time.Now
	ts.cleanup()

	// Token should have been cleaned up from the store.
	_, loaded := ts.tokens.Load(bt.Token)
	assert.False(t, loaded, "expired token should be cleaned up")
}
