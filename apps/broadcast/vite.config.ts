import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/broadcast/",
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
    sourcemap: true,
  },
  server: {
    port: 5174,
    proxy: {
      "/api": {
        target: process.env.CT_API_PROXY || "http://localhost:8080",
        changeOrigin: true,
      },
      // The broadcast listener's receive-only WebRTC signaling runs over
      // /ws/broadcast/{id}; ws:true upgrades and forwards it.
      "/ws": {
        target: process.env.CT_API_PROXY || "http://localhost:8080",
        ws: true,
        changeOrigin: true,
      },
    },
  },
});
