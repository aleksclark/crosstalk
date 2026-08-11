import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import {
  Button,
  CopyableId,
  DataState,
  PageHeader,
  Status,
} from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";
import { DebugPanel } from "../components/DebugPanel";

const BASE_URL = import.meta.env.VITE_API_URL || "";

// PeerState mirrors server/webrtc.PeerState (returned by /api/debug/peers).
// Debug endpoints are not in the generated OpenAPI client; keep the working path.
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
const STALE_MS = 10_000;

function peerLabel(peer: PeerState): string {
  if (peer.client_name?.trim()) return peer.client_name.trim();
  if (peer.client_type?.trim()) return peer.client_type.trim();
  return "Unnamed peer";
}

export function DebugPage() {
  const { token } = useAuth();
  const [peers, setPeers] = useState<PeerState[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [events, setEvents] = useState<UiEvent[]>([]);
  const [peersError, setPeersError] = useState<string | null>(null);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [loadingPeers, setLoadingPeers] = useState(true);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [now, setNow] = useState(() => Date.now());

  const authFetch = useCallback(
    (path: string, signal?: AbortSignal) =>
      fetch(`${BASE_URL}${path}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        signal,
      }),
    [token],
  );

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  // Poll the live peer list with abort + stale-response ignore.
  useEffect(() => {
    let active = true;
    let controller: AbortController | null = null;

    const load = async () => {
      controller?.abort();
      controller = new AbortController();
      const signal = controller.signal;
      try {
        const res = await authFetch("/api/debug/peers", signal);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body = (await res.json()) as { peers: PeerState[] };
        if (!active || signal.aborted) return;
        setPeers(body.peers ?? []);
        setPeersError(null);
        setLastUpdated(new Date());
      } catch (e) {
        if (!active || signal.aborted) return;
        setPeersError(e instanceof Error ? e.message : "failed to load peers");
      } finally {
        if (active && !signal.aborted) setLoadingPeers(false);
      }
    };

    void load();
    const timer = window.setInterval(() => void load(), 3000);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(timer);
    };
  }, [authFetch]);

  // Load events for the selected peer.
  useEffect(() => {
    if (!selectedId) {
      setEvents([]);
      setEventsError(null);
      return;
    }
    let active = true;
    let controller: AbortController | null = null;

    const load = async () => {
      controller?.abort();
      controller = new AbortController();
      const signal = controller.signal;
      try {
        const res = await authFetch(
          `/api/debug/peers/${encodeURIComponent(selectedId)}/events`,
          signal,
        );
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body = (await res.json()) as { events: ServerEvent[] };
        if (!active || signal.aborted) return;
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
        if (!active || signal.aborted) return;
        setEventsError(e instanceof Error ? e.message : "failed to load events");
      }
    };

    void load();
    const timer = window.setInterval(() => void load(), 3000);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(timer);
    };
  }, [authFetch, selectedId]);

  // Drop selection if the peer disappears.
  useEffect(() => {
    if (selectedId && !peers.some((p) => p.id === selectedId)) {
      setSelectedId(null);
    }
  }, [peers, selectedId]);

  const selectedPeer = peers.find((p) => p.id === selectedId) ?? null;
  const connectedCount = useMemo(
    () => peers.filter((p) => CONNECTED_ICE_STATES.has(p.ice_state)).length,
    [peers],
  );

  const stale =
    lastUpdated != null && now - lastUpdated.getTime() > STALE_MS;
  const ageSeconds =
    lastUpdated != null
      ? Math.max(0, Math.round((now - lastUpdated.getTime()) / 1000))
      : null;

  const summary =
    peersError
      ? `Peer list unavailable: ${peersError}`
      : `${peers.length} peer${peers.length === 1 ? "" : "s"} reported · ${connectedCount} with ICE connected/completed` +
        (ageSeconds != null
          ? ` · last updated ${ageSeconds}s ago${stale ? " (stale)" : ""}`
          : "");

  const onRowKeyDown = (
    e: ReactKeyboardEvent<HTMLTableRowElement | HTMLLIElement>,
    peerId: string,
  ) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      setSelectedId(peerId);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Monitor"
        title="Debug"
        lede="Live WebRTC peer inspector. Values come only from /api/debug endpoints — no fabricated health."
        meta={
          lastUpdated ? (
            <span className={stale ? "text-[var(--house-status-warning)]" : undefined}>
              Last updated {lastUpdated.toLocaleTimeString()}
              {stale ? " · stale" : ""}
            </span>
          ) : (
            <span>Waiting for first poll…</span>
          )
        }
      />

      <section
        aria-label="Peer summary"
        className="border-y border-border py-3"
      >
        <p className="house-type-lede">{summary}</p>
        {stale && !peersError ? (
          <div className="mt-2">
            <Status tone="warning">Stale — poll delayed</Status>
          </div>
        ) : null}
      </section>

      {loadingPeers && peers.length === 0 && !peersError ? (
        <DataState
          kind="loading"
          title="Loading peers"
          description="Polling /api/debug/peers."
        />
      ) : peersError && peers.length === 0 ? (
        <DataState
          kind="error"
          title="Could not load peers"
          description={peersError}
        />
      ) : (
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
          <section
            aria-label="Connected peers"
            className="border border-border bg-[var(--house-bg-surface)]"
          >
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <h2 className="house-type-section">Connected peers</h2>
              <span className="house-type-meta text-muted-foreground">
                {peers.length} total
              </span>
            </div>

            {peers.length === 0 ? (
              <p className="px-4 py-6 text-center text-sm text-muted-foreground">
                No peers connected
              </p>
            ) : (
              <>
                <div className="hidden md:block">
                  <table className="w-full text-sm" role="grid">
                    <thead>
                      <tr className="border-b border-border">
                        <th className="px-4 py-2 text-left house-type-label text-muted-foreground">
                          Client
                        </th>
                        <th className="px-4 py-2 text-left house-type-label text-muted-foreground">
                          ICE
                        </th>
                        <th className="px-4 py-2 text-left house-type-label text-muted-foreground">
                          DTLS
                        </th>
                        <th className="px-4 py-2 text-left house-type-label text-muted-foreground">
                          Signaling
                        </th>
                        <th className="px-4 py-2 text-left house-type-label text-muted-foreground">
                          Data
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {peers.map((peer) => {
                        const connected = CONNECTED_ICE_STATES.has(peer.ice_state);
                        const selected = peer.id === selectedId;
                        return (
                          <tr
                            key={peer.id}
                            tabIndex={0}
                            aria-selected={selected}
                            onClick={() => setSelectedId(peer.id)}
                            onKeyDown={(e) => onRowKeyDown(e, peer.id)}
                            className={`cursor-pointer border-b border-border/60 outline-none last:border-b-0 hover:bg-[var(--house-bg-raised)] focus-visible:bg-[var(--house-selected-bg)] ${
                              selected
                                ? "bg-[var(--house-selected-bg)] border-l-2 border-l-[var(--house-accent)]"
                                : "border-l-2 border-l-transparent"
                            }`}
                          >
                            <td className="px-4 py-2">
                              <div className="font-medium">{peerLabel(peer)}</div>
                              <div className="house-type-meta text-muted-foreground">
                                {peer.client_type || "unknown"} ·{" "}
                                <CopyableId value={peer.id} display={peer.id.slice(0, 8)} />
                              </div>
                            </td>
                            <td className="px-4 py-2">
                              <Status tone={connected ? "ok" : "warning"}>
                                {peer.ice_state || "—"}
                              </Status>
                            </td>
                            <td className="px-4 py-2 house-type-meta text-muted-foreground">
                              {peer.dtls_state || "—"}
                            </td>
                            <td className="px-4 py-2 house-type-meta text-muted-foreground">
                              {peer.signaling_state || "—"}
                            </td>
                            <td className="px-4 py-2 house-type-meta text-muted-foreground">
                              {peer.data_channel_open ? "open" : "closed"}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>

                <ul className="divide-y divide-border md:hidden" role="listbox">
                  {peers.map((peer) => {
                    const connected = CONNECTED_ICE_STATES.has(peer.ice_state);
                    const selected = peer.id === selectedId;
                    return (
                      <li
                        key={peer.id}
                        role="option"
                        aria-selected={selected}
                        tabIndex={0}
                        onClick={() => setSelectedId(peer.id)}
                        onKeyDown={(e) => onRowKeyDown(e, peer.id)}
                        className={`cursor-pointer px-4 py-3 outline-none focus-visible:bg-[var(--house-selected-bg)] ${
                          selected ? "bg-[var(--house-selected-bg)]" : ""
                        }`}
                      >
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-medium">{peerLabel(peer)}</span>
                          <Status tone={connected ? "ok" : "warning"}>
                            {peer.ice_state || "—"}
                          </Status>
                        </div>
                        <p className="mt-1 house-type-meta text-muted-foreground">
                          {peer.client_type || "unknown"} · {peer.id.slice(0, 8)}…
                        </p>
                      </li>
                    );
                  })}
                </ul>
              </>
            )}
          </section>

          <section
            aria-label="Peer inspector"
            className="border border-border bg-[var(--house-bg-surface)]"
          >
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <h2 className="house-type-section">Inspector</h2>
              {selectedPeer ? (
                <Button variant="ghost" size="sm" onClick={() => setSelectedId(null)}>
                  Close
                </Button>
              ) : null}
            </div>

            {!selectedPeer ? (
              <p className="px-4 py-6 text-sm text-muted-foreground">
                Select a peer row to inspect ICE, DTLS, signaling, and events.
              </p>
            ) : (
              <div className="space-y-4 p-4">
                <div>
                  <p className="font-medium">{peerLabel(selectedPeer)}</p>
                  <p className="mt-1 house-type-meta text-muted-foreground">
                    {selectedPeer.client_type || "unknown"}
                  </p>
                  <div className="mt-2">
                    <CopyableId value={selectedPeer.id} label="Copy peer ID" />
                  </div>
                </div>

                <dl className="divide-y divide-border border-y border-border">
                  <div className="grid grid-cols-[7rem_1fr] gap-2 py-2">
                    <dt className="house-type-meta text-muted-foreground">ICE</dt>
                    <dd className="house-type-meta">{selectedPeer.ice_state || "—"}</dd>
                  </div>
                  <div className="grid grid-cols-[7rem_1fr] gap-2 py-2">
                    <dt className="house-type-meta text-muted-foreground">ICE gather</dt>
                    <dd className="house-type-meta">
                      {selectedPeer.ice_gathering_state || "—"}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[7rem_1fr] gap-2 py-2">
                    <dt className="house-type-meta text-muted-foreground">DTLS</dt>
                    <dd className="house-type-meta">{selectedPeer.dtls_state || "—"}</dd>
                  </div>
                  <div className="grid grid-cols-[7rem_1fr] gap-2 py-2">
                    <dt className="house-type-meta text-muted-foreground">Signaling</dt>
                    <dd className="house-type-meta">
                      {selectedPeer.signaling_state || "—"}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[7rem_1fr] gap-2 py-2">
                    <dt className="house-type-meta text-muted-foreground">Data channel</dt>
                    <dd className="house-type-meta">
                      {selectedPeer.data_channel_open ? "open" : "closed"}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[7rem_1fr] gap-2 py-2">
                    <dt className="house-type-meta text-muted-foreground">Created</dt>
                    <dd className="house-type-meta">
                      {selectedPeer.created_at
                        ? new Date(selectedPeer.created_at).toLocaleString()
                        : "—"}
                    </dd>
                  </div>
                </dl>

                {eventsError ? (
                  <p role="alert" className="text-xs text-[var(--house-status-danger)]">
                    Failed to load events: {eventsError}
                  </p>
                ) : (
                  <DebugPanel
                    events={events}
                    title={`Events · ${peerLabel(selectedPeer)}`}
                  />
                )}
              </div>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
