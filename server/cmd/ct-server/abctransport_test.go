package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/crosstalk/abc"
	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/audiocodec"
)

const transportSentinelToken = "sentinel-abc-token-do-not-leak"

func TestIntegrationABCTransportAssignedWelcomeAndSend(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Transport Session")
	feedCh := createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	_, token := registerABC(t, env, adminToken, "Transport Booth", session.ID)

	sess := dialABCTransport(t, env, token)
	welcome := sess.Welcome()
	assert.Equal(t, session.ID, welcome.AssignedSessionID)
	assert.NotZero(t, welcome.Epoch)
	codec, ok := sess.NegotiatedCodec()
	if ok {
		assert.NotEmpty(t, codec.MimeType)
	}

	require.Eventually(t, func() bool {
		srcs, err := env.sources.List(ctx, session.ID)
		return err == nil && len(srcs) == 1
	}, 5*time.Second, 50*time.Millisecond, "assigned ABC must appear as a source")
	srcs, err := env.sources.List(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	assert.Equal(t, crosstalk.OriginABC, srcs[0].Origin)
	assert.Equal(t, "Transport Booth", srcs[0].Name)

	listener := newTicketMediaClient(t, env, "listener", adminToken, session.ID,
		[]string{}, []string{feedCh.Name}, "admin")

	const minSamples = audiocodec.FrameSize * 10
	tone := 587.0
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go streamABCTone(streamCtx, sess.SendTrack(), tone)

	got, ratio, samples := waitTone(t, listener, toneMenu, minSamples, 20*time.Second)
	require.GreaterOrEqual(t, len(samples), minSamples)
	require.True(t, pcmHasEnergy(samples))
	assert.Equal(t, tone, got)
	assert.Greater(t, ratio, 2.0)
}

func TestIntegrationABCTransportMonitorTrack(t *testing.T) {
	env := setupIntegrationServer(t)
	ctx := context.Background()

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Monitor Session")
	bcast := createChannel(t, env, adminToken, session.ID, "English Broadcast", "broadcast")
	abcID, token := registerABC(t, env, adminToken, "Monitor Booth", session.ID)
	resp := env.doRequest(t, http.MethodPut, "/api/abcs/"+abcID, adminToken,
		fmt.Sprintf(`{"name":"Monitor Booth","session_id":%q,"monitor_channel_id":%q}`, session.ID, bcast.ID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	sess := dialABCTransport(t, env, token)
	cap := newTransportCapture()
	sess.OnTrack(func(track abc.IncomingTrack) {
		go cap.decode(track)
	})

	producer := newTicketMediaClient(t, env, "producer", adminToken, session.ID,
		[]string{bcast.Name}, nil, "admin")

	const minSamples = audiocodec.FrameSize * 10
	tone := 880.0
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go producer.streamUntilCancel(streamCtx, tone)

	got, ratio, samples := waitCapturedTone(t, cap, toneMenu, minSamples, 25*time.Second)
	require.GreaterOrEqual(t, len(samples), minSamples, "ABC transport received too little monitor audio")
	require.True(t, pcmHasEnergy(samples), "ABC transport received only silence")
	assert.Equal(t, tone, got)
	assert.Greater(t, ratio, 2.0)
}

func TestIntegrationABCTransportInvalidToken(t *testing.T) {
	env := setupIntegrationServer(t)
	before := env.peerCount()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := abc.Dial(ctx, transportConfig(env, transportSentinelToken))
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.True(t, abc.IsAuthError(err), "got %T %v", err, err)
	assert.NotContains(t, err.Error(), transportSentinelToken)
	assert.Equal(t, before, env.peerCount(), "invalid token must not allocate a peer")
}

func TestIntegrationABCTransportMalformedControl(t *testing.T) {
	env := setupIntegrationServer(t)
	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")
	session := createSession(t, env, adminToken, "Protocol Session")
	createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")
	_, token := registerABC(t, env, adminToken, "Protocol Booth", session.ID)

	sess := dialABCTransport(t, env, token)
	err := sess.SendControl([]byte{0xff, 0x00, 0x01})
	require.Error(t, err)
	assert.True(t, abc.IsProtocolError(err), "got %T %v", err, err)

	select {
	case <-sess.Done():
	case <-time.After(8 * time.Second):
		t.Fatal("malformed control did not close the session")
	}
	assert.True(t, abc.IsProtocolError(sess.Err()), "got %T %v", sess.Err(), sess.Err())
	assert.NotContains(t, sess.Err().Error(), token)
	assert.Equal(t, abc.StateFailed, sess.State())
}

func TestIntegrationABCTransportAssignmentChangeEpoch(t *testing.T) {
	env := setupIntegrationServer(t)
	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session1 := createSession(t, env, adminToken, "Epoch Session A")
	session2 := createSession(t, env, adminToken, "Epoch Session B")
	createChannel(t, env, adminToken, session1.ID, "Floor Feed", "feed")
	createChannel(t, env, adminToken, session2.ID, "Floor Feed", "feed")
	abcID, token := registerABC(t, env, adminToken, "Epoch Booth", session1.ID)

	first := dialABCTransport(t, env, token)
	firstEpoch := first.Welcome().Epoch
	require.NotZero(t, firstEpoch)

	resp := env.doRequest(t, http.MethodPut, "/api/abcs/"+abcID, adminToken,
		fmt.Sprintf(`{"name":"Epoch Booth","session_id":%q}`, session2.ID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	select {
	case <-first.Done():
	case <-time.After(8 * time.Second):
		t.Fatal("assignment change did not end the first epoch")
	}
	assert.NotEmpty(t, first.CloseReason())

	second := dialABCTransport(t, env, token)
	assert.Equal(t, session2.ID, second.Welcome().AssignedSessionID)
	assert.NotEqual(t, firstEpoch, second.Welcome().Epoch)
}

func dialABCTransport(t *testing.T, env *testEnv, token string) *abc.Session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	sess, err := abc.Dial(ctx, transportConfig(env, token))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })
	require.Eventually(t, func() bool {
		ice := sess.ICEState()
		pc := sess.PeerConnectionState()
		return ice == abc.ICEStateConnected || ice == abc.ICEStateCompleted ||
			pc == pionwebrtc.PeerConnectionStateConnected
	}, 30*time.Second, 50*time.Millisecond, "ABC transport ICE did not connect")
	return sess
}

func transportConfig(env *testEnv, token string) abc.Config {
	return abc.Config{
		ServerURL:      env.server.URL,
		Token:          token,
		ClientName:     "abc-transport-test",
		RequireWelcome: true,
		DisableMDNS:    true,
		ICEServers:     []pionwebrtc.ICEServer{},
		WelcomeTimeout: 20 * time.Second,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func streamABCTone(ctx context.Context, dest abc.RTPWriter, freq float64) {
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
	for {
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
		_ = dest.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: seq, Timestamp: ts, SSRC: 0xABC0},
			Payload: payload,
		})
		start += len(frame)
	}
}

type transportCapture struct {
	mu       sync.Mutex
	received []int16
}

func newTransportCapture() *transportCapture {
	return &transportCapture{}
}

func (c *transportCapture) decode(track abc.IncomingTrack) {
	dec, err := audiocodec.NewOpusDecoder(1)
	if err != nil {
		return
	}
	defer dec.Close()
	pcm := make([]int16, audiocodec.FrameSize*2)
	for {
		pkt, err := track.ReadRTP()
		if err != nil {
			return
		}
		if pkt == nil || len(pkt.Payload) == 0 {
			continue
		}
		n, err := dec.Decode(pkt.Payload, pcm)
		if err != nil || n == 0 {
			continue
		}
		c.mu.Lock()
		c.received = append(c.received, pcm[:n]...)
		c.mu.Unlock()
	}
}

func (c *transportCapture) captured() []int16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	skip := audiocodec.FrameSize * 8
	if len(c.received) <= skip {
		return append([]int16(nil), c.received...)
	}
	return append([]int16(nil), c.received[skip:]...)
}

func waitCapturedTone(t *testing.T, cap *transportCapture, candidates []float64, minSamples int, timeout time.Duration) (freq float64, ratio float64, samples []int16) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		samples = cap.captured()
		if len(samples) >= minSamples && pcmHasEnergy(samples) {
			freq, ratio = detectTone(samples, candidates)
			if freq != 0 && ratio > 1.5 {
				return freq, ratio, samples
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	samples = cap.captured()
	freq, ratio = detectTone(samples, candidates)
	return freq, ratio, samples
}
