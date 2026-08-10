/**
 * Unit tests for reconnect policy (pure functions — no DOM/WebRTC).
 *
 * Run: pnpm --filter @crosstalk/broadcast test
 */
import { describe, expect, it } from "vitest";
import {
  DEFAULT_RECONNECT_POLICY,
  canAttemptReconnect,
  classifyWsClose,
  computeBackoffMs,
  nextAttemptAfterStable,
  terminalMessage,
} from "./reconnect-policy";

describe("computeBackoffMs", () => {
  const policy = {
    ...DEFAULT_RECONNECT_POLICY,
    initialMs: 1000,
    maxMs: 8000,
    factor: 2,
    jitterRatio: 0,
  };

  it("grows exponentially without jitter", () => {
    expect(computeBackoffMs(1, policy, () => 0.5)).toBe(1000);
    expect(computeBackoffMs(2, policy, () => 0.5)).toBe(2000);
    expect(computeBackoffMs(3, policy, () => 0.5)).toBe(4000);
    expect(computeBackoffMs(4, policy, () => 0.5)).toBe(8000);
    // capped
    expect(computeBackoffMs(5, policy, () => 0.5)).toBe(8000);
  });

  it("applies jitter within ±ratio", () => {
    const withJitter = { ...policy, jitterRatio: 0.2 };
    // random=1 → +20%; random=0 → -20%
    expect(computeBackoffMs(1, withJitter, () => 1)).toBe(1200);
    expect(computeBackoffMs(1, withJitter, () => 0)).toBe(800);
  });
});

describe("canAttemptReconnect", () => {
  it("allows attempts up to maxAttempts inclusive", () => {
    const p = { ...DEFAULT_RECONNECT_POLICY, maxAttempts: 3 };
    expect(canAttemptReconnect(1, p)).toBe(true);
    expect(canAttemptReconnect(3, p)).toBe(true);
    expect(canAttemptReconnect(4, p)).toBe(false);
    expect(canAttemptReconnect(0, p)).toBe(false);
  });
});

describe("nextAttemptAfterStable", () => {
  it("resets after stable play window", () => {
    const p = { ...DEFAULT_RECONNECT_POLICY, stablePlayMs: 10_000 };
    expect(nextAttemptAfterStable(10_000, 5, p)).toBe(0);
    expect(nextAttemptAfterStable(9_999, 5, p)).toBe(5);
  });
});

describe("classifyWsClose", () => {
  it("marks invalid/rotated token codes as non-retryable", () => {
    expect(classifyWsClose(4001).kind).toBe("invalid_token");
    expect(classifyWsClose(4003).kind).toBe("invalid_token");
    expect(classifyWsClose(1008).kind).toBe("invalid_token");
    expect(classifyWsClose(1000, "token expired").kind).toBe("invalid_token");
    expect(classifyWsClose(1000, "link rotated").kind).toBe("invalid_token");
  });

  it("marks session-ended codes as terminal", () => {
    expect(classifyWsClose(4000).kind).toBe("session_ended");
    expect(classifyWsClose(4002).kind).toBe("session_ended");
    expect(classifyWsClose(1000, "session ended").kind).toBe("session_ended");
    expect(classifyWsClose(1000, "broadcast stopped").kind).toBe("session_ended");
  });

  it("treats abnormal closes as retryable", () => {
    expect(classifyWsClose(1006).kind).toBe("retry");
    expect(classifyWsClose(1011).kind).toBe("retry");
    expect(classifyWsClose(1000, "").kind).toBe("retry");
  });

  it("can treat normal close as ended when opted in", () => {
    expect(classifyWsClose(1000, "", { treatNormalCloseAsEnded: true }).kind).toBe(
      "ended",
    );
  });
});

describe("terminalMessage", () => {
  it("returns truthful user-facing copy", () => {
    expect(terminalMessage("invalid_token")).toMatch(/invalid|expired/i);
    expect(terminalMessage("session_ended")).toMatch(/ended/i);
    expect(terminalMessage("max_retries")).toMatch(/reconnect/i);
    expect(terminalMessage("playback_failed")).toMatch(/playback/i);
  });
});
