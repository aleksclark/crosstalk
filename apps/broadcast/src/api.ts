/** Broadcast API types */
export interface BroadcastInfo {
  session_id: string;
  session_name: string;
  /**
   * Present only when the server reports a real count.
   * Omit/undefined → UI must hide the listener count (never invent one).
   */
  listener_count?: number;
  ice_servers?: RTCIceServer[];
  /** When false, the session/broadcast is not active. */
  active?: boolean;
}

export interface BroadcastError {
  error: string;
  code: string;
}

const API_BASE = import.meta.env.VITE_API_BASE ?? "";

/**
 * Fetch broadcast info from the public API.
 * No auth required — token in query param is the credential.
 *
 * On reconnect the SPA may re-call this to confirm the link is still valid
 * before opening a fresh WS. The URL token is the durable broadcast credential
 * (not a one-shot media ticket); each WS connect is a new media session.
 */
export async function fetchBroadcastInfo(
  sessionId: string,
  token: string,
): Promise<BroadcastInfo> {
  const res = await fetch(
    `${API_BASE}/api/sessions/${sessionId}/broadcast?token=${encodeURIComponent(token)}`,
  );

  if (!res.ok) {
    if (res.status === 404) {
      // Session gone or ended.
      let body: BroadcastError | null = null;
      try {
        body = (await res.json()) as BroadcastError;
      } catch {
        /* ignore */
      }
      const msg = body?.error?.toLowerCase() ?? "";
      if (msg.includes("ended") || body?.code === "SESSION_ENDED") {
        throw new BroadcastApiError("This broadcast has ended", "SESSION_ENDED");
      }
      throw new BroadcastApiError("Invalid or expired broadcast link", "INVALID_TOKEN");
    }
    if (res.status === 401 || res.status === 403) {
      throw new BroadcastApiError("Invalid or expired broadcast link", "INVALID_TOKEN");
    }
    throw new BroadcastApiError("Failed to load broadcast info", "FETCH_ERROR");
  }

  const data = (await res.json()) as BroadcastInfo & { active?: boolean };
  if (data.active === false) {
    throw new BroadcastApiError("This broadcast has ended", "SESSION_ENDED");
  }
  return data;
}

export class BroadcastApiError extends Error {
  constructor(
    message: string,
    public readonly code: string,
  ) {
    super(message);
    this.name = "BroadcastApiError";
  }
}
