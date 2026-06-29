import { useState } from "react";
import { VUMeter } from "./VUMeter";

interface AudioSource {
  id: string;
  name: string;
  level: number;
  muted: boolean;
  volume: number;
}

interface MixPanelProps {
  channelName: string;
  sources: AudioSource[];
  onMuteToggle?: (sourceId: string, muted: boolean) => void;
  onVolumeChange?: (sourceId: string, volume: number) => void;
}

export function MixPanel({
  channelName,
  sources,
  onMuteToggle,
  onVolumeChange,
}: MixPanelProps) {
  const [localSources, setLocalSources] = useState(sources);

  const handleMuteToggle = (sourceId: string) => {
    setLocalSources((prev) =>
      prev.map((s) =>
        s.id === sourceId ? { ...s, muted: !s.muted } : s
      )
    );
    const source = localSources.find((s) => s.id === sourceId);
    if (source) {
      onMuteToggle?.(sourceId, !source.muted);
    }
  };

  const handleVolumeChange = (sourceId: string, volume: number) => {
    setLocalSources((prev) =>
      prev.map((s) =>
        s.id === sourceId ? { ...s, volume } : s
      )
    );
    onVolumeChange?.(sourceId, volume);
  };

  return (
    <div className="bg-card border border-border rounded-lg p-4">
      <h3 className="text-sm font-semibold mb-3">{channelName}</h3>
      <div className="space-y-3">
        {localSources.map((source) => (
          <div key={source.id} className="flex items-center gap-3">
            {/* Mute button */}
            <button
              onClick={() => handleMuteToggle(source.id)}
              className={`w-8 h-8 rounded text-xs font-bold flex items-center justify-center transition-colors ${
                source.muted
                  ? "bg-red-500/20 text-red-400 border border-red-500/50"
                  : "bg-green-500/20 text-green-400 border border-green-500/50"
              }`}
            >
              {source.muted ? "M" : "🔊"}
            </button>

            {/* Source name */}
            <span className="text-xs text-foreground w-20 truncate">
              {source.name}
            </span>

            {/* Volume slider */}
            <input
              type="range"
              min={0}
              max={100}
              value={source.volume}
              onChange={(e) =>
                handleVolumeChange(source.id, Number(e.target.value))
              }
              className="flex-1 h-1 accent-primary cursor-pointer"
            />

            {/* VU meter */}
            <div className="w-24">
              <VUMeter level={source.muted ? 0 : source.level} />
            </div>
          </div>
        ))}
        {localSources.length === 0 && (
          <p className="text-xs text-muted-foreground">No audio sources</p>
        )}
      </div>
    </div>
  );
}
