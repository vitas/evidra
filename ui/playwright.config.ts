import { defineConfig, devices } from "@playwright/test";

const timeoutSeconds = Number(process.env.E2E_TIMEOUT_SECONDS || "120");

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  timeout: timeoutSeconds * 1000,
  expect: {
    timeout: 15_000,
  },
  reporter: [["list"], ["html", { outputFolder: "playwright-report", open: "never" }]],
  use: {
    baseURL: process.env.ARGO_BASE_URL || "https://localhost:8081",
    ignoreHTTPSErrors: true,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
