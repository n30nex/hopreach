import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const presets = require("../../public/sim-presets.js");
const fixture = require("../fixtures/ykf-sim-setup.json");
const sample = () => structuredClone(fixture);

test("YKF preset accepts bounded nodes, adjacency and both source snapshots", () => {
  const setup = sample();
  assert.equal(presets.validateSetup(setup), setup);
  const note = presets.provenanceText(setup.provenance);
  assert.match(note, /Original preset snapshot/);
  assert.match(note, /live.meshcore.ca/);
  assert.match(note, /dev.canadaverse.org/);
  assert.match(note, /0 dB SNR/);
  assert.match(note, /2026-10-12/);
});

test("invalid or unbounded feeds fail before reaching the workspace", () => {
  const mutations = [
    (s) => { s.nodes = []; },
    (s) => { s.nodes = Array(65).fill(s.nodes[0]); },
    (s) => { s.nodes[1].id = s.nodes[0].id; },
    (s) => { s.nodes[1].refId = "short"; },
    (s) => { s.nodes[0].lat = "43.5"; },
    (s) => { s.links = []; },
    (s) => { s.links[0].to = 99; },
    (s) => { s.links[0].snrDb = null; },
    (s) => { s.messageGenerators[0].nodeIndex = -1; },
    (s) => { s.messageGenerators[0].count = 1000000; },
    (s) => { s.nodePrefsOverrides["fixture-a"].radio.sf = 30; },
    (s) => { s.provenance.sources.pop(); },
    (s) => { s.provenance.sources[0].url = "https://unrelated.example/"; },
    (s) => { s.provenance.generatedAt = "invalid"; },
    (s) => { s.provenance.assumptions = []; },
    (s) => { s.maxSimTimeMs = 999999999; },
  ];
  for (const mutate of mutations) {
    const setup = sample(); mutate(setup);
    assert.throws(() => presets.validateSetup(setup), /Invalid YKF setup/);
  }
});

test("unknown metadata is not presented as a sourced preset", () => {
  assert.equal(presets.provenanceText(null), "");
  assert.equal(presets.provenanceText({ id: "other" }), "");
});

test("a delayed preset cannot repopulate a workspace cleared with New", async () => {
  let click, release, imports = 0;
  const pending = new Promise((resolve) => { release = resolve; });
  const button = { addEventListener(event, listener) { click = listener; } };
  const root = {
    HOPREACH_CONFIG: { iata: "YKF" },
    SimState: { simNodes: [], linksGeneration: 0 },
    location: { href: "https://example.test/?iata=YKF", search: "?iata=YKF" },
    history: { replaceState() {} },
    document: { getElementById() { return button; } },
    fetch: () => pending,
  };
  presets.init({ root, openPanel() {}, setStatus() {}, importSetup() { imports++; } });
  const loading = click();
  root.SimState.linksGeneration++; // clearNodes() invalidates the old workspace
  release(new Response(JSON.stringify(fixture), { status: 200 }));
  assert.equal(await loading, false);
  assert.equal(imports, 0);
});
