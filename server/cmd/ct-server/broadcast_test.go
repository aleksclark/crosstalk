package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
	"github.com/pion/ice/v4"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// ---------------------------------------------------------------------------
// Broadcast Token Endpoint Tests (Step 1)
// ---------------------------------------------------------------------------

func TestBroadcastToken_RequiresAuth(t *testing.T) {
	baseURL, _, _ := testServerFull(t, testServerOpts{})

	// POST without auth token should return 401.
	resp := apiDo(t, "POST", baseURL+"/api/sessions/nonexistent/broadcast-token", "", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBroadcastToken_SessionNotFound(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// POST with auth but a non-existent session ID.
	resp := apiDo(t, "POST", baseURL+"/api/sessions/does-not-exist/broadcast-token", token, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBroadcastToken_SessionEnded(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// Create a template with broadcast mapping.
	tmplBody := map[string]any{
		"name": "broadcast-ended-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "broadcast"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)
	tmplID := tmpl["id"].(string)

	// Create and then end a session.
	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmplID,
		"name":        "ended-session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID := session["id"].(string)

	// End the session.
	resp = apiDo(t, "DELETE", baseURL+"/api/sessions/"+sessionID, token, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Try to create broadcast token for ended session.
	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBroadcastToken_NoBroadcastMapping(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// Create a template WITHOUT broadcast mapping.
	tmplBody := map[string]any{
		"name": "no-broadcast-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
			{"name": "translator", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "translator:output"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)
	tmplID := tmpl["id"].(string)

	// Create a session.
	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmplID,
		"name":        "no-broadcast-session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID := session["id"].(string)

	// Try to create broadcast token — should fail with 400.
	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := apiJSON[map[string]any](t, resp)
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, errObj["message"], "no broadcast mappings")
}

func TestBroadcastToken_Success(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// Create a template with broadcast mapping.
	tmplBody := map[string]any{
		"name": "broadcast-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "broadcast"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)
	tmplID := tmpl["id"].(string)

	// Create a session.
	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmplID,
		"name":        "broadcast-session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID := session["id"].(string)

	// Create broadcast token — should succeed.
	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := apiJSON[map[string]any](t, resp)

	// Verify token format.
	btToken, ok := body["token"].(string)
	require.True(t, ok, "response should contain token")
	assert.True(t, strings.HasPrefix(btToken, "ctb_"), "token should start with ctb_ prefix")

	// Verify URL format.
	url, ok := body["url"].(string)
	require.True(t, ok, "response should contain url")
	assert.Contains(t, url, "/listen/"+sessionID)
	assert.Contains(t, url, "token="+btToken)

	// Verify expires_at is present and in the future.
	expiresAtStr, ok := body["expires_at"].(string)
	require.True(t, ok, "response should contain expires_at")
	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	require.NoError(t, err)
	assert.True(t, expiresAt.After(time.Now()), "expires_at should be in the future")
}

func TestBroadcastToken_TokenValidation(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// Create a template with broadcast mapping.
	tmplBody := map[string]any{
		"name": "validation-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "broadcast"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)
	tmplID := tmpl["id"].(string)

	// Create a session.
	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmplID,
		"name":        "validation-session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID := session["id"].(string)

	// Create broadcast token.
	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := apiJSON[map[string]any](t, resp)

	btToken := body["token"].(string)
	require.True(t, strings.HasPrefix(btToken, "ctb_"))

	// Creating a second token should also succeed (multiple tokens allowed).
	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body2 := apiJSON[map[string]any](t, resp)
	btToken2 := body2["token"].(string)
	assert.NotEqual(t, btToken, btToken2, "each call should produce a unique token")
}

// ---------------------------------------------------------------------------
// Broadcast Info Endpoint Tests (Step 2)
// ---------------------------------------------------------------------------

func TestBroadcastInfo_Success(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// Create template + session.
	tmplBody := map[string]any{
		"name": "info-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "broadcast"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)

	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmpl["id"].(string),
		"name":        "Info Session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID := session["id"].(string)

	// GET broadcast info (public, no auth).
	resp = apiDo(t, "GET", baseURL+"/api/sessions/"+sessionID+"/broadcast", "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	info := apiJSON[map[string]any](t, resp)
	assert.Equal(t, sessionID, info["session_id"])
	assert.Equal(t, "Info Session", info["name"])
	assert.Equal(t, "waiting", info["status"])
}

func TestBroadcastInfo_NotFound(t *testing.T) {
	baseURL, _, _ := testServerFull(t, testServerOpts{})

	// GET broadcast info for non-existent session (public, no auth).
	resp := apiDo(t, "GET", baseURL+"/api/sessions/nonexistent/broadcast", "", nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBroadcastInfo_SessionEnded(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	// Create template + session + end it.
	tmplBody := map[string]any{
		"name": "info-ended-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "broadcast"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)

	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmpl["id"].(string),
		"name":        "ended-info-session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID := session["id"].(string)

	// End session.
	resp = apiDo(t, "DELETE", baseURL+"/api/sessions/"+sessionID, token, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// GET broadcast info for ended session → 410 Gone.
	resp = apiDo(t, "GET", baseURL+"/api/sessions/"+sessionID+"/broadcast", "", nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusGone, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Broadcast WebSocket Signaling Tests (Step 2)
// ---------------------------------------------------------------------------

func TestBroadcastWS_InvalidToken(t *testing.T) {
	baseURL, _, _ := testServerFull(t, testServerOpts{})

	// Attempt WS connect with bad token.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/broadcast?token=bad_token"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.Error(t, err, "should fail with invalid token")
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestBroadcastWS_NoToken(t *testing.T) {
	baseURL, _, _ := testServerFull(t, testServerOpts{})

	// Attempt WS connect without token.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/broadcast"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.Error(t, err, "should fail without token")
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

// createBroadcastSession is a test helper that creates a template with a broadcast
// mapping, creates a session, and generates a broadcast token. Returns
// (sessionID, broadcastToken).
func createBroadcastSession(t *testing.T, baseURL, token string) (sessionID, broadcastToken string) {
	t.Helper()

	tmplBody := map[string]any{
		"name": "broadcast-ws-test",
		"roles": []map[string]any{
			{"name": "studio", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "studio:mic", "sink": "broadcast"},
		},
	}
	resp := apiDo(t, "POST", baseURL+"/api/templates", token, tmplBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tmpl := apiJSON[map[string]any](t, resp)

	resp = apiDo(t, "POST", baseURL+"/api/sessions", token, map[string]string{
		"template_id": tmpl["id"].(string),
		"name":        "broadcast-ws-session",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	session := apiJSON[map[string]any](t, resp)
	sessionID = session["id"].(string)

	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	btResp := apiJSON[map[string]any](t, resp)
	broadcastToken = btResp["token"].(string)

	return sessionID, broadcastToken
}

func TestBroadcastWS_ValidToken_Connects(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	sessionID, btToken := createBroadcastSession(t, baseURL, token)
	_ = sessionID

	// Connect to broadcast WS with valid token.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/broadcast?token=" + btToken

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wsConn, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "WS dial should succeed with valid broadcast token")
	defer wsConn.Close(websocket.StatusNormalClosure, "done")

	// Create a client PeerConnection for the listener (receive-only).
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	clientAPI := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	clientPC, err := clientAPI.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { clientPC.Close() })

	// Add a recvonly audio transceiver so the SDP has an audio section.
	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	require.NoError(t, err)

	// Create offer.
	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)
	gatherDone := webrtc.GatheringCompletePromise(clientPC)
	require.NoError(t, clientPC.SetLocalDescription(offer))
	<-gatherDone
	fullOffer := *clientPC.LocalDescription()

	// Send offer.
	offerMsg, err := json.Marshal(map[string]string{
		"type": "offer",
		"sdp":  fullOffer.SDP,
	})
	require.NoError(t, err)
	require.NoError(t, wsConn.Write(ctx, websocket.MessageText, offerMsg))

	// Read answer.
	answerSDP := readAnswer(t, ctx, wsConn)
	require.NotEmpty(t, answerSDP, "should receive SDP answer")

	// Set remote description.
	require.NoError(t, clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}))

	// Wait for ICE connected.
	iceConnected := make(chan struct{})
	clientPC.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			close(iceConnected)
		}
	})

	select {
	case <-iceConnected:
		t.Log("Broadcast listener ICE connected")
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for ICE connected on broadcast listener")
	}
}

func TestBroadcastWS_ReceivesAudio(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	sessionID, btToken := createBroadcastSession(t, baseURL, token)

	// --- Step 1: Connect studio peer and join session ---
	studio := newTestClient(t, baseURL, token)
	studio.sendHello()
	studio.expectWelcome(5 * time.Second)
	studio.sendJoinSession(sessionID, "studio")
	studio.expectSessionEvent(5 * time.Second) // CLIENT_JOINED

	// Give orchestrator time to set up broadcast binding.
	time.Sleep(300 * time.Millisecond)

	// --- Step 2: Add audio track from studio ---
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"studio-mic",
		"studio-mic",
	)
	require.NoError(t, err)

	sender, err := studio.pc.AddTrack(audioTrack)
	require.NoError(t, err)

	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	}()

	// Renegotiate studio peer.
	offer, err := studio.pc.CreateOffer(nil)
	require.NoError(t, err)
	gatherDone := webrtc.GatheringCompletePromise(studio.pc)
	require.NoError(t, studio.pc.SetLocalDescription(offer))
	<-gatherDone
	fullOffer := *studio.pc.LocalDescription()

	offerMsg, err := json.Marshal(map[string]string{
		"type": "offer",
		"sdp":  fullOffer.SDP,
	})
	require.NoError(t, err)
	require.NoError(t, studio.wsConn.Write(studio.ctx, websocket.MessageText, offerMsg))

	answerSDP := readAnswer(t, studio.ctx, studio.wsConn)
	require.NoError(t, studio.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}))

	// Start sending RTP packets.
	opusSilence := []byte{0xF8, 0xFF, 0xFE}
	go func() {
		for i := range 500 {
			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    111,
					SequenceNumber: uint16(i),
					Timestamp:      uint32(i * 960),
					SSRC:           99999,
				},
				Payload: opusSilence,
			}
			data, _ := pkt.Marshal()
			audioTrack.Write(data) //nolint:errcheck
			time.Sleep(20 * time.Millisecond)
		}
	}()

	// --- Step 3: Connect broadcast listener ---
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/broadcast?token=" + btToken
	listenerWS, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer listenerWS.Close(websocket.StatusNormalClosure, "done")

	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	clientAPI := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	listenerPC, err := clientAPI.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { listenerPC.Close() })

	// Track channel for the listener.
	listenerTrackCh := make(chan *webrtc.TrackRemote, 4)
	listenerPC.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		select {
		case listenerTrackCh <- track:
		default:
		}
	})

	// Add a recvonly audio transceiver.
	_, err = listenerPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	require.NoError(t, err)

	// Create and send offer.
	listenerOffer, err := listenerPC.CreateOffer(nil)
	require.NoError(t, err)
	listenerGatherDone := webrtc.GatheringCompletePromise(listenerPC)
	require.NoError(t, listenerPC.SetLocalDescription(listenerOffer))
	<-listenerGatherDone
	listenerFullOffer := *listenerPC.LocalDescription()

	listenerOfferMsg, err := json.Marshal(map[string]string{
		"type": "offer",
		"sdp":  listenerFullOffer.SDP,
	})
	require.NoError(t, err)
	require.NoError(t, listenerWS.Write(ctx, websocket.MessageText, listenerOfferMsg))

	// Read initial answer BEFORE starting the renegotiation handler.
	listenerAnswerSDP := readAnswer(t, ctx, listenerWS)
	require.NotEmpty(t, listenerAnswerSDP)
	require.NoError(t, listenerPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  listenerAnswerSDP,
	}))

	// NOW start the renegotiation handler goroutine to handle server-initiated
	// offers (track additions).
	go func() {
		for {
			readCtx, readCancel := context.WithTimeout(ctx, 30*time.Second)
			_, data, readErr := listenerWS.Read(readCtx)
			readCancel()
			if readErr != nil {
				return
			}

			var msg struct {
				Type      string                   `json:"type"`
				SDP       string                   `json:"sdp,omitempty"`
				Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}

			switch msg.Type {
			case "offer":
				sdpOffer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.SDP}
				if err := listenerPC.SetRemoteDescription(sdpOffer); err != nil {
					continue
				}
				ans, err := listenerPC.CreateAnswer(nil)
				if err != nil {
					continue
				}
				if err := listenerPC.SetLocalDescription(ans); err != nil {
					continue
				}
				respData, _ := json.Marshal(map[string]string{
					"type": "answer",
					"sdp":  listenerPC.LocalDescription().SDP,
				})
				listenerWS.Write(ctx, websocket.MessageText, respData) //nolint:errcheck
			case "ice":
				if msg.Candidate != nil {
					listenerPC.AddICECandidate(*msg.Candidate) //nolint:errcheck
				}
			}
		}
	}()

	// Wait for a track to arrive on the listener.
	select {
	case track := <-listenerTrackCh:
		t.Logf("Broadcast listener received track: codec=%s", track.Codec().MimeType)
		assert.Contains(t, strings.ToLower(track.Codec().MimeType), "opus")

		// Read a few RTP packets to verify data flows.
		buf := make([]byte, 1500)
		for i := range 3 {
			n, _, readErr := track.Read(buf)
			require.NoError(t, readErr, "should be able to read RTP from broadcast track")
			assert.Greater(t, n, 0, "RTP packet %d should have nonzero length", i)
		}
		t.Log("Successfully read RTP packets from broadcast track")

	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for broadcast track on listener")
	}
}

// ---------------------------------------------------------------------------
// Listener Count Tracking Tests (Step 3)
// ---------------------------------------------------------------------------

// connectBroadcastListener connects a broadcast listener via WebSocket and
// completes WebRTC signaling. It returns a cleanup function that closes the
// WebSocket connection (triggering RemoveListener on the server).
func connectBroadcastListener(t *testing.T, baseURL, btToken string) (cleanup func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/broadcast?token=" + btToken
	wsConn, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "broadcast listener WS dial should succeed")

	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	clientAPI := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	pc, err := clientAPI.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)

	// Add a recvonly audio transceiver.
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	require.NoError(t, err)

	// Create and send offer.
	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	gatherDone := webrtc.GatheringCompletePromise(pc)
	require.NoError(t, pc.SetLocalDescription(offer))
	<-gatherDone
	fullOffer := *pc.LocalDescription()

	offerMsg, err := json.Marshal(map[string]string{
		"type": "offer",
		"sdp":  fullOffer.SDP,
	})
	require.NoError(t, err)
	require.NoError(t, wsConn.Write(ctx, websocket.MessageText, offerMsg))

	// Read answer.
	answerSDP := readAnswer(t, ctx, wsConn)
	require.NotEmpty(t, answerSDP)
	require.NoError(t, pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}))

	// Wait for ICE connected.
	iceConnected := make(chan struct{})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			select {
			case <-iceConnected:
			default:
				close(iceConnected)
			}
		}
	})

	select {
	case <-iceConnected:
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for broadcast listener ICE connected")
	}

	return func() {
		wsConn.Close(websocket.StatusNormalClosure, "done")
		pc.Close()
		cancel()
	}
}

func TestBroadcastListenerCount_REST(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	sessionID, btToken := createBroadcastSession(t, baseURL, token)

	// Initially listener_count should be 0.
	resp := apiDo(t, "GET", baseURL+"/api/sessions/"+sessionID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	detail := apiJSON[map[string]any](t, resp)
	assert.Equal(t, float64(0), detail["listener_count"],
		"listener_count should be 0 with no listeners")

	// Connect a broadcast listener.
	cleanup := connectBroadcastListener(t, baseURL, btToken)

	// Give the orchestrator a moment to register the listener.
	time.Sleep(500 * time.Millisecond)

	// Now listener_count should be 1.
	resp = apiDo(t, "GET", baseURL+"/api/sessions/"+sessionID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	detail = apiJSON[map[string]any](t, resp)
	assert.Equal(t, float64(1), detail["listener_count"],
		"listener_count should be 1 after one listener connects")

	// Connect a second broadcast listener (need a new token since they're unique).
	resp = apiDo(t, "POST", baseURL+"/api/sessions/"+sessionID+"/broadcast-token", token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	btResp2 := apiJSON[map[string]any](t, resp)
	btToken2 := btResp2["token"].(string)

	cleanup2 := connectBroadcastListener(t, baseURL, btToken2)

	time.Sleep(500 * time.Millisecond)

	// listener_count should be 2.
	resp = apiDo(t, "GET", baseURL+"/api/sessions/"+sessionID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	detail = apiJSON[map[string]any](t, resp)
	assert.Equal(t, float64(2), detail["listener_count"],
		"listener_count should be 2 after two listeners connect")

	// Disconnect one listener.
	cleanup()

	time.Sleep(500 * time.Millisecond)

	// listener_count should be back to 1.
	resp = apiDo(t, "GET", baseURL+"/api/sessions/"+sessionID, token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	detail = apiJSON[map[string]any](t, resp)
	assert.Equal(t, float64(1), detail["listener_count"],
		"listener_count should be 1 after one listener disconnects")

	// Cleanup second listener.
	cleanup2()
}

func TestBroadcastListenerCount_Event(t *testing.T) {
	baseURL, token, _ := testServerFull(t, testServerOpts{})

	sessionID, btToken := createBroadcastSession(t, baseURL, token)

	// Connect an authenticated peer that will receive events.
	admin := newTestClient(t, baseURL, token)
	admin.sendHello()
	admin.expectWelcome(5 * time.Second)
	admin.sendJoinSession(sessionID, "studio")
	admin.expectSessionEvent(5 * time.Second) // CLIENT_JOINED

	// Give orchestrator time to set up.
	time.Sleep(300 * time.Millisecond)

	// Connect a broadcast listener.
	cleanup := connectBroadcastListener(t, baseURL, btToken)

	// The admin should receive a SESSION_LISTENER_COUNT_CHANGED event.
	ev := admin.drainUntilSessionEvent(
		crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED,
		5*time.Second,
	)
	require.NotNil(t, ev)
	assert.Equal(t, crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED, ev.GetType())
	assert.Contains(t, ev.GetMessage(), "listener_count:1")
	assert.Equal(t, sessionID, ev.GetSessionId())

	// Disconnect the listener.
	cleanup()

	// The admin should receive another event with count 0.
	ev = admin.drainUntilSessionEvent(
		crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED,
		5*time.Second,
	)
	require.NotNil(t, ev)
	assert.Equal(t, crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED, ev.GetType())
	assert.Contains(t, ev.GetMessage(), "listener_count:0")
}
