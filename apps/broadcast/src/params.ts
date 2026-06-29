/** Parse broadcast URL parameters from the current location */
export interface BroadcastParams {
  sessionId: string | null;
  token: string | null;
  debug: boolean;
}

export function parseBroadcastParams(): BroadcastParams {
  const url = new URL(window.location.href);

  // URL format: /listen/{session_id}?t={token}&debug=1
  const pathParts = url.pathname.split("/").filter(Boolean);
  let sessionId: string | null = null;

  // Look for "listen" segment followed by session_id
  const listenIdx = pathParts.indexOf("listen");
  if (listenIdx !== -1 && listenIdx + 1 < pathParts.length) {
    sessionId = pathParts[listenIdx + 1] ?? null;
  } else if (pathParts.length > 0) {
    // Fallback: last path segment is the session_id
    sessionId = pathParts[pathParts.length - 1] ?? null;
  }

  const token = url.searchParams.get("t");
  const debug = url.searchParams.get("debug") === "1";

  return { sessionId, token, debug };
}
