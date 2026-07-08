import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  base: "/translator/",
  plugins: [react()],
  resolve: {
    // Single React instance across the app and workspace packages.
    dedupe: ["react", "react-dom"],
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5174,
    proxy: {
      // ws:true is required so the WebRTC signaling WebSocket
      // (/api/sessions/{id}/ws) is upgraded and forwarded, not just plain HTTP.
      "/api": {
        target: process.env.CT_API_PROXY || "http://localhost:8080",
        ws: true,
        changeOrigin: true,
      },
    },
  },
});
