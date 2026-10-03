import { defineConfig, devices } from "@playwright/test";

/**
 * E2E against the real compose stack (`make up` at the repo root) plus the
 * Vite dev server. Not part of `pnpm test`; run with `pnpm e2e` after
 * `pnpm exec playwright install chromium`.
 */
export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  use: {
    baseURL: "http://localhost:5173",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "pnpm dev",
    url: "http://localhost:5173/login",
    reuseExistingServer: true,
    env: {
      VITE_KRATOS_PUBLIC_URL: "http://localhost:4433",
      VITE_API_URL: "http://localhost:8080",
    },
  },
});
