package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// --- WebSocket admission (before peer allocation) ---

func TestWebSocket_MissingTicketNoPeer(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, _ := env.createSession(t, adminTok, "WS")
	_ = env.createChannel(t, adminTok, sid, "Floor", "feed")

	beforePeers := env.peerCount()
	beforeSrc := env.sourceCount(t, sid)

	conn, resp, err := env.dialWS(t, "/api/sessions/"+sid+"/ws")
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	assert.Equal(t, beforePeers, env.peerCount())
	assert.Equal(t, beforeSrc, env.sourceCount(t, sid))
}

func TestWebSocket_MalformedForgedNoPeer(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, _ := env.createSession(t, adminTok, "WS")
	_ = env.createChannel(t, adminTok, sid, "Floor", "feed")

	// Acquire lease so generation exists.
	_, _ = env.leases.Acquire(context.Background(), sid, "test-api", time.Minute)

	beforePeers := env.peerCount()
	beforeSrc := env.sourceCount(t, sid)

	for _, tok := range []string{"forged", "deadbeefdeadbeefdeadbeefdeadbeef", "eyJhbGciOiJIUzI1NiJ9.e30.sig"} {
		conn, resp, err := env.dialWS(t, "/api/sessions/"+sid+"/ws?token="+tok)
		if conn != nil {
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
		require.Error(t, err, "token=%s", tok)
		if resp != nil {
			assert.True(t, resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden,
				"token=%s status=%d", tok, resp.StatusCode)
		}
		assert.Equal(t, beforePeers, env.peerCount(), "token=%s", tok)
		assert.Equal(t, beforeSrc, env.sourceCount(t, sid), "token=%s", tok)
	}
}

func TestWebSocket_ReplayTicketSecondFails(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, _ := env.createSession(t, adminTok, "Replay")
	_ = env.createChannel(t, adminTok, sid, "Floor", "feed")
	_ = env.createChannel(t, adminTok, sid, "English", "broadcast")

	ticket, _ := env.issueTicket(t, adminTok, sid, nil, nil)

	beforePeers := env.peerCount()
	beforeSrc := env.sourceCount(t, sid)

	// First consume should upgrade (peer may be created). We only need admission.
	conn1, resp1, err1 := env.dialWS(t, "/api/sessions/"+sid+"/ws?token="+ticket)
	// Success path: websocket accepted (err1 nil) OR we got past HTTP.
	if err1 == nil {
		require.NotNil(t, conn1)
		_ = conn1.Close(websocket.StatusNormalClosure, "")
		// Allow peer cleanup.
		time.Sleep(50 * time.Millisecond)
	} else if resp1 != nil {
		// If dial failed for non-auth reasons, still try replay semantics via consume.
		t.Logf("first dial err=%v status=%v", err1, resp1.StatusCode)
	}

	// After first successful admission the ticket is consumed. Second must fail
	// without leaving a net-new peer beyond what first created (and cleaned).
	peersAfterFirst := env.peerCount()
	srcAfterFirst := env.sourceCount(t, sid)

	conn2, resp2, err2 := env.dialWS(t, "/api/sessions/"+sid+"/ws?token="+ticket)
	if conn2 != nil {
		_ = conn2.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err2)
	if resp2 != nil {
		assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
	}
	// No additional peers/sources from the replay attempt.
	assert.Equal(t, peersAfterFirst, env.peerCount())
	assert.Equal(t, srcAfterFirst, env.sourceCount(t, sid))
	// And if first never connected, still no peers.
	if err1 != nil {
		assert.Equal(t, beforePeers, env.peerCount())
		assert.Equal(t, beforeSrc, env.sourceCount(t, sid))
	}
}

func TestWebSocket_TicketSessionMismatchForbidden(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sidA, _ := env.createSession(t, adminTok, "A")
	sidB, _ := env.createSession(t, adminTok, "B")
	_ = env.createChannel(t, adminTok, sidA, "Floor", "feed")
	_ = env.createChannel(t, adminTok, sidB, "Floor", "feed")
	_ = env.createChannel(t, adminTok, sidA, "En", "broadcast")
	_ = env.createChannel(t, adminTok, sidB, "En", "broadcast")

	ticketA, _ := env.issueTicket(t, adminTok, sidA, nil, nil)

	beforePeers := env.peerCount()
	beforeSrcB := env.sourceCount(t, sidB)

	conn, resp, err := env.dialWS(t, "/api/sessions/"+sidB+"/ws?token="+ticketA)
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	}
	assert.Equal(t, beforePeers, env.peerCount())
	assert.Equal(t, beforeSrcB, env.sourceCount(t, sidB))
}

func TestWebSocket_TranslatorACannotAccessSessionB(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	tr := env.createUser(t, "tr", "tr123", "translator")
	trTok := env.login(t, "tr", "tr123")

	sidA, _ := env.createSession(t, adminTok, "A")
	sidB, _ := env.createSession(t, adminTok, "B")
	_ = env.createChannel(t, adminTok, sidB, "Floor", "feed")
	require.NoError(t, env.users.AssignSessions(context.Background(), tr.ID, []string{sidA}))

	beforePeers := env.peerCount()
	beforeSrc := env.sourceCount(t, sidB)

	// Access JWT is not a media admit credential when MediaTickets is configured.
	// Unauthorized (ticket required), not assignment 403 — JWT never reaches authz.
	conn, resp, err := env.dialWS(t, "/api/sessions/"+sidB+"/ws?token="+trTok)
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	assert.Equal(t, beforePeers, env.peerCount())
	assert.Equal(t, beforeSrc, env.sourceCount(t, sidB))

	// Ticket mint for unassigned session is still forbidden at issue time.
	respIssue, dataIssue := env.doJSON(t, http.MethodPost, "/api/webrtc/token", trTok, map[string]any{
		"session_id": sidB,
	})
	assert.Equal(t, http.StatusForbidden, respIssue.StatusCode, string(dataIssue))
}

func TestWebSocket_AccessJWTRejectedWhenTicketsConfigured(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, _ := env.createSession(t, adminTok, "JWT-Reject")
	_ = env.createChannel(t, adminTok, sid, "Floor", "feed")
	_ = env.createChannel(t, adminTok, sid, "English", "broadcast")

	beforePeers := env.peerCount()
	beforeSrc := env.sourceCount(t, sid)

	// Even a valid assigned access JWT must not dial media WS directly.
	conn, resp, err := env.dialWS(t, "/api/sessions/"+sid+"/ws?token="+adminTok)
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	assert.Equal(t, beforePeers, env.peerCount())
	assert.Equal(t, beforeSrc, env.sourceCount(t, sid))
}

func TestWebSocket_UnknownABCNoPeer(t *testing.T) {
	env := setupAuthServer(t)
	beforePeers := env.peerCount()

	conn, resp, err := env.dialWS(t, "/ws/signaling?token=not-a-real-abc-token")
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	assert.Equal(t, beforePeers, env.peerCount())
}

func TestWebSocket_ABCOnlyAssignedSession(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sidA, _ := env.createSession(t, adminTok, "A")
	sidB, _ := env.createSession(t, adminTok, "B")
	_ = env.createChannel(t, adminTok, sidA, "Floor", "feed")
	_ = env.createChannel(t, adminTok, sidB, "Floor", "feed")

	// Create ABC assigned to A.
	resp, data := env.doJSON(t, http.MethodPost, "/api/abcs", adminTok, map[string]string{"name": "Booth1"})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(data, &created))
	_, data = env.doJSON(t, http.MethodPut, "/api/abcs/"+created.ID, adminTok, map[string]any{
		"session_id": sidA,
	})
	_ = data

	// Ticket for wrong session rejected.
	resp, data = env.doJSON(t, http.MethodPost, "/api/webrtc/token", created.Token, map[string]any{
		"session_id": sidB,
	})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(data))

	// Ticket for assigned session OK.
	ticket, _ := env.issueTicket(t, created.Token, sidA, nil, nil)
	assert.NotEmpty(t, ticket)
}

func TestWebSocket_BroadcastTokenRotationRejectsOld(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, oldTok := env.createSession(t, adminTok, "Bcast")
	_ = env.createChannel(t, adminTok, sid, "English", "broadcast")

	beforePeers := env.peerCount()

	// Old token works initially.
	conn, resp, err := env.dialWS(t, "/ws/broadcast/"+sid+"?token="+oldTok)
	if err == nil {
		require.NotNil(t, conn)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		time.Sleep(50 * time.Millisecond)
	} else if resp != nil {
		t.Logf("initial broadcast dial: %v status=%d", err, resp.StatusCode)
	}

	// Rotate.
	resp, data := env.doJSON(t, http.MethodPost, "/api/sessions/"+sid+"/broadcast-url", adminTok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var rotated struct {
		BroadcastToken string `json:"broadcast_token"`
	}
	require.NoError(t, json.Unmarshal(data, &rotated))
	require.NotEqual(t, oldTok, rotated.BroadcastToken)

	// Old token rejected; no peer growth from failed attempt.
	peersMid := env.peerCount()
	conn2, resp2, err2 := env.dialWS(t, "/ws/broadcast/"+sid+"?token="+oldTok)
	if conn2 != nil {
		_ = conn2.Close(websocket.StatusNormalClosure, "")
	}
	require.Error(t, err2)
	if resp2 != nil {
		assert.Equal(t, http.StatusForbidden, resp2.StatusCode)
	}
	assert.Equal(t, peersMid, env.peerCount())
	_ = beforePeers
}

func TestWebSocket_ValidTicketAdmits(t *testing.T) {
	env := setupAuthServer(t)
	env.createUser(t, "admin", "admin123", "admin")
	adminTok := env.login(t, "admin", "admin123")
	sid, _ := env.createSession(t, adminTok, "OK")
	_ = env.createChannel(t, adminTok, sid, "Floor", "feed")
	_ = env.createChannel(t, adminTok, sid, "English", "broadcast")

	ticket, _ := env.issueTicket(t, adminTok, sid, nil, nil)
	conn, resp, err := env.dialWS(t, "/api/sessions/"+sid+"/ws?token="+ticket)
	require.NoError(t, err, "resp=%v", resp)
	require.NotNil(t, conn)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Peer registration is synchronous after upgrade; give a brief moment.
	require.Eventually(t, func() bool {
		return env.peerCount() >= 1
	}, time.Second, 10*time.Millisecond)
}
