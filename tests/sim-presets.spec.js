// Only the published setup JSON is substituted. Map, API and simulator WASM
// still run against the normal offline container used by the other UI tests.
const { test, expect } = require("@playwright/test");
const fs = require("node:fs");
const { gotoReady } = require("./helpers");
const { openAccordion } = require("./sim-helpers");
const fixture = require("./fixtures/ykf-sim-setup.json");
const PRESET = "**/data/regions/YKF/sim-setup.json";

test.beforeEach(async ({ page }) => {
  // The small offline fixture is a single dataset. Set its selected IATA
  // without substituting any map, mesh API, terrain or WASM responses.
  await page.addInitScript(() => {
    let config;
    Object.defineProperty(window, "HOPREACH_CONFIG", {
      configurable: true,
      get: () => config,
      set(value) { config = value; config.iata = new URLSearchParams(location.search).get("iata") || "YKF"; },
    });
  });
});

async function loaded(page) {
  await gotoReady(page, "/?iata=YKF&setup=ykf-metro");
  await expect(page.locator("#sim-status")).toContainText("YKF metro setup loaded");
}

test("phone deep link loads one sourced snapshot without running, then runs real WASM", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  let requests = 0;
  await page.route(PRESET, (route) => { requests++; return route.fulfill({ json: fixture }); });
  await loaded(page);
  await expect(page.locator("#sim-panel")).toBeVisible();
  await expect(page.locator("#sim-node-count-badge")).toHaveText("3");
  await expect(page.locator("#sim-message-count-badge")).toHaveText("1");
  await expect(page.locator("#sim-preset-provenance")).toBeVisible();
  await expect(page.locator("#sim-preset-summary")).toContainText("0 dB signal levels");
  await page.locator("#sim-preset-provenance summary").click();
  await expect(page.locator("#sim-preset-details")).toBeVisible();
  await expect(page.locator("#sim-preset-details")).toContainText("0 dB SNR");
  await page.locator("#sim-preset-provenance summary").click();
  expect(await page.evaluate(() => window.SimState.lastReport)).toBeNull();
  expect(await page.evaluate(() => window.SimState.simLinks.length)).toBe(4);
  await page.click('[data-rail-target="sim-acc-run"]');
  await page.click("#sim-run");
  await expect(page.locator("#sim-status")).toHaveText("Done.", { timeout: 30000 });
  expect(await page.evaluate(() => window.SimState.lastReport.receptions.length)).toBeGreaterThan(0);
  expect(requests).toBe(1);
});

test("save, reload and export preserve snapshot provenance; New and saved selection clear the preset URL", async ({ page }) => {
  await page.route(PRESET, (route) => route.fulfill({ json: fixture }));
  await loaded(page);
  await page.locator(".sim-marker-icon").first().dispatchEvent("mouseover");
  await expect(page.locator(".leaflet-tooltip")).toContainText("<b>Fixture Cambridge</b>");
  await expect(page.locator(".leaflet-tooltip b")).toHaveCount(0);
  await page.locator(".sim-marker-icon").first().dispatchEvent("mouseout");
  await openAccordion(page, "sim-acc-setups");
  await page.fill("#sim-setup-name", "My YKF experiment");
  await page.click("#sim-setup-save");
  expect(new URL(page.url()).searchParams.has("setup")).toBe(false);
  const id = await page.evaluate(() => window.SimState.currentSetupId);
  await page.click("#sim-setup-new");
  await expect(page.locator("#sim-preset-provenance")).toBeHidden();
  expect(new URL(page.url()).searchParams.has("setup")).toBe(false);
  await page.selectOption("#sim-setup-select", id);
  expect(await page.evaluate(() => window.SimState.setupProvenance)).toEqual(fixture.provenance);
  expect(new URL(page.url()).searchParams.has("setup")).toBe(false);
  const [download] = await Promise.all([page.waitForEvent("download"), page.click("#sim-setup-export")]);
  const exported = JSON.parse(fs.readFileSync(await download.path(), "utf8"));
  expect(exported.provenance).toEqual(fixture.provenance);
  await page.reload();
  await page.click("#sim-toggle");
  await openAccordion(page, "sim-acc-setups");
  await page.selectOption("#sim-setup-select", id);
  expect(await page.evaluate(() => window.SimState.setupProvenance)).toEqual(fixture.provenance);
});

test("missing and malformed snapshots preserve a populated workspace; replacing needs confirmation", async ({ page }) => {
  let payload = fixture, status = 200;
  await page.route(PRESET, (route) => route.fulfill({ status, json: payload }));
  await loaded(page);
  await openAccordion(page, "sim-acc-setups");
  await page.fill("#sim-setup-name", "Unsaved changes");
  const before = await page.evaluate(() => JSON.stringify(window.SimState.simNodes));
  status = 404;
  await page.click("#sim-preset-ykf");
  await expect(page.locator("#sim-status")).toContainText("HTTP 404");
  status = 200; payload = { ...fixture, links: [{ from: 0, to: 999, snrDb: 0 }] };
  await page.click("#sim-preset-ykf");
  await expect(page.locator("#sim-status")).toContainText("Invalid YKF setup");
  expect(await page.evaluate(() => JSON.stringify(window.SimState.simNodes))).toBe(before);
  await expect(page.locator("#sim-setup-name")).toHaveValue("Unsaved changes");
  payload = fixture;
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.click("#sim-preset-ykf");
  await expect(page.locator("#sim-status")).toContainText("cancelled");
  await expect(page.locator("#sim-setup-name")).toHaveValue("Unsaved changes");
});

test("unknown preset and other IATA do not fetch or expose the YKF action", async ({ page }) => {
  let requests = 0;
  await page.route(PRESET, (route) => { requests++; return route.fulfill({ json: fixture }); });
  await gotoReady(page, "/?iata=YKF&setup=unknown");
  await page.click("#sim-toggle");
  await openAccordion(page, "sim-acc-setups");
  await expect(page.locator("#sim-preset-ykf")).toBeVisible();
  await expect(page.locator("#sim-node-count-badge")).toHaveText("0");
  await gotoReady(page, "/?iata=YYZ&setup=ykf-metro");
  await page.click("#sim-toggle");
  await openAccordion(page, "sim-acc-setups");
  await expect(page.locator("#sim-preset-ykf")).toBeHidden();
  expect(requests).toBe(0);
});
