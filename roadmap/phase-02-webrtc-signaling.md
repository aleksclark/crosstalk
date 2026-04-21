# Phase 2: WebRTC Signaling

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 1 (server must serve HTTP + auth)

WebSocket signaling endpoint and Pion peer connection lifecycle. After this phase, clients can establish WebRTC connections to the server.

## Tasks

### 2.1 WebSocket Signaling Endpoint
- [ ] Implement `/ws/signaling` WebSocket endpoint in `ws/` package
- [ ] Auth: validate token from query param on upgrade
- [ ] JSON message protocol: `{type: "offer"|"answer"|"ice", ...}`
- [ ] Handle SDP offer from client → create Pion PeerConnection → return SDP answer
- [ ] Handle bidirectional ICE candidate trickle

**Test**: `server/ws/signaling_test.go` — connect with valid token, send offer, receive answer. Connect without token → 401 on upgrade.

### 2.2 Pion Peer Connection Management
- [ ] `pion/` package: create PeerConnection with configured STUN/TURN servers
- [ ] Track ICE connection state (checking → connected → disconnected → failed)
- [ ] Peer connection registry: track active connections, clean up on disconnect
- [ ] Support adding/removing media tracks dynamically

**Test**: `server/pion/peer_test.go` — create two Pion peers in-process, complete offer/answer exchange, verify ICE state reaches `connected`, send a data channel message, verify receipt.

### 2.3 Data Channel Setup
- [ ] After peer connection, server creates `control` data channel (reliable, ordered)
- [ ] Wire data channel to message handler (Phase 3 will add Protobuf parsing)
- [ ] For now: echo any received message back (proves the channel works)

**Test**: `server/pion/datachannel_test.go` — two in-process peers, open data channel, send "ping", receive "ping" echo.

### 2.4 Wire into Server
- [ ] `cmd/ct-server/main.go` registers `/ws/signaling` with the HTTP server
- [ ] WebSocket handler uses the auth middleware for token validation
- [ ] ICE config read from server config (`webrtc.stun_servers`, `webrtc.turn`)

**Test**: Start server, connect via WebSocket with valid token, send SDP offer, receive SDP answer, verify peer connection state.

## Exit Criteria

A Go test client can:
1. Authenticate via REST token
2. Open WebSocket to `/ws/signaling`
3. Exchange SDP offer/answer
4. Complete ICE negotiation (peer connection reaches `connected`)
5. Open a data channel and send/receive a message

All verified in `go test` using Pion's in-process API (no network required).

## Spec Updates

- 2.2 WebRTC Signaling → 3
