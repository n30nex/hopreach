// A published, self-contained setup uses the same import path as a local file.
// Observation backends are normalised offline; this module never queries them.
(function (root, factory) {
  if (typeof module === "object" && module.exports) module.exports = factory();
  else root.SimPresets = factory();
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";
  const ID = "ykf-metro";
  const MAX_BYTES = 1024 * 1024;
  const SOURCE_ORIGINS = ["https://live.meshcore.ca", "https://dev.canadaverse.org"];
  const number = (n, lo, hi) => Number.isFinite(n) && n >= lo && n <= hi;
  const integer = (n, lo, hi) => Number.isInteger(n) && number(n, lo, hi);
  const text = (s, max = 512) => typeof s === "string" && s.length > 0 && s.length <= max;
  const date = (s) => text(s, 40) && /^\d{4}-\d{2}-\d{2}T/.test(s) && Number.isFinite(Date.parse(s));
  const list = (a, max) => Array.isArray(a) && a.length > 0 && a.length <= max;
  let context, loading = false;

  function requireValid(ok) {
    if (!ok) throw new Error("Invalid YKF setup snapshot");
  }

  function validProvenance(p) {
    if (!p || p.id !== ID || !date(p.generatedAt) || !text(p.summary) ||
        !list(p.assumptions, 16) || !p.assumptions.every((s) => text(s, 2048)) ||
        !Array.isArray(p.sources) || p.sources.length !== 2) return false;
    try {
      const origins = new Set(p.sources.map((s) => {
        const url = new URL(s.url);
        if (url.username || url.password || !date(s.fetchedAt) ||
            !integer(s.nodeCount, 0, 1000000) || !integer(s.pairCount, 0, 10000000)) throw new Error();
        return url.origin;
      }));
      return SOURCE_ORIGINS.every((origin) => origins.has(origin));
    } catch { return false; }
  }

  function validateSetup(s) {
    requireValid(s && text(s.name) && validProvenance(s.provenance));
    requireValid(list(s.nodes, 64) && list(s.links, 4096) && list(s.messageGenerators, 8));
    requireValid(["model", "corescope", "blend"].includes(s.connectivitySource));
    requireValid(integer(Number(s.seed), 0, 2147483647) && integer(Number(s.trials), 1, 100) &&
      integer(Number(s.maxSimTimeMs), 1000, 120000));
    const ids = new Set(), keys = new Set();
    for (const n of s.nodes) {
      requireValid(n && text(n.id, 128) && /^[a-z0-9_-]+$/i.test(n.id) &&
        !["__proto__", "constructor", "prototype"].includes(n.id) && !ids.has(n.id));
      requireValid(n.source === "real" && /^[a-f0-9]{64}$/i.test(n.refId) && !keys.has(n.refId.toLowerCase()));
      requireValid(text(n.label) && number(n.lat, -90, 90) && number(n.lon, -180, 180));
      requireValid(Array.isArray(n.regions) && n.regions.length <= 32 && n.regions.every((v) => text(v, 128)));
      ids.add(n.id); keys.add(n.refId.toLowerCase());
    }
    const pairs = new Set();
    for (const l of s.links) {
      requireValid(l && integer(l.from, 0, s.nodes.length - 1) && integer(l.to, 0, s.nodes.length - 1) &&
        l.from !== l.to && number(l.snrDb, -100, 100) && !pairs.has(`${l.from}:${l.to}`));
      pairs.add(`${l.from}:${l.to}`);
    }
    for (const g of s.messageGenerators) validateSender(g, s.nodes.length);
    requireValid(s.nodePrefsOverrides && Object.keys(s.nodePrefsOverrides).length <= s.nodes.length);
    for (const [id, prefs] of Object.entries(s.nodePrefsOverrides)) {
      const r = prefs && prefs.radio;
      requireValid(ids.has(id) && r && number(r.freqMhz, 100, 2500) && number(r.bwKhz, 7.8, 500) &&
        integer(r.sf, 7, 12) && integer(r.cr, 5, 8) && typeof r.explicitHeader === "boolean" && typeof r.crcEnabled === "boolean");
    }
    return s;
  }

  function validateSender(g, nodeCount) {
    requireValid(g && text(g.id, 128) && integer(g.nodeIndex, 0, nodeCount - 1) &&
      integer(g.count, 1, 100) && integer(g.minPayload, 1, 255) && integer(g.maxPayload, g.minPayload, 255) &&
      integer(g.minGapMs, 1, 120000) && integer(g.maxGapMs, g.minGapMs, 120000) &&
      integer(g.hashSize, 1, 3) && typeof g.direct === "boolean" && typeof g.region === "string" && g.region.length <= 128 && !g.fixed);
  }

  function provenanceText(p) {
    if (!validProvenance(p)) return "";
    const sources = p.sources.map((s) => {
      const hours = Math.max(0, Math.floor((Date.now() - Date.parse(s.fetchedAt)) / 3600000));
      return `${new URL(s.url).hostname}: fetched ${s.fetchedAt} (${hours}h ago), ${s.nodeCount} nodes / ${s.pairCount} neighbour pairs`;
    });
    return `Original preset snapshot · ${p.generatedAt}. ${p.summary} ${sources.join("; ")}. ` +
      `Fetch times are not the age of individual observations. ${p.assumptions.join(" ")} ` +
      "Nodes and settings may be edited after loading; this note describes the original snapshot.";
  }

  function clearURL() {
    const root = context.root, url = new URL(root.location.href);
    url.searchParams.delete("setup");
    root.history.replaceState(null, "", url.href);
  }

  function restore(provenance) {
    if (!context) return;
    const root = context.root, state = root.SimState;
    state.setupProvenance = validProvenance(provenance) ? provenance : null;
    const note = root.document.getElementById("sim-preset-provenance");
    note.hidden = !state.setupProvenance;
    root.document.getElementById("sim-preset-details").textContent = provenanceText(provenance);
    root.document.getElementById("sim-preset-summary").textContent = state.setupProvenance ?
      `Original preset snapshot · ${new Date(provenance.generatedAt).toLocaleString()}. ${provenance.summary} ` +
      "Link directions, 0 dB signal levels, radio settings, relay policy and traffic are simulation assumptions." : "";
    clearURL();
  }

  async function readSnapshot(response) {
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    if (Number(response.headers.get("content-length")) > MAX_BYTES) throw new Error("snapshot is too large");
    const reader = response.body.getReader(), decoder = new TextDecoder();
    let body = "", size = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_BYTES) { await reader.cancel(); throw new Error("snapshot is too large"); }
      body += decoder.decode(value, { stream: true });
    }
    return validateSetup(JSON.parse(body + decoder.decode()));
  }

  async function load() {
    const { root, importSetup, setStatus } = context;
    if (loading || root.HOPREACH_CONFIG.iata !== "YKF") return false;
    const button = root.document.getElementById("sim-preset-ykf");
    const generation = root.SimState.linksGeneration;
    loading = true; button.disabled = true;
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 12000);
    setStatus("sim-status", "Loading the YKF neighbour snapshot…");
    try {
      const response = await root.fetch("data/regions/YKF/sim-setup.json", { signal: controller.signal, cache: "no-cache" });
      const setup = await readSnapshot(response);
      if (generation !== root.SimState.linksGeneration) {
        setStatus("sim-status", "YKF setup cancelled because the workspace changed. Your current setup is unchanged.");
        return false;
      }
      if (root.SimState.simNodes.length && !root.confirm("Replace the current simulation workspace with the YKF metro setup? Saved setups will be kept.")) {
        setStatus("sim-status", "YKF setup cancelled. Your current setup is unchanged.");
        return false;
      }
      importSetup(setup);
      const url = new URL(root.location.href);
      url.searchParams.set("iata", "YKF"); url.searchParams.set("setup", ID);
      root.history.replaceState(null, "", url.href);
      setStatus("sim-status", "YKF metro setup loaded. Review the assumptions below, then Run. Save to keep your own copy.");
      return true;
    } catch (err) {
      const reason = err.name === "AbortError" ? "request timed out" : err.message;
      setStatus("sim-status", `YKF setup unavailable (${reason}). Try again later; your current setup is unchanged.`);
      return false;
    } finally {
      clearTimeout(timeout); loading = false; button.disabled = false;
    }
  }

  function init(options) {
    context = options;
    const { root, openPanel } = context;
    const button = root.document.getElementById("sim-preset-ykf");
    button.hidden = root.HOPREACH_CONFIG.iata !== "YKF";
    button.addEventListener("click", load);
    const requested = new URLSearchParams(root.location.search).get("setup");
    if (!button.hidden && requested === ID) {
      openPanel();
      Promise.resolve(root.__hopreachWasmReadyPromise).then(load);
    }
  }

  return { init, restore, validateSetup, provenanceText };
});
