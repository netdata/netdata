<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/configuration.md"
sidebar_label: "Configuration"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['configuration', 'netflow.yaml', 'tuning', 'retention', 'listener']
endmeta-->

# Configuration

The netflow plugin reads its configuration from `netflow.yaml`. Defaults are sane out of the box; most operators only adjust three things — the listener address, the journal retention, and (rarely) the per-tier overrides. This page documents every option, with its real default and the file that defines it.

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

The `listener`, `protocols`, and `journal` sections are flattened — their keys can also appear at the top level (the stock file does this for compatibility). Both forms are accepted.

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
| `timestamp_source` | `--netflow-timestamp-source` | `input` | Where the dashboard's flow timestamps come from. See below. |

You must keep at least one protocol enabled or the plugin refuses to start.

### `timestamp_source` values

- **`input`** (default) — the time the plugin received the datagram. Charts always look "now". This is the safest choice for dashboards.
- **`netflow_packet`** — the time the exporter put in the NetFlow/IPFIX header.
- **`netflow_first_switched`** — the time the flow actually started, from the per-record first-switched field. Records arrive with timestamps in the past (up to your active timeout). This gives the most accurate timeline but charts may show data appearing "behind" real time.

## `journal`

This is the section most operators tune. It controls where flow data lives, how much of it lives, and how the query engine guardrails its work.

```yaml
journal:
  journal_dir: flows
  size_of_journal_files: 10GB
  duration_of_journal_files: 7d
  query_1m_max_window: 6h
  query_5m_max_window: 24h
  query_max_groups: 50000
  query_facet_max_values_per_field: 5000
  tiers:
    raw:        { duration_of_journal_files: 24h }
    minute_1:   { duration_of_journal_files: 14d }
    minute_5:   { duration_of_journal_files: 30d }
    hour_1:     { duration_of_journal_files: 365d }
```

### Top-level retention

| Key | Default | Notes |
|---|---|---|
| `journal_dir` | `flows` | Relative paths resolve under `NETDATA_CACHE_DIR` (typically `/var/cache/netdata/flows`). Absolute paths are used as-is. |
| `size_of_journal_files` | `10GB` | Disk budget per tier (not total). Minimum `100MB`. Set to `null` to disable size-based retention. |
| `duration_of_journal_files` | `7d` | Time budget per tier. Set to `null` to disable time-based retention. |

**Important.** The top-level retention applies to **every tier independently** unless you override it per-tier. So with the defaults, all four tiers (raw, 1m, 5m, 1h) share the same 10GB / 7d budget. **This is rarely what you want.** The whole point of having rollup tiers is to keep them around longer than raw. See per-tier overrides below.

Either limit triggers rotation. With size = 10GB and duration = 7d, the tier expires whichever is hit first.

### Per-tier overrides

```yaml
tiers:
  raw:                          # name in YAML
    size_of_journal_files: 50GB
    duration_of_journal_files: 24h
  minute_1:
    duration_of_journal_files: 14d
  minute_5:
    duration_of_journal_files: 30d
  hour_1:
    duration_of_journal_files: 365d
    size_of_journal_files: null   # time-only retention for the long tail
```

| YAML name | Aliases | On-disk directory |
|---|---|---|
| `raw` | — | `flows/raw/` |
| `minute_1` | `1m`, `minute-1`, `minute1` | `flows/1m/` |
| `minute_5` | `5m`, `minute-5`, `minute5` | `flows/5m/` |
| `hour_1` | `1h`, `hour-1`, `hour1` | `flows/1h/` |

The on-disk directory names are short (`1m`, `5m`, `1h`); the YAML keys are explicit (`minute_1`, `minute_5`, `hour_1`). Mind the difference if you go look at the disk.

For each per-tier knob (`size_of_journal_files`, `duration_of_journal_files`):

- **Omit the key** to inherit the top-level default.
- Set to `null` to **disable** that limit on this tier.
- Set to a value to override.

A typical production profile is the example block above: 24 hours of raw, 2 weeks at 1-minute, 30 days at 5-minute, 1 year at 1-hour. This profile keeps detailed forensics within reach while supporting year-over-year capacity trends.

### Rotation

Each tier rotates files at `size_of_journal_files / 20`, clamped between 5 MB and 200 MB. Time-based rotation is fixed at one hour per file. You don't configure these directly.

### Query guardrails

| Key | Default | What it limits |
|---|---|---|
| `query_1m_max_window` | `6h` | Above this window, the dashboard skips the 1-minute tier and uses the 5-minute or 1-hour tier. |
| `query_5m_max_window` | `24h` | Above this window, the dashboard skips the 5-minute tier and uses the 1-hour tier. |
| `query_max_groups` | `50000` | Maximum groups returned by a single aggregation query. Past this, results overflow into a single `__overflow__` bucket and the response carries a warning. |
| `query_facet_max_values_per_field` | `5000` | Maximum distinct values returned per facet field. |

The query-window limits are about responsiveness — large windows on fine-grained tiers are slow. The group/value limits are about memory — wide aggregations on high-cardinality fields can blow up. Raise them carefully.

## `enrichment`

Enrichment is a large topic and lives in dedicated pages. The top-level enable/disable knobs:

```yaml
enrichment:
  # default_sampling_rate: 1024              # set to override; default is unset (rate=1)
  # override_sampling_rate: { 10.1.0.0/16: 1024 }  # per-prefix override map
  default_sampling_rate: ~
  override_sampling_rate: {}
  metadata_static: { exporters: {} }
  geoip: { asn_database: [], geo_database: [] }
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

Detailed configuration of each section lives on its own page:

- [GeoIP](/docs/network-flows/enrichment/ip-intelligence.md)
- [Static metadata](/docs/network-flows/enrichment/static-metadata.md)
- [Classifiers](/docs/network-flows/enrichment/classifiers.md)
- [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md)
- [BMP routing](/docs/network-flows/enrichment/bgp-routing.md)
- [BioRIS](/docs/network-flows/enrichment/bgp-routing.md)
- [Network sources](/docs/network-flows/enrichment/network-identity.md)
- [Decapsulation](/docs/network-flows/enrichment/decapsulation.md)

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
  size_of_journal_files: 100GB
  duration_of_journal_files: 7d
  tiers:
    raw:
      size_of_journal_files: 200GB
      duration_of_journal_files: 24h
    minute_1:
      duration_of_journal_files: 14d
    minute_5:
      duration_of_journal_files: 30d
    hour_1:
      duration_of_journal_files: 365d
      size_of_journal_files: null
```

The default 10GB / 7d on every tier is too tight for most production deployments. This profile gives you 24 hours of full-detail forensics, 14 days of 1-minute trends, 30 days of 5-minute snapshots, and a year of hourly aggregates. Storage required scales with your flow rate — see [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md).

## Things that go wrong

- **The plugin doesn't start.** Check `journalctl -u netdata --since "5 minutes ago" | grep netflow`. The most common cause is a typo in a YAML key (strict mode rejects unknowns).
- **Edits don't take effect.** Restart Netdata. There is no DynCfg integration for the plugin's configuration.
- **CLI flags I added don't do anything.** When running under Netdata, only the YAML is read.
- **Tiers fill up faster than expected.** All tiers share the top-level retention by default. Set explicit per-tier overrides.
- **Queries time out at 30 seconds.** Function calls have a hard 30s timeout in the plugin. If your query is too wide, narrow the time range or add filters that let a higher tier serve it.
- **`__overflow__` appears in results.** A group-by exceeded `query_max_groups` (default 50 000). Either narrow the filter, reduce the number of group-by fields, or raise the limit.

## What's next

- [Retention and Querying](/docs/network-flows/retention-querying.md) — How the four tiers work and how the dashboard picks one.
- [Sizing and Capacity Planning](/docs/network-flows/sizing-capacity.md) — How much disk and CPU you need.
- [Validation and Data Quality](/docs/network-flows/validation.md) — How to confirm the data is right.
- [Troubleshooting](/docs/network-flows/troubleshooting.md) — When things break.
