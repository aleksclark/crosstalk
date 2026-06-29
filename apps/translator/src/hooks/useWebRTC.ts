import { useState, useCallback, useRef, useEffect } from "react";

export interface WebRTCEvent {
  timestamp: number;
  type: string;
  detail: string;
}

export interface WebRTCStats {
  packetsReceived: number;
  packetsSent: number;
  bytesReceived: number;
  bytesSent: number;
  jitter: number;
  roundTripTime: number;
}

export interface ICECandidate {
  candidate: string;
  component: string;
  type: string;
  timestamp: number;
}

export interface UseWebRTCOptions {
  sessionId: string;
  token: string;
  audioDeviceId?: string;
}

export interface UseWebRTCReturn {
  connectionState: RTCPeerConnectionState;
  iceState: RTCIceConnectionState;
  signalingState: RTCSignalingState;
  localSdp: string | null;
  remoteSdp: string | null;
  candidates: ICECandidate[];
  events: WebRTCEvent[];
  stats: WebRTCStats;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  isMuted: boolean;
  connect: () => Promise<void>;
  disconnect: () => void;
  toggleMute: () => void;
}

export function useWebRTC(options: UseWebRTCOptions): UseWebRTCReturn {
  const { sessionId, token, audioDeviceId } = options;

  const [connectionState, setConnectionState] = useState<RTCPeerConnectionState>("new");
  const [iceState, setIceState] = useState<RTCIceConnectionState>("new");
  const [signalingState, setSignalingState] = useState<RTCSignalingState>("stable");
  const [localSdp, setLocalSdp] = useState<string | null>(null);
  const [remoteSdp, setRemoteSdp] = useState<string | null>(null);
  const [candidates, setCandidates] = useState<ICECandidate[]>([]);
  const [events, setEvents] = useState<WebRTCEvent[]>([]);
  const [stats, setStats] = useState<WebRTCStats>({
    packetsReceived: 0,
    packetsSent: 0,
    bytesReceived: 0,
    bytesSent: 0,
    jitter: 0,
    roundTripTime: 0,
  });
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [isMuted, setIsMuted] = useState(false);

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const statsIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const addEvent = useCallback((type: string, detail: string) => {
    setEvents((prev) => [...prev, { timestamp: Date.now(), type, detail }]);
  }, []);

  const pollStats = useCallback(async () => {
    const pc = pcRef.current;
    if (!pc) return;
    try {
      const report = await pc.getStats();
      let packetsReceived = 0;
      let packetsSent = 0;
      let bytesReceived = 0;
      let bytesSent = 0;
      let jitter = 0;
      let roundTripTime = 0;
      report.forEach((s) => {
        if (s.type === "inbound-rtp" && s.kind === "audio") {
          packetsReceived = s.packetsReceived ?? 0;
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
      setStats({ packetsReceived, packetsSent, bytesReceived, bytesSent, jitter, roundTripTime });
    } catch {
      // ignore stats errors
    }
  }, []);

  const connect = useCallback(async () => {
    addEvent("connect", `Starting connection to session ${sessionId}`);

    // Get user media
    const constraints: MediaStreamConstraints = {
      audio: audioDeviceId ? { deviceId: { exact: audioDeviceId } } : true,
      video: false,
    };
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    setLocalStream(stream);
    addEvent("media", "Local audio stream acquired");

    // Create peer connection
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });
    pcRef.current = pc;

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
    };
    pc.oniceconnectionstatechange = () => {
      setIceState(pc.iceConnectionState);
      addEvent("iceConnectionState", pc.iceConnectionState);
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
          timestamp: Date.now(),
        };
        setCandidates((prev) => [...prev, c]);
        addEvent("iceCandidate", `${c.type} ${c.component}: ${c.candidate.slice(0, 60)}`);
        // Send to signaling server
        wsRef.current?.send(JSON.stringify({ type: "candidate", candidate: ev.candidate.toJSON() }));
      } else {
        addEvent("iceGatheringComplete", "All ICE candidates gathered");
      }
    };

    pc.onicegatheringstatechange = () => {
      addEvent("iceGatheringState", pc.iceGatheringState);
    };

    // WebSocket signaling
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${wsProtocol}//${window.location.host}/api/sessions/${sessionId}/ws?token=${token}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      addEvent("signaling", "WebSocket connected");
    };

    ws.onmessage = async (ev) => {
      const msg = JSON.parse(ev.data as string);
      addEvent("signaling", `Received: ${msg.type}`);

      if (msg.type === "answer") {
        const desc = new RTCSessionDescription(msg.sdp);
        await pc.setRemoteDescription(desc);
        setRemoteSdp(JSON.stringify(msg.sdp, null, 2));
        addEvent("sdp", "Remote description set (answer)");
      } else if (msg.type === "offer") {
        const desc = new RTCSessionDescription(msg.sdp);
        await pc.setRemoteDescription(desc);
        setRemoteSdp(JSON.stringify(msg.sdp, null, 2));
        addEvent("sdp", "Remote description set (offer)");
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        setLocalSdp(JSON.stringify(answer, null, 2));
        ws.send(JSON.stringify({ type: "answer", sdp: answer }));
        addEvent("sdp", "Sent answer");
      } else if (msg.type === "candidate") {
        await pc.addIceCandidate(new RTCIceCandidate(msg.candidate));
        addEvent("iceCandidate", "Added remote ICE candidate");
      }
    };

    ws.onerror = (ev) => {
      addEvent("signaling", `WebSocket error: ${String(ev)}`);
    };

    ws.onclose = (ev) => {
      addEvent("signaling", `WebSocket closed: code=${ev.code} reason=${ev.reason}`);
    };

    // Create offer
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    setLocalSdp(JSON.stringify(offer, null, 2));
    addEvent("sdp", "Created and set local offer");

    // Wait for WS to be open before sending
    const sendOffer = () => {
      ws.send(JSON.stringify({ type: "offer", sdp: offer }));
      addEvent("signaling", "Sent offer via WebSocket");
    };
    if (ws.readyState === WebSocket.OPEN) {
      sendOffer();
    } else {
      ws.addEventListener("open", sendOffer, { once: true });
    }

    // Start stats polling
    statsIntervalRef.current = setInterval(pollStats, 1000);
  }, [sessionId, token, audioDeviceId, addEvent, pollStats]);

  const disconnect = useCallback(() => {
    if (statsIntervalRef.current) {
      clearInterval(statsIntervalRef.current);
      statsIntervalRef.current = null;
    }
    wsRef.current?.close();
    wsRef.current = null;
    pcRef.current?.close();
    pcRef.current = null;
    localStream?.getTracks().forEach((t) => t.stop());
    setLocalStream(null);
    setRemoteStream(null);
    setConnectionState("closed");
    setIceState("closed");
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
    localSdp,
    remoteSdp,
    candidates,
    events,
    stats,
    localStream,
    remoteStream,
    isMuted,
    connect,
    disconnect,
    toggleMute,
  };
}
