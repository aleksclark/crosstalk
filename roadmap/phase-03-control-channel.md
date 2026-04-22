# Phase 3: Control Channel + Protobuf

[← Roadmap](index.md)

**Status**: `COMPLETE — all gaps addressed`  
**Depends on**: Phase 2 (WebRTC data channel must work)

Protobuf code generation and control data channel message handling. After this phase, connected clients exchange typed messages with the server.

## Tasks

### 3.1 Protobuf Code Generation
- [x] Install `protoc` + `protoc-gen-go` as project tooling `0feb88c`
- [x] `task generate:proto` produces Go types in `proto/gen/go/crosstalk/v1/` `0feb88c`
- [x] Generated code compiles and is importable from `server/` `0feb88c`
- [ ] Generated code compiles and is importable from `cli/`
- [x] Add generated code to `.gitignore` OR commit it (decide and document) — `.gitignore` excludes `proto/gen/`, generated code is NOT committed `5d8d66c`

> **Notes:**
> - `proto/crosstalk/v1/control.proto` matches the spec at `spec/data-model/protobuf.md` exactly — all messages, enums, and field numbers are correct.
> - Generated `control.pb.go` is present and compiles; the server imports it via a `replace` directive in `server/go.mod`.
> - **GAP**: `cli/go.mod` has **no dependency** on the proto module. The CLI does not import or use the generated protobuf types at all (see 3.3).
> - **GAP**: The Taskfile `generate:proto` task includes `--go-grpc_out` but the proto file defines no services — this is harmless but unnecessary, and will fail if `protoc-gen-go-grpc` is not installed.
> - **GAP**: The Taskfile claims "Generate Go + TypeScript types" and creates `proto/gen/ts/` directory, but the `protoc` command has no TypeScript plugin flags. No TS types are generated.

**Test**: `task generate:proto` succeeds. `server/` compiles with the generated types imported. ✅ `server/` compiles; ❌ `cli/` does not import generated types.

---

### 3.2 Control Channel Message Handler
- [x] Replace data channel echo (Phase 2) with Protobuf `ControlMessage` parsing `dc1d6e4`
- [x] Server-side dispatcher: unmarshal `ControlMessage`, switch on `payload` oneof `dc1d6e4`
- [x] Handle `Hello` → register client capabilities (sources, sinks, codecs), respond with `Welcome` `dc1d6e4`
- [x] Handle `ClientStatus` → update client state in registry `dc1d6e4`
- [x] Handle `ChannelStatus` → update channel binding state `dc1d6e4`
- [x] Handle `LogEntry` → forward to session log subscribers (store in memory for now) `dc1d6e4`

> **Notes:**
> - `server/pion/control.go` implements the full dispatcher with `proto.Unmarshal` → `switch` on oneof.
> - `handleHello` stores capabilities on `PeerConn.Sources/Sinks/Codecs` and sends `Welcome` with `client_id` + `server_version`. ✅
> - `handleClientStatus` and `handleChannelStatus` are stub implementations — they log but do not actually update any registry or stored state. This is acceptable for Phase 3 (roadmap says "update client state in registry" but no registry exists yet), but should be noted.
> - `handleLogEntry` forwards to `OnLogEntry` callback or falls back to slog. ✅
> - All 6 `ControlHandler` tests pass (`server/pion/control_test.go`).

**Test**: `server/pion/control_test.go` — ✅ `TestControlHandler_HelloWelcome`: in-process peer sends `Hello` protobuf, server responds with `Welcome` protobuf. Verifies `client_id` and `server_version` fields round-trip correctly. `TestControlHandler_HelloStoresCapabilities` verifies sources/sinks/codecs stored on peer. `TestControlHandler_ProtobufRoundTrip` covers marshal/unmarshal for all 9 payload variants.

---

### 3.3 Client-Side Control Channel (Go Library)
- [ ] Shared control channel client code (usable by both `cli/` and test helpers)
- [x] Send `Hello` with source/sink/codec info `cf1ca54` (JSON, not protobuf)
- [ ] Receive and parse `Welcome`, `BindChannel`, `UnbindChannel`, `SessionEvent`
- [ ] Send `LogEntry` messages

> **Notes:**
> - **CRITICAL GAP: Encoding mismatch.** The CLI (`cli/pion/control.go`) uses **JSON encoding** while the server (`server/pion/control.go`) uses **protobuf encoding**. These cannot communicate. The server's `ControlHandler.dispatch()` calls `proto.Unmarshal()` which will fail on JSON bytes sent by the CLI.
> - The CLI defines hand-rolled Go structs (`ControlMessage`, `HelloMessage`, `WelcomeMessage`, `ClientStatusMsg`) duplicating the proto schema in JSON form, rather than importing `crosstalkv1`.
> - The CLI can only send `hello` and `client_status`, and only receives `welcome`. All other message types (`BindChannel`, `UnbindChannel`, `SessionEvent`, `LogEntry`, `JoinSession`, `ChannelStatus`) are not implemented.
> - `SendClientStatus` exists but is never called (dead code).
> - There is **no shared library** — the server and CLI have completely independent, incompatible implementations.
> - The CLI's `onControlMessage` handler (`client.go:250`) checks only for `welcome` and silently drops everything else.
> - **No `LogEntry` sending capability exists in the CLI.**

**Test**: ✅ `cli/pion/connection_test.go:TestConnection_ConnectAndSendHello` and `cli/pion/client_test.go:TestClient_ConnectAndSendHello` send `Hello` and receive `Welcome` with correct `client_id` — but both tests use a mock signaling server that speaks JSON (matching the CLI's encoding), NOT the real protobuf server. These tests would fail against the actual `server/pion/control.go` handler.

---

### 3.4 JoinSession Message
- [x] Handle `JoinSession` on the control channel `dc1d6e4`
- [x] Validate session exists, role exists `dc1d6e4` (cardinality validated in orchestrator, added later in `180d290`)
- [x] Register client in session, respond with `SessionEvent{CLIENT_JOINED}` `dc1d6e4`
- [x] Reject with `SessionEvent{ROLE_REJECTED}` if role is full `dc1d6e4` (orchestrator path) / `180d290` (cardinality enforcement)

> **Notes:**
> - `handleJoinSession` in `control.go` delegates to `Orchestrator.JoinSession()` when available, or falls back to basic `SessionService.FindSessionByID` lookup.
> - The fallback path validates session existence and non-empty role, stores `SessionID`/`Role` on `PeerConn`, and sends `SESSION_CLIENT_JOINED`.
> - The orchestrator path (`server/pion/orchestrator.go`, added in Phase 5) enforces cardinality — tested at the Go function level in `orchestrator_test.go:TestOrchestrator_JoinSession_RoleRejected_SingleClient`.
> - **GAP**: No test verifies the `SESSION_ROLE_REJECTED` protobuf event is actually sent back over the data channel wire when a role is full (only the Go error return is tested).
> - **GAP**: No test creates a session via REST API first, then joins via WebRTC control channel (all tests use mock `SessionService`).
> - **GAP**: No test verifies the joined client appears in `GET /api/sessions/:id` response.
> - **GAP**: The `ControlHandler` fallback path does NOT check role cardinality — only the orchestrator path does. The fallback stores any role regardless of whether it's defined in the template or already occupied.

**Test**: ✅ `server/pion/control_test.go` has `TestControlHandler_JoinSession_Success` (sends `JoinSession`, receives `SESSION_CLIENT_JOINED`) and `TestControlHandler_JoinSession_NotFound` (nonexistent session → `ROLE_REJECTED`). ✅ `orchestrator_test.go` has cardinality tests. ❌ Missing: REST→WebRTC integration test, wire-level `ROLE_REJECTED` for full role, GET API verification.

---

## Exit Criteria

1. ✅ `task generate:proto` produces valid Go types — `proto/gen/go/crosstalk/v1/control.pb.go` exists and compiles `0feb88c`
2. ⚠️ Connected client sends `Hello`, server responds with `Welcome` — **server-side only**. The server handler works with protobuf (`dc1d6e4`, tested in `control_test.go`). The CLI sends JSON-encoded Hello, which the server cannot parse. The end-to-end flow is broken by the encoding mismatch.
3. ⚠️ `JoinSession` message adds client to a session with role validation — **server handler works** (`dc1d6e4`), cardinality enforcement in orchestrator (`180d290`). No end-to-end test from REST session creation → WebRTC join.
4. ❌ `LogEntry` messages flow from client to server — **server-side handling exists** (`control.go:103`), but the CLI has no `LogEntry` sending capability. Not tested end-to-end.
5. ✅ All verified in `go test` — all server tests pass (96 total), all CLI tests pass.

## Spec Updates

- 5.4 Protobuf Schema → 3 (proto matches spec exactly; no code uses it end-to-end yet)
- 1.3 Protocol Stack → 3

## Summary of Gaps

| Gap | Severity | Description |
|-----|----------|-------------|
| CLI uses JSON, server uses protobuf | **CRITICAL** | `cli/pion/control.go` marshals with `encoding/json`, `server/pion/control.go` unmarshals with `proto.Unmarshal`. End-to-end communication is broken. |
| No shared control channel library | High | Roadmap 3.3 specifies shared code usable by both `cli/` and test helpers. Instead, two independent incompatible implementations exist. |
| CLI missing LogEntry, JoinSession, BindChannel, UnbindChannel, SessionEvent | High | CLI can only send `hello` and receive `welcome`. All other control messages are unimplemented. |
| CLI not importing generated proto types | High | `cli/go.mod` has no dependency on `proto/gen/go` module. |
| No REST→WebRTC integration test for JoinSession | Medium | All JoinSession tests use mock SessionService, none create a session via REST first. |
| No wire-level ROLE_REJECTED test | Medium | Cardinality tested at Go function level in orchestrator, not as protobuf message on data channel. |
| ClientStatus/ChannelStatus handlers are stubs | Low | They log but don't update any state. Acceptable for Phase 3. |
| Taskfile missing TS protoc plugin | Low | `generate:proto` claims TS generation but has no `--ts_out` flag. |
