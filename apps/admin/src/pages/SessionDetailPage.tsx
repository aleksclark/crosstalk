import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Button,
  CopyableId,
  DataState,
  Field,
  Modal,
  PageHeader,
  Status,
} from "@crosstalk/theme";
import { BroadcastShare, SessionAudioManager } from "@crosstalk/session-audio";
import type { components } from "@crosstalk/api-client";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";

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
  const [loadError, setLoadError] = useState<string | null>(null);

  const [showChannelForm, setShowChannelForm] = useState(false);
  const [editingChannelId, setEditingChannelId] = useState<string | null>(null);
  const [channelName, setChannelName] = useState("");
  const [channelType, setChannelType] = useState<ChannelType>("feed");
  const [channelBusy, setChannelBusy] = useState(false);
  const [channelError, setChannelError] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<Channel | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const [addingABC, setAddingABC] = useState(false);
  const [addingTranslator, setAddingTranslator] = useState(false);

  const audioClient = useMemo(
    () => (token ? getApiClient(token) : null),
    [token],
  );

  const fetchAll = useCallback(async () => {
    if (!token || !id) return;
    const controller = new AbortController();
    setLoadError(null);
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
        client.GET("/api/sessions/{id}", {
          params: { path: { id } },
          signal: controller.signal,
        }),
        client.GET("/api/sessions/{id}/channels", {
          params: { path: { id } },
          signal: controller.signal,
        }),
        client.GET("/api/sessions/{id}/sources", {
          params: { path: { id } },
          signal: controller.signal,
        }),
        client.GET("/api/sessions/{id}/recordings", {
          params: { path: { id } },
          signal: controller.signal,
        }),
        client.GET("/api/abcs", {
          params: { query: { limit: 100 } },
          signal: controller.signal,
        }),
        client.GET("/api/translators", {
          params: { query: { limit: 100 } },
          signal: controller.signal,
        }),
      ]);

      if (controller.signal.aborted) return;

      if (sessionRes.error || !sessionRes.data) {
        setSession(null);
        setLoadError(sessionRes.error?.detail || "Session not found");
        return;
      }
      setSession(sessionRes.data);
      setChannels(channelsRes.data?.data ?? []);
      setSources(sourcesRes.data?.data ?? []);
      setRecordings(recordingsRes.data?.data ?? []);
      setAbcs(abcsRes.data?.data ?? []);
      setTranslators(translatorsRes.data?.data ?? []);
    } catch (err) {
      if (controller.signal.aborted) return;
      setLoadError(err instanceof Error ? err.message : "Failed to load session");
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [token, id]);

  useEffect(() => {
    setLoading(true);
    void fetchAll();
  }, [fetchAll]);

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

  const confirmDeleteChannel = async () => {
    if (!token || !id || !deleteTarget) return;
    setDeleteBusy(true);
    setDeleteError(null);
    const client = getApiClient(token);
    const { error } = await client.DELETE("/api/sessions/{id}/channels/{ch_id}", {
      params: { path: { id, ch_id: deleteTarget.id } },
    });
    setDeleteBusy(false);
    if (error) {
      setDeleteError(error.detail || "Failed to delete channel");
      return;
    }
    setDeleteTarget(null);
    await fetchAll();
  };

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
      <DataState
        kind="loading"
        title="Loading session"
        description="Fetching session metadata, channels, and assignments."
      />
    );
  }

  if (!session) {
    return (
      <DataState
        kind={loadError?.toLowerCase().includes("not found") ? "empty" : "error"}
        title="Session unavailable"
        description={loadError ?? "Session not found"}
        action={
          <Link to="/sessions" className="text-sm text-primary hover:underline">
            ← Back to sessions
          </Link>
        }
      />
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

  const sectionClass = "border border-border bg-[var(--house-bg-surface)] p-4";

  return (
    <div className="space-y-6">
      <div className="sticky top-0 z-10 -mx-4 border-b border-border bg-background/95 px-4 py-3 backdrop-blur supports-[backdrop-filter]:bg-background/80 md:-mx-6 md:px-6">
        <PageHeader
          eyebrow="Sessions"
          title={session.name}
          lede={session.description || undefined}
          style={{ marginBottom: 0 }}
          meta={
            <>
              <span className="inline-flex items-center gap-2">
                ID <CopyableId value={session.id} />
              </span>
              <span>Created {new Date(session.created_at).toLocaleString()}</span>
              <Link to="/sessions" className="text-primary hover:underline">
                All sessions
              </Link>
            </>
          }
        />
      </div>

      <section className={sectionClass} aria-labelledby="broadcast-heading">
        <h2 id="broadcast-heading" className="house-type-section mb-3">
          Broadcast link
        </h2>
        <BroadcastShare sessionId={session.id} token={session.broadcast_token} />
      </section>

      <section className={sectionClass} aria-labelledby="channels-heading">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 id="channels-heading" className="house-type-section">
            Channels
          </h2>
          <Button variant="primary" size="sm" onClick={openCreateChannel}>
            + New Channel
          </Button>
        </div>

        {showChannelForm ? (
          <div className="mb-4 space-y-3 border border-border bg-[var(--house-bg-raised)] p-3">
            <h3 className="house-type-label">
              {editingChannelId ? "Edit channel" : "Create channel"}
            </h3>
            <div className="flex flex-col gap-3 md:flex-row md:items-end">
              <div className="min-w-0 flex-1">
                <Field
                  label="Name"
                  value={channelName}
                  onChange={(e) => setChannelName(e.target.value)}
                  placeholder="Floor Feed"
                  autoFocus
                />
              </div>
              <div className="w-full md:w-40">
                <Field
                  as="select"
                  label="Type"
                  value={channelType}
                  onChange={(e) => setChannelType(e.target.value as ChannelType)}
                >
                  <option value="feed">feed</option>
                  <option value="broadcast">broadcast</option>
                </Field>
              </div>
              <Button
                variant="primary"
                loading={channelBusy}
                disabled={channelBusy || !channelName.trim()}
                onClick={() => void handleSaveChannel()}
              >
                {channelBusy ? "Saving..." : editingChannelId ? "Save" : "Create"}
              </Button>
              <Button variant="ghost" onClick={resetChannelForm}>
                Cancel
              </Button>
            </div>
            {channelError ? (
              <p role="alert" className="house-type-meta text-[var(--house-status-danger)]">
                {channelError}
              </p>
            ) : null}
          </div>
        ) : null}

        {channels.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No channels configured for this session.
          </p>
        ) : (
          <ul className="divide-y divide-border border-y border-border">
            {channels.map((ch) => (
              <li
                key={ch.id}
                className="flex flex-wrap items-center justify-between gap-3 py-3"
              >
                <div>
                  <p className="text-sm font-medium">{ch.name}</p>
                  <p className="house-type-meta uppercase text-muted-foreground">
                    {ch.type}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => openEditChannel(ch)}
                    className="text-xs text-primary hover:underline"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setDeleteError(null);
                      setDeleteTarget(ch);
                    }}
                    className="text-xs text-[var(--house-status-danger)] hover:underline"
                  >
                    Delete
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <section className={sectionClass} aria-labelledby="abc-assign-heading">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 id="abc-assign-heading" className="house-type-section">
              Audio Booth Connectors
            </h2>
            {!addingABC && unassignedABCs.length > 0 ? (
              <button
                type="button"
                onClick={() => setAddingABC(true)}
                className="text-sm text-primary hover:underline"
              >
                + Assign ABC
              </button>
            ) : null}
          </div>

          {addingABC ? (
            <div className="mb-3">
              <Field
                as="select"
                label="Assign ABC"
                autoFocus
                defaultValue=""
                onChange={(e) => {
                  if (e.target.value) void setABCSession(e.target.value, id ?? "");
                }}
                onBlur={() => setAddingABC(false)}
              >
                <option value="" disabled>
                  Select an ABC to assign…
                </option>
                {unassignedABCs.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </Field>
            </div>
          ) : null}

          {assignedABCs.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No ABCs assigned to this session.
            </p>
          ) : (
            <ul className="divide-y divide-border border-y border-border">
              {assignedABCs.map((a) => (
                <li
                  key={a.id}
                  className="flex items-center justify-between gap-3 py-2"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium">{a.name}</span>
                    <Status tone={a.connected ? "ok" : "neutral"}>
                      {a.connected ? "Connected" : "Offline"}
                    </Status>
                  </div>
                  <button
                    type="button"
                    onClick={() => void setABCSession(a.id, "")}
                    className="text-xs text-[var(--house-status-danger)] hover:underline"
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className={sectionClass} aria-labelledby="xl8-assign-heading">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 id="xl8-assign-heading" className="house-type-section">
              Translators
            </h2>
            {!addingTranslator && unassignedTranslators.length > 0 ? (
              <button
                type="button"
                onClick={() => setAddingTranslator(true)}
                className="text-sm text-primary hover:underline"
              >
                + Assign Translator
              </button>
            ) : null}
          </div>

          {addingTranslator ? (
            <div className="mb-3">
              <Field
                as="select"
                label="Assign translator"
                autoFocus
                defaultValue=""
                onChange={(e) => {
                  const t = unassignedTranslators.find((x) => x.id === e.target.value);
                  if (t) assignTranslator(t);
                }}
                onBlur={() => setAddingTranslator(false)}
              >
                <option value="" disabled>
                  Select a translator to assign…
                </option>
                {unassignedTranslators.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.username}
                  </option>
                ))}
              </Field>
            </div>
          ) : null}

          {assignedTranslators.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No translators assigned to this session.
            </p>
          ) : (
            <ul className="divide-y divide-border border-y border-border">
              {assignedTranslators.map((t) => (
                <li
                  key={t.id}
                  className="flex items-center justify-between gap-3 py-2"
                >
                  <span className="text-sm font-medium">{t.username}</span>
                  <button
                    type="button"
                    onClick={() => removeTranslator(t)}
                    className="text-xs text-[var(--house-status-danger)] hover:underline"
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      <section aria-labelledby="mix-heading">
        <h2 id="mix-heading" className="house-type-section mb-3">
          Mix &amp; Monitor
        </h2>
        {audioClient && id && token ? (
          <SessionAudioManager
            client={audioClient}
            token={token}
            sessionId={id}
          />
        ) : null}
      </section>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <section className={sectionClass} aria-labelledby="sources-heading">
          <h2 id="sources-heading" className="house-type-section mb-3">
            Audio sources
          </h2>
          {sources.length === 0 ? (
            <p className="text-sm text-muted-foreground">No audio sources connected.</p>
          ) : (
            <ul className="divide-y divide-border border-y border-border">
              {sources.map((src) => (
                <li
                  key={src.id}
                  className="flex items-center justify-between gap-3 py-2"
                >
                  <span className="text-sm">
                    {src.name}
                    <span className="ml-2 house-type-meta text-muted-foreground">
                      {src.origin}
                    </span>
                  </span>
                  <Status tone={src.connected ? "ok" : "neutral"}>
                    {src.connected ? "Connected" : "Offline"}
                  </Status>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className={sectionClass} aria-labelledby="recordings-heading">
          <h2 id="recordings-heading" className="house-type-section mb-3">
            Recordings
          </h2>
          {recordings.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No recordings for this session.
            </p>
          ) : (
            <ul className="divide-y divide-border border-y border-border">
              {recordings.map((rec) => (
                <li
                  key={rec.id}
                  className="flex items-center justify-between gap-3 py-2 text-sm"
                >
                  <span className="house-type-code truncate">{rec.file_path}</span>
                  <span className="house-type-meta text-muted-foreground">
                    {(rec.size_bytes / 1024 / 1024).toFixed(1)} MB
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      <Modal
        open={!!deleteTarget}
        onClose={() => {
          if (!deleteBusy) setDeleteTarget(null);
        }}
        title="Delete channel"
        footer={
          <>
            <Button
              variant="ghost"
              disabled={deleteBusy}
              onClick={() => setDeleteTarget(null)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={deleteBusy}
              disabled={deleteBusy}
              onClick={() => void confirmDeleteChannel()}
            >
              Delete channel
            </Button>
          </>
        }
      >
        {deleteTarget ? (
          <div className="space-y-3 house-type-body text-foreground">
            <p>
              Delete channel <strong>{deleteTarget.name}</strong> (
              <span className="house-type-meta uppercase">{deleteTarget.type}</span>
              ) from session <strong>{session.name}</strong>?
            </p>
            <p className="text-muted-foreground">
              Its mix entries for this channel will be lost. Sources assigned only
              through this channel’s mix will no longer route here. This cannot be
              undone from the UI.
            </p>
            {deleteError ? (
              <p role="alert" className="text-[var(--house-status-danger)]">
                {deleteError}
              </p>
            ) : null}
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
