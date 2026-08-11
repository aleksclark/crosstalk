import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import { BroadcastShare, SessionAudioManager } from "@crosstalk/session-audio";
import type { components } from "@crosstalk/api-client";

type Session = components["schemas"]["SessionOut"];
type Channel = components["schemas"]["ChannelOut"];
type Source = components["schemas"]["SourceOut"];
type ABC = components["schemas"]["ABCOut"];
type Translator = components["schemas"]["TranslatorOut"];

type ChannelType = "feed" | "broadcast";

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { token } = useAuth();
  const [session, setSession] = useState<Session | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [abcs, setAbcs] = useState<ABC[]>([]);
  const [translators, setTranslators] = useState<Translator[]>([]);
  const [recordings, setRecordings] = useState<
    components["schemas"]["RecordingOut"][]
  >([]);
  const [loading, setLoading] = useState(true);

  // Channel CRUD form state
  const [showChannelForm, setShowChannelForm] = useState(false);
  const [editingChannelId, setEditingChannelId] = useState<string | null>(null);
  const [channelName, setChannelName] = useState("");
  const [channelType, setChannelType] = useState<ChannelType>("feed");
  const [channelBusy, setChannelBusy] = useState(false);
  const [channelError, setChannelError] = useState<string | null>(null);

  // Assignment picker state
  const [addingABC, setAddingABC] = useState(false);
  const [addingTranslator, setAddingTranslator] = useState(false);

  // A stable, authenticated client shared with SessionAudioManager so its
  // fetch/persist reuse the same error handling.
  const audioClient = useMemo(
    () => (token ? getApiClient(token) : null),
    [token],
  );

  const fetchAll = useCallback(async () => {
    if (!token || !id) return;
    const client = getApiClient(token);
    try {
      const [
        sessionRes,
        channelsRes,
        sourcesRes,
        recordingsRes,
        abcsRes,
        translatorsRes,
      ] = await Promise.all([
        client.GET("/api/sessions/{id}", { params: { path: { id } } }),
        client.GET("/api/sessions/{id}/channels", { params: { path: { id } } }),
        client.GET("/api/sessions/{id}/sources", { params: { path: { id } } }),
        client.GET("/api/sessions/{id}/recordings", { params: { path: { id } } }),
        client.GET("/api/abcs"),
        client.GET("/api/translators"),
      ]);

      if (sessionRes.data) setSession(sessionRes.data);
      const chans = channelsRes.data?.data ?? [];
      setChannels(chans);
      setSources(sourcesRes.data?.data ?? []);
      setRecordings(recordingsRes.data?.data ?? []);
      setAbcs(abcsRes.data?.data ?? []);
      setTranslators(translatorsRes.data?.data ?? []);
    } catch {
      // handle error
    } finally {
      setLoading(false);
    }
  }, [token, id]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // ── Channel CRUD ──────────────────────────────────────────────────────────
  const resetChannelForm = () => {
    setShowChannelForm(false);
    setEditingChannelId(null);
    setChannelName("");
    setChannelType("feed");
    setChannelError(null);
  };

  const openCreateChannel = () => {
    setEditingChannelId(null);
    setChannelName("");
    setChannelType("feed");
    setChannelError(null);
    setShowChannelForm(true);
  };

  const openEditChannel = (ch: Channel) => {
    setEditingChannelId(ch.id);
    setChannelName(ch.name);
    setChannelType(ch.type === "broadcast" ? "broadcast" : "feed");
    setChannelError(null);
    setShowChannelForm(true);
  };

  const handleSaveChannel = async () => {
    if (!token || !id || !channelName.trim()) return;
    setChannelBusy(true);
    setChannelError(null);
    const client = getApiClient(token);
    const body = { name: channelName.trim(), type: channelType };
    const { error } = editingChannelId
      ? await client.PUT("/api/sessions/{id}/channels/{ch_id}", {
          params: { path: { id, ch_id: editingChannelId } },
          body,
        })
      : await client.POST("/api/sessions/{id}/channels", {
          params: { path: { id } },
          body,
        });
    setChannelBusy(false);
    if (error) {
      setChannelError(error.detail || "Failed to save channel");
      return;
    }
    resetChannelForm();
    await fetchAll();
  };

  const handleDeleteChannel = async (chId: string) => {
    if (!token || !id) return;
    if (!window.confirm("Delete this channel? Its mix will be lost.")) return;
    const client = getApiClient(token);
    await client.DELETE("/api/sessions/{id}/channels/{ch_id}", {
      params: { path: { id, ch_id: chId } },
    });
    await fetchAll();
  };

  // ── ABC assignment ─────────────────────────────────────────────────────────
  const setABCSession = async (abcId: string, sessionId: string) => {
    if (!token) return;
    const client = getApiClient(token);
    await client.PUT("/api/abcs/{id}", {
      params: { path: { id: abcId } },
      body: { session_id: sessionId },
    });
    setAddingABC(false);
    await fetchAll();
  };

  // ── Translator assignment ────────────────────────────────────────────────
  const setTranslatorSessions = async (
    translatorId: string,
    sessionIds: string[],
  ) => {
    if (!token) return;
    const client = getApiClient(token);
    await client.PUT("/api/translators/{id}/sessions", {
      params: { path: { id: translatorId } },
      body: { session_ids: sessionIds },
    });
    setAddingTranslator(false);
    await fetchAll();
  };

  const assignTranslator = (t: Translator) => {
    if (!id) return;
    const next = Array.from(new Set([...(t.sessions ?? []), id]));
    void setTranslatorSessions(t.id, next);
  };

  const removeTranslator = (t: Translator) => {
    if (!id) return;
    const next = (t.sessions ?? []).filter((sid) => sid !== id);
    void setTranslatorSessions(t.id, next);
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

  const assignedABCs = abcs.filter((a) => a.session_id === id);
  const unassignedABCs = abcs.filter((a) => a.session_id !== id);
  const assignedTranslators = translators.filter((t) =>
    (t.sessions ?? []).includes(id ?? ""),
  );
  const unassignedTranslators = translators.filter(
    (t) => !(t.sessions ?? []).includes(id ?? ""),
  );

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
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Description
          </h3>
          <p className="text-sm mt-2">{session.description || "—"}</p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">Created</h3>
          <p className="text-sm mt-2">
            {new Date(session.created_at).toLocaleString()}
          </p>
        </div>
      </div>

      {/* Broadcast link + QR */}
      <div className="bg-card border border-border rounded-lg p-4">
        <h3 className="text-sm font-medium text-muted-foreground mb-3">
          Broadcast Link
        </h3>
        <BroadcastShare sessionId={session.id} token={session.broadcast_token} />
      </div>

      {/* Channels */}
      <div className="bg-card border border-border rounded-lg p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold">Channels</h2>
          <button
            onClick={openCreateChannel}
            className="bg-primary text-primary-foreground px-3 py-1.5 rounded-md text-sm font-medium hover:opacity-90"
          >
            + New Channel
          </button>
        </div>

        {showChannelForm && (
          <div className="bg-muted/40 border border-border rounded-md p-3 mb-3">
            <h3 className="text-sm font-semibold mb-3">
              {editingChannelId ? "Edit Channel" : "Create Channel"}
            </h3>
            <div className="flex items-end gap-3 flex-wrap">
              <div className="flex-1 min-w-[160px]">
                <label className="block text-xs text-muted-foreground mb-1">
                  Name
                </label>
                <input
                  type="text"
                  value={channelName}
                  onChange={(e) => setChannelName(e.target.value)}
                  placeholder="Floor Feed"
                  className="w-full bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                  autoFocus
                />
              </div>
              <div>
                <label className="block text-xs text-muted-foreground mb-1">
                  Type
                </label>
                <select
                  value={channelType}
                  onChange={(e) => setChannelType(e.target.value as ChannelType)}
                  className="bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                >
                  <option value="feed">feed</option>
                  <option value="broadcast">broadcast</option>
                </select>
              </div>
              <button
                onClick={handleSaveChannel}
                disabled={channelBusy || !channelName.trim()}
                className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90 disabled:opacity-50"
              >
                {channelBusy ? "Saving..." : editingChannelId ? "Save" : "Create"}
              </button>
              <button
                onClick={resetChannelForm}
                className="text-muted-foreground hover:text-foreground px-3 py-2 text-sm"
              >
                Cancel
              </button>
            </div>
            {channelError && (
              <p className="text-xs text-red-400 mt-2">{channelError}</p>
            )}
          </div>
        )}

        {channels.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No channels configured for this session.
          </p>
        ) : (
          <div className="space-y-2">
            {channels.map((ch) => (
              <div
                key={ch.id}
                className="flex items-center justify-between py-2 border-b border-border/50 last:border-b-0"
              >
                <span className="text-sm">
                  {ch.name}
                  <span className="text-xs text-muted-foreground ml-2 uppercase">
                    {ch.type}
                  </span>
                </span>
                <div className="flex items-center gap-3">
                  <button
                    onClick={() => openEditChannel(ch)}
                    className="text-xs text-primary hover:underline"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => handleDeleteChannel(ch.id)}
                    className="text-xs text-red-400 hover:underline"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Assignments: ABCs & Translators */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* ABCs */}
        <div className="bg-card border border-border rounded-lg p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-lg font-semibold">Audio Booth Connectors</h2>
            {!addingABC && unassignedABCs.length > 0 && (
              <button
                onClick={() => setAddingABC(true)}
                className="text-sm text-primary hover:underline"
              >
                + Assign ABC
              </button>
            )}
          </div>

          {addingABC && (
            <div className="mb-3">
              <select
                autoFocus
                defaultValue=""
                onChange={(e) => e.target.value && setABCSession(e.target.value, id ?? "")}
                onBlur={() => setAddingABC(false)}
                className="w-full bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              >
                <option value="" disabled>
                  Select an ABC to assign…
                </option>
                {unassignedABCs.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="space-y-2">
            {assignedABCs.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No ABCs assigned to this session.
              </p>
            ) : (
              assignedABCs.map((a) => (
                <div
                  key={a.id}
                  className="flex items-center justify-between py-2 border-b border-border/50 last:border-b-0"
                >
                  <span className="text-sm">
                    {a.name}
                    <span
                      className={`text-xs ml-2 ${
                        a.connected ? "text-green-400" : "text-gray-400"
                      }`}
                    >
                      {a.connected ? "Connected" : "Offline"}
                    </span>
                  </span>
                  <button
                    onClick={() => setABCSession(a.id, "")}
                    className="text-xs text-red-400 hover:underline"
                  >
                    Remove
                  </button>
                </div>
              ))
            )}
          </div>
        </div>

        {/* Translators */}
        <div className="bg-card border border-border rounded-lg p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-lg font-semibold">Translators</h2>
            {!addingTranslator && unassignedTranslators.length > 0 && (
              <button
                onClick={() => setAddingTranslator(true)}
                className="text-sm text-primary hover:underline"
              >
                + Assign Translator
              </button>
            )}
          </div>

          {addingTranslator && (
            <div className="mb-3">
              <select
                autoFocus
                defaultValue=""
                onChange={(e) => {
                  const t = unassignedTranslators.find(
                    (x) => x.id === e.target.value,
                  );
                  if (t) assignTranslator(t);
                }}
                onBlur={() => setAddingTranslator(false)}
                className="w-full bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              >
                <option value="" disabled>
                  Select a translator to assign…
                </option>
                {unassignedTranslators.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.username}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="space-y-2">
            {assignedTranslators.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No translators assigned to this session.
              </p>
            ) : (
              assignedTranslators.map((t) => (
                <div
                  key={t.id}
                  className="flex items-center justify-between py-2 border-b border-border/50 last:border-b-0"
                >
                  <span className="text-sm">{t.username}</span>
                  <button
                    onClick={() => removeTranslator(t)}
                    className="text-xs text-red-400 hover:underline"
                  >
                    Remove
                  </button>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Mix controls + monitoring */}
      <div>
        <h2 className="text-lg font-semibold mb-3">Mix &amp; Monitor</h2>
        {audioClient && id && token && (
          <SessionAudioManager
            client={audioClient}
            token={token}
            sessionId={id}
          />
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
