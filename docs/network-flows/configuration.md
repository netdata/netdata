<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/configuration.md"
sidebar_label: "Configuration"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['configuration', 'netflow.yaml', 'tuning', 'retention', 'listener']
endmeta-->

<!-- markdownlint-disable-file -->

# Configuration

The netflow plugin reads its configuration from `netflow.yaml`. The defaults are good for initial validation; production deployments usually tune the listener address and retention once the observed flow rate is known. This page documents every option, with its real default and the file that defines it.

## Where the file lives

| Path | Purpose |
|---|---|
| `/etc/netdata/netflow.yaml` | Your configuration. Edits here survive package upgrades. |
| `/usr/lib/netdata/conf.d/netflow.yaml` | The stock file shipped with the package. Reference only. |

The plugin reads the user file when it exists, and the stock file otherwise. To start customising, copy the stock file:

```bash
sudo cp /usr/lib/netdata/conf.d/netflow.yaml /etc/netdata/netflow.yaml
```

## Three things to know before you edit

1. **Restart required.** There is no live-reload for plugin configuration. After saving the file, run `sudo systemctl restart netdata`. Only the GeoIP databases reload on a timer; everything else needs a restart.
2. **Strict YAML.** Every section refuses unknown keys. A misspelled key fails the plugin at startup with an error in the journal. If you see "the plugin won't start after my edit", check for a typo before anything else.
3. **CLI flags vs YAML.** When the plugin runs as a Netdata Agent plugin (the normal case), only the YAML is read — CLI flags do nothing. The CLI flags shown below apply only if you run the binary directly outside of Netdata.

## Top-level layout

```yaml
enabled: true               # global on/off
listener: { ... }           # UDP socket and journal sync
protocols: { ... }          # which protocols to accept; decapsulation; timestamps
journal: { ... }            # tier directories, retention, query guardrails
enrichment: { ... }         # GeoIP, classifiers, ASN, BMP, BioRIS, network sources
```

The YAML form is strictly nested — every key lives inside its section as shown above. The flat form is only valid for CLI flags (e.g. `--netflow-listen 0.0.0.0:2055`), which the plugin exposes for one-off invocation; the YAML schema rejects unknown top-level keys.

## `enabled`

```yaml
enabled: true
```

Set to `false` to turn the entire flow plugin off. The plugin still loads but does nothing. Default: `true`.

## `listener`

Controls the UDP socket and the journal write cadence.

```yaml
listener:
  listen: "0.0.0.0:2055"
  max_packet_size: 9216
  sync_every_entries: 1024
  sync_interval: "1s"
```

| Key | CLI flag | Default | Notes |
|---|---|---|---|
| `listen` | `--netflow-listen` | `0.0.0.0:2055` | Address and port for the UDP socket. Same socket handles NetFlow v5/v7/v9, IPFIX, and sFlow. |
| `max_packet_size` | `--netflow-max-packet-size` | `9216` | Maximum UDP datagram in bytes. Increase for jumbo sFlow datagrams or routers that send oversized IPFIX. |
| `sync_every_entries` | `--netflow-sync-every-entries` | `1024` | Flush the raw journal to disk after this many records, regardless of `sync_interval`. |
| `sync_interval` | `--netflow-sync-interval` | `1s` | Maximum time between forced flushes. |

### UDP buffer tuning is not in this file

If you receive a high flow rate, the kernel UDP receive buffer matters more than `max_packet_size`. Tune at the kernel level:

```bash
sudo sysctl -w net.core.rmem_max=33554432
sudo sysctl -w net.core.rmem_default=8388608
sudo sysctl -w net.core.netdev_max_backlog=250000
```

Persist these in `/etc/sysctl.d/99-netflow.conf`. The plugin does not call `setsockopt(SO_RCVBUF)` itself; whatever the kernel default is, that's what the listener gets.

## `protocols`

```yaml
protocols:
  v5: true
  v7: true
  v9: true
  ipfix: true
  sflow: true
  decapsulation_mode: none
  timestamp_source: input
```

| Key | CLI flag | Default | Values |
|---|---|---|---|
| `v5` | `--netflow-enable-v5` | `true` | Boolean. NetFlow v5. |
| `v7` | `--netflow-enable-v7` | `true` | Boolean. NetFlow v7 (Catalyst). |
| `v9` | `--netflow-enable-v9` | `true` | Boolean. NetFlow v9. |
| `ipfix` | `--netflow-enable-ipfix` | `true` | Boolean. IPFIX. |
| `sflow` | `--netflow-enable-sflow` | `true` | Boolean. sFlow v5. |
| `decapsulation_mode` | `--netflow-decapsulation-mode` | `none` | `none`, `srv6`, `vxlan`. Strips outer headers from the data-link section, surfaces the inner 5-tuple. |
| `timestamp_source` | `--netflow-timestamp-source` | `input` | Which timestamp is stored as `_SOURCE_REALTIME_TIMESTAMP`. See below. |

You must keep at least one protocol enabled or the plugin refuses to start.

### `timestamp_source` values

- **`input`** (default) — the time the plugin received the datagram.
- **`netflow_packet`** — the time the exporter put in the NetFlow/IPFIX header.
- **`netflow_first_switched`** — the time the flow actually started, from the per-record first-switched field when the exporter provides it.

The Network Flows view still uses journal entry time, which is the time the Netdata Agent received the datagram, for query windows and tier selection. `timestamp_source` controls the stored source timestamp metadata; it does not make the dashboard time picker query by exporter timestamps.

## `journal`

This is the section most operators tune. It controls where flow data lives, how much of it lives, and how the query engine guardrails its work.

```yaml
journal:
  journal_dir: flows
  query_max_groups: 50000
  tiers:
    raw:      { size_of_journal_files: 50GB, duration_of_journal_files: 24h }
    minute_1: { size_of_journal_files: 5GB,  duration_of_journal_files: 14d }
    minute_5: { size_of_journal_files: 5GB,  duration_of_journal_files: 30d }
    hour_1:   { size_of_journal_files: 5GB,  duration_of_journal_files: 365d }
```

### Journal directory

| Key | Default | Notes |
|---|---|---|
| `journal_dir` | `flows` | Relative paths resolve under `NETDATA_CACHE_DIR` (typically `/var/cache/netdata/flows`). Absolute paths are used as-is. |

### Per-tier retention

Each tier has its own size and duration budget, configured under `tiers:` only — YAML has no global retention knobs. Raw and rollup tiers have very different storage and access patterns; they should be sized independently.

```yaml
tiers:
  raw:
    size_of_journal_files: 50GB
    duration_of_journal_files: 24h
  minute_1:
    size_of_journal_files: 5GB
    duration_of_journal_files: 14d
  minute_5:
    size_of_journal_files: 5GB
    duration_of_journal_files: 30d
  hour_1:
    size_of_journal_files: 5GB
    duration_of_journal_files: 365d
```

| YAML name | Aliases | On-disk directory |
|---|---|---|
| `raw` | — | `flows/raw/` |
| `minute_1` | `1m`, `minute-1`, `minute1` | `flows/1m/` |
| `minute_5` | `5m`, `minute-5`, `minute5` | `flows/5m/` |
| `hour_1` | `1h`, `hour-1`, `hour1` | `flows/1h/` |

The on-disk directory names are short (`1m`, `5m`, `1h`); the YAML keys are explicit (`minute_1`, `minute_5`, `hour_1`). Mind the difference if you go look at the disk.

Per-tier values:

| Key | Default per tier | Notes |
|---|---|---|
| `size_of_journal_files` | `10GB` | Disk budget for this tier. Minimum `100MB`. Set to `null` to disable size-based retention on this tier. |
| `duration_of_journal_files` | `7d` | Time budget for this tier. Set to `null` to disable time-based retention on this tier. |

Either limit triggers rotation. The tier expires whichever is hit first. At least one of the two must be set per tier (validation enforces this).

If you omit a tier entry entirely, that tier uses the built-in defaults (`10GB` / `7d`). If you provide a tier entry but omit one of the two knobs, the omitted knob falls back to its built-in default. Setting either to `null` explicitly disables that limit on that tier.

Standalone CLI runs still accept the legacy uniform retention flags:
`--netflow-retention-size-of-journal-files` and
`--netflow-retention-duration-of-journal-files`. They apply the same value to
all tiers and exist only for standalone/CLI compatibility; production
configuration should use the per-tier YAML shape above.

The example block at the top of this section is a typical production profile: 24 hours of raw, 2 weeks at 1-minute, 30 days at 5-minute, 1 year at 1-hour. Detailed forensics for the last day; long-term trends for the year.

### Rotation

Each tier rotates files at `size_of_journal_files / 20`, clamped between 5 MB and 200 MB. Time-based rotation is fixed at one hour per file. You don't configure these directly. If `size_of_journal_files` is set to `null` on a tier (size-based retention disabled), the rotation size falls back to 100 MB so files still rotate cleanly.

### Query guardrails

| Key | Default | What it limits |
|---|---|---|
| `query_max_groups` (alias: `query-max-groups`) | `50000` | Maximum number of distinct group keys a single aggregation query can build. When exceeded, additional groups are folded into a synthetic `__overflow__` bucket and the response carries a warning. Protects the query worker from memory blow-up on accidentally wide group-by combinations. |

The tier the planner uses for a given query is decided automatically from the time window and the query view (Sankey / time-series / map / etc.) — the planner aligns to the coarser tier when the window allows, and falls back to a finer tier for the unaligned head/tail. There are no separate "max window per tier" knobs.

## `enrichment`

Enrichment is a large topic and lives in dedicated pages. The top-level enable/disable knobs:

```yaml
enrichment:
  # default_sampling_rate: 1024                          # single rate, or per-prefix map
  # default_sampling_rate: { 10.1.0.0/16: 1024 }         # per-prefix form is also valid
  # override_sampling_rate: { 10.1.0.0/16: 1024 }        # per-prefix override map
  default_sampling_rate: ~
  override_sampling_rate: ~
  metadata_static: { exporters: {} }
  geoip: { asn_database: [], geo_database: [], optional: false }
  networks: {}
  network_sources: {}
  exporter_classifiers: []
  interface_classifiers: []
  classifier_cache_duration: 5m
  asn_providers: [flow, routing, geoip]
  net_providers: [flow, routing]
  routing_static: { prefixes: {} }
  routing_dynamic:
    bmp: { enabled: false }
    bioris: { enabled: false }
```

A note on `default_sampling_rate` vs. `override_sampling_rate`: both keys accept either a single integer (applied to every record from every exporter) or a per-prefix map. The intended split is "default" for `rate=1` records that lack a sampling rate, and "override" for replacing a known-wrong rate; the schema does not enforce that intent — either knob can take either form.

`enrichment.geoip.optional` (default `false`) decides what happens when an MMDB file declared in `asn_database` / `geo_database` is missing at startup: `false` aborts the plugin, `true` logs a warning and continues without that database.

For the cross-cutting picture — order of evaluation, the `asn_providers` and `net_providers` chains, the MMDB shared mechanism, the static-vs-dynamic composition rules — see the [Enrichment](/docs/network-flows/enrichment.md) page. Per-method configuration details (URLs, refresh cadence, license, vendor commands) live on the integration cards under flows.enrichment-methods:

- IP intelligence (MMDB): [DB-IP](/src/crates/netflow-plugin/integrations/db-ip_ip_intelligence.md), [MaxMind GeoIP / GeoLite2](/src/crates/netflow-plugin/integrations/maxmind_geoip_-_geolite2.md), [IPtoASN](/src/crates/netflow-plugin/integrations/iptoasn.md), [Custom MMDB](/src/crates/netflow-plugin/integrations/custom_mmdb_database.md).
- BGP routing: [BMP](/src/crates/netflow-plugin/integrations/bmp_bgp_monitoring_protocol.md), [bio-rd RIS](/src/crates/netflow-plugin/integrations/bio-rd_-_ripe_ris.md).
- Network sources: [AWS IP Ranges](/src/crates/netflow-plugin/integrations/aws_ip_ranges.md), [Azure IP Ranges](/src/crates/netflow-plugin/integrations/azure_ip_ranges.md), [GCP IP Ranges](/src/crates/netflow-plugin/integrations/gcp_ip_ranges.md), [NetBox](/src/crates/netflow-plugin/integrations/netbox.md), [Generic JSON-over-HTTP IPAM](/src/crates/netflow-plugin/integrations/generic_json-over-http_ipam.md).
- YAML-defined: [Static Metadata](/src/crates/netflow-plugin/integrations/static_metadata.md), [Classifiers](/src/crates/netflow-plugin/integrations/classifiers.md), [Decapsulation](/src/crates/netflow-plugin/integrations/decapsulation.md).
- Operational: [Enrichment Intel Downloader](/docs/network-flows/intel-downloader.md) — the bundled refresh tool for MMDB providers.

The enrichment section has no CLI flag — it is YAML-only.

## Common edits

### Listen on a different port

```yaml
listener:
  listen: "0.0.0.0:9995"
```

### Bind to a specific address

```yaml
listener:
  listen: "10.0.0.10:2055"
```

### Disable a protocol you don't use

```yaml
protocols:
  v5: false
```

### Move the journal directory

```yaml
journal:
  journal_dir: /var/lib/netflow
```

Absolute paths are used as-is. Relative paths resolve under `NETDATA_CACHE_DIR`.

### Strip VXLAN tunnel headers

```yaml
protocols:
  decapsulation_mode: vxlan
```

The plugin reads the inner 5-tuple from `dataLinkFrameSection` records (IPFIX IE 315) when the exporter ships them.

### Production retention profile

```yaml
journal:
  tiers:
    raw:
      size_of_journal_files: 200GB
      duration_of_journal_files: 24h
    minute_1:
      size_of_journal_files: 20GB
      duration_of_journal_files: 14d
    minute_5:
      size_of_journal_files: 20GB
      duration_of_journal_files: 30d
    hour_1:
      size_of_journal_files: 20GB
      duration_of_journal_files: 365d
```

The built-in defaults (10GB / 7d on every tier) are intended for first validation and small deployments. Most production deployments should size retention from observed flow rate. This profile gives you 24 hours of full-detail forensics, 14 days of 1-minute trends, 30 days of 5-minute snapshots, and a year of hourly aggregates. Storage required scales with your flow rate — see [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md).

## Things that go wrong

- **The plugin doesn't start.** Check `journalctl --namespace netdata --since "5 minutes ago" | grep netflow`. The most common cause is a typo in a YAML key (strict mode rejects unknowns).
- **Edits don't take effect.** Restart Netdata. There is no DynCfg integration for the plugin's configuration.
- **CLI flags I added don't do anything.** When running under Netdata, only the YAML is read.
- **Tiers fill up faster than expected.** Each tier has its own size/duration. Set per-tier values that match how long you actually need each tier.
- **Queries time out at 30 seconds.** Function calls have a hard 30s timeout in the plugin. If your query is too wide, narrow the time range or add filters that let a higher tier serve it.
- **`__overflow__` appears in results.** A group-by exceeded `query_max_groups` (default 50 000). Either narrow the filter, reduce the number of group-by fields, or raise the limit.

## What's next

- [Retention and Querying](/docs/network-flows/retention-querying.md) — How the four tiers work and how the dashboard picks one.
- [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) — How much disk and CPU you need.
- [Validation and Data Quality](/docs/network-flows/validation.md) — How to confirm the data is right.
- [Troubleshooting](/docs/network-flows/troubleshooting.md) — When things break.
