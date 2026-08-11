# Phase 2: Channel Discovery and Deterministic Selection

## Goal

Add `ct-play channel` and centralize session/channel selection rules used by both discovery and playback. This phase follows authenticated session discovery so it can resolve a real assigned session and enforce clear, deterministic behavior before any media ticket is minted.

## BDD Success Criteria

### Scenario: List channels for an explicit assigned session

- **Given** an authenticated translator assigned to a session containing named feed and broadcast channels
- **When** the translator runs `ct-play --session <session-id> channel`
- **Then** stdout lists each channel's ID, name, and type in deterministic order
- **And** the command exits zero.

### Scenario: Resolve session from environment or config

- **Given** `session_id` in a config file or `CT_PLAY_SESSION_ID` in the environment
- **When** the user runs `ct-play channel` without `--session`
- **Then** the command lists channels from the resolved session
- **And** an explicit `--session` overrides both sources.

### Scenario: Require a session for channel listing

- **Given** no session ID in flags, environment, or config
- **When** the user runs `ct-play channel`
- **Then** the command exits non-zero before requesting channels
- **And** the error instructs the user to run `ct-play session` and supply `--session` or `CT_PLAY_SESSION_ID`.

### Scenario: Reject an unassigned session

- **Given** a translator authenticated successfully but not assigned to the requested session
- **When** the translator runs `ct-play --session <unassigned-id> channel`
- **Then** the command exits non-zero with an authorization error
- **And** it does not print channel data or reveal whether the session exists beyond the server's approved error semantics.

### Scenario: Handle empty channel sets

- **Given** an assigned session with no channels
- **When** the user runs `ct-play --session <id> channel`
- **Then** the command exits zero with a stable header and no rows
- **And** it provides a concise diagnostic that the session has no available channels.

### Scenario: Resolve a playback channel by exact ID

- **Given** a configured session and channel ID belonging to that session
- **When** playback preparation resolves selections
- **Then** the exact channel is selected
- **And** its ID, name, and type are retained for ticket validation and diagnostics.

### Scenario: Reject a channel from another session

- **Given** a session ID and a channel ID belonging to a different session
- **When** playback preparation resolves selections
- **Then** resolution exits non-zero before creating a WebRTC connection
- **And** the error identifies the selected channel ID without exposing credentials.

## Implementation Instructions

1. Extend the `cli/httpapi` adapter with `ListChannels(ctx, accessToken, sessionID)` against `GET /api/sessions/{id}/channels`. Preserve server status semantics and bounded response handling from Phase 1.
2. Add `channel.go` under `cli/cmd/ct-play`:
   - Authenticate through the shared invocation session.
   - Require the resolved session ID.
   - Print `ID\tNAME\tTYPE` with stable ordering by type, name, then ID or another documented deterministic order.
   - Do not infer publishability solely from channel type; output is discovery, while the server-issued ticket remains authoritative.
3. Introduce a reusable ct-play application layer without creating sibling-package imports. One valid arrangement is:
   - dependency-free interfaces/value types in `cli/domain.go` or a self-contained `cli/play` package;
   - concrete HTTP and Pion/ffmpeg adapters injected by `cmd/ct-play`;
   - command handlers depend only on interfaces.
4. Implement session/channel resolution used by future playback:
   - Resolve session and channel only by ID for non-interactive operation.
   - Fetch sessions first and verify the selected session appears in the authenticated user's list; retain the server 403 behavior for direct channel requests as a second boundary.
   - Fetch channels and verify the selected channel ID belongs to the resolved session.
   - Return typed selection errors with actionable commands.
5. Preserve all configuration precedence established in Phase 1. Add focused tests proving `--session` and `--channel` override `CT_PLAY_SESSION_ID` / `CT_PLAY_CHANNEL_ID` and config values.
6. Update root help/examples:
   - `ct-play session`
   - `ct-play --session <id> channel`
   - environment/config equivalents.
7. Add unit tests for stable formatting, missing session errors, cross-session channel rejection, empty lists, and server failures. Use `httptest.Server` only for malformed response/status behavior.
8. Add real integration fixtures for multiple session/channel combinations to ensure no client-side cross-session leakage.

## End-to-End Test Plan

1. Start a real PostgreSQL-backed server and create:
   - translator A assigned to session A;
   - session A with one feed and two broadcast channels;
   - session B with a channel but no assignment to translator A.
2. Launch the real binary:
   - `ct-play --session <A> channel` using credentials from flags;
   - `CT_PLAY_SESSION_ID=<A> ct-play channel` using environment credentials;
   - `ct-play --config <file> channel` using config credentials.
3. Assert stdout contains all session A channel IDs/names/types in deterministic order and excludes session B's channel.
4. Launch with session B and assert a non-zero authorization failure and no channel output.
5. Launch without a session and assert the guidance references `ct-play session`.
6. Create an empty assigned session and assert zero-row successful output.
7. Prepare playback with session A plus session B's channel ID and assert failure occurs before the server records a media ticket or peer allocation. This may be observed through database ticket count and debug peer count in the test harness.
8. Exact gates:
   - `task build:play`
   - `task test:unit:go`
   - the full-process session/channel CLI integration command.

## Anti-Cheating Audit

- Confirm `ct-play channel` uses the authenticated REST endpoint and does not derive channels from local config or fixtures.
- Confirm integration tests create a channel in an unauthorized session and assert it is absent/fails.
- Inspect selection code to ensure channel IDs are verified against the selected session before ticket minting.
- Confirm tests launch the real binary and do not invoke a private resolver directly as their only evidence.
- Verify empty API/error responses are distinguished; an authorization/network failure must not become an empty successful list.
- Confirm the client does not claim a channel is publishable based only on `type`; ticket capability validation remains in Phase 3.
- Check that no server authorization logic is duplicated as the sole enforcement in the client.
- Confirm logs and errors do not contain the password, access JWT, or future media ticket.

## Completion Gate

- [ ] All BDD scenarios pass.
- [ ] `ct-play channel` outputs real channel ID, name, and type for an assigned session.
- [ ] Missing, unauthorized, empty, and cross-session cases have verified behavior.
- [ ] Session/channel flag, environment, and config precedence remains correct.
- [ ] Playback selection can return a verified session/channel pair without minting media.
- [ ] Full-process tests prove server-side authorization and isolation.
- [ ] `task build:play`, `task test:unit:go`, focused integration tests, and `task lint:go` pass.
