import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Button,
  CopyableId,
  DataState,
  PageHeader,
  Status,
} from "@crosstalk/theme";
import type { components } from "@crosstalk/api-client";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";

type ABC = components["schemas"]["ABCOut"];

/**
 * Minimal visual parity migration for ABC detail.
 * Intentionally avoids K2B audio-control work and large restyle overlap.
 */
export function ABCDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { token } = useAuth();
  const [abc, setAbc] = useState<ABC | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [restarting, setRestarting] = useState(false);
  const [restartMessage, setRestartMessage] = useState<string | null>(null);
  const [restartError, setRestartError] = useState<string | null>(null);

  useEffect(() => {
    if (!token || !id) return;
    const controller = new AbortController();
    setLoading(true);
    setLoadError(null);
    const client = getApiClient(token);
    void (async () => {
      try {
        const { data, error } = await client.GET("/api/abcs/{id}", {
          params: { path: { id } },
          signal: controller.signal,
        });
        if (controller.signal.aborted) return;
        if (error || !data) {
          setAbc(null);
          setLoadError(error?.detail || "ABC not found");
          return;
        }
        setAbc(data);
      } catch (err) {
        if (controller.signal.aborted) return;
        setLoadError(err instanceof Error ? err.message : "Failed to load ABC");
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [token, id]);

  const handleRestart = async () => {
    if (!token || !id || !abc) return;
    setRestarting(true);
    setRestartError(null);
    setRestartMessage(null);
    const client = getApiClient(token);
    try {
      const { error } = await client.POST("/api/abcs/{id}/restart", {
        params: { path: { id } },
      });
      if (error) {
        setRestartError(error.detail || "Restart request failed");
        return;
      }
      // Server may only queue a command; do not claim the device rebooted.
      setRestartMessage(
        `Restart command sent for “${abc.name}”. Connection state will update when the client reports in.`,
      );
    } catch (err) {
      setRestartError(err instanceof Error ? err.message : "Restart request failed");
    } finally {
      setRestarting(false);
    }
  };

  if (loading) {
    return (
      <DataState
        kind="loading"
        title="Loading ABC"
        description="Fetching connector details."
      />
    );
  }

  if (!abc) {
    return (
      <DataState
        kind="error"
        title="ABC unavailable"
        description={loadError ?? "ABC not found"}
        action={
          <Link to="/abcs" className="text-sm text-primary hover:underline">
            ← Back to ABCs
          </Link>
        }
      />
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="ABCs"
        title={abc.name}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Status tone={abc.connected ? "ok" : "neutral"}>
              {abc.connected ? "online" : "offline"}
            </Status>
            <Button
              variant="secondary"
              loading={restarting}
              disabled={restarting}
              onClick={() => void handleRestart()}
            >
              {restarting ? "Sending..." : "Restart"}
            </Button>
          </div>
        }
        meta={
          <>
            <span className="inline-flex items-center gap-2">
              ID <CopyableId value={abc.id} />
            </span>
            <Link to="/abcs" className="text-primary hover:underline">
              All ABCs
            </Link>
          </>
        }
      />

      {restartMessage ? (
        <p role="status" className="house-type-body text-[var(--house-status-ok)]">
          {restartMessage}
        </p>
      ) : null}
      {restartError ? (
        <p role="alert" className="house-type-body text-[var(--house-status-danger)]">
          {restartError}
        </p>
      ) : null}

      <dl className="divide-y divide-border border-y border-border">
        <div className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[10rem_1fr] sm:gap-6">
          <dt className="house-type-label text-muted-foreground">Assigned session</dt>
          <dd className="text-sm">
            {abc.session_id ? (
              <Link
                to={`/sessions/${abc.session_id}`}
                className="text-primary hover:underline"
              >
                {abc.session_name || abc.session_id}
              </Link>
            ) : (
              <span className="text-muted-foreground">Unassigned</span>
            )}
          </dd>
        </div>
        <div className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[10rem_1fr] sm:gap-6">
          <dt className="house-type-label text-muted-foreground">Last seen</dt>
          <dd className="house-type-meta text-muted-foreground">
            {abc.last_seen ? new Date(abc.last_seen).toLocaleString() : "Never"}
          </dd>
        </div>
        <div className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[10rem_1fr] sm:gap-6">
          <dt className="house-type-label text-muted-foreground">Registered</dt>
          <dd className="house-type-meta text-muted-foreground">
            {new Date(abc.created_at).toLocaleString()}
          </dd>
        </div>
      </dl>
    </div>
  );
}
