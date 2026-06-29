interface VUMeterProps {
  label: string;
  level: number; // 0 to 1
}

export function VUMeter({ label, level }: VUMeterProps) {
  const percentage = Math.round(level * 100);
  const barColor =
    level > 0.8
      ? "bg-red-500"
      : level > 0.5
        ? "bg-yellow-500"
        : "bg-green-500";

  return (
    <div className="flex items-center gap-3">
      <span className="text-xs text-gray-400 w-28 shrink-0">{label}</span>
      <div className="flex-1 h-3 bg-gray-900 rounded overflow-hidden">
        <div
          className={`h-full transition-all duration-75 ${barColor}`}
          style={{ width: `${percentage}%` }}
        />
      </div>
      <span className="text-xs text-gray-500 w-8 text-right">{percentage}%</span>
    </div>
  );
}
