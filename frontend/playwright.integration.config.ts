import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  outputDir: "test-results/integration",
  testMatch: "integration.spec.ts",
  workers: 1,
  use: { baseURL: "http://127.0.0.1:4173", trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command:
      "exec python3 ../scripts/dev/local_environment.py --mode preview --test-account",
    url: "http://127.0.0.1:4173",
    timeout: 180000,
    gracefulShutdown: { signal: "SIGTERM", timeout: 15000 },
    reuseExistingServer: false,
  },
});
