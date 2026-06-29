package webrtc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"

	crosstalkv2 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v2"
)

// testAPI returns a webrtc.API with mDNS disabled (avoids multicast in CI).
func testAPI() *webrtc.API {
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	return webrtc.NewAPI(webrtc.WithSettingEngine(se))
}

// createOfferWithICE creates an offer that includes ICE credentials by adding
// a transceiver first (which Pion requires to include full ICE negotiation).
func createOfferWithICE(t *testing.T) (*webrtc.PeerConnection, webrtc.SessionDescription) {
	t.Helper()
	api := testAPI()
	clientPC, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)

	// Add a transceiver so the offer contains media lines and ICE credentials.
	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	require.NoError(t, err)

	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)
	err = clientPC.SetLocalDescription(offer)
	require.NoError(t, err)

	return clientPC, offer
}

func TestPeerManager_CreateAndList(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	require.NotNil(t, peer)
	assert.NotEmpty(t, peer.ID)
	assert.Equal(t, 1, pm.Count())

	// ListPeers should return the peer.
	peers := pm.ListPeers()
	require.Len(t, peers, 1)
	assert.Equal(t, peer.ID, peers[0].ID)

	// FindPeer.
	found := pm.FindPeer(peer.ID)
	assert.Equal(t, peer, found)

	// Remove.
	pm.RemovePeer(peer.ID)
	assert.Equal(t, 0, pm.Count())
	assert.Nil(t, pm.FindPeer(peer.ID))
}

func TestPeerManager_EventsCapturedOnSDP(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	// Do an SDP exchange to trigger events.
	clientPC, offer := createOfferWithICE(t)
	defer clientPC.Close()

	_, err = peer.HandleOffer(offer)
	require.NoError(t, err)

	// Wait for events to be processed.
	time.Sleep(50 * time.Millisecond)

	events := peer.Events().Snapshot()
	assert.NotEmpty(t, events, "expected events after SDP exchange")

	// Check for SDP events.
	hasSDPEvent := false
	for _, e := range events {
		if e.Type == EventSDPOffer || e.Type == EventSDPAnswer {
			hasSDPEvent = true
			break
		}
	}
	assert.True(t, hasSDPEvent, "expected SDP event")
}

func TestPeerManager_SDPExchange(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	serverPeer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(serverPeer.ID)

	clientPC, offer := createOfferWithICE(t)
	defer clientPC.Close()

	// Server handles offer.
	answer, err := serverPeer.HandleOffer(offer)
	require.NoError(t, err)
	assert.Equal(t, webrtc.SDPTypeAnswer, answer.Type)
	assert.NotEmpty(t, answer.SDP)

	// Verify SDP events were captured.
	events := serverPeer.Events().Snapshot()
	hasOffer := false
	hasAnswer := false
	for _, e := range events {
		if e.Type == EventSDPOffer {
			hasOffer = true
		}
		if e.Type == EventSDPAnswer {
			hasAnswer = true
		}
	}
	assert.True(t, hasOffer, "expected sdp_offer event")
	assert.True(t, hasAnswer, "expected sdp_answer event")

	// Verify SDP stored.
	storedOffer, storedAnswer, found := pm.PeerSDP(serverPeer.ID)
	assert.True(t, found)
	assert.NotEmpty(t, storedOffer)
	assert.NotEmpty(t, storedAnswer)
}

func TestPeerManager_ICECandidateEvents(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	serverPeer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(serverPeer.ID)

	// Register ICE candidate callback (as signaling would).
	candidateReceived := make(chan struct{}, 10)
	serverPeer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			candidateReceived <- struct{}{}
		}
	})

	// Create a client to drive ICE.
	clientPC, offer := createOfferWithICE(t)
	defer clientPC.Close()

	answer, err := serverPeer.HandleOffer(offer)
	require.NoError(t, err)
	err = clientPC.SetRemoteDescription(answer)
	require.NoError(t, err)

	// Wait for at least one ICE candidate event.
	select {
	case <-candidateReceived:
		// Got one - check event ring.
		events := serverPeer.Events().Snapshot()
		hasLocalCandidate := false
		for _, e := range events {
			if e.Type == EventICECandidateLocal {
				hasLocalCandidate = true
				break
			}
		}
		assert.True(t, hasLocalCandidate, "expected ice_candidate_local event")
	case <-time.After(5 * time.Second):
		t.Log("no ICE candidate received (may be expected in CI without network)")
	}
}

func TestPeerState(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	peer.SetClientInfo("abc", "test-client")

	state := peer.State()
	assert.Equal(t, peer.ID, state.ID)
	assert.Equal(t, "abc", state.ClientType)
	assert.Equal(t, "test-client", state.ClientName)
	assert.NotEmpty(t, state.SignalingState)
}

func TestDebugHandler_ListPeers(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	handler := &DebugHandler{PeerManager: pm}

	req := httptest.NewRequest("GET", "/api/debug/peers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["count"])
	peers := resp["peers"].([]any)
	require.Len(t, peers, 1)
	p := peers[0].(map[string]any)
	assert.Equal(t, peer.ID, p["id"])
}

func TestDebugHandler_PeerEvents(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	// Push a test event.
	peer.Events().Push(MakeEvent(peer.ID, EventICEStateChange, map[string]string{"state": "connected"}))

	handler := &DebugHandler{PeerManager: pm}

	req := httptest.NewRequest("GET", "/api/debug/peers/"+peer.ID+"/events", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, peer.ID, resp["peer_id"])
	events := resp["events"].([]any)
	assert.NotEmpty(t, events)
}

func TestDebugHandler_PeerSDP(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	// Simulate SDP exchange with proper offer.
	clientPC, offer := createOfferWithICE(t)
	defer clientPC.Close()

	_, err = peer.HandleOffer(offer)
	require.NoError(t, err)

	handler := &DebugHandler{PeerManager: pm}

	req := httptest.NewRequest("GET", "/api/debug/peers/"+peer.ID+"/sdp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["last_offer"])
	assert.NotEmpty(t, resp["last_answer"])
}

func TestDebugHandler_NotFound(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	handler := &DebugHandler{PeerManager: pm}

	req := httptest.NewRequest("GET", "/api/debug/peers/nonexistent/events", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSignalingHandler_FullExchange(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	handler := &SignalingHandler{
		PeerManager:   pm,
		ServerVersion: "test-1.0",
	}

	// Start test server.
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Connect as WebSocket client.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close(websocket.StatusNormalClosure, "")

	// Create client-side PeerConnection with audio transceiver.
	clientPC, err := testAPI().NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	require.NoError(t, err)

	// Client creates offer.
	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)
	err = clientPC.SetLocalDescription(offer)
	require.NoError(t, err)

	// Send offer via WebSocket.
	msg := SignalMessage{
		Type: "offer",
		SDP:  offer.SDP,
	}
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	err = ws.Write(ctx, websocket.MessageText, data)
	require.NoError(t, err)

	// Read answer.
	_, respData, err := ws.Read(ctx)
	require.NoError(t, err)

	var resp SignalMessage
	require.NoError(t, json.Unmarshal(respData, &resp))
	assert.Equal(t, "answer", resp.Type)
	assert.NotEmpty(t, resp.SDP)
	assert.NotZero(t, resp.Seq)

	// Apply answer to client.
	err = clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  resp.SDP,
	})
	require.NoError(t, err)

	// Verify the server created a peer with events.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, pm.Count())

	peers := pm.ListPeers()
	require.Len(t, peers, 1)
	events := peers[0].Events().Snapshot()
	assert.NotEmpty(t, events)

	// Verify SDP events.
	hasSDP := false
	for _, e := range events {
		if e.Type == EventSDPOffer || e.Type == EventSDPAnswer {
			hasSDP = true
			break
		}
	}
	assert.True(t, hasSDP, "expected SDP events in ring buffer")
}

func TestSignalingHandler_ICETrickle(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	handler := &SignalingHandler{
		PeerManager:   pm,
		ServerVersion: "test-1.0",
	}

	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close(websocket.StatusNormalClosure, "")

	clientPC, err := testAPI().NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	require.NoError(t, err)

	// Exchange SDP first.
	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)
	_ = clientPC.SetLocalDescription(offer)

	msg := SignalMessage{Type: "offer", SDP: offer.SDP}
	data, _ := json.Marshal(msg)
	require.NoError(t, ws.Write(ctx, websocket.MessageText, data))

	_, respData, err := ws.Read(ctx)
	require.NoError(t, err)
	var resp SignalMessage
	require.NoError(t, json.Unmarshal(respData, &resp))
	_ = clientPC.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: resp.SDP})

	// Send a fake ICE candidate.
	iceMsg := SignalMessage{
		Type: "ice",
		Candidate: &webrtc.ICECandidateInit{
			Candidate: "candidate:1 1 udp 2130706431 192.168.1.1 12345 typ host",
		},
	}
	data, _ = json.Marshal(iceMsg)
	require.NoError(t, ws.Write(ctx, websocket.MessageText, data))

	// Give server time to process.
	time.Sleep(200 * time.Millisecond)

	// Verify remote ICE candidate event was captured.
	peers := pm.ListPeers()
	require.Len(t, peers, 1)
	events := peers[0].Events().Snapshot()
	hasRemoteICE := false
	for _, e := range events {
		if e.Type == EventICECandidateRemote {
			hasRemoteICE = true
			break
		}
	}
	assert.True(t, hasRemoteICE, "expected ice_candidate_remote event")
}

func TestControlHandler_HelloWelcome(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	handler := &ControlHandler{
		Peer:          peer,
		ServerVersion: "v3-test",
	}

	// Serialize a Hello message.
	hello := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_Hello{
			Hello: &crosstalkv2.Hello{
				ClientType: "abc",
				ClientName: "test-booth",
				Capabilities: []*crosstalkv2.AudioCapability{
					{Codec: "opus/48000/2", Channels: 2, SampleRate: 48000},
				},
			},
		},
	}
	data, err := proto.Marshal(hello)
	require.NoError(t, err)

	// Simulate receiving the message.
	var receivedHello bool
	handler.OnHello = func(p *PeerConn, h *crosstalkv2.Hello) {
		receivedHello = true
		assert.Equal(t, "abc", h.GetClientType())
		assert.Equal(t, "test-booth", h.GetClientName())
	}
	handler.dispatch(data)

	assert.True(t, receivedHello)

	// Verify events were captured.
	events := peer.Events().Snapshot()
	hasHello := false
	hasWelcome := false
	for _, e := range events {
		if e.Type == EventControlHello {
			hasHello = true
		}
		if e.Type == EventControlWelcome {
			hasWelcome = true
		}
	}
	assert.True(t, hasHello, "expected control_hello event")
	assert.True(t, hasWelcome, "expected control_welcome event")
}

func TestControlHandler_PingPong(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	handler := &ControlHandler{
		Peer:          peer,
		ServerVersion: "v3-test",
	}

	// Send a ping.
	sentAt := time.Now().UnixMilli()
	ping := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_Ping{
			Ping: &crosstalkv2.PingPong{
				SentAt: sentAt,
			},
		},
	}
	data, err := proto.Marshal(ping)
	require.NoError(t, err)

	handler.dispatch(data)

	// Verify ping event was captured.
	events := peer.Events().Snapshot()
	hasPing := false
	hasPong := false
	for _, e := range events {
		if e.Type == EventControlPing {
			hasPing = true
			// Verify latency field.
			var detail map[string]any
			require.NoError(t, json.Unmarshal(e.Detail, &detail))
			assert.Contains(t, detail, "latency_ms")
		}
		if e.Type == EventControlPong {
			hasPong = true
		}
	}
	assert.True(t, hasPing, "expected control_ping event")
	assert.True(t, hasPong, "expected control_pong event")
}

func TestDebugHandler_PeerDetail(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	peer.SetClientInfo("translator", "booth-1")

	handler := &DebugHandler{PeerManager: pm}

	req := httptest.NewRequest("GET", "/api/debug/peers/"+peer.ID, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var state PeerState
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &state))
	assert.Equal(t, peer.ID, state.ID)
	assert.Equal(t, "translator", state.ClientType)
	assert.Equal(t, "booth-1", state.ClientName)
	assert.NotEmpty(t, state.SignalingState)
}

func TestPeerManager_PeerStates(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	p1, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(p1.ID)

	p2, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(p2.ID)

	states := pm.PeerStates()
	assert.Len(t, states, 2)
}

func TestSignalingHandler_Auth(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	handler := &SignalingHandler{
		PeerManager:   pm,
		ServerVersion: "test-1.0",
		AuthFunc: func(r *http.Request) error {
			if r.URL.Query().Get("token") != "valid" {
				return http.ErrAbortHandler
			}
			return nil
		},
	}

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Without valid token.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?token=invalid"
	_, _, err := websocket.Dial(ctx, wsURL, nil)
	assert.Error(t, err)
}

func TestSignalingHandler_MessageSequencing(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	handler := &SignalingHandler{
		PeerManager:   pm,
		ServerVersion: "test-1.0",
	}

	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close(websocket.StatusNormalClosure, "")

	clientPC, err := testAPI().NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	require.NoError(t, err)

	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)
	_ = clientPC.SetLocalDescription(offer)

	// Send offer.
	msg := SignalMessage{Type: "offer", SDP: offer.SDP, Seq: 42}
	data, _ := json.Marshal(msg)
	require.NoError(t, ws.Write(ctx, websocket.MessageText, data))

	// Read answer - should have a sequence number.
	_, respData, err := ws.Read(ctx)
	require.NoError(t, err)

	var resp SignalMessage
	require.NoError(t, json.Unmarshal(respData, &resp))
	assert.Equal(t, "answer", resp.Type)
	assert.NotZero(t, resp.Seq, "response should have sequence number")

	// Verify signaling message events were captured.
	time.Sleep(100 * time.Millisecond)
	peers := pm.ListPeers()
	require.Len(t, peers, 1)
	events := peers[0].Events().Snapshot()

	hasSignalingEvent := false
	for _, e := range events {
		if e.Type == EventSignalingMessage {
			hasSignalingEvent = true
			var detail map[string]any
			_ = json.Unmarshal(e.Detail, &detail)
			// Should have direction field.
			assert.Contains(t, detail, "direction")
			break
		}
	}
	assert.True(t, hasSignalingEvent, "expected signaling_message events")
}
