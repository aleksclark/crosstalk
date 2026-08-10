import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import type { components } from "@crosstalk/api-client";

type ABC = components["schemas"]["ABCOut"];
type CreatedABC = components["schemas"]["CreateABCResponseBody"];

export function ABCListPage() {
  const { token } = useAuth();
  const [abcs, setAbcs] = useState<ABC[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdABC, setCreatedABC] = useState<CreatedABC | null>(null);
  const [copied, setCopied] = useState(false);

  const refresh = useCallback(async () => {
    if (!token) return;
    const client = getApiClient(token);
    const { data } = await client.GET("/api/abcs");
    setAbcs(data?.data ?? []);
    setLoading(false);
  }, [token]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const handleCreate = async () => {
    const name = newName.trim();
    if (!token || !name) return;
    setCreating(true);
    setError(null);
    const client = getApiClient(token);
    const { data, error: apiError } = await client.POST("/api/abcs", {
      body: { name },
    });
    setCreating(false);
    if (apiError || !data) {
      setError(apiError?.detail || "Failed to create ABC");
      return;
    }
    setCreatedABC(data);
    setNewName("");
    setShowCreate(false);
    await refresh();
  };

  const copyToken = async () => {
    if (!createdABC) return;
    try {
      await navigator.clipboard.writeText(createdABC.token);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setError("Clipboard unavailable. Select and copy the token manually.");
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading ABCs...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Audio Bridge Clients</h1>
        <button
          onClick={() => {
            setShowCreate((visible) => !visible);
            setError(null);
          }}
          className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90"
        >
          + New ABC
        </button>
      </div>

      {showCreate && (
        <div className="bg-card border border-border rounded-lg p-4">
          <h2 className="text-sm font-semibold mb-3">Provision Audio Bridge Client</h2>
          <div className="flex items-end gap-3">
            <div className="flex-1">
              <label htmlFor="abc-name" className="block text-xs text-muted-foreground mb-1">
                Name
              </label>
              <input
                id="abc-name"
                type="text"
                value={newName}
                onChange={(event) => setNewName(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") void handleCreate();
                }}
                placeholder="Booth A"
                className="w-full bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                autoFocus
              />
            </div>
            <button
              onClick={() => void handleCreate()}
              disabled={creating || !newName.trim()}
              className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90 disabled:opacity-50"
            >
              {creating ? "Creating..." : "Create"}
            </button>
            <button
              onClick={() => {
                setShowCreate(false);
                setNewName("");
                setError(null);
              }}
              className="text-muted-foreground hover:text-foreground px-3 py-2 text-sm"
            >
              Cancel
            </button>
          </div>
          {error && <p className="text-xs text-red-400 mt-2">{error}</p>}
        </div>
      )}

      {createdABC && (
        <div className="bg-card border border-primary/50 rounded-lg p-5" role="status">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h2 className="font-semibold">ABC provisioned: {createdABC.name}</h2>
              <p className="text-sm text-yellow-300 mt-1">
                Save this token now. It cannot be retrieved after this message is dismissed.
              </p>
            </div>
            <button
              onClick={() => {
                setCreatedABC(null);
                setCopied(false);
              }}
              className="text-muted-foreground hover:text-foreground text-sm"
              aria-label="Dismiss token"
            >
              Dismiss
            </button>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <input
              readOnly
              value={createdABC.token}
              aria-label="ABC token"
              onFocus={(event) => event.currentTarget.select()}
              className="min-w-0 flex-1 bg-muted border border-border rounded-md px-3 py-2 font-mono text-sm"
            />
            <button
              onClick={() => void copyToken()}
              className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90"
            >
              {copied ? "Copied" : "Copy token"}
            </button>
          </div>
        </div>
      )}

      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">Name</th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">Status</th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">Session</th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">Last Seen</th>
            </tr>
          </thead>
          <tbody>
            {abcs.map((abc) => (
              <tr key={abc.id} className="border-b border-border/50 hover:bg-accent/50">
                <td className="px-4 py-3">
                  <Link to={`/abcs/${abc.id}`} className="text-primary hover:underline font-medium">
                    {abc.name}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
                      abc.connected ? "bg-green-500/20 text-green-400" : "bg-gray-500/20 text-gray-400"
                    }`}
                  >
                    <span className="w-1.5 h-1.5 rounded-full bg-current" />
                    {abc.connected ? "online" : "offline"}
                  </span>
                </td>
                <td className="px-4 py-3 text-muted-foreground">
                  {abc.session_id ? (
                    <Link to={`/sessions/${abc.session_id}`} className="text-primary text-xs hover:underline">
                      {abc.session_id.slice(0, 8)}...
                    </Link>
                  ) : (
                    <span className="text-xs">Unassigned</span>
                  )}
                </td>
                <td className="px-4 py-3 text-muted-foreground text-xs">
                  {abc.last_seen ? new Date(abc.last_seen).toLocaleString() : "Never"}
                </td>
              </tr>
            ))}
            {abcs.length === 0 && (
              <tr>
                <td colSpan={4} className="px-4 py-8 text-center text-muted-foreground">
                  No ABCs registered.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
