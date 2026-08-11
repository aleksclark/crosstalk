# ct-play Phased Implementation Plan

## Outcome

Completing this plan delivers a production-quality `ct-play` command that authenticates with a CrossTalk server, discovers the authenticated user's assigned sessions and their channels, and streams WAV or MP3 files in real time into an explicitly selected, server-authorized channel over the existing one-time media-ticket WebRTC path.

## Current-State Summary

- The CLI module contains `ct-client`, `ct-abc`, and `ct-netcfg`, but no `ct-play` command (`cli/cmd/`). Existing commands use the standard `flag` package; Cobra and Viper are not present in `cli/go.mod`.
- Username/password authentication, assigned-session listing, session channel listing, and one-time media-ticket issuance already exist in the server REST API (`server/api/handlers.go`, `server/api/media_auth.go`, `server/api/authz.go`, and `server/api/types.go`).
- The current generated Go client in `client/client.gen.go` does not reflect the current session-scoped media-ticket request and must be regenerated before reuse.
- `cli/pion/connection.go` can publish an Opus RTP track, but it hardcodes `/ws/signaling` and is designed for persistent ABC signaling. Session media uses `/api/sessions/{id}/ws?token=<one-time-ticket>` and needs a finite playback-oriented connection lifecycle.
- `cli/pion/audio.go` converts device PCM to Opus RTP through `ffmpeg`; no adapter currently decodes a WAV or MP3 file and paces it into WebRTC.
- Real PostgreSQL, HTTP, Pion, Opus mixing, browser listeners, and audio-frequency assertions already exist in `server/cmd/ct-server/*flow_test.go`, `test/run-e2e.sh`, and `test/playwright/specs/golden-audio.spec.ts`. No current test launches an actual file-playing CLI process.
- Several older `spec/` documents describe obsolete 24-hour tokens and signaling paths. Implementation must follow current server code and generated OpenAPI, then correct the affected spec sections in a separate spec-only commit.

## Scope Boundaries

### In scope

- A new `bin/ct-play` executable built from `cli/cmd/ct-play`.
- Cobra command structure and Viper-backed flags, environment variables, and config files.
- Configuration for host, username, password, session ID, and channel ID.
- `ct-play session` to list assigned session names and IDs.
- `ct-play channel` to list channel names and IDs for the resolved session.
- `ct-play <file.wav|file.mp3>` with session/channel supplied by flags, environment, or config.
- `ct-play --session SESSION_ID --channel CHANNEL_ID <file>` explicit selection.
- Real-time WAV/MP3 decode through the existing runtime `ffmpeg` dependency, one-time ticket minting, session WebSocket signaling, and Opus RTP publication.
- Clear authorization, selection, format, transport, and subprocess errors.
- Real full-process tests proving received audio content through the production media path.

### Out of scope

- Interactive menus or fuzzy selection.
- Playlist, looping, seeking, pause/resume, transcoding output files, microphone capture, or video.
- Persisting access/refresh tokens or passwords outside the user-selected config source.
- Adding a generic long-lived service-token system.
- Bypassing or broadening server media RBAC. A channel visible through REST may still be non-publishable for the authenticated role; `ct-play` must fail clearly if the issued ticket does not authorize the selected channel.
- Replacing the existing `ct-client` or `ct-abc` command frameworks with Cobra/Viper.

## Global Constraints

- Follow the standard package layout: dependency-free contracts in the CLI root package, external dependency adapters in dedicated subpackages, and concrete wiring in `cli/cmd/ct-play`. Subpackages must not import sibling subpackages.
- Add Cobra and Viper as explicit CLI-module dependencies. Do not assume they already exist.
- Configuration precedence is: explicit flag, `CT_PLAY_*` environment variable, config file, built-in default. Exact bindings:

  | Setting | Flag | Environment | Config key |
  |---|---|---|---|
  | Server base URL | `--host` | `CT_PLAY_HOST` | `host` |
  | Username | `--username` | `CT_PLAY_USERNAME` | `username` |
  | Password | `--password` | `CT_PLAY_PASSWORD` | `password` |
  | Session ID | `--session` | `CT_PLAY_SESSION_ID` | `session_id` |
  | Channel ID | `--channel` | `CT_PLAY_CHANNEL_ID` | `channel_id` |
  | Config path | `--config` | `CT_PLAY_CONFIG` | n/a |

- Viper searches `$XDG_CONFIG_HOME/crosstalk/ct-play.{yaml,yml,json,toml}` and `$HOME/.config/crosstalk/ct-play.*` when `--config` is absent. Missing implicit config is allowed; a missing explicit config is an error.
- `host` is an HTTP(S) server base URL, normalized without a trailing slash. Production TLS verification remains enabled.
- Password values must never appear in logs, error text, process diagnostics, or generated URLs. The required `--password` flag is supported, but help text should recommend `CT_PLAY_PASSWORD` or a permission-restricted config file. Warn when a password-bearing config file is group/world readable.
- Command data is written through Cobra's output/error writers for testability. Operational diagnostics use structured `log/slog` on stderr and must not contain credentials or media tickets.
- Session and channel IDs are authoritative. Human output is deterministic and tabular with at least `ID` and `NAME`; channel output also includes `TYPE` so users can understand likely role restrictions.
- `ct-play channel` requires a resolved session ID. Bare playback requires both session and channel to resolve from flags, environment, or config; otherwise it exits before minting a ticket and points to `ct-play session` or `ct-play channel`.
- Login access tokens are held in memory only. Media tickets are minted immediately before dialing, used once, never printed, and reminted on any pre-media retry.
- Playback must be paced in real time, stream rather than buffer the whole file, terminate subprocesses on cancellation, close WebRTC cleanly at EOF, and return a non-zero exit code on partial/failed playback.
- The server remains the authorization source of truth. After minting, verify that `produce_channel_ids` contains the selected channel; never treat a successful HTTP response with an empty capability set as successful playback.
- WAV and MP3 support is provided through `ffmpeg`; startup validates its presence and errors with an installation-oriented message if unavailable.
- Spec corrections, if needed, are committed separately from code per `AGENTS.md` and `agent_docs/spec-maintenance.md`.

## Phase Overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: CLI foundation, configuration, and session discovery](./phase-01-foundation-and-sessions.md) | Deliver Cobra/Viper configuration, secure login, and `ct-play session` | None |
| [Phase 2: Channel discovery and deterministic selection](./phase-02-channel-discovery.md) | Deliver `ct-play channel` and shared session/channel resolution | Phase 1 |
| [Phase 3: Session media transport and file playback](./phase-03-file-playback.md) | Stream WAV and MP3 files into an authorized selected channel | Phases 1-2 |
| [Phase 4: Full-process proof and release integration](./phase-04-e2e-and-release.md) | Prove the actual binary end to end and wire build, CI, and corrected docs | Phases 1-3 |

## Requirement Traceability

| Requested capability | Primary phase | Final evidence |
|---|---|---|
| Flags/env/config for host, username, password, session ID, channel ID | Phase 1, completed for selection in Phase 2 | Cobra/Viper precedence tests plus full-process commands using each source |
| `ct-play session` lists assigned session name and ID | Phase 1 | Real login and REST integration test against assigned/unassigned sessions |
| `ct-play channel` lists available channel name and ID | Phase 2 | Real assigned-session channel integration test and authorization failures |
| `ct-play some_file.mp3` | Phase 3 | MP3 full-process audio-frequency test with session/channel from environment or config |
| `ct-play --session ... --channel ... some_file.mp3` | Phase 3 | Explicit-flag WAV/MP3 full-process test through a real listener |

## Completion Rule

The plan is complete only when all four phase completion gates pass, the actual `bin/ct-play` process authenticates against a real PostgreSQL-backed server, lists only authorized sessions, lists channels for the selected session, publishes both WAV and MP3 fixtures through one-time media-ticket WebRTC signaling, and a real receiver decodes and identifies the expected audio. Unit-only subprocess mocks, hard-coded fixture output, direct mixer injection, or tests that bypass the CLI, REST, ticket, WebSocket, WebRTC, and Opus boundaries do not satisfy completion.
