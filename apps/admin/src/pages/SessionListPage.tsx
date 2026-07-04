import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import type { components } from "@crosstalk/api-client";

type Session = components["schemas"]["SessionOut"];

export function SessionListPage() {
  const { token } = useAuth();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);

  const fetchSessions = async () => {
    if (!token) return;
    const client = getApiClient(token);
    try {
      const { data } = await client.GET("/api/sessions");
      setSessions(data?.data ?? []);
    } catch {
      // handle error
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSessions();
  }, [token]);

  const handleCreate = async () => {
    if (!token || !newName.trim()) return;
    setCreating(true);
    const client = getApiClient(token);
    try {
      await client.POST("/api/sessions", {
        body: { name: newName.trim() },
      });
      setNewName("");
      setShowCreate(false);
      await fetchSessions();
    } catch {
      // handle error
    } finally {
      setCreating(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading sessions...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Sessions</h1>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90"
        >
          + New Session
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="bg-card border border-border rounded-lg p-4">
          <div className="flex items-center gap-3">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="Session name..."
              className="flex-1 bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              autoFocus
            />
            <button
              onClick={handleCreate}
              disabled={creating || !newName.trim()}
              className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90 disabled:opacity-50"
            >
              {creating ? "Creating..." : "Create"}
            </button>
            <button
              onClick={() => setShowCreate(false)}
              className="text-muted-foreground hover:text-foreground px-3 py-2 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Sessions table */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Name
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Description
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Created
              </th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((session) => (
              <tr
                key={session.id}
                className="border-b border-border/50 hover:bg-accent/50"
              >
                <td className="px-4 py-3">
                  <Link
                    to={`/sessions/${session.id}`}
                    className="text-primary hover:underline font-medium"
                  >
                    {session.name}
                  </Link>
                </td>
                <td className="px-4 py-3 text-muted-foreground">
                  {session.description || (
                    <span className="text-xs italic">No description</span>
                  )}
                </td>
                <td className="px-4 py-3 text-muted-foreground">
                  {new Date(session.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
            {sessions.length === 0 && (
              <tr>
                <td
                  colSpan={3}
                  className="px-4 py-8 text-center text-muted-foreground"
                >
                  No sessions yet. Create one to get started.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
