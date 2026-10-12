const { defineConfig, devices } = require("@playwright/test");
module.exports = defineConfig({
  testDir: "./tests", testMatch: ["regions.spec.js", "meshmapper.spec.js"], fullyParallel: true,
  forbidOnly: !!process.env.CI, retries: 0, workers: process.env.CI ? 1 : 2,
  reporter: "list", projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
