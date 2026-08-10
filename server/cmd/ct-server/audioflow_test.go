package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
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
	"nhooyr.io/websocket"

	"github.com/aleksclark/crosstalk/server/audiocodec"
)

// ── Audio helpers ─────────────────────────────────────────────────────────

const sampleRate = 48000

// toneMenu is the set of well-separated frequencies (Hz) a source may emit.
// A source picks one at random for each test run.
var toneMenu = []float64{440, 587, 659, 880, 1046}

// sineFrame fills a FrameSize frame of a sine wave at freq, continuous across
// frames via startSample. Amplitude is ~0.5 of full scale by default.
func sineFrame(freq float64, startSample int) []int16 {
	return sineFrameAmp(freq, startSample, 0.5)
}

func sineFrameAmp(freq float64, startSample int, amp float64) []int16 {
	frame := make([]int16, audiocodec.FrameSize)
	for i := range frame {
		n := float64(startSample + i)
		frame[i] = int16(amp * math.MaxInt16 * math.Sin(2*math.Pi*freq*n/float64(sampleRate)))
	}
	return frame
}

// goertzel returns the relative magnitude of freq in samples.
func goertzel(samples []int16, freq float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	k := 2 * math.Cos(2*math.Pi*freq/float64(sampleRate))
	var s1, s2 float64
	for _, x := range samples {
		s0 := float64(x) + k*s1 - s2
		s2 = s1
		s1 = s0
	}
	power := s1*s1 + s2*s2 - k*s1*s2
	return math.Sqrt(math.Abs(power)) / float64(len(samples))
}

// detectTone returns the candidate frequency with the strongest Goertzel
// response, along with the ratio of the winner to the runner-up (confidence).
func detectTone(samples []int16, candidates []float64) (best float64, ratio float64) {
	type score struct {
		freq float64
		mag  float64
	}
	var top, second score
	for _, f := range candidates {
		s := score{f, goertzel(samples, f)}
		if s.mag > top.mag {
			second = top
			top = s
		} else if s.mag > second.mag {
			second = s
		}
	}
	if second.mag == 0 {
		return top.freq, math.Inf(1)
	}
	return top.freq, top.mag / second.mag
}

// ── WebRTC test client ──────────────────────────────────────────────────────

// mediaClient is a real pion WebRTC peer that connects to the server's
// session signaling endpoint, sends Opus tones, and captures received audio
// via a real Opus decoder.
type mediaClient struct {
	name      string
	pc        *pionwebrtc.PeerConnection
	ws        *websocket.Conn
	sendTrack *pionwebrtc.TrackLocalStaticRTP
	connected chan struct{}

	mu       sync.Mutex
	received []int16
}

// newMediaClient dials the given ws URL (must already include a fresh media
// ticket or broadcast token), negotiates an audio session with the server, and
// returns once ICE is connected. When serverOffer is true the client waits for
// the server's offer (public broadcast path) and does not publish a track.
func newMediaClient(t *testing.T, name, wsURL string) *mediaClient {
	t.Helper()
	return newMediaClientOpts(t, name, wsURL, false)
}

func newMediaClientOpts(t *testing.T, name, wsURL string, serverOffer bool) *mediaClient {
	t.Helper()

	var se pionwebrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := pionwebrtc.NewAPI(pionwebrtc.WithSettingEngine(se))

	pc, err := api.NewPeerConnection(pionwebrtc.Configuration{})
	require.NoError(t, err, "%s: new pc", name)

	mc := &mediaClient{
		name:      name,
		pc:        pc,
		connected: make(chan struct{}),
	}

	if !serverOffer {
		track, err := pionwebrtc.NewTrackLocalStaticRTP(
			pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus},
			name+"-mic", name,
		)
		require.NoError(t, err, "%s: new track", name)
		mc.sendTrack = track
		// Bidirectional audio transceiver: send tone + receive mix.
		_, err = pc.AddTransceiverFromTrack(track, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionSendrecv,
		})
		require.NoError(t, err, "%s: add transceiver", name)
	} else {
		// Receive-only transceiver for server-offer broadcast listeners.
		_, err = pc.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeAudio, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
		})
		require.NoError(t, err, "%s: add recvonly transceiver", name)
	}

	pc.OnTrack(func(remote *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		dec, derr := audiocodec.NewOpusDecoder(1)
		if derr != nil {
			return
		}
		defer dec.Close()
		pcm := make([]int16, audiocodec.FrameSize*2)
		for {
			pkt, _, rerr := remote.ReadRTP()
			if rerr != nil {
				return
			}
			if len(pkt.Payload) == 0 {
				continue
			}
			n, derr := dec.Decode(pkt.Payload, pcm)
			if derr != nil || n == 0 {
				continue
			}
			mc.mu.Lock()
			mc.received = append(mc.received, pcm[:n]...)
			mc.mu.Unlock()
		}
	})

	var once sync.Once
	markConnected := func() {
		once.Do(func() { close(mc.connected) })
	}
	pc.OnICEConnectionStateChange(func(state pionwebrtc.ICEConnectionState) {
		if state == pionwebrtc.ICEConnectionStateConnected ||
			state == pionwebrtc.ICEConnectionStateCompleted {
			markConnected()
		}
	})
	// PeerConnectionState is more reliable than ICE alone under heavy
	// localhost load (ICE can briefly stick in "checking" while DTLS is up).
	pc.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		if state == pionwebrtc.PeerConnectionStateConnected {
			markConnected()
		}
	})

	ctx := context.Background()
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "%s: ws dial", name)
	mc.ws = ws

	pc.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		msg, _ := json.Marshal(map[string]any{"type": "candidate", "candidate": init})
		_ = ws.Write(ctx, websocket.MessageText, msg)
	})

	// Reader must be running before any offer/answer exchange.
	go mc.readSignaling(ctx)

	if !serverOffer {
		offer, err := pc.CreateOffer(nil)
		require.NoError(t, err, "%s: create offer", name)
		require.NoError(t, pc.SetLocalDescription(offer), "%s: set local", name)
		// Wait for local ICE gathering so the offer/candidates are complete
		// before we block on the remote side (reduces host-candidate races).
		gatherComplete := pionwebrtc.GatheringCompletePromise(pc)
		offerMsg, _ := json.Marshal(map[string]string{"type": "offer", "sdp": offer.SDP})
		require.NoError(t, ws.Write(ctx, websocket.MessageText, offerMsg), "%s: send offer", name)
		select {
		case <-gatherComplete:
		case <-time.After(5 * time.Second):
		}
	}

	select {
	case <-mc.connected:
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: ICE connect timeout (pc=%s ice=%s)", name,
			pc.ConnectionState().String(), pc.ICEConnectionState().String())
	}

	t.Cleanup(func() {
		_ = ws.Close(websocket.StatusNormalClosure, "")
		_ = pc.Close()
	})
	return mc
}

// newBroadcastListener connects as a public broadcast listener using the
// durable broadcast token contract (/ws/broadcast/{id}?token=...).
func newBroadcastListener(t *testing.T, env *testEnv, sessionID, broadcastToken, name string) *mediaClient {
	t.Helper()
	wsURL := strings.Replace(env.server.URL, "http://", "ws://", 1) +
		"/ws/broadcast/" + sessionID + "?token=" + url.QueryEscape(broadcastToken)
	return newMediaClientOpts(t, name, wsURL, true)
}

// newTicketMediaClient issues a fresh one-time media ticket then connects.
func newTicketMediaClient(t *testing.T, env *testEnv, name, bearer, sessionID string, produce, listen []string, role string) *mediaClient {
	t.Helper()
	ticket := env.issueMediaTicket(t, bearer, sessionID, produce, listen, role)
	wsURL := strings.Replace(env.server.URL, "http://", "ws://", 1) +
		"/api/sessions/" + sessionID + "/ws?token=" + url.QueryEscape(ticket)
	// Optional query narrowing is already baked into the ticket; still pass
	// names only when useful for intercept-style tests.
	return newMediaClient(t, name, wsURL)
}

func (mc *mediaClient) readSignaling(ctx context.Context) {
	for {
		_, data, err := mc.ws.Read(ctx)
		if err != nil {
			return
		}
		var msg struct {
			Type      string                       `json:"type"`
			SDP       string                       `json:"sdp"`
			Candidate *pionwebrtc.ICECandidateInit `json:"candidate"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "answer":
			_ = mc.pc.SetRemoteDescription(pionwebrtc.SessionDescription{
				Type: pionwebrtc.SDPTypeAnswer, SDP: msg.SDP,
			})
		case "offer":
			// Server-initiated renegotiation.
			if err := mc.pc.SetRemoteDescription(pionwebrtc.SessionDescription{
				Type: pionwebrtc.SDPTypeOffer, SDP: msg.SDP,
			}); err != nil {
				continue
			}
			ans, err := mc.pc.CreateAnswer(nil)
			if err != nil {
				continue
			}
			_ = mc.pc.SetLocalDescription(ans)
			ansMsg, _ := json.Marshal(map[string]string{"type": "answer", "sdp": ans.SDP})
			_ = mc.ws.Write(ctx, websocket.MessageText, ansMsg)
		case "ice", "candidate":
			if msg.Candidate != nil {
				_ = mc.pc.AddICECandidate(*msg.Candidate)
			}
		}
	}
}

// stream sends `frames` 20ms sine-tone frames as real Opus RTP packets at the
// mixer's real-time cadence.
func (mc *mediaClient) stream(ctx context.Context, freq float64, frames int) {
	mc.streamAmp(ctx, freq, frames, 0.5)
}

func (mc *mediaClient) streamAmp(ctx context.Context, freq float64, frames int, amp float64) {
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
	// frames < 0 means stream until ctx cancel.
	for i := 0; frames < 0 || i < frames; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if mc.sendTrack == nil {
			return
		}
		frame := sineFrameAmp(freq, start, amp)
		n, err := enc.Encode(frame, buf)
		if err != nil || n == 0 {
			continue
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		seq++
		ts += uint32(audiocodec.FrameSize)
		_ = mc.sendTrack.WriteRTP(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    111,
				SequenceNumber: seq,
				Timestamp:      ts,
				SSRC:           uint32(0xA000 + len(mc.name)),
			},
			Payload: payload,
		})
		start += len(frame)
	}
}

// streamUntilCancel streams a continuous tone until ctx is cancelled.
func (mc *mediaClient) streamUntilCancel(ctx context.Context, freq float64) {
	mc.streamAmp(ctx, freq, -1, 0.5)
}

// captured returns a copy of received samples, skipping warm-up frames.
func (mc *mediaClient) captured() []int16 {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	skip := audiocodec.FrameSize * 8
	if len(mc.received) <= skip {
		return append([]int16(nil), mc.received...)
	}
	return append([]int16(nil), mc.received[skip:]...)
}

// ── Tests ──────────────────────────────────────────────────────────────────

// TestIntegrationAudioFlowABCToTranslatorToBroadcast is the full v3 golden
// audio-flow test over REAL WebRTC transport with real Opus + media tickets.
func TestIntegrationAudioFlowABCToTranslatorToBroadcast(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Sunday Service")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	broadcastCh := createChannel(t, env, adminToken, session.ID, "English Broadcast", "broadcast")

	resp := env.doRequest(t, http.MethodPost, "/api/translators", adminToken,
		`{"username":"maria","password":"translate-pass-123"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var translator struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&translator))
	resp.Body.Close()

	resp = env.doRequest(t, http.MethodPut, "/api/translators/"+translator.ID+"/sessions", adminToken,
		fmt.Sprintf(`{"session_ids":["%s"]}`, session.ID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	translatorToken := env.login(t, "maria", "translate-pass-123")
	resp = env.doRequest(t, http.MethodGet, "/api/sessions", translatorToken, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var sessionList struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sessionList))
	resp.Body.Close()
	sawSession := false
	for _, s := range sessionList.Data {
		if s.ID == session.ID {
			sawSession = true
		}
	}
	assert.True(t, sawSession, "translator should see the assigned session")

	// ABC produces into feed via registered board token path (/ws/signaling).
	_, abcTok := registerABC(t, env, adminToken, "Booth ABC", session.ID)
	abcClient, welcome := newABCClient(t, env, abcTok)
	require.NotNil(t, welcome)
	assert.Equal(t, session.ID, welcome.GetAssignedSessionId())
	// Translator: produce broadcast, listen feed via media ticket.
	translatorClient := newTicketMediaClient(t, env, "translator", translatorToken, session.ID,
		[]string{broadcastCh.Name}, []string{feedCh.Name}, "translator")
	// Public listener uses durable broadcast token contract (server-offer).
	broadcast := newBroadcastListener(t, env, session.ID, session.BroadcastToken, "broadcast")
	_ = feedCh // used via ticket selectors

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	abcTone := toneMenu[rng.Intn(len(toneMenu))]
	translatorTone := abcTone
	for translatorTone == abcTone {
		translatorTone = toneMenu[rng.Intn(len(toneMenu))]
	}
	t.Logf("ABC tone=%.0fHz  Translator tone=%.0fHz", abcTone, translatorTone)

	// Keep streaming while we wait for real Opus decode on both ears.
	// Continuous stream avoids under-race fixed-burst races where encode
	// finishes before the mix path is warm.
	const minSamples = audiocodec.FrameSize * 10
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go abcClient.streamUntilCancel(streamCtx, abcTone)
	go translatorClient.streamUntilCancel(streamCtx, translatorTone)

	feedTone, feedRatio, feedSamples := waitTone(t, translatorClient, toneMenu, minSamples, 20*time.Second)
	bcastTone, bcastRatio, bcastSamples := waitTone(t, broadcast, toneMenu, minSamples, 30*time.Second)
	require.GreaterOrEqual(t, len(feedSamples), minSamples, "translator received too little audio")
	require.GreaterOrEqual(t, len(bcastSamples), minSamples, "broadcast received too little audio")
	require.True(t, pcmHasEnergy(bcastSamples), "broadcast received only silence")
	require.True(t, pcmHasEnergy(feedSamples), "translator ear received only silence")

	t.Logf("feed(translator ear) detected=%.0fHz (ratio %.1f)  broadcast detected=%.0fHz (ratio %.1f)",
		feedTone, feedRatio, bcastTone, bcastRatio)

	assert.Equal(t, abcTone, feedTone, "translator should hear the ABC tone on the feed")
	assert.Greater(t, feedRatio, 2.0, "ABC tone should dominate at the translator")
	assert.Equal(t, translatorTone, bcastTone, "broadcast should hear the translator tone")
	assert.Greater(t, bcastRatio, 2.0, "translator tone should dominate on the broadcast")
	assert.NotEqual(t, translatorTone, feedTone, "translator tone must not leak onto the feed")
	assert.NotEqual(t, abcTone, bcastTone, "ABC tone must not leak onto the broadcast")
}

// TestIntegrationTranslatorMonitorKeepsProducing: translator ticket defaults
// still produce to broadcast when only listen is narrowed at issue time.
func TestIntegrationTranslatorMonitorKeepsProducing(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Monitor Session")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	_ = createChannel(t, env, adminToken, session.ID, "English Broadcast", "broadcast")

	// Translator ticket with default produce (broadcast) + listen feed only.
	// issue without produce so server defaults apply; listen narrowed to feed.
	translatorClient := newTicketMediaClient(t, env, "translator", adminToken, session.ID,
		nil, []string{feedCh.Name}, "translator")
	broadcast := newBroadcastListener(t, env, session.ID, session.BroadcastToken, "broadcast")

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	translatorTone := toneMenu[rng.Intn(len(toneMenu))]
	t.Logf("Translator tone=%.0fHz", translatorTone)

	const minSamples = audiocodec.FrameSize * 10
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go translatorClient.streamUntilCancel(streamCtx, translatorTone)

	bcastTone, bcastRatio, bcastSamples := waitTone(t, broadcast, toneMenu, minSamples, 30*time.Second)
	require.GreaterOrEqual(t, len(bcastSamples), minSamples,
		"broadcast received too little audio — translator likely not producing")
	require.True(t, pcmHasEnergy(bcastSamples), "broadcast received only silence")
	t.Logf("broadcast detected=%.0fHz (ratio %.1f)", bcastTone, bcastRatio)
	assert.Equal(t, translatorTone, bcastTone,
		"broadcast should hear the translator even while the translator monitors a feed")
	assert.Greater(t, bcastRatio, 2.0, "translator tone should dominate on the broadcast")
}

// TestIntegrationMixAssignmentRoutesAudio proves mix table routes without reconnect.
func TestIntegrationMixAssignmentRoutesAudio(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Mix Routing Session")
	_ = createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	broadcastCh := createChannel(t, env, adminToken, session.ID, "English Broadcast", "broadcast")

	_, boothTok := registerABC(t, env, adminToken, "Booth Mix", session.ID)
	boothABC, welcome := newABCClient(t, env, boothTok)
	require.NotNil(t, welcome)
	// adapter: stream via ABC client
	booth := boothABC

	var boothSrcID string
	require.Eventually(t, func() bool {
		srcs, err := env.sources.List(ctx, session.ID)
		if err != nil || len(srcs) == 0 {
			return false
		}
		boothSrcID = srcs[0].ID
		return true
	}, 3*time.Second, 50*time.Millisecond, "booth source should register")

	resp := env.doRequest(t, http.MethodPut,
		"/api/sessions/"+session.ID+"/channels/"+broadcastCh.ID+"/mix", adminToken,
		fmt.Sprintf(`{"entries":[{"source_id":%q,"muted":false,"level":1}]}`, boothSrcID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	broadcast := newBroadcastListener(t, env, session.ID, session.BroadcastToken, "broadcast")

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	boothTone := toneMenu[rng.Intn(len(toneMenu))]
	t.Logf("Booth tone=%.0fHz", boothTone)

	const minSamples = audiocodec.FrameSize * 10
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go booth.streamUntilCancel(streamCtx, boothTone)

	bcastTone, bcastRatio, bcastSamples := waitTone(t, broadcast, toneMenu, minSamples, 30*time.Second)
	require.GreaterOrEqual(t, len(bcastSamples), minSamples,
		"broadcast received too little audio — mix assignment did not route it")
	require.True(t, pcmHasEnergy(bcastSamples), "broadcast received only silence")
	t.Logf("broadcast detected=%.0fHz (ratio %.1f)", bcastTone, bcastRatio)
	assert.Equal(t, boothTone, bcastTone,
		"broadcast should hear the booth after it is assigned to the broadcast mix")
	assert.Greater(t, bcastRatio, 2.0, "booth tone should dominate on the broadcast")
}

// TestIntegrationProduceAndMonitorSameChannel reproduces the K2B scenario:
// a source that produces into a channel AND monitors (listens to) that same
// channel. The server must still receive the source's track and forward it,
// so a separate listener on that channel hears it.
func TestIntegrationProduceAndMonitorSameChannel(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Self Monitor Session")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")

	// ABC produces feed and monitors the same feed channel (self-monitor).
	abcID, prodTok := registerABC(t, env, adminToken, "SelfMon Booth", session.ID)
	resp := env.doRequest(t, http.MethodPut, "/api/abcs/"+abcID, adminToken,
		fmt.Sprintf(`{"name":"SelfMon Booth","session_id":%q,"monitor_channel_id":%q}`, session.ID, feedCh.ID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	producerABC, welcome := newABCClient(t, env, prodTok)
	require.NotNil(t, welcome)
	// Separate listener on feed via admin ticket listen-only (admin default listen=feed).
	listener := newTicketMediaClient(t, env, "listener", adminToken, session.ID,
		nil, []string{feedCh.Name}, "admin")

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	tone := toneMenu[rng.Intn(len(toneMenu))]
	t.Logf("Booth tone=%.0fHz", tone)

	const minSamples = audiocodec.FrameSize * 10
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go producerABC.streamUntilCancel(streamCtx, tone)

	got, ratio, samples := waitTone(t, listener, toneMenu, minSamples, 20*time.Second)
	require.GreaterOrEqual(t, len(samples), minSamples,
		"listener received too little audio — producer track not forwarded")
	require.True(t, pcmHasEnergy(samples), "listener received only silence")
	t.Logf("listener detected=%.0fHz (ratio %.1f)", got, ratio)
	assert.Equal(t, tone, got,
		"listener should hear a source that also monitors its own channel")
	assert.Greater(t, ratio, 2.0, "producer tone should dominate")
}

// TestIntegrationSimultaneousOpusMix is the production-path simultaneous mix
// proof at the full server boundary: two producers 440@1.0 + 880@0.25 on the
// same channel, listener decodes real Opus, both freqs + ratio, live mute/level.
func TestIntegrationSimultaneousOpusMix(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Simultaneous Mix")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")

	_, tokA := registerABC(t, env, adminToken, "prodA", session.ID)
	_, tokB := registerABC(t, env, adminToken, "prodB", session.ID)
	prodA, wA := newABCClient(t, env, tokA)
	require.NotNil(t, wA)
	prodB, wB := newABCClient(t, env, tokB)
	require.NotNil(t, wB)
	listener := newTicketMediaClient(t, env, "listener", adminToken, session.ID,
		nil, []string{feedCh.Name}, "admin")

	require.Eventually(t, func() bool {
		srcs, err := env.sources.List(ctx, session.ID)
		return err == nil && len(srcs) >= 2
	}, 5*time.Second, 50*time.Millisecond, "two sources should register")

	srcs, err := env.sources.List(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, srcs, 2)
	var idA, idB string
	for _, s := range srcs {
		switch {
		case strings.Contains(strings.ToLower(s.Name), "proda") || strings.Contains(s.Name, "prodA"):
			idA = s.ID
		case strings.Contains(strings.ToLower(s.Name), "prodb") || strings.Contains(s.Name, "prodB"):
			idB = s.ID
		}
	}
	if idA == "" || idB == "" {
		idA, idB = srcs[0].ID, srcs[1].ID
	}

	setMix := func(aMuted bool, aLevel, bLevel float64) {
		t.Helper()
		body := fmt.Sprintf(
			`{"entries":[{"source_id":%q,"muted":%v,"level":%v},{"source_id":%q,"muted":false,"level":%v}]}`,
			idA, aMuted, aLevel, idB, bLevel,
		)
		resp := env.doRequest(t, http.MethodPut,
			"/api/sessions/"+session.ID+"/channels/"+feedCh.ID+"/mix", adminToken, body)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}

	setMix(false, 1.0, 0.25)

	// Continuous producers for the whole test so mute/level changes apply live.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); prodA.streamUntilCancel(streamCtx, 440) }()
	go func() { defer wg.Done(); prodB.streamUntilCancel(streamCtx, 880) }()

	// Wait until both tones are audible before measuring levels.
	const minMixSamples = audiocodec.FrameSize * 40
	deadline := time.Now().Add(25 * time.Second)
	var samples []int16
	var mag440, mag880, mag660 float64
	for time.Now().Before(deadline) {
		samples = listener.captured()
		if len(samples) >= minMixSamples && pcmHasEnergy(samples) {
			mag440 = goertzel(samples, 440)
			mag880 = goertzel(samples, 880)
			mag660 = goertzel(samples, 660)
			if mag440 > mag660*1.5 && mag880 > mag660*1.1 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.GreaterOrEqual(t, len(samples), minMixSamples, "listener got too little mixed audio")
	t.Logf("mags: 440=%.3f 880=%.3f 660=%.3f", mag440, mag880, mag660)
	assert.Greater(t, mag440, mag660*1.5, "440 Hz must be present")
	assert.Greater(t, mag880, mag660*1.1, "880 Hz must be present")

	ratio := mag880 / mag440
	t.Logf("B/A magnitude ratio=%.3f (target ~0.25 or ~4 if swapped)", ratio)
	nearQuarter := math.Abs(ratio-0.25) < 0.22
	nearFour := math.Abs(ratio-4.0) < 2.5
	assert.True(t, nearQuarter || nearFour, "level ratio should reflect 1.0 vs 0.25, got %.3f", ratio)

	// Live mute source A without reconnect.
	setMix(true, 1.0, 0.25)
	time.Sleep(500 * time.Millisecond) // mix cache refresh bound ~200ms
	listener.mu.Lock()
	listener.received = nil
	listener.mu.Unlock()
	// Wait for post-mute audio with 880 still present.
	deadline = time.Now().Add(8 * time.Second)
	var after []int16
	var m440b, m880b float64
	for time.Now().Before(deadline) {
		after = listener.captured()
		if len(after) >= audiocodec.FrameSize*20 {
			m440b = goertzel(after, 440)
			m880b = goertzel(after, 880)
			if m880b > 1.0 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.GreaterOrEqual(t, len(after), audiocodec.FrameSize*20)
	t.Logf("after mute A: 440=%.3f 880=%.3f", m440b, m880b)
	if nearQuarter {
		assert.Greater(t, m880b, m440b*1.5+1, "muting A should leave 880 dominant")
	} else {
		assert.True(t, m440b > m880b*1.2+1 || m880b > m440b*1.2+1, "mute should unbalance mix")
	}

	// Level change without reconnect: unmute A, equal levels.
	setMix(false, 1.0, 1.0)
	time.Sleep(500 * time.Millisecond)
	listener.mu.Lock()
	listener.received = nil
	listener.mu.Unlock()
	deadline = time.Now().Add(8 * time.Second)
	var eq []int16
	var g440, g880 float64
	for time.Now().Before(deadline) {
		eq = listener.captured()
		if len(eq) >= audiocodec.FrameSize*20 {
			g440 = goertzel(eq, 440)
			g880 = goertzel(eq, 880)
			if g440 > 1.0 && g880 > 1.0 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.GreaterOrEqual(t, len(eq), audiocodec.FrameSize*20)
	require.Greater(t, g440, 1.0)
	require.Greater(t, g880, 1.0)
	rEq := g880 / g440
	t.Logf("after equal levels B/A=%.3f", rEq)
	assert.InDelta(t, 1.0, rEq, 0.75, "equal levels should yield roughly equal magnitudes")

	cancel()
	wg.Wait()
}

// ── REST helpers ──────────────────────────────────────────────────────────

type sessionResp struct {
	ID             string `json:"id"`
	BroadcastToken string `json:"broadcast_token"`
}
type channelResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// registerABC creates an ABC, optionally assigns a session, returns id+token.
func registerABC(t *testing.T, env *testEnv, adminToken, name, sessionID string) (id, token string) {
	t.Helper()
	resp := env.doRequest(t, http.MethodPost, "/api/abcs", adminToken, fmt.Sprintf(`{"name":%q}`, name))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var abc struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&abc))
	resp.Body.Close()
	require.NotEmpty(t, abc.Token)
	if sessionID != "" {
		resp = env.doRequest(t, http.MethodPut, "/api/abcs/"+abc.ID, adminToken,
			fmt.Sprintf(`{"name":%q,"session_id":%q}`, name, sessionID))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}
	return abc.ID, abc.Token
}

// waitTone waits until captured audio contains a detectable tone from candidates.
// Success requires at least minSamples of post-warmup PCM, a non-zero winner
// frequency, and a confident Goertzel ratio. Silence-only buffers (all-zero
// Opus comfort frames) must NOT pass — empty spectrum yields ratio=+Inf.
func waitTone(t *testing.T, mc *mediaClient, candidates []float64, minSamples int, timeout time.Duration) (freq float64, ratio float64, samples []int16) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		samples = mc.captured()
		if len(samples) >= minSamples && pcmHasEnergy(samples) {
			freq, ratio = detectTone(samples, candidates)
			if freq != 0 && !math.IsInf(ratio, 1) && ratio > 1.5 {
				return freq, ratio, samples
			}
			// Single-tone capture can yield +Inf when runner-up is exactly 0;
			// accept that only when the winner magnitude is clearly non-zero.
			if freq != 0 && math.IsInf(ratio, 1) && goertzel(samples, freq) > 1.0 {
				return freq, ratio, samples
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	samples = mc.captured()
	freq, ratio = detectTone(samples, candidates)
	return freq, ratio, samples
}

// pcmHasEnergy reports whether samples contain non-trivial amplitude (not
// all-silence / comfort-noise near zero).
func pcmHasEnergy(samples []int16) bool {
	const thresh int16 = 200
	var peak int16
	for _, s := range samples {
		v := s
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
		if peak >= thresh {
			return true
		}
	}
	return false
}

func createSession(t *testing.T, env *testEnv, token, name string) sessionResp {
	t.Helper()
	resp := env.doRequest(t, http.MethodPost, "/api/sessions", token,
		fmt.Sprintf(`{"name":%q}`, name))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var s sessionResp
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&s))
	require.NotEmpty(t, s.ID)
	require.NotEmpty(t, s.BroadcastToken)
	return s
}

func createChannel(t *testing.T, env *testEnv, token, sessionID, name, typ string) channelResp {
	t.Helper()
	resp := env.doRequest(t, http.MethodPost, "/api/sessions/"+sessionID+"/channels", token,
		fmt.Sprintf(`{"name":%q,"type":%q}`, name, typ))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var c channelResp
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&c))
	require.NotEmpty(t, c.ID)
	return c
}
