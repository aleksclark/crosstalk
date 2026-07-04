import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import type { components } from "@crosstalk/api-client";

type Translator = components["schemas"]["TranslatorOut"];
type Session = components["schemas"]["SessionOut"];

export function TranslatorListPage() {
  const { token } = useAuth();
  const [translators, setTranslators] = useState<Translator[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [assigningId, setAssigningId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!token) return;
    const client = getApiClient(token);
    const [tRes, sRes] = await Promise.all([
      client.GET("/api/translators"),
      client.GET("/api/sessions"),
    ]);
    setTranslators(tRes.data?.data ?? []);
    setSessions(sRes.data?.data ?? []);
    setLoading(false);
  }, [token]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleCreate = async () => {
    if (!token || !newUsername.trim() || !newPassword.trim()) return;
    setCreating(true);
    setError(null);
    const client = getApiClient(token);
    const { error: apiError } = await client.POST("/api/translators", {
      body: { username: newUsername.trim(), password: newPassword.trim() },
    });
    setCreating(false);
    if (apiError) {
      setError(apiError.detail || "Failed to create translator");
      return;
    }
    setNewUsername("");
    setNewPassword("");
    setShowCreate(false);
    await refresh();
  };

  const handleDelete = async (id: string) => {
    if (!token) return;
    const client = getApiClient(token);
    await client.DELETE("/api/translators/{id}", {
      params: { path: { id } },
    });
    await refresh();
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
    await refresh();
  };

  const sessionName = (id: string) =>
    sessions.find((s) => s.id === id)?.name ?? id.slice(0, 8);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading translators...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Translators</h1>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90"
        >
          + New Translator
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-semibold mb-3">
            Create Translator Account
          </h3>
          <div className="flex items-end gap-3">
            <div className="flex-1">
              <label className="block text-xs text-muted-foreground mb-1">
                Username
              </label>
              <input
                type="text"
                value={newUsername}
                onChange={(e) => setNewUsername(e.target.value)}
                placeholder="translator_xx"
                className="w-full bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                autoFocus
              />
            </div>
            <div className="flex-1">
              <label className="block text-xs text-muted-foreground mb-1">
                Password
              </label>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="••••••••"
                className="w-full bg-muted border border-border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              />
            </div>
            <button
              onClick={handleCreate}
              disabled={creating || !newUsername.trim() || !newPassword.trim()}
              className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90 disabled:opacity-50"
            >
              {creating ? "Creating..." : "Create"}
            </button>
            <button
              onClick={() => {
                setShowCreate(false);
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

      {/* Translators table */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Username
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Assigned Sessions
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Created
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {translators.map((t) => (
              <tr
                key={t.id}
                className="border-b border-border/50 hover:bg-accent/50"
              >
                <td className="px-4 py-3 font-medium">{t.username}</td>
                <td className="px-4 py-3 text-muted-foreground">
                  {t.sessions && t.sessions.length > 0 ? (
                    t.sessions.map(sessionName).join(", ")
                  ) : (
                    <span className="text-xs italic">Unassigned</span>
                  )}
                </td>
                <td className="px-4 py-3 text-muted-foreground text-xs">
                  {t.created_at
                    ? new Date(t.created_at).toLocaleDateString()
                    : "—"}
                </td>
                <td className="px-4 py-3">
                  {assigningId === t.id ? (
                    <select
                      autoFocus
                      defaultValue={t.sessions?.[0] ?? ""}
                      onChange={(e) => handleAssign(t.id, e.target.value)}
                      onBlur={() => setAssigningId(null)}
                      className="bg-muted border border-border rounded-md px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
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
                        onClick={() => setAssigningId(t.id)}
                        className="text-xs text-primary hover:underline"
                      >
                        Assign
                      </button>
                      <button
                        onClick={() => handleDelete(t.id)}
                        className="text-xs text-red-400 hover:underline"
                      >
                        Delete
                      </button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
            {translators.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-8 text-center text-muted-foreground"
                >
                  No translator accounts.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
