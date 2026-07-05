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

	"github.com/aleksclark/crosstalk/server/mixer"
	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
)

// TestIntegrationABCAudioFlow proves the full headless-ABC path end to end
// against the real server, exactly as the KickPi board uses it:
//
//   - The ABC connects to the generic /ws/signaling endpoint with its API
//     token (not a JWT), opens a "control" data channel, and publishes a mic
//     track — all in its initial offer.
//   - The server adopts the ABC's control channel (rather than creating its
//     own, which previously collided on SCTP and aborted the association),
//     replies to Hello with a Welcome carrying the assigned session, and
//     bridges the ABC as an "abc" producer into that session's feed channel.
//   - A listener subscribed to the feed hears the ABC's tone.
//
// This is the regression test for the control-channel abort and the ABC
// session wiring.
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

	// ── ABC connects like the KickPi CLI: control DC + published mic track ──
	abcTone := 587.0
	abcClient, welcome := newABCClient(t, env, abc.Token)
	require.NotNil(t, welcome, "ABC should receive a Welcome")
	assert.Equal(t, session.ID, welcome.GetAssignedSessionId(),
		"Welcome must carry the ABC's assigned session")

	// ── A listener subscribes to the feed to hear the ABC ──────────────────
	wsBase := strings.Replace(env.server.URL, "http://", "ws://", 1) + "/api/sessions/" + session.ID + "/ws"
	listener := newMediaClient(t, "listener", fmt.Sprintf("%s?listen=%s", wsBase, url.QueryEscape(feedCh.Name)))

	// ── ABC streams its tone; verify it reaches the feed listener ──────────
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go abcClient.stream(streamCtx, abcTone, 100)
	time.Sleep(2500 * time.Millisecond)

	samples := listener.captured()
	require.Greater(t, len(samples), mixer.FrameSize*10, "listener received too little audio")
	tone, ratio := detectTone(samples, toneMenu)
	t.Logf("feed listener heard %.0fHz (ratio %.1f)", tone, ratio)
	assert.Equal(t, abcTone, tone, "listener should hear the ABC's tone on the feed")
	assert.Greater(t, ratio, 3.0, "ABC tone should dominate on the feed")
}

// abcClient is a pion peer that mimics the headless ct-abc CLI: it opens a
// "control" data channel, publishes an Opus mic track, and speaks the v2
// control protocol.
type abcClient struct {
	pc        *pionwebrtc.PeerConnection
	ws        *websocket.Conn
	sendTrack *pionwebrtc.TrackLocalStaticRTP
	connected chan struct{}
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
	dc, err := pc.CreateDataChannel("control", nil)
	require.NoError(t, err)

	ac := &abcClient{pc: pc, sendTrack: track, connected: make(chan struct{})}
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
	})

	var once sync.Once
	pc.OnICEConnectionStateChange(func(s pionwebrtc.ICEConnectionState) {
		if s == pionwebrtc.ICEConnectionStateConnected {
			once.Do(func() { close(ac.connected) })
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

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, pc.SetLocalDescription(offer))
	offerMsg, _ := json.Marshal(map[string]string{"type": "offer", "sdp": offer.SDP})
	require.NoError(t, ws.Write(ctx, websocket.MessageText, offerMsg))

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
			case "ice", "candidate":
				if msg.Candidate != nil {
					_ = pc.AddICECandidate(*msg.Candidate)
				}
			}
		}
	}()

	select {
	case <-ac.connected:
	case <-time.After(20 * time.Second):
		t.Fatal("abc: ICE connect timeout")
	}

	var welcome *crosstalkv2.Welcome
	select {
	case welcome = <-welcomeCh:
	case <-time.After(5 * time.Second):
		t.Fatal("abc: no Welcome received (control channel likely aborted)")
	}

	t.Cleanup(func() {
		_ = ws.Close(websocket.StatusNormalClosure, "")
		_ = pc.Close()
	})
	return ac, welcome
}

// stream emits a continuous sine tone as MTU-sized RTP packets at 20ms cadence.
func (ac *abcClient) stream(ctx context.Context, freq float64, frames int) {
	const samplesPerPacket = 480
	enc := mixer.NullEncoder{}
	buf := make([]byte, samplesPerPacket*2)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var seq uint16
	var ts uint32
	start := 0
	for i := 0; i < frames; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		frame := sineFrame(freq, start)
		for off := 0; off < len(frame); off += samplesPerPacket {
			end := off + samplesPerPacket
			if end > len(frame) {
				end = len(frame)
			}
			chunk := frame[off:end]
			n, _ := enc.Encode(chunk, buf)
			payload := make([]byte, n)
			copy(payload, buf[:n])
			seq++
			ts += uint32(len(chunk))
			_ = ac.sendTrack.WriteRTP(&rtp.Packet{
				Header:  rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: seq, Timestamp: ts, SSRC: 0xABC0},
				Payload: payload,
			})
		}
		start += len(frame)
	}
}
