import { cn } from "../lib/utils";

interface VUMeterProps {
  level: number; // 0-100
  className?: string;
}

export function VUMeter({ level, className }: VUMeterProps) {
  const clampedLevel = Math.max(0, Math.min(100, level));
  const isHigh = clampedLevel > 80;
  const isMedium = clampedLevel > 50;

  return (
    <div className={cn("flex items-center gap-1 h-4", className)}>
      <div className="flex-1 h-full bg-muted rounded-sm overflow-hidden">
        <div
          className={cn(
            "h-full transition-all duration-75 rounded-sm",
            isHigh
              ? "bg-red-500"
              : isMedium
                ? "bg-yellow-500"
                : "bg-green-500"
          )}
          style={{ width: `${clampedLevel}%` }}
        />
      </div>
      <span className="text-xs text-muted-foreground w-8 text-right tabular-nums">
        {Math.round(clampedLevel)}
      </span>
    </div>
  );
}
