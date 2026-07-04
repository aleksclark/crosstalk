package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mixer"
	"github.com/aleksclark/crosstalk/server/orchestrator"
	"github.com/aleksclark/crosstalk/server/postgres"
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
	var s0, s1, s2 float64
	for _, x := range samples {
		s0 = float64(x) + k*s1 - s2
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
	scores := make([]score, 0, len(candidates))
	for _, f := range candidates {
		scores = append(scores, score{f, goertzel(samples, f)})
	}
	var top, second score
	for _, s := range scores {
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

// captureSink accumulates mixed frames delivered to a channel sink.
type captureSink struct {
	mu      sync.Mutex
	samples []int16
}

func (c *captureSink) writer() orchestrator.SinkWriter {
	return func(_ string, frame []int16) {
		cp := make([]int16, len(frame)) // frame is reused by the mixer; copy it
		copy(cp, frame)
		c.mu.Lock()
		c.samples = append(c.samples, cp...)
		c.mu.Unlock()
	}
}

// window returns a middle slice of captured samples, skipping warm-up frames.
func (c *captureSink) window() []int16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.samples
	skip := mixer.FrameSize * 5
	if len(s) <= skip {
		return append([]int16(nil), s...)
	}
	return append([]int16(nil), s[skip:]...)
}

// ── Test ──────────────────────────────────────────────────────────────────

// TestIntegrationAudioFlowABCToTranslatorToBroadcast exercises the full v3
// audio-routing path with REAL infrastructure end to end:
//
//   - Real PostgreSQL (Bun) database (per-test, provisioned by pgtest).
//   - Real REST API over httptest: admin logs in, creates a session with a
//     "feed" and a "broadcast" channel, creates a translator, and assigns the
//     session to that translator.
//   - Real translator login through the same auth path the translate interface
//     uses, verifying the translator sees the assigned session.
//   - Real orchestrator + mixer routing engine carrying actual PCM audio.
//
// Routing under test (mirrors the product's mix-based routing):
//
//	ABC ──(feed channel)──▶ Translator     (translator hears the ABC/floor feed)
//	Translator ──(broadcast channel)──▶ Broadcast client
//
// Each source emits a RANDOMLY selected tone. The test asserts that the
// correct tone arrives at the correct destination and that tones do not leak
// across channels (correct-destination validation).
func TestIntegrationAudioFlowABCToTranslatorToBroadcast(t *testing.T) {
	env := setupIntegrationServer(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	// Fresh stores bound to the same DB the API server uses.
	db := env.db
	channelStore := postgres.NewChannelStore(db)
	sourceStore := postgres.NewSourceStore(db)
	mixStore := postgres.NewMixStore(db)

	// ── 1. Admin creates a session + channels + translator (via REST) ──────
	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Sunday Service")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	broadcastCh := createChannel(t, env, adminToken, session.ID, "English Broadcast", "broadcast")

	// Create translator and assign the session.
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

	// ── 3. Register the connecting sources (ABC + translator mic) ──────────
	// Sources are created in the store when WebRTC peers connect. We model the
	// ABC and the translator's microphone connecting to the session.
	abcSource := &crosstalk.Source{
		ID: ulid.Make().String(), SessionID: session.ID,
		Name: "Booth A (ABC)", Origin: crosstalk.OriginABC, Connected: true,
	}
	translatorSource := &crosstalk.Source{
		ID: ulid.Make().String(), SessionID: session.ID,
		Name: "Maria (Translator)", Origin: crosstalk.OriginTranslator, Connected: true,
	}
	require.NoError(t, sourceStore.Create(ctx, abcSource))
	require.NoError(t, sourceStore.Create(ctx, translatorSource))

	// ── 4. Configure mix (admin, via REST) to express the routing ──────────
	// Feed channel carries only the ABC; broadcast channel carries only the
	// translator. The "off" source is muted per channel.
	setMix(t, env, adminToken, session.ID, feedCh.ID, []mixEntry{
		{abcSource.ID, false, 1.0},
		{translatorSource.ID, true, 1.0},
	})
	setMix(t, env, adminToken, session.ID, broadcastCh.ID, []mixEntry{
		{abcSource.ID, true, 1.0},
		{translatorSource.ID, false, 1.0},
	})

	// ── 5. Bring up the orchestrator + mixers for the session ──────────────
	orch := orchestrator.New(orchestrator.Config{
		SessionID:    session.ID,
		MixStore:     mixStore,
		ChannelStore: channelStore,
		SourceStore:  sourceStore,
		Logger:       log,
	})
	require.NoError(t, orch.Initialize(ctx))
	require.Equal(t, 2, orch.ChannelCount())

	require.NoError(t, orch.SourceConnect(ctx, *abcSource))
	require.NoError(t, orch.SourceConnect(ctx, *translatorSource))

	// Sinks: the translator listens to the feed; the broadcast client listens
	// to the broadcast channel.
	translatorEar := &captureSink{}
	broadcastEar := &captureSink{}
	orch.ForwardOutput(feedCh.ID, "translator-listener", translatorEar.writer())
	orch.ForwardOutput(broadcastCh.ID, "broadcast-client", broadcastEar.writer())

	orch.StartMixers()
	defer orch.StopMixers()

	// ── 6. Each source emits a randomly selected (distinct) tone ───────────
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	abcTone := toneMenu[rng.Intn(len(toneMenu))]
	translatorTone := abcTone
	for translatorTone == abcTone {
		translatorTone = toneMenu[rng.Intn(len(toneMenu))]
	}
	t.Logf("ABC tone=%.0fHz  Translator tone=%.0fHz", abcTone, translatorTone)

	// Stream ~700ms of audio from both sources, paced to the mixer's 20ms tick.
	const frames = 35
	var wg sync.WaitGroup
	wg.Add(2)
	stream := func(sourceID string, freq float64) {
		defer wg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		start := 0
		for i := 0; i < frames; i++ {
			<-ticker.C
			orch.WriteAudio(sourceID, sineFrame(freq, start))
			start += mixer.FrameSize
		}
	}
	go stream(abcSource.ID, abcTone)
	go stream(translatorSource.ID, translatorTone)
	wg.Wait()
	time.Sleep(60 * time.Millisecond) // let final frames drain to sinks

	// ── 7. Validate the correct tone reached each destination ──────────────
	feedSamples := translatorEar.window()
	bcastSamples := broadcastEar.window()
	require.Greater(t, len(feedSamples), mixer.FrameSize*5, "translator received too little audio")
	require.Greater(t, len(bcastSamples), mixer.FrameSize*5, "broadcast received too little audio")

	feedTone, feedRatio := detectTone(feedSamples, toneMenu)
	bcastTone, bcastRatio := detectTone(bcastSamples, toneMenu)
	t.Logf("feed detected=%.0fHz (ratio %.1f)  broadcast detected=%.0fHz (ratio %.1f)",
		feedTone, feedRatio, bcastTone, bcastRatio)

	// Translator (feed sink) must hear the ABC's tone, dominantly.
	assert.Equal(t, abcTone, feedTone, "translator should hear the ABC tone on the feed")
	assert.Greater(t, feedRatio, 3.0, "ABC tone should dominate on the feed")

	// Broadcast client must hear the translator's tone, dominantly.
	assert.Equal(t, translatorTone, bcastTone, "broadcast should hear the translator tone")
	assert.Greater(t, bcastRatio, 3.0, "translator tone should dominate on the broadcast")

	// Correct-destination: tones must not leak across channels.
	assert.NotEqual(t, translatorTone, feedTone, "translator tone must not leak onto the feed")
	assert.NotEqual(t, abcTone, bcastTone, "ABC tone must not leak onto the broadcast")
}

// ── REST helpers ──────────────────────────────────────────────────────────

type sessionResp struct {
	ID string `json:"id"`
}

type channelResp struct {
	ID string `json:"id"`
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

type mixEntry struct {
	sourceID string
	muted    bool
	level    float64
}

func setMix(t *testing.T, env *testEnv, token, sessionID, channelID string, entries []mixEntry) {
	t.Helper()
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf(`{"source_id":%q,"muted":%t,"level":%g}`,
			e.sourceID, e.muted, e.level))
	}
	body := fmt.Sprintf(`{"entries":[%s]}`, strings.Join(parts, ","))
	resp := env.doRequest(t, http.MethodPut,
		"/api/sessions/"+sessionID+"/channels/"+channelID+"/mix", token, body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
