/** Broadcast API types */
export interface BroadcastInfo {
  session_id: string;
  session_name: string;
  listener_count: number;
  ice_servers?: RTCIceServer[];
}

export interface BroadcastError {
  error: string;
  code: string;
}

const API_BASE = import.meta.env.VITE_API_BASE ?? "";

/**
 * Fetch broadcast info from the public API.
 * No auth required — token in query param is the credential.
 */
export async function fetchBroadcastInfo(
  sessionId: string,
  token: string,
): Promise<BroadcastInfo> {
  const res = await fetch(
    `${API_BASE}/api/sessions/${sessionId}/broadcast?token=${encodeURIComponent(token)}`,
  );

  if (!res.ok) {
    if (res.status === 404 || res.status === 403) {
      throw new BroadcastApiError("Invalid or expired broadcast link", "INVALID_TOKEN");
    }
    throw new BroadcastApiError("Failed to load broadcast info", "FETCH_ERROR");
  }

  return res.json() as Promise<BroadcastInfo>;
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
