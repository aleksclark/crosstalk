import themePreset from "@crosstalk/theme/preset";

/** @type {import('tailwindcss').Config} */
export default {
  presets: [themePreset],
  content: [
    "./index.html",
    "./src/**/*.{ts,tsx}",
    "../../packages/session-audio/src/**/*.{ts,tsx}",
    "../../packages/theme/src/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
};
