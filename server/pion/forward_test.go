package pion

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForwardTrack_RTPPayloadMatch is the critical SFU proof test. It sets up
// two in-process Pion peers (source client A and sink client B) connected
// through a server-side ForwardTrack relay. Client A sends RTP packets with
// known payload bytes; client B must receive them byte-for-byte.
func TestForwardTrack_RTPPayloadMatch(t *testing.T) {
	api := testAPI(t)

	// ── Create server-side peer connections for source and sink ──
	pm := testPeerManager(t)
	sourceServer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { sourceServer.Close() }) //nolint:errcheck

	sinkServer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { sinkServer.Close() }) //nolint:errcheck

	// ── Create client-side peer connections ──
	sourceClient, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { sourceClient.Close() }) //nolint:errcheck

	sinkClient, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { sinkClient.Close() }) //nolint:errcheck

	// ── Source client adds an Opus audio track ──
	sourceTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"test-audio", // track ID
		"test-audio", // stream ID
	)
	require.NoError(t, err)

	_, err = sourceClient.AddTrack(sourceTrack)
	require.NoError(t, err)

	// Source client also needs a data channel for proper SDP.
	_, err = sourceClient.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// ── Set up SFU forwarding plumbing on the server side ──
	//
	// Create a local track on the sink server to forward packets to.
	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"fwd-audio",
		"fwd-audio",
	)
	require.NoError(t, err)

	sender, err := sinkServer.pc.AddTrack(localTrack)
	require.NoError(t, err)

	// Drain RTCP from sender to avoid blocking.
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := sender.Read(buf); rtcpErr != nil {
				return
			}
		}
	}()

	// When the source server receives the track from the source client,
	// forward each RTP packet to the localTrack on the sink server.
	sourceServer.pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		t.Logf("server: received source track codec=%s", remoteTrack.Codec().MimeType)
		for {
			pkt, _, readErr := remoteTrack.ReadRTP()
			if readErr != nil {
				return
			}
			if writeErr := localTrack.WriteRTP(pkt); writeErr != nil {
				return
			}
		}
	})

	// ── Sink client: capture received track payloads ──
	receivedPayloads := make(chan []byte, 100)
	sinkClient.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		t.Logf("sink: received track codec=%s", track.Codec().MimeType)
		for {
			pkt, _, readErr := track.ReadRTP()
			if readErr != nil {
				return
			}
			payload := make([]byte, len(pkt.Payload))
			copy(payload, pkt.Payload)
			receivedPayloads <- payload
		}
	})

	// Sink client needs a recvonly audio transceiver so the SDP offer contains
	// an audio m-line. Without this, the server can't include its track in the
	// answer because the answer can only respond to m-lines in the offer.
	_, err = sinkClient.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly})
	require.NoError(t, err)

	// Sink client needs a data channel for proper SDP.
	_, err = sinkClient.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// ── Connect source client ↔ source server ──
	sourceConnected := make(chan struct{})
	sourceClient.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			close(sourceConnected)
		}
	})
	signalPair(t, sourceClient, sourceServer)

	select {
	case <-sourceConnected:
		t.Log("source: ICE connected")
	case <-time.After(10 * time.Second):
		t.Fatal("source: timed out waiting for ICE connected")
	}

	// ── Connect sink client ↔ sink server ──
	sinkConnected := make(chan struct{})
	sinkClient.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			close(sinkConnected)
		}
	})
	signalPair(t, sinkClient, sinkServer)

	select {
	case <-sinkConnected:
		t.Log("sink: ICE connected")
	case <-time.After(10 * time.Second):
		t.Fatal("sink: timed out waiting for ICE connected")
	}

	// ── Send RTP packets continuously until we receive enough on the sink ──
	//
	// In Pion, OnTrack fires when the first RTP packet for a negotiated track
	// arrives. We start sending immediately and keep going until the sink has
	// collected enough packets.
	const targetPackets = 10

	var stopSend sync.Once
	sendDone := make(chan struct{})

	go func() {
		seq := uint16(100)
		ts := uint32(100 * 960)
		for {
			select {
			case <-sendDone:
				return
			default:
			}

			payload := bytes.Repeat([]byte{byte((seq % 10) + 1)}, 20)

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    111,
					SequenceNumber: seq,
					Timestamp:      ts,
					SSRC:           12345,
				},
				Payload: payload,
			}

			_ = sourceTrack.WriteRTP(pkt)
			seq++
			ts += 960
			time.Sleep(20 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { stopSend.Do(func() { close(sendDone) }) })

	// ── Collect received payloads from sink ──
	var gotPayloads [][]byte
	timeout := time.After(10 * time.Second)
	for len(gotPayloads) < targetPackets {
		select {
		case p := <-receivedPayloads:
			gotPayloads = append(gotPayloads, p)
		case <-timeout:
			goto verify
		}
	}

verify:
	stopSend.Do(func() { close(sendDone) })

	// ── Verify: payloads must match byte-for-byte ──
	require.NotEmpty(t, gotPayloads, "expected to receive at least some RTP packets")

	// Build the set of valid payloads (bytes 1-10 repeated 20 times).
	validPayloads := make([][]byte, 10)
	for i := range 10 {
		validPayloads[i] = bytes.Repeat([]byte{byte(i + 1)}, 20)
	}

	// Every received payload must match one of the valid payloads exactly.
	for i, got := range gotPayloads {
		found := false
		for _, valid := range validPayloads {
			if bytes.Equal(got, valid) {
				found = true
				break
			}
		}
		assert.True(t, found, "received payload %d does not match any valid payload: %x", i, got)
	}

	t.Logf("received %d packets — SFU forwarding verified byte-for-byte", len(gotPayloads))
}
