import type { SVGProps } from "react";

/**
 * Editorial Instrument outline icons — 24px grid, 1.75 stroke, round caps/joins.
 * Paths mapped one-to-one from house-icons.svg semantics (plus audio/volume/mute/close).
 */

export type IconName =
  | "menu"
  | "dashboard"
  | "monitor"
  | "sessions"
  | "operate"
  | "audio"
  | "translators"
  | "language"
  | "debug"
  | "inspect"
  | "play"
  | "pause"
  | "volume"
  | "mute"
  | "check"
  | "alert"
  | "error"
  | "info"
  | "copy"
  | "refresh"
  | "trash"
  | "search"
  | "filter"
  | "user"
  | "chevron-down"
  | "chevron-right"
  | "chevron-left"
  | "chevron-up"
  | "arrow-left"
  | "arrow-right"
  | "close"
  | "external"
  | "save"
  | "clock"
  | "record"
  | "settings"
  | "configure"
  | "grid"
  | "panel"
  | "access"
  | "log";

export type IconSize = "compact" | "default" | "emphasis" | "grid";

const SIZE_PX: Record<IconSize, number> = {
  compact: 14,
  default: 16,
  emphasis: 18,
  grid: 24,
};

/** Path data for each symbol (viewBox 0 0 24 24). */
const PATHS: Record<IconName, string> = {
  menu: "M4 6h16M4 12h16M4 18h16",
  dashboard: "M4 18V5h16v13zM7 14l3-3 3 2 4-5",
  monitor: "M4 18V5h16v13zM7 14l3-3 3 2 4-5",
  sessions: "M5 4h14v16H5zM8 8h8M8 12h5M8 16h7M17 12l2 2-2 2",
  operate: "M5 4h14v16H5zM8 8h8M8 12h5M8 16h7M17 12l2 2-2 2",
  audio:
    "M4 10v4h3l4 3V7L7 10H4zM15.5 8.5a4 4 0 0 1 0 7M17.5 6a7 7 0 0 1 0 12",
  translators:
    "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18",
  language:
    "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18",
  debug:
    "M6 3h9l4 4v14H6zM14 3v5h5M11 14a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM13.2 16.2l2.3 2.3",
  inspect:
    "M6 3h9l4 4v14H6zM14 3v5h5M11 14a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM13.2 16.2l2.3 2.3",
  play: "M8 5l11 7-11 7z",
  pause: "M8 5v14M16 5v14",
  volume: "M4 10v4h3l4 3V7L7 10H4zM15.5 8.5a4 4 0 0 1 0 7",
  mute: "M4 10v4h3l4 3V7L7 10H4zM16 9l5 6M21 9l-5 6",
  check: "M5 12l4 4L19 6",
  alert: "M12 3L22 20H2zM12 9v5M12 17.5v.5",
  error: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM9 9l6 6M15 9l-6 6",
  info: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v6M12 7.5v.5",
  copy: "M8 8h12v12H8zM16 8V4H4v12h4",
  refresh:
    "M20 7v5h-5M4 17v-5h5M6.5 8a7 7 0 0 1 12 1l1.5 3M17.5 16a7 7 0 0 1-12-1L4 12",
  trash: "M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14M10 11v6M14 11v6",
  search: "M10.5 4a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM16 16l5 5",
  filter: "M4 5h16l-6 7v6l-4 2v-8z",
  user: "M9 5a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM3 20c.5-4 2.5-6 6-6s5.5 2 6 6M17 8h4M19 6v4",
  "chevron-down": "M6 9l6 6 6-6",
  "chevron-right": "M9 6l6 6-6 6",
  "chevron-left": "M15 6l-6 6 6 6",
  "chevron-up": "M6 15l6-6 6 6",
  "arrow-left": "M20 12H4M10 6l-6 6 6 6",
  "arrow-right": "M4 12h16M14 6l6 6-6 6",
  close: "M6 6l12 12M18 6L6 18",
  external: "M10 5H5v14h14v-5M13 3h8v8M21 3L11 13",
  save: "M5 3h12l2 2v16H5zM8 3v6h8V3M8 21v-7h8v7",
  clock: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2",
  record: "M6 3h9l4 4v14H6zM14 3v5h5M9 12h6M9 16h6",
  settings:
    "M4 7h10M18 7h2M4 17h2M10 17h10M4 12h4M12 12h8M16 5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM8 15a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM10 10a2 2 0 1 1 0 4 2 2 0 0 1 0-4z",
  configure:
    "M4 7h10M18 7h2M4 17h2M10 17h10M4 12h4M12 12h8M16 5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM8 15a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM10 10a2 2 0 1 1 0 4 2 2 0 0 1 0-4z",
  grid: "M4 4h16v16H4zM4 10h16M4 16h16M10 4v16M16 4v16",
  panel: "M3 4h18v16H3zM15 4v16M6 8h6M6 12h6",
  access: "M12 3L20 6v6c0 5-3.2 8-8 9-4.8-1-8-4-8-9V6zM9 12l2 2 4-5",
  log: "M5 3h14v18H5zM8 8h8M8 12h8M8 16h5",
};

export interface IconProps extends Omit<SVGProps<SVGSVGElement>, "name"> {
  name: IconName;
  /** compact=14, default=16, emphasis=18, grid=24 */
  size?: IconSize | number;
  title?: string;
}

export function Icon({
  name,
  size = "default",
  title,
  className,
  style,
  ...rest
}: IconProps) {
  const px = typeof size === "number" ? size : SIZE_PX[size];
  const d = PATHS[name];
  const labelled = Boolean(title) || rest["aria-label"] != null;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      width={px}
      height={px}
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={["house-icon", className].filter(Boolean).join(" ")}
      style={style}
      aria-hidden={labelled ? undefined : true}
      role={labelled ? "img" : undefined}
      {...rest}
    >
      {title ? <title>{title}</title> : null}
      <path d={d} />
    </svg>
  );
}

export const iconNames = Object.keys(PATHS) as IconName[];
