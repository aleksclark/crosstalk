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

  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  // Level is measured before the volume/mute stage, so it always reflects the
  // incoming signal.
  const level = useAudioLevel(stream);

  useEffect(() => {
    const el = audioRef.current;
    if (!el) return;
    el.srcObject = stream;
    if (stream) void el.play().catch(() => {});
  }, [stream]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume;
  }, [volume]);

  useEffect(() => {
    if (audioRef.current) audioRef.current.muted = muted;
  }, [muted]);

  const connected = state === "connected";

  return (
    <div className="flex items-center gap-3">
      <audio ref={audioRef} autoPlay className="hidden" />
      <button
        onClick={() => setMuted((m) => !m)}
        className={`flex h-8 w-8 shrink-0 items-center justify-center rounded text-xs font-bold transition-colors ${
          muted
            ? "border border-destructive/50 bg-destructive/20 text-destructive-foreground"
            : "border border-primary/50 bg-primary/20 text-primary-foreground"
        }`}
        title={muted ? "Unmute monitor" : "Mute monitor"}
        aria-pressed={muted}
      >
        {muted ? "M" : "🔊"}
      </button>

      <input
        type="range"
        min={0}
        max={100}
        step={1}
        value={Math.round(volume * 100)}
        onChange={(e) => setVolume(Number(e.target.value) / 100)}
        className="h-1 w-24 shrink-0 cursor-pointer accent-primary"
        title="Monitor volume"
        aria-label="Monitor volume"
      />

      <div className="min-w-0 flex-1">
        <VUMeter level={level} showValue={false} />
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
