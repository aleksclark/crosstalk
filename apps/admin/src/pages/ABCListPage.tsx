import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  Button,
  DataState,
  Field,
  Modal,
  PageHeader,
  Status,
} from "@crosstalk/theme";
import type { components } from "@crosstalk/api-client";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";

type ABC = components["schemas"]["ABCOut"];
type CreatedABC = components["schemas"]["CreateABCResponseBody"];
type SortField = "created_at" | "name" | "id";
type Direction = "asc" | "desc";

const PAGE_LIMIT = 25;

function parseSort(value: string | null): SortField {
  if (value === "name" || value === "id" || value === "created_at") return value;
  return "created_at";
}

function parseDirection(value: string | null): Direction {
  return value === "asc" ? "asc" : "desc";
}

export function ABCListPage() {
  const { token } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const q = searchParams.get("q") ?? "";
  const sort = parseSort(searchParams.get("sort"));
  const direction = parseDirection(searchParams.get("direction"));
  const cursor = searchParams.get("cursor") ?? "";

  const [abcs, setAbcs] = useState<ABC[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createdABC, setCreatedABC] = useState<CreatedABC | null>(null);
  const [copied, setCopied] = useState(false);
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
        const { data, error: apiError } = await client.GET("/api/abcs", {
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
          setError(apiError.detail || "Failed to load ABCs");
          setAbcs([]);
          setNextCursor(undefined);
          setTotal(null);
          return;
        }
        setAbcs(data?.data ?? []);
        setNextCursor(data?.next_cursor || undefined);
        setTotal(typeof data?.total === "number" ? data.total : null);
      } catch (err) {
        if (controller.signal.aborted) return;
        setError(err instanceof Error ? err.message : "Failed to load ABCs");
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
    const name = newName.trim();
    if (!token || !name) return;
    setCreating(true);
    setCreateError(null);
    const client = getApiClient(token);
    const { data, error: apiError } = await client.POST("/api/abcs", {
      body: { name },
    });
    setCreating(false);
    if (apiError || !data) {
      setCreateError(apiError?.detail || "Failed to create ABC");
      return;
    }
    setCreatedABC(data);
    setNewName("");
    setShowCreate(false);
    setCopied(false);
    if (cursor) updateParams({ cursor: null });
    else setReloadToken((n) => n + 1);
  };

  const copyToken = async () => {
    if (!createdABC) return;
    try {
      await navigator.clipboard.writeText(createdABC.token);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCreateError("Clipboard unavailable. Select and copy the token manually.");
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Operate"
        title="Audio Bridge Clients"
        lede="Provision ABCs and monitor connection state. Session names lead related identity."
        actions={
          <Button
            variant="primary"
            onClick={() => {
              setShowCreate((v) => !v);
              setCreateError(null);
            }}
          >
            + New ABC
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
            placeholder="Filter by ABC name"
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
        <div className="space-y-3 border border-border bg-[var(--house-bg-surface)] p-4">
          <h2 className="house-type-section">Provision Audio Bridge Client</h2>
          <Field
            id="abc-name"
            label="Name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void handleCreate();
            }}
            placeholder="Booth A"
            autoFocus
            error={createError ?? undefined}
          />
          <div className="flex flex-wrap gap-2">
            <Button
              variant="primary"
              loading={creating}
              disabled={creating || !newName.trim()}
              onClick={() => void handleCreate()}
            >
              {creating ? "Creating..." : "Create"}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setShowCreate(false);
                setNewName("");
                setCreateError(null);
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      ) : null}

      <Modal
        open={!!createdABC}
        onClose={() => {
          setCreatedABC(null);
          setCopied(false);
        }}
        title={createdABC ? `ABC provisioned: ${createdABC.name}` : "ABC token"}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => void copyToken()}
            >
              {copied ? "Copied" : "Copy token"}
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                setCreatedABC(null);
                setCopied(false);
              }}
            >
              Dismiss
            </Button>
          </>
        }
      >
        {createdABC ? (
          <div className="space-y-3" role="status">
            <p className="house-type-body text-[var(--house-status-warning)]">
              Save this token now. It cannot be retrieved after this message is
              dismissed or the page is reloaded.
            </p>
            <Field
              label="ABC token"
              readOnly
              value={createdABC.token}
              aria-label="ABC token"
              onFocus={(e) => e.currentTarget.select()}
              style={{ fontFamily: "var(--house-font-technical)" }}
            />
          </div>
        ) : null}
      </Modal>

      {loading ? (
        <DataState kind="loading" title="Loading ABCs" description="Fetching the current server page." />
      ) : error ? (
        <DataState
          kind="error"
          title="Could not load ABCs"
          description={error}
          action={
            <Button variant="secondary" onClick={() => setReloadToken((n) => n + 1)}>
              Retry
            </Button>
          }
        />
      ) : abcs.length === 0 ? (
        <DataState
          kind="empty"
          title={q ? "No ABCs match this search" : "No ABCs registered"}
          description={
            q
              ? "Try a different query or clear the search."
              : "Provision an ABC to connect booth hardware."
          }
          action={
            !q ? (
              <Button variant="primary" onClick={() => setShowCreate(true)}>
                + New ABC
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
                    Status
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Session
                  </th>
                  <th className="px-4 py-3 text-left house-type-label text-muted-foreground">
                    Last Seen
                  </th>
                </tr>
              </thead>
              <tbody>
                {abcs.map((abc) => (
                  <tr
                    key={abc.id}
                    className="border-b border-border/60 last:border-b-0 hover:bg-[var(--house-bg-raised)]"
                  >
                    <td className="px-4 py-3">
                      <Link
                        to={`/abcs/${abc.id}`}
                        className="font-medium text-primary hover:underline"
                      >
                        {abc.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3">
                      <Status tone={abc.connected ? "ok" : "neutral"}>
                        {abc.connected ? "online" : "offline"}
                      </Status>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {abc.session_id ? (
                        <Link
                          to={`/sessions/${abc.session_id}`}
                          className="text-primary hover:underline"
                        >
                          {abc.session_name || abc.session_id}
                        </Link>
                      ) : (
                        <span className="house-type-meta">Unassigned</span>
                      )}
                    </td>
                    <td className="px-4 py-3 house-type-meta text-muted-foreground">
                      {abc.last_seen
                        ? new Date(abc.last_seen).toLocaleString()
                        : "Never"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <ul className="divide-y divide-border border-y border-border md:hidden">
            {abcs.map((abc) => (
              <li key={abc.id} className="space-y-2 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Link
                    to={`/abcs/${abc.id}`}
                    className="font-medium text-primary hover:underline"
                  >
                    {abc.name}
                  </Link>
                  <Status tone={abc.connected ? "ok" : "neutral"}>
                    {abc.connected ? "online" : "offline"}
                  </Status>
                </div>
                <dl className="space-y-1">
                  <div className="grid grid-cols-[5rem_1fr] gap-2">
                    <dt className="house-type-meta text-muted-foreground">Session</dt>
                    <dd className="house-type-body-compact">
                      {abc.session_id ? (
                        <Link
                          to={`/sessions/${abc.session_id}`}
                          className="text-primary hover:underline"
                        >
                          {abc.session_name || abc.session_id}
                        </Link>
                      ) : (
                        "Unassigned"
                      )}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[5rem_1fr] gap-2">
                    <dt className="house-type-meta text-muted-foreground">Last seen</dt>
                    <dd className="house-type-meta text-muted-foreground">
                      {abc.last_seen
                        ? new Date(abc.last_seen).toLocaleString()
                        : "Never"}
                    </dd>
                  </div>
                </dl>
              </li>
            ))}
          </ul>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="house-type-meta text-muted-foreground">
              Showing {abcs.length}
              {total != null ? ` of ${total}` : ""} ABCs
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
