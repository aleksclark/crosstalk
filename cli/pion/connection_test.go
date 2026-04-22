package pion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"
)

// mockSignalingServer creates a test server that handles WebSocket signaling
// and acts as a WebRTC peer for testing. It speaks protobuf on the control
// channel, matching the real server behavior.
func mockSignalingServer(t *testing.T) (*httptest.Server, *mockServerState) {
	t.Helper()
	state := &mockServerState{
		helloReceived: make(chan *crosstalkv1.Hello, 1),
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
	helloReceived  chan *crosstalkv1.Hello
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

			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				var cm crosstalkv1.ControlMessage
				if err := proto.Unmarshal(msg.Data, &cm); err == nil {
					if hello := cm.GetHello(); hello != nil {
						s.helloReceived <- hello
						welcome := &crosstalkv1.ControlMessage{
							Payload: &crosstalkv1.ControlMessage_Welcome{
								Welcome: &crosstalkv1.Welcome{
									ClientId:      "test-client-1",
									ServerVersion: "0.1.0-test",
								},
							},
						}
						data, _ := proto.Marshal(welcome)
						dc.Send(data)
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
		candidateJSON := candidate.ToJSON()
		candidateBytes, _ := json.Marshal(candidateJSON)
		msg := crosstalk.SignalingMessage{
			Type:      "ice",
			Candidate: json.RawMessage(candidateBytes),
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

		if msg.Type == "ice" && len(msg.Candidate) > 0 {
			var candidateInit webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Candidate, &candidateInit); err == nil {
				pc.AddICECandidate(candidateInit)
			}
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
	var welcomeMsg *crosstalkv1.ControlMessage
	welcomeReceived := make(chan struct{}, 1)

	conn := NewConnection(srv.URL, "test-wrt-token",
		WithOnControlOpen(func() {
			controlOpened <- struct{}{}
		}),
		WithOnControlMessage(func(data []byte) {
			msg, err := ParseControlMessage(data)
			if err == nil && msg.GetWelcome() != nil {
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
	case hello := <-state.helloReceived:
		require.Len(t, hello.GetSources(), 1)
		assert.Equal(t, "mic-1", hello.GetSources()[0].GetName())
		assert.Equal(t, "audio", hello.GetSources()[0].GetType())
		require.Len(t, hello.GetSinks(), 1)
		assert.Equal(t, "speakers-1", hello.GetSinks()[0].GetName())
		require.Len(t, hello.GetCodecs(), 1)
		assert.Equal(t, "opus/48000/2", hello.GetCodecs()[0].GetName())
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Hello message on server")
	}

	// Verify we received Welcome from server
	select {
	case <-welcomeReceived:
		require.NotNil(t, welcomeMsg)
		welcome := welcomeMsg.GetWelcome()
		require.NotNil(t, welcome)
		assert.Equal(t, "test-client-1", welcome.GetClientId())
		assert.Equal(t, "0.1.0-test", welcome.GetServerVersion())
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
	welcome := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_Welcome{
			Welcome: &crosstalkv1.Welcome{
				ClientId:      "c1",
				ServerVersion: "1.0",
			},
		},
	}
	data, err := proto.Marshal(welcome)
	require.NoError(t, err)

	msg, err := ParseControlMessage(data)
	require.NoError(t, err)
	w := msg.GetWelcome()
	require.NotNil(t, w)
	assert.Equal(t, "c1", w.GetClientId())
	assert.Equal(t, "1.0", w.GetServerVersion())
}

func TestParseControlMessage_Invalid(t *testing.T) {
	_, err := ParseControlMessage([]byte("not protobuf"))
	assert.Error(t, err)
}

func TestSendJoinSession(t *testing.T) {
	srv, state := mockSignalingServer(t)
	defer srv.Close()

	controlOpened := make(chan struct{}, 1)

	// Capture messages on server side
	joinReceived := make(chan *crosstalkv1.JoinSession, 1)

	conn := NewConnection(srv.URL, "test-wrt-token",
		WithOnControlOpen(func() {
			controlOpened <- struct{}{}
		}),
		WithOnControlMessage(func(data []byte) {}),
	)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- conn.Connect(ctx)
	}()

	select {
	case <-controlOpened:
	case err := <-connectDone:
		t.Fatalf("Connect returned before control opened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for control channel to open")
	}

	// Set up server-side message capture for JoinSession
	state.mu.Lock()
	dc := state.controlDC
	state.mu.Unlock()
	require.NotNil(t, dc)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			if join := cm.GetJoinSession(); join != nil {
				joinReceived <- join
			}
		}
	})

	err := conn.SendJoinSession("session-abc", "host")
	require.NoError(t, err)

	select {
	case join := <-joinReceived:
		assert.Equal(t, "session-abc", join.GetSessionId())
		assert.Equal(t, "host", join.GetRole())
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for JoinSession on server")
	}
}

func TestSendLogEntry(t *testing.T) {
	srv, state := mockSignalingServer(t)
	defer srv.Close()

	controlOpened := make(chan struct{}, 1)
	logReceived := make(chan *crosstalkv1.LogEntry, 1)

	conn := NewConnection(srv.URL, "test-wrt-token",
		WithOnControlOpen(func() {
			controlOpened <- struct{}{}
		}),
		WithOnControlMessage(func(data []byte) {}),
	)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- conn.Connect(ctx)
	}()

	select {
	case <-controlOpened:
	case err := <-connectDone:
		t.Fatalf("Connect returned before control opened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for control channel to open")
	}

	state.mu.Lock()
	dc := state.controlDC
	state.mu.Unlock()
	require.NotNil(t, dc)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			if entry := cm.GetLogEntry(); entry != nil {
				logReceived <- entry
			}
		}
	})

	err := conn.SendLogEntry(crosstalkv1.LogSeverity_LOG_WARN, "cli-test", "something happened")
	require.NoError(t, err)

	select {
	case entry := <-logReceived:
		assert.Equal(t, crosstalkv1.LogSeverity_LOG_WARN, entry.GetSeverity())
		assert.Equal(t, "cli-test", entry.GetSource())
		assert.Equal(t, "something happened", entry.GetMessage())
		assert.Greater(t, entry.GetTimestamp(), int64(0))
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for LogEntry on server")
	}
}
