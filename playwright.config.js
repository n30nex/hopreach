// @ts-check
const { defineConfig, devices } = require("@playwright/test");

// Same production Go/WASM/nginx image, deterministic CoreScope + terrain input.
// A small complete CPU pass makes coverage/scope/replay tests non-skippable.
const composeFile = process.env.HOPREACH_E2E_COMPOSE || "tests/offline-fixtures/compose.yml";
const baseURL = process.env.HOPREACH_E2E_BASE_URL || `http://127.0.0.1:${process.env.HOPREACH_E2E_PORT || 8080}`;
const reporters = process.env.CI ? [["github"], ["html", { open: "never" }]] : [["list"]];
reporters.push(["./tests/offline-fixtures/no-skips-reporter.js"]);
module.exports = defineConfig({
  testDir: "./tests",
  testMatch: "**/*.spec.js",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: reporters,
  use: {
    baseURL,
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: process.env.HOPREACH_E2E_EXTERNAL_SERVER === "1" ? undefined : {
    // Build separately so compilation failures are distinct from UX failures.
    command: `docker compose -f "${composeFile}" up`,
    url: `${baseURL}/e2e-ready`,
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
});
