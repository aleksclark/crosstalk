# K2B audio-control evidence

Date: 2026-08-11
Branch: feature/k2b-audio-control
Base: production-integration @ 2b627f80e38aa0369dc1e7fb28e96ba7e934fb80

## LIVE GATE BLOCKED

K2B board SSH probe failed:

```bash
timeout 3 bash -c 'echo >/dev/tcp/192.168.0.109/22' || echo UNREACHABLE
# → UNREACHABLE
```

No live deploy, mixer readback, or on-device before/after metrics were collected.
Do **not** treat offline unit/integration success as live pass.

## Offline proof (credential-free)

| Gate | Result |
|------|--------|
| `task test:unit:go` | PASS |
| `task test:unit:ts` | PASS |
| `task test:integration` | PASS (incl. ABC audio control WebRTC suite) |
| `task test:e2e` | PASS (22 Playwright tests, incl. ABC audio controls + golden audio) |
| `task lint:go` | PASS (after narrow lint fixes) |
| `task lint:web` | PASS (0 errors) |
| `task build:server` | PASS |
| `task build:abc:arm64` | PASS |
| `task api:validate` | PASS |
| `git diff --check` / range | PASS |
| `task generate:proto:v2` | clean (no drift) |

### Integration audio-control coverage (server/cmd/ct-server)

- Capabilities → command → applied report
- Offline PUT then connect reconcile
- Heartbeat convergence
- Duplicate request_id idempotency
- Stale report ignored
- Wrong ABC cannot mutate other
- Malformed/oversized control rejected (16KiB cap)
- Disconnect/reconnect generation race

### E2E admin SPA audio controls

data-testids exercised: `abc-audio-controls`, volume/mute/gain, revision badges,
save, conflict, offline, unsupported, fetch error, translator denied.

## Binary identity (local build only)

```
bin/ct-abc-arm64 sha256:
ce4aaa2127aa1229d75432d3eaac0a42863ec366ee0749c8c48bb4b0eff56c10
```

On-device binary identity: N/A (board unreachable).

## Security notes (pragmatic pass)

- REST admin-only via `requireRole(admin)`; audit actor from JWT only
- No shell execution in audioctl (direct argv + shell-name reject)
- Mixer names not accepted from API body
- Reports bound to authenticated ABC peer
- Control protobuf size limited (`MaxControlMessageBytes`)
- Error logging redacts long/board stderr via `safeErrCode`
- Revision + request_id idempotency
- Managed marker `StateDirectory/audio-managed` after first managed apply

## Residual

Re-run Phase 8 live matrix when K2B `192.168.0.109` (MAC `60:48:9c:41:b3:e4`) is reachable.
