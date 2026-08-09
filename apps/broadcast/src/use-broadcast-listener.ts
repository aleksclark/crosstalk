import { useCallback, useEffect, useRef, useState } from "react";
import { useAudioLevel } from "@crosstalk/theme";
import type { BroadcastInfo } from "./api";
import {
  canAttemptReconnect,
  classifyWsClose,
  computeBackoffMs,
  DEFAULT_RECONNECT_POLICY,
  nextAttemptAfterStable,
  terminalMessage,
  type ListenerConnectionState,
  type ReconnectPolicyOptions,
  type TerminalReason,
} from "./reconnect-policy";

/** @deprecated Prefer ListenerConnectionState — kept for existing imports. */
export type ConnectionState = ListenerConnectionState;
export type { ListenerConnectionState, TerminalReason };

export interface DebugInfo {
  iceState: string;
  signalingState: string;
  candidates: string[];
  localSdp: string | null;
  remoteSdp: string | null;
}

export interface UseBroadcastListenerOptions {
  sessionId: string;
  /**
   * Long-lived broadcast URL token from the listen link.
   * Reused across reconnects — it is NOT a one-shot media ticket.
   * Each reconnect opens a fresh WS; the server mints a new PeerConn.
   */
  token: string;
  info: BroadcastInfo;
  /** Only connect after a user gesture (Listen button). */
  enabled: boolean;
  /** Optional overrides (tests). */
  reconnectPolicy?: Partial<ReconnectPolicyOptions>;
  /** Optional clock (tests). */
  now?: () => number;
  /** Optional random in [0,1) for backoff jitter (tests). */
  random?: () => number;
}

export interface UseBroadcastListenerReturn {
  state: ListenerConnectionState;
  error: string | null;
  /** Structured reason when state is ended/error. */
  terminalReason: TerminalReason | null;
  /** 1-based reconnect attempt currently in flight, else 0. */
  reconnectAttempt: number;
  /** Max reconnect attempts before giving up. */
  maxReconnectAttempts: number;
  volume: number;
  setVolume: (v: number) => void;
  paused: boolean;
  togglePause: () => void;
  /**
   * Live listener count from the server, or null when the server never
   * supplied a real value (do not invent one in the UI).
   */
  listenerCount: number | null;
  // Received-signal level (0..1), measured PRIOR to the volume control so the
  // meter reflects the incoming broadcast regardless of local volume.
  level: number;
  debugInfo: DebugInfo;
  /** Manual retry after a terminal error (not needed for ended sessions). */
  retry: () => void;
}

const WS_BASE = import.meta.env.VITE_WS_BASE ?? "";

/**
 * Build the public listener WebSocket URL.
 * Contract today: `/ws/broadcast/{sessionId}?token={broadcastToken}`.
 * Token is the URL credential; each connect is a fresh media session on the
 * server — we never replay a consumed one-shot media ticket.
 */
export function getWsUrl(sessionId: string, token: string): string {
  if (WS_BASE) {
    return `${WS_BASE}/ws/broadcast/${sessionId}?token=${encodeURIComponent(token)}`;
  }
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws/broadcast/${sessionId}?token=${encodeURIComponent(token)}`;
}

const emptyDebug = (): DebugInfo => ({
  iceState: "new",
  signalingState: "stable",
  candidates: [],
  localSdp: null,
  remoteSdp: null,
});

export function useBroadcastListener(
  options: UseBroadcastListenerOptions,
): UseBroadcastListenerReturn {
  const {
    sessionId,
    token,
    info,
    enabled,
    reconnectPolicy: policyOverrides,
    now: nowFn,
    random: randomFn,
  } = options;

  const policy: ReconnectPolicyOptions = {
    ...DEFAULT_RECONNECT_POLICY,
    ...policyOverrides,
  };
  const policyRef = useRef(policy);
  policyRef.current = policy;

  const clockRef = useRef(nowFn ?? (() => Date.now()));
  clockRef.current = nowFn ?? (() => Date.now());
  const randRef = useRef(randomFn ?? Math.random);
  randRef.current = randomFn ?? Math.random;

  const [state, setState] = useState<ListenerConnectionState>("idle");
  const [error, setError] = useState<string | null>(null);
  const [terminalReason, setTerminalReason] = useState<TerminalReason | null>(null);
  const [reconnectAttempt, setReconnectAttempt] = useState(0);
  const [volume, setVolumeState] = useState(1);
  const [paused, setPaused] = useState(false);
  // Only show a count once the server has actually sent one.
  const [listenerCount, setListenerCount] = useState<number | null>(
    typeof info.listener_count === "number" ? info.listener_count : null,
  );
  const [debugInfo, setDebugInfo] = useState<DebugInfo>(emptyDebug);
  const [retryNonce, setRetryNonce] = useState(0);

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [stream, setStream] = useState<MediaStream | null>(null);
  const level = useAudioLevel(stream);

  const volumeRef = useRef(volume);
  volumeRef.current = volume;
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  const attemptRef = useRef(0);
  const playingSinceRef = useRef<number | null>(null);
  const disposedRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const generationRef = useRef(0);
  // Once we've reached a terminal state, ignore late close/error events.
  const terminalRef = useRef(false);
  // Prevent ice-failed + ws-close (or repeated ICE events) from stacking attempts.
  const reconnectScheduledRef = useRef(false);

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
          void audioRef.current.play().catch(() => {
            /* gesture may be required again in some browsers */
          });
        }
      }
      return next;
    });
  }, []);

  const retry = useCallback(() => {
    terminalRef.current = false;
    reconnectScheduledRef.current = false;
    attemptRef.current = 0;
    playingSinceRef.current = null;
    setTerminalReason(null);
    setError(null);
    setReconnectAttempt(0);
    setState("connecting");
    setRetryNonce((n) => n + 1);
  }, []);

  const tearDownMedia = useCallback(() => {
    if (reconnectTimerRef.current != null) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    try {
      wsRef.current?.close();
    } catch {
      /* ignore */
    }
    try {
      pcRef.current?.close();
    } catch {
      /* ignore */
    }
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.srcObject = null;
    }
    wsRef.current = null;
    pcRef.current = null;
    audioRef.current = null;
    setStream(null);
  }, []);

  const markTerminal = useCallback(
    (reason: TerminalReason, uiState: "ended" | "error" = reason === "session_ended" ? "ended" : "error") => {
      if (terminalRef.current || disposedRef.current) return;
      terminalRef.current = true;
      reconnectScheduledRef.current = false;
      if (reconnectTimerRef.current != null) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      setTerminalReason(reason);
      setError(terminalMessage(reason));
      setState(uiState);
      setReconnectAttempt(0);
      // Bump generation so in-flight connect callbacks no-op.
      generationRef.current += 1;
      tearDownMedia();
    },
    [tearDownMedia],
  );

  const scheduleReconnect = useCallback(
    (trigger: string) => {
      if (disposedRef.current || terminalRef.current || reconnectScheduledRef.current) {
        return;
      }
      reconnectScheduledRef.current = true;

      const policyNow = policyRef.current;
      const clock = clockRef.current;
      const rand = randRef.current;

      // Reset attempt budget after stable play.
      if (playingSinceRef.current != null) {
        const played = clock() - playingSinceRef.current;
        attemptRef.current = nextAttemptAfterStable(played, attemptRef.current, policyNow);
        playingSinceRef.current = null;
      }

      const nextAttempt = attemptRef.current + 1;
      if (!canAttemptReconnect(nextAttempt, policyNow)) {
        console.warn("[broadcast] max reconnect attempts reached", { trigger });
        markTerminal("max_retries", "error");
        return;
      }

      attemptRef.current = nextAttempt;
      const delay = computeBackoffMs(nextAttempt, policyNow, rand);

      setState("reconnecting");
      setError(
        `Connection interrupted — reconnecting (attempt ${nextAttempt}/${policyNow.maxAttempts})…`,
      );
      setTerminalReason(null);
      setReconnectAttempt(nextAttempt);
      setDebugInfo(emptyDebug);

      console.info("[broadcast] scheduling reconnect", {
        trigger,
        attempt: nextAttempt,
        delayMs: delay,
      });

      // Advance generation so any lingering handlers from the old peer ignore events
      // (including the ws.onclose that tearDownMedia will trigger).
      const scheduledGen = generationRef.current + 1;
      generationRef.current = scheduledGen;

      // Tear down current media before waiting; next connect is a fresh WS.
      tearDownMedia();

      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        reconnectScheduledRef.current = false;
        if (disposedRef.current || terminalRef.current) return;
        if (generationRef.current !== scheduledGen) return;
        // Kick a new connect cycle by bumping retry nonce without clearing attempts.
        setRetryNonce((n) => n + 1);
      }, delay);
    },
    [markTerminal, tearDownMedia],
  );

  useEffect(() => {
    if (!enabled) {
      disposedRef.current = true;
      tearDownMedia();
      disposedRef.current = false;
      terminalRef.current = false;
      attemptRef.current = 0;
      playingSinceRef.current = null;
      setState("idle");
      setError(null);
      setTerminalReason(null);
      setReconnectAttempt(0);
      return;
    }

    if (terminalRef.current) {
      // Stay in terminal state until retry() clears it.
      return;
    }

    disposedRef.current = false;
    reconnectScheduledRef.current = false;
    const gen = generationRef.current + 1;
    generationRef.current = gen;

    const isCurrent = () =>
      !disposedRef.current && !terminalRef.current && generationRef.current === gen;

    setState(attemptRef.current > 0 ? "reconnecting" : "connecting");
    if (attemptRef.current === 0) {
      setError(null);
      setTerminalReason(null);
    }
    setDebugInfo(emptyDebug);

    const iceServers =
      info.ice_servers && info.ice_servers.length > 0
        ? info.ice_servers
        : [{ urls: "stun:stun.l.google.com:19302" }];

    const pc = new RTCPeerConnection({ iceServers });
    pcRef.current = pc;

    // Receive-only: add transceiver for audio
    pc.addTransceiver("audio", { direction: "recvonly" });

    const candidates: string[] = [];

    pc.onicecandidate = (e) => {
      if (!isCurrent()) return;
      if (e.candidate) {
        candidates.push(e.candidate.candidate);
        setDebugInfo((d) => ({ ...d, candidates: [...candidates] }));
        if (wsRef.current?.readyState === WebSocket.OPEN) {
          wsRef.current.send(
            JSON.stringify({ type: "candidate", candidate: e.candidate }),
          );
        }
      }
    };

    pc.oniceconnectionstatechange = () => {
      if (!isCurrent()) return;
      setDebugInfo((d) => ({ ...d, iceState: pc.iceConnectionState }));
      if (
        pc.iceConnectionState === "connected" ||
        pc.iceConnectionState === "completed"
      ) {
        playingSinceRef.current = clockRef.current();
        // Successful play resets the reconnect budget after stable duration
        // is handled on the next drop; here we just mark playing.
        setState("playing");
        setError(null);
        setTerminalReason(null);
        setReconnectAttempt(0);
      } else if (pc.iceConnectionState === "failed") {
        // Hard ICE failure — full reconnect with a fresh WS/PeerConn.
        // "disconnected" is often transient; wait for failed or WS close.
        scheduleReconnect("ice-failed");
      }
    };

    pc.onsignalingstatechange = () => {
      if (!isCurrent()) return;
      setDebugInfo((d) => ({ ...d, signalingState: pc.signalingState }));
    };

    pc.ontrack = (e) => {
      if (!isCurrent()) return;
      const mediaStream = e.streams[0] ?? new MediaStream([e.track]);

      // Play through a single HTML audio element. Volume (0–1) is handled by
      // the element's own gain. Do NOT also route through Web Audio destination
      // or we get double playback (echo).
      const audio = new Audio();
      audio.srcObject = mediaStream;
      audio.volume = volumeRef.current;
      audioRef.current = audio;

      const onAudioError = () => {
        if (!isCurrent()) return;
        markTerminal("playback_failed", "error");
      };
      audio.addEventListener("error", onAudioError);

      // Autoplay is unlocked by the user's Listen-button gesture. On reconnect
      // within the same page lifetime the gesture usually still holds; if not,
      // play() rejects and the user can hit Resume.
      if (!pausedRef.current) {
        void audio.play().catch(() => {
          // Will be resumed by user interaction.
        });
      }

      setStream(mediaStream);
      // Track arrival is a strong signal we're live even before ICE "connected"
      // in some browsers.
      if (playingSinceRef.current == null) {
        playingSinceRef.current = clockRef.current();
      }
      setState("playing");
      setError(null);
      setReconnectAttempt(0);
    };

    // Fresh WebSocket each connect — never reuse a closed socket, never
    // replay a one-shot media ticket (token is the durable URL credential).
    const wsUrl = getWsUrl(sessionId, token);
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    // Remote ICE candidates can arrive (trickled) before the offer has been
    // applied. Queue them until setRemoteDescription completes.
    let remoteDescSet = false;
    const pendingCandidates: RTCIceCandidateInit[] = [];
    const addCandidate = async (init: RTCIceCandidateInit) => {
      try {
        await pc.addIceCandidate(new RTCIceCandidate(init));
      } catch (err) {
        console.warn("[broadcast] skipped ICE candidate:", err);
      }
    };

    ws.addEventListener("open", () => {
      // Server sends the offer for receive-only listeners.
    });

    ws.addEventListener("message", async (event) => {
      if (!isCurrent()) return;
      try {
        const msg = JSON.parse(String(event.data)) as {
          type: string;
          sdp?: string;
          candidate?: RTCIceCandidateInit;
          listener_count?: number;
          reason?: string;
          error?: string;
          code?: string;
        };

        if (msg.type === "error" || msg.type === "session_ended" || msg.type === "ended") {
          const code = (msg.code ?? msg.reason ?? msg.error ?? msg.type).toLowerCase();
          if (
            code.includes("invalid") ||
            code.includes("expired") ||
            code.includes("forbidden") ||
            code.includes("unauthorized") ||
            code.includes("rotated")
          ) {
            markTerminal("invalid_token", "error");
            return;
          }
          markTerminal("session_ended", "ended");
          return;
        }

        if (msg.type === "offer" && msg.sdp) {
          await pc.setRemoteDescription({ type: "offer", sdp: msg.sdp });
          if (!isCurrent()) return;
          setDebugInfo((d) => ({ ...d, remoteSdp: msg.sdp ?? null }));

          const answer = await pc.createAnswer();
          await pc.setLocalDescription(answer);
          if (!isCurrent()) return;
          setDebugInfo((d) => ({ ...d, localSdp: answer.sdp ?? null }));

          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: "answer", sdp: answer.sdp }));
          }

          remoteDescSet = true;
          const queued = pendingCandidates.splice(0);
          for (const c of queued) await addCandidate(c);
        } else if (msg.type === "candidate" && msg.candidate) {
          if (remoteDescSet) {
            await addCandidate(msg.candidate);
          } else {
            pendingCandidates.push(msg.candidate);
          }
        } else if (msg.type === "listener_count" && typeof msg.listener_count === "number") {
          setListenerCount(msg.listener_count);
        }
      } catch (err) {
        console.error("[broadcast] signaling error:", err);
      }
    });

    ws.addEventListener("error", () => {
      // close will fire next with a concrete code; avoid double-handling.
      if (!isCurrent()) return;
    });

    ws.addEventListener("close", (e) => {
      if (!isCurrent()) return;

      const classification = classifyWsClose(e.code, e.reason ?? "");
      if (classification.kind === "invalid_token") {
        markTerminal("invalid_token", "error");
        return;
      }
      if (classification.kind === "session_ended" || classification.kind === "ended") {
        // Bare 1000 after we were never playing may be a rejected upgrade —
        // treat first-connect clean close with empty reason as connection_failed
        // only if we never reached playing; otherwise session ended.
        if (
          classification.kind === "ended" &&
          playingSinceRef.current == null &&
          attemptRef.current === 0
        ) {
          // Could be invalid token that the server closed cleanly.
          markTerminal("connection_failed", "error");
          return;
        }
        markTerminal("session_ended", "ended");
        return;
      }

      scheduleReconnect(`ws-close-${e.code}`);
    });

    return () => {
      disposedRef.current = true;
      if (reconnectTimerRef.current != null) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      try {
        ws.close();
      } catch {
        /* ignore */
      }
      try {
        pc.close();
      } catch {
        /* ignore */
      }
      if (audioRef.current) {
        audioRef.current.pause();
        audioRef.current.srcObject = null;
      }
      pcRef.current = null;
      wsRef.current = null;
      audioRef.current = null;
      setStream(null);
    };
    // info.ice_servers intentionally omitted — token/session/enabled drive reconnect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, sessionId, token, retryNonce]);

  return {
    state,
    error,
    terminalReason,
    reconnectAttempt,
    maxReconnectAttempts: policy.maxAttempts,
    volume,
    setVolume,
    paused,
    togglePause,
    listenerCount,
    level,
    debugInfo,
    retry,
  };
}
