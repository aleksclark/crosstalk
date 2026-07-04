import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../hooks/useAuth";
import { DebugPanel } from "../components/DebugPanel";

const BASE_URL = import.meta.env.VITE_API_URL || "";

// PeerState mirrors server/webrtc.PeerState (returned by /api/debug/peers).
interface PeerState {
  id: string;
  created_at: string;
  ice_state: string;
  dtls_state: string;
  signaling_state: string;
  ice_gathering_state: string;
  data_channel_open: boolean;
  client_type?: string;
  client_name?: string;
}

// ServerEvent mirrors server/webrtc.Event (returned by /peers/{id}/events).
interface ServerEvent {
  timestamp: string;
  peer_id: string;
  type: string;
  detail?: unknown;
}

interface UiEvent {
  id: string;
  timestamp: string;
  type: string;
  message: string;
  data?: unknown;
}

const CONNECTED_ICE_STATES = new Set(["connected", "completed"]);

export function DebugPage() {
  const { token } = useAuth();
  const [peers, setPeers] = useState<PeerState[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [events, setEvents] = useState<UiEvent[]>([]);
  const [peersError, setPeersError] = useState<string | null>(null);
  const [eventsError, setEventsError] = useState<string | null>(null);

  const authFetch = useCallback(
    (path: string) =>
      fetch(`${BASE_URL}${path}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      }),
    [token]
  );

  // Poll the live peer list.
  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const res = await authFetch("/api/debug/peers");
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body = (await res.json()) as { peers: PeerState[] };
        if (!active) return;
        setPeers(body.peers ?? []);
        setPeersError(null);
      } catch (e) {
        if (!active) return;
        setPeersError(e instanceof Error ? e.message : "failed to load peers");
      }
    };
    load();
    const timer = setInterval(load, 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [authFetch]);

  // Load events for the selected peer (and refresh while selected).
  useEffect(() => {
    if (!selectedId) {
      setEvents([]);
      setEventsError(null);
      return;
    }
    let active = true;
    const load = async () => {
      try {
        const res = await authFetch(
          `/api/debug/peers/${encodeURIComponent(selectedId)}/events`
        );
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body = (await res.json()) as { events: ServerEvent[] };
        if (!active) return;
        const mapped = (body.events ?? []).map((e, i) => ({
          id: `${e.timestamp}-${i}`,
          timestamp: e.timestamp,
          type: e.type,
          message: e.type.replace(/_/g, " "),
          data: e.detail,
        }));
        setEvents(mapped);
        setEventsError(null);
      } catch (e) {
        if (!active) return;
        setEventsError(e instanceof Error ? e.message : "failed to load events");
      }
    };
    load();
    const timer = setInterval(load, 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [authFetch, selectedId]);

  const selectedPeer = peers.find((p) => p.id === selectedId) ?? null;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Debug</h1>

      {/* Connected peers */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-border flex items-center justify-between">
          <h2 className="text-sm font-semibold">Connected Peers</h2>
          <span className="text-xs text-muted-foreground">
            {peers.length} connected
          </span>
        </div>

        {peersError && (
          <p className="px-4 py-3 text-xs text-red-400">
            Failed to load peers: {peersError}
          </p>
        )}

        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Peer ID
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Client
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                ICE
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                DTLS
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Signaling
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Data Channel
              </th>
            </tr>
          </thead>
          <tbody>
            {peers.map((peer) => {
              const connected = CONNECTED_ICE_STATES.has(peer.ice_state);
              return (
                <tr
                  key={peer.id}
                  onClick={() => setSelectedId(peer.id)}
                  className={`border-b border-border/50 cursor-pointer hover:bg-accent/50 ${
                    peer.id === selectedId ? "bg-accent/60" : ""
                  }`}
                >
                  <td className="px-4 py-2 font-mono text-xs">{peer.id}</td>
                  <td className="px-4 py-2 text-xs">
                    {peer.client_type || "unknown"}
                    {peer.client_name ? ` · ${peer.client_name}` : ""}
                  </td>
                  <td className="px-4 py-2">
                    <span
                      className={`text-xs px-2 py-0.5 rounded ${
                        connected
                          ? "bg-green-500/20 text-green-400"
                          : "bg-yellow-500/20 text-yellow-400"
                      }`}
                    >
                      {peer.ice_state || "-"}
                    </span>
                  </td>
                  <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                    {peer.dtls_state || "-"}
                  </td>
                  <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                    {peer.signaling_state || "-"}
                  </td>
                  <td className="px-4 py-2 text-xs text-muted-foreground">
                    {peer.data_channel_open ? "open" : "closed"}
                  </td>
                </tr>
              );
            })}
            {peers.length === 0 && !peersError && (
              <tr>
                <td
                  colSpan={6}
                  className="px-4 py-6 text-center text-muted-foreground text-xs"
                >
                  No peers connected
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Per-peer event log — only shown when a peer is selected */}
      {selectedPeer && (
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm">
            <span className="font-semibold">Events for</span>
            <span className="font-mono text-xs bg-muted px-2 py-0.5 rounded">
              {selectedPeer.id}
            </span>
            <button
              onClick={() => setSelectedId(null)}
              className="ml-auto text-xs text-muted-foreground hover:text-foreground"
            >
              Close
            </button>
          </div>
          {eventsError ? (
            <p className="text-xs text-red-400">
              Failed to load events: {eventsError}
            </p>
          ) : (
            <DebugPanel
              events={events}
              title={`WebRTC Event Log (${selectedPeer.id})`}
            />
          )}
        </div>
      )}
    </div>
  );
}
