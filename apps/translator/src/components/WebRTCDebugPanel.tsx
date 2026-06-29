import { useState } from "react";
import type { WebRTCEvent, WebRTCStats, ICECandidate } from "../hooks/useWebRTC";

interface WebRTCDebugPanelProps {
  connectionState: RTCPeerConnectionState;
  iceState: RTCIceConnectionState;
  signalingState: RTCSignalingState;
  localSdp: string | null;
  remoteSdp: string | null;
  candidates: ICECandidate[];
  events: WebRTCEvent[];
  stats: WebRTCStats;
}

export function WebRTCDebugPanel(props: WebRTCDebugPanelProps) {
  const [expanded, setExpanded] = useState(false);
  const {
    connectionState,
    iceState,
    signalingState,
    localSdp,
    remoteSdp,
    candidates,
    events,
    stats,
  } = props;

  return (
    <div className="bg-gray-800 border border-gray-700 rounded-lg overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full px-4 py-3 flex items-center justify-between text-left hover:bg-gray-750 transition-colors"
      >
        <span className="font-medium text-gray-300">🔧 WebRTC Debug Panel</span>
        <span className="text-gray-500 text-sm">{expanded ? "▼" : "▶"}</span>
      </button>

      {expanded && (
        <div className="border-t border-gray-700 p-4 space-y-4 text-sm">
          {/* States */}
          <section>
            <h3 className="text-xs font-semibold text-gray-500 uppercase mb-2">Connection States</h3>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
              <StateChip label="Connection" value={connectionState} />
              <StateChip label="ICE" value={iceState} />
              <StateChip label="Signaling" value={signalingState} />
            </div>
          </section>

          {/* Stats */}
          <section>
            <h3 className="text-xs font-semibold text-gray-500 uppercase mb-2">Packet Stats</h3>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 text-gray-300">
              <StatItem label="Pkts Recv" value={stats.packetsReceived} />
              <StatItem label="Pkts Sent" value={stats.packetsSent} />
              <StatItem label="Bytes Recv" value={formatBytes(stats.bytesReceived)} />
              <StatItem label="Bytes Sent" value={formatBytes(stats.bytesSent)} />
              <StatItem label="Jitter" value={`${(stats.jitter * 1000).toFixed(1)}ms`} />
              <StatItem label="RTT" value={`${(stats.roundTripTime * 1000).toFixed(1)}ms`} />
            </div>
          </section>

          {/* ICE Candidates */}
          <section>
            <h3 className="text-xs font-semibold text-gray-500 uppercase mb-2">
              ICE Candidates ({candidates.length})
            </h3>
            <div className="max-h-32 overflow-y-auto space-y-1">
              {candidates.map((c, i) => (
                <div key={i} className="text-xs text-gray-400 font-mono truncate">
                  [{c.type}] {c.component}: {c.candidate.slice(0, 80)}
                </div>
              ))}
              {candidates.length === 0 && (
                <p className="text-gray-600 text-xs">No candidates yet</p>
              )}
            </div>
          </section>

          {/* SDP */}
          <section>
            <h3 className="text-xs font-semibold text-gray-500 uppercase mb-2">SDP Summary</h3>
            <div className="space-y-2">
              <SdpBlock label="Local SDP" sdp={localSdp} />
              <SdpBlock label="Remote SDP" sdp={remoteSdp} />
            </div>
          </section>

          {/* Event Log */}
          <section>
            <h3 className="text-xs font-semibold text-gray-500 uppercase mb-2">
              Event Log ({events.length})
            </h3>
            <div className="max-h-48 overflow-y-auto space-y-0.5 font-mono">
              {events
                .slice()
                .reverse()
                .map((ev, i) => (
                  <div key={i} className="text-xs text-gray-400 flex gap-2">
                    <span className="text-gray-600 shrink-0">
                      {new Date(ev.timestamp).toLocaleTimeString()}
                    </span>
                    <span className="text-blue-400 shrink-0">[{ev.type}]</span>
                    <span className="truncate">{ev.detail}</span>
                  </div>
                ))}
              {events.length === 0 && (
                <p className="text-gray-600 text-xs">No events yet</p>
              )}
            </div>
          </section>
        </div>
      )}
    </div>
  );
}

function StateChip({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-gray-900 rounded px-2 py-1">
      <span className="text-gray-500 text-xs">{label}: </span>
      <span className="text-gray-200 text-xs font-mono">{value}</span>
    </div>
  );
}

function StatItem({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <span className="text-gray-500 text-xs">{label}: </span>
      <span className="text-gray-200 text-xs font-mono">{value}</span>
    </div>
  );
}

function SdpBlock({ label, sdp }: { label: string; sdp: string | null }) {
  const [showFull, setShowFull] = useState(false);

  if (!sdp) {
    return (
      <div>
        <span className="text-gray-500 text-xs">{label}: </span>
        <span className="text-gray-600 text-xs">Not set</span>
      </div>
    );
  }

  const summary = sdp.slice(0, 100) + (sdp.length > 100 ? "..." : "");

  return (
    <div>
      <div className="flex items-center gap-2">
        <span className="text-gray-500 text-xs">{label}</span>
        <button
          onClick={() => setShowFull(!showFull)}
          className="text-blue-500 text-xs hover:underline"
        >
          {showFull ? "collapse" : "expand"}
        </button>
      </div>
      <pre className="text-xs text-gray-400 bg-gray-900 rounded p-1 mt-1 overflow-x-auto max-h-40">
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
