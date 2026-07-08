import { useCallback, useEffect, useRef, useState } from "react";

export interface VUMeterProps {
  // Signal level, 0..1.
  level: number;
  // Optional label rendered to the left of the meter.
  label?: string;
  // Whether to render the numeric percentage on the right. Default true.
  showValue?: boolean;
  className?: string;
}

// Peak colour thresholds (fraction of full scale).
const WARN = 0.6;
const PEAK = 0.85;

function levelColor(level: number): string {
  if (level >= PEAK) return "#ef4444"; // red-500
  if (level >= WARN) return "#eab308"; // yellow-500
  return "#22c55e"; // green-500
}

// VUMeter is a shared, theme-consistent audio level meter used across all SPAs.
// It is styled with inline styles (no Tailwind classes) so it renders
// identically regardless of each app's Tailwind version or content scanning.
// The track/text pick up the shared theme via CSS custom properties.
export function VUMeter({ level, label, showValue = true, className }: VUMeterProps) {
  const clamped = Math.max(0, Math.min(1, level));
  const pct = Math.round(clamped * 100);

  return (
    <div
      className={className}
      style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}
    >
      {label != null && (
        <span
          style={{
            fontSize: 12,
            color: "hsl(var(--muted-foreground))",
            width: 96,
            flexShrink: 0,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {label}
        </span>
      )}
      <div
        style={{
          flex: 1,
          height: 10,
          minWidth: 40,
          borderRadius: 9999,
          background: "hsl(var(--muted))",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            height: "100%",
            width: `${pct}%`,
            background: levelColor(clamped),
            transition: "width 75ms linear",
          }}
        />
      </div>
      {showValue && (
        <span
          style={{
            fontSize: 12,
            color: "hsl(var(--muted-foreground))",
            width: 34,
            textAlign: "right",
            fontVariantNumeric: "tabular-nums",
            flexShrink: 0,
          }}
        >
          {pct}
        </span>
      )}
    </div>
  );
}

// useAudioLevel computes a smoothed RMS level (0..1) from a MediaStream via a
// Web Audio AnalyserNode. It taps the raw stream, so the level is independent
// of any downstream playback gain/mute (it reads the signal PRIOR to volume
// control). Returns 0 when there is no stream.
export function useAudioLevel(stream: MediaStream | null): number {
  const [level, setLevel] = useState(0);
  const rafRef = useRef<number>(0);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const ctxRef = useRef<AudioContext | null>(null);

  const tick = useCallback(() => {
    const analyser = analyserRef.current;
    if (!analyser) {
      setLevel(0);
      return;
    }
    const data = new Uint8Array(analyser.fftSize);
    analyser.getByteTimeDomainData(data);
    let sum = 0;
    for (let i = 0; i < data.length; i++) {
      const v = (data[i]! - 128) / 128;
      sum += v * v;
    }
    const rms = Math.sqrt(sum / data.length);
    setLevel(Math.min(1, rms * 3)); // amplify a little for visibility
    rafRef.current = requestAnimationFrame(tick);
  }, []);

  useEffect(() => {
    if (!stream || stream.getAudioTracks().length === 0) {
      setLevel(0);
      return;
    }
    const AudioCtx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext })
        .webkitAudioContext;
    const ctx = new AudioCtx();
    ctxRef.current = ctx;
    const source = ctx.createMediaStreamSource(stream);
    const analyser = ctx.createAnalyser();
    analyser.fftSize = 256;
    // Note: analyser is intentionally NOT connected to ctx.destination, so this
    // adds no audible output (no double playback / echo) — it only measures.
    source.connect(analyser);
    analyserRef.current = analyser;
    rafRef.current = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(rafRef.current);
      analyserRef.current = null;
      void ctx.close().catch(() => {});
      ctxRef.current = null;
    };
  }, [stream, tick]);

  return level;
}
