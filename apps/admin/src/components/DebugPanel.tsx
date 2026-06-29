import { useState } from "react";
import { cn } from "../lib/utils";

interface DebugEvent {
  id: string;
  timestamp: string;
  type: string;
  message: string;
  data?: unknown;
}

interface DebugPanelProps {
  events: DebugEvent[];
  title?: string;
  className?: string;
}

export function DebugPanel({
  events,
  title = "WebRTC Events",
  className,
}: DebugPanelProps) {
  const [collapsed, setCollapsed] = useState(false);
  const [filter, setFilter] = useState("");

  const filteredEvents = filter
    ? events.filter(
        (e) =>
          e.type.toLowerCase().includes(filter.toLowerCase()) ||
          e.message.toLowerCase().includes(filter.toLowerCase())
      )
    : events;

  return (
    <div className={cn("bg-card border border-border rounded-lg", className)}>
      {/* Header */}
      <div
        className="flex items-center justify-between px-4 py-2 border-b border-border cursor-pointer"
        onClick={() => setCollapsed(!collapsed)}
      >
        <div className="flex items-center gap-2">
          <span className="text-xs">{collapsed ? "▶" : "▼"}</span>
          <h3 className="text-sm font-semibold">{title}</h3>
          <span className="text-xs text-muted-foreground">
            ({events.length} events)
          </span>
        </div>
      </div>

      {!collapsed && (
        <>
          {/* Filter */}
          <div className="px-4 py-2 border-b border-border">
            <input
              type="text"
              placeholder="Filter events..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="w-full bg-muted text-foreground text-xs px-2 py-1 rounded border border-border focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>

          {/* Event list */}
          <div className="max-h-64 overflow-y-auto font-mono text-xs">
            {filteredEvents.map((event) => (
              <div
                key={event.id}
                className="px-4 py-1.5 border-b border-border/50 hover:bg-accent/50"
              >
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">
                    {new Date(event.timestamp).toLocaleTimeString()}
                  </span>
                  <span className="text-primary font-semibold">
                    [{event.type}]
                  </span>
                  <span className="text-foreground">{event.message}</span>
                </div>
                {event.data != null && (
                  <pre className="text-muted-foreground mt-1 pl-4 text-[10px] overflow-hidden text-ellipsis">
                    {String(JSON.stringify(event.data, null, 2)).slice(0, 200)}
                  </pre>
                )}
              </div>
            ))}
            {filteredEvents.length === 0 && (
              <p className="px-4 py-2 text-muted-foreground">No events</p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
