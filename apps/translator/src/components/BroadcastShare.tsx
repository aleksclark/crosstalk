import { useState } from "react";
import { QRCodeSVG } from "qrcode.react";

// broadcastBaseUrl resolves the public base URL of the broadcast SPA. In
// production the server serves it at /broadcast on the same origin; in dev the
// broadcast app runs on its own port, so set VITE_BROADCAST_URL (e.g.
// http://localhost:5177/broadcast).
function broadcastBaseUrl(): string {
  const configured = import.meta.env.VITE_BROADCAST_URL;
  if (configured) return configured.replace(/\/$/, "");
  return `${window.location.origin}/broadcast`;
}

export function buildBroadcastUrl(sessionId: string, token: string): string {
  return `${broadcastBaseUrl()}/listen/${sessionId}?t=${encodeURIComponent(token)}`;
}

interface BroadcastShareProps {
  sessionId: string;
  token?: string | null;
}

export function BroadcastShare({ sessionId, token }: BroadcastShareProps) {
  const [showQR, setShowQR] = useState(false);
  const [copied, setCopied] = useState(false);

  if (!token) {
    return (
      <p className="text-sm text-gray-500">
        No broadcast token for this session yet.
      </p>
    );
  }

  const url = buildBroadcastUrl(sessionId, token);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard unavailable
    }
  };

  return (
    <div className="space-y-3">
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        className="block text-sm font-mono text-blue-400 hover:underline truncate"
        title={url}
      >
        {url}
      </a>
      <div className="flex items-center gap-2">
        <button
          onClick={copy}
          className="text-xs bg-gray-700 hover:bg-gray-600 text-gray-200 rounded px-3 py-1.5 transition-colors"
        >
          {copied ? "Copied!" : "Copy link"}
        </button>
        <button
          onClick={() => setShowQR((v) => !v)}
          className="text-xs bg-gray-700 hover:bg-gray-600 text-gray-200 rounded px-3 py-1.5 transition-colors"
        >
          {showQR ? "Hide QR code" : "Show QR code"}
        </button>
      </div>
      {showQR && (
        <div className="inline-block bg-white p-3 rounded-md">
          <QRCodeSVG value={url} size={192} level="M" />
        </div>
      )}
    </div>
  );
}
