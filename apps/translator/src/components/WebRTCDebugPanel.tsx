import { useState } from "react";
import { Icon, Status } from "@crosstalk/theme";
import type { WebRTCEvent, WebRTCStats, ICECandidate } from "../hooks/useWebRTC";

interface WebRTCDebugPanelProps {
  connectionState: RTCPeerConnectionState;
  iceState: RTCIceConnectionState;
  signalingState: RTCSignalingState;
  dataChannelState: RTCDataChannelState | "none";
  localSdp: string | null;
  remoteSdp: string | null;
  candidates: ICECandidate[];
  events: WebRTCEvent[];
  stats: WebRTCStats;
}

/**
 * Native disclosure inspector for WebRTC diagnostics.
 * Mono for SDP/stats/events; local event reversal is a bounded diagnostic log.
 */
export function WebRTCDebugPanel(props: WebRTCDebugPanelProps) {
  const {
    connectionState,
    iceState,
    signalingState,
    dataChannelState,
    localSdp,
    remoteSdp,
    candidates,
    events,
    stats,
  } = props;

  return (
    <details
      data-testid="webrtc-debug"
      style={{
        borderTop: "1px solid var(--house-rule-subtle)",
        borderBottom: "1px solid var(--house-rule-subtle)",
      }}
    >
      <summary
        className="house-type-section"
        style={{
          cursor: "pointer",
          listStyle: "none",
          display: "flex",
          alignItems: "center",
          gap: "var(--house-space-2)",
          minHeight: "var(--house-control-height)",
          padding: "var(--house-space-3) 0",
          color: "var(--house-text-secondary)",
          fontWeight: 600,
        }}
      >
        <Icon name="debug" size="compact" />
        Connection inspector
        <span className="house-type-meta" style={{ marginLeft: "auto", color: "var(--house-text-tertiary)" }}>
          {connectionState} · ICE {iceState}
        </span>
      </summary>

      <div
        style={{
          display: "flex",
          flexDirection: "column",
          gap: "var(--house-space-5)",
          paddingBottom: "var(--house-space-5)",
        }}
      >
        <section>
          <h3 className="house-type-eyebrow" style={{ margin: "0 0 var(--house-space-2)", color: "var(--house-text-tertiary)" }}>
            States
          </h3>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
              gap: "var(--house-space-2)",
            }}
          >
            <MetaRow label="Connection" value={connectionState} />
            <MetaRow label="ICE" value={iceState} />
            <MetaRow label="Signaling" value={signalingState} />
            <MetaRow label="Data channel" value={dataChannelState} />
          </div>
        </section>

        <section>
          <h3 className="house-type-eyebrow" style={{ margin: "0 0 var(--house-space-2)", color: "var(--house-text-tertiary)" }}>
            Packet stats
          </h3>
          <div
            className="house-type-code"
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
              gap: "var(--house-space-2)",
              color: "var(--house-text-secondary)",
            }}
          >
            <MetaRow label="Pkts recv" value={String(stats.packetsReceived)} mono />
            <MetaRow label="Pkts sent" value={String(stats.packetsSent)} mono />
            <MetaRow label="Pkts lost" value={String(stats.packetsLost)} mono />
            <MetaRow label="Bytes recv" value={formatBytes(stats.bytesReceived)} mono />
            <MetaRow label="Bytes sent" value={formatBytes(stats.bytesSent)} mono />
            <MetaRow label="Jitter" value={`${(stats.jitter * 1000).toFixed(1)}ms`} mono />
            <MetaRow label="RTT" value={`${(stats.roundTripTime * 1000).toFixed(1)}ms`} mono />
          </div>
        </section>

        <section>
          <h3 className="house-type-eyebrow" style={{ margin: "0 0 var(--house-space-2)", color: "var(--house-text-tertiary)" }}>
            ICE candidates ({candidates.length})
          </h3>
          <div
            className="house-type-code"
            style={{
              maxHeight: 128,
              overflow: "auto",
              display: "flex",
              flexDirection: "column",
              gap: 2,
              color: "var(--house-text-tertiary)",
            }}
          >
            {candidates.map((c, i) => (
              <div key={i} style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                <span style={{ color: c.direction === "local" ? "var(--house-accent)" : "var(--house-text-secondary)" }}>
                  {c.direction}
                </span>{" "}
                [{c.type}] {c.component}: {c.candidate.slice(0, 70)}
              </div>
            ))}
            {candidates.length === 0 ? <p style={{ margin: 0 }}>No candidates yet</p> : null}
          </div>
        </section>

        <section>
          <h3 className="house-type-eyebrow" style={{ margin: "0 0 var(--house-space-2)", color: "var(--house-text-tertiary)" }}>
            SDP
          </h3>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--house-space-3)" }}>
            <SdpBlock label="Local SDP" sdp={localSdp} />
            <SdpBlock label="Remote SDP" sdp={remoteSdp} />
          </div>
        </section>

        <section>
          <h3 className="house-type-eyebrow" style={{ margin: "0 0 var(--house-space-2)", color: "var(--house-text-tertiary)" }}>
            Event log ({events.length})
          </h3>
          <div
            className="house-type-code"
            style={{
              maxHeight: 192,
              overflow: "auto",
              display: "flex",
              flexDirection: "column",
              gap: 2,
            }}
          >
            {events
              .slice()
              .reverse()
              .map((ev, i) => (
                <div key={i} style={{ display: "flex", gap: "var(--house-space-2)", minWidth: 0 }}>
                  <span style={{ color: "var(--house-text-tertiary)", flexShrink: 0 }}>
                    {new Date(ev.timestamp).toLocaleTimeString()}
                  </span>
                  <span style={{ flexShrink: 0, color: severityColor(ev.type, ev.detail) }}>[{ev.type}]</span>
                  <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", color: "var(--house-text-secondary)" }}>
                    {ev.detail}
                  </span>
                </div>
              ))}
            {events.length === 0 ? (
              <p style={{ margin: 0, color: "var(--house-text-tertiary)" }}>No events yet</p>
            ) : null}
          </div>
        </section>

        <Status tone="info">Local diagnostic log only — not a server resource collection.</Status>
      </div>
    </details>
  );
}

function MetaRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
      <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
        {label}
      </span>
      <span
        className={mono ? "house-type-code" : "house-type-body"}
        style={{ color: "var(--house-text-primary)" }}
      >
        {value}
      </span>
    </div>
  );
}

function SdpBlock({ label, sdp }: { label: string; sdp: string | null }) {
  const [showFull, setShowFull] = useState(false);

  if (!sdp) {
    return (
      <div>
        <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
          {label}:{" "}
        </span>
        <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
          Not set
        </span>
      </div>
    );
  }

  const summary = sdp.slice(0, 100) + (sdp.length > 100 ? "…" : "");

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: "var(--house-space-2)" }}>
        <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
          {label}
        </span>
        <button
          type="button"
          onClick={() => setShowFull(!showFull)}
          className="house-type-meta"
          style={{
            background: "none",
            border: "none",
            color: "var(--house-accent)",
            cursor: "pointer",
            padding: 0,
            font: "inherit",
          }}
        >
          {showFull ? "collapse" : "expand"}
        </button>
      </div>
      <pre
        className="house-type-code"
        style={{
          margin: "var(--house-space-1) 0 0",
          padding: "var(--house-space-2)",
          background: "var(--house-bg-sunken)",
          border: "1px solid var(--house-rule-subtle)",
          borderRadius: "var(--house-radius-md)",
          overflow: "auto",
          maxHeight: 160,
          color: "var(--house-text-secondary)",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
        }}
      >
        {showFull ? sdp : summary}
      </pre>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

function severityColor(type: string, detail: string): string {
  const s = `${type} ${detail}`.toLowerCase();
  if (s.includes("error") || s.includes("fail")) return "var(--house-status-danger)";
  if (s.includes("closed") || s.includes("disconnect")) return "var(--house-status-danger)";
  if (s.includes("connected") || s.includes("open") || s.includes("complete")) return "var(--house-status-ok)";
  if (s.includes("connecting") || s.includes("checking") || s.includes("mute")) return "var(--house-status-warning)";
  return "var(--house-status-info)";
}
