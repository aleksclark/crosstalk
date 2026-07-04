// Package sessionrtc bridges WebRTC peers into a session's audio mixer.
//
// It owns one orchestrator.SessionOrchestrator per live session and connects
// each peer's media to it:
//
//   - Inbound: the peer's remote audio track is read as RTP, its payload
//     decoded to PCM (via mixer.NullDecoder), and written into the session
//     mixer as the peer's source.
//   - Outbound: the mixed output of a chosen channel is delivered to the peer
//     over a server-added audio track (PCM re-encoded via mixer.NullEncoder).
//
// The RTP payload carries raw little-endian int16 PCM rather than Opus; this
// matches the mixer's first-class Null codec and lets the full WebRTC transport
// (ICE/DTLS/SRTP/RTP) plus the real orchestrator + mixer be exercised
// end-to-end without a native Opus dependency.
package sessionrtc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mixer"
	"github.com/aleksclark/crosstalk/server/orchestrator"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// rtpPayloadType is the dynamic payload type used for the (PCM-carrying) audio.
const rtpPayloadType = 111

// Stores holds the persistence dependencies needed to build orchestrators.
type Stores struct {
	Channels crosstalk.ChannelService
	Sources  crosstalk.SourceService
	Mix      crosstalk.MixService
}

// Manager owns per-session orchestrators and bridges peers into them.
type Manager struct {
	stores Stores
	logger *slog.Logger

	mu    sync.Mutex
	orchs map[string]*orchestrator.SessionOrchestrator
}

// NewManager creates a session media manager.
func NewManager(stores Stores, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		stores: stores,
		logger: logger,
		orchs:  make(map[string]*orchestrator.SessionOrchestrator),
	}
}

// orchestrator returns the (lazily created, initialized, running) orchestrator
// for a session.
func (m *Manager) orchestrator(ctx context.Context, sessionID string) (*orchestrator.SessionOrchestrator, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if orch, ok := m.orchs[sessionID]; ok {
		return orch, nil
	}

	orch := orchestrator.New(orchestrator.Config{
		SessionID:    sessionID,
		MixStore:     m.stores.Mix,
		ChannelStore: m.stores.Channels,
		SourceStore:  m.stores.Sources,
		Logger:       m.logger,
	})
	if err := orch.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("sessionrtc: initialize orchestrator: %w", err)
	}
	orch.StartMixers()
	m.orchs[sessionID] = orch
	return orch, nil
}

// Bridge wires a peer's media into the session mixer:
//
//   - sourceID identifies the (already persisted) source the peer produces into.
//   - listenChannelID, when non-empty, is the channel whose mixed output is sent
//     back to the peer on a server-added track.
//
// It must be called before the SDP answer is generated so the outbound track is
// negotiated in the initial exchange.
func (m *Manager) Bridge(ctx context.Context, peer *webrtc.PeerConn, sessionID, sourceID, listenChannelID string) error {
	orch, err := m.orchestrator(ctx, sessionID)
	if err != nil {
		return err
	}

	// Outbound: deliver a channel's mixed output to this peer.
	if listenChannelID != "" {
		track, err := pionwebrtc.NewTrackLocalStaticRTP(
			pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus},
			"mix-"+listenChannelID,
			"session-"+sessionID,
		)
		if err != nil {
			return fmt.Errorf("sessionrtc: create outbound track: %w", err)
		}
		sender, err := peer.AddTrack(track)
		if err != nil {
			return fmt.Errorf("sessionrtc: add outbound track: %w", err)
		}
		// Drain RTCP so the sender doesn't stall.
		go func() {
			buf := make([]byte, 1500)
			for {
				if _, _, rerr := sender.Read(buf); rerr != nil {
					return
				}
			}
		}()

		orch.ForwardOutput(listenChannelID, peer.ID, newSinkWriter(track))
	}

	// A listen-only peer (no source) is fully wired now: it only receives.
	if sourceID == "" {
		peer.OnClose(func() {
			if listenChannelID != "" {
				orch.RemoveSink(listenChannelID, peer.ID)
			}
		})
		m.logger.Info("sessionrtc: bridged listen-only peer",
			"peer", peer.ID, "session", sessionID, "listen_channel", listenChannelID)
		return nil
	}

	src, err := m.stores.Sources.Get(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("sessionrtc: load source %s: %w", sourceID, err)
	}

	// Inbound: route the peer's remote track into the mixer as sourceID.
	peer.OnRemoteTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		if track.Kind() != pionwebrtc.RTPCodecTypeAudio {
			return
		}
		go readTrackIntoMixer(orch, sourceID, track, m.logger)
	})

	// Register the source as connected in the orchestrator.
	if err := orch.SourceConnect(ctx, *src); err != nil {
		return fmt.Errorf("sessionrtc: source connect: %w", err)
	}

	// Tear down on peer close.
	peer.OnClose(func() {
		if listenChannelID != "" {
			orch.RemoveSink(listenChannelID, peer.ID)
		}
		_ = orch.SourceDisconnect(context.Background(), sourceID)
	})

	m.logger.Info("sessionrtc: bridged peer",
		"peer", peer.ID, "session", sessionID,
		"source", sourceID, "listen_channel", listenChannelID)
	return nil
}

// samplesPerPacket bounds each RTP audio packet so the PCM payload stays well
// under a typical 1500-byte UDP MTU (480 samples = 960 bytes + headers).
const samplesPerPacket = 480

// newSinkWriter returns an orchestrator sink that packetizes mixed PCM frames
// into MTU-sized RTP packets and writes them to the given track.
func newSinkWriter(track *pionwebrtc.TrackLocalStaticRTP) orchestrator.SinkWriter {
	enc := mixer.NullEncoder{}
	buf := make([]byte, samplesPerPacket*2)
	var seq uint16
	var ts uint32
	const ssrc = 0x53455353 // "SESS"

	return func(_ string, frame []int16) {
		for off := 0; off < len(frame); off += samplesPerPacket {
			end := off + samplesPerPacket
			if end > len(frame) {
				end = len(frame)
			}
			chunk := frame[off:end]
			n, err := enc.Encode(chunk, buf)
			if err != nil {
				return
			}
			payload := make([]byte, n)
			copy(payload, buf[:n])
			seq++
			ts += uint32(len(chunk))
			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    rtpPayloadType,
					SequenceNumber: seq,
					Timestamp:      ts,
					SSRC:           ssrc,
				},
				Payload: payload,
			}
			_ = track.WriteRTP(pkt)
		}
	}
}

// readTrackIntoMixer reads RTP from a remote track, decodes the PCM payload, and
// writes the samples into the session mixer as sourceID.
func readTrackIntoMixer(orch *orchestrator.SessionOrchestrator, sourceID string, track *pionwebrtc.TrackRemote, logger *slog.Logger) {
	dec := mixer.NullDecoder{}
	pcm := make([]int16, mixer.FrameSize*8)
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			logger.Debug("sessionrtc: remote track read ended", "source", sourceID, "err", err)
			return
		}
		if len(pkt.Payload) == 0 {
			continue
		}
		n, err := dec.Decode(pkt.Payload, pcm)
		if err != nil || n == 0 {
			continue
		}
		samples := make([]int16, n)
		copy(samples, pcm[:n])
		orch.WriteAudio(sourceID, samples)
	}
}
