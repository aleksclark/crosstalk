import { useEffect, useRef, useState } from "react";

export interface UseChannelMonitorOptions {
  sessionId: string;
  token: string;
  // Channel name to monitor. When null/empty the monitor is torn down.
  channel: string | null;
  // Base origin for the signaling WebSocket. Defaults to window.location.origin.
  baseUrl?: string;
}

export interface UseChannelMonitorReturn {
  // The received audio stream, or null when not monitoring.
  stream: MediaStream | null;
  state: RTCPeerConnectionState | "idle";
}

// useChannelMonitor opens a receive-only WebRTC connection to a single session
// channel and exposes the incoming audio as a MediaStream. It is the playback
// half of "monitoring": pair it with an <audio> element (or an AnalyserNode).
//
// The connection is a client-offer flow with a single recvonly audio
// transceiver, listening to the named channel via the ?listen= query param.
// Changing `channel` tears down the old connection and opens a new one.
export function useChannelMonitor(
  opts: UseChannelMonitorOptions,
): UseChannelMonitorReturn {
  const { sessionId, token, channel, baseUrl } = opts;
  const [stream, setStream] = useState<MediaStream | null>(null);
  const [state, setState] = useState<RTCPeerConnectionState | "idle">("idle");

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const pendingRef = useRef<RTCIceCandidateInit[]>([]);

  useEffect(() => {
    if (!channel || !sessionId || !token) {
      setState("idle");
      setStream(null);
      return;
    }

    let cancelled = false;

    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });
    pcRef.current = pc;

    // The server adopts the offerer's "control" data channel.
    pc.createDataChannel("control");
    pc.addTransceiver("audio", { direction: "recvonly" });

    const remote = new MediaStream();
    setStream(remote);
    pc.ontrack = (ev) => {
      ev.streams[0]?.getTracks().forEach((t) => remote.addTrack(t));
    };
    pc.onconnectionstatechange = () => {
      if (!cancelled) setState(pc.connectionState);
    };

    const origin = baseUrl || window.location.origin;
    const wsProtocol = origin.startsWith("https") ? "wss:" : "ws:";
    const host = origin.replace(/^https?:/, "");
    const params = new URLSearchParams({ token, listen: channel });
    const ws = new WebSocket(
      `${wsProtocol}${host}/api/sessions/${sessionId}/ws?${params.toString()}`,
    );
    wsRef.current = ws;

    pc.onicecandidate = (ev) => {
      if (!ev.candidate) return;
      const payload = JSON.stringify({
        type: "candidate",
        candidate: ev.candidate.toJSON(),
      });
      if (ws.readyState === WebSocket.OPEN) ws.send(payload);
      else pendingRef.current.push(ev.candidate.toJSON());
    };

    ws.onopen = () => {
      const pending = pendingRef.current;
      pendingRef.current = [];
      for (const c of pending) {
        ws.send(JSON.stringify({ type: "candidate", candidate: c }));
      }
    };

    ws.onmessage = async (ev) => {
      const msg = JSON.parse(ev.data as string);
      if (msg.type === "answer") {
        await pc.setRemoteDescription({ type: "answer", sdp: msg.sdp });
      } else if (msg.type === "candidate") {
        try {
          await pc.addIceCandidate(new RTCIceCandidate(msg.candidate));
        } catch {
          // ignore late/duplicate candidates
        }
      }
    };

    (async () => {
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      const send = () =>
        ws.send(JSON.stringify({ type: "offer", sdp: offer.sdp }));
      if (ws.readyState === WebSocket.OPEN) send();
      else ws.addEventListener("open", send, { once: true });
    })();

    return () => {
      cancelled = true;
      pendingRef.current = [];
      ws.close();
      pc.close();
      wsRef.current = null;
      pcRef.current = null;
      setStream(null);
      setState("idle");
    };
  }, [sessionId, token, channel, baseUrl]);

  return { stream, state };
}
