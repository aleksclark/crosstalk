import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { createApiClient, type components } from "@crosstalk/api-client";

type Session = components["schemas"]["SessionOut"];

export function SessionListPage() {
  const { getToken, logout, user } = useAuth();
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchSessions = async () => {
      const token = getToken();
      if (!token) return;
      const client = createApiClient({ baseUrl: window.location.origin, token });
      const { data, error: apiError } = await client.GET("/api/sessions");
      if (apiError) {
        setError("Failed to load sessions");
      } else if (data) {
        setSessions(data.data ?? []);
      }
      setLoading(false);
    };
    fetchSessions();
  }, [getToken]);

  return (
    <div className="min-h-screen p-4 max-w-2xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-bold text-white">Sessions</h1>
          {user && <p className="text-sm text-gray-400">Logged in as {user.username}</p>}
        </div>
        <button
          onClick={logout}
          className="px-3 py-1 text-sm bg-gray-800 hover:bg-gray-700 text-gray-300 rounded border border-gray-700 transition-colors"
        >
          Logout
        </button>
      </div>

      {loading && <p className="text-gray-400">Loading sessions...</p>}
      {error && <p className="text-red-400">{error}</p>}

      {!loading && sessions.length === 0 && (
        <p className="text-gray-500">No sessions assigned.</p>
      )}

      <div className="space-y-3">
        {sessions.map((session) => (
          <button
            key={session.id}
            onClick={() => navigate(`/sessions/${session.id}/connect`)}
            className="w-full text-left p-4 bg-gray-800 border border-gray-700 rounded-lg hover:border-blue-500 transition-colors"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="w-2 h-2 rounded-full bg-blue-500" />
                <span className="font-medium text-white">{session.name}</span>
              </div>
              {session.description && (
                <div className="text-sm text-gray-400">
                  {session.description}
                </div>
              )}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
