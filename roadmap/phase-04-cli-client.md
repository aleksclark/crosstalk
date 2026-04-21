# Phase 4: CLI Client Core

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 3 (server must handle Hello/Welcome and JoinSession)

The CLI client authenticates, connects via WebSocket, establishes WebRTC, reports capabilities via the control channel, and discovers PipeWire sources/sinks.

## Tasks

### 4.1 Config Loading
- [ ] JSON config loading with `--config` flag and `CROSSTALK_CONFIG` env var
- [ ] Env var overrides (`CROSSTALK_SERVER`, `CROSSTALK_TOKEN`, etc.)
- [ ] Validate against `cli/config.schema.json`
- [ ] Configure `slog` JSON logger

**Test**: `cli/cmd/ct-client/config_test.go` — load config with env overrides, verify env wins.

### 4.2 REST Authentication
- [ ] HTTP client: set `Authorization: Bearer <token>` from config
- [ ] `POST /api/webrtc/token` → receive short-lived WebRTC token
- [ ] On 401/403: exit immediately with clear error

**Test**: `cli/pion/auth_test.go` — mock HTTP server returns 200 with token, verify client stores it. Mock returns 401 → client returns error.

### 4.3 WebSocket + WebRTC Connection
- [ ] Open WebSocket to `/ws/signaling?token=<webrtc_token>`
- [ ] Create Pion PeerConnection, generate SDP offer, send over WebSocket
- [ ] Receive SDP answer, apply it
- [ ] Exchange ICE candidates
- [ ] Open `control` data channel, send `Hello` with capabilities

**Test**: Start real `ct-server`, start `ct-client` with valid config, verify `GET /api/clients` returns the client with `state: READY`.

### 4.4 PipeWire Source/Sink Discovery
- [ ] `pipewire/` package: enumerate audio source/sink nodes via `pw-cli` or D-Bus
- [ ] Filter by `CROSSTALK_SOURCE_NAME` / `CROSSTALK_SINK_NAME` if set
- [ ] Report discovered sources/sinks in `Hello` message
- [ ] Watch for hotplug events, send `ClientStatus` updates

**Test**: On a system with PipeWire running, `pipewire.Discover()` returns at least one source and one sink. With `CROSSTALK_SOURCE_NAME` set, only the named source is returned.

### 4.5 Connection Resilience
- [ ] Exponential backoff reconnect on WebRTC disconnect (1s, 2s, 4s, ... cap 60s)
- [ ] Exit on REST auth failure (don't retry bad creds)
- [ ] Log all connection state transitions

**Test**: Start client against server, kill server, verify client logs reconnection attempts with increasing backoff. Restart server, verify client reconnects and re-sends `Hello`.

## Exit Criteria

1. `ct-client` starts with JSON config
2. Authenticates to server via REST token
3. Establishes WebRTC connection via WebSocket signaling
4. Sends `Hello` with PipeWire sources/sinks
5. Appears in `GET /api/clients` with capabilities
6. Reconnects after server restart

Tested with a real `ct-server` process and PipeWire loopback devices.

## Spec Updates

- 3.1 Auth & Connection → 3
- 3.2 PipeWire Integration → 3
