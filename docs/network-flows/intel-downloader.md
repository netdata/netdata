<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/intel-downloader.md"
sidebar_label: "Enrichment Intel Downloader"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['ip intelligence', 'mmdb', 'downloader', 'db-ip', 'iptoasn', 'topology-ip-intel-downloader', 'enrichment', 'refresh']
endmeta-->

<!-- markdownlint-disable-file -->

# Enrichment Intel Downloader

`topology-ip-intel-downloader` is a small Netdata-supplied tool that keeps the IP intelligence MMDB databases used by the netflow plugin (and the topology subsystem) up to date. It fetches the upstream payloads, normalises them into a fixed Netdata MMDB layout, applies CIDR classification policy, and atomically replaces the files on disk. The netflow plugin's resolver picks up the new files within 30 seconds — no plugin restart required.

Packaged 32-bit installs ship the stock MMDB payload but do not include the downloader binary. Source builds from a Git checkout also do not include the generated stock MMDB payload by default.

The downloader is a separate executable so you can run it on whatever schedule fits your environment without coupling it to the agent's lifecycle.

## What it does

- Fetches the configured ASN and Geo source files over HTTPS, with gzip / zip transparently decoded.
- Parses the upstream format (MMDB or TSV/CSV), keeping the first source per family that covers a given range — first-source-wins on overlap.
- Re-emits the data as two Netdata-format MMDB files plus a metadata JSON manifest.
- Stamps Netdata classification metadata (`netdata.ip_class`, `netdata.track_individual`) over `localhost_cidrs`, `private_cidrs`, and any operator-defined `interesting_cidrs` so the plugin can identify private/loopback/operator-flagged ranges via a normal MMDB lookup.
- Publishes each output atomically via stage-then-`rename(2)` — the resolver never sees a torn file.

The output is always the same fixed file set, regardless of which providers fed the run:

```
/var/cache/netdata/topology-ip-intel/
├── topology-ip-asn.mmdb     # ASN database
├── topology-ip-geo.mmdb     # Geographic database
└── topology-ip-intel.json   # Manifest: when, from where, how many ranges
```

The directory and filenames match the shipped defaults.

## Supported sources

The tool only knows how to talk to a fixed set of providers — anything else is rejected at validation:

| Provider:Artifact | Family | Format | Origin |
|---|---|---|---|
| `dbip:asn-lite` | ASN | `mmdb` (default) or `csv` | DB-IP free monthly download page |
| `dbip:country-lite` | Geo | `mmdb` (default) or `csv` | DB-IP free monthly download page |
| `dbip:city-lite` | Geo | `mmdb` (default) or `csv` | DB-IP free monthly download page |
| `iptoasn:combined` | ASN or Geo | `tsv` | `https://iptoasn.com/data/ip2asn-combined.tsv.gz` (direct URL) |
| `caida:prefix2as` | ASN | `tsv` | CAIDA RouteViews prefix-to-AS creation log |
| `maxmind:geolite2-asn` | ASN | `mmdb` | MaxMind authenticated GeoLite2 download |
| `maxmind:geolite2-country` | Geo | `csv` | MaxMind authenticated GeoLite2 Country CSV ZIP download |
| `ip2location:country-lite` | Geo | `csv` | IP2Location Lite country CSV ZIP download |
| `ipdeny:country-zones` | Geo | `cidr` | IPDeny country zone archive |
| `ipip:country` | Geo | `txt` | IPIP country text ZIP download |

DB-IP artifacts are resolved from the current monthly URL on the DB-IP landing page (`https://db-ip.com/db/download/<artifact>`). The downloaded URL uses the DB-IP free database pattern `https://download.db-ip.com/free/dbip-<artifact>-YYYY-MM.<ext>.gz`.

The IPtoASN TSV feed is converted into the same Netdata MMDB layout as the DB-IP feeds, so consumers don't care which source produced the file.

CAIDA prefix2as is ASN-only and has no AS organization names. The downloader resolves the latest `.pfx2as.gz` entry from CAIDA's creation log before fetching it.

MaxMind built-in sources require `MAXMIND_LICENSE_KEY` in the downloader environment. `maxmind:geolite2-asn@mmdb` downloads the official GeoLite2 ASN tarball and extracts the MMDB member. `maxmind:geolite2-country@csv` downloads the official GeoLite2 Country CSV **ZIP bundle** and needs the locations file plus the IPv4/IPv6 block CSVs inside that bundle; `csv` here does not mean a single raw CSV file.

IP2Location `country-lite@csv` is also the provider's official CSV ZIP bundle. IPDeny `country-zones@cidr` is the `all-zones.tar.gz` archive, and IPIP `country@txt` is the country text ZIP.

You can still pull *any* MMDB build (including a custom one) into the resolver by configuring `enrichment.geoip.asn_database` / `geo_database` directly — the downloader is one of several producers; the plugin doesn't care who wrote the MMDB. See the [Custom MMDB Database](/src/crates/netflow-plugin/integrations/custom_mmdb_database.md) card. If you prefer MaxMind's own updater, run [`geoipupdate`](/src/crates/netflow-plugin/integrations/maxmind_geoip_-_geolite2.md) and point `enrichment.geoip.asn_database` / `enrichment.geoip.geo_database` at the MMDB files it produces.

## Configuration file

The downloader reads YAML config from the first existing file in this order:

1. `/etc/netdata/topology-ip-intel.yaml` (operator overrides)
2. `/usr/lib/netdata/conf.d/topology-ip-intel.yaml` (stock, shipped by the package)

If neither exists, the built-in defaults are used. Pass `--config /path/to/file.yaml` to force a specific path.

The shipped stock file is:

```yaml
sources:
  - name: dbip-asn
    family: asn
    provider: dbip
    artifact: asn-lite
    format: mmdb

  - name: dbip-geo
    family: geo
    provider: dbip
    artifact: city-lite
    format: mmdb

output:
  directory: /var/cache/netdata/topology-ip-intel
  asn_file: topology-ip-asn.mmdb
  geo_file: topology-ip-geo.mmdb
  metadata_file: topology-ip-intel.json

policy:
  localhost_cidrs:
    - 127.0.0.0/8
    - ::1/128
  private_cidrs:
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16
    - 100.64.0.0/10
    - fc00::/7
    - fe80::/10
  interesting_cidrs: []

http:
  timeout: 2m
  user_agent: netdata-topology-ip-intel-downloader/1.0
```

| Key | Notes |
|---|---|
| `sources[]` | Ordered list per family. Each entry needs `family` (`asn` or `geo`), `provider`, `artifact`. `format` is inferred from the provider/artifact when omitted. Optional `url` overrides the built-in URL; optional `path` reads from a local file instead. Earlier entries win on overlap. |
| `output.directory` | Where the MMDB and metadata files land. Must match what the netflow plugin reads (see below). |
| `output.asn_file` / `output.geo_file` / `output.metadata_file` | File names only — paths are rejected by validation. |
| `policy.localhost_cidrs` / `private_cidrs` | Stamped into both MMDBs as `netdata.ip_class = "localhost"` / `"private"`. |
| `policy.interesting_cidrs` | Operator-defined public ranges to track individually. Stamped as `netdata.ip_class = "interesting"`. |
| `http.timeout` | Per-request timeout. Default `2m`. |
| `http.user_agent` | Sent to upstream providers. Default `netdata-topology-ip-intel-downloader/1.0`. |

CLI flags can override the config without editing the file:

| Flag | Purpose |
|---|---|
| `--config PATH` | Force a specific YAML config path. |
| `--output-dir DIR` | Override `output.directory`. |
| `--asn provider:artifact[@format]` | Replace the ASN source list. Repeatable; first wins. |
| `--geo provider:artifact[@format]` | Replace the Geo source list. Repeatable; first wins. |
| `--no-asn` | Disable ASN output and delete any stale `topology-ip-asn.mmdb`. |
| `--no-geo` | Disable Geo output and delete any stale `topology-ip-geo.mmdb`. |

## Scheduled execution

**Netdata does not ship a systemd timer or cron entry for the downloader.** This is intentional — the appropriate refresh cadence depends on the provider's update cadence, your bandwidth, and your change-control policy, and a packaged timer would force one choice on every install.

Set up your own. A simple systemd timer is the recommended pattern:

```ini
# /etc/systemd/system/netdata-topology-ip-intel.service
[Unit]
Description=Refresh Netdata IP intelligence databases

[Service]
Type=oneshot
ExecStart=/usr/sbin/topology-ip-intel-downloader
User=netdata
Group=netdata
```

```ini
# /etc/systemd/system/netdata-topology-ip-intel.timer
[Unit]
Description=Weekly refresh of Netdata IP intelligence databases

[Timer]
OnCalendar=weekly
RandomizedDelaySec=1h
Persistent=true

[Install]
WantedBy=timers.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now netdata-topology-ip-intel.timer
```

Refresh cadence depends on the sources you enable. DB-IP refreshes its free Lite databases monthly; weekly is a safe over-poll that picks up every release within a few days while staying polite to the upstream. IPtoASN refreshes hourly, but downstream consumers rarely need that resolution — daily is plenty if you switch to it. CAIDA prefix2as, MaxMind, IP2Location, IPDeny, and IPIP have their own publication schedules and terms; choose a timer cadence that is polite to the upstream and fast enough for your environment.

Run the packaged binary as the `netdata` user (or root) so it can write to `/var/cache/netdata/topology-ip-intel/`.

## Manual invocation

Trigger an out-of-schedule refresh:

```bash
sudo systemctl start netdata-topology-ip-intel.service   # if you set up the unit above
```

Or invoke the binary directly — it loads the same config, prints the execution plan, and writes to the same destination:

```bash
sudo -u netdata /usr/sbin/topology-ip-intel-downloader
```

A successful run finishes in well under a minute on a typical link and prints something like:

```
effective source plan:
ASN sources (first wins):
- 1. dbip:asn-lite@mmdb
GEO sources (first wins):
- 1. dbip:city-lite@mmdb
output actions:
- write topology-ip-asn.mmdb
- write topology-ip-geo.mmdb
- write topology-ip-intel.json
updated IP intelligence databases using config /usr/lib/netdata/conf.d/topology-ip-intel.yaml
asn_mmdb=/var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb
geo_mmdb=/var/cache/netdata/topology-ip-intel/topology-ip-geo.mmdb
metadata=/var/cache/netdata/topology-ip-intel/topology-ip-intel.json
asn_ranges=1234567 geo_ranges=8901234
```

The plan is printed *before* any download, so you can verify the effective source list without committing to a fetch.

## Output and atomic replacement

Atomic publication is the contract this tool provides to the netflow plugin's resolver:

1. A staging directory is created inside `output.directory` (`.tmp-topology-ip-intel-stage-*`) and removed on exit.
2. The MMDB writer streams into a per-file temp inside that staging directory.
3. Each finished MMDB is fsync-closed, chmodded `0644`, and renamed into its final name.
4. The metadata JSON is renamed last, so a partially-completed run never updates the manifest.

Because `rename(2)` is atomic on the same filesystem, a reader that opens the file at any moment sees either the old complete file or the new complete file — never a half-written one. The netflow plugin's resolver re-stats and re-opens the MMDBs every 30 seconds, so a fresh download is live within at most 30 seconds of completion. No plugin restart, no agent restart.

## Failure modes

The tool exits non-zero with a diagnostic on `stderr` for any of these cases:

| Failure | Behaviour |
|---|---|
| Config syntax / validation error | Exits before any network activity. Existing MMDBs are untouched. |
| Upstream unreachable / non-200 status | The run aborts before any output is staged. Existing MMDBs are untouched. |
| Decompression / parse error | Same as above — abort before publishing. |
| Disk full / rename failure during publish | The staging directory is cleaned up; the previously-published file remains in place. |

Net result: **a failed run keeps the previously good databases**. The plugin keeps serving stale-but-correct enrichment until the next successful run replaces them. There is no built-in retry — schedule the timer often enough that a single missed run isn't critical.

If you run the downloader from a systemd timer, the failure is visible via `systemctl status netdata-topology-ip-intel.service` and `journalctl -u netdata-topology-ip-intel`. There is no log file written by the tool itself; it only writes to stdout/stderr.

## Integration with the netflow plugin's auto-detect

The netflow plugin auto-discovers MMDB files at startup when neither `enrichment.geoip.asn_database` nor `enrichment.geoip.geo_database` is set. The lookup order is:

1. `<cache_dir>/topology-ip-intel/topology-ip-asn.mmdb` and `topology-ip-geo.mmdb` — the directory the downloader writes to.
2. `<stock_data_dir>/topology-ip-intel/...` — the package-shipped stock payload (typically `/usr/share/netdata/topology-ip-intel/`), used as fallback when no fresh copy exists yet.

`<cache_dir>` defaults to `/var/cache/netdata`; `<stock_data_dir>` defaults to `/usr/share/netdata`. The downloader's default output directory matches the cache path the plugin checks first, so a fresh run automatically supersedes the stock payload.

When the plugin auto-detects MMDBs this way it forces `optional: true` on the geoip stanza — a missing or transiently-unreadable file does not crash the plugin. If you instead set `asn_database` / `geo_database` explicitly in `netflow.yaml`, you control the `optional` flag yourself; see [Configuration](/docs/network-flows/configuration.md#enrichment).

## What's next

- Per-provider details (refresh cadence, license, schema, attribution requirements):
  - [DB-IP IP Intelligence](/src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md) — the default the downloader fetches.
  - [IPtoASN](/src/crates/netflow-plugin/integrations/iptoasn.md) — public-domain TSV feed; converted to MMDB by this tool.
  - [CAIDA RouteViews Prefix-to-AS](/src/crates/netflow-plugin/integrations/caida_routeviews_prefix-to-as.md) — prefix-to-AS TSV feed; ASN-only.
  - [MaxMind GeoIP / GeoLite2](/src/crates/netflow-plugin/integrations/maxmind_geoip_-_geolite2.md) — authenticated MaxMind downloads or MMDB files managed by `geoipupdate`.
  - [IP2Location LITE IP-Country](/src/crates/netflow-plugin/integrations/ip2location_lite_ip-country.md) — public country-only CSV ZIP feed.
  - [IPDeny Country Zones](/src/crates/netflow-plugin/integrations/ipdeny_country_zones.md) — country CIDR archive.
  - [IPIP Country Database](/src/crates/netflow-plugin/integrations/ipip_country_database.md) — country text ZIP feed.
  - [Custom MMDB Database](/src/crates/netflow-plugin/integrations/custom_mmdb_database.md) — your own MMDB build.
- The enrichment mechanism that consumes these files: [Enrichment](/docs/network-flows/enrichment.md) (the MMDB shared mechanism section).
- The plugin knobs that point at the files: [Configuration › `enrichment.geoip`](/docs/network-flows/configuration.md#enrichment).
