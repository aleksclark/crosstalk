import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import type { components } from "@crosstalk/api-client";

type ABC = components["schemas"]["ABC"];

export function ABCDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { token } = useAuth();
  const [abc, setAbc] = useState<ABC | null>(null);
  const [loading, setLoading] = useState(true);
  const [restarting, setRestarting] = useState(false);

  useEffect(() => {
    async function fetchABC() {
      if (!token || !id) return;
      const client = getApiClient(token);
      try {
        const { data } = await client.GET("/api/abcs/{id}", {
          params: { path: { id } },
        });
        if (data) {
          setAbc(data);
        }
      } catch {
        // handle error
      } finally {
        setLoading(false);
      }
    }
    fetchABC();
  }, [token, id]);

  const handleRestart = async () => {
    if (!token || !id) return;
    setRestarting(true);
    const client = getApiClient(token);
    try {
      await client.POST("/api/abcs/{id}/restart", {
        params: { path: { id } },
      });
    } catch {
      // handle error
    } finally {
      setRestarting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading ABC...</p>
      </div>
    );
  }

  if (!abc) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">ABC not found</p>
        <Link to="/abcs" className="text-primary text-sm mt-2 inline-block">
          ← Back to ABCs
        </Link>
      </div>
    );
  }

  // Demo connection history
  const connectionHistory = [
    { time: "2024-01-15 14:30:00", event: "Connected", duration: "2h 15m" },
    { time: "2024-01-15 12:00:00", event: "Disconnected", duration: "-" },
    { time: "2024-01-15 09:45:00", event: "Connected", duration: "2h 15m" },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <Link
              to="/abcs"
              className="text-muted-foreground hover:text-foreground text-sm"
            >
              ABCs /
            </Link>
            <h1 className="text-2xl font-bold">{abc.name}</h1>
          </div>
          <p className="text-muted-foreground text-sm mt-1">ID: {abc.id}</p>
        </div>
        <div className="flex items-center gap-3">
          <span
            className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-medium ${
              abc.status === "online"
                ? "bg-green-500/20 text-green-400"
                : abc.status === "error"
                  ? "bg-red-500/20 text-red-400"
                  : "bg-gray-500/20 text-gray-400"
            }`}
          >
            <span className="w-2 h-2 rounded-full bg-current" />
            {abc.status}
          </span>
          <button
            onClick={handleRestart}
            disabled={restarting}
            className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90 disabled:opacity-50"
          >
            {restarting ? "Restarting..." : "Restart"}
          </button>
        </div>
      </div>

      {/* Info cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Assigned Session
          </h3>
          {abc.session_id ? (
            <Link
              to={`/sessions/${abc.session_id}`}
              className="text-primary text-sm mt-2 inline-block hover:underline"
            >
              {abc.session_id.slice(0, 8)}...
            </Link>
          ) : (
            <p className="text-sm mt-2 text-muted-foreground">Unassigned</p>
          )}
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Last Seen
          </h3>
          <p className="text-sm mt-2">
            {abc.last_seen_at
              ? new Date(abc.last_seen_at).toLocaleString()
              : "Never"}
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Registered
          </h3>
          <p className="text-sm mt-2">
            {new Date(abc.created_at).toLocaleString()}
          </p>
        </div>
      </div>

      {/* Connection history */}
      <div className="bg-card border border-border rounded-lg p-4">
        <h2 className="text-lg font-semibold mb-3">Connection History</h2>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left py-2 text-muted-foreground font-medium">
                Time
              </th>
              <th className="text-left py-2 text-muted-foreground font-medium">
                Event
              </th>
              <th className="text-left py-2 text-muted-foreground font-medium">
                Duration
              </th>
            </tr>
          </thead>
          <tbody>
            {connectionHistory.map((entry, i) => (
              <tr key={i} className="border-b border-border/50">
                <td className="py-2 text-muted-foreground">{entry.time}</td>
                <td className="py-2">{entry.event}</td>
                <td className="py-2 text-muted-foreground">{entry.duration}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
