// Deterministic browser contract for the real regional controls and API client.
// No terrain service, live mesh, credentials or quiet-network skips required.
const { test, expect } = require("@playwright/test");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const root = join(__dirname, "..");
const shell = readFileSync(join(root, "public/index.html"), "utf8");
const config = { siteName: "Canadaverse Coverage", regions: { defaultIata: "YKF", entries: [
  { iata: "YKF", name: "Waterloo", centerLat: 43.46, centerLon: -80.38, zoom: 9, radiusKm: 200 },
  { iata: "YYZ", name: "Toronto", centerLat: 43.68, centerLon: -79.63, zoom: 9, radiusKm: 200 },
] } };

async function fixture(page, mode = "normal") {
  const requests = [];
  const controls = shell.slice(shell.indexOf('<section id="regional-controls"'), shell.indexOf('<div id="error-banner"'));
  const replay = shell.slice(shell.indexOf('<div class="sim-promoted" id="sim-promoted-replay"'), shell.indexOf('<!-- Advanced-only:'));
  await page.route("http://regional.test/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    requests.push(path);
    if (path.endsWith(".js")) {
      await route.fulfill({ contentType: "application/javascript", body: readFileSync(join(root, "public", path.slice(1)), "utf8") });
    } else if (path.includes("/api/nodes")) {
      await route.fulfill({ status: mode === "error" ? 502 : 200, contentType: "application/json", body: JSON.stringify({ nodes: mode === "empty" ? [] : [{ public_key: path.includes("YKF") ? "waterloo" : "toronto" }] }) });
    } else {
      await route.fulfill({ contentType: "text/html", body: `<!doctype html><html><head><meta charset="utf-8"><style>.hidden,[hidden]{display:none!important}</style></head><body>
        <a id="canadaverse-brand" hidden>Canadaverse</a>${controls}${replay}<footer id="regional-footer" hidden>HopReach</footer>
        <p id="node-result"></p><script>window.HOPREACH_CONFIG=${JSON.stringify(config)}</script>
        <script src="/regions.js"></script><script src="/mesh-api.js"></script>
        <script>MeshApi.nodes().then(nodes=>{document.getElementById('node-result').textContent=nodes.map(n=>n.public_key).join(',');HopReachRegions.showDataState('ready',nodes.length)}).catch(()=>HopReachRegions.showDataState('error'))</script>
        </body></html>` });
    }
  });
  return requests;
}

test("YKF/YYZ navigation, browser history and API isolation", async ({ page }) => {
  const requests = await fixture(page);
  await page.goto("http://regional.test/?iata=YKF");
  await expect(page.locator("#node-result")).toHaveText("waterloo");
  await page.getByLabel("Canada / IATA region").selectOption("YYZ");
  await expect(page).toHaveURL("http://regional.test/?iata=YYZ");
  await expect(page.locator("#node-result")).toHaveText("toronto");
  await page.goBack();
  await expect(page.getByLabel("Canada / IATA region")).toHaveValue("YKF");
  await expect(page.locator("#node-result")).toHaveText("waterloo");
  await page.goForward();
  await expect(page.getByLabel("Canada / IATA region")).toHaveValue("YYZ");
  await expect(page.locator("#node-result")).toHaveText("toronto");
  expect(requests.filter((p) => p.includes("/api/")).every((p) => /^\/mesh-api\/regions\/(YKF|YYZ)\/api\/nodes$/.test(p))).toBeTruthy();
  await expect(page.locator("#regional-status")).toContainText("200 km");
  await expect(page.locator("#sim-replay-hash-go")).toBeHidden();
  await expect(page.getByText("Regional packet replay is unavailable")).toBeVisible();
});

test("foreign/invalid IATA is explained and never requested", async ({ page }) => {
  const requests = await fixture(page);
  await page.goto("http://regional.test/?iata=SEA");
  await expect(page).toHaveURL("http://regional.test/?iata=YKF");
  await expect(page.locator("#regional-warning")).toContainText("not configured");
  await expect(page.locator("#node-result")).toHaveText("waterloo");
  expect(requests.some((p) => p.includes("/SEA/"))).toBeFalsy();
});

for (const mode of ["empty", "error"]) {
  test(`${mode} regional data stays distinct on a phone`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await fixture(page, mode);
    await page.goto("http://regional.test/?iata=YYZ");
    await expect(page.locator("#regional-status")).toContainText(mode === "empty" ? "does not prove" : "unavailable");
    await expect(page.getByLabel("Canada / IATA region")).toBeVisible();
  });
}
