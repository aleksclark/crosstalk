import { useState } from "react";
import { Icon } from "@crosstalk/theme";
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
          e.message.toLowerCase().includes(filter.toLowerCase()),
      )
    : events;

  return (
    <div className={cn("border border-border bg-[var(--house-bg-raised)]", className)}>
      <button
        type="button"
        className="flex w-full items-center justify-between gap-2 border-b border-border px-3 py-2 text-left outline-none focus-visible:ring-2 focus-visible:ring-[var(--house-focus)]"
        onClick={() => setCollapsed(!collapsed)}
        aria-expanded={!collapsed}
      >
        <div className="flex min-w-0 items-center gap-2">
          <Icon
            name={collapsed ? "chevron-right" : "chevron-down"}
            size="compact"
            aria-hidden
          />
          <h3 className="house-type-label truncate">{title}</h3>
          <span className="house-type-meta text-muted-foreground">
            ({events.length})
          </span>
        </div>
      </button>

      {!collapsed ? (
        <>
          <div className="border-b border-border px-3 py-2">
            <label className="house-visually-hidden" htmlFor="debug-event-filter">
              Filter events
            </label>
            <input
              id="debug-event-filter"
              type="text"
              placeholder="Filter events..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="w-full rounded-[var(--house-radius-md)] border border-border bg-[var(--house-bg-sunken)] px-2 py-1 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-[var(--house-focus)]"
            />
          </div>

          <div className="max-h-64 overflow-y-auto house-type-code">
            {filteredEvents.map((event) => (
              <div
                key={event.id}
                className="border-b border-border/50 px-3 py-1.5 last:border-b-0 hover:bg-[var(--house-bg-surface)]"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">
                    {new Date(event.timestamp).toLocaleTimeString()}
                  </span>
                  <span className="font-semibold text-primary">[{event.type}]</span>
                  <span className="text-foreground">{event.message}</span>
                </div>
                {event.data != null ? (
                  <pre className="mt-1 overflow-hidden text-ellipsis pl-2 text-[10px] text-muted-foreground">
                    {String(JSON.stringify(event.data, null, 2)).slice(0, 200)}
                  </pre>
                ) : null}
              </div>
            ))}
            {filteredEvents.length === 0 ? (
              <p className="px-3 py-2 text-muted-foreground">No events</p>
            ) : null}
          </div>
        </>
      ) : null}
    </div>
  );
}
