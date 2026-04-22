# Phase 5: Session Orchestration + Audio Forwarding

[← Roadmap](index.md)

**Status**: `PARTIAL — 3/6 tasks done (5.2, 5.4, 5.6), 2 partial (5.1, 5.3), 1 not started (5.5)`  
**Depends on**: Phase 3 (control channel) + Phase 4 (CLI client can connect)

The heart of CrossTalk — create sessions from templates, assign clients to roles, resolve channel bindings, and forward audio tracks between peer connections.

## Tasks

### 5.1 Session Lifecycle
- [x] `POST /api/sessions` creates session from template, status `waiting` — `180d290`
  > Implemented in `server/http/handler.go:625–661`. Sets `crosstalk.SessionWaiting`. Tested by `TestIntegration_FullCRUD` and `TestCreateSession` (handler unit test).
- [ ] When first `JoinSession` arrives and a binding activates → status `active`
  > **GAP**: `SessionActive` constant is defined (`server/domain.go:72`) but **never written** anywhere in the codebase. `Orchestrator.JoinSession()` evaluates bindings and sends `BindChannel` but never updates the persisted session status to `"active"`. No `UpdateSessionStatus` method exists on `SessionService`. Sessions remain `"waiting"` until ended.
- [x] `DELETE /api/sessions/:id` → sends `SessionEnded` to all clients, status `ended` — `180d290` (partial)
  > **GAP**: The REST handler (`server/http/handler.go:677–688`) only calls `SessionService.EndSession()` which updates the DB status to `"ended"`. It does **not** call `Orchestrator.EndSession()`, so connected WebRTC clients are never notified via `SessionEnded`, bindings are not deactivated, and track forwarding is not stopped. The `Orchestrator.EndSession()` method exists and correctly handles all of this (`server/pion/orchestrator.go:191–243`), but the HTTP handler has no reference to the orchestrator. The `Handler` struct has no `Orchestrator` field — the orchestrator only lives in the WebSocket/signaling layer (`server/ws/signaling.go:37`).
- [x] Partial sessions: missing roles don't block — only available bindings activate — `180d290`
  > Implemented in `Orchestrator.evaluateBindings()` → `ResolveBindings()`. Tested by `TestOrchestrator_PartialBindings` (only record binding activates when interviewer joins without candidate) and `TestResolveBindings_PartialRoles` (domain unit test).

**Test**: Create session via REST → status is `waiting`. Client joins → status is `active`. End session via REST → status is `ended`, client receives `SessionEnded` event.
> **Test gap**: `TestIntegration_FullCRUD` validates `waiting → ended` via REST but never tests the `waiting → active` transition (which isn't implemented). No integration test verifies that `DELETE /api/sessions/:id` sends `SessionEnded` to connected WebRTC clients — the `TestOrchestrator_EndSession` unit test calls `Orchestrator.EndSession()` directly, bypassing the REST→orchestrator gap.

### 5.2 Binding Resolution
- [x] Given connected clients and template mappings, compute which bindings can activate — `2a56d5f`
- [x] Only activate bindings where both source and sink roles are filled — `2a56d5f`
- [x] For `→ record` bindings, activate when the source role is filled — `2a56d5f`
- [x] For `→ broadcast` bindings, activate when the source role is filled — `2a56d5f`
- [x] When a new client joins or disconnects, re-evaluate all bindings — `180d290`

> All implemented in `server/domain.go:199–238` (`ResolveBindings`) and `server/pion/orchestrator.go:257–290` (`evaluateBindings`). Re-evaluation on join/leave triggered from `JoinSession` and `LeaveSession`.

**Test**: `server/domain_test.go` (pure domain logic, no deps) — given a template and set of connected roles, `ResolveBindings()` returns the correct active binding list. Test partial states, role add/remove.
> **Complete**: `TestResolveBindings_BothRolesConnected`, `_PartialRoles`, `_RecordAndBroadcast`, `_NoRolesConnected`, `_MultipleBindings` — all cover the acceptance criteria thoroughly. `TestOrchestrator_LeaveSession_DeactivatesBindings` validates re-evaluation on disconnect.

### 5.3 BindChannel Commands
- [x] When a binding activates, server sends `BindChannel` to both source and sink clients — `180d290`
  > Implemented in `server/pion/orchestrator.go:310–356` (`activateBinding`). Sends `BindChannel` with correct `Direction` (SOURCE/SINK), `LocalName`, `TrackID`, and `ChannelID`.
- [x] Source client receives: "send your 'mic' source on track X" — `180d290`
  > Tested in `TestOrchestrator_JoinSession_Success`: client receives `BindChannel{direction: SOURCE, local_name: "mic"}` for a record binding.
- [ ] Sink client receives: "play track X on your 'output' sink"
  > **GAP**: No test validates that a sink client receives `BindChannel{direction: SINK}`. `TestOrchestrator_JoinSession_Success` only tests with a single "interviewer" peer — it validates the record binding (SOURCE) but never adds a second peer to verify the role→role sink binding. `TestOrchestrator_EndSession` joins two peers but only checks for `SessionEnded`, not for `BindChannel{SINK}` on client B.
- [x] Track allocation: server creates Pion tracks for each active binding — `180d290`
  > Implemented in `ForwardTrack()` (`server/pion/forward.go:23–108`) which creates `TrackLocalStaticRTP` for each binding.

**Test**: Two in-process Pion clients join a session. Client A receives `BindChannel{direction: SOURCE}`, client B receives `BindChannel{direction: SINK}`.
> **Partial**: `TestOrchestrator_JoinSession_Success` validates SOURCE binding. No unit test validates SINK binding on a second peer. `TestIntegration_SessionWithAudioForwarding` sets up both roles but doesn't assert BindChannel messages — it only checks for SessionEvent and track receipt.

### 5.4 Audio Track Forwarding (SFU)
- [x] Server receives audio track from source client's PeerConnection — `180d290`
- [x] Server forwards RTP packets as-is to sink client's PeerConnection — `180d290`
- [x] No transcoding, no mixing — pure forwarding — `180d290`
- [x] Handle track lifecycle: remove forwarding when binding deactivates — `180d290`

> All implemented in `server/pion/forward.go:23–108` (ForwardTrack) and `server/pion/orchestrator.go:360–386` (deactivateBinding removes track + calls `stopForward()`).

**Test**: Two in-process Pion clients. Client A sends Opus audio track. Client B receives audio track. Compare sent RTP packets to received RTP packets — payload bytes must match (proving pure forwarding).
> **Complete**: `TestForwardTrack_RTPPayloadMatch` (`server/pion/forward_test.go`) is a thorough SFU proof test — two in-process Pion clients, 10 RTP packets with known payloads, byte-for-byte comparison on the sink side. All tests pass. `TestOrchestrator_LeaveSession_DeactivatesBindings` validates binding deactivation sends `UnbindChannel`.

### 5.5 CLI Client: PipeWire ↔ WebRTC Bridge
- [ ] On `BindChannel{direction: SOURCE}`: read PCM from PipeWire node, encode Opus, send as Pion track
- [ ] On `BindChannel{direction: SINK}`: receive Pion track, decode Opus, write PCM to PipeWire node
- [ ] On `UnbindChannel`: tear down the bridge
- [ ] Report `ChannelStatus{ACTIVE}` when bridge is running

> **NOT IMPLEMENTED**: The CLI client (`cli/`) has no `BindChannel` or `UnbindChannel` handling. The control protocol only supports `hello`, `client_status`, and `welcome` message types (`cli/pion/control.go`). PipeWire integration is limited to `Discover()` (enumerating sources/sinks via `pw-cli`); there is no audio capture, playback, or bridging code. No Opus encoder/decoder exists in the CLI — only codec capability strings for negotiation. `ChannelStatus` reporting is not implemented.

**Test**: Start server + CLI client with PipeWire loopback. Create session, join client. Play a 1kHz tone into loopback source. Verify `ChannelStatus{ACTIVE}` and `bytes_transferred > 0`.
> **No test exists** — the feature is not implemented.

### 5.6 Role Cardinality Enforcement
- [x] Single-client role: reject second client with `SESSION_ROLE_REJECTED` — `180d290`
  > Implemented in `Orchestrator.JoinSession()` (`server/pion/orchestrator.go:115–121`): checks `foundRole.MultiClient` and rejects if role is already occupied.
- [x] Multi-client role: accept multiple clients, each gets sink bindings — `180d290`
  > Implemented: multi-client roles skip the cardinality check. Tested in `TestOrchestrator_JoinSession_MultiClient` — two observers join the same session successfully.
- [x] Multi-client roles cannot be mapping sources (validated at template creation, but also enforced at runtime) — `2a56d5f`
  > Template validation enforced in `SessionTemplate.Validate()` (`server/domain.go`). Tested by `TestSessionTemplate_Validate_MultiClientSourceRejected`. Runtime enforcement relies on `ResolveBindings` simply not matching multi-client roles as sources (since they're validated at template creation time).

**Test**: Create template with single-client role. Client A joins → success. Client B tries same role → receives `ROLE_REJECTED`. Create template with multi-client role, two clients join → both succeed.
> **Complete**: Unit tests: `TestOrchestrator_JoinSession_RoleRejected_SingleClient` (rejects with "already occupied"), `TestOrchestrator_JoinSession_MultiClient` (both succeed). Integration test: `TestIntegration_RoleCardinality` — full E2E with `SESSION_ROLE_REJECTED` event containing "single-client" message.

## Exit Criteria

> Two Pion test clients join a session in different roles. Audio sent by client A arrives at client B per the template mapping. Verified by comparing sent/received RTP payloads.

**PARTIALLY MET**: `TestForwardTrack_RTPPayloadMatch` proves byte-for-byte SFU forwarding between two in-process Pion peers, but it tests `ForwardTrack` directly rather than going through `Orchestrator.JoinSession()` → `evaluateBindings()` → `activateBinding()`. `TestIntegration_SessionWithAudioForwarding` tests the full path but has a **soft failure** — it logs "timed out waiting for track on Client B" and falls back to checking orchestrator state via REST, which cannot confirm actual audio receipt. The RTP payload comparison only happens in the isolated `ForwardTrack` test.

> CLI client with PipeWire loopback: audio played into loopback source → sent via WebRTC → received by second client.

**NOT MET**: CLI client has no BindChannel handling, no PipeWire bridge, no audio pipeline. This exit criterion is blocked on task 5.5.

## Spec Updates

- 2.3 Session Orchestration → 5
- 3.3 Channel Lifecycle → 4
- 5.2 Session Templates → 4
- 5.3 Sessions → 4

## Summary of Gaps

| Gap | Severity | Description |
|-----|----------|-------------|
| No `waiting → active` transition | Medium | `SessionActive` defined but never set. Sessions stay `"waiting"` until ended. |
| REST DELETE doesn't notify clients | High | `handleDeleteSession` only updates DB — doesn't call `Orchestrator.EndSession()`. Connected clients never receive `SessionEnded`, forwarding continues. |
| No SINK BindChannel test | Low | No test verifies that the sink peer receives `BindChannel{direction: SINK}`. |
| Integration SFU test is soft | Medium | `TestIntegration_SessionWithAudioForwarding` accepts timeout as non-failure; doesn't prove end-to-end audio receipt through orchestrator. |
| CLI BindChannel/UnbindChannel | High | CLI has no channel lifecycle handling — entire task 5.5 is unimplemented. |
| CLI PipeWire bridge | High | No audio capture/playback or Opus encode/decode in CLI. |
| CLI ChannelStatus reporting | Medium | No `ChannelStatus` message support in CLI. |
