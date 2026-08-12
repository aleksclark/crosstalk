import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { createApiClient, type components } from "@crosstalk/api-client";
import { BroadcastShare, SessionAudioManager } from "@crosstalk/session-audio";
import {
  Button,
  CopyableId,
  DataState,
  Field,
  PageHeader,
  Status,
  VUMeter,
  useAudioLevel,
  type StatusTone,
} from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";
import { useWebRTC, type ConnectFailureKind, type ConnectPhase } from "../hooks/useWebRTC";
import { WebRTCDebugPanel } from "../components/WebRTCDebugPanel";
import { OperateShell } from "../components/OperateShell";

type Session = components["schemas"]["SessionOut"];

type SessionLoadState =
  | { kind: "loading" }
  | { kind: "ready"; session: Session }
  | { kind: "not-found"; message: string }
  | { kind: "denied"; message: string }
  | { kind: "error"; message: string };

type MicPermission = "unknown" | "prompt" | "granted" | "denied";

export function SessionConnectPage() {
  const { id: sessionId } = useParams<{ id: string }>();
  const { getToken, logout, user } = useAuth();
  const navigate = useNavigate();

  const [sessionState, setSessionState] = useState<SessionLoadState>({ kind: "loading" });
  const [audioDevices, setAudioDevices] = useState<MediaDeviceInfo[]>([]);
  const [selectedDevice, setSelectedDevice] = useState<string>("");
  const [micPermission, setMicPermission] = useState<MicPermission>("unknown");
  const [devicesError, setDevicesError] = useState<string | null>(null);
  const [broadcastToken, setBroadcastToken] = useState<string | null>(null);
  const [enumerateRequested, setEnumerateRequested] = useState(false);

  const token = getToken();

  const audioClient = useMemo(
    () => (token ? createApiClient({ baseUrl: window.location.origin, token }) : null),
    [token],
  );

  // Fetch session object — name first; distinguish denied / not-found / network.
  useEffect(() => {
    if (!sessionId || !audioClient) return;
    const ac = new AbortController();
    setSessionState({ kind: "loading" });

    void (async () => {
      try {
        const { data, error, response } = await audioClient.GET("/api/sessions/{id}", {
          params: { path: { id: sessionId } },
          signal: ac.signal,
        });
        if (ac.signal.aborted) return;

        if (error || !response.ok || !data) {
          const status = error?.status ?? response.status;
          const detail = error?.detail ?? error?.title ?? "Failed to load session";
          if (status === 403 || status === 401) {
            setSessionState({ kind: "denied", message: detail });
            return;
          }
          if (status === 404) {
            setSessionState({ kind: "not-found", message: detail || "Session not found" });
            return;
          }
          setSessionState({ kind: "error", message: detail });
          return;
        }
        setSessionState({ kind: "ready", session: data });
      } catch (err) {
        if (ac.signal.aborted) return;
        const message = err instanceof Error ? err.message : "Network error loading session";
        setSessionState({ kind: "error", message });
      }
    })();

    return () => ac.abort();
  }, [sessionId, audioClient]);

  // Broadcast token for share panel (secondary).
  useEffect(() => {
    if (!sessionId || !audioClient) return;
    const ac = new AbortController();
    audioClient
      .GET("/api/sessions/{id}/broadcast-url", {
        params: { path: { id: sessionId } },
        signal: ac.signal,
      })
      .then(({ data }) => {
        if (!ac.signal.aborted) setBroadcastToken(data?.broadcast_token ?? null);
      })
      .catch(() => {
        if (!ac.signal.aborted) setBroadcastToken(null);
      });
    return () => ac.abort();
  }, [sessionId, audioClient]);

  // Optional SFU routing overrides via ?produce=&listen= (deep links / e2e).
  // Presence of empty produce/listen remains distinct from omission.
  const search = new URLSearchParams(window.location.search);
  const produce = search.get("produce") ?? undefined;
  const listen = search.get("listen") ?? "";

  const webrtc = useWebRTC({
    sessionId: sessionId ?? "",
    token: token ?? "",
    audioDeviceId: selectedDevice || undefined,
    produce,
    listen,
  });

  const isConnected = webrtc.connectionState === "connected";
  const isConnecting =
    webrtc.phase === "ticket-mint" ||
    webrtc.phase === "permission" ||
    webrtc.phase === "signaling" ||
    webrtc.connectionState === "connecting";

  // Mic permission + device enumeration — only after explicit user request
  // (or after connect already granted). Explains the permission prompt.
  const refreshDevices = useCallback(async () => {
    setDevicesError(null);
    setEnumerateRequested(true);
    try {
      if (navigator.permissions?.query) {
        try {
          const result = await navigator.permissions.query({
            name: "microphone" as PermissionName,
          });
          setMicPermission(result.state as MicPermission);
          result.onchange = () => setMicPermission(result.state as MicPermission);
        } catch {
          // permissions.query may reject microphone on some browsers
        }
      }

      // getUserMedia is required for labels; stop tracks immediately.
      const tempStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      tempStream.getTracks().forEach((t) => t.stop());
      setMicPermission("granted");

      const devices = await navigator.mediaDevices.enumerateDevices();
      const audioInputs = devices.filter((d) => d.kind === "audioinput");
      setAudioDevices(audioInputs);
      if (audioInputs.length === 0) {
        setDevicesError("No microphones found on this device.");
      } else if (!selectedDevice || !audioInputs.some((d) => d.deviceId === selectedDevice)) {
        setSelectedDevice(audioInputs[0]!.deviceId);
      }
    } catch (err) {
      const name = err instanceof DOMException ? err.name : "";
      if (name === "NotAllowedError" || name === "PermissionDeniedError") {
        setMicPermission("denied");
        setDevicesError("Microphone permission denied. Allow access in the browser, then try again.");
      } else if (name === "NotFoundError" || name === "DevicesNotFoundError") {
        setMicPermission("granted");
        setDevicesError("No microphone found.");
      } else {
        setDevicesError(err instanceof Error ? err.message : "Could not access microphones");
      }
    }
  }, [selectedDevice]);

  // After a successful connect, keep device list current if we never enumerated.
  useEffect(() => {
    if (isConnected && audioDevices.length === 0 && !enumerateRequested) {
      void refreshDevices();
    }
  }, [isConnected, audioDevices.length, enumerateRequested, refreshDevices]);

  const handleConnect = async () => {
    try {
      // Ensure devices/permission are known before mint when possible.
      if (!enumerateRequested && audioDevices.length === 0) {
        await refreshDevices();
      }
      await webrtc.connect();
    } catch (err) {
      console.error("connect failed", err);
    }
  };

  const handleDisconnect = () => {
    webrtc.disconnect();
  };

  const handleReconnect = async () => {
    webrtc.disconnect();
    // Brief tick so tracks release before re-minting.
    await new Promise((r) => setTimeout(r, 50));
    await handleConnect();
  };

  // Mic input level (pre-transmit / pre-mute semantics via useAudioLevel).
  const inputLevel = useAudioLevel(webrtc.localStream);

  const handleLogout = () => {
    webrtc.disconnect();
    logout();
    navigate("/login", { replace: true });
  };

  const connectionStrip = (
    <ConnectionStrip
      audioDevices={audioDevices}
      selectedDevice={selectedDevice}
      onSelectDevice={setSelectedDevice}
      micPermission={micPermission}
      devicesError={devicesError}
      onPrepareMic={() => void refreshDevices()}
      enumerateRequested={enumerateRequested}
      isConnected={isConnected}
      isConnecting={isConnecting}
      phase={webrtc.phase}
      lastError={webrtc.lastError}
      isMuted={webrtc.isMuted}
      inputLevel={inputLevel}
      onConnect={() => void handleConnect()}
      onDisconnect={handleDisconnect}
      onReconnect={() => void handleReconnect()}
      onToggleMute={webrtc.toggleMute}
      connectionState={webrtc.connectionState}
    />
  );

  if (sessionState.kind === "loading") {
    return (
      <OperateShell
        username={user?.username}
        scope="Loading session"
        backTo="/"
        backLabel="Back to Sessions"
        onLogout={handleLogout}
      >
        <DataState kind="loading" title="Loading session" description="Fetching session details." />
      </OperateShell>
    );
  }

  if (sessionState.kind === "denied") {
    return (
      <OperateShell
        username={user?.username}
        scope="Access denied"
        backTo="/"
        backLabel="Back to Sessions"
        onLogout={handleLogout}
      >
        <DataState
          kind="denied"
          title="Session not assigned to you"
          description={sessionState.message}
          action={
            <Button variant="secondary" onClick={() => navigate("/")}>
              Back to sessions
            </Button>
          }
        />
      </OperateShell>
    );
  }

  if (sessionState.kind === "not-found") {
    return (
      <OperateShell
        username={user?.username}
        scope="Not found"
        backTo="/"
        backLabel="Back to Sessions"
        onLogout={handleLogout}
      >
        <DataState
          kind="empty"
          title="Session not found"
          description={sessionState.message}
          action={
            <Button variant="secondary" onClick={() => navigate("/")}>
              Back to sessions
            </Button>
          }
        />
      </OperateShell>
    );
  }

  if (sessionState.kind === "error") {
    return (
      <OperateShell
        username={user?.username}
        scope="Error"
        backTo="/"
        backLabel="Back to Sessions"
        onLogout={handleLogout}
      >
        <DataState
          kind="error"
          title="Could not load session"
          description={sessionState.message}
          action={
            <Button variant="primary" icon="refresh" onClick={() => window.location.reload()}>
              Retry
            </Button>
          }
        />
      </OperateShell>
    );
  }

  const session = sessionState.session;

  return (
    <OperateShell
      username={user?.username}
      scope={session.name}
      backTo="/"
      backLabel="Back to Sessions"
      onLogout={handleLogout}
      strip={connectionStrip}
    >
      <PageHeader
        eyebrow="Operate"
        title={session.name}
        lede={session.description || "Connect your microphone and manage session audio."}
        meta={
          <>
            <span>
              ID <CopyableId value={session.id} />
            </span>
            {session.updated_at ? (
              <span>Updated {new Date(session.updated_at).toLocaleString()}</span>
            ) : null}
          </>
        }
      />

      {/* Mobile: connection strip already sticky above; workspace stacks below. */}
      <section
        aria-labelledby="session-audio-heading"
        style={{
          marginBottom: "var(--house-space-6)",
          borderTop: "1px solid var(--house-rule-subtle)",
          paddingTop: "var(--house-space-5)",
        }}
      >
        <h2 id="session-audio-heading" className="house-type-section" style={{ margin: "0 0 var(--house-space-4)" }}>
          Session Audio
        </h2>
        {audioClient && sessionId && token ? (
          <SessionAudioManager client={audioClient} token={token} sessionId={sessionId} />
        ) : (
          <p className="house-type-lede" style={{ color: "var(--house-text-tertiary)" }}>
            Sign in to manage audio.
          </p>
        )}
      </section>

      <section
        aria-labelledby="broadcast-heading"
        style={{
          marginBottom: "var(--house-space-6)",
          borderTop: "1px solid var(--house-rule-subtle)",
          paddingTop: "var(--house-space-5)",
        }}
      >
        <h2
          id="broadcast-heading"
          className="house-type-section"
          style={{ margin: "0 0 var(--house-space-3)", color: "var(--house-text-secondary)" }}
        >
          Broadcast link
        </h2>
        <BroadcastShare sessionId={sessionId ?? ""} token={broadcastToken} />
      </section>

      <WebRTCDebugPanel
        connectionState={webrtc.connectionState}
        iceState={webrtc.iceState}
        signalingState={webrtc.signalingState}
        dataChannelState={webrtc.dataChannelState}
        localSdp={webrtc.localSdp}
        remoteSdp={webrtc.remoteSdp}
        candidates={webrtc.candidates}
        events={webrtc.events}
        stats={webrtc.stats}
      />
    </OperateShell>
  );
}

/* -------------------------------------------------------------------------- */
/* Connection strip                                                           */
/* -------------------------------------------------------------------------- */

interface ConnectionStripProps {
  audioDevices: MediaDeviceInfo[];
  selectedDevice: string;
  onSelectDevice: (id: string) => void;
  micPermission: MicPermission;
  devicesError: string | null;
  onPrepareMic: () => void;
  enumerateRequested: boolean;
  isConnected: boolean;
  isConnecting: boolean;
  phase: ConnectPhase;
  lastError: { kind: ConnectFailureKind; message: string } | null;
  isMuted: boolean;
  inputLevel: number;
  onConnect: () => void;
  onDisconnect: () => void;
  onReconnect: () => void;
  onToggleMute: () => void;
  connectionState: RTCPeerConnectionState;
}

function ConnectionStrip(props: ConnectionStripProps) {
  const {
    audioDevices,
    selectedDevice,
    onSelectDevice,
    micPermission,
    devicesError,
    onPrepareMic,
    enumerateRequested,
    isConnected,
    isConnecting,
    phase,
    lastError,
    isMuted,
    inputLevel,
    onConnect,
    onDisconnect,
    onReconnect,
    onToggleMute,
    connectionState,
  } = props;

  const status = connectionStatusView(phase, connectionState, lastError, isMuted);
  const needsReconnect =
    phase === "failed" ||
    phase === "disconnected" ||
    lastError?.kind === "disconnected" ||
    connectionState === "failed" ||
    connectionState === "disconnected";

  return (
    <div
      data-testid="connection-strip"
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--house-space-3)",
      }}
    >
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "flex-end",
          gap: "var(--house-space-3)",
        }}
      >
        <div style={{ flex: "1 1 220px", minWidth: 0 }}>
          {!enumerateRequested && audioDevices.length === 0 ? (
            <div style={{ display: "flex", flexDirection: "column", gap: "var(--house-space-2)" }}>
              <span className="house-type-label" style={{ color: "var(--house-text-secondary)" }}>
                Microphone
              </span>
              <Button
                variant="secondary"
                icon="audio"
                onClick={onPrepareMic}
                disabled={isConnected || isConnecting}
                style={{ width: "100%" }}
                data-testid="prepare-mic"
              >
                Allow microphone access
              </Button>
              <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
                The browser will ask for permission so device names can be listed.
              </span>
            </div>
          ) : (
            <div data-testid="mic-device">
            <Field
              as="select"
              id="mic-device"
              label="Input device"
              value={selectedDevice}
              onChange={(e) => onSelectDevice(e.target.value)}
              disabled={isConnected || isConnecting}
            >
              {audioDevices.map((device) => (
                <option key={device.deviceId} value={device.deviceId}>
                  {device.label || `Microphone ${device.deviceId.slice(0, 8)}`}
                </option>
              ))}
              {audioDevices.length === 0 ? <option value="">No microphones found</option> : null}
            </Field>
            </div>
          )}
        </div>

        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "var(--house-space-2)",
            flex: "1 1 240px",
            alignItems: "center",
          }}
        >
          {!isConnected && !needsReconnect ? (
            <Button
              variant="primary"
              onClick={onConnect}
              loading={isConnecting}
              disabled={isConnecting}
              style={{ flex: "1 1 120px", minHeight: 44 }}
              data-testid="connect-btn"
            >
              {isConnecting ? phaseLabel(phase) : "Connect"}
            </Button>
          ) : null}

          {isConnected ? (
            <Button
              variant="destructive"
              onClick={onDisconnect}
              style={{ flex: "1 1 120px", minHeight: 44 }}
              data-testid="disconnect-btn"
            >
              Disconnect
            </Button>
          ) : null}

          {needsReconnect && !isConnected ? (
            <Button
              variant="primary"
              icon="refresh"
              onClick={onReconnect}
              loading={isConnecting}
              disabled={isConnecting}
              style={{ flex: "1 1 120px", minHeight: 44 }}
              data-testid="reconnect-btn"
            >
              Reconnect
            </Button>
          ) : null}

          <Button
            variant={isMuted ? "secondary" : "primary"}
            icon={isMuted ? "mute" : "audio"}
            onClick={onToggleMute}
            disabled={!isConnected}
            style={{
              flex: "1 1 120px",
              minHeight: 44,
              ...(isMuted
                ? {
                    borderColor: "var(--house-status-warning)",
                    color: "var(--house-status-warning)",
                  }
                : {}),
            }}
            data-testid="mute-btn"
            aria-pressed={isMuted}
          >
            {isMuted ? "Muted" : "Live"}
          </Button>
        </div>
      </div>

      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: "var(--house-space-3)",
        }}
      >
        <span data-testid="connection-status">
          <Status tone={status.tone}>{status.label}</Status>
        </span>
        <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
          Mic permission: {micPermissionLabel(micPermission)}
        </span>
        <div style={{ flex: "1 1 160px", minWidth: 120 }}>
          <VUMeter label="Input (Mic)" level={inputLevel} />
        </div>
      </div>

      {devicesError ? (
        <div role="alert">
          <Status tone="warning">{devicesError}</Status>
        </div>
      ) : null}

      {lastError ? (
        <div role="alert" data-testid="connection-error">
          <Status tone={lastError.kind === "disconnected" ? "warning" : "danger"}>
            {failureTitle(lastError.kind)}: {lastError.message}
          </Status>
        </div>
      ) : null}

      {phase === "ticket-mint" ? (
        <Status tone="info">Minting one-time media ticket…</Status>
      ) : null}
      {phase === "permission" ? (
        <Status tone="info">Requesting microphone…</Status>
      ) : null}
      {phase === "signaling" ? <Status tone="info">Negotiating WebRTC…</Status> : null}
    </div>
  );
}

function phaseLabel(phase: ConnectPhase): string {
  switch (phase) {
    case "ticket-mint":
      return "Minting ticket…";
    case "permission":
      return "Requesting mic…";
    case "signaling":
      return "Connecting…";
    default:
      return "Connecting…";
  }
}

function micPermissionLabel(p: MicPermission): string {
  switch (p) {
    case "granted":
      return "granted";
    case "denied":
      return "denied";
    case "prompt":
      return "not decided";
    default:
      return "unknown";
  }
}

function failureTitle(kind: ConnectFailureKind): string {
  switch (kind) {
    case "ticket-mint":
      return "Ticket mint failed";
    case "permission-denied":
      return "Permission denied";
    case "no-device":
      return "No device";
    case "signaling-failed":
      return "Signaling failed";
    case "disconnected":
      return "Disconnected";
    default:
      return "Connection error";
  }
}

function connectionStatusView(
  phase: ConnectPhase,
  connectionState: RTCPeerConnectionState,
  lastError: { kind: ConnectFailureKind; message: string } | null,
  isMuted: boolean,
): { tone: StatusTone; label: ReactNode } {
  if (phase === "connected" || connectionState === "connected") {
    return {
      tone: isMuted ? "warning" : "ok",
      label: isMuted ? "Connected · mic muted" : "Connected · mic live",
    };
  }
  if (phase === "ticket-mint") return { tone: "info", label: "Minting ticket" };
  if (phase === "permission") return { tone: "info", label: "Awaiting mic permission" };
  if (phase === "signaling" || connectionState === "connecting") {
    return { tone: "info", label: "Connecting" };
  }
  if (phase === "disconnected" || lastError?.kind === "disconnected") {
    return { tone: "warning", label: "Disconnected — reconnect required" };
  }
  if (phase === "failed" || connectionState === "failed") {
    return { tone: "danger", label: lastError ? failureTitle(lastError.kind) : "Failed" };
  }
  return { tone: "neutral", label: "Idle" };
}
