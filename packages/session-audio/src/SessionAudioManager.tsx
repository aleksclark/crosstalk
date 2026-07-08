import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import type { createApiClient, components } from "@crosstalk/api-client";
import { ChannelMonitor } from "./ChannelMonitor";

type Client = ReturnType<typeof createApiClient>;
type Channel = components["schemas"]["ChannelOut"];
type Source = components["schemas"]["SourceOut"];
type MixEntry = components["schemas"]["MixEntryOut"];
type ABC = components["schemas"]["ABCOut"];

export interface SessionAudioManagerProps {
  // An authenticated openapi-fetch client (reuses each app's error handling).
  client: Client;
  // The caller's JWT, used to open per-channel monitor connections.
  token: string;
  sessionId: string;
  // Allow editing the mix (assign/remove sources, mute, level). Defaults true.
  editable?: boolean;
  // Always open a receive-only monitor per channel (volume/mute/VU). Default true.
  monitor?: boolean;
  // Show per-ABC monitor-channel selectors for booth boards. Defaults true.
  showABCMonitors?: boolean;
  // Base origin for monitor signaling websockets. Defaults to window.location.origin.
  baseUrl?: string;
  className?: string;
}

// SessionAudioManager is the shared UI for wiring a session's audio. For each
// channel it:
//   - always opens a receive-only monitor with its own volume, mute and VU
//     meter (the VU meter reflects the incoming signal regardless of mute or
//     volume), and
//   - lets the operator edit the mix: which sources feed the channel, each with
//     mute and level.
// It also lets booth boards (ABCs) pick which channel they monitor.
export function SessionAudioManager({
  client,
  token,
  sessionId,
  editable = true,
  monitor = true,
  showABCMonitors = true,
  baseUrl,
  className,
}: SessionAudioManagerProps) {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [abcs, setAbcs] = useState<ABC[]>([]);
  const [mixByChannel, setMixByChannel] = useState<Record<string, MixEntry[]>>(
    {},
  );
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    const [channelsRes, sourcesRes, abcsRes] = await Promise.all([
      client.GET("/api/sessions/{id}/channels", {
        params: { path: { id: sessionId } },
      }),
      client.GET("/api/sessions/{id}/sources", {
        params: { path: { id: sessionId } },
      }),
      client.GET("/api/abcs"),
    ]);
    const chans = channelsRes.data?.data ?? [];
    setChannels(chans);
    setSources(sourcesRes.data?.data ?? []);
    setAbcs(
      (abcsRes.data?.data ?? []).filter((a) => a.session_id === sessionId),
    );

    const mixes = await Promise.all(
      chans.map((ch) =>
        client.GET("/api/sessions/{id}/channels/{ch_id}/mix", {
          params: { path: { id: sessionId, ch_id: ch.id } },
        }),
      ),
    );
    const map: Record<string, MixEntry[]> = {};
    chans.forEach((ch, i) => {
      map[ch.id] = mixes[i]?.data?.data ?? [];
    });
    setMixByChannel(map);
    setLoading(false);
  }, [client, sessionId]);

  useEffect(() => {
    setLoading(true);
    void reload();
  }, [reload]);

  const setABCMonitor = useCallback(
    async (abcId: string, channelId: string | null) => {
      // Optimistic: reflect the change locally, then persist. Changing the
      // monitor forces the board to reconnect and re-bridge server-side.
      setAbcs((prev) =>
        prev.map((a) =>
          a.id === abcId
            ? { ...a, monitor_channel_id: channelId ?? undefined }
            : a,
        ),
      );
      await client.PUT("/api/abcs/{id}", {
        params: { path: { id: abcId } },
        body: { monitor_channel_id: channelId ?? "" },
      });
    },
    [client],
  );

  const persist = useCallback(
    async (channelId: string, entries: MixEntry[]) => {
      await client.PUT("/api/sessions/{id}/channels/{ch_id}/mix", {
        params: { path: { id: sessionId, ch_id: channelId } },
        body: {
          entries: entries.map((e) => ({
            source_id: e.source_id,
            muted: e.muted,
            level: e.level,
          })),
        },
      });
    },
    [client, sessionId],
  );

  const applyMix = useCallback(
    (channelId: string, next: MixEntry[]) => {
      setMixByChannel((prev) => ({ ...prev, [channelId]: next }));
      void persist(channelId, next);
    },
    [persist],
  );

  const assignSource = useCallback(
    (channelId: string, sourceId: string) => {
      const current = mixByChannel[channelId] ?? [];
      if (current.some((e) => e.source_id === sourceId)) return;
      applyMix(channelId, [
        ...current,
        {
          id: "",
          channel_id: channelId,
          source_id: sourceId,
          muted: false,
          level: 1,
        },
      ]);
    },
    [mixByChannel, applyMix],
  );

  const removeSource = useCallback(
    (channelId: string, sourceId: string) => {
      const current = mixByChannel[channelId] ?? [];
      applyMix(
        channelId,
        current.filter((e) => e.source_id !== sourceId),
      );
    },
    [mixByChannel, applyMix],
  );

  const setMuted = useCallback(
    (channelId: string, sourceId: string, muted: boolean) => {
      const current = mixByChannel[channelId] ?? [];
      applyMix(
        channelId,
        current.map((e) => (e.source_id === sourceId ? { ...e, muted } : e)),
      );
    },
    [mixByChannel, applyMix],
  );

  const setLevel = useCallback(
    (channelId: string, sourceId: string, level: number) => {
      const current = mixByChannel[channelId] ?? [];
      applyMix(
        channelId,
        current.map((e) => (e.source_id === sourceId ? { ...e, level } : e)),
      );
    },
    [mixByChannel, applyMix],
  );

  const sourceName = useCallback(
    (id: string) => sources.find((s) => s.id === id)?.name ?? id.slice(0, 8),
    [sources],
  );

  const containerCls = ["space-y-4", className].filter(Boolean).join(" ");

  if (loading) {
    return (
      <div className={containerCls}>
        <p className="text-sm text-muted-foreground">Loading session audio…</p>
      </div>
    );
  }

  return (
    <div className={containerCls}>
      {channels.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No channels configured for this session.
        </p>
      ) : (
        channels.map((ch) => (
          <ChannelCard
            key={ch.id}
            channel={ch}
            entries={mixByChannel[ch.id] ?? []}
            sources={sources}
            editable={editable}
            sourceName={sourceName}
            onAssign={(sid) => assignSource(ch.id, sid)}
            onRemove={(sid) => removeSource(ch.id, sid)}
            onMute={(sid, m) => setMuted(ch.id, sid, m)}
            onLevel={(sid, l) => setLevel(ch.id, sid, l)}
            monitor={
              monitor ? (
                <ChannelMonitor
                  sessionId={sessionId}
                  token={token}
                  channelName={ch.name}
                  baseUrl={baseUrl}
                />
              ) : null
            }
          />
        ))
      )}

      {showABCMonitors && abcs.length > 0 && (
        <div className="rounded-lg border border-border bg-card p-4">
          <h3 className="mb-3 text-sm font-semibold text-foreground">
            Booth monitors
          </h3>
          <div className="space-y-3">
            {abcs.map((abc) => (
              <div key={abc.id} className="flex items-center gap-3">
                <span
                  className={`h-2 w-2 shrink-0 rounded-full ${
                    abc.connected ? "bg-green-500" : "bg-muted-foreground"
                  }`}
                  title={abc.connected ? "Connected" : "Offline"}
                />
                <span className="w-28 truncate text-xs text-foreground">
                  {abc.name}
                </span>
                <select
                  value={abc.monitor_channel_id ?? ""}
                  disabled={!editable}
                  onChange={(e) => setABCMonitor(abc.id, e.target.value || null)}
                  className="flex-1 rounded border border-input bg-background px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-2 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <option value="">None (not monitoring)</option>
                  {channels.map((ch) => (
                    <option key={ch.id} value={ch.id}>
                      {ch.name} — {ch.type}
                    </option>
                  ))}
                </select>
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            Changing a monitor reconnects the booth board to apply the new
            channel.
          </p>
        </div>
      )}
    </div>
  );
}

interface ChannelCardProps {
  channel: Channel;
  entries: MixEntry[];
  sources: Source[];
  editable: boolean;
  sourceName: (id: string) => string;
  onAssign: (sourceId: string) => void;
  onRemove: (sourceId: string) => void;
  onMute: (sourceId: string, muted: boolean) => void;
  onLevel: (sourceId: string, level: number) => void;
  monitor: ReactNode;
}

function ChannelCard({
  channel,
  entries,
  sources,
  editable,
  sourceName,
  onAssign,
  onRemove,
  onMute,
  onLevel,
  monitor,
}: ChannelCardProps) {
  const assignedIds = useMemo(
    () => new Set(entries.map((e) => e.source_id)),
    [entries],
  );
  const available = sources.filter((s) => !assignedIds.has(s.id));

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-foreground">
          {channel.name}
          <span className="ml-2 text-xs uppercase text-muted-foreground">
            {channel.type}
          </span>
        </h3>
        {editable && available.length > 0 && (
          <select
            defaultValue=""
            onChange={(e) => {
              if (e.target.value) {
                onAssign(e.target.value);
                e.target.value = "";
              }
            }}
            className="rounded border border-input bg-background px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
          >
            <option value="">+ Assign source…</option>
            {available.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        )}
      </div>

      {/* Monitor: always-on listening for this channel (volume / mute / VU). */}
      {monitor && (
        <div className="mb-3 rounded-md border border-border bg-background/40 p-2">
          <div className="mb-1 text-xs font-medium text-muted-foreground">
            Monitor
          </div>
          {monitor}
        </div>
      )}

      {/* Mix: sources feeding this channel (server-side routing). */}
      <div className="mb-1 text-xs font-medium text-muted-foreground">
        Sources
      </div>
      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground">No sources assigned.</p>
      ) : (
        <div className="space-y-3">
          {entries.map((e) => (
            <div key={e.source_id} className="flex items-center gap-3">
              <button
                onClick={() => editable && onMute(e.source_id, !e.muted)}
                disabled={!editable}
                className={`flex h-8 w-8 items-center justify-center rounded text-xs font-bold transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${
                  e.muted
                    ? "border border-destructive/50 bg-destructive/20 text-destructive-foreground"
                    : "border border-primary/50 bg-primary/20 text-primary-foreground"
                }`}
                title={e.muted ? "Unmute" : "Mute"}
              >
                {e.muted ? "M" : "🔊"}
              </button>

              <span className="w-28 truncate text-xs text-foreground">
                {sourceName(e.source_id)}
              </span>

              <input
                type="range"
                min={0}
                max={200}
                step={1}
                value={Math.round(e.level * 100)}
                disabled={!editable}
                onChange={(ev) =>
                  onLevel(e.source_id, Number(ev.target.value) / 100)
                }
                className="h-1 flex-1 cursor-pointer accent-primary disabled:cursor-not-allowed"
              />
              <span className="w-10 text-right text-xs tabular-nums text-muted-foreground">
                {Math.round(e.level * 100)}%
              </span>

              {editable && (
                <button
                  onClick={() => onRemove(e.source_id)}
                  className="text-xs text-destructive hover:underline"
                >
                  Remove
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
