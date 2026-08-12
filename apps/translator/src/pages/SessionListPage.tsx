import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { createApiClient, type components } from "@crosstalk/api-client";
import { Button, DataState, Icon, PageHeader } from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";
import { OperateShell } from "../components/OperateShell";

type Session = components["schemas"]["SessionOut"];

type ListState =
  | { kind: "loading" }
  | { kind: "ready"; sessions: Session[]; total?: number }
  | { kind: "empty" }
  | { kind: "error"; message: string }
  | { kind: "denied"; message: string };

const PAGE_LIMIT = 25;

export function SessionListPage() {
  const { getToken, logout, user } = useAuth();
  const navigate = useNavigate();
  const [state, setState] = useState<ListState>({ kind: "loading" });
  const [reloadKey, setReloadKey] = useState(0);

  const load = useCallback(async (signal?: AbortSignal) => {
    const token = getToken();
    if (!token) return;
    setState({ kind: "loading" });
    try {
      const client = createApiClient({ baseUrl: window.location.origin, token });
      const { data, error, response } = await client.GET("/api/sessions", {
        params: {
          query: {
            sort: "name",
            direction: "asc",
            limit: PAGE_LIMIT,
          },
        },
        signal,
      });

      if (signal?.aborted) return;

      if (error || !response.ok) {
        const status = error?.status ?? response.status;
        const detail = error?.detail ?? error?.title ?? "Failed to load sessions";
        if (status === 401 || status === 403) {
          setState({ kind: "denied", message: detail });
          return;
        }
        setState({ kind: "error", message: detail });
        return;
      }

      const sessions = data?.data ?? [];
      if (sessions.length === 0) {
        setState({ kind: "empty" });
        return;
      }
      setState({
        kind: "ready",
        sessions,
        total: data?.total,
      });
    } catch (err) {
      if (signal?.aborted) return;
      const message = err instanceof Error ? err.message : "Network error loading sessions";
      setState({ kind: "error", message });
    }
  }, [getToken]);

  useEffect(() => {
    const ac = new AbortController();
    void load(ac.signal);
    return () => ac.abort();
  }, [load, reloadKey]);

  const handleLogout = () => {
    logout();
    navigate("/login", { replace: true });
  };

  const retry = () => setReloadKey((k) => k + 1);

  const scopeLabel =
    state.kind === "ready"
      ? state.total != null
        ? `Assigned sessions · ${state.total}`
        : `Assigned sessions · ${state.sessions.length}`
      : "Assigned sessions";

  return (
    <OperateShell username={user?.username} scope={scopeLabel} onLogout={handleLogout}>
      <PageHeader
        eyebrow="Operate"
        title="Sessions"
        lede="Open a session to connect your microphone and monitor channels."
      />

      {state.kind === "loading" ? (
        <DataState kind="loading" title="Loading assigned sessions" description="Fetching sessions in your scope." />
      ) : null}

      {state.kind === "empty" ? (
        <DataState
          kind="empty"
          title="No sessions assigned"
          description="An administrator must assign you to a session before you can connect."
          action={
            <Button variant="secondary" icon="refresh" onClick={retry}>
              Retry
            </Button>
          }
        />
      ) : null}

      {state.kind === "error" ? (
        <DataState
          kind="error"
          title="Could not load sessions"
          description={state.message}
          action={
            <Button variant="primary" icon="refresh" onClick={retry}>
              Retry
            </Button>
          }
        />
      ) : null}

      {state.kind === "denied" ? (
        <DataState
          kind="denied"
          title="Access denied"
          description={state.message}
          action={
            <Button variant="secondary" onClick={handleLogout}>
              Sign out
            </Button>
          }
        />
      ) : null}

      {state.kind === "ready" ? (
        <ul
          role="list"
          data-testid="session-list"
          style={{
            listStyle: "none",
            margin: 0,
            padding: 0,
            borderTop: "1px solid var(--house-rule-subtle)",
          }}
        >
          {state.sessions.map((session) => (
            <li key={session.id} style={{ borderBottom: "1px solid var(--house-rule-subtle)" }}>
              <button
                type="button"
                onClick={() => navigate(`/sessions/${session.id}/connect`)}
                className="w-full text-left"
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: "var(--house-space-3)",
                  minHeight: "var(--house-row-height, 48px)",
                  padding: "var(--house-space-3) 0",
                  background: "transparent",
                  border: "none",
                  color: "inherit",
                  cursor: "pointer",
                  font: "inherit",
                }}
                data-testid={`session-row-${session.id}`}
              >
                <div className="min-w-0" style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  <span className="house-type-section" style={{ fontWeight: 600 }}>
                    {session.name}
                  </span>
                  {session.description ? (
                    <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
                      {session.description}
                    </span>
                  ) : (
                    <span className="house-type-meta" style={{ color: "var(--house-text-tertiary)" }}>
                      ID {session.id.slice(0, 8)}…
                    </span>
                  )}
                </div>
                <Icon name="chevron-right" size="default" aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </OperateShell>
  );
}
