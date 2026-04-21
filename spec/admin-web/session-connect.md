# Session Connect View

[← Back to Index](../index.md) · [Admin Web Overview](overview.md)

---

## Overview

The session connect view (`/sessions/:id/connect`) turns the browser into a full WebRTC client within a session. It provides audio I/O plus debugging tools critical for development and live monitoring.

## Layout

```
┌──────────────────────────────────────────────────────────┐
│  Session: "Translation #5"    Role: translator    [End]  │
├──────────────────────────┬───────────────────────────────┤
│                          │                               │
│   Audio Channels         │   WebRTC Debug                │
│                          │                               │
│   ┌─ studio:input ────┐  │   ICE State: connected        │
│   │ ▓▓▓▓▓▓░░░░  -3dB │  │   ICE Candidates: 4 local,    │
│   │ [volume slider]   │  │     2 remote                  │
│   └───────────────────┘  │   STUN: stun.l.google.com     │
│                          │   Selected pair: ...           │
│   ┌─ audience:feed ───┐  │   Tracks: 2 audio (send/recv) │
│   │ ▓▓▓▓░░░░░░  -8dB │  │   Data channels: 1 (control)  │
│   │ [volume slider]   │  │   Bytes sent: 1.2MB           │
│   └───────────────────┘  │   Bytes recv: 890KB           │
│                          │   Packet loss: 0.1%           │
│   Mic: [device select]  │   Jitter: 12ms                │
│   ▓▓▓▓▓▓▓░░░  -1dB     │   RTT: 23ms                   │
│                          │                               │
├──────────────────────────┴───────────────────────────────┤
│  Session Logs                                            │
│                                                          │
│  10:30:01 [server]  translator connected                 │
│  10:30:01 [server]  binding translator:mic → studio      │
│  10:30:02 [transl]  channel active: mic → track_abc      │
│  10:30:02 [studio]  channel active: output → track_abc   │
│  10:30:15 [server]  recording started: translator-mic    │
│  10:30:45 [transl]  status: bytes_transferred=128000     │
│                                                          │
│  [auto-scroll] [filter: severity ▾] [clear]              │
└──────────────────────────────────────────────────────────┘
```

## Audio Panel (Left)

### Incoming Channels

For each audio channel the browser receives:
- **Channel name** with role prefix
- **VU meter** — real-time level visualization (Web Audio API `AnalyserNode`)
- **Volume control** — per-channel gain slider
- Level shown in dBFS

### Microphone (Outgoing)

- Browser mic device selector (`getUserMedia` with device enumeration)
- VU meter for mic level
- Mute toggle

## WebRTC Debug Panel (Right)

Surfaces the browser's native WebRTC stats via `RTCPeerConnection.getStats()`:

- **ICE state** — checking, connected, disconnected, failed
- **ICE candidates** — local and remote candidate pairs, selected pair
- **STUN/TURN** — which servers are in use
- **Track info** — media tracks with direction, codec, bitrate
- **Data channel** — state, messages sent/received
- **Quality metrics** — packet loss, jitter, RTT, bytes transferred
- Auto-refreshing (every 1-2 seconds)

## Session Logs Panel (Bottom)

All signaling and status messages from the session, streamed over the WebRTC data channel:

- Messages from **all clients** in the session (not just this browser)
- Server-side session events
- Formatted with timestamp, source, and message
- Severity-based filtering (debug, info, warn, error)
- Auto-scroll with manual override
- Clear button
