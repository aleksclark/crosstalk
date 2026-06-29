import { useCallback, useEffect, useRef, useState } from "react";
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
  const gainRef = useRef<GainNode | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);

  const setVolume = useCallback((v: number) => {
    setVolumeState(v);
    if (gainRef.current) {
      gainRef.current.gain.value = v;
    }
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

      // Create audio element for playback
      const audio = new Audio();
      audio.srcObject = stream;
      audio.volume = volume;
      audioRef.current = audio;

      // iOS: resume AudioContext on user gesture (already handled by Listen button)
      void audio.play().catch(() => {
        // Will be resumed by user interaction
      });

      // Try AudioContext for gain control
      try {
        const ctx = new AudioContext();
        audioCtxRef.current = ctx;
        const source = ctx.createMediaStreamSource(stream);
        const gain = ctx.createGain();
        gain.gain.value = volume;
        gainRef.current = gain;
        source.connect(gain);
        gain.connect(ctx.destination);

        // iOS AudioContext resume
        if (ctx.state === "suspended") {
          void ctx.resume();
        }
      } catch {
        // Fallback to HTML Audio element (already set up)
      }
    };

    // WebSocket signaling
    const ws = new WebSocket(getWsUrl(sessionId, token));
    wsRef.current = ws;

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
        } else if (msg.type === "candidate" && msg.candidate) {
          await pc.addIceCandidate(new RTCIceCandidate(msg.candidate));
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
      audioCtxRef.current?.close().catch(() => {});
      pcRef.current = null;
      wsRef.current = null;
      audioRef.current = null;
      gainRef.current = null;
      audioCtxRef.current = null;
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
    debugInfo,
  };
}
