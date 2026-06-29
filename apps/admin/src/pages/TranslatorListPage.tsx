import { useState } from "react";

interface Translator {
  id: string;
  username: string;
  sessionId: string | null;
  sessionName: string | null;
  createdAt: string;
}

export function TranslatorListPage() {
  const [translators, setTranslators] = useState<Translator[]>([
    {
      id: "1",
      username: "translator_es",
      sessionId: null,
      sessionName: null,
      createdAt: "2024-01-10T10:00:00Z",
    },
    {
      id: "2",
      username: "translator_fr",
      sessionId: "abc-123",
      sessionName: "Conference 1",
      createdAt: "2024-01-11T10:00:00Z",
    },
  ]);
  const [showCreate, setShowCreate] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");

  const handleCreate = () => {
    if (!newUsername.trim() || !newPassword.trim()) return;
    const newTranslator: Translator = {
      id: String(Date.now()),
      username: newUsername,
      sessionId: null,
      sessionName: null,
      createdAt: new Date().toISOString(),
    };
    setTranslators([...translators, newTranslator]);
    setNewUsername("");
    setNewPassword("");
    setShowCreate(false);
  };

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
          <h3 className="text-sm font-semibold mb-3">Create Translator Account</h3>
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
              disabled={!newUsername.trim() || !newPassword.trim()}
              className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90 disabled:opacity-50"
            >
              Create
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

      {/* Translators table */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Username
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Assigned Session
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
                  {t.sessionName ?? (
                    <span className="text-xs italic">Unassigned</span>
                  )}
                </td>
                <td className="px-4 py-3 text-muted-foreground text-xs">
                  {new Date(t.createdAt).toLocaleDateString()}
                </td>
                <td className="px-4 py-3">
                  <button className="text-xs text-primary hover:underline">
                    Assign
                  </button>
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
