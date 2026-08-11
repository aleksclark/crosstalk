# Phase 3: Session Media Transport and File Playback

## Goal

Deliver the core user outcome: `ct-play` streams a WAV or MP3 file in real time into the selected channel through the production one-time media-ticket, WebSocket signaling, WebRTC, Opus, and mixer path. This phase depends on verified authentication and selection so media setup can fail early and safely.

## BDD Success Criteria

### Scenario: Play a WAV file with explicit flags

- **Given** an authenticated user, an assigned session, a server-authorized publish channel, and a valid WAV file
- **When** the user runs `ct-play --session <session-id> --channel <channel-id> <file.wav>`
- **Then** `ct-play` mints a one-time ticket scoped to that session/channel
- **And** connects to `/api/sessions/{session-id}/ws`
- **And** publishes real-time Opus audio until file EOF
- **And** a real listener on that channel receives the expected audio
- **And** `ct-play` exits zero after clean transport shutdown.

### Scenario: Play an MP3 using environment or config selection

- **Given** valid host/credentials/session/channel in `CT_PLAY_*` values or a config file and a valid MP3 file
- **When** the user runs `ct-play <file.mp3>`
- **Then** the same production media path carries the decoded MP3 audio
- **And** the process exits zero only after the complete file is sent.

### Scenario: Reject unsupported or missing files

- **Given** a missing path, directory, unsupported extension, or malformed WAV/MP3
- **When** playback is requested
- **Then** the command exits non-zero with an actionable file/format/decode error
- **And** no successful playback is reported
- **And** any allocated media resources are closed.

### Scenario: Reject a non-publishable channel

- **Given** a user who can list a selected channel but whose role cannot publish into it
- **When** playback requests a scoped media ticket
- **Then** `ct-play` verifies the response does not include the selected channel in `produce_channel_ids`
- **And** exits non-zero before sending audio
- **And** explains that the authenticated role is not authorized to publish to that channel.

### Scenario: Handle one-time ticket expiry or consumption

- **Given** a media ticket that expires or is consumed before the WebSocket is admitted
- **When** the session connection fails before media starts
- **Then** `ct-play` may retry once by minting a fresh ticket
- **And** never reuses the old ticket
- **And** exits non-zero after the bounded retry fails.

### Scenario: Preserve real-time pacing

- **Given** a five-second input fixture
- **When** `ct-play` publishes it under normal conditions
- **Then** elapsed playback time remains within a documented tolerance around five seconds
- **And** packets are not emitted as an unbounded burst
- **And** the receiver observes continuous audio rather than only an initial buffer.

### Scenario: Cancel playback cleanly

- **Given** active file playback
- **When** the process receives SIGINT or its context is canceled
- **Then** ffmpeg, WebSocket, peer connection, and RTP writer terminate within a bounded interval
- **And** `ct-play` returns the documented cancellation exit behavior
- **And** the server eventually marks the source disconnected.

### Scenario: Diagnose missing ffmpeg

- **Given** `ffmpeg` is absent from `PATH`
- **When** playback is requested
- **Then** the command exits before authentication/media allocation with an explicit runtime dependency error
- **And** session/channel listing remains usable without ffmpeg.

## Implementation Instructions

1. Extend `cli/httpapi` with current media-ticket issuance:
   - `POST /api/webrtc/token` with `session_id`, `produce: [selectedChannelID]`, and `listen: []`;
   - parse `token`, expiration, role, `produce_channel_ids`, and owner generation;
   - validate the selected channel ID is present before dialing;
   - do not print or persist the ticket.
2. Add a finite session-media Pion adapter. Prefer a new self-contained package (for example `cli/sessionmedia`) over forcing persistent ABC behavior into `cli/pion.Connection` unless the refactor preserves all current callers and tests. It must:
   - dial `ws(s)://<host>/api/sessions/<sessionID>/ws?token=<ticket>`;
   - create an Opus `TrackLocalStaticRTP` before the client offer;
   - optionally create the expected `control` data channel if required by server negotiation, but not depend on Hello/Welcome or ABC control semantics;
   - queue local candidates until the WebSocket opens and remote candidates until the remote description exists;
   - send an offer, apply the answer, and expose a readiness signal only after the peer connection is connected;
   - drain RTCP and serialize WebSocket writes safely;
   - close all resources deterministically at EOF, cancellation, or error.
3. Add a dedicated ffmpeg adapter, likely `cli/ffmpeg`, that wraps the external process without importing the Pion adapter:
   - validate `.wav` and `.mp3` case-insensitively and verify a regular readable file;
   - execute ffmpeg with real-time input pacing (`-re`), audio-only decode, 48 kHz Opus, 20 ms frames, and RTP output to an ephemeral localhost UDP listener, mirroring proven options in `cli/pion/audio.go`;
   - expose parsed RTP packets or an interface callback, not Pion-specific types if package boundaries would otherwise import a sibling adapter;
   - capture bounded stderr for errors while streaming structured diagnostic lines without secrets;
   - treat non-zero exit, empty output, malformed RTP, and premature EOF as failures;
   - terminate the process group and reap it on cancellation.
4. Add a `cli/play` orchestration service using injected domain interfaces for authentication/API, file packet source, and session media sink. It owns this sequence:
   1. validate ffmpeg and input file;
   2. authenticate;
   3. resolve/verify session and channel through Phase 2;
   4. mint and validate a one-time ticket;
   5. establish media transport;
   6. stream RTP with context cancellation;
   7. wait for ffmpeg completion and flush/close transport;
   8. report duration and channel metadata without credentials.
5. Wire root positional behavior in Cobra:
   - known subcommands `session` and `channel` take precedence;
   - root accepts exactly one positional audio file;
   - zero args shows help and exits according to Cobra conventions;
   - more than one file is a usage error;
   - examples include both required invocation forms from the request.
6. Define bounded retry semantics:
   - one retry only for pre-media ticket expiry/consumption or transient WebSocket admission failure;
   - remint a ticket before retry;
   - never restart file playback after any RTP packet has been accepted unless a future explicit resume design exists.
7. Add unit tests:
   - file validation and extension handling;
   - ffmpeg argv contains real-time pacing and expected codec parameters;
   - RTP parsing, empty output, malformed packet, cancellation, and child reaping;
   - media signaling order and candidate queues using a protocol test server;
   - ticket capability validation and no-replay retry behavior;
   - Cobra positional dispatch.
8. Update runtime/help documentation with ffmpeg requirement, authorization caveat, and examples. Do not claim feed-channel publication works for admin/translator roles unless the server ticket authorizes it.

## End-to-End Test Plan

1. Build the actual `bin/ct-play` and start a real PostgreSQL-backed `ct-server` with production media-ticket, Pion, mixer, and Opus wiring.
2. Create a user and assigned session with a channel the role may publish into. For current user RBAC this is a broadcast channel. Also create a feed channel for the negative authorization case.
3. Connect a real listener to the broadcast channel using existing `newTicketMediaClient`/broadcast listener infrastructure or a browser listener through Playwright.
4. Launch actual subprocesses:
   - explicit flags with `test/fixtures/test-tone-1khz-5s.wav`;
   - environment/config selection with `test/fixtures/test-speech-5s.mp3` or a deterministic MP3 tone fixture.
5. Decode received audio through the production codec boundary and assert:
   - expected dominant frequency for the WAV fixture;
   - non-silence and expected duration/content signature for MP3, or use a deterministic MP3 tone and assert frequency;
   - elapsed sender duration is within real-time tolerance;
   - process exits zero at EOF.
6. Run against the feed channel with translator/admin credentials; assert non-zero authorization failure and no received audio.
7. Test missing ffmpeg by launching with a controlled `PATH`; assert discovery commands still work and playback fails clearly.
8. Test SIGINT during a longer fixture; assert subprocess termination, no orphan ffmpeg process, peer cleanup, and disconnected source state.
9. Test one-time ticket semantics by forcing a pre-dial delay/controlled first admission failure through a production-equivalent test seam; assert a fresh ticket is minted and no ticket is replayed. Do not add a production behavior flag solely for this test.
10. Exact gates:
    - `task build:play`
    - focused CLI unit tests
    - `task test:integration`
    - the new ct-play full-process audio test.

## Anti-Cheating Audit

- Confirm the full-process test launches `bin/ct-play` with a real file and does not inject tones directly into the mixer or Pion track.
- Inspect the ffmpeg adapter to ensure it reads the provided path and emits real RTP, not fixture-specific generated audio.
- Verify the receiver decodes actual WebRTC/Opus media and asserts content, not merely process exit or API status.
- Confirm ticket minting uses the REST endpoint and the session WebSocket consumes the returned one-time ticket.
- Search for ticket reuse, hard-coded tickets, disabled expiry checks, or test-only server admission branches.
- Verify `produce_channel_ids` is checked against the selected channel and that the negative role/channel test receives no audio.
- Confirm `-re` or equivalent pacing is present and elapsed-time assertions would fail burst transmission.
- Inspect cancellation for process-group termination, `Wait`, socket closure, and peer cleanup; check the test for orphan processes.
- Confirm ffmpeg stderr and all structured logs redact credentials and tickets.
- Ensure the old `/ws/signaling` ABC endpoint is not accidentally used by ct-play.

## Completion Gate

- [ ] WAV and MP3 BDD scenarios pass through the actual binary and real receiver.
- [ ] Both requested invocation forms are supported exactly.
- [ ] One-time ticket, scoped capability, and session WebSocket contracts are correctly enforced.
- [ ] Playback is real-time paced and exits cleanly at EOF.
- [ ] Missing/invalid files, missing ffmpeg, unauthorized channels, ticket failures, network errors, and cancellation are covered.
- [ ] No credential or ticket leakage is found.
- [ ] `task build:play`, `task test:unit:go`, focused full-process audio tests, `task test:integration`, and `task lint:go` pass.
