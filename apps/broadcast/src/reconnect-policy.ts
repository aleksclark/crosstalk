/**
 * Pure reconnect policy for the broadcast listener.
 *
 * Bounded exponential backoff with full jitter. Terminal (non-retryable)
 * failures short-circuit the loop so invalid/rotated links and ended sessions
 * never spin forever.
 */

export type ListenerConnectionState =
  | "idle"
  | "connecting"
  | "playing"
  | "reconnecting"
  | "ended"
  | "error";

/** Why the listener stopped and will not retry. */
export type TerminalReason =
  | "invalid_token"
  | "session_ended"
  | "playback_failed"
  | "max_retries"
  | "connection_failed";

export interface ReconnectPolicyOptions {
  /** Base delay for attempt 1 (ms). Default 1000. */
  initialMs: number;
  /** Cap on delay before jitter (ms). Default 30_000. */
  maxMs: number;
  /** Hard stop after this many reconnect attempts. Default 8. */
  maxAttempts: number;
  /** After playing this long, attempt counter resets. Default 10_000. */
  stablePlayMs: number;
  /** Exponential factor. Default 2. */
  factor: number;
  /** Jitter as a fraction of the base delay (0–1). Default 0.2 → ±20%. */
  jitterRatio: number;
}

export const DEFAULT_RECONNECT_POLICY: ReconnectPolicyOptions = {
  initialMs: 1_000,
  maxMs: 30_000,
  maxAttempts: 8,
  stablePlayMs: 10_000,
  factor: 2,
  jitterRatio: 0.2,
};

/**
 * WebSocket close codes treated as permanent auth/link failures.
 * 4001/4003 are the SPA's historical contract; 4401/4403 are HTTP-ish variants
 * some gateways emit. 1008 = policy violation (often invalid token).
 */
export const INVALID_TOKEN_CLOSE_CODES = new Set([4001, 4003, 4401, 4403, 1008]);

/** Close codes that mean the broadcast/session is over — do not reconnect. */
export const SESSION_ENDED_CLOSE_CODES = new Set([
  4000, // application: session ended
  4002, // application: broadcast stopped
  1001, // going away (server shutting down the session peer)
]);

export type CloseClass =
  | { kind: "retry" }
  | { kind: "invalid_token" }
  | { kind: "session_ended" }
  | { kind: "ended" };

/**
 * Classify a WebSocket close for reconnect decisions.
 * Normal 1000 with an "ended"/"stopped" reason is terminal; bare 1000 is retryable
 * only when the peer dropped unexpectedly mid-session (caller may override).
 */
export function classifyWsClose(
  code: number,
  reason = "",
  opts?: { treatNormalCloseAsEnded?: boolean },
): CloseClass {
  const r = reason.toLowerCase();

  if (INVALID_TOKEN_CLOSE_CODES.has(code)) {
    return { kind: "invalid_token" };
  }
  if (SESSION_ENDED_CLOSE_CODES.has(code)) {
    return { kind: "session_ended" };
  }
  if (
    r.includes("invalid") ||
    r.includes("expired") ||
    r.includes("forbidden") ||
    r.includes("unauthorized") ||
    r.includes("rotated")
  ) {
    return { kind: "invalid_token" };
  }
  if (
    r.includes("session ended") ||
    r.includes("session_ended") ||
    r.includes("broadcast ended") ||
    r.includes("broadcast stopped") ||
    r.includes("ended")
  ) {
    return { kind: "session_ended" };
  }

  // Clean server close with no reason — often session teardown.
  if (code === 1000 && (opts?.treatNormalCloseAsEnded || r.includes("normal"))) {
    return { kind: "ended" };
  }

  // 1000/1005/1006 and other transient codes → retry.
  return { kind: "retry" };
}

export function terminalMessage(reason: TerminalReason): string {
  switch (reason) {
    case "invalid_token":
      return "Invalid or expired broadcast link";
    case "session_ended":
      return "This broadcast has ended";
    case "playback_failed":
      return "Playback failed";
    case "max_retries":
      return "Could not reconnect after multiple attempts";
    case "connection_failed":
      return "Connection failed";
    default:
      return "Unable to connect";
  }
}

/**
 * Exponential backoff with full-ish jitter.
 * attempt is 1-based (first reconnect = 1).
 *
 * delay = min(max, initial * factor^(attempt-1)) * (1 + jitter * (2*rand - 1))
 */
export function computeBackoffMs(
  attempt: number,
  policy: ReconnectPolicyOptions = DEFAULT_RECONNECT_POLICY,
  random: () => number = Math.random,
): number {
  const a = Math.max(1, Math.floor(attempt));
  const base = Math.min(
    policy.maxMs,
    policy.initialMs * Math.pow(policy.factor, a - 1),
  );
  const jitterSpan = policy.jitterRatio * base;
  const jitter = (random() * 2 - 1) * jitterSpan;
  return Math.max(0, Math.round(base + jitter));
}

/** Whether another reconnect is allowed for this attempt count (1-based next attempt). */
export function canAttemptReconnect(
  nextAttempt: number,
  policy: ReconnectPolicyOptions = DEFAULT_RECONNECT_POLICY,
): boolean {
  return nextAttempt >= 1 && nextAttempt <= policy.maxAttempts;
}

/**
 * After stable play, reset the attempt counter so a later blip gets a full budget.
 */
export function nextAttemptAfterStable(
  playedMs: number,
  currentAttempt: number,
  policy: ReconnectPolicyOptions = DEFAULT_RECONNECT_POLICY,
): number {
  if (playedMs >= policy.stablePlayMs) {
    return 0;
  }
  return currentAttempt;
}
