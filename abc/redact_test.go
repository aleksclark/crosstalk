package abc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sentinelToken = "sentinel-abc-token-do-not-leak"

func TestRedactURL_StripsTokenQuery(t *testing.T) {
	raw := "ws://crosstalk.example:8443/ws/signaling?token=" + sentinelToken
	got := RedactURL(raw)
	assert.NotContains(t, got, sentinelToken)
	assert.Contains(t, got, "token=%5Bredacted%5D")
	assert.Contains(t, got, "crosstalk.example:8443")
}

func TestRedactURL_PreservesOtherQuery(t *testing.T) {
	raw := "https://host/ws/signaling?token=" + sentinelToken + "&peer=abc"
	got := RedactURL(raw)
	assert.NotContains(t, got, sentinelToken)
	assert.Contains(t, got, "peer=abc")
}

func TestRedactURL_UnparseableDoesNotRecurse(t *testing.T) {
	raw := "http://[" + sentinelToken
	got := RedactURL(raw)
	assert.Equal(t, raw, got)
}

func TestRedactError_RemovesTokenSubstring(t *testing.T) {
	err := errors.New("opening websocket: failed dial ws://h/ws/signaling?token=" + sentinelToken)
	got := redactError(err, sentinelToken)
	require.Error(t, got)
	assert.NotContains(t, got.Error(), sentinelToken)
	assert.True(t, errors.Is(got, err))
}

func TestAuthError_DoesNotIncludeToken(t *testing.T) {
	err := authErrorFromStatus(401)
	assert.True(t, IsAuthError(err))
	assert.NotContains(t, err.Error(), sentinelToken)
	assert.Contains(t, err.Error(), "401")
}

func TestProtocolError_DoesNotIncludeToken(t *testing.T) {
	err := &ProtocolError{Reason: "malformed control frame"}
	assert.True(t, IsProtocolError(err))
	assert.NotContains(t, err.Error(), sentinelToken)
}
