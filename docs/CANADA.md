# Canadian coverage by IATA

This fork adds separately cached Canadian regional maps to HopReach. Start
with `config.canada.example.yaml`, set the CoreScope API URL and confirm the
radio model assumptions for your local network. YKF (Waterloo) is the default;
YYZ (Toronto) and the other configured Canadian IATA regions are selectable.
Links such as `/?iata=YYZ` retain the selection, including browser history.

## What belongs to a map

CoreScope's **observed IATA membership** selects the repeater directory with
`/api/nodes?role=repeater&region=YKF`. An airport is a map centre, not an inferred
assignment of a node to a region. The Canadian country polygon then excludes
foreign coordinates. A configured `radius_km` (200 in the example, maximum 250)
bounds admitted repeaters and terrain work around that centre. The UI and
`meta.json.region` disclose that limit: this is not a claim to include every
node a distant receiver ever observed under that IATA.

The catalogue is the intersection of the source's configured IATAs and
OurAirports records with `iso_country=CA`; do not substitute a `Y` prefix rule
(Canadian codes also include XCM and ZBM). Operators add/remove entries to
match their observation service. Only configured codes are accepted by the API.

Coverage colours are **predicted RF reach** using the existing terrain model,
which may naturally extend across a border. They are not CoreDrive RX reception
measurements and do not account for foliage/buildings. Observed neighbour links
are restricted to Canadian members of the selected map, but their counts use
the source's receivers as a whole because CoreScope's reach API does not isolate
its evidence by IATA. Frequency, power, antenna heights/gains and fade margin
remain visible model assumptions, not measured device configuration.

Complete packet observations cannot currently be isolated by IATA by the
CoreScope detail API. Regional packet replay therefore returns an explicit
unavailable response and the UI explains it; it never presents truncated
observations as a complete replay. Terrain planning, line of sight, placement
tools, plan sharing and synthetic flood simulation remain available.

## Runtime and resources

The existing pipeline runs configured regions sequentially under its existing
process lock. Each region owns `data/regions/<IATA>/` with its own cached raster,
metadata, freshness check and progress. A failed region does not prevent the
others from refreshing. No visitor request starts terrain computation.

The example keeps its daily cron and uses a one-hour restart guard. A 24-hour
minimum measured from the prior run can skip the next scheduled refresh after
normal timing drift or a midday install. Per-tier same-day checks still prevent
redundant raster computation.

`source.request_concurrency` caps the pipeline's reach batch and public mesh API
work across regions. Use 1 for a small CoreScope host. Public requests have a
bounded 32-request admission queue and use `corescope.request_timeout_seconds`.
Node directories are cached for five minutes, at most 5000 nodes per region.
The country polygon is parsed once per process and shared across adapters.
An empty region clears previously cached coverage metadata instead of showing
an old prediction with zero repeaters.

The example uses CPU standard coverage at 640 px and terrain zoom 10, with
calibrated/precision tiers requiring a GPU. If no calibrated tier can run,
the unused batch of upstream reach requests and calibration search is skipped.
On-click neighbour evidence remains available. These settings are configurable
in YAML; a future operator customizer can expose the same values without
changing the dataset contract. Run a representative region and measure peak
RSS/time before raising resolution, radius, range or enabled tiers.

## Country boundary and attribution

Mount the complete Canada ADM0 polygon at `/config/canada.geojson`. The pilot
uses geoBoundaries `CAN-ADM0-50000805`, representing 2020, sourced from
OpenStreetMap / Wambacher, under Open Data Commons Open Database License 1.0:

- [Pinned GeoJSON](https://github.com/wmgeolab/geoBoundaries/raw/9469f09/releaseData/gbOpen/CAN/ADM0/geoBoundaries-CAN-ADM0.geojson)
- SHA256: `e85d9ccd79bcba3af39dd8fc2539c9b9b5412518144a4f4e25e65401a6f64df0`
- [Boundary source](https://www.geoboundaries.org/), [OpenStreetMap licence](https://www.openstreetmap.org/copyright)
- [OurAirports public-domain airport catalogue](https://ourairports.com/data/)

The frontend retains basemap attribution, adds boundary/catalogue attribution,
and links both upstream HopReach and this fork's complete source and licence.
Changes here are maintained in the fork while upstream external contributions
remain closed. No CLA acceptance is implied.

## Verification

`go test ./...` and `go vet ./...` require Linux because the existing terrain
loader uses memory-mapped files. `npm test` runs deterministic unit/contracts.
`npx playwright test --config=playwright.regions.config.js` verifies the actual
regional selector and mesh client in a deterministic browser fixture, including
YKF/YYZ navigation, back/forward, invalid URLs, empty/error states and API
isolation. It does not replace full map/planner/simulator validation against the
deployed candidate and cached terrain output.

## Measured coverage from MeshMapper

Enable the `meshmapper` section and mount its integration-key file read-only.
`MESHMAPPER_API_KEY` is a server-only alternative; neither credential nor its
file path is rendered into browser configuration. Persist `/data/meshmapper`
outside the public document root. The [official Coverage API](https://wiki.meshmapper.net/coverage-api/)
is requested once before the sequential terrain batch, or by
`hopreach -sync-measured` under the same process lock. This command also respects
the saved cooldown: `-force` never bypasses upstream quotas.

This importer requires a **global schema 2 key**. Single/group/merged regional
responses are rejected explicitly because a merged grid cannot be assigned
faithfully to separate IATAs. The minimum global interval is 12 hours; 304s,
failures and process interruptions consume that interval. ETags and next-attempt
times persist before/after requests, and longer Retry-After/429 reset windows win.
The client uses gzip, a 10-minute deadline, 256 MiB wire and 4 GiB decoded caps.
There are no visitor-triggered upstream requests or automatic rapid retries.

The global document is parsed one region and cell at a time. Only configured
Canadian IATA cells whose centres pass the exact Canadian polygon and that
IATA's radius survive. Foreign sections are discarded; late JSON metadata is
supported. When a section identifies its IATA after the cells, each configured
IATA has an independent capped private candidate; unused candidates are removed
as soon as that section is identified. A different region cannot consume the
selected region's cap. At most 100,000 cells per IATA are retained, with an explicit partial
flag. Bounded private candidates are published only after the entire envelope
passes validation. Errors preserve last-good regional files. An omitted region
retains its old snapshot and is disclosed in the public refresh status.

The optional **Measured coverage · MeshMapper** checkbox draws source-provided
cell bounds on one canvas. A sparse latitude index culls outside the viewport;
over 6,000 visible cells prompts a zoom-in rather than drawing an arbitrary
subset. The browser fetches only the selected regional cache and safe refresh
status once per page. The checkbox state is retained in the URL. Freshness,
missing/error/stale state and partial caches are labelled with MeshMapper
attribution. These are historical measurements across **all reported radio
configurations**; global feeds do not support radio filtering. They are separate
from the terrain prediction's configured frequency, power and antenna model.

Country filtering uses an exact latitude-edge index preserving polygon holes
and unions. On a synthetic 36,000-edge polygon (Windows amd64, i5-4690K),
`go test ./internal/geo -bench BenchmarkPreparedBoundary -benchtime=200ms`
measured 51,796 ns/point for the full scan and 475.7 ns/point indexed, both with
zero query allocations. This measures containment only, not end-to-end import
throughput or the real country polygon. The stream regression imports a large
gzipped foreign region while bounding decoder reads and retaining no foreign
files; unit tests also cover last-good preservation, gzip, ETags, quota state,
Retry-After, null SNR, epoch milliseconds and invalid geometry. Browser fixtures
exercise the actual canvas module, attribution, separate layer toggle and IATA
switch without credentials or a live MeshMapper request.
