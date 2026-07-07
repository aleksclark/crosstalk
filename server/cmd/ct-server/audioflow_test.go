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

	"github.com/aleksclark/crosstalk/server/mixer"
)

// ── Audio helpers ─────────────────────────────────────────────────────────

const sampleRate = 48000

// toneMenu is the set of well-separated frequencies (Hz) a source may emit.
// A source picks one at random for each test run.
var toneMenu = []float64{440, 587, 659, 880, 1046}

// sineFrame fills a FrameSize frame of a sine wave at freq, continuous across
// frames via startSample. Amplitude is ~0.5 of full scale.
func sineFrame(freq float64, startSample int) []int16 {
	frame := make([]int16, mixer.FrameSize)
	for i := range frame {
		n := float64(startSample + i)
		frame[i] = int16(0.5 * math.MaxInt16 * math.Sin(2*math.Pi*freq*n/float64(sampleRate)))
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
// session signaling endpoint, sends a tone, and captures received audio.
type mediaClient struct {
	name      string
	pc        *pionwebrtc.PeerConnection
	ws        *websocket.Conn
	sendTrack *pionwebrtc.TrackLocalStaticRTP
	connected chan struct{}

	mu       sync.Mutex
	received []int16
}

// newMediaClient dials the given ws URL, negotiates a bidirectional audio
// session with the server, and returns once ICE is connected.
func newMediaClient(t *testing.T, name, wsURL string) *mediaClient {
	t.Helper()

	var se pionwebrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := pionwebrtc.NewAPI(pionwebrtc.WithSettingEngine(se))

	pc, err := api.NewPeerConnection(pionwebrtc.Configuration{})
	require.NoError(t, err, "%s: new pc", name)

	track, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus},
		name+"-mic", name,
	)
	require.NoError(t, err, "%s: new track", name)

	// One bidirectional audio transceiver: the client sends its tone and
	// receives the server's mixed channel output on the same m-line.
	_, err = pc.AddTransceiverFromTrack(track, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionSendrecv,
	})
	require.NoError(t, err, "%s: add transceiver", name)

	mc := &mediaClient{
		name:      name,
		pc:        pc,
		sendTrack: track,
		connected: make(chan struct{}),
	}

	pc.OnTrack(func(remote *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		dec := mixer.NullDecoder{}
		pcm := make([]int16, mixer.FrameSize*8)
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
	pc.OnICEConnectionStateChange(func(state pionwebrtc.ICEConnectionState) {
		if state == pionwebrtc.ICEConnectionStateConnected {
			once.Do(func() { close(mc.connected) })
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

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err, "%s: create offer", name)
	require.NoError(t, pc.SetLocalDescription(offer), "%s: set local", name)

	offerMsg, _ := json.Marshal(map[string]string{"type": "offer", "sdp": offer.SDP})
	require.NoError(t, ws.Write(ctx, websocket.MessageText, offerMsg), "%s: send offer", name)

	go mc.readSignaling(ctx)

	select {
	case <-mc.connected:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: ICE connect timeout", name)
	}

	t.Cleanup(func() {
		_ = ws.Close(websocket.StatusNormalClosure, "")
		_ = pc.Close()
	})
	return mc
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

// stream sends `frames` 20ms sine-tone frames, split into MTU-sized RTP
// packets, at the mixer's real-time cadence.
func (mc *mediaClient) stream(ctx context.Context, freq float64, frames int) {
	const samplesPerPacket = 480 // 960-byte payload, safely under MTU
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
		}
		start += len(frame)
	}
}

// captured returns a copy of received samples, skipping warm-up frames.
func (mc *mediaClient) captured() []int16 {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	skip := mixer.FrameSize * 8
	if len(mc.received) <= skip {
		return append([]int16(nil), mc.received...)
	}
	return append([]int16(nil), mc.received[skip:]...)
}

// ── Test ──────────────────────────────────────────────────────────────────

// TestIntegrationAudioFlowABCToTranslatorToBroadcast is the full v3 golden
// audio-flow test over REAL WebRTC transport, end to end:
//
//   - Real PostgreSQL (Bun) database (per-test, provisioned by pgtest).
//   - Real REST API: admin logs in, creates a session with a "feed" and a
//     "broadcast" channel, creates a translator, assigns the session to them,
//     and configures the per-channel mix.
//   - Real translator login through the same auth path the translate interface
//     uses, verifying the translator sees the assigned session.
//   - Real WebRTC peers (pion) connecting to /api/sessions/{id}/ws: full
//     ICE/DTLS/SRTP/RTP transport into the real orchestrator + mixer.
//
// Routing under test:
//
//	ABC ──(feed channel)──▶ Translator     (translator hears the ABC/floor feed)
//	Translator ──(broadcast channel)──▶ Broadcast client
//
// Each source emits a RANDOMLY selected tone. The test asserts the correct tone
// arrives at the correct destination and does not leak across channels.
func TestIntegrationAudioFlowABCToTranslatorToBroadcast(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	// ── 1. Admin creates session + channels + translator (via REST) ────────
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

	// ── 2. Translator logs in via the translate interface (auth path) ──────
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

	// ── 3. Connect real WebRTC peers to the SFU, declaring produce/listen ──
	// Routing is expressed by which channels each peer produces into / listens
	// to; the server auto-registers a Source + mix entry per produced channel.
	//
	//   ABC         produces → "Floor Feed"
	//   Translator  listens  ← "Floor Feed", produces → "English Broadcast"
	//   Broadcast   listens  ← "English Broadcast"
	wsBase := strings.Replace(env.server.URL, "http://", "ws://", 1) + "/api/sessions/" + session.ID + "/ws"
	abcWS := fmt.Sprintf("%s?produce=%s", wsBase, url.QueryEscape(feedCh.Name))
	translatorWS := fmt.Sprintf("%s?produce=%s&listen=%s",
		wsBase, url.QueryEscape(broadcastCh.Name), url.QueryEscape(feedCh.Name))
	broadcastWS := fmt.Sprintf("%s?listen=%s", wsBase, url.QueryEscape(broadcastCh.Name))

	abc := newMediaClient(t, "abc", abcWS)
	translatorClient := newMediaClient(t, "translator", translatorWS)
	broadcast := newMediaClient(t, "broadcast", broadcastWS)

	// ── 4. Each producing source emits a randomly selected (distinct) tone ──
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	abcTone := toneMenu[rng.Intn(len(toneMenu))]
	translatorTone := abcTone
	for translatorTone == abcTone {
		translatorTone = toneMenu[rng.Intn(len(toneMenu))]
	}
	t.Logf("ABC tone=%.0fHz  Translator tone=%.0fHz", abcTone, translatorTone)

	const frames = 100 // ~2s of audio
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); abc.stream(streamCtx, abcTone, frames) }()
	go func() { defer wg.Done(); translatorClient.stream(streamCtx, translatorTone, frames) }()
	wg.Wait()
	time.Sleep(200 * time.Millisecond) // let final frames traverse the pipeline

	// ── 7. Validate the correct tone reached each destination ──────────────
	feedSamples := translatorClient.captured()
	bcastSamples := broadcast.captured()
	require.Greater(t, len(feedSamples), mixer.FrameSize*10, "translator received too little audio")
	require.Greater(t, len(bcastSamples), mixer.FrameSize*10, "broadcast received too little audio")

	feedTone, feedRatio := detectTone(feedSamples, toneMenu)
	bcastTone, bcastRatio := detectTone(bcastSamples, toneMenu)
	t.Logf("feed(translator ear) detected=%.0fHz (ratio %.1f)  broadcast detected=%.0fHz (ratio %.1f)",
		feedTone, feedRatio, bcastTone, bcastRatio)

	// Translator hears the ABC's tone on the feed, dominantly.
	assert.Equal(t, abcTone, feedTone, "translator should hear the ABC tone on the feed")
	assert.Greater(t, feedRatio, 3.0, "ABC tone should dominate at the translator")

	// Broadcast client hears the translator's tone, dominantly.
	assert.Equal(t, translatorTone, bcastTone, "broadcast should hear the translator tone")
	assert.Greater(t, bcastRatio, 3.0, "translator tone should dominate on the broadcast")

	// Correct-destination: tones must not leak across channels.
	assert.NotEqual(t, translatorTone, feedTone, "translator tone must not leak onto the feed")
	assert.NotEqual(t, abcTone, bcastTone, "ABC tone must not leak onto the broadcast")
}

// TestIntegrationTranslatorMonitorKeepsProducing is the regression test for
// "selecting a channel to monitor stops translator→broadcast audio". When a
// translator picks a monitor channel, the client connects with only a listen
// selector (?listen=<feed>) and no produce param. The server must still apply
// the translator's default produce (broadcast), so audio keeps flowing to the
// broadcast — overriding one direction must not wipe the other.
func TestIntegrationTranslatorMonitorKeepsProducing(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Monitor Session")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	broadcastCh := createChannel(t, env, adminToken, session.ID, "English Broadcast", "broadcast")

	wsBase := strings.Replace(env.server.URL, "http://", "ws://", 1) + "/api/sessions/" + session.ID + "/ws"

	// Translator connects with ONLY a listen selector (as the monitor picker
	// does): produce is absent and must fall back to the role default
	// (broadcast).
	translatorWS := fmt.Sprintf("%s?listen=%s", wsBase, url.QueryEscape(feedCh.Name))
	translatorClient := newMediaClient(t, "translator", translatorWS)

	// Broadcast listener on the broadcast channel.
	broadcastWS := fmt.Sprintf("%s?listen=%s", wsBase, url.QueryEscape(broadcastCh.Name))
	broadcast := newMediaClient(t, "broadcast", broadcastWS)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	translatorTone := toneMenu[rng.Intn(len(toneMenu))]
	t.Logf("Translator tone=%.0fHz", translatorTone)

	const frames = 100 // ~2s
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go translatorClient.stream(streamCtx, translatorTone, frames)
	time.Sleep(2500 * time.Millisecond)

	bcastSamples := broadcast.captured()
	require.Greater(t, len(bcastSamples), mixer.FrameSize*10,
		"broadcast received too little audio — translator likely not producing")
	bcastTone, bcastRatio := detectTone(bcastSamples, toneMenu)
	t.Logf("broadcast detected=%.0fHz (ratio %.1f)", bcastTone, bcastRatio)
	assert.Equal(t, translatorTone, bcastTone,
		"broadcast should hear the translator even while the translator monitors a feed")
	assert.Greater(t, bcastRatio, 3.0, "translator tone should dominate on the broadcast")
}

// ── REST helpers ──────────────────────────────────────────────────────────
type sessionResp struct {
	ID string `json:"id"`
}

type channelResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
