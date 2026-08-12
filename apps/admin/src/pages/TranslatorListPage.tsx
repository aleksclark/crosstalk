import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Button,
  DataState,
  Field,
  Modal,
  PageHeader,
} from "@crosstalk/theme";
import type { components } from "@crosstalk/api-client";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";

type Translator = components["schemas"]["TranslatorOut"];
type Session = components["schemas"]["SessionOut"];
type SortField = "created_at" | "username" | "id";
type Direction = "asc" | "desc";

const PAGE_LIMIT = 25;

function parseSort(value: string | null): SortField {
  if (value === "username" || value === "id" || value === "created_at") return value;
  return "created_at";
}

function parseDirection(value: string | null): Direction {
  return value === "asc" ? "asc" : "desc";
}

function sessionLabels(t: Translator, sessions: Session[]): string {
  if (t.session_names && Object.keys(t.session_names).length > 0) {
    return Object.values(t.session_names).join(", ");
  }
  if (t.sessions && t.sessions.length > 0) {
    return t.sessions
      .map((id) => sessions.find((s) => s.id === id)?.name ?? id.slice(0, 8))
      .join(", ");
  }
  return "";
}

export function TranslatorListPage() {
  const { token } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const q = searchParams.get("q") ?? "";
  const sort = parseSort(searchParams.get("sort"));
  const direction = parseDirection(searchParams.get("direction"));
  const cursor = searchParams.get("cursor") ?? "";

  const [translators, setTranslators] = useState<Translator[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createSuccess, setCreateSuccess] = useState<string | null>(null);
  const [assigningId, setAssigningId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Translator | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [draftQ, setDraftQ] = useState(q);

  useEffect(() => {
    setDraftQ(q);
  }, [q]);

  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    const client = getApiClient(token);
    void (async () => {
      try {
        const [tRes, sRes] = await Promise.all([
          client.GET("/api/translators", {
            params: {
              query: {
                q: q || undefined,
                sort,
                direction,
                limit: PAGE_LIMIT,
                cursor: cursor || undefined,
              },
            },
            signal: controller.signal,
          }),
          client.GET("/api/sessions", {
            params: { query: { limit: 100, sort: "name", direction: "asc" } },
            signal: controller.signal,
          }),
        ]);
        if (controller.signal.aborted) return;
        if (tRes.error) {
          setError(tRes.error.detail || "Failed to load translators");
          setTranslators([]);
          return;
        }
        setTranslators(tRes.data?.data ?? []);
        setNextCursor(tRes.data?.next_cursor || undefined);
        setTotal(typeof tRes.data?.total === "number" ? tRes.data.total : null);
        setSessions(sRes.data?.data ?? []);
      } catch (err) {
        if (controller.signal.aborted) return;
        setError(err instanceof Error ? err.message : "Failed to load translators");
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [token, q, sort, direction, cursor, reloadToken]);

  const updateParams = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(patch)) {
      if (value == null || value === "") next.delete(key);
      else next.set(key, value);
    }
    setSearchParams(next, { replace: true });
  };

  const handleCreate = async () => {
    if (!token || !newUsername.trim() || !newPassword.trim()) return;
    setCreating(true);
    setCreateError(null);
    setCreateSuccess(null);
    const client = getApiClient(token);
    const { error: apiError } = await client.POST("/api/translators", {
      body: { username: newUsername.trim(), password: newPassword.trim() },
    });
    setCreating(false);
    if (apiError) {
      setCreateError(apiError.detail || "Failed to create translator");
      return;
    }
    const created = newUsername.trim();
    setNewUsername("");
    setNewPassword("");
    setShowCreate(false);
    setCreateSuccess(`Created translator “${created}”.`);
    if (cursor) updateParams({ cursor: null });
    else setReloadToken((n) => n + 1);
  };

  const confirmDelete = async () => {
    if (!token || !deleteTarget) return;
    setDeleteBusy(true);
    setDeleteError(null);
    const client = getApiClient(token);
    const { error } = await client.DELETE("/api/translators/{id}", {
      params: { path: { id: deleteTarget.id } },
    });
    setDeleteBusy(false);
    if (error) {
      setDeleteError(error.detail || "Failed to delete translator");
      return;
    }
    setDeleteTarget(null);
    setReloadToken((n) => n + 1);
  };

  const handleAssign = async (id: string, sessionId: string) => {
    if (!token) return;
    const client = getApiClient(token);
    const sessionIds = sessionId ? [sessionId] : [];
    await client.PUT("/api/translators/{id}/sessions", {
      params: { path: { id } },
      body: { session_ids: sessionIds },
    });
    setAssigningId(null);
    setReloadToken((n) => n + 1);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Operate"
        title="Translators"
        lede="Create accounts, assign sessions, and remove access. Usernames lead row identity."
        actions={
          <Button
            variant="primary"
            onClick={() => {
              setShowCreate((v) => !v);
              setCreateError(null);
            }}
          >
            + New Translator
          </Button>
        }
        meta={
          total != null ? (
            <span>
              {total} total · page size {PAGE_LIMIT}
            </span>
          ) : null
        }
      />

      {createSuccess ? (
        <p role="status" className="house-type-body text-[var(--house-status-ok)]">
          {createSuccess}
        </p>
      ) : null}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          updateParams({ q: draftQ.trim(), cursor: null });
        }}
        className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end"
      >
        <div className="min-w-0 flex-1">
          <Field
            label="Search"
            value={draftQ}
            onChange={(e) => setDraftQ(e.target.value)}
            placeholder="Filter by username"
          />
        </div>
        <div className="w-full md:w-40">
          <Field
            as="select"
            label="Sort"
            value={sort}
            onChange={(e) => updateParams({ sort: e.target.value, cursor: null })}
          >
            <option value="created_at">Created</option>
            <option value="username">Username</option>
            <option value="id">ID</option>
          </Field>
        </div>
        <div className="w-full md:w-36">
          <Field
            as="select"
            label="Direction"
            value={direction}
            onChange={(e) =>
              updateParams({ direction: e.target.value, cursor: null })
            }
          >
            <option value="desc">Descending</option>
            <option value="asc">Ascending</option>
          </Field>
        </div>
        <Button type="submit" variant="secondary">
          Apply
        </Button>
      </form>

      {showCreate ? (
        <div className="space-y-3 border border-border bg-[var(--house-bg-surface)] p-4">
          <h2 className="house-type-section">Create Translator Account</h2>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <Field
              label="Username"
              value={newUsername}
              onChange={(e) => setNewUsername(e.target.value)}
              placeholder="translator_xx"
              autoFocus
              autoComplete="off"
            />
            <Field
              label="Password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
            />
          </div>
          {createError ? (
            <p role="alert" className="house-type-meta text-[var(--house-status-danger)]">
              {createError}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button
              variant="primary"
              loading={creating}
              disabled={creating || !newUsername.trim() || !newPassword.trim()}
              onClick={() => void handleCreate()}
            >
              {creating ? "Creating..." : "Create"}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setShowCreate(false);
                setCreateError(null);
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      ) : null}

      {loading ? (
        <DataState
          kind="loading"
          title="Loading translators"
          description="Fetching the current server page."
        />
      ) : error ? (
        <DataState
          kind="error"
          title="Could not load translators"
          description={error}
          action={
            <Button variant="secondary" onClick={() => setReloadToken((n) => n + 1)}>
              Retry
            </Button>
          }
        />
      ) : translators.length === 0 ? (
        <DataState
          kind="empty"
          title={q ? "No translators match this search" : "No translator accounts"}
          description={
            q
              ? "Try a different query or clear the search."
              : "Create a translator account to assign session access."
          }
          action={
            !q ? (
              <Button variant="primary" onClick={() => setShowCreate(true)}>
                + New Translator
              </Button>
            ) : (
              <Button
                variant="secondary"
                onClick={() => updateParams({ q: null, cursor: null })}
              >
                Clear search
              </Button>
            )
          }
        />
      ) : (
        <>
          <div className="hidden overflow-hidden border border-border md:block">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Username
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Assigned Sessions
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Created
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {translators.map((t) => {
                  const labels = sessionLabels(t, sessions);
                  return (
                    <tr
                      key={t.id}
                      className="border-b border-border/60 last:border-b-0 hover:bg-[var(--house-bg-raised)]"
                    >
                      <td className="px-4 py-3 font-medium">{t.username}</td>
                      <td className="px-4 py-3 text-muted-foreground">
                        {labels || (
                          <span className="house-type-meta italic">Unassigned</span>
                        )}
                      </td>
                      <td className="px-4 py-3 house-type-meta text-muted-foreground">
                        {t.created_at
                          ? new Date(t.created_at).toLocaleDateString()
                          : "—"}
                      </td>
                      <td className="px-4 py-3">
                        {assigningId === t.id ? (
                          <select
                            autoFocus
                            defaultValue={t.sessions?.[0] ?? ""}
                            onChange={(e) => void handleAssign(t.id, e.target.value)}
                            onBlur={() => setAssigningId(null)}
                            className="rounded-[var(--house-radius-md)] border border-border bg-[var(--house-bg-sunken)] px-2 py-1 text-xs outline-none focus-visible:ring-2 focus-visible:ring-[var(--house-focus)]"
                          >
                            <option value="">Unassigned</option>
                            {sessions.map((s) => (
                              <option key={s.id} value={s.id}>
                                {s.name}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <div className="flex items-center gap-3">
                            <button
                              type="button"
                              onClick={() => setAssigningId(t.id)}
                              className="text-xs text-primary hover:underline"
                            >
                              Assign
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                setDeleteError(null);
                                setDeleteTarget(t);
                              }}
                              className="text-xs text-[var(--house-status-danger)] hover:underline"
                            >
                              Delete
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          <ul className="divide-y divide-border border-y border-border md:hidden">
            {translators.map((t) => {
              const labels = sessionLabels(t, sessions);
              return (
                <li key={t.id} className="space-y-2 py-3">
                  <p className="font-medium">{t.username}</p>
                  <dl className="space-y-1">
                    <div className="grid grid-cols-[5.5rem_1fr] gap-2">
                      <dt className="house-type-meta text-muted-foreground">Sessions</dt>
                      <dd className="house-type-body-compact text-muted-foreground">
                        {labels || "Unassigned"}
                      </dd>
                    </div>
                    <div className="grid grid-cols-[5.5rem_1fr] gap-2">
                      <dt className="house-type-meta text-muted-foreground">Created</dt>
                      <dd className="house-type-meta text-muted-foreground">
                        {t.created_at
                          ? new Date(t.created_at).toLocaleDateString()
                          : "—"}
                      </dd>
                    </div>
                  </dl>
                  <div className="flex gap-3">
                    <button
                      type="button"
                      onClick={() => setAssigningId(t.id)}
                      className="text-xs text-primary hover:underline"
                    >
                      Assign
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setDeleteError(null);
                        setDeleteTarget(t);
                      }}
                      className="text-xs text-[var(--house-status-danger)] hover:underline"
                    >
                      Delete
                    </button>
                  </div>
                  {assigningId === t.id ? (
                    <select
                      autoFocus
                      defaultValue={t.sessions?.[0] ?? ""}
                      onChange={(e) => void handleAssign(t.id, e.target.value)}
                      onBlur={() => setAssigningId(null)}
                      className="w-full rounded-[var(--house-radius-md)] border border-border bg-[var(--house-bg-sunken)] px-2 py-2 text-sm"
                    >
                      <option value="">Unassigned</option>
                      {sessions.map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                    </select>
                  ) : null}
                </li>
              );
            })}
          </ul>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="house-type-meta text-muted-foreground">
              Showing {translators.length}
              {total != null ? ` of ${total}` : ""} translators
            </p>
            <div className="flex gap-2">
              {cursor ? (
                <Button variant="secondary" onClick={() => updateParams({ cursor: null })}>
                  First page
                </Button>
              ) : null}
              {nextCursor ? (
                <Button
                  variant="secondary"
                  onClick={() => updateParams({ cursor: nextCursor })}
                >
                  Next page
                </Button>
              ) : null}
            </div>
          </div>
        </>
      )}

      <Modal
        open={!!deleteTarget}
        onClose={() => {
          if (!deleteBusy) setDeleteTarget(null);
        }}
        title="Delete translator"
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
              onClick={() => void confirmDelete()}
            >
              Delete account
            </Button>
          </>
        }
      >
        {deleteTarget ? (
          <div className="space-y-3 house-type-body">
            <p>
              Delete translator account <strong>{deleteTarget.username}</strong>?
            </p>
            <p className="text-muted-foreground">
              Session assignments for this account will be removed. The user will
              no longer be able to sign in to the translator app.
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
