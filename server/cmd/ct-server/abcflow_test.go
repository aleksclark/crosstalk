package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/audiocodec"
	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
)

// TestIntegrationABCAudioFlow proves the full headless-ABC path end to end
// against the real server with real Opus + registered ABC token path.
func TestIntegrationABCAudioFlow(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Booth Session")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")

	// Register an ABC and assign it to the session.
	resp := env.doRequest(t, http.MethodPost, "/api/abcs", adminToken, `{"name":"Booth A"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var abc struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&abc))
	resp.Body.Close()
	require.NotEmpty(t, abc.Token)

	resp = env.doRequest(t, http.MethodPut, "/api/abcs/"+abc.ID, adminToken,
		fmt.Sprintf(`{"name":"Booth A","session_id":%q}`, session.ID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Prefer registered media-ticket path for ABC (auth-expected), fall back
	// is still ABC long-lived token on /ws/signaling.
	ticket := env.issueMediaTicket(t, abc.Token, session.ID, []string{"type:feed"}, nil, "abc")
	_ = ticket // ticket path exercised below for session WS; signaling uses ABC token.

	// ── ABC connects like the KickPi CLI: control DC + published mic track ──
	// Use the registered ABC API token on /ws/signaling (production board path).
	abcTone := 587.0
	abcClient, welcome := newABCClient(t, env, abc.Token)
	require.NotNil(t, welcome, "ABC should receive a Welcome")
	assert.Equal(t, session.ID, welcome.GetAssignedSessionId(),
		"Welcome must carry the ABC's assigned session")

	// Also prove ABC can mint a media ticket (auth lane contract).
	require.NotEmpty(t, ticket)

	// ── A listener subscribes to the feed to hear the ABC ──────────────────
	listener := newTicketMediaClient(t, env, "listener", adminToken, session.ID,
		[]string{}, []string{feedCh.Name}, "admin")

	// ── ABC streams real Opus tone; verify it reaches the feed listener ────
	const minSamples = audiocodec.FrameSize * 10
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go abcClient.streamUntilCancel(streamCtx, abcTone)

	tone, ratio, samples := waitTone(t, listener, toneMenu, minSamples, 20*time.Second)
	require.GreaterOrEqual(t, len(samples), minSamples, "listener received too little audio")
	require.True(t, pcmHasEnergy(samples), "listener received only silence")
	t.Logf("feed listener heard %.0fHz (ratio %.1f)", tone, ratio)
	assert.Equal(t, abcTone, tone, "listener should hear the ABC's tone on the feed")
	assert.Greater(t, ratio, 2.0, "ABC tone should dominate on the feed")
}

// TestIntegrationABCLateAssignment is the regression test for "the K2B board
// doesn't show up as an audio source."
func TestIntegrationABCLateAssignment(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Booth Session")
	createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")

	// Register an ABC but DO NOT assign it to the session yet.
	resp := env.doRequest(t, http.MethodPost, "/api/abcs", adminToken, `{"name":"Late Booth"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var abc struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&abc))
	resp.Body.Close()

	// ── Board connects while unassigned: no session, no source ──────────────
	c1, welcome1 := newABCClient(t, env, abc.Token)
	require.NotNil(t, welcome1)
	assert.Empty(t, welcome1.GetAssignedSessionId(),
		"unassigned board must not be bridged into any session")

	srcs, err := env.sources.List(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, srcs, "an unassigned board must not create a source")

	// ── Assign the board to the session; this must close its live peer so it
	//    reconnects (real boards auto-reconnect). ────────────────────────────
	resp = env.doRequest(t, http.MethodPut, "/api/abcs/"+abc.ID, adminToken,
		fmt.Sprintf(`{"session_id":%q}`, session.ID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// The old peer's signaling websocket should be closed by the server.
	assertABCPeerClosed(t, c1)

	// ── Board reconnects (same token) now that it is assigned ───────────────
	_, welcome2 := newABCClient(t, env, abc.Token)
	require.NotNil(t, welcome2)
	assert.Equal(t, session.ID, welcome2.GetAssignedSessionId(),
		"reconnected board must be bridged into its assigned session")

	// A source now exists for the session, reused by stable identity.
	require.Eventually(t, func() bool {
		s, lerr := env.sources.List(ctx, session.ID)
		return lerr == nil && len(s) == 1
	}, 3*time.Second, 50*time.Millisecond,
		"assigned board must appear as exactly one audio source")

	s, err := env.sources.List(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, s, 1)
	assert.Equal(t, crosstalk.OriginABC, s[0].Origin)
	assert.Equal(t, "Late Booth", s[0].Name, "source uses the ABC's name as label")
}

// assertABCPeerClosed waits until the ABC client's peer connection drops.
func assertABCPeerClosed(t *testing.T, ac *abcClient) {
	t.Helper()
	require.Eventually(t, func() bool {
		switch ac.pc.ConnectionState() {
		case pionwebrtc.PeerConnectionStateDisconnected,
			pionwebrtc.PeerConnectionStateFailed,
			pionwebrtc.PeerConnectionStateClosed:
			return true
		}
		switch ac.pc.ICEConnectionState() {
		case pionwebrtc.ICEConnectionStateDisconnected,
			pionwebrtc.ICEConnectionStateFailed,
			pionwebrtc.ICEConnectionStateClosed:
			return true
		}
		return false
	}, 8*time.Second, 100*time.Millisecond,
		"assignment change did not close the ABC's peer connection")
}

// abcClient is a pion peer that mimics the headless ct-abc CLI: it opens a
// "control" data channel, publishes an Opus mic track, and speaks the v2
// control protocol. Auth uses the registered ABC API token on /ws/signaling.
type abcClient struct {
	pc        *pionwebrtc.PeerConnection
	ws        *websocket.Conn
	dc        *pionwebrtc.DataChannel
	sendTrack *pionwebrtc.TrackLocalStaticRTP
	connected chan struct{}

	mu       sync.Mutex
	commands []*crosstalkv2.AudioControlCommand
	cmdWait  chan *crosstalkv2.AudioControlCommand
}

func newABCClient(t *testing.T, env *testEnv, token string) (*abcClient, *crosstalkv2.Welcome) {
	t.Helper()

	var se pionwebrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := pionwebrtc.NewAPI(pionwebrtc.WithSettingEngine(se))
	pc, err := api.NewPeerConnection(pionwebrtc.Configuration{})
	require.NoError(t, err, "abc: new pc")

	track, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus}, "abc-mic", "abc",
	)
	require.NoError(t, err)
	_, err = pc.AddTransceiverFromTrack(track, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionSendrecv,
	})
	require.NoError(t, err)

	// The ABC (offerer) creates the control channel; the server adopts it.
	// Ordered+reliable (default) matches production ct-abc control channel.
	dc, err := pc.CreateDataChannel("control", nil)
	require.NoError(t, err)

	ac := &abcClient{
		pc:        pc,
		dc:        dc,
		sendTrack: track,
		connected: make(chan struct{}),
		cmdWait:   make(chan *crosstalkv2.AudioControlCommand, 8),
	}
	welcomeCh := make(chan *crosstalkv2.Welcome, 1)

	dc.OnOpen(func() {
		// Send v2 Hello (client_type="abc").
		hello := &crosstalkv2.ControlMessage{Payload: &crosstalkv2.ControlMessage_Hello{
			Hello: &crosstalkv2.Hello{ClientType: "abc", ClientName: "booth-test"},
		}}
		data, _ := proto.Marshal(hello)
		_ = dc.Send(data)
	})
	dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
		var cm crosstalkv2.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err != nil {
			return
		}
		if w := cm.GetWelcome(); w != nil {
			select {
			case welcomeCh <- w:
			default:
			}
		}
		if cmd := cm.GetAudioControlCommand(); cmd != nil {
			// Clone via re-marshal so callers can keep the pointer safely.
			cloned := proto.Clone(cmd).(*crosstalkv2.AudioControlCommand)
			ac.mu.Lock()
			ac.commands = append(ac.commands, cloned)
			ac.mu.Unlock()
			select {
			case ac.cmdWait <- cloned:
			default:
			}
		}
	})

	var once sync.Once
	markConnected := func() {
		once.Do(func() { close(ac.connected) })
	}
	pc.OnICEConnectionStateChange(func(s pionwebrtc.ICEConnectionState) {
		if s == pionwebrtc.ICEConnectionStateConnected ||
			s == pionwebrtc.ICEConnectionStateCompleted {
			markConnected()
		}
	})
	pc.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		if s == pionwebrtc.PeerConnectionStateConnected {
			markConnected()
		}
	})

	ctx := context.Background()
	wsURL := strings.Replace(env.server.URL, "http://", "ws://", 1) + "/ws/signaling?token=" + url.QueryEscape(token)
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "abc: ws dial")
	ac.ws = ws

	pc.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		m, _ := json.Marshal(map[string]any{"type": "candidate", "candidate": init})
		_ = ws.Write(ctx, websocket.MessageText, m)
	})

	// Start the signaling read loop BEFORE sending the offer so a fast
	// server answer / trickle candidates cannot race past an unstarted reader
	// (that race presents as ICE connect timeout under load).
	go func() {
		for {
			_, data, rerr := ws.Read(ctx)
			if rerr != nil {
				return
			}
			var msg struct {
				Type      string                       `json:"type"`
				SDP       string                       `json:"sdp"`
				Candidate *pionwebrtc.ICECandidateInit `json:"candidate"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "answer":
				_ = pc.SetRemoteDescription(pionwebrtc.SessionDescription{
					Type: pionwebrtc.SDPTypeAnswer, SDP: msg.SDP,
				})
			case "offer":
				// Server-initiated renegotiation (e.g. late subscribe track).
				if err := pc.SetRemoteDescription(pionwebrtc.SessionDescription{
					Type: pionwebrtc.SDPTypeOffer, SDP: msg.SDP,
				}); err != nil {
					continue
				}
				ans, err := pc.CreateAnswer(nil)
				if err != nil {
					continue
				}
				_ = pc.SetLocalDescription(ans)
				ansMsg, _ := json.Marshal(map[string]string{"type": "answer", "sdp": ans.SDP})
				_ = ws.Write(ctx, websocket.MessageText, ansMsg)
			case "ice", "candidate":
				if msg.Candidate != nil {
					_ = pc.AddICECandidate(*msg.Candidate)
				}
			}
		}
	}()

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, pc.SetLocalDescription(offer))
	gatherComplete := pionwebrtc.GatheringCompletePromise(pc)
	offerMsg, _ := json.Marshal(map[string]string{"type": "offer", "sdp": offer.SDP})
	require.NoError(t, ws.Write(ctx, websocket.MessageText, offerMsg))
	select {
	case <-gatherComplete:
	case <-time.After(5 * time.Second):
	}

	select {
	case <-ac.connected:
	case <-time.After(60 * time.Second):
		t.Fatalf("abc: ICE connect timeout (pc=%s ice=%s)",
			pc.ConnectionState().String(), pc.ICEConnectionState().String())
	}

	var welcome *crosstalkv2.Welcome
	select {
	case welcome = <-welcomeCh:
	case <-time.After(20 * time.Second):
		t.Fatal("abc: no Welcome received (control channel likely aborted)")
	}

	t.Cleanup(func() {
		_ = ws.Close(websocket.StatusNormalClosure, "")
		_ = pc.Close()
	})
	return ac, welcome
}

// waitAudioCommand waits for the next AudioControlCommand from the server.
func (ac *abcClient) waitAudioCommand(t *testing.T, timeout time.Duration) *crosstalkv2.AudioControlCommand {
	t.Helper()
	select {
	case cmd := <-ac.cmdWait:
		return cmd
	case <-time.After(timeout):
		t.Fatalf("abc: timed out waiting for AudioControlCommand")
		return nil
	}
}

// sendControl sends a raw control protobuf message on the reliable DC.
func (ac *abcClient) sendControl(t *testing.T, msg *crosstalkv2.ControlMessage) {
	t.Helper()
	require.NotNil(t, ac.dc)
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	// Data channel may still be opening under load; retry briefly.
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if ac.dc.ReadyState() == pionwebrtc.DataChannelStateOpen {
			last = ac.dc.Send(data)
			if last == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(t, last, "send control message")
}

// sendAudioReport sends an AudioControlReport on the control channel.
func (ac *abcClient) sendAudioReport(t *testing.T, report *crosstalkv2.AudioControlReport) {
	t.Helper()
	ac.sendControl(t, &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_AudioControlReport{
			AudioControlReport: report,
		},
	})
}

// stream emits a continuous sine tone as real Opus RTP packets at 20ms cadence.
// frames < 0 streams until ctx cancel.
func (ac *abcClient) stream(ctx context.Context, freq float64, frames int) {
	enc, err := audiocodec.NewOpusEncoder()
	if err != nil {
		return
	}
	defer enc.Close()
	buf := make([]byte, 4000)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var seq uint16
	var ts uint32
	start := 0
	for i := 0; frames < 0 || i < frames; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		frame := sineFrame(freq, start)
		n, err := enc.Encode(frame, buf)
		if err != nil || n == 0 {
			continue
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		seq++
		ts += uint32(audiocodec.FrameSize)
		_ = ac.sendTrack.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: seq, Timestamp: ts, SSRC: 0xABC0},
			Payload: payload,
		})
		start += len(frame)
	}
}

// streamUntilCancel streams until ctx is cancelled.
func (ac *abcClient) streamUntilCancel(ctx context.Context, freq float64) {
	ac.stream(ctx, freq, -1)
}
