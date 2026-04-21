# Testing

[← Back to Index](../index.md)

Three-tier testing strategy. Core philosophy: **prove real functionality, not mocks**.

Agent-driven development makes it easy to claim things work when they don't. The testing environment must be iron-clad — demonstrating actual behavior end-to-end.

---

## Sections

| Section | Description |
|---------|-------------|
| [Unit Tests](unit-tests.md) | Fast, isolated, mocks allowed |
| [Integration Tests](integration-tests.md) | Docker Compose, Playwright, real services |
| [E2E / Golden Tests](e2e-golden.md) | Actual audio through real hardware |

## Testing Pyramid

```
         ╱╲
        ╱E2E╲         Few, slow, high confidence
       ╱ Gold ╲        Real audio, real hardware
      ╱────────╲
     ╱Integration╲     Moderate count, real services
    ╱  Docker + PW  ╲   Playwright for web UI
   ╱────────────────╲
  ╱    Unit Tests     ╲  Many, fast, mocks OK
 ╱____________________╲
```

## Guiding Principles

1. **Mocks only at unit level** — integration and E2E tests use real services (real SQLite, real Pion, real PipeWire)
2. **Clean setup/teardown** — every test run starts from a known state, no leaked state between tests
3. **Reproducible** — tests must pass in CI and on any developer's machine (within Docker)
4. **Audio is verified** — E2E tests actually play and record audio, then compare waveforms
5. **No "it compiles therefore it works"** — every feature needs a test that exercises it for real
