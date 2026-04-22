# Phase 2: WebRTC Signaling

[← Roadmap](index.md)

**Status**: `COMPLETE — all tasks implemented, minor test gaps noted`  
**Depends on**: Phase 1 (server must serve HTTP + auth)

WebSocket signaling endpoint and Pion peer connection lifecycle. After this phase, clients can establish WebRTC connections to the server.

## Tasks

### 2.1 WebSocket Signaling Endpoint
- [x] Implement `/ws/signaling` WebSocket endpoint in `ws/` package `ad928ba`
- [x] Auth: validate token from query param on upgrade `ad928ba`
- [x] JSON message protocol: `{type: "offer"|"answer"|"ice", ...}` `ad928ba`
- [x] Handle SDP offer from client → create Pion PeerConnection → return SDP answer `ad928ba`
- [x] Handle bidirectional ICE candidate trickle `ad928ba`

**Test**: `server/ws/signaling_test.go` — connect with valid token, send offer, receive answer. Connect without token → 401 on upgrade.

> **Review**: All 4 tests pass (`TestSignaling_ValidToken_OfferAnswer`, `TestSignaling_InvalidToken_Rejected`, `TestSignaling_MissingToken_Rejected`, `TestSignaling_ICETrickle`). Tests validate the acceptance criteria: valid offer→answer exchange, 401 on invalid/missing token, and ICE trickle round-trip. No gaps.

### 2.2 Pion Peer Connection Management
- [x] `pion/` package: create PeerConnection with configured STUN/TURN servers `a9b6db9`
- [x] Track ICE connection state (checking → connected → disconnected → failed) `a9b6db9`
- [x] Peer connection registry: track active connections, clean up on disconnect `a9b6db9`
- [x] Support adding/removing media tracks dynamically `a9b6db9`

**Test**: `server/pion/peer_test.go` — create two Pion peers in-process, complete offer/answer exchange, verify ICE state reaches `connected`, send a data channel message, verify receipt.

> **Review**: Implementation exists and tests pass. Notes on each sub-task:
>
> - **STUN/TURN config**: `NewPeerManager` correctly reads `WebRTCConfig.STUNServers` and `WebRTCConfig.TURN` to build `[]webrtc.ICEServer`. Validated by `WebRTCConfig` struct in `config.go` matching `config.schema.json`.
> - **ICE state tracking**: `PeerConn.OnICEConnectionStateChange` is a passthrough wrapper to Pion's callback. **Gap**: No server-side code registers this callback to log or react to state changes (disconnected/failed). The spec says "Connection state tracking — monitors ICE connection state, fires events on connect/disconnect/fail" but the server never self-registers a handler; it only exposes the API for external callers.
> - **Peer registry**: `PeerManager.peers` map with `CreatePeerConnection`, `RemovePeer`, `Count`. Tested in `TestPeerManager_Registry`.
> - **Dynamic tracks**: No PeerConn-level AddTrack/RemoveTrack API. Track manipulation is done directly on `pc` (raw Pion PeerConnection) inside `forward.go`. Functionally sufficient for the SFU use case.
> - **Test gap**: `TestPeerConnection_OfferAnswer` only checks that ICE reaches `connected`; no test validates the full state machine (checking → connected → disconnected → failed). The roadmap test spec ("verify ICE state reaches connected") is met, but the spec's requirement to "fire events on disconnect/fail" is not tested.

### 2.3 Data Channel Setup
- [x] After peer connection, server creates `control` data channel (reliable, ordered) `a9b6db9`
- [x] Wire data channel to message handler (Phase 3 will add Protobuf parsing) `a9b6db9`
- [x] For now: echo any received message back (proves the channel works) `a9b6db9`

**Test**: `server/pion/datachannel_test.go` — two in-process peers, open data channel, send "ping", receive "ping" echo.

> **Review**: Implementation is complete. The control data channel is created in `datachannel.go:createControlChannel` with `Ordered: true` and a default echo handler. The echo handler is later replaced by the protobuf `ControlHandler` via `Install()` (Phase 3 work, but wired here).
>
> - **Test file location**: The roadmap specifies `datachannel_test.go` but the test is actually in `peer_test.go:TestPeerConnection_DataChannelEcho`. It validates the acceptance criteria correctly: two in-process peers, client receives the server-created "control" channel, sends "ping", receives "ping" echo.
> - **Minor gap**: No dedicated `datachannel_test.go` file exists — the test lives in `peer_test.go`. Functionally equivalent.

### 2.4 Wire into Server
- [x] `cmd/ct-server/main.go` registers `/ws/signaling` with the HTTP server `74dfd03`
- [x] WebSocket handler uses the auth middleware for token validation `74dfd03`
- [x] ICE config read from server config (`webrtc.stun_servers`, `webrtc.turn`) `74dfd03`

**Test**: Start server, connect via WebSocket with valid token, send SDP offer, receive SDP answer, verify peer connection state.

> **Review**: The route is mounted at `handler.go:65` via `r.Handle("/ws/signaling", h.SignalingHandler)`. Auth is handled inside `SignalingHandler.ServeHTTP` (query param token validation) rather than through the shared auth middleware — this is by design since WebSocket upgrade doesn't use Bearer headers.
>
> - Integration tests in `main_test.go`: `TestServerIntegration_WebSocketSignaling` (full SDP exchange over a real HTTP server), `TestServerIntegration_WebSocketSignaling_NoToken` (401), `TestServerIntegration_WebSocketSignaling_InvalidToken` (401). All pass.
> - **Test gap**: The integration test validates SDP offer→answer and the WebSocket staying open, but does **not** verify peer connection state reaches `connected` or test data channel messaging end-to-end through the server. The roadmap says "verify peer connection state" — this is only partially met (SDP exchange proves the signaling works, but ICE connection completion is not asserted).

## Exit Criteria

A Go test client can:
1. ✅ Authenticate via REST token — `testServer` seeds an API token used in all integration tests
2. ✅ Open WebSocket to `/ws/signaling` — `TestServerIntegration_WebSocketSignaling`
3. ✅ Exchange SDP offer/answer — same test, plus `TestSignaling_ValidToken_OfferAnswer`
4. ⚠️ Complete ICE negotiation (peer connection reaches `connected`) — verified in unit tests (`TestPeerConnection_OfferAnswer`) but **not** in the server integration test
5. ⚠️ Open a data channel and send/receive a message — verified in unit test (`TestPeerConnection_DataChannelEcho`) but **not** in the server integration test

> **Exit criteria assessment**: All 5 criteria are functionally met at the unit test level. Criteria 4 and 5 lack integration-level validation (through the full server stack). The unit tests use in-process Pion peers which proves the functionality works, but the integration test stops at SDP exchange without completing ICE or opening a data channel.

All verified in `go test` using Pion's in-process API (no network required).

## Spec Updates

- 2.2 WebRTC Signaling → 3

## Reviewer Notes

**All 22 tests in `server/ws/` and `server/pion/` pass.** Implementation is solid and exceeds Phase 2 scope (protobuf control handler, session orchestrator, SFU forwarding, and recording are already implemented in later phases built on this foundation).

### Gaps to address (none blocking):

1. **ICE state lifecycle handling**: The spec requires "Connection state tracking — monitors ICE connection state, fires events on connect/disconnect/fail" but the server never registers its own `OnICEConnectionStateChange` callback. No server-side reaction to `disconnected` or `failed` states (e.g., peer cleanup, reconnection, session notification). The callback wrapper exists but is unused by the server itself.

2. **Integration test depth**: `TestServerIntegration_WebSocketSignaling` validates SDP exchange but doesn't wait for ICE `connected` state or test data channel messaging through the server. Adding this would strengthen confidence that the full stack works end-to-end.

3. **Test file naming**: Roadmap specifies `server/pion/datachannel_test.go` but the echo test is in `server/pion/peer_test.go`. Minor organizational discrepancy.
