<!-- markdownlint-disable-file MD013 MD043 -->

# `netflow-plugin`

Rust NetFlow/IPFIX/sFlow ingestion and query plugin.

It stores flow entries in journal tiers under the Netdata cache directory and exposes
`flows:netflow`.

## Configuration

When running under Netdata, config is loaded from `netflow.yaml` in:

- `${NETDATA_USER_CONFIG_DIR}/netflow.yaml` (preferred)
- `${NETDATA_STOCK_CONFIG_DIR}/netflow.yaml` (fallback)

Top-level toggle:

- `enabled: true|false` controls the netflow plugin itself.

If `journal.journal_dir` is relative (default: `flows`), it is resolved against
`NETDATA_CACHE_DIR`. With the standard cache directory this becomes:

- `/var/cache/netdata/flows/raw`
- `/var/cache/netdata/flows/1m`
- `/var/cache/netdata/flows/5m`
- `/var/cache/netdata/flows/1h`

If `enrichment.geoip` does not define explicit MMDB paths, the plugin auto-detects
packaged databases in this order:

- `${NETDATA_CACHE_DIR}/topology-ip-intel`
- `${NETDATA_STOCK_DATA_DIR}/topology-ip-intel`

`netdata-plugin-netflow` now ships a stock MMDB seed set under
`${NETDATA_STOCK_DATA_DIR}/topology-ip-intel`. Freshly downloaded data written by
`topology-ip-intel-downloader` stays in `${NETDATA_CACHE_DIR}/topology-ip-intel` and
overrides the stock copy automatically.

Important:

- packaged installs ship the stock MMDB payload
- source installs from a Git checkout do not include the generated stock MMDBs by default
- local/source installs should run `topology-ip-intel-downloader` if they want a local cache copy
- packaged 32-bit installs still ship the stock MMDB payload, but do not include `topology-ip-intel-downloader`

### `protocols.decapsulation_mode`

Controls packet decapsulation for datalink payload parsing:

- `none` (default): keep outer header view.
- `srv6`: enable SRv6 decapsulation for supported payloads.
- `vxlan`: enable VXLAN decapsulation for supported payloads.

### `protocols.timestamp_source`

Controls which timestamp is written as `_SOURCE_REALTIME_TIMESTAMP` for decoded flows:

- `input` (default): packet receive time at ingestion.
- `netflow_packet`: NetFlow/IPFIX packet export timestamp.
- `netflow_first_switched`: first-switched timestamp from flow fields when available.

### `enrichment` (Akvorado-style static metadata and sampling)

Optional enrichment is applied at ingestion time. When enabled, it follows Akvorado core behavior
for static metadata and sampling defaults/overrides:

- metadata lookup by exporter prefix + interface index
- sampling override/default by exporter prefix
- exporter/interface classifier rules (`exporter_classifiers`, `interface_classifiers`)
- classifier cache TTL (`classifier_cache_duration`, default `5m`, minimum `1s`)
- routing static lookup for AS/mask/next-hop/path/community fields (`routing_static`)
- routing dynamic lookup from BMP updates (`routing_dynamic.bmp`)
- BMP decode-error tolerance is configurable (`routing_dynamic.bmp.max_consecutive_decode_errors`)
- network attribute lookup by source/destination IP prefix (`networks`)
- GeoIP lookup from local MMDB files (`geoip`)
- periodic remote network source refresh (`network_sources`)
- network attribute merge order: GeoIP base, then remote `network_sources`, then static `networks`
- static `networks` supernet/subnet inheritance (more specific prefixes override non-empty fields)
- AS and net provider order controls (`asn_providers`, `net_providers`)
- flows are dropped when metadata/sampling requirements are not met
- when both static and dynamic routes match, dynamic routes are preferred

Example:

```yaml
enrichment:
  classifier_cache_duration: 5m
  default_sampling_rate:
    192.0.2.0/24: 1000
  override_sampling_rate:
    192.0.2.128/25: 4000
  metadata_static:
    exporters:
      192.0.2.0/24:
        name: edge-router
        region: eu
        role: peering
        tenant: tenant-a
        site: par
        group: blue
        default:
          name: Default0
          description: Default interface
          speed: 1000
        if_indexes:
          10:
            name: Gi10
            description: 10th interface
            speed: 1000
            provider: transit-a
            connectivity: transit
            boundary: external
  geoip:
    asn_database:
      - /usr/share/GeoIP/GeoLite2-ASN.mmdb
    geo_database:
      - /usr/share/GeoIP/GeoLite2-City.mmdb
    optional: true
  networks:
    198.51.100.0/24:
      name: customer-a
      role: customer
      site: par1
      region: eu-west
      country: FR
      state: Ile-de-France
      city: Paris
      tenant: tenant-a
      asn: 64500
    203.0.113.0/24: transit-a
  network_sources:
    amazon:
      url: "https://ip-ranges.amazonaws.com/ip-ranges.json"
      method: GET
      tls:
        enable: true
        verify: true
        ca_file: ""
        cert_file: ""
        key_file: ""
      interval: 10m
      timeout: 30s
      transform: |
        (.prefixes + .ipv6_prefixes)[] |
        { prefix: (.ip_prefix // .ipv6_prefix), tenant: "amazon", region: .region, role: .service|ascii_downcase }
      headers:
        X-Example: "value"
  asn_providers: [flow, routing, geoip]
  net_providers: [flow, routing]
  routing_static:
    prefixes:
      198.51.100.0/24:
        asn: 64600
        as_path: [64550, 64600]
        communities: [123456, 654321]
        large_communities:
          - asn: 64600
            local_data1: 7
            local_data2: 8
        next_hop: 203.0.113.9
  routing_dynamic:
    bmp:
      enabled: true
      listen: "0.0.0.0:10179"
      receive_buffer: 0 # bytes; 0 keeps kernel default
      max_consecutive_decode_errors: 8
      rds: ["0", "65000:100", "192.0.2.1:42"]
      collect_asns: true
      collect_as_paths: true
      collect_communities: true
      keep: 5m
    bioris:
      enabled: false
      ris_instances: []
      timeout: 200ms
      refresh: 30m
      refresh_timeout: 10s
```

`routing_dynamic.bioris` is implemented as stream-based route ingestion:

- the plugin periodically discovers routers with `GetRouters`
- it consumes `DumpRIB` streams per router/AFI for baseline reconciliation
- it keeps `ObserveRIB` streams per router/AFI for incremental updates between refreshes
- enrichment keeps lookup local/in-memory (no per-flow remote RPC on the ingestion hot path)

`network_sources.*.transform` accepts jq expressions (compiled/executed via `jaq`) and
should emit objects with fields compatible with:

- `prefix` (required)
- `name`, `role`, `site`, `region`, `country`, `state`, `city`, `tenant`, `asn`, `asn_name` (optional)

`network_sources.*.tls` follows Akvorado-style source TLS controls:

- `enable` toggles TLS settings for this source.
- `verify` must remain `true`; disabling TLS certificate verification is not supported.
- `skip_verify` is accepted only for compatibility with earlier drafts and must remain `false`.
- `ca_file` sets a custom CA bundle.
- `cert_file` and `key_file` set optional client identity (if `key_file` is empty, `cert_file` is reused).

Example:

```yaml
enabled: true

listener:
  listen: "0.0.0.0:2055"

protocols:
  v5: true
  v7: true
  v9: true
  ipfix: true
  sflow: true
  decapsulation_mode: srv6
  timestamp_source: input

journal:
  journal_dir: flows
  tiers:
    raw:
      size_of_journal_files: 200GB
      duration_of_journal_files: 24h
    minute_1:
      size_of_journal_files: 40GB
      duration_of_journal_files: 14d
    minute_5:
      size_of_journal_files: 30GB
      duration_of_journal_files: 30d
    hour_1:
      size_of_journal_files: 20GB
      duration_of_journal_files: 365d
  query_max_groups: 50000
```

Standalone CLI runs still accept the legacy uniform retention flags
`--netflow-retention-size-of-journal-files` and
`--netflow-retention-duration-of-journal-files`. They apply the same value to
all tiers and exist only for standalone/CLI compatibility; YAML configuration is
per-tier.

`query_max_groups` caps the number of distinct group keys a single
aggregation query may build before extra groups are folded into a synthetic
`__overflow__` bucket. The response carries a warning when this happens. The
limit protects the query worker from accidentally wide group-by combinations
exhausting memory.

Journal rotation size is not user-configured. The plugin derives it per tier:

- if `size_of_journal_files` is set, rotation size is `clamp(size / 20, 5MB, 200MB)`
- `size_of_journal_files` must be at least `100MB`
- if `size_of_journal_files` is omitted or explicitly set to `null`, the plugin
  uses a fixed internal rotation size of `100MB`
- the internal time-based rotation cadence remains `1h`

The plugin also exposes internal memory charts to help diagnose resident growth
in production:

- `netflow.memory_resident_bytes`
  - `rss`, `hwm`
  - `rss_anon`, `rss_file`, `rss_shmem`, `anon_huge_pages`
- `netflow.memory_resident_mapping_bytes`
  - resident heap bytes
  - anonymous non-heap mappings
  - raw journal mmap bytes
  - `1m` journal mmap bytes
  - `5m` journal mmap bytes
  - `1h` journal mmap bytes
  - GeoIP ASN MMDB resident bytes
  - GeoIP geo/country MMDB resident bytes
  - other file-backed mappings
  - shmem mappings
- `netflow.memory_allocator_bytes`
  - `heap_in_use`, `heap_free`, `heap_arena`
  - `mmap_in_use`, `releasable`
- `netflow.memory_accounted_bytes`
  - facet runtime archived/active/contribution/published/path buckets
  - materialized tier indexes
  - open tier rows
  - GeoIP ASN MMDB resident bytes
  - GeoIP geo/country MMDB resident bytes
  - unaccounted process RSS remainder
- `netflow.memory_tier_index_bytes`
  - tier row storage
  - tier field dictionaries
  - tier lookup tables
  - tier schema/index metadata
- `netflow.decoder_scopes`
  - NetFlow v9 parser scopes
  - IPFIX parser scopes
  - legacy parser scopes
  - persisted decoder namespaces
  - hydrated namespace-source mappings

These charts are intended for debugging memory explosions under high-cardinality
traffic, not for billing or hard enforcement decisions.

`journal.tiers` configures retention independently for:

- `raw`
- `minute_1` (aliases: `1m`, `minute-1`, `minute1`)
- `minute_5` (aliases: `5m`, `minute-5`, `minute5`)
- `hour_1` (aliases: `1h`, `hour-1`, `hour1`)

If a tier is omitted, it uses the built-in tier default (`10GB / 7d`). There
are no top-level journal retention knobs; set retention on each tier you want
to tune.

To make a tier time-only, set `size_of_journal_files: null`.
To make a tier size-only, set `duration_of_journal_files: null`.

## Performance benchmarking

The plugin ships two complementary benchmarks:

- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release ingest::bench_tests::bench_ingestion_protocol_matrix -- --ignored --nocapture`
  unpaced full UDP→journal max throughput per protocol, plus decode-only and
  post-decode phases
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release ingest::resource_bench_tests::bench_resource_envelope_child -- --ignored --nocapture`
  paced post-decode resource envelope at a configurable rate, controlled via
  env vars: `NETFLOW_RESOURCE_BENCH_PROTOCOL`, `NETFLOW_RESOURCE_BENCH_PROFILE`,
  `NETFLOW_RESOURCE_BENCH_LAYER`, `NETFLOW_RESOURCE_BENCH_FLOWS_PER_SEC`,
  `NETFLOW_RESOURCE_BENCH_WARMUP_SECS`, `NETFLOW_RESOURCE_BENCH_MEASURE_SECS`

The resource-envelope benchmark scope:

- pre-decoded flow records pushed into the ingest pipeline (UDP receive and
  protocol decode are excluded; measure decode separately with the protocol
  matrix benchmark)
- full pipeline active: raw journal + 1-minute + 5-minute + 1-hour tier
  accumulation, real disk-backed journals
- enrichment is NOT loaded (no GeoIP MMDB, no static metadata, no classifiers,
  no static networks); cardinality fields are pre-populated by the harness
- reports achieved flows/s, CPU% of one core, peak/final RSS, real disk read
  and write bytes/s from `/proc/self/io`

`cpu_percent_of_one_core` is the sum of user+system ticks across all threads
of the test process during the measurement window, divided by wall time, as a
percent of one core. 100% means one core's worth of CPU was consumed; values
above 100% are normal for multi-threaded saturation.

Reference measurements:

- CPU: `12th Gen Intel(R) Core(TM) i9-12900K`
- storage: ext4 on `Seagate FireCuda 530`
- methodology: release mode, `5s` warmup, `15s` measurement window,
  disk-backed journals, post-decode paced ingest, all-tiers-batched layer

### Paced post-decode resource envelope (3A)

Per protocol, per cardinality, at 10 offered rates from 100 to 60 000 flows/s.
Cardinality is synthetic: low-cardinality cycles 256 unique records, high-
cardinality cycles 4 096 unique records. Real exporter data sits between the
two.

Low cardinality, NetFlow v9:

| offered | achieved | CPU | disk write | RAM peak |
|---:|---:|---:|---:|---:|
| 100 | 80 | 0.3% | 95 KiB/s | 13 MiB |
| 1 000 | 1 000 | 1.3% | 804 KiB/s | 23 MiB |
| 10 000 | 10 000 | 12.6% | 7.7 MiB/s | 75 MiB |
| 30 000 | 30 000 | 35.7% | 22.9 MiB/s | 83 MiB |
| 60 000 | 60 000 | 70.3% | 45.6 MiB/s | 98 MiB |

Low cardinality, IPFIX:

| offered | achieved | CPU | disk write | RAM peak |
|---:|---:|---:|---:|---:|
| 100 | 61 | 0.1% | 75 KiB/s | 13 MiB |
| 1 000 | 975 | 1.0% | 730 KiB/s | 24 MiB |
| 10 000 | 9 996 | 11.5% | 6.8 MiB/s | 61 MiB |
| 30 000 | 29 988 | 32.9% | 20.4 MiB/s | 83 MiB |
| 60 000 | 59 977 | 64.1% | 40.8 MiB/s | 78 MiB |

Low cardinality, sFlow:

| offered | achieved | CPU | disk write | RAM peak |
|---:|---:|---:|---:|---:|
| 100 | 99 | 0.2% | 107 KiB/s | 13 MiB |
| 1 000 | 985 | 1.5% | 847 KiB/s | 22 MiB |
| 10 000 | 9 989 | 16.9% | 8.4 MiB/s | 75 MiB |
| 30 000 | 29 967 | 46.2% | 25.2 MiB/s | 84 MiB |
| 60 000 | 59 984 | 87.1% | 50.3 MiB/s | 80 MiB |

High cardinality, NetFlow v9 (saturates around 30 000 flows/s):

| offered | achieved | CPU | disk write | RAM peak |
|---:|---:|---:|---:|---:|
| 100 | 80 | 0.5% | 409 KiB/s | 25 MiB |
| 1 000 | 1 000 | 4.3% | 2.0 MiB/s | 58 MiB |
| 10 000 | 10 000 | 36.8% | 7.2 MiB/s | 104 MiB |
| 30 000 | 29 331 | 98.0% | 24.9 MiB/s | 119 MiB |
| 60 000 | 26 475 | 98.8% | 30.3 MiB/s | 247 MiB |

High cardinality, IPFIX (saturates around 30-40 000 flows/s):

| offered | achieved | CPU | disk write | RAM peak |
|---:|---:|---:|---:|---:|
| 100 | 60 | 0.2% | 214 KiB/s | 22 MiB |
| 1 000 | 961 | 3.2% | 2.0 MiB/s | 61 MiB |
| 10 000 | 9 970 | 28.0% | 7.7 MiB/s | 121 MiB |
| 30 000 | 29 985 | 84.8% | 23.0 MiB/s | 121 MiB |
| 60 000 | 28 835 | 98.5% | 36.8 MiB/s | 193 MiB |

High cardinality, sFlow (saturates around 30 000 flows/s):

| offered | achieved | CPU | disk write | RAM peak |
|---:|---:|---:|---:|---:|
| 100 | 100 | 0.6% | 604 KiB/s | 29 MiB |
| 1 000 | 999 | 4.7% | 3.2 MiB/s | 77 MiB |
| 10 000 | 9 990 | 35.3% | 9.9 MiB/s | 113 MiB |
| 30 000 | 29 257 | 98.3% | 30.9 MiB/s | 129 MiB |
| 60 000 | 30 227 | 98.6% | 29.1 MiB/s | 122 MiB |

### Unpaced full UDP→journal protocol matrix (3B)

Single-threaded peak throughput at native fixture cardinality:

| protocol | full ingest (decode + journal) | decode only | post-decode only |
|---|---:|---:|---:|
| NetFlow v9 | 99 000 flows/s | 811 000 flows/s | 116 000 flows/s |
| IPFIX | 107 000 flows/s | 807 000 flows/s | 124 000 flows/s |
| sFlow | 88 000 flows/s | 2 392 000 flows/s | 99 000 flows/s |

### Interpretation

- The post-decode ingest hot path is currently single-threaded. CPU pins at
  ~98-99% of one core at saturation; it does not scale further with more cores.
- Low-cardinality saturation is above 60 000 flows/s for NetFlow v9 and IPFIX,
  and around 70 000 flows/s for sFlow on this host. The matrix above does not
  reach those ceilings on purpose; extrapolate from the CPU% column.
- High-cardinality saturation is around 30 000 flows/s post-decode for all
  three protocols. Above the knee, achieved rate stays at the plateau while
  offered rate grows.
- Adding decode (~10 µs/flow) on top of post-decode ingest brings the practical
  full-path ceiling to roughly 22-25 000 flows/s at high cardinality on this
  host, with the four-tier pipeline running and no enrichment. UDP socket
  receive is not measured; at these flow rates packet rate (1-5k pps) is well
  below typical socket limits.
- Disk reads stay near zero because the benchmark isolates the ingest path
  from query workloads. The journals themselves are indexed and rewrite pages
  during normal operation; this benchmark does not exercise that overhead at
  steady state because it only runs for the warmup + measurement window.
- Higher cardinality raises steady memory and per-flow encoding cost, lowering
  the throughput ceiling.
- These numbers are specific to this host. They do not include GeoIP/MMDB or
  any other enrichment; loading enrichment adds per-lookup CPU cost on top.

## plugins.d protocol

When stdout is not a TTY (normal `plugins.d` runtime), `netflow-plugin` emits
`PLUGIN_KEEPALIVE` periodically to avoid parser inactivity timeouts.
