import { useEffect, useRef, useState, type CSSProperties } from "react";
import {
  IconButton,
  Status,
  VUMeter,
  useAudioLevel,
  type StatusTone,
} from "@crosstalk/theme";
import { useChannelMonitor } from "./useChannelMonitor";

export interface ChannelMonitorProps {
  sessionId: string;
  token: string;
  // Channel name to monitor (the SFU ?listen= selector).
  channelName: string;
  // Base origin for the signaling WebSocket. Defaults to window.location.origin.
  baseUrl?: string;
  className?: string;
  style?: CSSProperties;
}

interface MonitorPreferences {
  muted: boolean;
  volume: number;
}

function loadPreferences(key: string): MonitorPreferences {
  try {
    const saved = JSON.parse(localStorage.getItem(key) ?? "null") as Partial<MonitorPreferences> | null;
    return {
      muted: typeof saved?.muted === "boolean" ? saved.muted : true,
      volume:
        typeof saved?.volume === "number" && Number.isFinite(saved.volume)
          ? Math.min(1, Math.max(0, saved.volume))
          : 1,
    };
  } catch {
    return { muted: true, volume: 1 };
  }
}

function savePreferences(key: string, preferences: MonitorPreferences) {
  try {
    localStorage.setItem(key, JSON.stringify(preferences));
  } catch {
    return;
  }
}

function monitorStatus(state: RTCPeerConnectionState | "idle"): {
  tone: StatusTone;
  label: string;
} {
  switch (state) {
    case "connected":
      return { tone: "ok", label: "Monitoring" };
    case "connecting":
    case "new":
      return { tone: "info", label: "Connecting" };
    case "disconnected":
      return { tone: "warning", label: "Disconnected" };
    case "failed":
      return { tone: "danger", label: "Failed" };
    case "closed":
      return { tone: "neutral", label: "Closed" };
    case "idle":
    default:
      return { tone: "neutral", label: "Idle" };
  }
}

// ChannelMonitor opens an always-on, receive-only monitor for a single channel
// and renders a per-channel listening control: a mute toggle, a volume slider,
// and a VU meter.
//
// The VU meter taps the RAW received stream (via useAudioLevel), so it reflects
// the channel's actual signal regardless of the local monitor mute/volume — a
// muted or silenced channel still shows incoming level. Mute is implemented by
// muting the <audio> element (not pausing), so decoding continues and the meter
// keeps working.
//
// Local preferences default to muted and use storage keys:
//   crosstalk.monitor.v1.<sessionId>.<channelName>
export function ChannelMonitor({
  sessionId,
  token,
  channelName,
  baseUrl,
  className,
  style,
}: ChannelMonitorProps) {
  const { stream, state } = useChannelMonitor({
    sessionId,
    token,
    channel: channelName,
    baseUrl,
  });

  const storageKey = `crosstalk.monitor.v1.${encodeURIComponent(sessionId)}.${encodeURIComponent(channelName)}`;
  const initialPreferences = useRef<MonitorPreferences | null>(null);
  if (!initialPreferences.current) initialPreferences.current = loadPreferences(storageKey);
  const [volume, setVolume] = useState(initialPreferences.current.volume);
  const [muted, setMuted] = useState(initialPreferences.current.muted);
  const [playbackBlocked, setPlaybackBlocked] = useState(false);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  // Level is measured before the volume/mute stage, so it always reflects the
  // incoming signal.
  const level = useAudioLevel(stream);

  useEffect(() => {
    const el = audioRef.current;
    if (!el) return;
    el.srcObject = stream;
    setPlaybackBlocked(false);
    if (stream) {
      void el.play().catch(() => setPlaybackBlocked(true));
    }
  }, [stream]);

  useEffect(() => {
    const resumeAllMonitors = () => {
      const el = audioRef.current;
      if (!el || !el.srcObject || !el.paused) return;
      void el.play().then(() => setPlaybackBlocked(false)).catch(() => setPlaybackBlocked(true));
    };
    window.addEventListener("pointerdown", resumeAllMonitors);
    window.addEventListener("keydown", resumeAllMonitors);
    return () => {
      window.removeEventListener("pointerdown", resumeAllMonitors);
      window.removeEventListener("keydown", resumeAllMonitors);
    };
  }, []);

  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume;
  }, [volume]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.muted = muted;
  }, [muted]);

  const resumePlayback = () => {
    const el = audioRef.current;
    if (!el) return;
    void el.play().then(() => setPlaybackBlocked(false)).catch(() => setPlaybackBlocked(true));
  };

  const { tone, label } = monitorStatus(state);
  const volumePct = Math.round(volume * 100);

  return (
    <div
      className={className}
      style={{
        display: "flex",
        flexWrap: "wrap",
        alignItems: "center",
        gap: "var(--house-space-3)",
        ...style,
      }}
    >
      <audio ref={audioRef} autoPlay className="house-visually-hidden" />

      <IconButton
        icon={playbackBlocked ? "play" : muted ? "mute" : "volume"}
        label={
          playbackBlocked
            ? `Start monitor audio for ${channelName}`
            : muted
              ? `Unmute monitor for ${channelName}`
              : `Mute monitor for ${channelName}`
        }
        variant={
          playbackBlocked ? "primary" : muted ? "destructive" : "secondary"
        }
        aria-pressed={playbackBlocked ? undefined : muted}
        onClick={() => {
          if (playbackBlocked) {
            resumePlayback();
            return;
          }
          setMuted((current) => {
            const next = !current;
            savePreferences(storageKey, { muted: next, volume });
            return next;
          });
        }}
      />

      <label
        style={{
          display: "flex",
          alignItems: "center",
          gap: "var(--house-space-2)",
          minWidth: "8rem",
          flex: "0 1 12rem",
        }}
      >
        <span className="house-visually-hidden">
          Monitor volume for {channelName}
        </span>
        <input
          type="range"
          min={0}
          max={100}
          step={1}
          value={volumePct}
          onChange={(e) => {
            const next = Number(e.target.value) / 100;
            setVolume(next);
            savePreferences(storageKey, { muted, volume: next });
          }}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={volumePct}
          aria-valuetext={`${volumePct} percent`}
          style={{
            width: "100%",
            minHeight: "var(--house-control-height)",
            accentColor: "var(--house-accent)",
            cursor: "pointer",
          }}
        />
      </label>

      <div style={{ minWidth: 0, flex: "1 1 8rem" }}>
        <VUMeter level={level} showValue={false} label={undefined} />
        {playbackBlocked ? (
          <p
            style={{
              margin: "var(--house-space-1) 0 0",
              font: "400 var(--house-type-metadata) / var(--house-leading-metadata) var(--house-font-technical)",
              color: "var(--house-status-warning)",
            }}
          >
            Browser blocked playback. Press play to hear {channelName}.
          </p>
        ) : null}
      </div>

      <Status tone={tone}>{label}</Status>
    </div>
  );
}
