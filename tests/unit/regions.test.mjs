import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const regions = require("../../public/regions.js");
const config = { dataUrl: "data/repeaters.geojson", regions: { defaultIata: "YKF", entries: [
  { iata: "YKF", name: "Waterloo", centerLat: 43.46, centerLon: -80.38, zoom: 9, radiusKm: 200 },
  { iata: "YYZ", name: "Toronto", centerLat: 43.68, centerLon: -79.63, zoom: 9, radiusKm: 200 },
] } };
test("IATA selection isolates every data and API URL without mutating defaults", () => {
  const ykf = regions.resolve(config, "?iata=YKF");
  const yyz = regions.resolve(config, "?iata=yyz");
  assert.equal(ykf.config.dataBase, "data/regions/YKF");
  assert.equal(yyz.config.dataUrl, "data/regions/YYZ/repeaters.geojson");
  assert.equal(yyz.config.metaUrl, "data/regions/YYZ/meta.json");
  assert.equal(yyz.config.meshApiBase, "/mesh-api/regions/YYZ/api");
  assert.deepEqual(yyz.config.mapCenter, [43.68, -79.63]);
  assert.equal(config.dataUrl, "data/repeaters.geojson");
});
test("unknown, foreign and traversal IATAs resolve visibly to configured default", () => {
  for (const q of ["?iata=SEA", "?iata=../YKF", "?iata=<script>"]) {
    const r = regions.resolve(config, q);
    assert.equal(r.invalid, true); assert.equal(r.region.iata, "YKF");
  }
  assert.equal(regions.resolve(config, "").invalid, false);
});
test("native navigation provides deep links without carrying another region's shared plan", () => {
  const url = regions.regionURL("https://example.test/?iata=YKF&plan=abc", "YYZ", config.regions.entries);
  assert.equal(url, "https://example.test/?iata=YYZ");
  assert.throws(() => regions.regionURL(url, "SEA", config.regions.entries));
});
test("ordinary deployment remains unchanged and empty/error states are distinct", () => {
  const c = { dataUrl: "data/repeaters.geojson" };
  assert.equal(regions.resolve(c, "?iata=YKF").config, c);
  const r = config.regions.entries[0];
  assert.match(regions.dataMessage(r, "ready", 0), /does not prove/);
  assert.match(regions.dataMessage(r, "error"), /unavailable/);
  assert.match(regions.dataMessage(r, "ready", 2), /terrain prediction/);
});
