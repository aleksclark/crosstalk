import { useEffect, useState, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import { MixPanel } from "../components/MixPanel";
import type { components } from "@crosstalk/api-client";

type Session = components["schemas"]["SessionOut"];
type Channel = components["schemas"]["ChannelOut"];
type Source = components["schemas"]["SourceOut"];
type MixEntry = components["schemas"]["MixEntryOut"];

interface PanelSource {
  id: string;
  name: string;
  level: number;
  muted: boolean;
  volume: number;
}

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { token } = useAuth();
  const [session, setSession] = useState<Session | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [mixByChannel, setMixByChannel] = useState<Record<string, MixEntry[]>>({});
  const [recordings, setRecordings] = useState<
    components["schemas"]["RecordingOut"][]
  >([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchAll() {
      if (!token || !id) return;
      const client = getApiClient(token);
      try {
        const [sessionRes, channelsRes, sourcesRes, recordingsRes] =
          await Promise.all([
            client.GET("/api/sessions/{id}", { params: { path: { id } } }),
            client.GET("/api/sessions/{id}/channels", {
              params: { path: { id } },
            }),
            client.GET("/api/sessions/{id}/sources", {
              params: { path: { id } },
            }),
            client.GET("/api/sessions/{id}/recordings", {
              params: { path: { id } },
            }),
          ]);

        if (sessionRes.data) setSession(sessionRes.data);
        const chans = channelsRes.data?.data ?? [];
        setChannels(chans);
        setSources(sourcesRes.data?.data ?? []);
        setRecordings(recordingsRes.data?.data ?? []);

        const mixEntries = await Promise.all(
          chans.map((ch) =>
            client.GET("/api/sessions/{id}/channels/{ch_id}/mix", {
              params: { path: { id, ch_id: ch.id } },
            }),
          ),
        );
        const mixMap: Record<string, MixEntry[]> = {};
        chans.forEach((ch, i) => {
          mixMap[ch.id] = mixEntries[i].data?.data ?? [];
        });
        setMixByChannel(mixMap);
      } catch {
        // handle error
      } finally {
        setLoading(false);
      }
    }
    fetchAll();
  }, [token, id]);

  const persistMix = useCallback(
    async (channelId: string, entries: MixEntry[]) => {
      if (!token || !id) return;
      const client = getApiClient(token);
      await client.PUT("/api/sessions/{id}/channels/{ch_id}/mix", {
        params: { path: { id, ch_id: channelId } },
        body: {
          entries: entries.map((e) => ({
            source_id: e.source_id,
            muted: e.muted,
            level: e.level,
          })),
        },
      });
    },
    [token, id],
  );

  const handleMuteToggle = useCallback(
    (channelId: string, sourceId: string, muted: boolean) => {
      setMixByChannel((prev) => {
        const next = (prev[channelId] ?? []).map((e) =>
          e.source_id === sourceId ? { ...e, muted } : e,
        );
        void persistMix(channelId, next);
        return { ...prev, [channelId]: next };
      });
    },
    [persistMix],
  );

  const handleVolumeChange = useCallback(
    (channelId: string, sourceId: string, volume: number) => {
      setMixByChannel((prev) => {
        const next = (prev[channelId] ?? []).map((e) =>
          e.source_id === sourceId ? { ...e, level: volume / 50 } : e,
        );
        void persistMix(channelId, next);
        return { ...prev, [channelId]: next };
      });
    },
    [persistMix],
  );

  const panelSourcesFor = (channelId: string): PanelSource[] => {
    const entries = mixByChannel[channelId] ?? [];
    return entries.map((e) => {
      const src = sources.find((s) => s.id === e.source_id);
      return {
        id: e.source_id,
        name: src?.name ?? e.source_id.slice(0, 8),
        level: 0,
        muted: e.muted,
        volume: Math.round(e.level * 50),
      };
    });
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading session...</p>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">Session not found</p>
        <Link to="/sessions" className="text-primary text-sm mt-2 inline-block">
          ← Back to sessions
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <Link
              to="/sessions"
              className="text-muted-foreground hover:text-foreground text-sm"
            >
              Sessions /
            </Link>
            <h1 className="text-2xl font-bold">{session.name}</h1>
          </div>
          <p className="text-muted-foreground text-sm mt-1">ID: {session.id}</p>
        </div>
      </div>

      {/* Session info cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Description
          </h3>
          <p className="text-sm mt-2">
            {session.description || "—"}
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Created
          </h3>
          <p className="text-sm mt-2">
            {new Date(session.created_at).toLocaleString()}
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Broadcast URL
          </h3>
          <p className="text-sm font-mono mt-2 text-primary truncate">
            /session/{session.id}/broadcast
          </p>
        </div>
      </div>

      {/* Mix controls */}
      <div>
        <h2 className="text-lg font-semibold mb-3">Mix Controls</h2>
        {channels.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No channels configured for this session.
          </p>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {channels.map((ch) => (
              <MixPanel
                key={ch.id}
                channelName={`${ch.name} — ${ch.type}`}
                sources={panelSourcesFor(ch.id)}
                onMuteToggle={(sourceId, muted) =>
                  handleMuteToggle(ch.id, sourceId, muted)
                }
                onVolumeChange={(sourceId, volume) =>
                  handleVolumeChange(ch.id, sourceId, volume)
                }
              />
            ))}
          </div>
        )}
      </div>

      {/* Sources & Recordings */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-card border border-border rounded-lg p-4">
          <h2 className="text-lg font-semibold mb-3">Audio Sources</h2>
          <div className="space-y-2">
            {sources.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No audio sources connected.
              </p>
            ) : (
              sources.map((src) => (
                <div
                  key={src.id}
                  className="flex items-center justify-between py-2 border-b border-border/50 last:border-b-0"
                >
                  <span className="text-sm">
                    {src.name}
                    <span className="text-xs text-muted-foreground ml-2">
                      {src.origin}
                    </span>
                  </span>
                  <span
                    className={`text-xs ${
                      src.connected ? "text-green-400" : "text-gray-400"
                    }`}
                  >
                    {src.connected ? "Connected" : "Offline"}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="bg-card border border-border rounded-lg p-4">
          <h2 className="text-lg font-semibold mb-3">Recordings</h2>
          <div className="space-y-2 text-sm">
            {recordings.length === 0 ? (
              <p className="text-muted-foreground">
                No recordings for this session.
              </p>
            ) : (
              recordings.map((rec) => (
                <div
                  key={rec.id}
                  className="flex items-center justify-between py-2 border-b border-border/50 last:border-b-0"
                >
                  <span className="font-mono text-xs truncate">
                    {rec.file_path}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {(rec.size_bytes / 1024 / 1024).toFixed(1)} MB
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
