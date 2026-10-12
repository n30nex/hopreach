// One bounded, explicit refresh of the public YKF simulator preset. Real node
// data stays outside Git: node tools/build-ykf-setup.mjs /private/ykf-setup.json
import { createRequire } from "node:module";
import { lstat, open, realpath, rename, unlink } from "node:fs/promises";
import { basename, dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const require = createRequire(import.meta.url);
const { RADIO_PRESETS } = require("../public/sim-constants.js");
const radioPreset = RADIO_PRESETS.find((p) => p.label === "USA/Canada (Recommended)");
const urls = {
  beacon: "https://live.meshcore.ca/api/v1/nodes?iata=YKF&type=2&neighbors=true&limit=200",
  coreNodes: "https://dev.canadaverse.org/api/nodes?region=YKF&limit=500",
  coreGraph: "https://dev.canadaverse.org/api/analytics/neighbor-graph",
};
const bounds = { south: 43.30, north: 43.64, west: -80.63, east: -80.18 };
const cities = [
  { name: "Cambridge", lat: 43.3616, lon: -80.3144 },
  { name: "Guelph", lat: 43.5448, lon: -80.2482 },
  { name: "Kitchener", lat: 43.4516, lon: -80.4925 },
  { name: "Waterloo", lat: 43.4643, lon: -80.5204 },
];
const maxNodes = 64;
const maxLinks = 4096;
const maxBodyBytes = 8 * 1024 * 1024;

function publicKey(value) {
  return typeof value === "string" && /^[a-f0-9]{64}$/i.test(value) ? value.toLowerCase() : null;
}

function coordinate(value) {
  if (typeof value !== "number" && (typeof value !== "string" || !value.trim())) return NaN;
  return Number(value);
}

function envelope(value, field, name) {
  if (!value || typeof value !== "object" || value.error != null || value.success === false ||
      value.truncated === true || value.hasMore === true || value.has_more === true || !Array.isArray(value[field])) {
    throw new Error(`${name}: failed, malformed or incomplete response`);
  }
  return value[field];
}

function validTimestamp(value) {
  return typeof value === "string" && Number.isFinite(Date.parse(value)) ? value : null;
}

/** Pure transformation; raw inputs and the timestamp are never mutated. */
export function buildSetup(beaconResponse, coreNodes, coreGraph, generatedAt) {
  if (!validTimestamp(generatedAt)) throw new Error("generatedAt must be an ISO timestamp");
  const beacon = envelope(beaconResponse, "items", "Beacon");
  const core = envelope(coreNodes, "nodes", "CoreScope nodes");
  const graph = envelope(coreGraph, "edges", "CoreScope graph");
  if (beaconResponse.hasMore !== false || coreNodes.has_more !== false ||
      beacon.length > 200 || core.length > 500 || graph.length > 50000 ||
      coreGraph.stats?.total_edges !== graph.length) {
    throw new Error("Incomplete, oversized or inconsistent source snapshot; refusing to truncate");
  }

  const nodesByKey = new Map();
  const beaconIDs = new Map();
  const freshness = new Map();
  const sourceNodes = { beacon: new Set(), corescope: new Set() };
  function addNode(row, source) {
    if (!row || (source === "beacon" ? row.nodeType !== 2 : row.role !== "repeater")) return;
    const key = publicKey(source === "beacon" ? row.publicKey : row.public_key);
    if (!key) return;
    if (source === "beacon" && typeof row.id === "string" && row.id) {
      if (beaconIDs.has(row.id) && beaconIDs.get(row.id) !== key) throw new Error("Beacon UUID maps to conflicting full public keys");
      beaconIDs.set(row.id, key);
    }
    const seen = freshness.get(key) || { publicKey: key, beaconLastHeardMs: null, coreScopeLastHeard: null };
    if (source === "beacon") {
      const times = Array.isArray(row.iatas) ? row.iatas.filter((i) => i?.iata === "YKF" && Number.isSafeInteger(i.lastHeard) && i.lastHeard >= 0).map((i) => i.lastHeard) : [];
      if (times.length) seen.beaconLastHeardMs = Math.max(seen.beaconLastHeardMs || 0, ...times);
    } else {
      seen.coreScopeLastHeard = validTimestamp(row.last_heard) || validTimestamp(row.last_seen);
    }
    freshness.set(key, seen);
    const lat = coordinate(row.lat), lon = coordinate(source === "beacon" ? row.lng : row.lon);
    if (!Number.isFinite(lat) || !Number.isFinite(lon) || lat < bounds.south || lat > bounds.north || lon < bounds.west || lon > bounds.east) return;
    sourceNodes[source].add(key);
    // CoreScope's usable position wins when both lists contain the same key,
    // matching the coverage map. Neither timestamp proves position freshness.
    nodesByKey.set(key, {
      id: key, refId: key, source: "real", role: "repeater",
      label: typeof row.name === "string" && row.name.trim() ? row.name.trim().slice(0, 160) : `Repeater ${key.slice(0, 12)}`,
      lat, lon, regions: ["*"], denyUnscoped: false, hashSize: 3,
      address: key.slice(0, 12).toUpperCase(), coordinateSource: source,
    });
  }
  beacon.forEach((row) => addNode(row, "beacon"));
  core.forEach((row) => addNode(row, "corescope"));
  if (nodesByKey.size > maxNodes) throw new Error(`Snapshot exceeds ${maxNodes} local nodes; refusing to truncate`);
  const nodes = [...nodesByKey.values()].sort((a, b) => a.id.localeCompare(b.id));
  const indexByKey = new Map(nodes.map((n, i) => [n.id, i]));
  const pairs = new Map();
  function addPair(a, b, source) {
    a = publicKey(a); b = publicKey(b);
    if (!a || !b || a === b || !indexByKey.has(a) || !indexByKey.has(b)) return;
    const [from, to] = [indexByKey.get(a), indexByKey.get(b)].sort((x, y) => x - y);
    const id = `${from}:${to}`;
    if (!pairs.has(id)) pairs.set(id, { from, to, sources: new Set() });
    pairs.get(id).sources.add(source);
    if (pairs.size * 2 > maxLinks) throw new Error(`Snapshot exceeds ${maxLinks} directed links; refusing to truncate`);
  }
  for (const row of beacon) {
    if (row?.neighborIds != null && !Array.isArray(row.neighborIds)) throw new Error("Malformed Beacon neighborIds");
    const from = beaconIDs.get(row?.id);
    if (!from || row.nodeType !== 2 || publicKey(row.publicKey) !== from) continue;
    for (const id of row?.neighborIds || []) addPair(from, beaconIDs.get(id), "beacon");
  }
  for (const edge of graph) {
    if (edge?.ambiguous === false) addPair(edge.source, edge.target, "corescope");
  }
  const links = [];
  const pairCounts = { beacon: 0, corescope: 0 };
  for (const pair of [...pairs.values()].sort((a, b) => a.from - b.from || a.to - b.to)) {
    const evidenceSources = [...pair.sources].sort();
    evidenceSources.forEach((source) => pairCounts[source]++);
    // The APIs expose adjacency, not reliable direction or pairwise SNR.
    for (const [from, to] of [[pair.from, pair.to], [pair.to, pair.from]]) links.push({ from, to, snrDb: 0, evidenceSources });
  }
  const connected = new Set(links.map((l) => l.from));
  if (connected.size < 4) throw new Error("Need four distinct connected real repeaters for the city senders");
  const used = new Set();
  const messageGenerators = cities.map((city) => {
    let nodeIndex = -1, nearest = Infinity;
    nodes.forEach((n, i) => {
      if (!connected.has(i) || used.has(i)) return;
      const distance = Math.hypot(n.lat - city.lat, (n.lon - city.lon) * Math.cos(city.lat * Math.PI / 180));
      if (distance < nearest) { nearest = distance; nodeIndex = i; }
    });
    used.add(nodeIndex);
    return {
      id: `ykf-${city.name.toLowerCase()}`, label: `${city.name} synthetic sender`, nodeIndex,
      region: "", direct: false, hashSize: 3, count: 3, minPayload: 10, maxPayload: 30, minGapMs: 10000, maxGapMs: 15000,
    };
  });
  const { freqMhz, bwKhz, sf, cr } = radioPreset;
  return {
    id: "ykf-metro", name: "YKF — Cambridge, Guelph, Kitchener & Waterloo",
    savedAt: Date.parse(generatedAt), nodes, links, connectivitySource: "corescope", messageGenerators,
    nodePrefsOverrides: Object.fromEntries(nodes.map((n) => [n.id, { radio: { freqMhz, bwKhz, sf, cr, explicitHeader: true, crcEnabled: true } }])),
    seed: 1, maxSimTimeMs: 60000, trials: 5,
    provenance: {
      id: "ykf-metro", generatedAt,
      summary: `${nodes.length} observed repeaters and ${pairs.size} known-neighbor pairs from live.meshcore.ca and dev.canadaverse.org, limited to Cambridge, Guelph, Kitchener and Waterloo.`,
      bounds: { ...bounds },
      assumptions: [
        "Known-neighbor adjacency is observed; edge directions and edge ages are unknown. Each pair is simulated symmetrically, without proving reciprocal reception.",
        "0 dB SNR is an assumed simulation placeholder for every link, not a measured signal or a conversion from observation counts.",
        "This snapshot ages after it is generated. Node last-heard values do not date individual links or positions; stale and unknown-age nodes are retained, and missing edges mean unknown connectivity.",
        "YKF selects the node catalogues. Beacon neighbor IDs span all IATAs; CoreScope supplies its cached global graph, intersected with those local full public keys. Edge IATA provenance is unknown.",
        "The area is a rectangular processing extent: latitude 43.30–43.64, longitude -80.63–-80.18; it is not an official municipal or radio-coverage boundary.",
        "All nodes use the USA/Canada preset (910.525 MHz, 62.5 kHz, SF7, CR5), accept all scopes (*) and allow unscoped traffic as simulation assumptions, not verified device configuration.",
        "Four distinct linked repeaters nearest the named city centres originate synthetic messages: three each, 10–30-byte payloads, 10–15-second gaps, 3-byte hashes. No messages are sent over radio.",
        "Unchanged simulator defaults supply power, relay delays, loop detection and channel fading; these are model settings rather than measurements of the selected devices.",
      ],
      sources: [
        { id: "beacon", url: urls.beacon, fetchedAt: generatedAt, nodeCount: sourceNodes.beacon.size, pairCount: pairCounts.beacon },
        { id: "corescope", url: urls.coreGraph, nodesUrl: urls.coreNodes, fetchedAt: generatedAt, nodeCount: sourceNodes.corescope.size, pairCount: pairCounts.corescope },
      ],
      nodeFreshness: nodes.map((n) => freshness.get(n.id)),
    },
  };
}

async function fetchJSON(url) {
  const response = await fetch(url, { signal: AbortSignal.timeout(30000), redirect: "error", headers: { Accept: "application/json", "User-Agent": "Canadaverse-HopReach-preset/1.0" } });
  if (!response.ok || !response.headers.get("content-type")?.includes("application/json")) throw new Error(`Source did not return JSON successfully: ${new URL(url).hostname} (${response.status})`);
  const chunks = [];
  let length = 0;
  for await (const chunk of response.body) {
    length += chunk.length;
    if (length > maxBodyBytes) throw new Error("Source response exceeded 8 MiB; refusing to truncate");
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

async function outsideGitOutput(value) {
  const output = resolve(value);
  const parent = await realpath(dirname(output));
  for (let path = parent; ; path = dirname(path)) {
    try { await lstat(join(path, ".git")); } catch (error) {
      if (error.code !== "ENOENT") throw error;
      if (dirname(path) === path) break;
      continue;
    }
    throw new Error("Output must be outside every Git checkout; live node data must not enter source control");
  }
  const target = join(parent, basename(output));
  try {
    const info = await lstat(target);
    if (!info.isFile() || info.isSymbolicLink()) throw new Error("Output must be a regular file, not a link or directory");
  } catch (error) { if (error.code !== "ENOENT") throw error; }
  return target;
}

async function main() {
  if (process.argv.length !== 3) throw new Error("Usage: node tools/build-ykf-setup.mjs <output.json outside Git>");
  const output = await outsideGitOutput(process.argv[2]);
  // Sequential: one request per fixed URL, with no retries or per-node fanout.
  const beacon = await fetchJSON(urls.beacon);
  const beaconFetchedAt = new Date().toISOString();
  const coreNodes = await fetchJSON(urls.coreNodes);
  const coreNodesFetchedAt = new Date().toISOString();
  const coreGraph = await fetchJSON(urls.coreGraph);
  const setup = buildSetup(beacon, coreNodes, coreGraph, new Date().toISOString());
  setup.provenance.sources[0].fetchedAt = beaconFetchedAt;
  setup.provenance.sources[1].nodesFetchedAt = coreNodesFetchedAt;
  const temporary = `${output}.${process.pid}.tmp`;
  let created = false;
  try {
    const handle = await open(temporary, "wx", 0o600);
    created = true;
    try { await handle.writeFile(JSON.stringify(setup, null, 2) + "\n"); await handle.sync(); } finally { await handle.close(); }
    await rename(temporary, output);
  } finally {
    if (created) await unlink(temporary).catch((error) => { if (error.code !== "ENOENT") throw error; });
  }
  console.log(JSON.stringify({ nodes: setup.nodes.length, pairs: setup.links.length / 2, directedLinks: setup.links.length, sources: setup.provenance.sources, generatedAt: setup.provenance.generatedAt }));
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  main().catch((error) => { console.error(error.message); process.exitCode = 1; });
}
