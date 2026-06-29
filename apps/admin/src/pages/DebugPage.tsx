import { useState } from "react";
import { DebugPanel } from "../components/DebugPanel";

interface Peer {
  id: string;
  type: string;
  remoteAddress: string;
  state: string;
  sessionId: string | null;
}

export function DebugPage() {
  const [peers] = useState<Peer[]>([
    {
      id: "peer-001",
      type: "ABC",
      remoteAddress: "192.168.1.100:54321",
      state: "connected",
      sessionId: "session-abc",
    },
    {
      id: "peer-002",
      type: "Translator",
      remoteAddress: "192.168.1.101:54322",
      state: "connected",
      sessionId: "session-abc",
    },
    {
      id: "peer-003",
      type: "Admin",
      remoteAddress: "192.168.1.1:54323",
      state: "new",
      sessionId: null,
    },
  ]);

  const [events] = useState([
    {
      id: "1",
      timestamp: new Date().toISOString(),
      type: "ICE",
      message: "ICE candidate gathered",
      data: { candidate: "candidate:1 1 udp 2122260223 ..." },
    },
    {
      id: "2",
      timestamp: new Date(Date.now() - 1000).toISOString(),
      type: "SDP",
      message: "Offer created",
    },
    {
      id: "3",
      timestamp: new Date(Date.now() - 2000).toISOString(),
      type: "DTLS",
      message: "DTLS handshake complete",
    },
    {
      id: "4",
      timestamp: new Date(Date.now() - 3000).toISOString(),
      type: "TRACK",
      message: "Audio track added",
      data: { trackId: "audio-001", kind: "audio" },
    },
    {
      id: "5",
      timestamp: new Date(Date.now() - 5000).toISOString(),
      type: "CONNECTION",
      message: "Peer connection state: connected",
    },
  ]);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Debug</h1>

      {/* Peer list */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-border">
          <h2 className="text-sm font-semibold">Connected Peers</h2>
        </div>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Peer ID
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Type
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Address
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                State
              </th>
              <th className="text-left px-4 py-2 text-muted-foreground font-medium text-xs">
                Session
              </th>
            </tr>
          </thead>
          <tbody>
            {peers.map((peer) => (
              <tr
                key={peer.id}
                className="border-b border-border/50 hover:bg-accent/50"
              >
                <td className="px-4 py-2 font-mono text-xs">{peer.id}</td>
                <td className="px-4 py-2 text-xs">{peer.type}</td>
                <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                  {peer.remoteAddress}
                </td>
                <td className="px-4 py-2">
                  <span
                    className={`text-xs px-2 py-0.5 rounded ${
                      peer.state === "connected"
                        ? "bg-green-500/20 text-green-400"
                        : "bg-yellow-500/20 text-yellow-400"
                    }`}
                  >
                    {peer.state}
                  </span>
                </td>
                <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                  {peer.sessionId ?? "-"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Session topology */}
      <div className="bg-card border border-border rounded-lg p-4">
        <h2 className="text-sm font-semibold mb-3">Session Topology</h2>
        <div className="flex items-center justify-center py-8">
          <div className="flex items-center gap-4">
            <div className="bg-muted rounded-lg px-4 py-3 text-center">
              <span className="text-xs text-muted-foreground">ABC</span>
              <p className="text-sm font-medium mt-1">peer-001</p>
            </div>
            <div className="w-12 border-t border-dashed border-border" />
            <div className="bg-primary/20 rounded-lg px-4 py-3 text-center">
              <span className="text-xs text-primary">Server</span>
              <p className="text-sm font-medium mt-1">SFU</p>
            </div>
            <div className="w-12 border-t border-dashed border-border" />
            <div className="bg-muted rounded-lg px-4 py-3 text-center">
              <span className="text-xs text-muted-foreground">Translator</span>
              <p className="text-sm font-medium mt-1">peer-002</p>
            </div>
          </div>
        </div>
      </div>

      {/* Event log */}
      <DebugPanel events={events} title="WebRTC Event Log" />
    </div>
  );
}
