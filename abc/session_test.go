package abc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDial_InvalidTokenIsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ws/signaling", r.URL.Path)
		assert.Equal(t, sentinelToken, r.URL.Query().Get("token"))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := Dial(ctx, Config{
		ServerURL:      srv.URL,
		Token:          sentinelToken,
		RequireWelcome: true,
		DisableMDNS:    true,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.True(t, IsAuthError(err), "got %T %v", err, err)
	assert.NotContains(t, err.Error(), sentinelToken)
}

func TestDial_MissingTokenRejectedBeforeNetwork(t *testing.T) {
	_, err := Dial(context.Background(), Config{ServerURL: "http://127.0.0.1:1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Token")
	assert.False(t, IsAuthError(err))
}

func TestDial_MissingServerURLRejected(t *testing.T) {
	_, err := Dial(context.Background(), Config{Token: sentinelToken})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ServerURL")
}

func TestSignalingURL_EscapesTokenAndRedacts(t *testing.T) {
	raw, err := signalingURL("http://host.example:8080", "a b/"+sentinelToken)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "ws://host.example:8080/ws/signaling?"))
	assert.Contains(t, raw, "token=")
	redacted := RedactURL(raw)
	assert.NotContains(t, redacted, sentinelToken)
	assert.NotContains(t, redacted, "a+b")
}

func TestSendControl_MalformedClosesSession(t *testing.T) {
	s := &Session{
		cfg:    Config{Token: sentinelToken},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		done:   make(chan struct{}),
		ctx:    context.Background(),
		cancel: func() {},
	}
	err := s.SendControl([]byte{0xff, 0x00, 0x01})
	require.Error(t, err)
	assert.True(t, IsProtocolError(err))
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("session did not close after malformed outbound control")
	}
	assert.True(t, IsProtocolError(s.Err()))
	assert.NotContains(t, s.Err().Error(), sentinelToken)
	assert.Equal(t, StateFailed, s.State())
}

func TestHandleControl_MalformedClosesSession(t *testing.T) {
	s := &Session{
		cfg:    Config{Token: sentinelToken},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		done:   make(chan struct{}),
		ctx:    context.Background(),
		cancel: func() {},
	}
	s.handleControl([]byte{0xff, 0x00, 0x01})
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("session did not close after malformed control")
	}
	assert.True(t, IsProtocolError(s.Err()))
	assert.NotContains(t, s.Err().Error(), sentinelToken)
	assert.Equal(t, StateFailed, s.State())
}

func TestSessionEpochsAreDistinct(t *testing.T) {
	a := nextEpoch.Add(1)
	b := nextEpoch.Add(1)
	assert.NotEqual(t, a, b)
}

func TestOnTrackFlushesPendingTracks(t *testing.T) {
	s := &Session{
		cfg:    Config{Token: sentinelToken},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		done:   make(chan struct{}),
		ctx:    context.Background(),
		cancel: func() {},
	}
	s.dispatchTrack(IncomingTrack{ID: "pending"})
	got := make(chan IncomingTrack, 1)
	s.OnTrack(func(track IncomingTrack) { got <- track })
	select {
	case track := <-got:
		assert.Equal(t, "pending", track.ID)
	case <-time.After(time.Second):
		t.Fatal("pending track was not delivered")
	}
}
