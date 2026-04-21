package pion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	crosstalk "github.com/anthropics/crosstalk/cli"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// mockSignalingServer creates a test server that handles WebSocket signaling
// and acts as a WebRTC peer for testing.
func mockSignalingServer(t *testing.T) (*httptest.Server, *mockServerState) {
	t.Helper()
	state := &mockServerState{
		helloReceived: make(chan *ControlMessage, 1),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/signaling":
			state.handleSignaling(t, w, r)
		case "/api/webrtc/token":
			json.NewEncoder(w).Encode(map[string]string{"token": "test-wrt-token"})
		default:
			http.NotFound(w, r)
		}
	}))

	return srv, state
}

type mockServerState struct {
	mu             sync.Mutex
	peerConn       *webrtc.PeerConnection
	controlDC      *webrtc.DataChannel
	helloReceived  chan *ControlMessage
	clientID       string
}

func (s *mockServerState) handleSignaling(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	// Verify token is in query
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Logf("websocket accept error: %v", err)
		return
	}
	defer ws.CloseNow()

	ctx := r.Context()

	// Create server-side peer connection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{},
	}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		t.Logf("peer connection error: %v", err)
		return
	}
	s.mu.Lock()
	s.peerConn = pc
	s.mu.Unlock()
	defer pc.Close()

	// Handle data channels created by remote peer
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == "control" {
			s.mu.Lock()
			s.controlDC = dc
			s.mu.Unlock()

			dc.OnOpen(func() {
				// Send Welcome message
				welcome := ControlMessage{
					Type: ControlTypeWelcome,
					Welcome: &WelcomeMessage{
						ClientID:      "test-client-1",
						ServerVersion: "0.1.0-test",
					},
				}
				data, _ := json.Marshal(welcome)
				dc.Send(data)
			})

			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				var ctrlMsg ControlMessage
				if err := json.Unmarshal(msg.Data, &ctrlMsg); err == nil {
					if ctrlMsg.Type == ControlTypeHello {
						s.helloReceived <- &ctrlMsg
					}
				}
			})
		}
	})

	// Send ICE candidates to client via WebSocket
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		msg := crosstalk.SignalingMessage{
			Type:      "ice",
			Candidate: candidate.ToJSON().Candidate,
		}
		data, _ := json.Marshal(msg)
		ws.Write(ctx, websocket.MessageText, data)
	})

	// Read client's SDP offer
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Logf("read offer error: %v", err)
		return
	}

	var offerMsg crosstalk.SignalingMessage
	if err := json.Unmarshal(data, &offerMsg); err != nil {
		t.Logf("unmarshal offer error: %v", err)
		return
	}

	if offerMsg.Type != "offer" {
		t.Logf("expected offer, got %s", offerMsg.Type)
		return
	}

	// Set remote description (client's offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerMsg.SDP,
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		t.Logf("set remote desc error: %v", err)
		return
	}

	// Create and set answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.Logf("create answer error: %v", err)
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		t.Logf("set local desc error: %v", err)
		return
	}

	// Send answer to client
	answerMsg := crosstalk.SignalingMessage{
		Type: "answer",
		SDP:  answer.SDP,
	}
	answerData, _ := json.Marshal(answerMsg)
	if err := ws.Write(ctx, websocket.MessageText, answerData); err != nil {
		t.Logf("write answer error: %v", err)
		return
	}

	// Continue reading ICE candidates from client
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			// Connection closed - expected
			break
		}

		var msg crosstalk.SignalingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.Type == "ice" && msg.Candidate != "" {
			pc.AddICECandidate(webrtc.ICECandidateInit{
				Candidate: msg.Candidate,
			})
		}
	}
}

func TestConnection_ConnectAndSendHello(t *testing.T) {
	srv, state := mockSignalingServer(t)
	defer srv.Close()

	// Track connection state changes
	var stateChanges []webrtc.ICEConnectionState
	var statesMu sync.Mutex

	controlOpened := make(chan struct{}, 1)
	var welcomeMsg *ControlMessage
	welcomeReceived := make(chan struct{}, 1)

	conn := NewConnection(srv.URL, "test-wrt-token",
		WithOnControlOpen(func() {
			controlOpened <- struct{}{}
		}),
		WithOnControlMessage(func(data []byte) {
			msg, err := ParseControlMessage(data)
			if err == nil && msg.Type == ControlTypeWelcome {
				welcomeMsg = msg
				welcomeReceived <- struct{}{}
			}
		}),
		WithOnConnectionStateChange(func(s webrtc.ICEConnectionState) {
			statesMu.Lock()
			stateChanges = append(stateChanges, s)
			statesMu.Unlock()
		}),
	)
	defer conn.Close()

	// Start connection in background
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- conn.Connect(ctx)
	}()

	// Wait for control channel to open
	select {
	case <-controlOpened:
		// OK
	case err := <-connectDone:
		t.Fatalf("Connect returned before control opened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for control channel to open")
	}

	// Send Hello
	sources := []crosstalk.Source{{Name: "mic-1", Type: "audio"}}
	sinks := []crosstalk.Sink{{Name: "speakers-1", Type: "audio"}}
	codecs := []crosstalk.Codec{{Name: "opus/48000/2", MediaType: "audio"}}

	err := conn.SendHello(sources, sinks, codecs)
	require.NoError(t, err)

	// Wait for server to receive Hello
	select {
	case helloMsg := <-state.helloReceived:
		require.NotNil(t, helloMsg.Hello)
		assert.Len(t, helloMsg.Hello.Sources, 1)
		assert.Equal(t, "mic-1", helloMsg.Hello.Sources[0].Name)
		assert.Equal(t, "audio", helloMsg.Hello.Sources[0].Type)
		assert.Len(t, helloMsg.Hello.Sinks, 1)
		assert.Equal(t, "speakers-1", helloMsg.Hello.Sinks[0].Name)
		assert.Len(t, helloMsg.Hello.Codecs, 1)
		assert.Equal(t, "opus/48000/2", helloMsg.Hello.Codecs[0].Name)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Hello message on server")
	}

	// Verify we received Welcome from server
	select {
	case <-welcomeReceived:
		require.NotNil(t, welcomeMsg)
		require.NotNil(t, welcomeMsg.Welcome)
		assert.Equal(t, "test-client-1", welcomeMsg.Welcome.ClientID)
		assert.Equal(t, "0.1.0-test", welcomeMsg.Welcome.ServerVersion)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Welcome message")
	}
}

func TestConnection_ControlChannelClosed(t *testing.T) {
	conn := NewConnection("http://localhost", "token")
	err := conn.SendControl([]byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not open")
}

func TestConnection_ConnectionState(t *testing.T) {
	conn := NewConnection("http://localhost", "token")
	assert.Equal(t, webrtc.ICEConnectionStateNew, conn.ConnectionState())
}

func TestHttpToWS(t *testing.T) {
	assert.Equal(t, "ws://example.com/path", httpToWS("http://example.com/path"))
	assert.Equal(t, "wss://example.com/path", httpToWS("https://example.com/path"))
	assert.Equal(t, "ws://example.com/path", httpToWS("ws://example.com/path"))
}

func TestParseControlMessage(t *testing.T) {
	data := `{"type":"welcome","welcome":{"client_id":"c1","server_version":"1.0"}}`
	msg, err := ParseControlMessage([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, ControlTypeWelcome, msg.Type)
	require.NotNil(t, msg.Welcome)
	assert.Equal(t, "c1", msg.Welcome.ClientID)
}

func TestParseControlMessage_Invalid(t *testing.T) {
	_, err := ParseControlMessage([]byte("not json"))
	assert.Error(t, err)
}
