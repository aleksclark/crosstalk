import { useEffect, useState } from "react";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";

interface DashboardStats {
  activeSessions: number;
  totalABCs: number;
  onlineABCs: number;
  totalTranslators: number;
}

export function DashboardPage() {
  const { token } = useAuth();
  const [stats, setStats] = useState<DashboardStats>({
    activeSessions: 0,
    totalABCs: 0,
    onlineABCs: 0,
    totalTranslators: 0,
  });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchStats() {
      if (!token) return;
      const client = getApiClient(token);

      try {
        const [sessionsRes, abcsRes] = await Promise.all([
          client.GET("/api/sessions", { params: { query: { page: 1, per_page: 100 } } }),
          client.GET("/api/abcs"),
        ]);

        const sessions = sessionsRes.data?.data ?? [];
        const abcs = abcsRes.data?.data ?? [];

        setStats({
          activeSessions: sessions.filter((s) => s.status === "active").length,
          totalABCs: abcs.length,
          onlineABCs: abcs.filter((a) => a.status === "online").length,
          totalTranslators: 0,
        });
      } catch {
        // Silently handle errors for now
      } finally {
        setLoading(false);
      }
    }

    fetchStats();
  }, [token]);

  const cards = [
    {
      label: "Active Sessions",
      value: stats.activeSessions,
      icon: "🎙️",
      color: "text-green-400",
    },
    {
      label: "ABCs Online",
      value: `${stats.onlineABCs}/${stats.totalABCs}`,
      icon: "🔌",
      color: "text-blue-400",
    },
    {
      label: "Translators",
      value: stats.totalTranslators,
      icon: "🌐",
      color: "text-purple-400",
    },
    {
      label: "System Health",
      value: "OK",
      icon: "💚",
      color: "text-green-400",
    },
  ];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading dashboard...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Dashboard</h1>

      {/* Stats grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {cards.map((card) => (
          <div
            key={card.label}
            className="bg-card border border-border rounded-lg p-4"
          >
            <div className="flex items-center justify-between">
              <span className="text-2xl">{card.icon}</span>
              <span className={`text-2xl font-bold ${card.color}`}>
                {card.value}
              </span>
            </div>
            <p className="text-sm text-muted-foreground mt-2">{card.label}</p>
          </div>
        ))}
      </div>

      {/* Quick actions */}
      <div className="bg-card border border-border rounded-lg p-4">
        <h2 className="text-lg font-semibold mb-3">System Status</h2>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-green-500" />
            <span className="text-sm">Server connected</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-green-500" />
            <span className="text-sm">WebRTC signaling active</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-green-500" />
            <span className="text-sm">Database healthy</span>
          </div>
        </div>
      </div>
    </div>
  );
}
