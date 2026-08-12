import { useEffect, useId, useRef, useState, type CSSProperties } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Button, Status } from "@crosstalk/theme";

// broadcastBaseUrl resolves the public base URL of the broadcast SPA. In
// production the server serves it at /broadcast on the same origin; in dev the
// broadcast app runs on its own port, so set VITE_BROADCAST_URL (e.g.
// http://localhost:5177/broadcast).
//
// Optional `baseUrl` prop overrides env/origin for non-Vite hosts or tests.
function resolveBroadcastBaseUrl(baseUrl?: string): string {
  if (baseUrl) return baseUrl.replace(/\/$/, "");
  try {
    // Vite injects import.meta.env at build time in SPA bundles.
    const env = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env;
    const configured = env?.VITE_BROADCAST_URL;
    if (configured) return configured.replace(/\/$/, "");
  } catch {
    // import.meta may be unavailable in some non-bundled contexts.
  }
  if (typeof window !== "undefined" && window.location?.origin) {
    return `${window.location.origin}/broadcast`;
  }
  return "/broadcast";
}

/** Build the public listener URL. Token stays in the query string only. */
export function buildBroadcastUrl(
  sessionId: string,
  token: string,
  baseUrl?: string,
): string {
  return `${resolveBroadcastBaseUrl(baseUrl)}/listen/${sessionId}?t=${encodeURIComponent(token)}`;
}

/** Redact token query values from a URL for display/title text. */
export function redactBroadcastUrl(url: string): string {
  try {
    const u = new URL(url, "http://local.invalid");
    if (u.searchParams.has("t")) {
      u.searchParams.set("t", "…");
    }
    // Preserve relative vs absolute form from input when possible.
    if (url.startsWith("http://") || url.startsWith("https://")) {
      return u.toString();
    }
    return `${u.pathname}${u.search}${u.hash}`;
  } catch {
    return url.replace(/([?&]t=)[^&]*/i, "$1…");
  }
}

export interface BroadcastShareProps {
  sessionId: string;
  token?: string | null;
  /** Optional override for the broadcast SPA base URL. */
  baseUrl?: string;
  className?: string;
  style?: CSSProperties;
}

export function BroadcastShare({
  sessionId,
  token,
  baseUrl,
  className,
  style,
}: BroadcastShareProps) {
  const [showQR, setShowQR] = useState(false);
  const [copied, setCopied] = useState(false);
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const qrTitleId = useId();

  useEffect(
    () => () => {
      if (copyTimer.current) clearTimeout(copyTimer.current);
    },
    [],
  );

  if (!token) {
    return (
      <p
        className={className}
        style={{
          margin: 0,
          font: "400 var(--house-type-body) / var(--house-leading-body) var(--house-font-product)",
          color: "var(--house-text-tertiary)",
          ...style,
        }}
      >
        No broadcast token for this session yet.
      </p>
    );
  }

  const url = buildBroadcastUrl(sessionId, token, baseUrl);
  const displayUrl = redactBroadcastUrl(url);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard unavailable — leave state unchanged
    }
  };

  return (
    <div
      className={className}
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--house-space-3)",
        ...style,
      }}
    >
      <div style={{ minWidth: 0 }}>
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={`Open broadcast listener for session ${sessionId} (opens in a new tab)`}
          title={displayUrl}
          style={{
            display: "block",
            font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
            color: "var(--house-accent)",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {displayUrl}
        </a>
      </div>

      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: "var(--house-space-2)",
        }}
      >
        <Button
          variant="secondary"
          icon={copied ? "check" : "copy"}
          onClick={() => void copy()}
          aria-label={
            copied
              ? "Broadcast link copied"
              : `Copy broadcast listener link for session ${sessionId}`
          }
        >
          {copied ? "Copied" : "Copy link"}
        </Button>
        <Button
          variant="ghost"
          icon="grid"
          onClick={() => setShowQR((v) => !v)}
          aria-expanded={showQR}
          aria-controls={showQR ? qrTitleId : undefined}
          aria-label={
            showQR
              ? `Hide broadcast QR code for session ${sessionId}`
              : `Show broadcast QR code for session ${sessionId}`
          }
        >
          {showQR ? "Hide QR code" : "Show QR code"}
        </Button>
        {copied ? <Status tone="ok">Link copied</Status> : null}
      </div>

      {showQR ? (
        <div
          id={qrTitleId}
          role="img"
          aria-label={`QR code for broadcast listener link, session ${sessionId}`}
          style={{
            display: "inline-block",
            padding: "var(--house-space-3)",
            background: "#ffffff",
            borderRadius: "var(--house-radius-md)",
            border: "1px solid var(--house-rule-subtle)",
          }}
        >
          <QRCodeSVG value={url} size={192} level="M" />
        </div>
      ) : null}
    </div>
  );
}
