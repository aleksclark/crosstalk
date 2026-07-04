// Package sessionrtc is the WebRTC SFU that carries session audio between peers.
//
// It is a passthrough forwarder: audio arrives from a publishing peer as RTP
// and is forwarded, payload untouched, to every peer subscribed to the channel
// the source feeds. Because payloads are never decoded, the same code path
// carries browser Opus and the raw-PCM packets used by the Go integration
// tests — the server needs no audio codec (important for CGO-free builds).
//
// Routing mirrors the session's data model:
//
//   - A publishing peer "produces" into one or more channels (by name). Each
//     produce registers a Source and a channel_mix entry so the admin can see
//     and mute it; forwarding is gated on that mute state.
//   - A subscribing peer "listens" to one or more channels; it receives a
//     server-added track per channel carrying that channel's forwarded audio.
//
// Per-source volume (mix level) cannot be applied without decoding, so only
// mute/unmute is honored on this path. True level mixing lives in the
// orchestrator/mixer packages, exercised directly by their unit tests.
package sessionrtc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	pionwebrtc "github.com/pion/webrtc/v4"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// Stores holds the persistence dependencies the SFU needs.
type Stores struct {
	Channels crosstalk.ChannelService
	Sources  crosstalk.SourceService
	Mix      crosstalk.MixService
}

// BridgeOpts describes how a peer participates in a session.
type BridgeOpts struct {
	// Role records the source origin ("translator", "admin", "abc") for any
	// channels this peer produces into.
	Role string
	// Produce lists channel names this peer publishes audio into.
	Produce []string
	// Listen lists channel names this peer receives audio from.
	Listen []string
	// ServerOffer requests a server-initiated SDP offer (used by receive-only
	// broadcast listeners that add no track of their own).
	ServerOffer bool
}

// Manager owns one forwarder per live session.
type Manager struct {
	stores Stores
	logger *slog.Logger

	mu       sync.Mutex
	sessions map[string]*sessionFwd
}

// NewManager creates a session SFU manager.
func NewManager(stores Stores, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		stores:   stores,
		logger:   logger,
		sessions: make(map[string]*sessionFwd),
	}
}

// forwarder returns the (lazily created) forwarder for a session.
func (m *Manager) forwarder(sessionID string) *sessionFwd {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.sessions[sessionID]; ok {
		return f
	}
	f := &sessionFwd{
		sessionID: sessionID,
		stores:    m.stores,
		logger:    m.logger,
		channels:  make(map[string]*channelHub),
	}
	m.sessions[sessionID] = f
	return f
}

// Bridge wires a peer into the session per opts. It must run before the SDP
// answer/offer is produced so server-added tracks are negotiated up front.
func (m *Manager) Bridge(ctx context.Context, peer *webrtc.PeerConn, sessionID string, opts BridgeOpts) error {
	f := m.forwarder(sessionID)
	return f.bridge(ctx, peer, opts)
}

// ── per-session forwarder ────────────────────────────────────────────────

type sessionFwd struct {
	sessionID string
	stores    Stores
	logger    *slog.Logger

	mu       sync.Mutex
	channels map[string]*channelHub // channelID -> hub
	resolved bool
	byName   map[string]string // channel name -> id
	byType   map[string][]string
}

// channelHub carries one channel's forwarded audio to all subscribers via a
// single shared local track.
type channelHub struct {
	channel crosstalk.Channel
	track   *pionwebrtc.TrackLocalStaticRTP
}

// resolveChannels loads the session's channels once and builds name/type
// indexes plus a hub (with a shared local track) per channel.
func (f *sessionFwd) resolveChannels(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resolved {
		return nil
	}
	chans, err := f.stores.Channels.List(ctx, f.sessionID)
	if err != nil {
		return fmt.Errorf("sessionrtc: list channels: %w", err)
	}
	f.byName = make(map[string]string, len(chans))
	f.byType = make(map[string][]string)
	for _, ch := range chans {
		hub, err := newChannelHub(ch)
		if err != nil {
			return err
		}
		f.channels[ch.ID] = hub
		f.byName[ch.Name] = ch.ID
		f.byType[string(ch.Type)] = append(f.byType[string(ch.Type)], ch.ID)
	}
	f.resolved = true
	return nil
}

func newChannelHub(ch crosstalk.Channel) (*channelHub, error) {
	track, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus},
		"ch-"+ch.ID,
		"session-"+ch.SessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("sessionrtc: create channel track: %w", err)
	}
	return &channelHub{channel: ch, track: track}, nil
}

// channelIDsFor maps requested channel names to IDs. Unknown names are skipped.
// The special name "*broadcast" / "*feed" expands to all channels of that type.
func (f *sessionFwd) channelIDsFor(names []string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		switch {
		case n == "":
			continue
		case strings.HasPrefix(n, "type:"):
			for _, id := range f.byType[strings.TrimPrefix(n, "type:")] {
				add(id)
			}
		default:
			add(f.byName[n])
		}
	}
	return out
}

func (f *sessionFwd) hub(id string) *channelHub {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.channels[id]
}

// bridge performs the produce/listen wiring for a peer.
func (f *sessionFwd) bridge(ctx context.Context, peer *webrtc.PeerConn, opts BridgeOpts) error {
	if err := f.resolveChannels(ctx); err != nil {
		return err
	}

	listenIDs := f.channelIDsFor(opts.Listen)
	produceIDs := f.channelIDsFor(opts.Produce)

	// Subscribe: add each listened channel's shared track to this peer.
	//
	// Server-offer listeners (receive-only broadcast) get the track now, before
	// the server creates its offer. Client-offer peers get it in OnBeforeAnswer
	// so it binds to the transceiver the client already offered — a single
	// sendrecv m-line both sends the peer's audio and receives the subscribed
	// channel, with no extra renegotiation.
	subscribe := func() {
		for _, chID := range listenIDs {
			h := f.hub(chID)
			if h == nil {
				continue
			}
			sender, err := peer.AddTrack(h.track)
			if err != nil {
				f.logger.Error("sessionrtc: subscribe failed", "channel", chID, "err", err)
				continue
			}
			go func() {
				buf := make([]byte, 1500)
				for {
					if _, _, rerr := sender.Read(buf); rerr != nil {
						return
					}
				}
			}()
		}
	}
	if len(listenIDs) > 0 {
		if opts.ServerOffer {
			subscribe()
		} else {
			peer.OnBeforeAnswer(subscribe)
		}
	}

	// Publish: register a source + mix entry per produced channel, then forward
	// this peer's inbound RTP into those channels (gated by mute).
	if len(produceIDs) > 0 {
		src, err := f.ensureSource(ctx, peer.ID, opts.Role)
		if err != nil {
			return err
		}
		for _, chID := range produceIDs {
			if err := f.ensureMixEntry(ctx, chID, src.ID); err != nil {
				return err
			}
		}
		peer.OnRemoteTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
			if track.Kind() != pionwebrtc.RTPCodecTypeAudio {
				return
			}
			go f.forward(src.ID, produceIDs, track)
		})
		peer.OnClose(func() {
			_ = f.stores.Sources.Update(context.Background(), &crosstalk.Source{
				ID: src.ID, SessionID: f.sessionID, Name: src.Name,
				Origin: src.Origin, Connected: false,
			})
		})
	}

	f.logger.Info("sessionrtc: bridged peer",
		"peer", peer.ID, "session", f.sessionID,
		"role", opts.Role, "produce", produceIDs, "listen", listenIDs)
	return nil
}

// ensureSource creates (or reuses) the Source this peer publishes as.
func (f *sessionFwd) ensureSource(ctx context.Context, peerID, role string) (*crosstalk.Source, error) {
	origin := crosstalk.SourceOrigin(role)
	switch origin {
	case crosstalk.OriginABC, crosstalk.OriginTranslator, crosstalk.OriginAdmin:
	default:
		origin = crosstalk.OriginTranslator
	}
	src := &crosstalk.Source{
		ID:        ulid.Make().String(),
		SessionID: f.sessionID,
		// The peer ID (a ULID) is globally unique, keeping the (session, name)
		// constraint satisfied even for same-role peers that connect together.
		Name:      fmt.Sprintf("%s %s", role, peerID),
		Origin:    origin,
		PeerID:    &peerID,
		Connected: true,
	}
	if err := f.stores.Sources.Create(ctx, src); err != nil {
		return nil, fmt.Errorf("sessionrtc: create source: %w", err)
	}
	return src, nil
}

// ensureMixEntry makes sure (channel, source) has a mix entry (default: unmuted,
// level 1.0) so the source is visible and controllable in the admin mix UI.
func (f *sessionFwd) ensureMixEntry(ctx context.Context, channelID, sourceID string) error {
	entries, err := f.stores.Mix.GetMix(ctx, channelID)
	if err != nil {
		return fmt.Errorf("sessionrtc: get mix: %w", err)
	}
	for _, e := range entries {
		if e.SourceID == sourceID {
			return nil
		}
	}
	entries = append(entries, crosstalk.MixEntry{
		ChannelID: channelID, SourceID: sourceID, Muted: false, Level: 1.0,
	})
	if err := f.stores.Mix.SetMix(ctx, channelID, entries); err != nil {
		return fmt.Errorf("sessionrtc: set mix: %w", err)
	}
	return nil
}

// forward copies RTP from a remote track into every produced channel's shared
// track, skipping channels where this source is currently muted.
func (f *sessionFwd) forward(sourceID string, channelIDs []string, track *pionwebrtc.TrackRemote) {
	f.logger.Info("sessionrtc: forwarding source track",
		"source", sourceID, "channels", channelIDs, "codec", track.Codec().MimeType)
	// Cache of muted state per channel, refreshed periodically so admin mute
	// changes take effect without decoding or reconnecting.
	muted := make(map[string]bool, len(channelIDs))
	var lastRefresh time.Time
	refresh := func() {
		for _, chID := range channelIDs {
			entries, err := f.stores.Mix.GetMix(context.Background(), chID)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.SourceID == sourceID {
					muted[chID] = e.Muted
				}
			}
		}
		lastRefresh = time.Now()
	}
	refresh()

	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			f.logger.Debug("sessionrtc: source track ended", "source", sourceID, "err", err)
			return
		}
		if time.Since(lastRefresh) > 500*time.Millisecond {
			refresh()
		}
		for _, chID := range channelIDs {
			if muted[chID] {
				continue
			}
			if h := f.hub(chID); h != nil {
				_ = h.track.WriteRTP(pkt)
			}
		}
	}
}
