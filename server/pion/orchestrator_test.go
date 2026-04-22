package pion

import (
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

// orchestratorTestEnv bundles mock services and a connected client+server pair
// for testing the Orchestrator.
type orchestratorTestEnv struct {
	orchestrator *Orchestrator
	sessions     *mock.SessionService
	templates    *mock.SessionTemplateService
}

// newOrchestratorTestEnv creates an Orchestrator wired to mock services with a
// pre-configured session and template.
func newOrchestratorTestEnv(t *testing.T) *orchestratorTestEnv {
	t.Helper()

	tmpl := &crosstalk.SessionTemplate{
		ID:   "tmpl-1",
		Name: "interview",
		Roles: []crosstalk.Role{
			{Name: "interviewer", MultiClient: false},
			{Name: "candidate", MultiClient: false},
			{Name: "observer", MultiClient: true},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "interviewer:mic", Sink: "candidate:speaker"},
			{Source: "candidate:mic", Sink: "interviewer:speaker"},
			{Source: "interviewer:mic", Sink: "record"},
		},
	}

	session := &crosstalk.Session{
		ID:         "sess-1",
		TemplateID: "tmpl-1",
		Name:       "Test Session",
		Status:     crosstalk.SessionWaiting,
	}

	sessions := &mock.SessionService{
		FindSessionByIDFn: func(id string) (*crosstalk.Session, error) {
			if id == session.ID {
				return session, nil
			}
			return nil, nil
		},
	}

	templates := &mock.SessionTemplateService{
		FindTemplateByIDFn: func(id string) (*crosstalk.SessionTemplate, error) {
			if id == tmpl.ID {
				return tmpl, nil
			}
			return nil, nil
		},
	}

	orch := NewOrchestrator(sessions, templates)

	return &orchestratorTestEnv{
		orchestrator: orch,
		sessions:     sessions,
		templates:    templates,
	}
}

// createConnectedServerPeer creates a server PeerConn with a connected client
// PeerConnection, waits for ICE connection and the control data channel to be
// ready, and returns both plus the client-side control DC.
func createConnectedServerPeer(t *testing.T) (*PeerConn, *webrtc.PeerConnection, *webrtc.DataChannel) {
	t.Helper()

	pm := testPeerManager(t)
	server, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { server.Close() }) //nolint:errcheck

	client := createClientPC(t)

	controlReady := make(chan *webrtc.DataChannel, 1)
	client.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "control" {
			return
		}
		dc.OnOpen(func() {
			controlReady <- dc
		})
	})

	_, err = client.CreateDataChannel("init", nil)
	require.NoError(t, err)

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

	return server, client, dc
}

// recvControlMessages collects up to n control messages from the client-side
// data channel within the given timeout.
func recvControlMessages(t *testing.T, dc *webrtc.DataChannel, n int, timeout time.Duration) []*crosstalkv1.ControlMessage {
	t.Helper()

	ch := make(chan []byte, n*2)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		ch <- msg.Data
	})

	var msgs []*crosstalkv1.ControlMessage
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for len(msgs) < n {
		select {
		case data := <-ch:
			var cm crosstalkv1.ControlMessage
			require.NoError(t, proto.Unmarshal(data, &cm))
			msgs = append(msgs, &cm)
		case <-timer.C:
			return msgs
		}
	}
	return msgs
}

func TestOrchestrator_JoinSession_Success(t *testing.T) {
	env := newOrchestratorTestEnv(t)
	server, _, dc := createConnectedServerPeer(t)

	// Collect messages: expect SessionEvent(JOINED) + BindChannel(SOURCE for record)
	msgs := make(chan *crosstalkv1.ControlMessage, 10)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			msgs <- &cm
		}
	})

	err := env.orchestrator.JoinSession(server, "sess-1", "interviewer")
	require.NoError(t, err)

	// Verify the peer is registered in the live session.
	ls := env.orchestrator.GetLiveSession("sess-1")
	require.NotNil(t, ls)
	assert.Len(t, ls.Clients, 1)
	assert.Equal(t, "interviewer", ls.Clients[server.ID].Role)

	// Verify session/role stored on peer.
	server.mu.Lock()
	assert.Equal(t, "sess-1", server.SessionID)
	assert.Equal(t, "interviewer", server.Role)
	server.mu.Unlock()

	// Expect at least 2 messages: SessionEvent(JOINED) + BindChannel(SOURCE for record).
	var received []*crosstalkv1.ControlMessage
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for len(received) < 2 {
		select {
		case m := <-msgs:
			received = append(received, m)
		case <-timer.C:
			t.Fatalf("expected at least 2 messages, got %d", len(received))
		}
	}

	// Find the SessionEvent.
	var joinedEvent *crosstalkv1.SessionEvent
	var bindCmd *crosstalkv1.BindChannel
	for _, m := range received {
		if ev := m.GetSessionEvent(); ev != nil {
			joinedEvent = ev
		}
		if bc := m.GetBindChannel(); bc != nil {
			bindCmd = bc
		}
	}

	require.NotNil(t, joinedEvent, "expected SESSION_CLIENT_JOINED event")
	assert.Equal(t, crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED, joinedEvent.GetType())
	assert.Equal(t, "sess-1", joinedEvent.GetSessionId())

	// The record binding should activate (only source side needed).
	require.NotNil(t, bindCmd, "expected BindChannel command for record binding")
	assert.Equal(t, crosstalkv1.Direction_SOURCE, bindCmd.GetDirection())
	assert.Equal(t, "mic", bindCmd.GetLocalName())
}

func TestOrchestrator_JoinSession_RoleRejected_SingleClient(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	// First client joins as interviewer.
	server1, _, _ := createConnectedServerPeer(t)
	err := env.orchestrator.JoinSession(server1, "sess-1", "interviewer")
	require.NoError(t, err)

	// Second client tries same single-client role.
	server2, _, _ := createConnectedServerPeer(t)
	err = env.orchestrator.JoinSession(server2, "sess-1", "interviewer")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already occupied")

	// Verify only one client registered.
	ls := env.orchestrator.GetLiveSession("sess-1")
	require.NotNil(t, ls)
	assert.Len(t, ls.Clients, 1)
}

func TestOrchestrator_JoinSession_MultiClient(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	// Two clients join as observer (multi-client role).
	server1, _, _ := createConnectedServerPeer(t)
	err := env.orchestrator.JoinSession(server1, "sess-1", "observer")
	require.NoError(t, err)

	server2, _, _ := createConnectedServerPeer(t)
	err = env.orchestrator.JoinSession(server2, "sess-1", "observer")
	require.NoError(t, err)

	// Both should be registered.
	ls := env.orchestrator.GetLiveSession("sess-1")
	require.NotNil(t, ls)
	assert.Len(t, ls.Clients, 2)
}

func TestOrchestrator_EndSession(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	// Two clients join different roles.
	server1, _, dc1 := createConnectedServerPeer(t)
	err := env.orchestrator.JoinSession(server1, "sess-1", "interviewer")
	require.NoError(t, err)

	server2, _, dc2 := createConnectedServerPeer(t)
	err = env.orchestrator.JoinSession(server2, "sess-1", "candidate")
	require.NoError(t, err)

	// Set up receivers for both clients to capture SessionEnded.
	endedCh1 := make(chan *crosstalkv1.SessionEvent, 5)
	dc1.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			if ev := cm.GetSessionEvent(); ev != nil && ev.GetType() == crosstalkv1.SessionEventType_SESSION_ENDED {
				endedCh1 <- ev
			}
		}
	})
	endedCh2 := make(chan *crosstalkv1.SessionEvent, 5)
	dc2.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			if ev := cm.GetSessionEvent(); ev != nil && ev.GetType() == crosstalkv1.SessionEventType_SESSION_ENDED {
				endedCh2 <- ev
			}
		}
	})

	// End the session.
	env.orchestrator.EndSession("sess-1")

	// Verify both clients received SESSION_ENDED.
	timeout := time.After(3 * time.Second)

	select {
	case ev := <-endedCh1:
		assert.Equal(t, crosstalkv1.SessionEventType_SESSION_ENDED, ev.GetType())
	case <-timeout:
		t.Fatal("client 1: timed out waiting for SESSION_ENDED")
	}

	select {
	case ev := <-endedCh2:
		assert.Equal(t, crosstalkv1.SessionEventType_SESSION_ENDED, ev.GetType())
	case <-timeout:
		t.Fatal("client 2: timed out waiting for SESSION_ENDED")
	}

	// Live session should be cleaned up.
	ls := env.orchestrator.GetLiveSession("sess-1")
	assert.Nil(t, ls)
}

func TestOrchestrator_PartialBindings(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	// Only interviewer joins — no candidate. Only the "record" binding should
	// activate (it needs only the source). The role→role bindings
	// (interviewer:mic → candidate:speaker and candidate:mic → interviewer:speaker)
	// should NOT activate because candidate is absent.
	server, _, dc := createConnectedServerPeer(t)

	msgsCh := make(chan *crosstalkv1.ControlMessage, 10)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			msgsCh <- &cm
		}
	})

	err := env.orchestrator.JoinSession(server, "sess-1", "interviewer")
	require.NoError(t, err)

	// Collect messages.
	var received []*crosstalkv1.ControlMessage
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
collect:
	for {
		select {
		case m := <-msgsCh:
			received = append(received, m)
		case <-timer.C:
			break collect
		}
	}

	// Count BindChannel commands.
	var binds []*crosstalkv1.BindChannel
	for _, m := range received {
		if bc := m.GetBindChannel(); bc != nil {
			binds = append(binds, bc)
		}
	}

	// Should have exactly 1 BindChannel (for the record mapping).
	require.Len(t, binds, 1, "expected exactly 1 BindChannel for record mapping")
	assert.Equal(t, "mic", binds[0].GetLocalName())
	assert.Equal(t, crosstalkv1.Direction_SOURCE, binds[0].GetDirection())

	// Verify only 1 active binding in the live session.
	ls := env.orchestrator.GetLiveSession("sess-1")
	require.NotNil(t, ls)
	assert.Len(t, ls.Bindings, 1)
}

func TestOrchestrator_LeaveSession_DeactivatesBindings(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	// Both roles join.
	server1, _, _ := createConnectedServerPeer(t)
	err := env.orchestrator.JoinSession(server1, "sess-1", "interviewer")
	require.NoError(t, err)

	server2, _, dc2 := createConnectedServerPeer(t)
	err = env.orchestrator.JoinSession(server2, "sess-1", "candidate")
	require.NoError(t, err)

	// Verify bindings are active (3 total: 2 role→role + 1 record).
	ls := env.orchestrator.GetLiveSession("sess-1")
	require.NotNil(t, ls)
	assert.Len(t, ls.Bindings, 3)

	// Set up receiver for candidate to capture UnbindChannel messages.
	unbindCh := make(chan *crosstalkv1.UnbindChannel, 10)
	dc2.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			if ub := cm.GetUnbindChannel(); ub != nil {
				unbindCh <- ub
			}
		}
	})

	// Interviewer leaves — the 2 role→role bindings should deactivate,
	// leaving only the record binding... but actually, since interviewer left,
	// the record binding (interviewer:mic → record) should also deactivate.
	env.orchestrator.LeaveSession(server1)

	ls = env.orchestrator.GetLiveSession("sess-1")
	if ls != nil {
		// Only candidate is left; no bindings should be active since candidate
		// has no source mapping without interviewer.
		assert.Len(t, ls.Bindings, 0, "expected 0 bindings after interviewer left")
	}

	// Candidate should have received at least 1 UnbindChannel (for the
	// candidate:mic → interviewer:speaker binding that involved candidate as source).
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	var unbinds []*crosstalkv1.UnbindChannel
unbindCollect:
	for {
		select {
		case ub := <-unbindCh:
			unbinds = append(unbinds, ub)
		case <-timer.C:
			break unbindCollect
		}
	}

	// At least one UnbindChannel should be sent to candidate.
	assert.NotEmpty(t, unbinds, "candidate should receive UnbindChannel")
}

func TestOrchestrator_JoinSession_TransitionsToActive(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	var updatedStatus crosstalk.SessionStatus
	var updatedID string
	env.sessions.UpdateSessionStatusFn = func(id string, status crosstalk.SessionStatus) error {
		updatedID = id
		updatedStatus = status
		return nil
	}

	server, _, _ := createConnectedServerPeer(t)

	err := env.orchestrator.JoinSession(server, "sess-1", "interviewer")
	require.NoError(t, err)

	// The record binding (interviewer:mic → record) activates immediately,
	// which should transition the session to "active".
	assert.Equal(t, "sess-1", updatedID)
	assert.Equal(t, crosstalk.SessionActive, updatedStatus)
	assert.True(t, env.sessions.UpdateSessionStatusInvoked)

	ls := env.orchestrator.GetLiveSession("sess-1")
	require.NotNil(t, ls)
	assert.Equal(t, crosstalk.SessionActive, ls.Session.Status)
}

func TestOrchestrator_JoinSession_SinkReceivesBindChannel(t *testing.T) {
	env := newOrchestratorTestEnv(t)

	// Join interviewer first (produces record binding only).
	server1, _, _ := createConnectedServerPeer(t)
	err := env.orchestrator.JoinSession(server1, "sess-1", "interviewer")
	require.NoError(t, err)

	// Join candidate — this should activate role→role bindings.
	server2, _, dc2 := createConnectedServerPeer(t)

	msgsCh := make(chan *crosstalkv1.ControlMessage, 10)
	dc2.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			msgsCh <- &cm
		}
	})

	err = env.orchestrator.JoinSession(server2, "sess-1", "candidate")
	require.NoError(t, err)

	// Collect messages from candidate's data channel.
	var received []*crosstalkv1.ControlMessage
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
collect:
	for {
		select {
		case m := <-msgsCh:
			received = append(received, m)
		case <-timer.C:
			break collect
		}
	}

	// Find BindChannel with SINK direction.
	var sinkBinds []*crosstalkv1.BindChannel
	for _, m := range received {
		if bc := m.GetBindChannel(); bc != nil && bc.GetDirection() == crosstalkv1.Direction_SINK {
			sinkBinds = append(sinkBinds, bc)
		}
	}

	require.NotEmpty(t, sinkBinds, "candidate should receive BindChannel{SINK}")
	assert.Equal(t, "speaker", sinkBinds[0].GetLocalName())
}
