import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import type { createApiClient, components } from "@crosstalk/api-client";
import {
  Button,
  CopyableId,
  DataState,
  Field,
  IconButton,
  Status,
  type StatusTone,
} from "@crosstalk/theme";
import { ChannelMonitor } from "./ChannelMonitor";

type Client = ReturnType<typeof createApiClient>;
type Channel = components["schemas"]["ChannelOut"];
type Source = components["schemas"]["SourceOut"];
type MixEntry = components["schemas"]["MixEntryOut"];
type ABC = components["schemas"]["ABCOut"];

type LoadState = "loading" | "ready" | "error" | "denied";
type SaveState = "idle" | "saving" | "saved" | "failed";

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
  style?: CSSProperties;
}

function httpStatus(err: unknown): number | undefined {
  if (err && typeof err === "object" && "status" in err) {
    const s = (err as { status?: unknown }).status;
    return typeof s === "number" ? s : undefined;
  }
  return undefined;
}

function statusFromResponse(res: { response?: { status?: number }; error?: unknown }): number | undefined {
  return res.response?.status ?? httpStatus(res.error);
}

// SessionAudioManager is the shared UI for wiring a session's audio. For each
// channel it:
//   - always opens a receive-only monitor with its own volume, mute and VU
//     meter (the VU meter reflects the incoming signal regardless of mute or
//     volume), and
//   - lets the operator edit the mix: which sources feed the channel, each with
//     mute and level.
// It also lets booth boards (ABCs) pick which channel they monitor.
//
// Mix mutations are optimistically reflected in the UI, then coalesced and
// serialized per channel. "Saved" is only shown after a successful server ack;
// failures expose retry without dropping the latest intended level.
export function SessionAudioManager({
  client,
  token,
  sessionId,
  editable = true,
  monitor = true,
  showABCMonitors = true,
  baseUrl,
  className,
  style,
}: SessionAudioManagerProps) {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [abcs, setAbcs] = useState<ABC[]>([]);
  const [mixByChannel, setMixByChannel] = useState<Record<string, MixEntry[]>>(
    {},
  );
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [mixSaveByChannel, setMixSaveByChannel] = useState<
    Record<string, SaveState>
  >({});
  const [abcSaveById, setAbcSaveById] = useState<Record<string, SaveState>>({});

  // Latest desired mix payload per channel (coalesced).
  const pendingMixRef = useRef<Record<string, MixEntry[]>>({});
  // True while a PUT is in flight for the channel.
  const writingMixRef = useRef<Record<string, boolean>>({});
  // When true, pending holds a failed payload awaiting explicit retry
  // (or a newer edit that clears the flag).
  const mixFailedRef = useRef<Record<string, boolean>>({});

  const setMixSave = useCallback((channelId: string, state: SaveState) => {
    setMixSaveByChannel((prev) =>
      prev[channelId] === state ? prev : { ...prev, [channelId]: state },
    );
  }, []);

  const persistMix = useCallback(
    async (channelId: string, entries: MixEntry[]) => {
      const res = await client.PUT("/api/sessions/{id}/channels/{ch_id}/mix", {
        params: { path: { id: sessionId, ch_id: channelId } },
        body: {
          entries: entries.map((e) => ({
            source_id: e.source_id,
            muted: e.muted,
            level: e.level,
          })),
        },
      });
      if (res.error) {
        const status = statusFromResponse(res);
        throw Object.assign(new Error("Failed to save mix"), { status });
      }
    },
    [client, sessionId],
  );

  const drainMix = useCallback(
    async (channelId: string) => {
      if (writingMixRef.current[channelId]) return;
      writingMixRef.current[channelId] = true;
      try {
        while (pendingMixRef.current[channelId] !== undefined) {
          // Do not auto-loop a failed payload; wait for retry or a newer queue.
          if (mixFailedRef.current[channelId]) break;

          const payload = pendingMixRef.current[channelId]!;
          // Clear before await so concurrent edits during the request re-queue.
          delete pendingMixRef.current[channelId];
          setMixSave(channelId, "saving");
          try {
            await persistMix(channelId, payload);
            // Only claim saved when nothing newer is pending.
            if (pendingMixRef.current[channelId] === undefined) {
              setMixSave(channelId, "saved");
            }
          } catch {
            if (pendingMixRef.current[channelId] === undefined) {
              // No newer edit — park the failed payload for retry.
              pendingMixRef.current[channelId] = payload;
              mixFailedRef.current[channelId] = true;
              setMixSave(channelId, "failed");
              break;
            }
            // A newer edit arrived while this write failed; loop to send it.
          }
        }
      } finally {
        writingMixRef.current[channelId] = false;
        // Cover the race where a queue arrived after the last pending check
        // but before the write lock was released.
        if (
          pendingMixRef.current[channelId] !== undefined &&
          !mixFailedRef.current[channelId]
        ) {
          void drainMix(channelId);
        }
      }
    },
    [persistMix, setMixSave],
  );

  const queueMixWrite = useCallback(
    (channelId: string, next: MixEntry[]) => {
      setMixByChannel((prev) => ({ ...prev, [channelId]: next }));
      pendingMixRef.current[channelId] = next;
      mixFailedRef.current[channelId] = false;
      // Reflect pending work immediately; do not claim saved.
      setMixSave(channelId, "saving");
      void drainMix(channelId);
    },
    [drainMix, setMixSave],
  );

  const retryMix = useCallback(
    (channelId: string) => {
      const pending =
        pendingMixRef.current[channelId] ?? mixByChannel[channelId];
      if (!pending) return;
      pendingMixRef.current[channelId] = pending;
      mixFailedRef.current[channelId] = false;
      setMixSave(channelId, "saving");
      void drainMix(channelId);
    },
    [drainMix, mixByChannel, setMixSave],
  );

  const reload = useCallback(async () => {
    setLoadState("loading");
    setLoadError(null);
    try {
      const [channelsRes, sourcesRes, abcsRes] = await Promise.all([
        client.GET("/api/sessions/{id}/channels", {
          params: { path: { id: sessionId } },
        }),
        client.GET("/api/sessions/{id}/sources", {
          params: { path: { id: sessionId } },
        }),
        client.GET("/api/abcs"),
      ]);

      const denied = [channelsRes, sourcesRes, abcsRes].some((r) => {
        const s = statusFromResponse(r);
        return s === 401 || s === 403;
      });
      if (denied) {
        setLoadState("denied");
        setLoadError("You do not have access to session audio for this session.");
        return;
      }

      const failed = [channelsRes, sourcesRes, abcsRes].find((r) => r.error);
      if (failed) {
        setLoadState("error");
        setLoadError("Could not load session channels, sources, or booth boards.");
        return;
      }

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
      const mixDenied = mixes.some((r) => {
        const s = statusFromResponse(r);
        return s === 401 || s === 403;
      });
      if (mixDenied) {
        setLoadState("denied");
        setLoadError("You do not have access to mix configuration for this session.");
        return;
      }
      if (mixes.some((r) => r.error)) {
        setLoadState("error");
        setLoadError("Could not load channel mix configuration.");
        return;
      }

      const map: Record<string, MixEntry[]> = {};
      chans.forEach((ch, i) => {
        map[ch.id] = mixes[i]?.data?.data ?? [];
      });
      setMixByChannel(map);
      setLoadState("ready");
    } catch (err) {
      setLoadState("error");
      setLoadError(
        err instanceof Error ? err.message : "Could not load session audio.",
      );
    }
  }, [client, sessionId]);

  useEffect(() => {
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
      setAbcSaveById((prev) => ({ ...prev, [abcId]: "saving" }));
      try {
        const res = await client.PUT("/api/abcs/{id}", {
          params: { path: { id: abcId } },
          body: { monitor_channel_id: channelId ?? "" },
        });
        if (res.error) {
          throw Object.assign(new Error("Failed to update booth monitor"), {
            status: statusFromResponse(res),
          });
        }
        setAbcSaveById((prev) => ({ ...prev, [abcId]: "saved" }));
      } catch {
        setAbcSaveById((prev) => ({ ...prev, [abcId]: "failed" }));
      }
    },
    [client],
  );

  const assignSource = useCallback(
    (channelId: string, sourceId: string) => {
      const current = mixByChannel[channelId] ?? [];
      if (current.some((e) => e.source_id === sourceId)) return;
      queueMixWrite(channelId, [
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
    [mixByChannel, queueMixWrite],
  );

  const removeSource = useCallback(
    (channelId: string, sourceId: string) => {
      const current = mixByChannel[channelId] ?? [];
      queueMixWrite(
        channelId,
        current.filter((e) => e.source_id !== sourceId),
      );
    },
    [mixByChannel, queueMixWrite],
  );

  const setMuted = useCallback(
    (channelId: string, sourceId: string, muted: boolean) => {
      const current = mixByChannel[channelId] ?? [];
      queueMixWrite(
        channelId,
        current.map((e) => (e.source_id === sourceId ? { ...e, muted } : e)),
      );
    },
    [mixByChannel, queueMixWrite],
  );

  const setLevel = useCallback(
    (channelId: string, sourceId: string, level: number) => {
      const current = mixByChannel[channelId] ?? [];
      queueMixWrite(
        channelId,
        current.map((e) => (e.source_id === sourceId ? { ...e, level } : e)),
      );
    },
    [mixByChannel, queueMixWrite],
  );

  const sourceById = useMemo(() => {
    const map = new Map<string, Source>();
    for (const s of sources) map.set(s.id, s);
    return map;
  }, [sources]);

  const containerStyle: CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: "var(--house-space-4)",
    ...style,
  };

  if (loadState === "loading") {
    return (
      <div className={className} style={containerStyle}>
        <DataState
          kind="loading"
          title="Loading session audio"
          description="Fetching channels, sources, and mix configuration."
        />
      </div>
    );
  }

  if (loadState === "denied") {
    return (
      <div className={className} style={containerStyle}>
        <DataState
          kind="denied"
          title="Access denied"
          description={loadError ?? "You cannot view session audio."}
          action={
            <Button variant="secondary" onClick={() => void reload()}>
              Retry
            </Button>
          }
        />
      </div>
    );
  }

  if (loadState === "error") {
    return (
      <div className={className} style={containerStyle}>
        <DataState
          kind="error"
          title="Session audio unavailable"
          description={loadError ?? "Something went wrong loading audio."}
          action={
            <Button variant="secondary" onClick={() => void reload()}>
              Retry
            </Button>
          }
        />
      </div>
    );
  }

  return (
    <div className={className} style={containerStyle}>
      {channels.length === 0 ? (
        <DataState
          kind="empty"
          title="No channels"
          description="No channels are configured for this session yet."
        />
      ) : (
        channels.map((ch) => (
          <ChannelSection
            key={ch.id}
            channel={ch}
            entries={mixByChannel[ch.id] ?? []}
            sources={sources}
            sourceById={sourceById}
            editable={editable}
            saveState={mixSaveByChannel[ch.id] ?? "idle"}
            onRetry={() => retryMix(ch.id)}
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
        <section
          style={{
            borderTop: "1px solid var(--house-rule-subtle)",
            paddingTop: "var(--house-space-4)",
            display: "flex",
            flexDirection: "column",
            gap: "var(--house-space-3)",
          }}
        >
          <header
            style={{
              display: "flex",
              flexWrap: "wrap",
              alignItems: "baseline",
              justifyContent: "space-between",
              gap: "var(--house-space-2)",
            }}
          >
            <h3 className="house-type-section" style={{ margin: 0 }}>
              Booth monitors
            </h3>
            <p
              style={{
                margin: 0,
                font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
                color: "var(--house-text-tertiary)",
              }}
            >
              Changing a monitor reconnects the booth board to apply the new
              channel.
            </p>
          </header>

          <ul
            style={{
              listStyle: "none",
              margin: 0,
              padding: 0,
              display: "flex",
              flexDirection: "column",
            }}
          >
            {abcs.map((abc) => {
              const save = abcSaveById[abc.id] ?? "idle";
              return (
                <li
                  key={abc.id}
                  style={{
                    display: "grid",
                    gridTemplateColumns: "minmax(0, 1fr) minmax(12rem, 18rem) auto",
                    gap: "var(--house-space-3)",
                    alignItems: "center",
                    padding: "var(--house-space-3) 0",
                    borderBottom: "1px solid var(--house-rule-subtle)",
                    minHeight: "var(--house-row-height)",
                  }}
                >
                  <div style={{ minWidth: 0 }}>
                    <div
                      style={{
                        font: "500 var(--house-type-body) / var(--house-leading-body) var(--house-font-product)",
                        color: "var(--house-text-primary)",
                      }}
                    >
                      {abc.name}
                    </div>
                    <div
                      style={{
                        display: "flex",
                        flexWrap: "wrap",
                        alignItems: "center",
                        gap: "var(--house-space-2)",
                        marginTop: "var(--house-space-1)",
                      }}
                    >
                      <CopyableId value={abc.id} label={`Copy booth ID for ${abc.name}`} />
                      <Status tone={abc.connected ? "ok" : "neutral"}>
                        {abc.connected ? "Connected" : "Offline"}
                      </Status>
                      <SaveStatus state={save} onRetry={() => void setABCMonitor(abc.id, abc.monitor_channel_id ?? null)} />
                    </div>
                  </div>
                  <Field
                    as="select"
                    label={`Monitor channel for ${abc.name}`}
                    value={abc.monitor_channel_id ?? ""}
                    disabled={!editable || save === "saving"}
                    onChange={(e) =>
                      void setABCMonitor(abc.id, e.target.value || null)
                    }
                  >
                    <option value="">None (not monitoring)</option>
                    {channels.map((ch) => (
                      <option key={ch.id} value={ch.id}>
                        {ch.name} — {ch.type}
                      </option>
                    ))}
                  </Field>
                  <span aria-hidden="true" />
                </li>
              );
            })}
          </ul>
        </section>
      )}
    </div>
  );
}

function SaveStatus({
  state,
  onRetry,
}: {
  state: SaveState;
  onRetry?: () => void;
}) {
  if (state === "idle") return null;
  if (state === "saving") {
    return <Status tone="info">Saving</Status>;
  }
  if (state === "saved") {
    return <Status tone="ok">Saved</Status>;
  }
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--house-space-2)",
      }}
    >
      <Status tone="danger">Save failed</Status>
      {onRetry ? (
        <Button variant="ghost" size="sm" onClick={onRetry}>
          Retry
        </Button>
      ) : null}
    </span>
  );
}

interface ChannelSectionProps {
  channel: Channel;
  entries: MixEntry[];
  sources: Source[];
  sourceById: Map<string, Source>;
  editable: boolean;
  saveState: SaveState;
  onRetry: () => void;
  onAssign: (sourceId: string) => void;
  onRemove: (sourceId: string) => void;
  onMute: (sourceId: string, muted: boolean) => void;
  onLevel: (sourceId: string, level: number) => void;
  monitor: ReactNode;
}

function channelTypeTone(type: string): StatusTone {
  switch (type) {
    case "floor":
      return "info";
    case "feed":
      return "ok";
    case "broadcast":
      return "warning";
    default:
      return "neutral";
  }
}

function ChannelSection({
  channel,
  entries,
  sources,
  sourceById,
  editable,
  saveState,
  onRetry,
  onAssign,
  onRemove,
  onMute,
  onLevel,
  monitor,
}: ChannelSectionProps) {
  const assignedIds = useMemo(
    () => new Set(entries.map((e) => e.source_id)),
    [entries],
  );
  const available = sources.filter((s) => !assignedIds.has(s.id));

  return (
    <section
      style={{
        borderTop: "1px solid var(--house-rule-strong)",
        paddingTop: "var(--house-space-4)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--house-space-3)",
      }}
    >
      <header
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: "var(--house-space-3)",
        }}
      >
        <div style={{ minWidth: 0, flex: "1 1 auto" }}>
          <h3
            className="house-type-section"
            style={{ margin: 0, color: "var(--house-text-primary)" }}
          >
            {channel.name}
          </h3>
          <div
            style={{
              display: "flex",
              flexWrap: "wrap",
              alignItems: "center",
              gap: "var(--house-space-2)",
              marginTop: "var(--house-space-2)",
            }}
          >
            <Status tone={channelTypeTone(channel.type)}>{channel.type}</Status>
            <CopyableId
              value={channel.id}
              label={`Copy channel ID for ${channel.name}`}
            />
            <SaveStatus state={saveState} onRetry={onRetry} />
          </div>
        </div>

        {editable && available.length > 0 ? (
          <Field
            as="select"
            label={`Assign source to ${channel.name}`}
            defaultValue=""
            onChange={(e) => {
              if (e.target.value) {
                onAssign(e.target.value);
                e.target.value = "";
              }
            }}
            style={{ minWidth: "12rem", flex: "0 1 16rem" }}
          >
            <option value="">Assign source…</option>
            {available.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Field>
        ) : null}
      </header>

      {monitor ? (
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--house-space-2)",
            paddingBottom: "var(--house-space-3)",
            borderBottom: "1px solid var(--house-rule-subtle)",
          }}
        >
          <div className="house-type-label" style={{ color: "var(--house-text-tertiary)" }}>
            Local monitor
          </div>
          {monitor}
        </div>
      ) : null}

      <div
        style={{
          display: "flex",
          flexDirection: "column",
          gap: "var(--house-space-2)",
        }}
      >
        <div className="house-type-label" style={{ color: "var(--house-text-tertiary)" }}>
          Mix sources
        </div>
        {entries.length === 0 ? (
          <p
            style={{
              margin: 0,
              font: "400 var(--house-type-body) / var(--house-leading-body) var(--house-font-product)",
              color: "var(--house-text-tertiary)",
            }}
          >
            No sources assigned.
          </p>
        ) : (
          <ul
            style={{
              listStyle: "none",
              margin: 0,
              padding: 0,
              display: "flex",
              flexDirection: "column",
            }}
          >
            {entries.map((e) => {
              const src = sourceById.get(e.source_id);
              const name = src?.name ?? "Unknown source";
              const levelPct = Math.round(e.level * 100);
              return (
                <li
                  key={e.source_id}
                  style={{
                    display: "grid",
                    gridTemplateColumns:
                      "var(--house-control-height) minmax(0, 1fr) minmax(8rem, 1fr) 3rem auto",
                    gap: "var(--house-space-3)",
                    alignItems: "center",
                    padding: "var(--house-space-2) 0",
                    borderBottom: "1px solid var(--house-rule-subtle)",
                    minHeight: "var(--house-row-height)",
                  }}
                >
                  <IconButton
                    icon={e.muted ? "mute" : "volume"}
                    label={
                      e.muted
                        ? `Unmute ${name} in ${channel.name}`
                        : `Mute ${name} in ${channel.name}`
                    }
                    variant={e.muted ? "destructive" : "secondary"}
                    disabled={!editable}
                    aria-pressed={e.muted}
                    onClick={() => editable && onMute(e.source_id, !e.muted)}
                  />

                  <div style={{ minWidth: 0 }}>
                    <div
                      style={{
                        font: "500 var(--house-type-body) / var(--house-leading-body) var(--house-font-product)",
                        color: "var(--house-text-primary)",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {name}
                    </div>
                    <CopyableId
                      value={e.source_id}
                      label={`Copy source ID for ${name}`}
                    />
                  </div>

                  <label
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "var(--house-space-2)",
                      minWidth: 0,
                    }}
                  >
                    <span className="house-visually-hidden">
                      Level for {name} in {channel.name}
                    </span>
                    <input
                      type="range"
                      min={0}
                      max={200}
                      step={1}
                      value={levelPct}
                      disabled={!editable}
                      onChange={(ev) =>
                        onLevel(e.source_id, Number(ev.target.value) / 100)
                      }
                      aria-valuemin={0}
                      aria-valuemax={200}
                      aria-valuenow={levelPct}
                      aria-valuetext={`${levelPct} percent`}
                      style={{
                        width: "100%",
                        minHeight: "var(--house-control-height)",
                        accentColor: "var(--house-accent)",
                        cursor: editable ? "pointer" : "not-allowed",
                      }}
                    />
                  </label>

                  <span
                    style={{
                      font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
                      color: "var(--house-text-tertiary)",
                      textAlign: "right",
                      fontVariantNumeric: "tabular-nums",
                    }}
                  >
                    {levelPct}%
                  </span>

                  {editable ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      icon="trash"
                      onClick={() => onRemove(e.source_id)}
                      aria-label={`Remove ${name} from ${channel.name}`}
                    >
                      Remove
                    </Button>
                  ) : (
                    <span />
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}
