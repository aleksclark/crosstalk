import type { Config } from "tailwindcss";
import themePreset from "@crosstalk/theme/preset";

export default {
  presets: [themePreset],
  content: [
    "./index.html",
    "./src/**/*.{ts,tsx}",
    "../../packages/session-audio/src/**/*.{ts,tsx}",
    "../../packages/theme/src/**/*.{ts,tsx}",
  ],
} satisfies Config;
