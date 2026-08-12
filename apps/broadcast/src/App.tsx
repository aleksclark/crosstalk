import { useEffect, useId, useState, type CSSProperties, type ReactNode } from "react";
import {
  Button,
  DataState,
  Icon,
  Logo,
  Status,
  VUMeter,
  type StatusTone,
} from "@crosstalk/theme";
import { parseBroadcastParams } from "./params";
import { fetchBroadcastInfo, BroadcastApiError, type BroadcastInfo } from "./api";
import {
  useBroadcastListener,
  type ListenerConnectionState,
  type TerminalReason,
} from "./use-broadcast-listener";

type AppState =
  | { kind: "loading" }
  | { kind: "error"; message: string; code?: string }
  | { kind: "ready"; info: BroadcastInfo }
  | { kind: "listening"; info: BroadcastInfo };

export function App() {
  const { sessionId, token, debug } = parseBroadcastParams();
  const [appState, setAppState] = useState<AppState>({ kind: "loading" });

  useEffect(() => {
    if (!sessionId || !token) {
      setAppState({
        kind: "error",
        message: "Invalid broadcast link. Please check the URL.",
        code: "INVALID_TOKEN",
      });
      return;
    }

    let cancelled = false;
    fetchBroadcastInfo(sessionId, token)
      .then((info) => {
        if (!cancelled) setAppState({ kind: "ready", info });
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof BroadcastApiError) {
          setAppState({ kind: "error", message: err.message, code: err.code });
        } else {
          setAppState({
            kind: "error",
            message: "Failed to connect. Please try again.",
            code: "FETCH_ERROR",
          });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [sessionId, token]);

  if (appState.kind === "loading") {
    return <LoadingScreen />;
  }

  if (appState.kind === "error") {
    return <ErrorScreen message={appState.message} code={appState.code} />;
  }

  return (
    <ListenerView
      sessionId={sessionId!}
      token={token!}
      info={appState.info}
      listening={appState.kind === "listening"}
      onListen={() => setAppState({ kind: "listening", info: appState.info })}
      debug={debug}
    />
  );
}

/* -------------------------------------------------------------------------- */
/* Shell                                                                      */
/* -------------------------------------------------------------------------- */

const shellStyle: CSSProperties = {
  minHeight: "100svh",
  display: "flex",
  flexDirection: "column",
  background: "var(--house-bg-canvas)",
  color: "var(--house-text-primary)",
  paddingTop: "max(var(--house-space-4), env(safe-area-inset-top))",
  paddingRight: "max(var(--house-space-4), env(safe-area-inset-right))",
  paddingBottom: "max(var(--house-space-4), env(safe-area-inset-bottom))",
  paddingLeft: "max(var(--house-space-4), env(safe-area-inset-left))",
};

const instrumentStyle: CSSProperties = {
  width: "100%",
  maxWidth: 420,
  margin: "0 auto",
  flex: "1 1 auto",
  display: "flex",
  flexDirection: "column",
  gap: "var(--house-space-6)",
  justifyContent: "center",
  minHeight: 0,
};

function InstrumentShell({
  children,
  brand = true,
}: {
  children: ReactNode;
  brand?: boolean;
}) {
  return (
    <div style={shellStyle} data-surface="monitor-listen">
      <div style={instrumentStyle}>
        {brand ? (
          <header
            style={{
              display: "flex",
              alignItems: "center",
              gap: "var(--house-space-3)",
              borderBottom: "1px solid var(--house-rule-subtle)",
              paddingBottom: "var(--house-space-3)",
            }}
          >
            <Logo
              style={{ height: 28, width: "auto" }}
              className="broadcast-brand-logo"
            />
            <span className="house-type-eyebrow" style={{ color: "var(--house-text-tertiary)" }}>
              Broadcast
            </span>
          </header>
        ) : null}
        {children}
      </div>
    </div>
  );
}

function LoadingScreen() {
  return (
    <InstrumentShell>
      <div data-testid="broadcast-loading">
        <DataState
          kind="loading"
          title="Validating broadcast link"
          description="Checking that this listen link is still active."
        />
      </div>
    </InstrumentShell>
  );
}

function ErrorScreen({ message, code }: { message: string; code?: string }) {
  const ended = code === "SESSION_ENDED";
  const invalid = code === "INVALID_TOKEN";
  const title = ended
    ? "Broadcast ended"
    : invalid
      ? "Invalid or expired link"
      : "Unable to connect";
  const kind = invalid || ended ? "denied" : "error";

  return (
    <InstrumentShell>
      <div data-testid="broadcast-error">
        <DataState
          kind={kind}
          title={title}
          action={
            <p
              className="house-type-lede"
              data-testid="broadcast-error-message"
              style={{ margin: 0 }}
            >
              {message}
            </p>
          }
        />
      </div>
    </InstrumentShell>
  );
}

/* -------------------------------------------------------------------------- */
/* Listener                                                                   */
/* -------------------------------------------------------------------------- */

interface ListenerViewProps {
  sessionId: string;
  token: string;
  info: BroadcastInfo;
  listening: boolean;
  onListen: () => void;
  debug: boolean;
}

type StatusPresentation = {
  text: string;
  sentence: string;
  tone: StatusTone;
  testId: string;
  live?: boolean;
  spinning?: boolean;
};

function statusPresentation(
  listening: boolean,
  state: ListenerConnectionState,
  paused: boolean,
  reconnectAttempt: number,
  maxAttempts: number,
  terminalReason: TerminalReason | null,
  error: string | null,
): StatusPresentation | null {
  if (!listening) {
    return {
      text: "Ready",
      sentence: "Tap Listen when you are ready to hear the live mix.",
      tone: "neutral",
      testId: "status-ready",
    };
  }

  switch (state) {
    case "playing":
      if (paused) {
        return {
          text: "Paused",
          sentence: "Playback is paused. Resume to continue listening.",
          tone: "neutral",
          testId: "status-paused",
        };
      }
      return {
        text: "LIVE",
        sentence: "Receiving the live broadcast mix.",
        tone: "danger",
        testId: "status-live",
        live: true,
      };
    case "connecting":
      return {
        text: "Connecting",
        sentence: "Opening a secure listen connection…",
        tone: "warning",
        testId: "status-warn",
        spinning: true,
      };
    case "reconnecting":
      return {
        text: "Reconnecting",
        sentence:
          reconnectAttempt > 0
            ? `Connection interrupted. Reconnecting (attempt ${reconnectAttempt} of ${maxAttempts})…`
            : "Connection interrupted. Reconnecting…",
        tone: "warning",
        testId: "status-warn",
        spinning: true,
      };
    case "ended":
      return {
        text: "Ended",
        sentence: error ?? "This broadcast has ended.",
        tone: "neutral",
        testId: "status-ended",
      };
    case "error": {
      if (terminalReason === "invalid_token") {
        return {
          text: "Invalid link",
          sentence: error ?? "This listen link is no longer valid.",
          tone: "danger",
          testId: "status-error",
        };
      }
      if (terminalReason === "max_retries") {
        return {
          text: "Connection lost",
          sentence:
            error ??
            `Could not reconnect after ${maxAttempts} attempts. Check your network and try again.`,
          tone: "danger",
          testId: "status-error",
        };
      }
      if (terminalReason === "playback_failed") {
        return {
          text: "Playback blocked",
          sentence:
            error ??
            "The browser blocked audio playback. Tap Try again after interacting with the page.",
          tone: "warning",
          testId: "status-error",
        };
      }
      return {
        text: "Disconnected",
        sentence: error ?? "The listen connection failed.",
        tone: "danger",
        testId: "status-error",
      };
    }
    default:
      return {
        text: "Starting",
        sentence: "Preparing the listener…",
        tone: "info",
        testId: "status-warn",
        spinning: true,
      };
  }
}

function canRetry(
  state: ListenerConnectionState,
  terminalReason: TerminalReason | null,
): boolean {
  if (state !== "error") return false;
  // Terminal invalid/ended never offer retry.
  if (terminalReason === "invalid_token" || terminalReason === "session_ended") {
    return false;
  }
  return true;
}

function ListenerView({
  sessionId,
  token,
  info,
  listening,
  onListen,
  debug,
}: ListenerViewProps) {
  const broadcast = useBroadcastListener({
    sessionId,
    token,
    info,
    enabled: listening,
  });

  const status = statusPresentation(
    listening,
    broadcast.state,
    broadcast.paused,
    broadcast.reconnectAttempt,
    broadcast.maxReconnectAttempts,
    broadcast.terminalReason,
    broadcast.error,
  );

  const showControls =
    listening &&
    broadcast.state !== "ended" &&
    !(broadcast.state === "error" && broadcast.terminalReason === "invalid_token") &&
    !(broadcast.state === "error" && broadcast.terminalReason === "session_ended");

  const showRetry = canRetry(broadcast.state, broadcast.terminalReason);
  const volumeId = useId();
  const volumePct = Math.round(broadcast.volume * 100);

  return (
    <InstrumentShell>
      <section
        aria-labelledby="broadcast-session-title"
        style={{ display: "flex", flexDirection: "column", gap: "var(--house-space-4)" }}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--house-space-2)" }}>
          <p className="house-type-eyebrow" style={{ margin: 0, color: "var(--house-text-tertiary)" }}>
            Live listen
          </p>
          <h1
            id="broadcast-session-title"
            className="house-type-title"
            style={{ margin: 0 }}
          >
            {info.session_name}
          </h1>
          {status ? (
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                alignItems: "flex-start",
                gap: "var(--house-space-2)",
              }}
            >
              <StatusBadge presentation={status} />
              <p
                className="house-type-lede"
                style={{ margin: 0 }}
                role="status"
                aria-live="polite"
                data-testid="listener-status-sentence"
              >
                {status.sentence}
              </p>
            </div>
          ) : null}
        </div>

        {/* Primary action region */}
        {!listening ? (
          <Button
            variant="primary"
            onClick={onListen}
            data-testid="listen-button"
            icon="audio"
            style={{
              width: "100%",
              minHeight: "var(--house-target-mobile)",
              fontSize: "var(--house-type-section)",
              fontWeight: 600,
            }}
          >
            Listen
          </Button>
        ) : showControls ? (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              gap: "var(--house-space-4)",
              paddingTop: "var(--house-space-2)",
              borderTop: "1px solid var(--house-rule-subtle)",
            }}
          >
            <Button
              variant="secondary"
              onClick={broadcast.togglePause}
              data-testid="pause-button"
              icon={broadcast.paused ? "play" : "pause"}
              style={{
                width: "100%",
                minHeight: "var(--house-target-mobile)",
                fontSize: "var(--house-type-section)",
              }}
            >
              {broadcast.paused ? "Resume" : "Pause"}
            </Button>

            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "var(--house-space-2)",
              }}
            >
              <label
                htmlFor={volumeId}
                className="house-type-label"
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  color: "var(--house-text-secondary)",
                }}
              >
                <span>Volume</span>
                <span
                  className="house-type-meta"
                  style={{ color: "var(--house-text-tertiary)" }}
                  aria-hidden
                >
                  {volumePct}%
                </span>
              </label>
              <input
                id={volumeId}
                type="range"
                min={0}
                max={1}
                step={0.01}
                value={broadcast.volume}
                onChange={(e) => broadcast.setVolume(parseFloat(e.target.value))}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={volumePct}
                aria-valuetext={`${volumePct} percent`}
                className="broadcast-volume"
                data-testid="volume-slider"
              />
            </div>

            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "var(--house-space-2)",
              }}
              aria-label="Incoming signal level"
            >
              <span className="house-type-label" style={{ color: "var(--house-text-tertiary)" }}>
                Signal
              </span>
              <VUMeter level={broadcast.level} showValue={false} />
            </div>

            {broadcast.listenerCount != null ? (
              <p
                className="house-type-meta"
                style={{
                  margin: 0,
                  display: "inline-flex",
                  alignItems: "center",
                  gap: "var(--house-space-2)",
                }}
                data-testid="listener-count"
              >
                <Icon name="user" size="compact" />
                {broadcast.listenerCount} listener
                {broadcast.listenerCount !== 1 ? "s" : ""}
              </p>
            ) : null}
          </div>
        ) : null}

        {/* Terminal / reconnect messaging (keeps legacy test hooks) */}
        {broadcast.error ? (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              gap: "var(--house-space-3)",
              paddingTop: "var(--house-space-2)",
              borderTop: "1px solid var(--house-rule-subtle)",
            }}
            data-testid="listener-status-message"
          >
            <p
              className="house-type-body"
              style={{
                margin: 0,
                color:
                  broadcast.state === "reconnecting"
                    ? "var(--house-status-warning)"
                    : broadcast.state === "ended"
                      ? "var(--house-text-secondary)"
                      : "var(--house-status-danger)",
              }}
              data-testid="broadcast-error-message"
              role="status"
            >
              {broadcast.error}
            </p>
            {showRetry ? (
              <Button
                variant="secondary"
                onClick={broadcast.retry}
                data-testid="retry-button"
                icon="refresh"
                style={{ minHeight: "var(--house-target-mobile)", alignSelf: "flex-start" }}
              >
                Try again
              </Button>
            ) : null}
          </div>
        ) : null}

        {broadcast.state === "ended" && !broadcast.error ? (
          <p
            className="house-type-body"
            style={{ margin: 0, color: "var(--house-text-secondary)" }}
            data-testid="broadcast-ended-message"
            role="status"
          >
            This broadcast has ended
          </p>
        ) : null}

        {debug && listening ? (
          <DebugPanel
            debugInfo={broadcast.debugInfo}
            state={broadcast.state}
            reconnectAttempt={broadcast.reconnectAttempt}
            maxAttempts={broadcast.maxReconnectAttempts}
          />
        ) : null}
      </section>
    </InstrumentShell>
  );
}

/* -------------------------------------------------------------------------- */
/* Status badge                                                               */
/* -------------------------------------------------------------------------- */

function StatusBadge({ presentation }: { presentation: StatusPresentation }) {
  if (presentation.live) {
    return (
      <span
        className="broadcast-live-badge"
        data-testid="status-live"
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: "var(--house-space-2)",
          border: "1px solid var(--house-status-danger)",
          borderRadius: "var(--house-radius-sm)",
          padding: "var(--house-space-1) var(--house-space-2)",
          font: "500 var(--house-type-metadata) / 1.3 var(--house-font-technical)",
          color: "var(--house-status-danger)",
          background: "var(--house-status-danger-bg)",
        }}
      >
        <span
          className="broadcast-live-dot"
          aria-hidden
          style={{
            position: "relative",
            display: "inline-flex",
            width: 10,
            height: 10,
          }}
        >
          <span className="broadcast-live-ping" />
          <span
            style={{
              position: "relative",
              display: "inline-flex",
              width: 10,
              height: 10,
              borderRadius: "var(--house-radius-pill)",
              background: "var(--house-status-danger)",
            }}
          />
        </span>
        LIVE
      </span>
    );
  }

  return (
    <span data-testid={presentation.testId}>
      <Status
        tone={presentation.tone}
        hideIcon={presentation.spinning}
        style={presentation.spinning ? { gap: "var(--house-space-2)" } : undefined}
      >
        {presentation.spinning ? (
          <span
            className="broadcast-spin"
            aria-hidden
            style={{
              width: 12,
              height: 12,
              border: "2px solid currentColor",
              borderTopColor: "transparent",
              borderRadius: "var(--house-radius-pill)",
              display: "inline-block",
            }}
          />
        ) : null}
        {presentation.text}
      </Status>
    </span>
  );
}

/* -------------------------------------------------------------------------- */
/* Debug disclosure                                                           */
/* -------------------------------------------------------------------------- */

function DebugPanel({
  debugInfo,
  state,
  reconnectAttempt,
  maxAttempts,
}: {
  debugInfo: {
    iceState: string;
    signalingState: string;
    candidates: string[];
    localSdp: string | null;
    remoteSdp: string | null;
  };
  state: ListenerConnectionState;
  reconnectAttempt: number;
  maxAttempts: number;
}) {
  return (
    <details
      className="broadcast-debug"
      open
      style={{
        borderTop: "1px solid var(--house-rule-subtle)",
        paddingTop: "var(--house-space-3)",
      }}
    >
      <summary
        className="house-type-label"
        style={{
          cursor: "pointer",
          color: "var(--house-text-tertiary)",
          listStyle: "none",
          display: "flex",
          alignItems: "center",
          gap: "var(--house-space-2)",
          minHeight: "var(--house-target-mobile)",
        }}
      >
        <Icon name="debug" size="compact" />
        Debug
      </summary>
      <div
        className="house-type-code"
        style={{
          marginTop: "var(--house-space-3)",
          display: "flex",
          flexDirection: "column",
          gap: "var(--house-space-2)",
          color: "var(--house-text-secondary)",
          overflowX: "auto",
        }}
      >
        <div>
          <span style={{ color: "var(--house-text-tertiary)" }}>State:</span>{" "}
          <span data-testid="debug-state" style={{ color: "var(--house-status-warning)" }}>
            {state}
          </span>
          {reconnectAttempt > 0 ? (
            <span style={{ color: "var(--house-text-tertiary)" }}>
              {" "}
              (attempt {reconnectAttempt}/{maxAttempts})
            </span>
          ) : null}
        </div>
        <div>
          <span style={{ color: "var(--house-text-tertiary)" }}>ICE:</span>{" "}
          <span style={{ color: "var(--house-status-ok)" }}>{debugInfo.iceState}</span>
        </div>
        <div>
          <span style={{ color: "var(--house-text-tertiary)" }}>Signaling:</span>{" "}
          <span style={{ color: "var(--house-status-info)" }}>{debugInfo.signalingState}</span>
        </div>
        <div>
          <span style={{ color: "var(--house-text-tertiary)" }}>
            Candidates ({debugInfo.candidates.length}):
          </span>
          {debugInfo.candidates.length > 0 ? (
            <pre
              style={{
                margin: "var(--house-space-1) 0 0",
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
                maxHeight: 128,
                overflowY: "auto",
                color: "var(--house-text-tertiary)",
              }}
            >
              {debugInfo.candidates.join("\n")}
            </pre>
          ) : null}
        </div>
        {debugInfo.remoteSdp ? (
          <details>
            <summary style={{ cursor: "pointer", color: "var(--house-text-tertiary)" }}>
              Remote SDP
            </summary>
            <pre
              style={{
                margin: "var(--house-space-1) 0 0",
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
                maxHeight: 160,
                overflowY: "auto",
                color: "var(--house-text-tertiary)",
              }}
            >
              {debugInfo.remoteSdp}
            </pre>
          </details>
        ) : null}
        {debugInfo.localSdp ? (
          <details>
            <summary style={{ cursor: "pointer", color: "var(--house-text-tertiary)" }}>
              Local SDP
            </summary>
            <pre
              style={{
                margin: "var(--house-space-1) 0 0",
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
                maxHeight: 160,
                overflowY: "auto",
                color: "var(--house-text-tertiary)",
              }}
            >
              {debugInfo.localSdp}
            </pre>
          </details>
        ) : null}
      </div>
    </details>
  );
}
