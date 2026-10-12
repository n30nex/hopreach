# Deterministic container E2E

Run the same Dockerfile, Go API, coverage pipeline, nginx, WASM planner, and
simulator used by a deployment. Only the CoreScope API and elevation input are
synthetic. Six fictional Ontario repeaters provide bidirectional reach, scoped
and unscoped floods, resolved paths, and multiple packet observations. The PNG
terrain is uniformly 100 m. No captured packets or private identifiers are used.

```sh
python3 -B -m unittest discover -s tests/offline-fixtures -p test_server.py
docker compose -f tests/offline-fixtures/compose.yml build
npx playwright test
docker compose -f tests/offline-fixtures/compose.yml down --volumes --remove-orphans
```

The stack has its own Compose project/network and no production volumes or
credentials. Its CoreScope URL and both server/browser DEM paths point only to
the fixture, checked by the fixture tests. The published port binds to loopback.
The network permits outbound traffic. The existing page loads pinned Leaflet
assets and map basemaps from public CDNs; they are not sources of mesh data.

`/e2e-ready` waits for all four real coverage tiers and both scope overlays.
Tiny 48/64-pixel CPU rasters keep the complete pipeline inexpensive. Timestamps
are anchored once when the fixture starts, so paging is stable and the replay
window remains recent. Restart the fixture when testing more than six hours
after its startup.

Every existing `tests/*.spec.js` stays in the default suite, including the four
regional controls tests. The replay scenarios use the synthetic unregional API;
regional mode deliberately disables replay until regional packet isolation is
supported. Any skipped test fails the offline suite rather than passing quietly.

To reuse this stack on another local port, set `HOPREACH_E2E_PORT`.
`HOPREACH_E2E_BASE_URL` overrides its browser URL (for example, a forwarded stack).
`HOPREACH_E2E_COMPOSE` selects a different isolated test Compose file.
To test an already-started fixture stack, wait for
`/e2e-ready`, then set `HOPREACH_E2E_EXTERNAL_SERVER=1`. CI always builds and
starts a fresh stack. These settings do not change the production defaults.
