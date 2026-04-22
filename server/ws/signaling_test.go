package ws_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	crosstalk "github.com/anthropics/crosstalk/server"
	"github.com/anthropics/crosstalk/server/mock"
	ctpion "github.com/anthropics/crosstalk/server/pion"
	"github.com/anthropics/crosstalk/server/ws"
)

const validToken = "ct_test_token_abc123"

// validTokenHash is the SHA-256 hex digest of validToken, pre-computed to
// match what SignalingHandler will compute.
var validTokenHash = ws.HashTokenForTest(validToken)

// testAPI returns a webrtc.API with mDNS disabled for in-process tests.
func testAPI(t *testing.T) *webrtc.API {
	t.Helper()
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	return webrtc.NewAPI(webrtc.WithSettingEngine(se))
}

// testPeerManager returns a PeerManager wired to the mDNS-disabled API.
func testPeerManager(t *testing.T) *ctpion.PeerManager {
	t.Helper()
	return ctpion.NewPeerManagerWithAPI(crosstalk.WebRTCConfig{}, testAPI(t))
}

// newMockTokenService returns a mock.TokenService that recognises validToken.
func newMockTokenService() *mock.TokenService {
	return &mock.TokenService{
		FindTokenByHashFn: func(hash string) (*crosstalk.APIToken, error) {
			if hash == validTokenHash {
				return &crosstalk.APIToken{
					ID:   "tok-1",
					Name: "test-token",
				}, nil
			}
			return nil, errors.New("not found")
		},
	}
}

// setupServer creates an httptest server with the SignalingHandler.
func setupServer(t *testing.T) (*httptest.Server, *ctpion.PeerManager) {
	t.Helper()
	pm := testPeerManager(t)
	handler := &ws.SignalingHandler{
		TokenService:   newMockTokenService(),
		SessionService: &mock.SessionService{
			FindSessionByIDFn: func(id string) (*crosstalk.Session, error) { return nil, fmt.Errorf("not found") },
			ListSessionsFn:    func() ([]crosstalk.Session, error) { return nil, nil },
			CreateSessionFn:   func(s *crosstalk.Session) error { return nil },
			EndSessionFn:      func(id string) error { return nil },
		},
		PeerManager:   pm,
		ServerVersion: "test",
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, pm
}

// wsURL converts an httptest server URL to a WebSocket URL with the given token.
func wsURL(srv *httptest.Server, token string) string {
	u := strings.Replace(srv.URL, "http://", "ws://", 1)
	if token != "" {
		return u + "?token=" + token
	}
	return u
}

// createClientPC creates a client-side PeerConnection with mDNS disabled.
func createClientPC(t *testing.T) *webrtc.PeerConnection {
	t.Helper()
	api := testAPI(t)
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { pc.Close() }) //nolint:errcheck
	return pc
}

func TestSignaling_ValidToken_OfferAnswer(t *testing.T) {
	srv, _ := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connect to WebSocket with valid token.
	conn, _, err := websocket.Dial(ctx, wsURL(srv, validToken), nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// Create a client-side PeerConnection and generate an SDP offer.
	clientPC := createClientPC(t)

	// Client needs at least one data channel for a valid SDP.
	_, err = clientPC.CreateDataChannel("init", nil)
	require.NoError(t, err)

	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)

	gatherDone := webrtc.GatheringCompletePromise(clientPC)
	require.NoError(t, clientPC.SetLocalDescription(offer))
	<-gatherDone

	fullOffer := clientPC.LocalDescription()

	// Send offer over WebSocket.
	offerMsg := ws.SignalMessage{
		Type: "offer",
		SDP:  fullOffer.SDP,
	}
	data, err := json.Marshal(offerMsg)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, data))

	// Read answer back.
	_, respData, err := conn.Read(ctx)
	require.NoError(t, err)

	var answerMsg ws.SignalMessage
	require.NoError(t, json.Unmarshal(respData, &answerMsg))

	assert.Equal(t, "answer", answerMsg.Type)
	assert.NotEmpty(t, answerMsg.SDP)
	assert.Contains(t, answerMsg.SDP, "v=0") // Valid SDP starts with v=0.

	// Set the answer on the client to verify it's a valid SDP.
	err = clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerMsg.SDP,
	})
	require.NoError(t, err)
}

func TestSignaling_InvalidToken_Rejected(t *testing.T) {
	srv, _ := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt to connect with an invalid token.
	_, resp, err := websocket.Dial(ctx, wsURL(srv, "bad_token"), nil)
	require.Error(t, err)

	// The server should reject with 401 before upgrading.
	if resp != nil {
		assert.Equal(t, 401, resp.StatusCode)
	}
}

func TestSignaling_MissingToken_Rejected(t *testing.T) {
	srv, _ := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt to connect without a token parameter.
	_, resp, err := websocket.Dial(ctx, wsURL(srv, ""), nil)
	require.Error(t, err)

	// The server should reject with 401 before upgrading.
	if resp != nil {
		assert.Equal(t, 401, resp.StatusCode)
	}
}

func TestSignaling_ICETrickle(t *testing.T) {
	srv, _ := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connect with valid token.
	conn, _, err := websocket.Dial(ctx, wsURL(srv, validToken), nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// Create client PeerConnection and offer.
	clientPC := createClientPC(t)

	_, err = clientPC.CreateDataChannel("init", nil)
	require.NoError(t, err)

	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)

	gatherDone := webrtc.GatheringCompletePromise(clientPC)
	require.NoError(t, clientPC.SetLocalDescription(offer))
	<-gatherDone

	fullOffer := clientPC.LocalDescription()

	// Send offer.
	offerMsg := ws.SignalMessage{
		Type: "offer",
		SDP:  fullOffer.SDP,
	}
	data, err := json.Marshal(offerMsg)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, data))

	// Read answer.
	_, respData, err := conn.Read(ctx)
	require.NoError(t, err)

	var answerMsg ws.SignalMessage
	require.NoError(t, json.Unmarshal(respData, &answerMsg))
	require.Equal(t, "answer", answerMsg.Type)

	// Set answer on client so remote description is set before adding candidates.
	require.NoError(t, clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerMsg.SDP,
	}))

	// Send an ICE candidate to the server.
	candidate := webrtc.ICECandidateInit{
		Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 50000 typ host",
	}
	iceMsg := ws.SignalMessage{
		Type:      "ice",
		Candidate: &candidate,
	}
	data, err = json.Marshal(iceMsg)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, data))

	// The server should not close the connection after receiving the ICE
	// candidate. Verify by reading — we should either get server-side ICE
	// candidates trickled back or hit the context timeout (both are OK).
	// We just need to confirm no error happens immediately.
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()

	// Read any server ICE candidates that arrive (or timeout, which is fine).
	_, msgData, err := conn.Read(readCtx)
	if err == nil {
		// We got a message — it should be an ICE candidate from the server.
		var msg ws.SignalMessage
		require.NoError(t, json.Unmarshal(msgData, &msg))
		assert.Equal(t, "ice", msg.Type)
		assert.NotNil(t, msg.Candidate)
	} else {
		// Timeout is expected if the server has no candidates to trickle.
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
}
