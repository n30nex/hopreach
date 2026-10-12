# YKF metro simulation setup

On a Canadian deployment, choose **YKF → Sim → Saved setups → Load YKF metro
setup**, or open `/?iata=YKF&setup=ykf-metro`. Loading creates an unsaved copy;
**Save** stores your copy in this browser. Nothing transmits over MQTT or radio.

The setup contains actual repeater positions and known-neighbour adjacency from
the public `live.meshcore.ca` Beacon catalogue and `dev.canadaverse.org`
CoreScope catalogue/graph. Full public keys identify nodes. The processing
extent is latitude 43.30–43.64, longitude -80.63–-80.18, covering Cambridge,
Guelph, Kitchener and Waterloo. This rectangle is not an official boundary.

The APIs do not establish reliable pairwise SNR or direction. Each observed
pair becomes two **assumed** simulation directions with **0 dB assumed SNR**.
Missing links mean unknown connectivity. Stale and unknown-age nodes remain
included; fetch dates do not date individual links or positions. Both source
counts and detailed assumptions remain attached to saved/exported copies.

Four connected real repeaters nearest the city centres originate synthetic
traffic: three messages each over a 60-second run, with five trials selected
for the optional **Predict settings** action.
Radio settings use the existing USA/Canada preset. Device policies, traffic,
power and relay timing are simulation assumptions. Review/edit the settings
before drawing conclusions about a real deployment.

## Publish or refresh the snapshot

This is a dated snapshot, not a continuously updated live graph. An operator
refreshes it explicitly using the existing Node.js development runtime:

```sh
node tools/build-ykf-setup.mjs /path/outside-git/sim-setup.json
npm test
```

The destination directory must already exist outside every Git checkout. The
tool makes three bounded, sequential public requests without credentials or
per-node requests. Failed, paginated, inconsistent or oversized responses leave
the previous file untouched. The current safety limit is 64 local nodes and
4096 directed links; exceeding it fails rather than silently dropping data.

Publish the resulting JSON as `data/regions/YKF/sim-setup.json` in the same
persistent public-data directory as the regional maps. Keep a recovery copy of
an existing snapshot and use an atomic replacement. Real node names, keys and
positions must not be committed; the repository tests use fictional fixtures.
No new server configuration or customizer is required. The existing simulation
controls allow editing; export/import also preserves individual link settings.

Loading in the browser makes one bounded same-origin request. It does not
refresh upstream sources or regenerate coverage. The saved-setup and region
navigation controls clear the preset bookmark when switching away from it.
