import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { buildSetup } from "../../tools/build-ykf-setup.mjs";

const at = "2026-01-01T12:00:00.000Z";
const key = (n) => n.toString(16).padStart(64, "0");
const positions = [[43.3616, -80.3144], [43.5448, -80.2482], [43.4516, -80.4925], [43.4643, -80.5204]];

function fixture() {
  const items = positions.map(([lat, lng], i) => ({
    id: `synthetic-${i}`, publicKey: key(i + 1), nodeType: 2,
    name: `Synthetic repeater ${i + 1}`, lat, lng,
    iatas: [{ iata: "YKF", lastHeard: Date.parse(at) - 1000 }],
    neighborIds: i < 3 ? [`synthetic-${i + 1}`] : [],
  }));
  return [
    { items, hasMore: false },
    { nodes: items.map((n) => ({ public_key: n.publicKey, role: "repeater", name: n.name, lat: n.lat, lon: n.lng, last_heard: at })), has_more: false },
    { nodes: [], edges: [{ source: key(1), target: key(2), ambiguous: false, avg_snr: -6, weight: 123 }], stats: { total_edges: 1 } },
  ];
}

test("merges only full public keys and deduplicates pairs with both source labels", () => {
  const args = fixture();
  args[0].items[1].neighborIds.push("synthetic-0");
  args[1].nodes[0].public_key = key(1).toUpperCase();
  args[2].edges.push({ source: key(2), target: key(1), ambiguous: false });
  args[2].stats.total_edges++;
  const setup = buildSetup(...args, at);
  assert.equal(setup.nodes.length, 4);
  assert.equal(setup.links.length, 6);
  assert.deepEqual(setup.links.find((l) => l.from === 0 && l.to === 1).evidenceSources, ["beacon", "corescope"]);
  assert.deepEqual(setup.provenance.sources.map((s) => s.pairCount), [3, 1]);
  assert.ok(setup.nodes.every((n) => n.id === n.refId && /^[a-f0-9]{64}$/.test(n.id) && n.source === "real"));
});

test("filters bounds, invalid coordinates/keys, non-repeaters, ambiguous and self edges", () => {
  const args = fixture();
  const extras = [
    { publicKey: key(10), lat: 43.65, lng: -80.4 },
    { publicKey: key(11), lat: 43.4, lng: -80.64 },
    { publicKey: key(12), lat: null, lng: -80.4 },
    { publicKey: key(13), lat: "", lng: -80.4 },
    { publicKey: "000001", lat: 43.4, lng: -80.4 },
    { publicKey: key(14), lat: 43.4, lng: -80.4, nodeType: 1 },
  ];
  args[0].items.push(...extras.map((x, i) => ({ id: `excluded-${i}`, nodeType: 2, neighborIds: [], ...x })));
  args[0].items[0].neighborIds.push("synthetic-0", ...extras.map((_, i) => `excluded-${i}`), "absent");
  args[0].items.push({ id: "synthetic-0", publicKey: key(14), nodeType: 1, neighborIds: ["synthetic-3"] });
  args[2].edges.push(
    { source: key(1), target: key(3), ambiguous: true },
    { source: key(1), target: key(4) },
    { source: key(1), target: key(1), ambiguous: false },
    { source: key(1).slice(0, 6), target: key(4), ambiguous: false },
    { source: key(1), target: key(10), ambiguous: false },
  );
  args[2].stats.total_edges = args[2].edges.length;
  const setup = buildSetup(...args, at);
  assert.equal(setup.nodes.length, 4);
  assert.equal(setup.links.length, 6);
  assert.ok(setup.links.every((l) => l.from !== l.to));
});

test("joins Beacon adjacency to a CoreScope-only usable position by full key", () => {
  const args = fixture();
  args[0].items[0].lat = null;
  const setup = buildSetup(...args, at);
  assert.equal(setup.nodes.length, 4);
  assert.deepEqual(setup.links.find((l) => l.from === 0 && l.to === 1).evidenceSources, ["beacon", "corescope"]);
});

test("includes a CoreScope-only repeater and its observed pair without fabricating Beacon evidence", () => {
  const args = fixture();
  args[1].nodes.push({ public_key: key(5), name: "Synthetic fifth repeater", role: "repeater", lat: 43.4, lon: -80.4 });
  args[2].edges.push({ source: key(4), target: key(5), ambiguous: false });
  args[2].stats.total_edges++;
  const setup = buildSetup(...args, at);
  assert.equal(setup.nodes.length, 5);
  assert.deepEqual(setup.links.find((l) => l.from === 3 && l.to === 4).evidenceSources, ["corescope"]);
  assert.equal(setup.provenance.sources[0].nodeCount, 4);
  assert.equal(setup.provenance.sources[1].nodeCount, 5);
});

test("includes the bounding rectangle edges and does not mutate or depend on input ordering", () => {
  const args = fixture();
  args[0].items.push({ id: "boundary", publicKey: key(8), nodeType: 2, lat: 43.30, lng: -80.63, neighborIds: ["synthetic-0"] });
  args[0].items.push({ id: "other-boundary", publicKey: key(9), nodeType: 2, lat: 43.64, lng: -80.18, neighborIds: ["synthetic-3"] });
  const before = structuredClone(args);
  const setup = buildSetup(...args, at);
  assert.deepEqual(args, before);
  args[0].items.reverse();
  args[1].nodes.reverse();
  assert.deepEqual(buildSetup(...args, at), setup);
  assert.equal(setup.nodes.length, 6);
});

test("keeps weak or stale node evidence but records unknown ages and simulation assumptions", () => {
  const args = fixture();
  args[0].items[0].iatas = [{ iata: "YYZ", lastHeard: Date.parse(at) }];
  args[1].nodes[0].last_heard = null;
  args[1].nodes[1].last_heard = "2025-01-01T00:00:00Z";
  const setup = buildSetup(...args, at);
  assert.equal(setup.nodes.length, 4);
  assert.deepEqual(setup.provenance.nodeFreshness[0], { publicKey: key(1), beaconLastHeardMs: null, coreScopeLastHeard: null });
  assert.equal(setup.provenance.nodeFreshness[1].coreScopeLastHeard, "2025-01-01T00:00:00Z");
  assert.ok(setup.provenance.assumptions.some((s) => /direction.*unknown/i.test(s)));
  assert.ok(setup.provenance.assumptions.some((s) => /0 dB.*assum/i.test(s)));
  assert.ok(setup.provenance.assumptions.some((s) => /snapshot.*age/i.test(s)));
  assert.ok(setup.links.every((l) => l.snrDb === 0 && setup.links.some((r) => r.from === l.to && r.to === l.from)));
});

test("selects four distinct connected real sender indices, overrides radio and bounds the workload", () => {
  const setup = buildSetup(...fixture(), at);
  assert.equal(new Set(setup.messageGenerators.map((g) => g.nodeIndex)).size, 4);
  assert.deepEqual(setup.messageGenerators.map((g) => g.nodeIndex), [0, 1, 2, 3]);
  assert.equal(setup.seed, 1);
  assert.equal(setup.maxSimTimeMs, 60000);
  assert.equal(setup.trials, 5);
  for (const g of setup.messageGenerators) {
    assert.equal(g.count, 3);
    assert.equal(g.hashSize, 3);
    assert.equal(g.region, "");
    assert.equal(g.direct, false);
    assert.deepEqual([g.minPayload, g.maxPayload, g.minGapMs, g.maxGapMs], [10, 30, 10000, 15000]);
    assert.ok(setup.links.some((l) => l.from === g.nodeIndex));
  }
  for (const n of setup.nodes) {
    assert.equal(n.address, n.id.slice(0, 12).toUpperCase());
    assert.deepEqual(n.regions, ["*"]);
    assert.equal(n.denyUnscoped, false);
    assert.deepEqual(setup.nodePrefsOverrides[n.id].radio, { freqMhz: 910.525, bwKhz: 62.5, sf: 7, cr: 5, explicitHeader: true, crcEnabled: true });
  }
});

test("does not invent connectivity or silently reuse a sender when fewer than four are connected", () => {
  const args = fixture();
  args[0].items.forEach((n) => { n.neighborIds = []; });
  assert.throws(() => buildSetup(...args, at), /four.*connected/i);
});

test("rejects paginated, truncated, failed, malformed or inconsistent source envelopes", () => {
  for (const mutate of [
    (a) => { a[0].hasMore = true; },
    (a) => { delete a[0].hasMore; },
    (a) => { a[1].has_more = true; },
    (a) => { a[2].truncated = true; },
    (a) => { a[0].success = false; },
    (a) => { a[1].nodes = null; },
    (a) => { a[2].edges = {}; },
    (a) => { a[2].stats.total_edges = 50; },
    (a) => { a[0].items[1].id = a[0].items[0].id; },
  ]) {
    const args = fixture();
    mutate(args);
    assert.throws(() => buildSetup(...args, at));
  }
  assert.throws(() => buildSetup(...fixture(), "not-a-date"), /generatedAt/);
});

test("64 nodes allow at most 4032 distinct directed links; a 65th node fails rather than truncates", () => {
  const args = fixture();
  args[1].nodes = [];
  args[2] = { nodes: [], edges: [], stats: { total_edges: 0 } };
  args[0].items = Array.from({ length: 64 }, (_, i) => ({
    id: `dense-${i}`, publicKey: key(i + 1), nodeType: 2, name: `Synthetic ${i}`,
    lat: 43.4 + i / 10000, lng: -80.4, neighborIds: Array.from({ length: 64 }, (_, j) => `dense-${j}`),
  }));
  const setup = buildSetup(...args, at);
  assert.equal(setup.nodes.length, 64);
  assert.equal(setup.links.length, 4032);
  args[0].items.push({ id: "dense-64", publicKey: key(65), nodeType: 2, lat: 43.4, lng: -80.4, neighborIds: [] });
  assert.throws(() => buildSetup(...args, at), /64/);
});

test("CLI refuses a Git checkout output before any network call", () => {
  const script = fileURLToPath(new URL("../../tools/build-ykf-setup.mjs", import.meta.url));
  const forbidden = fileURLToPath(new URL("../../private-live-data.json", import.meta.url));
  const result = spawnSync(process.execPath, [script, forbidden], { encoding: "utf8", timeout: 5000 });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /outside every Git checkout/);
  assert.equal(result.stdout, "");
});
