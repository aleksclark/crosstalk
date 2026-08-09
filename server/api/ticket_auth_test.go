package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Media ticket issuance ---

func TestTicket_IssueScopedOneTimeMediaTicket(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	tr := env.createUser(t, "tr", "tr123", "translator")
	trTok := env.login(t, "tr", "tr123")

	sid, _ := env.createSession(t, adminTok, "TicketSess")
	feedID := env.createChannel(t, adminTok, sid, "Floor", "feed")
	bcastID := env.createChannel(t, adminTok, sid, "English", "broadcast")
	require.NoError(t, env.users.AssignSessions(context.Background(), tr.ID, []string{sid}))

	tok, body := env.issueTicket(t, trTok, sid, nil, nil)
	assert.NotEmpty(t, tok)
	assert.Equal(t, sid, body["session_id"])
	assert.Equal(t, "translator", body["role"])

	prod := body["produce_channel_ids"]
	listen := body["listen_channel_ids"]
	prodIDs := toStrings(prod)
	listenIDs := toStrings(listen)
	assert.Contains(t, prodIDs, bcastID)
	assert.Contains(t, listenIDs, feedID)
	assert.NotContains(t, prodIDs, feedID) // translator does not produce into feed by default
}

func TestTicket_ChannelNarrowingCannotExpand(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	tr := env.createUser(t, "tr", "tr123", "translator")
	trTok := env.login(t, "tr", "tr123")

	sid, _ := env.createSession(t, adminTok, "Narrow")
	feedID := env.createChannel(t, adminTok, sid, "Floor", "feed")
	bcastID := env.createChannel(t, adminTok, sid, "English", "broadcast")
	require.NoError(t, env.users.AssignSessions(context.Background(), tr.ID, []string{sid}))

	// Request produce=feed (not in translator default produce) → empty produce.
	_, body := env.issueTicket(t, trTok, sid, []string{feedID}, []string{feedID})
	prod := toStrings(body["produce_channel_ids"])
	listen := toStrings(body["listen_channel_ids"])
	assert.Empty(t, prod, "cannot expand produce into feed")
	assert.Equal(t, []string{feedID}, listen)
	assert.NotContains(t, prod, bcastID)
}

func TestTicket_TranslatorCannotIssueForUnassignedSession(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	env.createUser(t, "tr", "tr123", "translator")
	trTok := env.login(t, "tr", "tr123")

	sid, _ := env.createSession(t, adminTok, "Other")
	resp, data := env.doJSON(t, http.MethodPost, "/api/webrtc/token", trTok, map[string]any{
		"session_id": sid,
	})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(data))
}

func TestTicket_ListenerRoleCannotProduce(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, _ := env.createSession(t, adminTok, "ListenOnly")
	_ = env.createChannel(t, adminTok, sid, "English", "broadcast")
	_ = env.createChannel(t, adminTok, sid, "Floor", "feed")

	resp, data := env.doJSON(t, http.MethodPost, "/api/webrtc/token", adminTok, map[string]any{
		"session_id": sid,
		"role":       "listener",
		"produce":    []string{"type:broadcast"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var body map[string]any
	require.NoError(t, json.Unmarshal(data, &body))
	assert.Equal(t, "listener", body["role"])
	assert.Empty(t, toStrings(body["produce_channel_ids"]))
}
