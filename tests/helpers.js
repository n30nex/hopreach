// @ts-check
const { expect } = require("@playwright/test");

// Waits for the page to be genuinely interactive: the map rendered and the
// WASM module (see wasm/main.go, public/wasm-bridge.js) ready — everything
// the planning tools and simulator actually need. Deliberately does NOT
// wait for real repeater data (meta.json/repeaters.geojson) to have
// loaded: that depends on the container's own background fetch reaching a
// live, third-party CoreScope instance over the real network, which can be
// meaningfully slower (or less reliable) from CI infrastructure than from
// a normal dev machine — a dependency most tests here have no actual need
// for (add-repeater/LOS/companion-pin/"load planned repeaters" are all
// client-only). Tests that use existing repeaters wait separately below.
async function gotoReady(page, path = "/") {
  await page.goto(path);
  await page.waitForSelector(".leaflet-container");
  await page.evaluate(() => window.__hopreachWasmReadyPromise);
}

// Cluster icons count visual groups, not loaded repeaters. Wait for populated
// UI totals and any rendered marker shape, then let a test uncluster if it needs
// to count or click individual nodes.
async function waitForRepeaterData(page, minimum = 1) {
  await expect.poll(async () => {
    const counts = await page.locator("#count-active, #count-degraded, #count-silent").allTextContents();
    return counts.length === 3 ? counts.reduce((sum, count) => sum + Number(count), 0) : 0;
  }, { timeout: 30_000 }).toBeGreaterThanOrEqual(minimum);
  await expect.poll(() => page.locator("#map .leaflet-marker-icon, #map path.leaflet-interactive").count(), {
    timeout: 30_000,
  }).toBeGreaterThan(0);
}

module.exports = { gotoReady, waitForRepeaterData };
