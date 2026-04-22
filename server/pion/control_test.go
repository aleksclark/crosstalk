package pion

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mock"

	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
)

// controlTestEnv bundles the boilerplate for in-process control channel tests.
// It creates a server PeerConn with an installed ControlHandler and a connected
// client PeerConnection that has received the "control" data channel.
type controlTestEnv struct {
	server    *PeerConn
	client    *webrtc.PeerConnection
	controlDC *webrtc.DataChannel // client-side "control" data channel
	handler   *ControlHandler
	sessions  *mock.SessionService
}

// setupControlTest creates a connected client+server pair with the
// ControlHandler installed on the server side. It blocks until the client has
// received the "control" data channel and ICE is connected.
func setupControlTest(t *testing.T) *controlTestEnv {
	t.Helper()

	pm := testPeerManager(t)
	server, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { server.Close() }) //nolint:errcheck

	sessions := &mock.SessionService{}
	handler := &ControlHandler{
		Peer:           server,
		SessionService: sessions,
		ServerVersion:  "test-0.1.0",
	}
	handler.Install()

	client := createClientPC(t)

	// Capture the "control" data channel when the client receives it.
	controlReady := make(chan *webrtc.DataChannel, 1)
	client.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "control" {
			return
		}
		dc.OnOpen(func() {
			controlReady <- dc
		})
	})

	// Client needs at least one data channel for the offer SDP.
	_, err = client.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// Wait for ICE connected.
	connected := make(chan struct{})
	client.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			close(connected)
		}
	})

	signalPair(t, client, server)

	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for ICE connected state")
	}

	var dc *webrtc.DataChannel
	select {
	case dc = <-controlReady:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for control data channel")
	}

	return &controlTestEnv{
		server:    server,
		client:    client,
		controlDC: dc,
		handler:   handler,
		sessions:  sessions,
	}
}

// sendProto marshals a ControlMessage and sends it on the client-side control
// data channel.
func (env *controlTestEnv) sendProto(t *testing.T, msg *crosstalkv1.ControlMessage) {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	require.NoError(t, env.controlDC.Send(data))
}

// recvProto waits for the next message on the client-side control data channel
// and unmarshals it as a ControlMessage.
func (env *controlTestEnv) recvProto(t *testing.T) *crosstalkv1.ControlMessage {
	t.Helper()
	ch := make(chan []byte, 1)
	env.controlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		ch <- msg.Data
	})
	select {
	case data := <-ch:
		var cm crosstalkv1.ControlMessage
		require.NoError(t, proto.Unmarshal(data, &cm))
		return &cm
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for control message from server")
		return nil
	}
}

func TestControlHandler_HelloWelcome(t *testing.T) {
	env := setupControlTest(t)

	// Set up receiver before sending.
	respCh := make(chan []byte, 1)
	env.controlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		respCh <- msg.Data
	})

	hello := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_Hello{
			Hello: &crosstalkv1.Hello{
				Sources: []*crosstalkv1.SourceInfo{{Name: "mic", Type: "audio"}},
				Sinks:   []*crosstalkv1.SinkInfo{{Name: "speaker", Type: "audio"}},
				Codecs:  []*crosstalkv1.CodecInfo{{Name: "opus/48000/2", MediaType: "audio"}},
			},
		},
	}
	env.sendProto(t, hello)

	select {
	case data := <-respCh:
		var resp crosstalkv1.ControlMessage
		require.NoError(t, proto.Unmarshal(data, &resp))

		welcome := resp.GetWelcome()
		require.NotNil(t, welcome, "expected Welcome response")
		assert.Equal(t, env.server.ID, welcome.GetClientId())
		assert.Equal(t, "test-0.1.0", welcome.GetServerVersion())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Welcome response")
	}
}

func TestControlHandler_HelloStoresCapabilities(t *testing.T) {
	env := setupControlTest(t)

	// Set up receiver to consume the Welcome response.
	respCh := make(chan []byte, 1)
	env.controlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		respCh <- msg.Data
	})

	sources := []*crosstalkv1.SourceInfo{
		{Name: "mic", Type: "audio"},
		{Name: "camera", Type: "video"},
	}
	sinks := []*crosstalkv1.SinkInfo{
		{Name: "speaker", Type: "audio"},
	}
	codecs := []*crosstalkv1.CodecInfo{
		{Name: "opus/48000/2", MediaType: "audio"},
		{Name: "VP8", MediaType: "video"},
	}

	hello := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_Hello{
			Hello: &crosstalkv1.Hello{
				Sources: sources,
				Sinks:   sinks,
				Codecs:  codecs,
			},
		},
	}
	env.sendProto(t, hello)

	// Wait for the Welcome response to ensure the handler has processed Hello.
	select {
	case <-respCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Welcome response")
	}

	env.server.mu.Lock()
	defer env.server.mu.Unlock()

	require.Len(t, env.server.Sources, 2)
	assert.Equal(t, "mic", env.server.Sources[0].GetName())
	assert.Equal(t, "audio", env.server.Sources[0].GetType())
	assert.Equal(t, "camera", env.server.Sources[1].GetName())
	assert.Equal(t, "video", env.server.Sources[1].GetType())

	require.Len(t, env.server.Sinks, 1)
	assert.Equal(t, "speaker", env.server.Sinks[0].GetName())

	require.Len(t, env.server.Codecs, 2)
	assert.Equal(t, "opus/48000/2", env.server.Codecs[0].GetName())
	assert.Equal(t, "audio", env.server.Codecs[0].GetMediaType())
}

func TestControlHandler_JoinSession_Success(t *testing.T) {
	env := setupControlTest(t)

	// Configure mock to return a valid session.
	env.sessions.FindSessionByIDFn = func(id string) (*crosstalk.Session, error) {
		if id == "session-123" {
			return &crosstalk.Session{
				ID:     "session-123",
				Name:   "test-session",
				Status: crosstalk.SessionWaiting,
			}, nil
		}
		return nil, fmt.Errorf("not found")
	}

	respCh := make(chan []byte, 1)
	env.controlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		respCh <- msg.Data
	})

	join := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_JoinSession{
			JoinSession: &crosstalkv1.JoinSession{
				SessionId: "session-123",
				Role:      "interviewer",
			},
		},
	}
	env.sendProto(t, join)

	select {
	case data := <-respCh:
		var resp crosstalkv1.ControlMessage
		require.NoError(t, proto.Unmarshal(data, &resp))

		ev := resp.GetSessionEvent()
		require.NotNil(t, ev, "expected SessionEvent response")
		assert.Equal(t, crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED, ev.GetType())
		assert.Equal(t, "session-123", ev.GetSessionId())
		assert.Contains(t, ev.GetMessage(), "interviewer")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SessionEvent response")
	}

	// Verify session info stored on peer.
	env.server.mu.Lock()
	defer env.server.mu.Unlock()
	assert.Equal(t, "session-123", env.server.SessionID)
	assert.Equal(t, "interviewer", env.server.Role)
	assert.True(t, env.sessions.FindSessionByIDInvoked)
}

func TestControlHandler_JoinSession_NotFound(t *testing.T) {
	env := setupControlTest(t)

	// Configure mock to return nil (session not found).
	env.sessions.FindSessionByIDFn = func(id string) (*crosstalk.Session, error) {
		return nil, fmt.Errorf("not found")
	}

	respCh := make(chan []byte, 1)
	env.controlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		respCh <- msg.Data
	})

	join := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_JoinSession{
			JoinSession: &crosstalkv1.JoinSession{
				SessionId: "nonexistent-session",
				Role:      "observer",
			},
		},
	}
	env.sendProto(t, join)

	select {
	case data := <-respCh:
		var resp crosstalkv1.ControlMessage
		require.NoError(t, proto.Unmarshal(data, &resp))

		ev := resp.GetSessionEvent()
		require.NotNil(t, ev, "expected SessionEvent response")
		assert.Equal(t, crosstalkv1.SessionEventType_SESSION_ROLE_REJECTED, ev.GetType())
		assert.Equal(t, "nonexistent-session", ev.GetSessionId())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SessionEvent response")
	}

	// Verify session info was NOT stored on peer.
	env.server.mu.Lock()
	defer env.server.mu.Unlock()
	assert.Empty(t, env.server.SessionID)
	assert.Empty(t, env.server.Role)
}

func TestControlHandler_LogEntry(t *testing.T) {
	env := setupControlTest(t)

	// Set up a callback to capture the log entry.
	logCh := make(chan *crosstalkv1.LogEntry, 1)
	env.handler.OnLogEntry = func(peer *PeerConn, entry *crosstalkv1.LogEntry) {
		logCh <- entry
	}

	entry := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_LogEntry{
			LogEntry: &crosstalkv1.LogEntry{
				Timestamp: 1700000000000,
				Severity:  crosstalkv1.LogSeverity_LOG_WARN,
				Source:    "client-abc",
				Message:   "something happened",
			},
		},
	}
	env.sendProto(t, entry)

	select {
	case got := <-logCh:
		assert.Equal(t, int64(1700000000000), got.GetTimestamp())
		assert.Equal(t, crosstalkv1.LogSeverity_LOG_WARN, got.GetSeverity())
		assert.Equal(t, "client-abc", got.GetSource())
		assert.Equal(t, "something happened", got.GetMessage())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for LogEntry callback")
	}
}

func TestControlHandler_ProtobufRoundTrip(t *testing.T) {
	// Test that marshal/unmarshal of ControlMessage preserves all fields for
	// each payload variant.
	tests := []struct {
		name string
		msg  *crosstalkv1.ControlMessage
	}{
		{
			name: "Hello",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_Hello{
					Hello: &crosstalkv1.Hello{
						Sources: []*crosstalkv1.SourceInfo{{Name: "mic", Type: "audio"}},
						Sinks:   []*crosstalkv1.SinkInfo{{Name: "speaker", Type: "audio"}},
						Codecs:  []*crosstalkv1.CodecInfo{{Name: "opus/48000/2", MediaType: "audio"}},
					},
				},
			},
		},
		{
			name: "Welcome",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_Welcome{
					Welcome: &crosstalkv1.Welcome{
						ClientId:      "peer-xyz",
						ServerVersion: "1.2.3",
					},
				},
			},
		},
		{
			name: "JoinSession",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_JoinSession{
					JoinSession: &crosstalkv1.JoinSession{
						SessionId: "sess-001",
						Role:      "interviewer",
					},
				},
			},
		},
		{
			name: "SessionEvent",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_SessionEvent{
					SessionEvent: &crosstalkv1.SessionEvent{
						Type:      crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED,
						Message:   "joined as host",
						SessionId: "sess-002",
					},
				},
			},
		},
		{
			name: "LogEntry",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_LogEntry{
					LogEntry: &crosstalkv1.LogEntry{
						Timestamp: 1700000000000,
						Severity:  crosstalkv1.LogSeverity_LOG_ERROR,
						Source:    "server",
						Message:   "disk full",
					},
				},
			},
		},
		{
			name: "ClientStatus",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_ClientStatus{
					ClientStatus: &crosstalkv1.ClientStatus{
						State:   crosstalkv1.ClientState_CLIENT_BUSY,
						Sources: []*crosstalkv1.SourceInfo{{Name: "cam", Type: "video"}},
					},
				},
			},
		},
		{
			name: "ChannelStatus",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_ChannelStatus{
					ChannelStatus: &crosstalkv1.ChannelStatus{
						ChannelId:        "ch-1",
						State:            crosstalkv1.ChannelState_CHANNEL_ACTIVE,
						BytesTransferred: 42000,
					},
				},
			},
		},
		{
			name: "BindChannel",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_BindChannel{
					BindChannel: &crosstalkv1.BindChannel{
						ChannelId: "ch-1",
						LocalName: "mic",
						Direction: crosstalkv1.Direction_SOURCE,
						TrackId:   "track-abc",
					},
				},
			},
		},
		{
			name: "UnbindChannel",
			msg: &crosstalkv1.ControlMessage{
				Payload: &crosstalkv1.ControlMessage_UnbindChannel{
					UnbindChannel: &crosstalkv1.UnbindChannel{
						ChannelId: "ch-2",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := proto.Marshal(tt.msg)
			require.NoError(t, err)

			var got crosstalkv1.ControlMessage
			require.NoError(t, proto.Unmarshal(data, &got))

			assert.True(t, proto.Equal(tt.msg, &got), "round-trip mismatch for %s", tt.name)
		})
	}
}
