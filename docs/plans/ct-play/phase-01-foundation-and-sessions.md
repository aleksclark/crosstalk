# Phase 1: CLI Foundation, Configuration, and Session Discovery

## Goal

Establish the `ct-play` executable, Cobra command tree, Viper configuration contract, and secure username/password REST authentication. This phase ends with a useful `ct-play session` command that lists only sessions assigned to the authenticated user. It comes first because every later selection and playback operation depends on consistent configuration, authentication, and API error handling.

## BDD Success Criteria

### Scenario: List assigned sessions using flags

- **Given** a real CrossTalk server with a translator assigned to two named sessions and not assigned to a third
- **When** the translator runs `ct-play --host <server> --username <user> --password <password> session`
- **Then** stdout contains the IDs and names of the two assigned sessions in deterministic order
- **And** stdout does not contain the unassigned session
- **And** the command exits zero.

### Scenario: Resolve configuration precedence

- **Given** conflicting host, username, password, session, and channel values in a config file, `CT_PLAY_*` environment variables, and explicit flags
- **When** any `ct-play` command resolves configuration
- **Then** explicit flags override environment values
- **And** environment values override config-file values
- **And** config-file values override built-in defaults
- **And** the resolved password is never emitted.

### Scenario: Use environment-only credentials

- **Given** valid `CT_PLAY_HOST`, `CT_PLAY_USERNAME`, and `CT_PLAY_PASSWORD` values
- **When** the user runs `ct-play session` without credential flags
- **Then** authentication succeeds and assigned sessions are listed.

### Scenario: Use an explicit configuration file

- **Given** a readable YAML, JSON, or TOML config containing valid credentials and selection keys
- **When** the user runs `ct-play --config <path> session`
- **Then** Viper loads the file and the command succeeds
- **And** a missing explicit file causes a non-zero exit with its path identified
- **And** an absent implicit default config does not cause failure.

### Scenario: Reject invalid credentials safely

- **Given** an existing username and an incorrect password
- **When** the user runs `ct-play session`
- **Then** the command exits non-zero with an authentication error
- **And** neither the submitted password nor returned token appears in stdout, stderr, or structured logs.

### Scenario: Report unreachable or malformed hosts

- **Given** an invalid URL or an unreachable server
- **When** the user runs `ct-play session`
- **Then** the command exits non-zero with a bounded, actionable network or URL error
- **And** it does not retry forever.

### Scenario: Expose stable CLI help

- **Given** the built `ct-play` binary
- **When** the user runs `ct-play --help` or `ct-play session --help`
- **Then** help documents the supported flags, `CT_PLAY_*` environment names, config search path, and credential-safety recommendation
- **And** the root command accepts at most one future audio-file positional argument without consuming `session` as a filename.

## Implementation Instructions

1. Add Cobra and Viper to `cli/go.mod` and `cli/go.sum`. Cobra/Viper wiring belongs under `cli/cmd/ct-play`; do not migrate unrelated CLI binaries.
2. Add `cli/cmd/ct-play/main.go` and command-construction files such as `root.go` and `session.go`:
   - `main` configures JSON `slog`, executes the root command with signal-aware context, maps errors to non-zero exit, and never prints secrets.
   - `newRootCommand` accepts output/error writers and injectable application services for unit tests.
   - Define persistent flags `--host`, `--username`, `--password`, `--session`, `--channel`, and `--config`.
   - Bind exact `CT_PLAY_*` environment names from the index contract with `AutomaticEnv` disabled or tightly scoped so unrelated variables cannot alter behavior.
   - Normalize hyphenated flags to underscore config keys deliberately rather than depending on implicit Viper replacement behavior.
3. Add a dependency-free configuration/domain contract in `cli/domain.go` only if it is broadly reusable; otherwise keep ct-play-specific value types in the command package. Validation must distinguish missing host, username, and password and must not include values in errors.
4. Create a dedicated HTTP dependency adapter, likely `cli/httpapi`, wrapping `net/http` rather than placing REST calls in Cobra handlers. Required operations:
   - `Login(ctx, host, username, password)` against `POST /api/auth/login`;
   - `ListSessions(ctx, accessToken)` against `GET /api/sessions`;
   - typed status/error decoding with bounded response bodies and context cancellation.
5. Decide before implementation whether to use the generated `client` module. If used, run `task api:generate:go`, add the local module requirement/replace, and verify the generated methods support current media-ticket bodies. If it remains stale or awkward, use the small `net/http` adapter and document why duplicate transport types are intentionally limited to ct-play.
6. Add the `session` subcommand:
   - Authenticate once per invocation.
   - Print `ID\tNAME` with stable sorting, preferably name then ID.
   - Print the header even for an empty result and an explicit stderr diagnostic or zero-row output that remains scriptable.
   - Do not show `broadcast_token` or other unrelated session fields.
7. Add `task build:play` to `Taskfile.yml`, producing `bin/ct-play`. Keep `task build:cli` behavior unchanged for compatibility.
8. Add command and adapter unit tests:
   - Cobra parsing and root/subcommand dispatch;
   - exact flag/env/file/default precedence for all five requested values;
   - malformed config and file-permission warning behavior;
   - HTTP status mapping, bounded bodies, context cancellation, and secret redaction;
   - stable table output.
9. After each logical change, run focused `go test` commands in `cli/`, then `task build:play`.

## End-to-End Test Plan

1. Extend or add a full-process CLI test harness under `test/` that starts the real `ct-server` against a fresh PostgreSQL database using the same setup as `test/run-e2e.sh` or `server/cmd/ct-server/integration_test.go`.
2. Through real admin REST APIs:
   - create a translator;
   - create three sessions;
   - assign only two sessions to that translator.
3. Launch the actual `bin/ct-play session` subprocess using:
   - flags for one run;
   - `CT_PLAY_*` environment values for another;
   - a permission-restricted temporary config file for another.
4. Assert exact stdout IDs/names and absence of the unassigned session. Assert stderr contains no password, access token, or refresh token.
5. Run with incorrect credentials and an unreachable host; assert bounded non-zero failures.
6. Run with config/env/flags intentionally conflicting and verify the selected server/account proves precedence through observable session output, not private Viper state.
7. Exact gates:
   - `task build:play`
   - `task test:unit:go`
   - the new full-process CLI test command established by this phase.

Unit tests may use `httptest.Server` for malformed-response and transport edge cases, but acceptance requires the full-process test against the real server and PostgreSQL.

## Anti-Cheating Audit

- Inspect `cli/cmd/ct-play` to ensure `session` invokes the real HTTP adapter rather than printing fixtures or embedded session data.
- Confirm tests launch `bin/ct-play`, not `Execute()` or private command functions only.
- Confirm the assigned-session assertion relies on server-side authorization and that the test creates an unassigned session to detect client-side filtering shortcuts.
- Search output, logging calls, errors, and test artifacts for password, access-token, and refresh-token values.
- Verify Viper is actually bound to every requested flag/environment/config key and that tests prove precedence behavior externally.
- Confirm HTTP failures are not swallowed into empty successful listings.
- Confirm no test-only environment path bypasses authentication or substitutes an in-memory session store.
- Verify no global Viper singleton leaks configuration between commands or tests; use an invocation-scoped Viper instance.

## Completion Gate

- [ ] All BDD scenarios pass through the actual binary where specified.
- [ ] Cobra and Viper dependencies are explicit and isolated to ct-play wiring.
- [ ] `ct-play session` lists only real assigned sessions with ID and name.
- [ ] All five requested configuration values have tested flag/env/config precedence.
- [ ] Credential and token redaction audits pass.
- [ ] `task build:play` succeeds.
- [ ] `task test:unit:go` and focused full-process session tests pass.
- [ ] `task lint:go` passes for the changed CLI packages.
- [ ] No generated artifacts are stale if the generated Go client is used.
