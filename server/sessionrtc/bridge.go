// Package sessionrtc is the WebRTC SFU that carries session audio between peers.
//
// Production path is REAL decoded mixing:
//
//  1. read remote RTP
//  2. bounded reorder window (seq/ts), 3–6 packets / ≤120ms
//  3. decode each source independently to 20ms/960 PCM
//  4. place on channel clock
//  5. PLC/silence for late/missing after bound
//  6. mix (level/mute/limit) → Opus encode → one monotonic RTP stream
//
// Encoded RTP passthrough is forbidden as the production path.
// NullEncoder/NullDecoder must not appear here.
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
	"github.com/aleksclark/crosstalk/server/audiocodec"
	"github.com/aleksclark/crosstalk/server/mixer"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// ── Observer seam (no OTel dependency) ───────────────────────────────────

// Observer receives non-blocking media-path telemetry. Implementations must
// not block; the media engine calls these from hot paths.
type Observer interface {
	PeerAuthorized(sessionID, subject, role string)
	SourceFrame(sessionID, sourceID string, samples int)
	FrameDropped(sessionID, sourceID, reason string)
	MixedFrame(sessionID, channelID string, activeSources int, peak float64)
	PeerClosed(sessionID, subject, reason string)
}

// NopObserver is the default no-op Observer.
type NopObserver struct{}

func (NopObserver) PeerAuthorized(string, string, string)          {}
func (NopObserver) SourceFrame(string, string, int)                {}
func (NopObserver) FrameDropped(string, string, string)            {}
func (NopObserver) MixedFrame(string, string, int, float64)        {}
func (NopObserver) PeerClosed(string, string, string)              {}

// ── Codec factory seam ───────────────────────────────────────────────────

// CodecFactory creates per-source decoders and per-channel encoders.
type CodecFactory interface {
	NewEncoder() (audiocodec.Encoder, error)
	NewDecoder(channels int) (audiocodec.Decoder, error)
}

// ── Stores / opts / manager ──────────────────────────────────────────────

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
	// Subject is an optional auth subject for observer events.
	Subject string
}

// Manager owns one forwarder per live session.
type Manager struct {
	stores  Stores
	logger  *slog.Logger
	obs     Observer
	factory CodecFactory

	mu       sync.Mutex
	sessions map[string]*sessionFwd
}

// ManagerOption configures the Manager.
type ManagerOption func(*Manager)

// WithObserver sets the media observer (default: NopObserver).
func WithObserver(o Observer) ManagerOption {
	return func(m *Manager) {
		if o != nil {
			m.obs = o
		}
	}
}

// WithCodecFactory overrides the Opus codec factory (tests may inject fakes).
func WithCodecFactory(f CodecFactory) ManagerOption {
	return func(m *Manager) {
		if f != nil {
			m.factory = f
		}
	}
}

// NewManager creates a session SFU manager with production Opus codecs.
func NewManager(stores Stores, logger *slog.Logger, opts ...ManagerOption) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		stores:   stores,
		logger:   logger,
		obs:      NopObserver{},
		factory:  audiocodec.Factory{},
		sessions: make(map[string]*sessionFwd),
	}
	for _, o := range opts {
		o(m)
	}
	return m
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
		obs:       m.obs,
		factory:   m.factory,
		channels:  make(map[string]*channelHub),
		sources:   make(map[string]*sourcePump),
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
	obs       Observer
	factory   CodecFactory

	mu       sync.Mutex
	channels map[string]*channelHub // channelID -> hub
	sources  map[string]*sourcePump // sourceID -> pump
	resolved bool
	byName   map[string]string // channel name -> id
	byType   map[string][]string
}

// channelHub owns one channel's mixer, Opus encoder, and shared local track.
// Each 20ms tick: snapshot mix entries → mix PCM → encode Opus → write RTP.
type channelHub struct {
	channel   crosstalk.Channel
	sessionID string
	track     *pionwebrtc.TrackLocalStaticRTP
	mix       *mixer.Mixer
	enc       audiocodec.Encoder
	obs       Observer
	logger    *slog.Logger

	mu      sync.Mutex
	seq     uint16
	ts      uint32
	started bool

	// cached mix levels refreshed periodically (no PG on every packet).
	mixMu     sync.RWMutex
	mixCache  map[string]cachedMix // sourceID -> entry
	lastFetch time.Time
	mixStore  crosstalk.MixService
}

type cachedMix struct {
	level float64
	muted bool
}

// sourcePump reads RTP from one remote track, reorders within a bound,
// decodes independently, and writes PCM into every channel mixer input.
type sourcePump struct {
	sourceID  string
	sessionID string
	dec       audiocodec.Decoder
	obs       Observer
	logger    *slog.Logger
	fwd       *sessionFwd

	// jitter: bounded reorder window
	jb *jitterBuffer

	stopCh   chan struct{}
	stopOnce sync.Once
}

// ── jitter buffer ────────────────────────────────────────────────────────

const (
	// jitterMaxPackets is the reorder window size (3–6 packets).
	jitterMaxPackets = 6
	// jitterMaxSpanTS is ≤120ms at 48kHz (120ms * 48 = 5760).
	jitterMaxSpanTS = 5760
	// frameSamples is one Opus frame.
	frameSamples = audiocodec.FrameSize // 960
	// default frame timestamp delta.
	frameTSDelta = uint32(frameSamples)
)

type jbPacket struct {
	seq     uint16
	ts      uint32
	payload []byte
}

// jitterBuffer is a bounded reorder queue keyed by RTP sequence number.
// Overflow drops the oldest packet and reports via onDrop.
type jitterBuffer struct {
	mu       sync.Mutex
	packets  map[uint16]*jbPacket
	order    []uint16 // insertion-ordered seqs for oldest-drop
	maxPkts  int
	maxSpan  uint32
	haveBase bool
	nextSeq  uint16
	onDrop   func(reason string)
}

func newJitterBuffer(onDrop func(reason string)) *jitterBuffer {
	return &jitterBuffer{
		packets: make(map[uint16]*jbPacket, jitterMaxPackets),
		maxPkts: jitterMaxPackets,
		maxSpan: jitterMaxSpanTS,
		onDrop:  onDrop,
	}
}

// Push inserts a packet. Duplicates are ignored. On overflow, oldest is dropped.
// nextSeq is not locked until the first PopReady so early reordering works.
func (j *jitterBuffer) Push(seq uint16, ts uint32, payload []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if _, exists := j.packets[seq]; exists {
		return
	}
	// If we have already started consuming, ignore packets that are too old
	// (already delivered or declared lost).
	if j.haveBase {
		// seq is "before" nextSeq when gap from seq→nextSeq is small positive
		// under uint16 wrap: nextSeq-seq in (0, maxPkts*2].
		gap := j.nextSeq - seq
		if gap != 0 && gap <= uint16(j.maxPkts*2) {
			// late / already-passed
			if j.onDrop != nil {
				j.onDrop("late")
			}
			return
		}
	}

	cp := make([]byte, len(payload))
	copy(cp, payload)
	j.packets[seq] = &jbPacket{seq: seq, ts: ts, payload: cp}
	j.order = append(j.order, seq)

	// Bound by count.
	for len(j.packets) > j.maxPkts {
		j.dropOldestLocked("overflow")
	}
	// Bound by timestamp span.
	j.trimSpanLocked()
}

func (j *jitterBuffer) dropOldestLocked(reason string) {
	if len(j.order) == 0 {
		return
	}
	old := j.order[0]
	j.order = j.order[1:]
	if _, ok := j.packets[old]; ok {
		delete(j.packets, old)
		if j.onDrop != nil {
			j.onDrop(reason)
		}
	}
}

func (j *jitterBuffer) trimSpanLocked() {
	if len(j.packets) < 2 {
		return
	}
	var minTS, maxTS uint32
	first := true
	for _, p := range j.packets {
		if first {
			minTS, maxTS = p.ts, p.ts
			first = false
			continue
		}
		// unsigned-aware min/max for nearby timestamps
		if tsLess(p.ts, minTS) {
			minTS = p.ts
		}
		if tsLess(maxTS, p.ts) {
			maxTS = p.ts
		}
	}
	for maxTS-minTS > j.maxSpan && len(j.packets) > 1 {
		j.dropOldestLocked("span")
		// recompute min
		first = true
		for _, p := range j.packets {
			if first {
				minTS, maxTS = p.ts, p.ts
				first = false
				continue
			}
			if tsLess(p.ts, minTS) {
				minTS = p.ts
			}
			if tsLess(maxTS, p.ts) {
				maxTS = p.ts
			}
		}
	}
}

func tsLess(a, b uint32) bool {
	return int32(a-b) < 0
}
// PopReady returns the next in-sequence packet if present.
// On the first call it anchors nextSeq to the lowest sequence currently held
// so packets that arrived reordered before the first pop are delivered in order.
// If the head is missing past the window, it advances (caller should PLC).
func (j *jitterBuffer) PopReady() (payload []byte, ts uint32, ok bool, lost bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.packets) == 0 {
		return nil, 0, false, false
	}
	if !j.haveBase {
		// Anchor to the lowest seq currently buffered (reorder-friendly start).
		var minSeq uint16
		first := true
		for s := range j.packets {
			if first || seqLess(s, minSeq) {
				minSeq = s
				first = false
			}
		}
		j.nextSeq = minSeq
		j.haveBase = true
	}
	if p, exists := j.packets[j.nextSeq]; exists {
		delete(j.packets, j.nextSeq)
		j.removeOrderLocked(j.nextSeq)
		payload = p.payload
		ts = p.ts
		j.nextSeq++
		return payload, ts, true, false
	}
	// Head missing: if buffer is full of newer packets, declare loss and advance.
	if len(j.packets) >= j.maxPkts {
		j.nextSeq++
		return nil, 0, false, true
	}
	// Check if oldest held packet is far ahead in seq space (> maxPkts gap).
	var minGap uint16 = 0xffff
	for s := range j.packets {
		gap := s - j.nextSeq
		if gap < minGap {
			minGap = gap
		}
	}
	if minGap > 0 && int(minGap) >= j.maxPkts {
		j.nextSeq++
		return nil, 0, false, true
	}
	return nil, 0, false, false
}

// seqLess reports whether a is before b in uint16 sequence space (half-range).
func seqLess(a, b uint16) bool {
	return int16(a-b) < 0
}

// ForcePopPLC advances the expected sequence for a missing packet (PLC slot).
func (j *jitterBuffer) ForceLost() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.haveBase {
		j.nextSeq++
	}
}

func (j *jitterBuffer) removeOrderLocked(seq uint16) {
	for i, s := range j.order {
		if s == seq {
			j.order = append(j.order[:i], j.order[i+1:]...)
			return
		}
	}
}

func (j *jitterBuffer) Len() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.packets)
}

// ── channel hub ──────────────────────────────────────────────────────────

func newChannelHub(ch crosstalk.Channel, sessionID string, factory CodecFactory, mixStore crosstalk.MixService, obs Observer, logger *slog.Logger) (*channelHub, error) {
	track, err := pionwebrtc.NewTrackLocalStaticRTP(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeOpus},
		"ch-"+ch.ID,
		"session-"+ch.SessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("sessionrtc: create channel track: %w", err)
	}
	enc, err := factory.NewEncoder()
	if err != nil {
		return nil, fmt.Errorf("sessionrtc: create encoder: %w", err)
	}
	h := &channelHub{
		channel:   ch,
		sessionID: sessionID,
		track:     track,
		enc:       enc,
		obs:       obs,
		logger:    logger,
		mixCache:  make(map[string]cachedMix),
		mixStore:  mixStore,
	}
	h.mix = mixer.New(func(frame []int16) {
		h.emitMixed(frame)
	}, mixer.WithStats(func(st mixer.MixStats) {
		h.obs.MixedFrame(sessionID, ch.ID, st.ActiveSources, st.Peak)
	}), mixer.WithRingSize(frameSamples*10))
	return h, nil
}

// start launches the 20ms mix loop once.
func (h *channelHub) start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return
	}
	h.started = true
	go h.mix.Run()
}

// ensureInput registers a mixer input for sourceID.
func (h *channelHub) ensureInput(sourceID string, level float64, muted bool) {
	h.mix.AddInput(sourceID, level, muted)
	h.start()
}

// setMix applies level/mute without reconnect.
func (h *channelHub) setMix(sourceID string, level float64, muted bool) {
	_ = h.mix.SetLevel(sourceID, level)
	_ = h.mix.SetMuted(sourceID, muted)
	h.mixMu.Lock()
	h.mixCache[sourceID] = cachedMix{level: level, muted: muted}
	h.mixMu.Unlock()
}

// refreshMixCache pulls mix entries from the store at a low rate (not per packet).
func (h *channelHub) refreshMixCache() {
	h.mixMu.RLock()
	stale := time.Since(h.lastFetch) > 200*time.Millisecond
	h.mixMu.RUnlock()
	if !stale || h.mixStore == nil {
		return
	}
	entries, err := h.mixStore.GetMix(context.Background(), h.channel.ID)
	if err != nil {
		return
	}
	h.mixMu.Lock()
	h.mixCache = make(map[string]cachedMix, len(entries))
	for _, e := range entries {
		h.mixCache[e.SourceID] = cachedMix{level: e.Level, muted: e.Muted}
		_ = h.mix.SetLevel(e.SourceID, e.Level)
		_ = h.mix.SetMuted(e.SourceID, e.Muted)
	}
	h.lastFetch = time.Now()
	h.mixMu.Unlock()
}

// writePCM pushes one decoded frame into the mixer for sourceID.
func (h *channelHub) writePCM(sourceID string, pcm []int16) {
	h.refreshMixCache()
	// Ensure input exists (default unmuted/level 1 if not yet in cache).
	if h.mix.GetInput(sourceID) == nil {
		level, muted := 1.0, false
		h.mixMu.RLock()
		if c, ok := h.mixCache[sourceID]; ok {
			level, muted = c.level, c.muted
		}
		h.mixMu.RUnlock()
		h.ensureInput(sourceID, level, muted)
	}
	_ = h.mix.WriteToInput(sourceID, pcm)
}

// emitMixed encodes one PCM frame to Opus and writes monotonic RTP.
func (h *channelHub) emitMixed(frame []int16) {
	buf := make([]byte, 4000)
	n, err := h.enc.Encode(frame, buf)
	if err != nil || n == 0 {
		return
	}
	h.mu.Lock()
	h.seq++
	h.ts += frameTSDelta
	seq, ts := h.seq, h.ts
	h.mu.Unlock()

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111, // common dynamic PT for Opus; Pion rewrites as needed
			SequenceNumber: seq,
			Timestamp:      ts,
			Marker:         false,
		},
		Payload: buf[:n],
	}
	_ = h.track.WriteRTP(pkt)
}

// nextHeader is retained for unit tests of monotonic seq/ts advancement.
func (h *channelHub) nextHeader(inTS uint32) (seq uint16, ts uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	const defaultDelta = 960
	_ = inTS
	h.ts += defaultDelta
	return h.seq, h.ts
}

// ── resolve / bridge ─────────────────────────────────────────────────────

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
		hub, err := newChannelHub(ch, f.sessionID, f.factory, f.stores.Mix, f.obs, f.logger)
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

// channelIDsFor maps requested channel names to IDs. Unknown names are skipped.
// The special name "type:broadcast" / "type:feed" expands to all channels of that type.
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

	subject := opts.Subject
	if subject == "" {
		subject = opts.Identity
	}
	if subject == "" {
		subject = peer.ID
	}
	f.obs.PeerAuthorized(f.sessionID, subject, opts.Role)

	// Subscribe: add each listened channel's shared track to this peer.
	subscribe := func() {
		for _, chID := range listenIDs {
			h := f.hub(chID)
			if h == nil {
				continue
			}
			h.start()
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

	// Publish: register source + mix entry, then decode→mix path.
	if len(produceIDs) > 0 {
		src, err := f.ensureSource(ctx, peer.ID, opts)
		if err != nil {
			return err
		}
		for _, chID := range produceIDs {
			if err := f.ensureMixEntry(ctx, chID, src.ID); err != nil {
				return err
			}
			if h := f.hub(chID); h != nil {
				// Seed mixer input from current mix entry.
				level, muted := 1.0, false
				if entries, err := f.stores.Mix.GetMix(ctx, chID); err == nil {
					for _, e := range entries {
						if e.SourceID == src.ID {
							level, muted = e.Level, e.Muted
							break
						}
					}
				}
				h.ensureInput(src.ID, level, muted)
			}
		}
		peer.OnRemoteTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
			if track.Kind() != pionwebrtc.RTPCodecTypeAudio {
				return
			}
			// Reject unsupported codecs before activating the decode pump.
			codec := track.Codec()
			info := audiocodec.CodecInfo{
				MimeType:  codec.MimeType,
				ClockRate: codec.ClockRate,
				Channels:  codec.Channels,
			}
			if err := audiocodec.ValidateCodec(info); err != nil {
				f.logger.Error("sessionrtc: rejecting unsupported codec",
					"source", src.ID, "codec", codec.MimeType, "err", err)
				f.obs.FrameDropped(f.sessionID, src.ID, "unsupported_codec")
				return
			}
			channels := int(codec.Channels)
			if channels == 0 {
				channels = 1
			}
			go f.runSource(src.ID, track, channels)
		})
		peer.OnClose(func() {
			f.obs.PeerClosed(f.sessionID, subject, "peer_closed")
			f.stopSource(src.ID)
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

// runSource is the per-source decode pump: RTP → jitter → decode → channel mixers.
func (f *sessionFwd) runSource(sourceID string, track *pionwebrtc.TrackRemote, channels int) {
	dec, err := f.factory.NewDecoder(channels)
	if err != nil {
		f.logger.Error("sessionrtc: decoder create failed", "source", sourceID, "err", err)
		return
	}
	pump := &sourcePump{
		sourceID:  sourceID,
		sessionID: f.sessionID,
		dec:       dec,
		obs:       f.obs,
		logger:    f.logger,
		fwd:       f,
		stopCh:    make(chan struct{}),
	}
	pump.jb = newJitterBuffer(func(reason string) {
		f.obs.FrameDropped(f.sessionID, sourceID, reason)
	})

	f.mu.Lock()
	if old, ok := f.sources[sourceID]; ok {
		old.stop()
	}
	f.sources[sourceID] = pump
	f.mu.Unlock()

	f.logger.Info("sessionrtc: decoding source track",
		"source", sourceID, "codec", track.Codec().MimeType, "channels", channels)

	// Reader goroutine: never blocks the mix loop; bounded push only.
	errCh := make(chan error, 1)
	go func() {
		for {
			pkt, _, rerr := track.ReadRTP()
			if rerr != nil {
				errCh <- rerr
				return
			}
			select {
			case <-pump.stopCh:
				return
			default:
			}
			if len(pkt.Payload) == 0 {
				continue
			}
			pump.jb.Push(pkt.SequenceNumber, pkt.Timestamp, pkt.Payload)
		}
	}()

	// Decode clock: drain jitter at ~20ms, PLC on gaps, fan-out PCM to hubs.
	ticker := time.NewTicker(5 * time.Millisecond) // poll faster than frame rate
	defer ticker.Stop()
	pcm := make([]int16, frameSamples)
	// active channel targets refreshed slowly from mix table.
	var active []string
	var lastRefresh time.Time
	refresh := func() {
		var next []string
		for _, chID := range f.allChannelIDs() {
			h := f.hub(chID)
			if h == nil {
				continue
			}
			// Source contributes to any channel that has a mix entry (muted still
			// drains into mixer so unmute is instant; mixer honors mute).
			if h.mix.GetInput(sourceID) != nil {
				next = append(next, chID)
				continue
			}
			// Also pick up newly assigned channels via mix store (throttled).
			if f.stores.Mix != nil {
				entries, err := f.stores.Mix.GetMix(context.Background(), chID)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if e.SourceID == sourceID {
						h.ensureInput(sourceID, e.Level, e.Muted)
						next = append(next, chID)
						break
					}
				}
			}
		}
		active = next
		lastRefresh = time.Now()
	}
	refresh()

	for {
		select {
		case <-pump.stopCh:
			_ = dec.Close()
			return
		case err := <-errCh:
			f.logger.Debug("sessionrtc: source track ended", "source", sourceID, "err", err)
			_ = dec.Close()
			return
		case <-ticker.C:
			if time.Since(lastRefresh) > 500*time.Millisecond {
				refresh()
			}
			// Drain all currently ready packets without blocking.
			for {
				payload, _, ok, lost := pump.jb.PopReady()
				if lost {
					// PLC for the missing frame.
					ns, _ := dec.DecodePLC(pcm)
					if ns > 0 {
						f.fanoutPCM(sourceID, active, pcm[:ns])
					}
					continue
				}
				if !ok {
					break
				}
				ns, derr := dec.Decode(payload, pcm)
				if derr != nil || ns == 0 {
					f.obs.FrameDropped(f.sessionID, sourceID, "decode_error")
					continue
				}
				f.obs.SourceFrame(f.sessionID, sourceID, ns)
				f.fanoutPCM(sourceID, active, pcm[:ns])
			}
		}
	}
}

func (f *sessionFwd) fanoutPCM(sourceID string, channelIDs []string, pcm []int16) {
	for _, chID := range channelIDs {
		if h := f.hub(chID); h != nil {
			h.writePCM(sourceID, pcm)
		}
	}
}

func (f *sessionFwd) stopSource(sourceID string) {
	f.mu.Lock()
	p, ok := f.sources[sourceID]
	if ok {
		delete(f.sources, sourceID)
	}
	f.mu.Unlock()
	if ok {
		p.stop()
	}
}

func (p *sourcePump) stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		if p.dec != nil {
			_ = p.dec.Close()
		}
	})
}

// ensureSource creates (or reuses) the Source this peer publishes as.
func (f *sessionFwd) ensureSource(ctx context.Context, peerID string, opts BridgeOpts) (*crosstalk.Source, error) {
	origin := crosstalk.SourceOrigin(opts.Role)
	switch origin {
	case crosstalk.OriginABC, crosstalk.OriginTranslator, crosstalk.OriginAdmin:
	default:
		origin = crosstalk.OriginTranslator
	}

	identity := opts.Identity
	if identity == "" {
		identity = peerID
	}
	label := opts.Label
	if label == "" {
		label = fmt.Sprintf("%s %s", opts.Role, identity)
	}

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

func (f *sessionFwd) allChannelIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.channels))
	for id := range f.channels {
		out = append(out, id)
	}
	return out
}

// ApplyMix updates live mixer levels/mutes for a channel without reconnect.
// Safe to call from API handlers after persisting mix state.
func (m *Manager) ApplyMix(sessionID, channelID string, entries []crosstalk.MixEntry) {
	m.mu.Lock()
	f := m.sessions[sessionID]
	m.mu.Unlock()
	if f == nil {
		return
	}
	h := f.hub(channelID)
	if h == nil {
		return
	}
	for _, e := range entries {
		h.setMix(e.SourceID, e.Level, e.Muted)
	}
}
