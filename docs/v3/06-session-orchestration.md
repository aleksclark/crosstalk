# Step 06: Session Orchestration & Channel Mixing

## Goal

Implement the session orchestrator that manages live audio routing. The key new feature vs v2: channels have a **mixer** that combines multiple sources with per-source mute/level controls, rather than simple 1:1 forwarding.

## How Mixing Works

In v2, audio forwarding was 1:1: one source track forwarded to one sink track. In v3:

1. A **channel** (e.g. "broadcast") has 0 or more **sources** contributing audio
2. Each source has a mute toggle + level slider (0.0 - 2.0) per channel
3. The channel output is the mixed result of all unmuted sources at their configured levels
4. The mix is computed server-side (not client-side) to ensure consistent output

### Server-Side Mixing

Since Pion works with raw RTP packets (Opus), mixing requires:
1. Decode Opus → PCM for each source
2. Mix PCM samples (scale by level, sum, clip)
3. Encode mixed PCM → Opus
4. Send mixed Opus RTP to all sinks (broadcast listeners, translator feed)

**Alternative**: Use a silent track + multiple tracks approach where each listener gets all unmuted source tracks separately and the client mixes. This avoids transcoding but increases bandwidth.

**Decision**: Server-side mixing is correct for broadcast (one stream to many listeners). For the feed channel going to translators, server-side mixing is also correct because the translator just needs a single "what's happening in the room" stream.

### Implementation: Opus Decode/Mix/Encode

- Use `hraban/opus` (Go bindings for libopus) for decode/encode
- Mix buffer: accumulate PCM samples from all active sources in a frame-aligned ring buffer
- Mix interval: one Opus frame = 20ms at 48kHz = 960 samples
- Output: single mixed Opus stream per channel

## Tasks

### 6.1 Mixer engine
- [ ] `mixer/` package — pure audio mixing logic, no WebRTC deps
- [ ] `Mixer` struct: manages N input streams, produces 1 output stream
- [ ] Per-input: mute bool, level float64, ring buffer for PCM frames
- [ ] Mix loop: every 20ms, read one frame from each input, scale + sum, produce output frame
- [ ] Thread-safe: inputs can be added/removed while mixer runs
- [ ] Unit test: mix 2 known PCM signals, verify output is sum at correct levels

### 6.2 Opus codec integration
- [ ] `mixer/opus.go` — Opus encoder/decoder wrappers
- [ ] Decode incoming Opus RTP → PCM samples → feed into mixer input
- [ ] Encode mixer output PCM → Opus RTP → forward to sinks
- [ ] Handle packet loss gracefully (PLC in Opus decoder)

### 6.3 Session orchestrator
- [ ] `orchestrator/` package — manages live session state
- [ ] When source connects: create mixer input, load saved mix state from DB
- [ ] When source disconnects: mark input inactive (don't remove — preserves mix state for reconnect)
- [ ] When mix changes (API or control channel): update mixer input params in real-time
- [ ] Track mapping: source → session → channels where source is included

### 6.4 Channel output routing
- [ ] Each channel has exactly one mixer producing one output stream
- [ ] Output stream forwarded to all connected sinks:
  - Broadcast channel → all broadcast listeners
  - Feed channel → all connected translators
- [ ] ForwardTrack from v2 reused for output → sink forwarding (no mixing there, just fan-out)

### 6.5 Source lifecycle
- [ ] Source connects (ABC or translator WebRTC peer sends audio track)
- [ ] Server identifies source by name (from Hello message)
- [ ] Lookup or create source record in DB
- [ ] Load mix state for all channels this source participates in
- [ ] Wire source's decoded audio into each channel's mixer
- [ ] On disconnect: mark source inactive, mixer input goes silent (contributes zeros)

### 6.6 Mix control API
- [ ] `PUT /api/sessions/{id}/channels/{ch_id}/mix` — update mute/level for a source
- [ ] Real-time: API call updates mixer input immediately (no restart needed)
- [ ] Also controllable via control channel `MixUpdate` message (for low-latency UI)
- [ ] Mix changes broadcast to all connected admins/translators via control channel

## Data Flow

```
ABC-1 Audio ──→ Opus Decode ──→ Mixer Input (level=1.0) ──┐
                                                           │
ABC-2 Audio ──→ Opus Decode ──→ Mixer Input (level=0.7) ──┼──→ Mixer ──→ Opus Encode ──→ Broadcast Channel
                                                           │                              (→ all listeners)
Translator Audio ──→ Opus Decode ──→ Mixer Input (muted) ──┘

ABC-1 Audio ──→ Opus Decode ──→ Mixer Input (level=1.0) ──┐
                                                           ├──→ Mixer ──→ Opus Encode ──→ Feed Channel
ABC-2 Audio ──→ Opus Decode ──→ Mixer Input (level=1.0) ──┘                              (→ translators)
```

## Acceptance Criteria

- [ ] Two test audio sources feed into a channel → mixed output contains both signals
- [ ] Muting a source removes it from the mix (output is only the other source)
- [ ] Level adjustment scales the source's contribution proportionally
- [ ] Mix state persists: disconnect source, reconnect, same mute/level applies
- [ ] API mix update takes effect within one Opus frame (20ms)
- [ ] Broadcast listeners receive the mixed stream
- [ ] Feed listeners (translators) receive the feed channel mix

## Reusable from v2

- `server/pion/forward.go` — ForwardTrack pattern for output fan-out
- `server/pion/orchestrator.go` — LiveSession/LiveClient state management pattern
- `server/domain.go` — ResolveBindings concept (simplified: sources → channels instead of role mappings)
