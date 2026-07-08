import { useCallback, useEffect, useRef, useState } from "react";
import { useAudioLevel } from "@crosstalk/theme";
import type { BroadcastInfo } from "./api";

export type ConnectionState = "idle" | "connecting" | "live" | "error" | "closed";

export interface DebugInfo {
  iceState: string;
  signalingState: string;
  candidates: string[];
  localSdp: string | null;
  remoteSdp: string | null;
}

export interface UseBroadcastListenerOptions {
  sessionId: string;
  token: string;
  info: BroadcastInfo;
  enabled: boolean; // only connect after user gesture
}

export interface UseBroadcastListenerReturn {
  state: ConnectionState;
  error: string | null;
  volume: number;
  setVolume: (v: number) => void;
  paused: boolean;
  togglePause: () => void;
  listenerCount: number;
  // Received-signal level (0..1), measured PRIOR to the volume control so the
  // meter reflects the incoming broadcast regardless of local volume.
  level: number;
  debugInfo: DebugInfo;
}

const WS_BASE = import.meta.env.VITE_WS_BASE ?? "";

function getWsUrl(sessionId: string, token: string): string {
  if (WS_BASE) {
    return `${WS_BASE}/ws/broadcast/${sessionId}?token=${encodeURIComponent(token)}`;
  }
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws/broadcast/${sessionId}?token=${encodeURIComponent(token)}`;
}

export function useBroadcastListener(
  options: UseBroadcastListenerOptions,
): UseBroadcastListenerReturn {
  const { sessionId, token, info, enabled } = options;
  const [state, setState] = useState<ConnectionState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [volume, setVolumeState] = useState(1);
  const [paused, setPaused] = useState(false);
  const [listenerCount, setListenerCount] = useState(info.listener_count);
  const [debugInfo, setDebugInfo] = useState<DebugInfo>({
    iceState: "new",
    signalingState: "stable",
    candidates: [],
    localSdp: null,
    remoteSdp: null,
  });

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  // The raw received stream, tapped for the pre-volume level meter.
  const [stream, setStream] = useState<MediaStream | null>(null);
  const level = useAudioLevel(stream);

  const setVolume = useCallback((v: number) => {
    setVolumeState(v);
    if (audioRef.current) {
      audioRef.current.volume = v;
    }
  }, []);

  const togglePause = useCallback(() => {
    setPaused((p) => {
      const next = !p;
      if (audioRef.current) {
        if (next) {
          audioRef.current.pause();
        } else {
          void audioRef.current.play();
        }
      }
      return next;
    });
  }, []);

  useEffect(() => {
    if (!enabled) return;

    setState("connecting");
    setError(null);

    const pc = new RTCPeerConnection({
      iceServers: info.ice_servers ?? [{ urls: "stun:stun.l.google.com:19302" }],
    });
    pcRef.current = pc;

    // Receive-only: add transceiver for audio
    pc.addTransceiver("audio", { direction: "recvonly" });

    // Track debug info
    const candidates: string[] = [];

    pc.onicecandidate = (e) => {
      if (e.candidate) {
        candidates.push(e.candidate.candidate);
        setDebugInfo((d) => ({ ...d, candidates: [...candidates] }));
        // Send to signaling server
        wsRef.current?.send(JSON.stringify({ type: "candidate", candidate: e.candidate }));
      }
    };

    pc.oniceconnectionstatechange = () => {
      setDebugInfo((d) => ({ ...d, iceState: pc.iceConnectionState }));
      if (pc.iceConnectionState === "connected" || pc.iceConnectionState === "completed") {
        setState("live");
      } else if (pc.iceConnectionState === "failed" || pc.iceConnectionState === "disconnected") {
        setState("error");
        setError("Connection lost");
      }
    };

    pc.onsignalingstatechange = () => {
      setDebugInfo((d) => ({ ...d, signalingState: pc.signalingState }));
    };

    pc.ontrack = (e) => {
      const stream = e.streams[0] ?? new MediaStream([e.track]);

      // Play through a single HTML audio element. Volume (0–1) is handled by
      // the element's own gain. A previous version ALSO routed the same stream
      // through a Web Audio graph to ctx.destination, which produced two
      // simultaneous playbacks of the same audio — the source of the echo.
      const audio = new Audio();
      audio.srcObject = stream;
      audio.volume = volume;
      audioRef.current = audio;

      // Autoplay is unlocked by the user's Listen-button gesture.
      void audio.play().catch(() => {
        // Will be resumed by user interaction.
      });

      // Expose the raw stream for the pre-volume level meter.
      setStream(stream);
    };

    // WebSocket signaling
    const ws = new WebSocket(getWsUrl(sessionId, token));
    wsRef.current = ws;

    // Remote ICE candidates can arrive (trickled) before the offer has been
    // applied, since the server starts gathering as soon as it sends the offer.
    // addIceCandidate() throws if the remote description isn't set yet, so queue
    // early candidates and flush them once setRemoteDescription completes.
    let remoteDescSet = false;
    const pendingCandidates: RTCIceCandidateInit[] = [];
    const addCandidate = async (init: RTCIceCandidateInit) => {
      try {
        await pc.addIceCandidate(new RTCIceCandidate(init));
      } catch (err) {
        // A single malformed/late candidate must not abort signaling.
        console.warn("[broadcast] skipped ICE candidate:", err);
      }
    };

    ws.onopen = () => {
      // Server will send offer
    };

    ws.onmessage = async (event) => {
      try {
        const msg = JSON.parse(event.data as string) as {
          type: string;
          sdp?: string;
          candidate?: RTCIceCandidateInit;
          listener_count?: number;
        };

        if (msg.type === "offer" && msg.sdp) {
          await pc.setRemoteDescription({ type: "offer", sdp: msg.sdp });
          setDebugInfo((d) => ({ ...d, remoteSdp: msg.sdp ?? null }));

          const answer = await pc.createAnswer();
          await pc.setLocalDescription(answer);
          setDebugInfo((d) => ({ ...d, localSdp: answer.sdp ?? null }));

          ws.send(JSON.stringify({ type: "answer", sdp: answer.sdp }));

          // Flush any candidates that arrived before the offer.
          remoteDescSet = true;
          const queued = pendingCandidates.splice(0);
          for (const c of queued) await addCandidate(c);
        } else if (msg.type === "candidate" && msg.candidate) {
          if (remoteDescSet) {
            await addCandidate(msg.candidate);
          } else {
            pendingCandidates.push(msg.candidate);
          }
        } else if (msg.type === "listener_count" && msg.listener_count != null) {
          setListenerCount(msg.listener_count);
        }
      } catch (err) {
        console.error("[broadcast] signaling error:", err);
      }
    };

    ws.onerror = () => {
      setState("error");
      setError("Connection failed");
    };

    ws.onclose = (e) => {
      if (state !== "error") {
        if (e.code === 4001 || e.code === 4003) {
          setState("error");
          setError("Invalid or expired broadcast link");
        } else {
          setState("closed");
        }
      }
    };

    return () => {
      ws.close();
      pc.close();
      audioRef.current?.pause();
      pcRef.current = null;
      wsRef.current = null;
      audioRef.current = null;
      setStream(null);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, sessionId, token]);

  return {
    state,
    error,
    volume,
    setVolume,
    paused,
    togglePause,
    listenerCount,
    level,
    debugInfo,
  };
}
