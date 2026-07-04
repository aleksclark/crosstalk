import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import type { components } from "@crosstalk/api-client";

type ABC = components["schemas"]["ABCOut"];

export function ABCListPage() {
  const { token } = useAuth();
  const [abcs, setAbcs] = useState<ABC[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchABCs() {
      if (!token) return;
      const client = getApiClient(token);
      try {
        const { data } = await client.GET("/api/abcs");
        setAbcs(data?.data ?? []);
      } catch {
        // handle error
      } finally {
        setLoading(false);
      }
    }
    fetchABCs();
  }, [token]);

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
      </div>

      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Name
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Status
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Session
              </th>
              <th className="text-left px-4 py-3 text-muted-foreground font-medium">
                Last Seen
              </th>
            </tr>
          </thead>
          <tbody>
            {abcs.map((abc) => (
              <tr
                key={abc.id}
                className="border-b border-border/50 hover:bg-accent/50"
              >
                <td className="px-4 py-3">
                  <Link
                    to={`/abcs/${abc.id}`}
                    className="text-primary hover:underline font-medium"
                  >
                    {abc.name}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
                      abc.connected
                        ? "bg-green-500/20 text-green-400"
                        : "bg-gray-500/20 text-gray-400"
                    }`}
                  >
                    <span className="w-1.5 h-1.5 rounded-full bg-current" />
                    {abc.connected ? "online" : "offline"}
                  </span>
                </td>
                <td className="px-4 py-3 text-muted-foreground">
                  {abc.session_id ? (
                    <Link
                      to={`/sessions/${abc.session_id}`}
                      className="text-primary text-xs hover:underline"
                    >
                      {abc.session_id.slice(0, 8)}...
                    </Link>
                  ) : (
                    <span className="text-xs">Unassigned</span>
                  )}
                </td>
                <td className="px-4 py-3 text-muted-foreground text-xs">
                  {abc.last_seen
                    ? new Date(abc.last_seen).toLocaleString()
                    : "Never"}
                </td>
              </tr>
            ))}
            {abcs.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-8 text-center text-muted-foreground"
                >
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
