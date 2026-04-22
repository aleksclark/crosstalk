# Phase 5: Session Orchestration + Audio Forwarding

[← Roadmap](index.md)

**Status**: `COMPLETE — all server-side gaps addressed, CLI gaps properly deferred`  
**Depends on**: Phase 3 (control channel) + Phase 4 (CLI client can connect)

The heart of CrossTalk — create sessions from templates, assign clients to roles, resolve channel bindings, and forward audio tracks between peer connections.

## Tasks

### 5.1 Session Lifecycle
- [x] `POST /api/sessions` creates session from template, status `waiting` — `180d290`
  > Implemented in `server/http/handler.go:625–661`. Sets `crosstalk.SessionWaiting`. Tested by `TestIntegration_FullCRUD` and `TestCreateSession` (handler unit test).
- [x] When first `JoinSession` arrives and a binding activates → status `active`
  > Implemented: `UpdateSessionStatus(id, status)` added to `SessionService` interface + SQLite impl. Called from `Orchestrator.evaluateBindings()` when the first binding activates and session status is `SessionWaiting`. Tested by `TestOrchestrator_JoinSession_TransitionsToActive` (unit) and `TestIntegration_SessionActiveTransition` (integration — creates session, joins, verifies status is "active" via REST).
- [x] `DELETE /api/sessions/:id` → sends `SessionEnded` to all clients, status `ended`
  > `SessionOrchestrator` interface added to domain types. `Orchestrator` field added to HTTP `Handler` struct, wired in `cmd/ct-server/main.go`. `handleDeleteSession` calls `Orchestrator.EndSession()` before updating the DB — connected WebRTC clients receive `SessionEnded`, bindings are deactivated, and track forwarding is stopped. Tested by `TestIntegration_DeleteSessionNotifiesClients` (integration — client receives SESSION_ENDED after REST DELETE).
- [x] Partial sessions: missing roles don't block — only available bindings activate — `180d290`
  > Implemented in `Orchestrator.evaluateBindings()` → `ResolveBindings()`. Tested by `TestOrchestrator_PartialBindings` (only record binding activates when interviewer joins without candidate) and `TestResolveBindings_PartialRoles` (domain unit test).

**Test**: Create session via REST → status is `waiting`. Client joins → status is `active`. End session via REST → status is `ended`, client receives `SessionEnded` event.
> **Complete**: `TestIntegration_SessionActiveTransition` validates `waiting → active`. `TestIntegration_DeleteSessionNotifiesClients` validates `DELETE` sends `SESSION_ENDED` to connected clients. `TestIntegration_FullCRUD` validates `waiting → ended` via REST.

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
- [x] Sink client receives: "play track X on your 'output' sink"
  > Tested in `TestOrchestrator_JoinSession_SinkReceivesBindChannel`: candidate joins after interviewer, receives `BindChannel{direction: SINK, local_name: "speaker"}` for the role→role binding.
- [x] Track allocation: server creates Pion tracks for each active binding — `180d290`
  > Implemented in `ForwardTrack()` (`server/pion/forward.go:23–108`) which creates `TrackLocalStaticRTP` for each binding.
- [x] SFU renegotiation: server triggers renegotiation after adding track to sink peer
  > `PeerConn.Negotiate()` creates a server-side SDP offer and delivers it via `OnNegotiationNeeded` callback. Signaling layer registers this callback to forward the offer over WebSocket. Client responds with an answer handled by `PeerConn.HandleAnswer()`. `ForwardTrack` calls `sinkPeer.Negotiate()` after `AddTrack`. `TestIntegration_SessionWithAudioForwarding` now hard-asserts that Client B receives the forwarded track.

**Test**: Two in-process Pion clients join a session. Client A receives `BindChannel{direction: SOURCE}`, client B receives `BindChannel{direction: SINK}`.
> **Complete**: `TestOrchestrator_JoinSession_Success` validates SOURCE binding. `TestOrchestrator_JoinSession_SinkReceivesBindChannel` validates SINK binding on a second peer. `TestIntegration_SessionWithAudioForwarding` validates end-to-end track delivery through SFU with renegotiation.

### 5.4 Audio Track Forwarding (SFU)
- [x] Server receives audio track from source client's PeerConnection — `180d290`
- [x] Server forwards RTP packets as-is to sink client's PeerConnection — `180d290`
- [x] No transcoding, no mixing — pure forwarding — `180d290`
- [x] Handle track lifecycle: remove forwarding when binding deactivates — `180d290`

> All implemented in `server/pion/forward.go:23–108` (ForwardTrack) and `server/pion/orchestrator.go:360–386` (deactivateBinding removes track + calls `stopForward()`).

**Test**: Two in-process Pion clients. Client A sends Opus audio track. Client B receives audio track. Compare sent RTP packets to received RTP packets — payload bytes must match (proving pure forwarding).
> **Complete**: `TestForwardTrack_RTPPayloadMatch` (`server/pion/forward_test.go`) is a thorough SFU proof test — two in-process Pion clients, 10 RTP packets with known payloads, byte-for-byte comparison on the sink side. `TestIntegration_SessionWithAudioForwarding` validates end-to-end track delivery through the full orchestrator path with server-initiated renegotiation. `TestOrchestrator_LeaveSession_DeactivatesBindings` validates binding deactivation sends `UnbindChannel`.

### 5.5 CLI Client: PipeWire ↔ WebRTC Bridge
- [ ] On `BindChannel{direction: SOURCE}`: read PCM from PipeWire node, encode Opus, send as Pion track
- [ ] On `BindChannel{direction: SINK}`: receive Pion track, decode Opus, write PCM to PipeWire node
- [ ] On `UnbindChannel`: tear down the bridge
- [ ] Report `ChannelStatus{ACTIVE}` when bridge is running

> **DEFERRED**: The CLI client (`cli/`) has BindChannel/UnbindChannel handling and audio track management (added in Phase 4), but the PipeWire audio bridge (PCM capture/playback + Opus encode/decode) is deferred. A TODO in `server/pion/orchestrator.go` documents the session persistence aspect. This task depends on PipeWire integration work that is out of scope for this phase.

**Test**: Start server + CLI client with PipeWire loopback. Create session, join client. Play a 1kHz tone into loopback source. Verify `ChannelStatus{ACTIVE}` and `bytes_transferred > 0`.
> **No test exists** — the PipeWire bridge feature is deferred.

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

**MET**: `TestForwardTrack_RTPPayloadMatch` proves byte-for-byte SFU forwarding between two in-process Pion peers. `TestIntegration_SessionWithAudioForwarding` tests the full path through `Orchestrator.JoinSession()` → `evaluateBindings()` → `activateBinding()` → `ForwardTrack()` with server-initiated renegotiation, and hard-asserts that Client B receives the forwarded Opus track.

> CLI client with PipeWire loopback: audio played into loopback source → sent via WebRTC → received by second client.

**NOT MET (DEFERRED)**: CLI client has BindChannel handling but no PipeWire bridge (audio capture/playback + Opus encode/decode). This exit criterion is deferred to a future phase.

## Spec Updates

- 2.3 Session Orchestration → 7
- 3.3 Channel Lifecycle → 6
- 5.2 Session Templates → 4
- 5.3 Sessions → 6

## Summary of Remaining Gaps

| Gap | Severity | Description |
|-----|----------|-------------|
| CLI PipeWire bridge | High | No audio capture/playback or Opus encode/decode in CLI. Deferred. |
| CLI ChannelStatus reporting | Medium | No `ChannelStatus` message support in CLI. Deferred. |
| Session persistence | Low | Live session state is in-memory only; doesn't survive restart. TODO added. |

## Fix Review

Reviewed 2026-04-22 against the 7 gaps identified in the original Phase 5 review.

### Gap #1: No `waiting → active` transition — FIXED

**Implementation** (`server/pion/orchestrator.go:296–303`):
`evaluateBindings()` checks `len(ls.Bindings) > 0 && ls.Session.Status == SessionWaiting` after activating new bindings, then calls `SessionService.UpdateSessionStatus(id, SessionActive)` and updates the in-memory session status. This is correct — the transition fires exactly once (the first time a binding activates), and persists to SQLite.

**DB layer** (`server/sqlite/session.go:79–95`):
`UpdateSessionStatus` is a clean `UPDATE sessions SET status = ? WHERE id = ?` with proper `RowsAffected` check and `ErrNoRows` on miss.

**Unit test** (`server/pion/orchestrator_test.go:428–453`, `TestOrchestrator_JoinSession_TransitionsToActive`):
Uses a mock with `UpdateSessionStatusFn` that captures the ID and status. Asserts: correct session ID, `SessionActive` status, `UpdateSessionStatusInvoked` flag, and the in-memory `ls.Session.Status` via `GetLiveSession`. Not a fake — the mock captures and the test verifies all four properties.

**Integration test** (`server/cmd/ct-server/integration_test.go:1040–1078`, `TestIntegration_SessionActiveTransition`):
Creates a session via REST (asserts status=`"waiting"`), connects a WebRTC client, joins the session, then fetches the session again via `GET /api/sessions/:id` and asserts status=`"active"`. This is end-to-end through SQLite — proves the DB was actually updated.

**Verdict**: Real fix, properly tested at both unit and integration levels.

### Gap #2: REST DELETE doesn't notify clients — FIXED

**Interface** (`server/domain.go:133–136`):
`SessionOrchestrator` interface with `EndSession(sessionID string)` added to domain types — clean dependency inversion.

**Handler wiring** (`server/http/handler.go:40–41, 676–689`):
`Orchestrator` field on `Handler` struct. `handleDeleteSession` calls `h.Orchestrator.EndSession(id)` before `h.SessionService.EndSession(id)`. Nil-guarded so it works without orchestrator (e.g., in unit tests).

**Main wiring** (`server/cmd/ct-server/main.go:117`):
`Orchestrator: orch` assigned to the handler — the Pion orchestrator satisfies `SessionOrchestrator`.

**Orchestrator behavior** (`server/pion/orchestrator.go:196–248`):
`EndSession` deactivates all bindings (stops forwarding, closes recorders), sends `SESSION_ENDED` to all clients, and removes the live session. This is the correct behavior — clients get notified and resources are cleaned up.

**Integration test** (`server/cmd/ct-server/integration_test.go:988–1034`, `TestIntegration_DeleteSessionNotifiesClients`):
Connects a WebRTC client, joins a session as "host", sends `DELETE /api/sessions/:id`, then uses `drainUntilSessionEvent(SESSION_ENDED, 5s)` to assert the client received the event. Also verifies the session is `"ended"` in the DB via GET. The `drainUntilSessionEvent` method is real — it reads from the control data channel, deserializes protobuf, and `t.Fatal`s on timeout.

**Verdict**: Real fix, properly tested end-to-end. The full chain from REST DELETE → Orchestrator.EndSession → WebRTC control message → client is proven.

### Gap #3: No SINK BindChannel test — FIXED

**Test** (`server/pion/orchestrator_test.go:455–503`, `TestOrchestrator_JoinSession_SinkReceivesBindChannel`):
Joins interviewer first (activates only the record binding), then joins candidate as a second peer. Registers an `OnMessage` handler on candidate's data channel to capture protobuf messages. Collects messages for 3s, filters for `BindChannel` with `Direction_SINK`, then asserts `require.NotEmpty(sinkBinds)` and `assert.Equal("speaker", sinkBinds[0].GetLocalName())`.

This is a genuine two-peer test with real Pion connections — not a mock. The data channel is the actual server-created control DC, and the messages are real protobuf over SCTP.

**Verdict**: Real test, correct assertion. Proves the sink peer receives `BindChannel{SINK}`.

### Gap #4: SFU renegotiation + hard assertion — FIXED

**`Negotiate()` method** (`server/pion/peer.go:223–244`):
Creates a server-side SDP offer and delivers it via the `onNegotiationNeeded` callback.

**Callback wiring** (`server/ws/signaling.go:116–130`):
The signaling handler registers a callback that serializes the offer to JSON and sends it over the WebSocket connection. Client answer is handled by `handleAnswer` which calls `peer.HandleAnswer`.

**`ForwardTrack`** (`server/pion/forward.go:29–117`):
After `sinkPeer.pc.AddTrack(localTrack)`, calls `sinkPeer.Negotiate()`. This triggers the server→client offer, client responds with answer, renegotiation completes, and the track becomes available on the client.

**Integration test** (`server/cmd/ct-server/integration_test.go:569–707`, `TestIntegration_SessionWithAudioForwarding`):
Client B calls `startRenegotiationHandler()` which runs a goroutine reading WebSocket messages. When it receives a server `"offer"`, it does `SetRemoteDescription` + `CreateAnswer` + `SetLocalDescription` and sends the answer back. Client A adds an Opus track, renegotiates, sends 20 RTP packets. The test then does:
```go
select {
case track := <-clientB.onTrackCh:
    assert.Contains(t, strings.ToLower(track.Codec().MimeType), "opus")
case <-time.After(15 * time.Second):
    t.Fatal("timed out waiting for forwarded track on Client B")
}
```
This is a **hard assertion** — `t.Fatal` on timeout, not a soft skip. The `onTrackCh` is populated by a real `pc.OnTrack` handler. The test proves end-to-end: Client A audio track → server SFU → server renegotiation → Client B receives track.

**Verdict**: Real fix. The old soft timeout has been replaced with `t.Fatal`. Server-initiated renegotiation is properly wired through signaling.

### Gaps #5–7: CLI PipeWire bridge, BindChannel, ChannelStatus — PROPERLY DEFERRED

Documented in task 5.5 with rationale: "The CLI client has BindChannel/UnbindChannel handling and audio track management (added in Phase 4), but the PipeWire audio bridge (PCM capture/playback + Opus encode/decode) is deferred." The summary table at the bottom lists all three deferred gaps with severity ratings. A TODO comment in `orchestrator.go:22–26` documents the session persistence aspect.

These are correctly scoped out — PipeWire integration is a different dependency boundary from the server-side SFU work that Phase 5 covers.

### Fake/Stub Analysis

No fakes detected:

- **Mock `SessionService.UpdateSessionStatus`**: Has a nil-guard (returns `nil` if `Fn` not set), but the test that exercises it (`TransitionsToActive`) explicitly sets the `Fn` and verifies captured values. The nil-guard only means other tests that don't care about this path don't need to wire it — appropriate design.
- **`clientA_orch_verify`**: Returns `nil` and only checks `status != "ended"` via REST. This is the weakest assertion in the suite, but it's supplementary — the actual track receipt is proven by `onTrackCh` above it. Not a fake, just a secondary check.
- **All integration tests use real infrastructure**: SQLite DB, real Pion peer connections, real WebSocket signaling, real protobuf over SCTP data channels. No mocks in the integration layer.
- **`drainUntilSessionEvent`**: `t.Fatal`s on timeout — not a soft pass.

### Summary

| Gap | Status | Evidence |
|-----|--------|----------|
| #1 waiting→active | FIXED | `evaluateBindings` + `UpdateSessionStatus` + unit + integration test |
| #2 DELETE notifies | FIXED | `SessionOrchestrator` interface + handler wiring + integration test |
| #3 SINK BindChannel | FIXED | Two-peer unit test with real Pion DCs |
| #4 SFU renegotiation | FIXED | `Negotiate()` + signaling callback + hard `t.Fatal` assertion |
| #5 CLI BindChannel | DEFERRED | Documented with rationale |
| #6 CLI PipeWire | DEFERRED | Documented with rationale |
| #7 CLI ChannelStatus | DEFERRED | Documented with rationale |

**All tests pass.** No test failures, no fakes, no stubs that silently succeed.
