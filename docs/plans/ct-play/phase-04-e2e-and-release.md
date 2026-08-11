# Phase 4: Full-Process Proof and Release Integration

## Goal

Turn the completed command into a release-ready, continuously verified product surface. This phase consolidates full-process coverage for discovery and playback, wires CI/build/documentation, and corrects obsolete specs only after the replacement behavior is proven.

## BDD Success Criteria

### Scenario: Complete operator workflow

- **Given** a production-wired server, an authenticated assigned user, a session, a publishable channel, and a WAV or MP3 fixture
- **When** the operator runs `ct-play session`, chooses an ID, runs `ct-play --session <id> channel`, chooses an ID, and runs `ct-play --session <id> --channel <id> <file>`
- **Then** each command succeeds through its public boundary
- **And** a real listener receives the selected file audio
- **And** the sender exits cleanly at EOF.

### Scenario: Configuration-only workflow

- **Given** a permission-restricted config file containing host, username, password, session ID, and channel ID
- **When** the operator runs `ct-play <file.mp3>`
- **Then** no additional flags are required
- **And** the correct session/channel receives the file
- **And** no secret appears in command output or logs.

### Scenario: Environment-only workflow

- **Given** all five requested `CT_PLAY_*` environment values
- **When** the operator runs discovery and playback commands
- **Then** they behave identically to explicit flags
- **And** explicit flags can override individual environment values.

### Scenario: Repeated invocations use fresh tickets

- **Given** one valid file and channel
- **When** `ct-play` is run twice sequentially
- **Then** both invocations succeed
- **And** database evidence shows distinct consumed tickets
- **And** no ticket from the first invocation is replayed.

### Scenario: Build artifact is independently runnable

- **Given** a clean checkout with documented runtime dependencies
- **When** `task build:play` runs
- **Then** `bin/ct-play --help`, `session`, `channel`, and playback work without source-tree-only assumptions
- **And** CI executes the same artifact-oriented tests.

### Scenario: Documentation matches production contracts

- **Given** the proven implementation
- **When** users read CLI help and repository documentation
- **Then** they see the actual current login, 60-second one-time ticket, session WebSocket, supported formats, config precedence, RBAC caveat, and ffmpeg requirement
- **And** obsolete 24-hour token or `/ws/signaling` guidance is not presented as ct-play behavior.

## Implementation Instructions

1. Consolidate a reusable full-process test harness, likely `test/run-ct-play-e2e.sh` plus Go/Playwright receiver assertions, that:
   - creates/drops a fresh PostgreSQL database;
   - starts a real `ct-server` with production media services;
   - builds `bin/ct-play`;
   - creates users, assignments, sessions, and channels through public REST APIs;
   - launches actual ct-play subprocesses;
   - receives and analyzes real audio;
   - cleans processes, temporary config files, tickets, and databases on all exits.
2. Add Taskfile entries:
   - `build:play` from Phase 1;
   - `test:e2e:play` or similarly precise command for the full-process suite;
   - include the suite in the appropriate release/full test aggregate without requiring physical K2B hardware.
3. Update `.github/workflows/ci.yml` if needed:
   - install or verify `ffmpeg` for ct-play tests;
   - use PostgreSQL 16 and existing libopus prerequisites;
   - build and exercise `bin/ct-play`;
   - retain test artifacts on failure without exposing generated config credentials or tokens.
4. Add fixture policy:
   - reuse `test/fixtures/test-tone-1khz-5s.wav`;
   - add or generate a deterministic MP3 tone fixture if the existing speech fixture cannot support strong content assertions;
   - document fixture provenance and keep files small.
5. Add release-facing documentation, such as `docs/ct-play.md` or the project's established CLI docs location:
   - installation/build;
   - ffmpeg runtime dependency;
   - flags, exact environment names, config search path and precedence;
   - secure password handling recommendation;
   - `session`, `channel`, explicit playback, environment-only, and config-only examples;
   - current role/channel publishability caveat;
   - exit/error behavior.
6. Correct affected living specs in a separate spec-only commit after tests prove behavior:
   - `spec/server/rest-api.md` current JWT login and 60-second one-time media tickets;
   - `spec/server/webrtc-signaling.md` session media endpoint and actual mixed/decoded server behavior where relevant;
   - `spec/cli-client/auth-connection.md` ct-play username/password flow and current ticket lifecycle;
   - update `spec/index.md` confidence/prose as required.
7. If the generated Go client is adopted, ensure `task api:generate:go` is part of verification and generated output is committed according to project convention. If not adopted, document the intentionally small `httpapi` surface and avoid stale generated-client claims.
8. Run broad regression gates because shared Pion code may affect existing clients:
   - all CLI unit tests;
   - server integration/audio-flow tests;
   - existing browser golden audio suite if signaling code changed;
   - physical K2B suite only if shared ABC/device capture behavior changed.
9. Add a final security review:
   - no secrets in argv in recommended examples except the explicitly supported but discouraged password flag;
   - no password/token logs;
   - config permission warnings;
   - TLS verification remains enabled;
   - one-time ticket scoping and RBAC remain server-enforced.

## End-to-End Test Plan

The release suite must include all of the following using real public boundaries:

1. **Session discovery**: real translator assignment, actual `ct-play session`, unauthorized session absence.
2. **Channel discovery**: actual `ct-play channel`, names/IDs/types, unauthorized session failure.
3. **Explicit WAV playback**: flags supply session/channel; listener decodes a 1 kHz tone.
4. **Bare MP3 playback**: environment or config supplies session/channel; listener validates deterministic MP3 content.
5. **Precedence**: conflicting config/environment/flags direct audio to distinguishable sessions/channels; listener evidence proves which won.
6. **RBAC denial**: selected visible but non-publishable feed channel yields no audio and non-zero exit.
7. **Fresh-ticket replay resistance**: two runs succeed with distinct tickets; deliberate ticket replay fails.
8. **Cancellation**: SIGINT cleans ffmpeg and WebRTC; no orphan process or connected source remains.
9. **Dependency and input errors**: missing ffmpeg, missing file, malformed media, unsupported extension, unreachable server, bad credentials, no session, no channel.
10. **Regression**: existing `task test:integration` audio tests remain green; if shared signaling changed, run `task test:e2e` or the precise browser golden audio target.

Required commands at completion:

```text
task build:play
task test:unit:go
task test:integration
task test:e2e:play
task lint:go
```

If existing aggregate names differ during implementation, update this plan/doc command list to the final committed names rather than leaving fictional commands.

## Anti-Cheating Audit

- Inspect the E2E harness to ensure it launches the compiled `bin/ct-play`, a real server process, PostgreSQL, ffmpeg, and a real Pion/browser receiver.
- Confirm no first-party server/API/media component is mocked in integration or E2E tests.
- Verify audio assertions inspect decoded received content and duration, not only source creation, ticket status, or process exit.
- Confirm fixture frequencies/content are not hard-coded into ct-play production code.
- Check CI installs runtime dependencies rather than skipping tests when ffmpeg or PostgreSQL is unavailable.
- Search for skipped tests, broad retries, arbitrary sleeps replacing readiness checks, and test-only behavior flags.
- Confirm repeated invocation tests query durable ticket state or otherwise prove distinct ticket use.
- Ensure artifacts and logs redact passwords, access JWTs, refresh tokens, and media tickets.
- Verify documentation examples do not put passwords in shell history as the recommended path.
- Confirm spec confidence increases are backed by named passing tests and spec changes are isolated in their own commit.
- Check shared Pion changes against `ct-client` and `ct-abc` regression tests; ct-play must not silently break existing edge clients.

## Completion Gate

- [ ] Every original requirement maps to a passing full-process scenario.
- [ ] WAV and MP3 content are proven at a real receiver.
- [ ] Flags, environment, and config-only workflows are proven through the actual binary.
- [ ] Authorization, ticket freshness, cancellation, and error cases pass.
- [ ] Build and CI tasks are real, documented, and green.
- [ ] Existing server/CLI/media regression suites pass.
- [ ] Security and anti-cheating audits find no secret leakage, bypasses, mocks, or fixture shortcuts.
- [ ] Generated output is current if used.
- [ ] User documentation matches actual behavior.
- [ ] Spec corrections and confidence updates are committed separately with provenance tags.
