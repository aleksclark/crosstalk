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
  },
});
