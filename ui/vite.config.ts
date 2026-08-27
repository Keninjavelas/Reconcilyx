import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  // GitHub Pages serves project sites beneath /<repository>/; local builds stay at /.
  base: process.env.VITE_BASE_PATH ?? "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: true,
    target: "es2022",
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ["react", "react-dom", "react-router-dom", "zustand"],
          charts: ["recharts"],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_API_BASE ?? process.env.RECONCILYX_API_URL ?? "http://localhost:8082",
        changeOrigin: true,
      },
      "/healthz": {
        target: process.env.VITE_API_BASE ?? process.env.RECONCILYX_API_URL ?? "http://localhost:8082",
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
});
