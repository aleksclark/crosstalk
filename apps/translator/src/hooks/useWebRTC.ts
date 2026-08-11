import { useState, useCallback, useRef, useEffect } from "react";

export interface WebRTCEvent {
  timestamp: number;
  type: string;
  detail: string;
}

export interface WebRTCStats {
  packetsReceived: number;
  packetsSent: number;
  packetsLost: number;
  bytesReceived: number;
  bytesSent: number;
  jitter: number;
  roundTripTime: number;
}

export interface ICECandidate {
  candidate: string;
  component: string;
  type: string;
  direction: "local" | "remote";
  timestamp: number;
}

export interface UseWebRTCOptions {
  sessionId: string;
  /** Long-lived access JWT used only to mint a one-time media ticket. */
  token: string;
  audioDeviceId?: string;
  // Optional explicit SFU routing. When omitted the server routes by role
  // (translators listen to feed channels and produce into broadcast channels).
  // Comma-separated channel names; "type:feed" / "type:broadcast" select all
  // channels of a type.
  produce?: string;
  listen?: string;
}

/** High-level connect phase for truthful Operate UI (no invented auto-reconnect). */
export type ConnectPhase =
  | "idle"
  | "ticket-mint"
  | "permission"
  | "signaling"
  | "connected"
  | "failed"
  | "disconnected";

export type ConnectFailureKind =
  | "ticket-mint"
  | "permission-denied"
  | "no-device"
  | "signaling-failed"
  | "disconnected"
  | "unknown";

export interface UseWebRTCReturn {
  connectionState: RTCPeerConnectionState;
  iceState: RTCIceConnectionState;
  signalingState: RTCSignalingState;
  dataChannelState: RTCDataChannelState | "none";
  localSdp: string | null;
  remoteSdp: string | null;
  candidates: ICECandidate[];
  events: WebRTCEvent[];
  stats: WebRTCStats;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  isMuted: boolean;
  /** Operational phase for the connect workflow UI. */
  phase: ConnectPhase;
  /** Last failure classification + message, when phase is failed/disconnected. */
  lastError: { kind: ConnectFailureKind; message: string } | null;
  connect: () => Promise<void>;
  disconnect: () => void;
  toggleMute: () => void;
}

/** Mint a one-time media ticket via POST /api/webrtc/token. */
export async function mintMediaTicket(
  accessToken: string,
  sessionId: string,
  opts?: { produce?: string; listen?: string; role?: string },
): Promise<string> {
  const body: Record<string, unknown> = { session_id: sessionId };
  if (opts?.role) body.role = opts.role;
  if (opts?.produce !== undefined) {
    body.produce = opts.produce === "" ? [] : opts.produce.split(",").filter(Boolean);
  }
  if (opts?.listen !== undefined) {
    body.listen = opts.listen === "" ? [] : opts.listen.split(",").filter(Boolean);
  }

  const resp = await fetch("/api/webrtc/token", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    const text = await resp.text().catch(() => "");
    throw new Error(`media ticket mint failed (${resp.status}): ${text || resp.statusText}`);
  }
  const data = (await resp.json()) as { token?: string };
  if (!data.token) {
    throw new Error("media ticket mint returned empty token");
  }
  return data.token;
}

export function useWebRTC(options: UseWebRTCOptions): UseWebRTCReturn {
  const { sessionId, token, audioDeviceId, produce, listen } = options;

  const [connectionState, setConnectionState] = useState<RTCPeerConnectionState>("new");
  const [iceState, setIceState] = useState<RTCIceConnectionState>("new");
  const [signalingState, setSignalingState] = useState<RTCSignalingState>("stable");
  const [dataChannelState, setDataChannelState] = useState<RTCDataChannelState | "none">("none");
  const [localSdp, setLocalSdp] = useState<string | null>(null);
  const [remoteSdp, setRemoteSdp] = useState<string | null>(null);
  const [candidates, setCandidates] = useState<ICECandidate[]>([]);
  const [events, setEvents] = useState<WebRTCEvent[]>([]);
  const [stats, setStats] = useState<WebRTCStats>({
    packetsReceived: 0,
    packetsSent: 0,
    packetsLost: 0,
    bytesReceived: 0,
    bytesSent: 0,
    jitter: 0,
    roundTripTime: 0,
  });
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [isMuted, setIsMuted] = useState(false);
  const [phase, setPhase] = useState<ConnectPhase>("idle");
  const [lastError, setLastError] = useState<{ kind: ConnectFailureKind; message: string } | null>(
    null,
  );

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const dcRef = useRef<RTCDataChannel | null>(null);
  const pendingCandidatesRef = useRef<RTCIceCandidateInit[]>([]);
  const pendingRemoteCandidatesRef = useRef<RTCIceCandidateInit[]>([]);
  const statsIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const intentionalCloseRef = useRef(false);

  const addEvent = useCallback((type: string, detail: string) => {
    setEvents((prev) => [...prev, { timestamp: Date.now(), type, detail }]);
  }, []);

  const fail = useCallback((kind: ConnectFailureKind, message: string) => {
    setLastError({ kind, message });
    setPhase(kind === "disconnected" ? "disconnected" : "failed");
    setConnectionState("failed");
  }, []);

  const pollStats = useCallback(async () => {
    const pc = pcRef.current;
    if (!pc) return;
    try {
      const report = await pc.getStats();
      let packetsReceived = 0;
      let packetsSent = 0;
      let packetsLost = 0;
      let bytesReceived = 0;
      let bytesSent = 0;
      let jitter = 0;
      let roundTripTime = 0;
      report.forEach((s) => {
        if (s.type === "inbound-rtp" && s.kind === "audio") {
          packetsReceived = s.packetsReceived ?? 0;
          packetsLost = s.packetsLost ?? 0;
          bytesReceived = s.bytesReceived ?? 0;
          jitter = s.jitter ?? 0;
        }
        if (s.type === "outbound-rtp" && s.kind === "audio") {
          packetsSent = s.packetsSent ?? 0;
          bytesSent = s.bytesSent ?? 0;
        }
        if (s.type === "candidate-pair" && s.state === "succeeded") {
          roundTripTime = s.currentRoundTripTime ?? 0;
        }
      });
      setStats({ packetsReceived, packetsSent, packetsLost, bytesReceived, bytesSent, jitter, roundTripTime });
    } catch {
      // ignore stats errors
    }
  }, []);

  const connect = useCallback(async () => {
    intentionalCloseRef.current = false;
    setLastError(null);
    addEvent("connect", `Starting connection to session ${sessionId}`);
    setConnectionState("connecting");
    setPhase("ticket-mint");

    // Mint one-time media ticket (access JWT is not a WS admit credential).
    addEvent("auth", "Requesting media ticket");
    let mediaTicket: string;
    try {
      mediaTicket = await mintMediaTicket(token, sessionId, { produce, listen });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      addEvent("error", msg);
      fail("ticket-mint", msg);
      throw err;
    }
    addEvent("auth", "Media ticket obtained");

    // Get user media
    setPhase("permission");
    const constraints: MediaStreamConstraints = {
      audio: audioDeviceId ? { deviceId: { exact: audioDeviceId } } : true,
      video: false,
    };
    let stream: MediaStream;
    try {
      stream = await navigator.mediaDevices.getUserMedia(constraints);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      addEvent("error", `getUserMedia failed: ${msg}`);
      const name = err instanceof DOMException ? err.name : "";
      if (name === "NotAllowedError" || name === "PermissionDeniedError") {
        fail("permission-denied", "Microphone permission denied. Allow access and reconnect.");
      } else if (name === "NotFoundError" || name === "DevicesNotFoundError") {
        fail("no-device", "No microphone found. Connect a device and reconnect.");
      } else if (name === "OverconstrainedError" || name === "ConstraintNotSatisfiedError") {
        fail("no-device", "Selected microphone is unavailable. Choose another device and reconnect.");
      } else {
        fail("permission-denied", msg);
      }
      throw err;
    }
    setLocalStream(stream);
    addEvent("media", "Local audio stream acquired");

    // Create peer connection
    setPhase("signaling");
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });
    pcRef.current = pc;

    // Control data channel — the offerer opens it; the server adopts it.
    // Its readyState surfaces in the debug panel and mirrors the server-side
    // "data channel open" view.
    const dc = pc.createDataChannel("control");
    dcRef.current = dc;
    setDataChannelState(dc.readyState);
    dc.onopen = () => {
      setDataChannelState(dc.readyState);
      addEvent("dataChannel", "Control channel open");
    };
    dc.onclose = () => {
      setDataChannelState(dc.readyState);
      addEvent("dataChannel", "Control channel closed");
    };
    dc.onerror = (ev) => {
      addEvent("error", `Control channel error: ${String((ev as RTCErrorEvent).error?.message ?? ev)}`);
    };
    dc.onmessage = (ev) => {
      addEvent("dataChannel", `Control message (${(ev.data as string).length ?? 0} bytes)`);
    };

    // Add tracks
    stream.getTracks().forEach((track) => pc.addTrack(track, stream));

    // Remote stream
    const remote = new MediaStream();
    setRemoteStream(remote);
    pc.ontrack = (ev) => {
      ev.streams[0]?.getTracks().forEach((track) => remote.addTrack(track));
      addEvent("track", `Remote track added: ${ev.track.kind}`);
    };

    // Connection state
    pc.onconnectionstatechange = () => {
      setConnectionState(pc.connectionState);
      addEvent("connectionState", pc.connectionState);
      if (pc.connectionState === "connected") {
        setPhase("connected");
        setLastError(null);
      } else if (pc.connectionState === "failed") {
        if (!intentionalCloseRef.current) {
          fail("signaling-failed", "Peer connection failed. Reconnect to try again.");
        }
      } else if (pc.connectionState === "disconnected") {
        if (!intentionalCloseRef.current) {
          setPhase("disconnected");
          setLastError({
            kind: "disconnected",
            message: "Connection dropped. Reconnect when ready — no automatic reconnect.",
          });
        }
      } else if (pc.connectionState === "closed") {
        if (!intentionalCloseRef.current) {
          setPhase("disconnected");
        }
      }
    };
    pc.oniceconnectionstatechange = () => {
      setIceState(pc.iceConnectionState);
      addEvent("iceConnectionState", pc.iceConnectionState);
      if (
        (pc.iceConnectionState === "failed" || pc.iceConnectionState === "disconnected") &&
        !intentionalCloseRef.current
      ) {
        if (pc.iceConnectionState === "failed") {
          fail("signaling-failed", "ICE connection failed. Reconnect to try again.");
        } else {
          setPhase("disconnected");
          setLastError({
            kind: "disconnected",
            message: "ICE disconnected. Reconnect when ready — no automatic reconnect.",
          });
        }
      }
    };
    pc.onsignalingstatechange = () => {
      setSignalingState(pc.signalingState);
      addEvent("signalingState", pc.signalingState);
    };

    // ICE candidates
    pc.onicecandidate = (ev) => {
      if (ev.candidate) {
        const c: ICECandidate = {
          candidate: ev.candidate.candidate,
          component: ev.candidate.component ?? "unknown",
          type: ev.candidate.type ?? "unknown",
          direction: "local",
          timestamp: Date.now(),
        };
        setCandidates((prev) => [...prev, c]);
        addEvent("iceCandidate", `local ${c.type} ${c.component}: ${c.candidate.slice(0, 60)}`);
        // Send to the signaling server. Candidates can be produced before the
        // WebSocket finishes opening, so queue them and flush once it's open.
        const payload = JSON.stringify({ type: "candidate", candidate: ev.candidate.toJSON() });
        const sock = wsRef.current;
        if (sock && sock.readyState === WebSocket.OPEN) {
          sock.send(payload);
        } else {
          pendingCandidatesRef.current.push(ev.candidate.toJSON());
        }
      } else {
        addEvent("iceGatheringComplete", "All ICE candidates gathered");
      }
    };

    pc.onicegatheringstatechange = () => {
      addEvent("iceGatheringState", pc.iceGatheringState);
    };

    // WebSocket signaling with one-time media ticket (not access JWT).
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const params = new URLSearchParams({ token: mediaTicket });
    // Presence matters: a defined-but-empty value ("") means "route nothing in
    // this direction", while an undefined value means "let the server apply the
    // role default". The server distinguishes these via param presence, so only
    // omit the param when it is undefined.
    if (produce !== undefined) params.set("produce", produce);
    if (listen !== undefined) params.set("listen", listen);
    const wsUrl = `${wsProtocol}//${window.location.host}/api/sessions/${sessionId}/ws?${params.toString()}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      addEvent("signaling", "WebSocket connected");
      // Flush any ICE candidates gathered before the socket opened.
      const pending = pendingCandidatesRef.current;
      pendingCandidatesRef.current = [];
      for (const cand of pending) {
        ws.send(JSON.stringify({ type: "candidate", candidate: cand }));
      }
      if (pending.length > 0) {
        addEvent("iceCandidate", `Flushed ${pending.length} queued local candidate(s)`);
      }
    };

    ws.onmessage = async (ev) => {
      const msg = JSON.parse(ev.data as string);
      addEvent("signaling", `Received: ${msg.type}`);

      if (msg.type === "answer") {
        await pc.setRemoteDescription({ type: "answer", sdp: msg.sdp });
        setRemoteSdp(msg.sdp);
        addEvent("sdp", "Remote description set (answer)");
        const pending = pendingRemoteCandidatesRef.current;
        pendingRemoteCandidatesRef.current = [];
        for (const candidate of pending) {
          await pc.addIceCandidate(new RTCIceCandidate(candidate));
        }
      } else if (msg.type === "offer") {
        await pc.setRemoteDescription({ type: "offer", sdp: msg.sdp });
        setRemoteSdp(msg.sdp);
        addEvent("sdp", "Remote description set (offer)");
        const pending = pendingRemoteCandidatesRef.current;
        pendingRemoteCandidatesRef.current = [];
        for (const candidate of pending) {
          await pc.addIceCandidate(new RTCIceCandidate(candidate));
        }
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        setLocalSdp(answer.sdp ?? null);
        ws.send(JSON.stringify({ type: "answer", sdp: answer.sdp }));
        addEvent("sdp", "Sent answer");
      } else if (msg.type === "candidate") {
        const init = msg.candidate as RTCIceCandidateInit;
        if (pc.remoteDescription) {
          await pc.addIceCandidate(new RTCIceCandidate(init));
        } else {
          pendingRemoteCandidatesRef.current.push(init);
        }
        const raw = init.candidate ?? "";
        setCandidates((prev) => [
          ...prev,
          {
            candidate: raw,
            component: raw.includes("component") ? "unknown" : String(init.sdpMLineIndex ?? "unknown"),
            type: raw.split(" ")[7] ?? "remote",
            direction: "remote",
            timestamp: Date.now(),
          },
        ]);
        addEvent("iceCandidate", `remote: ${raw.slice(0, 60)}`);
      }
    };

    ws.onerror = (ev) => {
      addEvent("signaling", `WebSocket error: ${String(ev)}`);
      if (!intentionalCloseRef.current) {
        fail("signaling-failed", "Signaling WebSocket error. Reconnect to try again.");
      }
    };

    ws.onclose = (ev) => {
      addEvent("signaling", `WebSocket closed: code=${ev.code} reason=${ev.reason}`);
      if (!intentionalCloseRef.current && pc.connectionState !== "connected") {
        fail(
          "signaling-failed",
          ev.reason
            ? `Signaling closed (${ev.code}): ${ev.reason}`
            : `Signaling closed (code ${ev.code}). Reconnect to try again.`,
        );
      }
    };

    // Create offer
    let offer: RTCSessionDescriptionInit;
    try {
      offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      setLocalSdp(JSON.stringify(offer, null, 2));
      addEvent("sdp", "Created and set local offer");
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      addEvent("error", msg);
      fail("signaling-failed", msg);
      throw err;
    }

    // Wait for WS to be open before sending
    const sendOffer = () => {
      ws.send(JSON.stringify({ type: "offer", sdp: offer.sdp }));
      addEvent("signaling", "Sent offer via WebSocket");
    };
    if (ws.readyState === WebSocket.OPEN) {
      sendOffer();
    } else {
      ws.addEventListener("open", sendOffer, { once: true });
    }

    // Start stats polling
    statsIntervalRef.current = setInterval(pollStats, 1000);
  }, [sessionId, token, audioDeviceId, produce, listen, addEvent, pollStats, fail]);

  const disconnect = useCallback(() => {
    intentionalCloseRef.current = true;
    if (statsIntervalRef.current) {
      clearInterval(statsIntervalRef.current);
      statsIntervalRef.current = null;
    }
    dcRef.current?.close();
    dcRef.current = null;
    pendingCandidatesRef.current = [];
    pendingRemoteCandidatesRef.current = [];
    wsRef.current?.close();
    wsRef.current = null;
    pcRef.current?.close();
    pcRef.current = null;
    localStream?.getTracks().forEach((t) => t.stop());
    setLocalStream(null);
    setRemoteStream(null);
    setConnectionState("closed");
    setIceState("closed");
    setDataChannelState("closed");
    setPhase("idle");
    setLastError(null);
    setIsMuted(false);
    addEvent("disconnect", "Connection closed");
  }, [localStream, addEvent]);

  const toggleMute = useCallback(() => {
    if (localStream) {
      localStream.getAudioTracks().forEach((track) => {
        track.enabled = !track.enabled;
      });
      setIsMuted((prev) => !prev);
      addEvent("mute", isMuted ? "Unmuted" : "Muted");
    }
  }, [localStream, isMuted, addEvent]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (statsIntervalRef.current) clearInterval(statsIntervalRef.current);
      wsRef.current?.close();
      pcRef.current?.close();
      localStream?.getTracks().forEach((t) => t.stop());
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    connectionState,
    iceState,
    signalingState,
    dataChannelState,
    localSdp,
    remoteSdp,
    candidates,
    events,
    stats,
    localStream,
    remoteStream,
    isMuted,
    phase,
    lastError,
    connect,
    disconnect,
    toggleMute,
  };
}
