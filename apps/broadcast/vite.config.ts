import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/broadcast/",
  plugins: [react(), tailwindcss()],
  // Ensure a single React instance across the app and workspace packages
  // (@crosstalk/theme). Without this a second React copy is bundled and its
  // hooks run with a null dispatcher ("Cannot read properties of null
  // (reading 'useState')").
  resolve: {
    dedupe: ["react", "react-dom"],
  },
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
