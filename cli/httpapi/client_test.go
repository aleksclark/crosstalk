package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginAndListSessions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/auth/login":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "alice", body["username"])
			assert.Equal(t, "secret", body["password"])
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token":  "access-xyz",
				"refresh_token": "refresh-should-not-leak",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions":
			assert.Equal(t, "Bearer access-xyz", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "s2", "name": "Beta"},
					{"id": "s1", "name": "Alpha"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := New(ts.URL + "/")
	tok, err := c.Login(context.Background(), "alice", "secret")
	require.NoError(t, err)
	assert.Equal(t, "access-xyz", tok)

	sessions, err := c.ListSessions(context.Background(), tok)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	assert.Equal(t, "s2", sessions[0].ID)
}

func TestListChannelsAndMediaTicket(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions/sess1/channels":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "c1", "session_id": "sess1", "name": "EN", "type": "broadcast"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/webrtc/token":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "sess1", body["session_id"])
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":               "ticket-abc",
				"expires_at":          time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano),
				"session_id":          "sess1",
				"role":                "translator",
				"produce_channel_ids": []string{"c1"},
				"listen_channel_ids":  []string{},
				"owner_generation":    1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := New(ts.URL)
	chs, err := c.ListChannels(context.Background(), "tok", "sess1")
	require.NoError(t, err)
	require.Len(t, chs, 1)
	assert.Equal(t, "broadcast", chs[0].Type)

	ticket, err := c.IssueMediaTicket(context.Background(), "tok", "sess1", []string{"c1"})
	require.NoError(t, err)
	assert.Equal(t, "ticket-abc", ticket.Token)
	assert.Equal(t, []string{"c1"}, ticket.ProduceChannelIDs)
}

func TestLoginUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid credentials"}`))
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.Login(context.Background(), "alice", "wrong")
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.NotContains(t, err.Error(), "wrong")
}

func TestRequireHost(t *testing.T) {
	c := New("")
	_, err := c.Login(context.Background(), "a", "b")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "host")
}

func TestContextCancel(t *testing.T) {
	block := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer ts.Close()
	defer close(block)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(ts.URL)
	_, err := c.Login(ctx, "a", "b")
	require.Error(t, err)
}
