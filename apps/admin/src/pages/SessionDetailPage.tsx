import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import { MixPanel } from "../components/MixPanel";
import type { components } from "@crosstalk/api-client";

type Session = components["schemas"]["Session"];

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { token } = useAuth();
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchSession() {
      if (!token || !id) return;
      const client = getApiClient(token);
      try {
        const { data } = await client.GET("/api/sessions/{id}", {
          params: { path: { id } },
        });
        if (data) {
          setSession(data);
        }
      } catch {
        // handle error
      } finally {
        setLoading(false);
      }
    }
    fetchSession();
  }, [token, id]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">Loading session...</p>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">Session not found</p>
        <Link to="/sessions" className="text-primary text-sm mt-2 inline-block">
          ← Back to sessions
        </Link>
      </div>
    );
  }

  // Demo mix panel data
  const demoSources = [
    { id: "1", name: "Floor Mic", level: 65, muted: false, volume: 80 },
    { id: "2", name: "Translator A", level: 45, muted: false, volume: 70 },
    { id: "3", name: "Translator B", level: 0, muted: true, volume: 60 },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <Link
              to="/sessions"
              className="text-muted-foreground hover:text-foreground text-sm"
            >
              Sessions /
            </Link>
            <h1 className="text-2xl font-bold">{session.name}</h1>
          </div>
          <p className="text-muted-foreground text-sm mt-1">ID: {session.id}</p>
        </div>
        <span
          className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-medium ${
            session.status === "active"
              ? "bg-green-500/20 text-green-400"
              : "bg-gray-500/20 text-gray-400"
          }`}
        >
          <span className="w-2 h-2 rounded-full bg-current" />
          {session.status}
        </span>
      </div>

      {/* Session info cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Channels
          </h3>
          <p className="text-2xl font-bold mt-1">
            {session.channel_count ?? 0}
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Connected Clients
          </h3>
          <p className="text-2xl font-bold mt-1">
            {session.client_count ?? 0}
          </p>
        </div>
        <div className="bg-card border border-border rounded-lg p-4">
          <h3 className="text-sm font-medium text-muted-foreground">
            Broadcast URL
          </h3>
          <p className="text-sm font-mono mt-2 text-primary truncate">
            /session/{session.id}/broadcast
          </p>
        </div>
      </div>

      {/* Mix controls */}
      <div>
        <h2 className="text-lg font-semibold mb-3">Mix Controls</h2>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <MixPanel channelName="Channel 1 — Floor" sources={demoSources} />
          <MixPanel
            channelName="Channel 2 — Translation"
            sources={[
              { id: "4", name: "EN→ES", level: 55, muted: false, volume: 75 },
              { id: "5", name: "EN→FR", level: 30, muted: false, volume: 70 },
            ]}
          />
        </div>
      </div>

      {/* Sources & Recordings */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-card border border-border rounded-lg p-4">
          <h2 className="text-lg font-semibold mb-3">Audio Sources</h2>
          <div className="space-y-2">
            <div className="flex items-center justify-between py-2 border-b border-border/50">
              <span className="text-sm">Floor Microphone</span>
              <span className="text-xs text-green-400">Connected</span>
            </div>
            <div className="flex items-center justify-between py-2 border-b border-border/50">
              <span className="text-sm">ABC-01</span>
              <span className="text-xs text-green-400">Connected</span>
            </div>
            <div className="flex items-center justify-between py-2">
              <span className="text-sm">ABC-02</span>
              <span className="text-xs text-yellow-400">Reconnecting</span>
            </div>
          </div>
        </div>

        <div className="bg-card border border-border rounded-lg p-4">
          <h2 className="text-lg font-semibold mb-3">Recordings</h2>
          <div className="space-y-2 text-sm text-muted-foreground">
            <p>No recordings for this session.</p>
          </div>
        </div>
      </div>
    </div>
  );
}
