# Phase 4: CLI Client Core

[← Roadmap](index.md)

**Status**: `PARTIAL — 3/5 tasks complete, 2 have gaps`  
**Depends on**: Phase 3 (server must handle Hello/Welcome and JoinSession)

The CLI client authenticates, connects via WebSocket, establishes WebRTC, reports capabilities via the control channel, and discovers PipeWire sources/sinks.

## Tasks

### 4.1 Config Loading
- [x] JSON config loading with `--config` flag and `CROSSTALK_CONFIG` env var — `3cd1cd6`
- [x] Env var overrides (`CROSSTALK_SERVER`, `CROSSTALK_TOKEN`, etc.) — `3cd1cd6`
- [ ] Validate against `cli/config.schema.json`
- [x] Configure `slog` JSON logger — `3cd1cd6`

> **Gaps**: The `config.schema.json` file exists but is never loaded or used at runtime.
> Validation is hand-written in `validateConfig()` (checks `server_url`, `token`, `log_level`),
> which covers required fields but does **not** validate against the JSON Schema itself.
> No JSON Schema validation library is imported. The schema is cosmetic only.

**Test**: `cli/cmd/ct-client/config_test.go` — ✅ 9 tests pass. Covers: file load, env overrides,
env-only (no file), missing `server_url`, missing `token`, invalid `log_level`, explicit
missing file, `--config` flag priority, slog level parsing. Tests **do validate** that env wins
over file values.

### 4.2 REST Authentication
- [x] HTTP client: set `Authorization: Bearer <token>` from config — `d88e7d7`
- [x] `POST /api/webrtc/token` → receive short-lived WebRTC token — `d88e7d7`
- [x] On 401/403: exit immediately with clear error — `d88e7d7`, `fbcf4f8`

> **Note**: Implementation is solid. The spec (§3.1) also mentions `GET /api/clients/self` as a
> preliminary token-validation call before requesting the WebRTC token. This is **not implemented**
> — the client skips directly to WebSocket auth using the API token (per `fbcf4f8`). This is a
> design simplification, not a bug, but diverges from the spec auth flow diagram.
>
> The spec also mentions WebRTC token refresh at ~23h — not implemented (out of scope for Phase 4).

**Test**: `cli/pion/auth_test.go` — ✅ 7 tests pass. Uses `httptest.Server` mock. Covers: 200 +
token stored, 401 → `AuthError`, 403 → `AuthError`, 500 → non-auth error, empty token, connection
refused, `IsAuthError` helper. Matches the roadmap's test spec exactly.

### 4.3 WebSocket + WebRTC Connection
- [x] Open WebSocket to `/ws/signaling?token=<webrtc_token>` — `cf1ca54`, `fbcf4f8`
- [x] Create Pion PeerConnection, generate SDP offer, send over WebSocket — `cf1ca54`
- [x] Receive SDP answer, apply it — `cf1ca54`
- [x] Exchange ICE candidates — `cf1ca54`, `d36c011`
- [x] Open `control` data channel, send `Hello` with capabilities — `cf1ca54`

> **Note**: The connection uses the API token directly for WebSocket auth (`fbcf4f8`), bypassing
> the `POST /api/webrtc/token` step at runtime. The `RequestWebRTCToken()` method exists and is
> tested but is not called in `Client.connectOnce()`. The `Connection` struct receives whatever
> token `Client` passes through.
>
> ICE candidate serialization was fixed in `d36c011` to handle the server sending
> `ICECandidateInit` objects vs plain strings.

**Test**: `cli/pion/connection_test.go` — ✅ 6 tests pass.
`TestConnection_ConnectAndSendHello` uses a full `mockSignalingServer` that creates a real
server-side PeerConnection, performs real SDP offer/answer exchange, real ICE exchange, opens
a real `control` data channel, and verifies Hello receipt + Welcome send. This is a genuine
in-process WebRTC integration test (not against the real `ct-server`).

Also `cli/pion/client_test.go` `TestClient_ConnectAndSendHello` — ✅ validates the full
`Client.Run()` lifecycle with mock PipeWire returning devices, verifying Hello contains
correct sources/sinks.

> **Gap**: The roadmap says "Start real `ct-server`, start `ct-client` with valid config, verify
> `GET /api/clients` returns the client with `state: READY`." — this E2E test against the real
> server does **not** exist. Tests use an in-process mock signaling server, not `ct-server`.
> This is a pragmatic choice but doesn't fulfill the stated acceptance criterion.

### 4.4 PipeWire Source/Sink Discovery
- [x] `pipewire/` package: enumerate audio source/sink nodes via `pw-cli` — `0ee28c7`
- [x] Filter by `CROSSTALK_SOURCE_NAME` / `CROSSTALK_SINK_NAME` if set — `0ee28c7`
- [x] Report discovered sources/sinks in `Hello` message — `cf1ca54`
- [ ] Watch for hotplug events, send `ClientStatus` updates

> **Gap**: Hotplug watching is **not implemented**. There is no file watcher, D-Bus subscription,
> or polling loop for PipeWire device changes. The `SendClientStatus()` method exists on
> `Connection` but is never called from `Client`. Discovery runs once at connect time only.

**Test**: `cli/pipewire/pipewire_test.go` — ✅ 4 tests pass. `TestParsePWNodes` parses realistic
`pw-cli` output for 5 nodes (Audio/Source, Audio/Sink, Audio/Source/Virtual, Audio/Sink/Virtual,
Video/Source). `TestService_DiscoverWithFilter` validates name filtering logic inline (not through
`Discover()` method — it manually iterates parsed nodes). Does **not** test `Discover()` with
a real or mocked `pw-cli` process.

> **Gap**: The roadmap test spec says "On a system with PipeWire running, `pipewire.Discover()`
> returns at least one source and one sink." — no such integration test exists. Only parsing and
> filtering logic are unit-tested. The `Discover()` → `listNodes()` → `exec.Command("pw-cli")`
> path is untested (requires real PipeWire).

### 4.5 Connection Resilience
- [x] Exponential backoff reconnect on WebRTC disconnect (1s, 2s, 4s, ... cap 60s) — `d780b2f`
- [x] Exit on REST auth failure (don't retry bad creds) — `d780b2f`, `fbcf4f8`
- [x] Log all connection state transitions — `cf1ca54`, `d780b2f`

> **Note**: Backoff works correctly: 1s→2s→4s→8s→16s→32s→60s (capped). Auth errors
> (401/403 from both REST and WebSocket) are detected and cause immediate exit.
> ICE state changes are logged via `slog.Info`. WebSocket 401/403 detection was added in
> `fbcf4f8`.

**Test**: `cli/pion/client_test.go` — ✅ 4 tests pass.
- `TestClient_CalculateBackoff` — validates exact backoff sequence including 60s cap.
- `TestClient_AuthFailureNoRetry` — 401 from server → `Run()` returns immediately with auth error.
- `TestClient_ReconnectAfterFailure` — mock connection fails first attempt, succeeds second.
  Verifies `connectAttempts ≥ 2` and client becomes connected.

> **Gap**: The roadmap test spec says "Start client against server, kill server, verify client
> logs reconnection attempts with increasing backoff. Restart server, verify client reconnects
> and re-sends `Hello`." — this full server-kill/restart E2E test does **not** exist. The
> reconnect test uses a mock connection factory that simulates failure, not a real server restart.
> The test also doesn't verify that backoff durations increase or that Hello is re-sent on
> reconnect (only checks attempt count).

## Exit Criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | `ct-client` starts with JSON config | ✅ | `cli/cmd/ct-client/main.go` + config_test.go |
| 2 | Authenticates to server via REST token | ⚠️ | `AuthClient.RequestWebRTCToken()` exists and is tested, but not called at runtime — client uses API token directly for WS auth |
| 3 | Establishes WebRTC connection via WebSocket signaling | ✅ | `Connection.Connect()` + tested with in-process mock server |
| 4 | Sends `Hello` with PipeWire sources/sinks | ✅ | `Connection.SendHello()` + verified in client_test.go |
| 5 | Appears in `GET /api/clients` with capabilities | ❌ | No E2E test against real `ct-server` exists |
| 6 | Reconnects after server restart | ⚠️ | Reconnect logic exists and is tested with mock, but not tested against real server kill/restart |

> **Not yet tested with a real `ct-server` process and PipeWire loopback devices** as the exit
> criteria require. All tests use in-process mocks. The PipeWire integration test requires a
> system with PipeWire running.

## Summary of Gaps

1. **JSON Schema validation** (4.1): `config.schema.json` exists but is decorative — no runtime schema validation.
2. **`GET /api/clients/self`** (spec §3.1): Spec auth flow shows a preliminary token validation call; not implemented.
3. **WebRTC token flow** (4.2/4.3): `RequestWebRTCToken()` exists but is bypassed at runtime; client uses API token directly for WS auth.
4. **PipeWire hotplug** (4.4): No event watching. Discovery is one-shot at connect time. `SendClientStatus` is never called.
5. **PipeWire integration test** (4.4): `Discover()` is not tested end-to-end (only parsing logic).
6. **E2E against real server** (4.3, 4.5): No test starts `ct-server` and verifies `GET /api/clients`. No server kill/restart reconnection test.
7. **WebRTC token refresh** (spec §3.1): Token refresh at ~23h mark not implemented (likely out of Phase 4 scope).

## Spec Updates

- 3.1 Auth & Connection → 3
- 3.2 PipeWire Integration → 3
