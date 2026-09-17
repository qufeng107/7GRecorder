import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";
import { mockPlugin } from "./src/dev/mockPlugin";

export default defineConfig(({ mode }) => ({
  plugins: [react(), ...(mode === "mock" ? [mockPlugin()] : [])],
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy:
      mode === "mock"
        ? undefined
        : {
            "/api": "http://127.0.0.1:18080",
            "/health": "http://127.0.0.1:18080",
          },
  },
  preview: {
    proxy: {
      "/api": "http://127.0.0.1:18080",
      "/health": "http://127.0.0.1:18080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./vitest.setup.ts",
    include: ["src/**/*.test.{ts,tsx}"],
  },
}));
