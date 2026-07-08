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
	"github.com/pion/rtp"
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
	// Identity is a stable, connection-independent key for the producing
	// participant (e.g. a user ID or "abc:<id>"). Reconnects with the same
	// Identity reuse the same Source instead of spawning a new one. When empty
	// the (ephemeral) peer ID is used, restoring the old per-connection behavior.
	Identity string
	// Label is the human-friendly Source name shown in the admin mix UI. When
	// empty a name is derived from the role and identity.
	Label string
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
//
// Multiple sources may forward into the same channel (e.g. a booth and a
// translator both assigned to a broadcast channel). Their RTP streams have
// independent sequence-number and timestamp spaces, so the hub rewrites every
// outgoing packet into ONE continuous, monotonic stream. Without this, when a
// source with higher sequence numbers stops, the remaining source's lower
// sequence numbers look "stale" to the listener's jitter buffer and playback
// stalls until the receiver is recreated (a page reload).
type channelHub struct {
	channel crosstalk.Channel
	track   *pionwebrtc.TrackLocalStaticRTP

	mu       sync.Mutex
	seq      uint16
	ts       uint32
	haveLast bool
	lastInTS uint32
}

// writeRTP forwards one packet onto the channel's shared track, rewriting the
// sequence number to the hub's monotonic counter and advancing the timestamp by
// a sane per-packet delta so the outbound stream stays continuous across source
// switches. The incoming packet is not mutated (it may be written to several
// hubs), so a shallow copy with a rewritten header is sent.
func (h *channelHub) writeRTP(pkt *rtp.Packet) error {
	seq, ts := h.nextHeader(pkt.Timestamp)
	out := *pkt
	out.Header = pkt.Header
	out.Header.SequenceNumber = seq
	out.Header.Timestamp = ts
	return h.track.WriteRTP(&out)
}

// nextHeader advances the hub's monotonic sequence/timestamp counters for one
// outbound packet, given the inbound packet's timestamp. Sequence numbers are
// strictly monotonic regardless of which source fed the packet; the timestamp
// advances by the source's own inter-packet delta when it looks sane, else by a
// default 20ms Opus frame (covering first packet and source-switch
// discontinuities). Split out from writeRTP so the rewrite is unit-testable.
func (h *channelHub) nextHeader(inTS uint32) (seq uint16, ts uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	const defaultDelta = 960 // 20ms at 48kHz
	var delta uint32 = defaultDelta
	if h.haveLast {
		d := inTS - h.lastInTS   // wrap-safe (uint32)
		if d > 0 && d <= 12000 { // up to 250ms — within one continuous source
			delta = d
		}
	}
	h.haveLast = true
	h.lastInTS = inTS
	h.ts += delta
	return h.seq, h.ts
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

	// Publish: register a source + a mix entry per produced channel (so it is
	// visible/controllable in the admin mix UI), then forward this peer's
	// inbound RTP. Which channels actually receive the audio is driven by the
	// mix table (any channel with an unmuted entry for this source), so admin/
	// translator mix edits route audio without a reconnect.
	if len(produceIDs) > 0 {
		src, err := f.ensureSource(ctx, peer.ID, opts)
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
			go f.forward(src.ID, track)
		})
		peer.OnClose(func() {
			_ = f.stores.Sources.Update(context.Background(), &crosstalk.Source{
				ID: src.ID, SessionID: f.sessionID, Name: src.Name,
				Origin: src.Origin, PeerID: src.PeerID, Connected: false,
			})
		})
	}

	f.logger.Info("sessionrtc: bridged peer",
		"peer", peer.ID, "session", f.sessionID,
		"role", opts.Role, "produce", produceIDs, "listen", listenIDs)
	return nil
}

// ensureSource creates (or reuses) the Source this peer publishes as. When
// opts.Identity is set, an existing Source with the same identity is reused
// (marked connected again) so a translator that reconnects does not accumulate
// a fresh source on every connect/disconnect cycle. The identity is persisted
// in the Source's PeerID column as the durable dedup key.
func (f *sessionFwd) ensureSource(ctx context.Context, peerID string, opts BridgeOpts) (*crosstalk.Source, error) {
	origin := crosstalk.SourceOrigin(opts.Role)
	switch origin {
	case crosstalk.OriginABC, crosstalk.OriginTranslator, crosstalk.OriginAdmin:
	default:
		origin = crosstalk.OriginTranslator
	}

	identity := opts.Identity
	if identity == "" {
		// No stable identity supplied: fall back to the ephemeral peer ID,
		// preserving the previous per-connection source behavior.
		identity = peerID
	}
	label := opts.Label
	if label == "" {
		label = fmt.Sprintf("%s %s", opts.Role, identity)
	}

	// Reuse an existing source with the same durable identity, if any.
	if existing := f.findSourceByIdentity(ctx, identity); existing != nil {
		existing.Name = label
		existing.Origin = origin
		existing.Connected = true
		if err := f.stores.Sources.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("sessionrtc: reactivate source: %w", err)
		}
		return existing, nil
	}

	src := &crosstalk.Source{
		ID:        ulid.Make().String(),
		SessionID: f.sessionID,
		Name:      label,
		Origin:    origin,
		PeerID:    &identity,
		Connected: true,
	}
	if err := f.stores.Sources.Create(ctx, src); err != nil {
		// A concurrent connect from the same identity may have won the race and
		// created the row first (violating UNIQUE(session, name)); reuse it.
		if existing := f.findSourceByIdentity(ctx, identity); existing != nil {
			existing.Connected = true
			if uerr := f.stores.Sources.Update(ctx, existing); uerr != nil {
				return nil, fmt.Errorf("sessionrtc: reactivate source: %w", uerr)
			}
			return existing, nil
		}
		return nil, fmt.Errorf("sessionrtc: create source: %w", err)
	}
	return src, nil
}

// findSourceByIdentity returns the session's source whose durable identity
// (PeerID) matches, or nil. Errors are treated as "not found" so a lookup
// failure degrades to creating a fresh source rather than dropping the peer.
func (f *sessionFwd) findSourceByIdentity(ctx context.Context, identity string) *crosstalk.Source {
	if identity == "" {
		return nil
	}
	sources, err := f.stores.Sources.List(ctx, f.sessionID)
	if err != nil {
		return nil
	}
	for i := range sources {
		if sources[i].PeerID != nil && *sources[i].PeerID == identity {
			return &sources[i]
		}
	}
	return nil
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

// forward copies RTP from a remote source track into every channel that has an
// unmuted mix entry for the source. The target set is recomputed periodically
// so admin/translator mix edits — assigning the source to a channel, removing
// it, or muting it — take effect without decoding or a reconnect. The mix table
// is the single source of truth for routing.
func (f *sessionFwd) forward(sourceID string, track *pionwebrtc.TrackRemote) {
	f.logger.Info("sessionrtc: forwarding source track",
		"source", sourceID, "codec", track.Codec().MimeType)

	// active is the set of channel IDs currently receiving this source.
	var active []string
	var lastRefresh time.Time
	refresh := func() {
		var next []string
		for _, chID := range f.allChannelIDs() {
			entries, err := f.stores.Mix.GetMix(context.Background(), chID)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.SourceID == sourceID && !e.Muted {
					next = append(next, chID)
					break
				}
			}
		}
		active = next
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
		for _, chID := range active {
			if h := f.hub(chID); h != nil {
				_ = h.writeRTP(pkt)
			}
		}
	}
}

// allChannelIDs returns a snapshot of the session's resolved channel IDs.
func (f *sessionFwd) allChannelIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.channels))
	for id := range f.channels {
		out = append(out, id)
	}
	return out
}
