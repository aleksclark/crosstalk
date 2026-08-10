import { useEffect, useRef, useState } from "react";
import { VUMeter, useAudioLevel } from "@crosstalk/theme";
import { useChannelMonitor } from "./useChannelMonitor";

export interface ChannelMonitorProps {
  sessionId: string;
  token: string;
  // Channel name to monitor (the SFU ?listen= selector).
  channelName: string;
  // Base origin for the signaling WebSocket. Defaults to window.location.origin.
  baseUrl?: string;
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

// ChannelMonitor opens an always-on, receive-only monitor for a single channel
// and renders a per-channel listening control: a mute toggle, a volume slider,
// and a VU meter.
//
// The VU meter taps the RAW received stream (via useAudioLevel), so it reflects
// the channel's actual signal regardless of the local monitor mute/volume — a
// muted or silenced channel still shows incoming level. Mute is implemented by
// muting the <audio> element (not pausing), so decoding continues and the meter
// keeps working.
export function ChannelMonitor({
  sessionId,
  token,
  channelName,
  baseUrl,
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

  const connected = state === "connected";

  const resumePlayback = () => {
    const el = audioRef.current;
    if (!el) return;
    void el.play().then(() => setPlaybackBlocked(false)).catch(() => setPlaybackBlocked(true));
  };

  return (
    <div className="flex items-center gap-3">
      <audio ref={audioRef} autoPlay className="hidden" />
      <button
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
        className={`flex h-8 w-8 shrink-0 items-center justify-center rounded text-xs font-bold transition-colors ${
          playbackBlocked
            ? "border border-yellow-500/50 bg-yellow-500/20 text-yellow-200"
            : muted
              ? "border border-destructive/50 bg-destructive/20 text-destructive-foreground"
              : "border border-primary/50 bg-primary/20 text-primary-foreground"
        }`}
        title={playbackBlocked ? "Start monitor audio" : muted ? "Unmute monitor" : "Mute monitor"}
        aria-pressed={muted}
      >
        {playbackBlocked ? "▶" : muted ? "M" : "A"}
      </button>

      <input
        type="range"
        min={0}
        max={100}
        step={1}
        value={Math.round(volume * 100)}
        onChange={(e) => {
          const next = Number(e.target.value) / 100;
          setVolume(next);
          savePreferences(storageKey, { muted, volume: next });
        }}
        className="h-1 w-24 shrink-0 cursor-pointer accent-primary"
        title="Monitor volume"
        aria-label="Monitor volume"
      />

      <div className="min-w-0 flex-1">
        <VUMeter level={level} showValue={false} />
        {playbackBlocked && (
          <span className="text-[10px] text-yellow-300">Click ▶ to hear this channel</span>
        )}
      </div>

      <span
        className="w-2 shrink-0"
        title={connected ? "Monitoring" : `Monitor: ${state}`}
      >
        <span
          className={`block h-2 w-2 rounded-full ${
            connected ? "bg-green-500" : "bg-muted-foreground"
          }`}
        />
      </span>
    </div>
  );
}
