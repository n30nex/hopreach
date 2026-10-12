// Regional datasets are selected before any map/simulator module starts.
// Navigation deliberately creates a fresh page: old terrain, observed links
// and in-flight simulator state can never bleed into the next IATA region.
(function (root, factory) {
  if (typeof module === "object" && module.exports) module.exports = factory();
  else { root.HopReachRegions = factory(); root.HopReachRegions.init(root); }
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  function resolve(config, search) {
    const entries = config.regions && config.regions.entries;
    if (!Array.isArray(entries) || !entries.length) return { config, region: null };
    const requested = (new URLSearchParams(search).get("iata") || "").toUpperCase();
    const region = entries.find((r) => r.iata === requested) ||
      entries.find((r) => r.iata === config.regions.defaultIata);
    if (!region || !/^[A-Z]{3}$/.test(region.iata)) throw new Error("No valid default Canadian region is configured.");
    const dataBase = `data/regions/${region.iata}`;
    return {
      region, invalid: !!requested && requested !== region.iata,
      config: Object.assign({}, config, {
        iata: region.iata, dataBase,
        dataUrl: `${dataBase}/repeaters.geojson`, metaUrl: `${dataBase}/meta.json`,
        meshApiBase: `/mesh-api/regions/${region.iata}/api`,
        mapCenter: [region.centerLat, region.centerLon], mapZoom: region.zoom,
      }),
    };
  }

  function regionURL(href, iata, entries) {
    if (!entries.some((r) => r.iata === iata)) throw new Error("Unknown Canadian IATA region");
    const url = new URL(href);
    url.searchParams.set("iata", iata);
    // A shared plan is tied to the view it was shared from.
    url.searchParams.delete("plan");
    url.searchParams.delete("setup");
    return url.href;
  }

  function dataMessage(region, state, count) {
    if (state === "error") return `${region.iata} data is unavailable. Try again later or select another region.`;
    if (state === "ready" && count === 0) return `No located Canadian repeaters are currently available for ${region.iata} within ${region.radiusKm} km. This does not prove there is no coverage.`;
    if (state === "loading") return `Loading the cached ${region.iata} map…`;
    return `${region.iata} · Canadian repeaters observed in this IATA region, within ${region.radiusKm} km of its airport. Coverage is a terrain prediction; neighbour links are observed evidence across the source's receivers.`;
  }

  function init(root) {
    const result = resolve(root.HOPREACH_CONFIG, root.location.search);
    root.HOPREACH_CONFIG = result.config;
    if (!result.region) return;
    const doc = root.document;
    doc.documentElement.classList.add("canadaverse");
    doc.title = `${result.config.siteName} · ${result.region.iata}`;
    const select = doc.getElementById("iata-select");
    const entries = result.config.regions.entries;
    for (const region of entries) {
      const option = doc.createElement("option");
      option.value = region.iata; option.textContent = `${region.iata} — ${region.name}`;
      select.appendChild(option);
    }
    select.value = result.region.iata;
    // Browsers restore a select's last value from their back/forward cache.
    // Before navigation that value was the NEXT region, not this page's data.
    root.addEventListener("pageshow", () => { select.value = result.region.iata; });
    select.addEventListener("change", () => root.location.assign(regionURL(root.location.href, select.value, entries)));
    doc.getElementById("regional-controls").hidden = false;
    doc.getElementById("regional-footer").hidden = false;
    doc.getElementById("canadaverse-brand").hidden = false;
    const status = doc.getElementById("regional-status");
    status.textContent = result.invalid ? `That IATA is not configured. Showing ${result.region.iata}.` : dataMessage(result.region, "loading");
    if (result.invalid) {
      doc.getElementById("regional-warning").textContent = status.textContent;
      doc.getElementById("regional-warning").hidden = false;
    }
    if (!new URLSearchParams(root.location.search).has("iata") || result.invalid) {
      const url = new URL(root.location.href); url.searchParams.set("iata", result.region.iata);
      root.history.replaceState(null, "", url.href);
    }
    const replay = doc.getElementById("sim-promoted-replay");
    for (const element of replay.querySelectorAll(".plan-row, .plan-field-row, button")) element.classList.add("hidden");
    replay.querySelector(".sim-promoted-title").textContent = "Regional packet replay is unavailable";
    replay.querySelector(".sim-promoted-body").textContent = "The observation source cannot isolate complete packet observations by IATA. Terrain planning and synthetic flood simulation remain available.";
  }

  function showDataState(state, count) {
    if (typeof window === "undefined" || !window.HOPREACH_CONFIG.iata) return;
    const c = window.HOPREACH_CONFIG;
    const region = c.regions.entries.find((r) => r.iata === c.iata);
    document.getElementById("regional-status").textContent = dataMessage(region, state, count);
  }

  return { resolve, regionURL, dataMessage, init, showDataState };
});
