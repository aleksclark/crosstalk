# Step 05: WebRTC Foundation & Debug Logging

## Goal

Establish the WebRTC layer with Pion, but this time with verbose debug logging and status tracking as a first-class concern. Every ICE state change, every SDP exchange, every track event — logged and exposed via API.

## Design Principle: WebRTC Debug by Default

> "All WebRTC implementations should have verbose logging + status tracking by default, logging & displaying as much information as possible (except actual raw audio blobs/binary)"

This means:
- Every ICE candidate (local + remote) is logged with full details
- Every ICE state transition is logged and stored
- SDP offers/answers are logged (full text at debug level, summary at info level)
- Track additions/removals are logged
- DTLS state changes are logged
- Data channel open/close/error events are logged
- All of the above is available via REST API per-connection
- WebSocket signaling messages are logged bidirectionally

## Tasks

### 5.1 WebRTC debug event system
- [ ] `webrtc/events.go` — event types for all WebRTC lifecycle events
- [ ] Ring buffer per peer connection (last 200 events)
- [ ] Events include: timestamp, peer_id, event_type, detail (JSON)
- [ ] Event types: `ice_candidate_local`, `ice_candidate_remote`, `ice_state_change`, `dtls_state_change`, `signaling_state_change`, `sdp_offer`, `sdp_answer`, `track_added`, `track_removed`, `data_channel_open`, `data_channel_close`, `data_channel_error`, `connection_closed`

### 5.2 PeerManager with instrumentation
- [ ] Reuse v2's `PeerManager` pattern (ICE config, UDP mux)
- [ ] Wrap every Pion callback with event emission
- [ ] `GET /api/debug/peers` — list all active peers with connection state
- [ ] `GET /api/debug/peers/{id}/events` — event history for a specific peer
- [ ] `GET /api/debug/peers/{id}/stats` — current ICE/DTLS/transport stats

### 5.3 WebSocket signaling with logging
- [ ] Reuse v2's signaling pattern (offer/answer/ice over WebSocket)
- [ ] Log every signaling message (direction, type, peer_id)
- [ ] Add message sequence numbers for correlation
- [ ] SDP logging: log codec negotiation result, track count, ICE candidates in SDP

### 5.4 Control data channel (Protobuf)
- [ ] New protobuf schema for v3 control messages (see below)
- [ ] Bidirectional structured logging over control channel
- [ ] Server-initiated commands: restart, reconfigure, update-mix
- [ ] Client status reporting: connection quality, audio levels

### 5.5 ICE configuration
- [ ] STUN server config (default: Google STUN)
- [ ] TURN relay config (optional, for restrictive networks)
- [ ] UDP mux for single-port deployments (Fly.io pattern from v2)
- [ ] NAT1To1 IP configuration for known-public-IP deployments
- [ ] ICE lite mode option for server-side

## New Control Protocol (Protobuf)

```protobuf
message ControlMessage {
  oneof payload {
    // Client → Server
    Hello hello = 1;
    SourceStatus source_status = 2;

    // Server → Client
    Welcome welcome = 10;
    MixUpdate mix_update = 11;
    RestartCommand restart = 12;
    SessionAssignment session_assignment = 13;

    // Bidirectional
    LogEntry log_entry = 20;
    PingPong ping = 21;
  }
}

message Hello {
  string client_type = 1;  // "abc", "translator", "admin"
  string client_name = 2;
  repeated AudioCapability capabilities = 3;
}

message Welcome {
  string peer_id = 1;
  string server_version = 2;
  string assigned_session_id = 3;  // pre-assigned for ABCs
}

message MixUpdate {
  string channel_id = 1;
  string source_id = 2;
  bool muted = 3;
  float level = 4;
}

message RestartCommand {
  string reason = 1;
}

message SessionAssignment {
  string session_id = 1;
  string role = 2;  // what this client is in the session
}

message SourceStatus {
  string source_name = 1;
  bool active = 2;
  float peak_level = 3;  // 0.0-1.0
}

message LogEntry {
  int64 timestamp = 1;
  string severity = 2;
  string source = 3;
  string message = 4;
  map<string, string> fields = 5;
}

message PingPong {
  int64 sent_at = 1;
}
```

## Debug API Endpoints

```
GET  /api/debug/peers                    # list all peers with state
GET  /api/debug/peers/{id}               # peer detail (ICE state, tracks, channels)
GET  /api/debug/peers/{id}/events        # event ring buffer
GET  /api/debug/peers/{id}/sdp           # last offer + answer
GET  /api/debug/sessions/{id}/topology   # session graph (peers, tracks, forwarding)
```

## Acceptance Criteria

- [ ] WebSocket signaling connects and completes SDP exchange (test with Pion test client)
- [ ] Every ICE state transition is logged and visible via `/api/debug/peers/{id}/events`
- [ ] Control data channel opens and exchanges Hello/Welcome
- [ ] SDP content is logged (codec lines, track count) at info level
- [ ] `GET /api/debug/peers` returns list of connected peers with their ICE state
- [ ] PingPong round-trip measures latency and logs it

## Reusable from v2

- `server/pion/peer.go` — PeerManager, CreatePeerConnection, ICE config, UDP mux setup
- `server/pion/control.go` — control channel pattern (data channel + protobuf)
- `server/ws/signaling.go` — WebSocket upgrade, SDP/ICE message loop
- `proto/crosstalk/v1/control.proto` — message envelope pattern (adapt messages)
- `cli/pion/connection.go` — client-side WebSocket + PeerConnection lifecycle
