# Pinned Faro browser assets

DEM embeds the exact published `@grafana/faro-web-sdk` and
`@grafana/faro-web-tracing` 2.11.0 IIFE files. Agent builds and runtime require
neither npm nor network access. Do not reformat, minify, patch, or rebuild these
files. Their SHA-256 values identify their public URLs and strong ETags.

`manifest.json` records published package archive integrity, bundle and source-map
digests, source-map input paths, the upstream source revision and lockfile, and
license provenance. The source maps are audit inputs; they are not served or
embedded. The upstream `sourceMappingURL` comments are preserved with all other
published bytes; a developer-tools request for those maps returns 404.

`NOTICES.txt` contains the applicable upstream license texts and attributions. It
is embedded and served at a digest-addressed URL; each asset response links it
with `rel="license"`. This keeps notices available with the installed binary.

## Verification

Run the following commands from the repository root with Python 3.
The default check is offline. The explicit `--upstream` option reads public
archives and the pinned lockfile, verifies integrity, compares the published
bundle bytes and source inventories, and checks that the notice text includes
each audited upstream license. It never extracts archives to disk, installs
packages, or runs package scripts.

```sh
python3 src/go/plugin/dem/rum/faro/assets/verify.py
python3 src/go/plugin/dem/rum/faro/assets/verify.py --upstream
cd src/go
go test ./plugin/dem/rum/faro ./plugin/dem/rum/httpapi
```

## Updating

An SDK update requires review of browser behavior and bundled dependencies.
Download the selected published packages, verify their registry integrity,
and copy only the exact IIFE bytes. Inspect both source maps and the matching
upstream revision's lockfile to identify the actual bundled packages and versions;
declared dependency ranges alone are insufficient. Inspect all bundled package
licenses, nested notices and copied-code attributions, including generated helpers.

For this version, the core bundle includes Faro core, UAParser.js, Web Vitals,
and OXC runtime helpers. Tracing includes Faro tracing and the OpenTelemetry
packages listed in the manifest. The OXC runtime README attributes its helpers
to Babel. OpenTelemetry instrumentation distributes a separate BSD license for
copied shimmer code, and OpenTelemetry core's `utils/lodash.merge.js` identifies
its lodash-derived code as MIT. Supplemental Babel and lodash license sources
record attribution provenance; they do not assert that those complete package
versions are bundled. The matching source maps establish what is bundled.

Refresh the manifest and consolidated notices, update the SHA-256 constants in
`../assets.go` (and `Version` in `../bootstrap.go` for a version change), and run
both verification modes and the Go/browser tests. Any byte change MUST get a new
asset URL. Do not overwrite an existing public asset identity with new bytes.
All receiver routes serving a bootstrap MUST serve the assets it references;
coordinate routing and release deployment across replicas. No historical asset
compatibility store or runtime download fallback is provided.
