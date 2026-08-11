import { useEffect, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Button, DataState, Field, PageHeader } from "@crosstalk/theme";
import type { components } from "@crosstalk/api-client";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";

type Session = components["schemas"]["SessionOut"];
type SortField = "created_at" | "updated_at" | "name" | "id";
type Direction = "asc" | "desc";

const DEFAULT_SORT: SortField = "created_at";
const DEFAULT_DIRECTION: Direction = "desc";
const PAGE_LIMIT = 25;

function parseSort(value: string | null): SortField {
  if (
    value === "updated_at" ||
    value === "name" ||
    value === "id" ||
    value === "created_at"
  ) {
    return value;
  }
  return DEFAULT_SORT;
}

function parseDirection(value: string | null): Direction {
  return value === "asc" ? "asc" : DEFAULT_DIRECTION;
}

export function SessionListPage() {
  const { token } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();

  const q = searchParams.get("q") ?? "";
  const sort = parseSort(searchParams.get("sort"));
  const direction = parseDirection(searchParams.get("direction"));
  const cursor = searchParams.get("cursor") ?? "";

  const [sessions, setSessions] = useState<Session[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createSuccess, setCreateSuccess] = useState<string | null>(null);

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
        const { data, error: apiError } = await client.GET("/api/sessions", {
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
        });
        if (controller.signal.aborted) return;
        if (apiError) {
          setError(apiError.detail || "Failed to load sessions");
          setSessions([]);
          setNextCursor(undefined);
          setTotal(null);
          return;
        }
        setSessions(data?.data ?? []);
        setNextCursor(data?.next_cursor || undefined);
        setTotal(typeof data?.total === "number" ? data.total : null);
      } catch (err) {
        if (controller.signal.aborted) return;
        setError(err instanceof Error ? err.message : "Failed to load sessions");
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

  const handleSearchSubmit = (e: FormEvent) => {
    e.preventDefault();
    updateParams({ q: draftQ.trim(), cursor: null });
  };

  const handleCreate = async (e?: FormEvent) => {
    e?.preventDefault();
    if (!token || !newName.trim()) return;
    setCreating(true);
    setCreateError(null);
    setCreateSuccess(null);
    const client = getApiClient(token);
    try {
      const { error: apiError } = await client.POST("/api/sessions", {
        body: { name: newName.trim() },
      });
      if (apiError) {
        setCreateError(apiError.detail || "Failed to create session");
        return;
      }
      const createdName = newName.trim();
      setNewName("");
      setShowCreate(false);
      setCreateSuccess(`Created session “${createdName}”.`);
      if (cursor) {
        updateParams({ cursor: null });
      } else {
        setReloadToken((n) => n + 1);
      }
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "Failed to create session");
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Operate"
        title="Sessions"
        lede="Server-backed session collection. Search and sort are applied by the API."
        actions={
          <Button
            variant="primary"
            onClick={() => {
              setShowCreate((v) => !v);
              setCreateError(null);
            }}
          >
            + New Session
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
        onSubmit={handleSearchSubmit}
        className="flex flex-col gap-3 border-b border-border pb-4 md:flex-row md:items-end"
      >
        <div className="min-w-0 flex-1">
          <Field
            label="Search"
            name="q"
            value={draftQ}
            onChange={(e) => setDraftQ(e.target.value)}
            placeholder="Filter by name or description"
          />
        </div>
        <div className="w-full md:w-44">
          <Field
            as="select"
            label="Sort"
            value={sort}
            onChange={(e) => updateParams({ sort: e.target.value, cursor: null })}
          >
            <option value="created_at">Created</option>
            <option value="updated_at">Updated</option>
            <option value="name">Name</option>
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
        <form
          onSubmit={(e) => void handleCreate(e)}
          className="space-y-3 border border-border bg-[var(--house-bg-surface)] p-4"
        >
          <h2 className="house-type-section">Create session</h2>
          <Field
            label="Session name"
            name="session-name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="Session name..."
            required
            autoFocus
            error={createError ?? undefined}
          />
          <div className="flex flex-wrap gap-2">
            <Button
              type="submit"
              variant="primary"
              loading={creating}
              disabled={creating || !newName.trim()}
            >
              {creating ? "Creating..." : "Create"}
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setShowCreate(false);
                setCreateError(null);
              }}
            >
              Cancel
            </Button>
          </div>
        </form>
      ) : null}

      {loading ? (
        <DataState
          kind="loading"
          title="Loading sessions"
          description="Fetching the current server page."
        />
      ) : error ? (
        <DataState
          kind="error"
          title="Could not load sessions"
          description={error}
          action={
            <Button variant="secondary" onClick={() => setReloadToken((n) => n + 1)}>
              Retry
            </Button>
          }
        />
      ) : sessions.length === 0 ? (
        <DataState
          kind="empty"
          title={q ? "No sessions match this search" : "No sessions yet"}
          description={
            q
              ? "Try a different query or clear the search."
              : "Create a session to start routing audio."
          }
          action={
            !q ? (
              <Button variant="primary" onClick={() => setShowCreate(true)}>
                + New Session
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
                    Name
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Description
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Created
                  </th>
                </tr>
              </thead>
              <tbody>
                {sessions.map((session) => (
                  <tr
                    key={session.id}
                    className="border-b border-border/60 last:border-b-0 hover:bg-[var(--house-bg-raised)]"
                  >
                    <td className="px-4 py-3">
                      <Link
                        to={`/sessions/${session.id}`}
                        className="font-medium text-primary hover:underline"
                      >
                        {session.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {session.description || (
                        <span className="house-type-meta italic">No description</span>
                      )}
                    </td>
                    <td className="px-4 py-3 house-type-meta text-muted-foreground">
                      {new Date(session.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <ul className="divide-y divide-border border-y border-border md:hidden">
            {sessions.map((session) => (
              <li key={session.id} className="py-3">
                <Link
                  to={`/sessions/${session.id}`}
                  className="font-medium text-primary hover:underline"
                >
                  {session.name}
                </Link>
                <dl className="mt-2 space-y-1">
                  <div className="grid grid-cols-[6rem_1fr] gap-2">
                    <dt className="house-type-meta text-muted-foreground">Description</dt>
                    <dd className="house-type-body-compact text-muted-foreground">
                      {session.description || "—"}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[6rem_1fr] gap-2">
                    <dt className="house-type-meta text-muted-foreground">Created</dt>
                    <dd className="house-type-meta text-muted-foreground">
                      {new Date(session.created_at).toLocaleDateString()}
                    </dd>
                  </div>
                </dl>
              </li>
            ))}
          </ul>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="house-type-meta text-muted-foreground">
              Showing {sessions.length}
              {total != null ? ` of ${total}` : ""} sessions
              {cursor ? " · continued page" : ""}
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
    </div>
  );
}
