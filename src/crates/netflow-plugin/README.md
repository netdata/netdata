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
  size_of_journal_files: 10GB
  duration_of_journal_files: 7d
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
  query_1m_max_window: 6h
  query_5m_max_window: 24h
  query_max_groups: 50000
  query_facet_max_values_per_field: 5000
```

`query_max_groups` and `query_facet_max_values_per_field` are guardrails for
query-time accumulator cardinality. When limits are hit, overflow is reported
via response stats/facet metadata instead of growing unbounded memory.

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

`journal.tiers` optionally allows per-tier retention overrides for:

- `raw`
- `minute_1` (aliases: `1m`, `minute-1`, `minute1`)
- `minute_5` (aliases: `5m`, `minute-5`, `minute5`)
- `hour_1` (aliases: `1h`, `hour-1`, `hour1`)

If a tier override is omitted, that tier inherits top-level journal retention
(`size_of_journal_files`, `duration_of_journal_files`).

To make a tier time-only, set `size_of_journal_files: null`.
To make a tier size-only, set `duration_of_journal_files: null`.

## Performance benchmarking

The plugin now ships manual ingestion benchmarks for both throughput ceilings
and paced resource-envelope measurements:

- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release bench_ingestion_protocol_matrix -- --ignored --nocapture`
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release bench_ingestion_cardinality_matrix -- --ignored --nocapture`
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml --release ingest::resource_bench_tests::bench_resource_envelope_matrix -- --ignored --nocapture`

The resource-envelope benchmark is intentionally explicit about its scope:

- it measures the real ingest hot path after decode
- it uses mixed flow records derived from shipped NetFlow/IPFIX/sFlow fixtures
- it uses disk-backed journals under `src/crates/target/netflow-resource-bench`
- it reports achieved flows/s, CPU utilization, peak/final RSS, and actual disk
  write throughput from `/proc/self/io`

Reference measurements on this workstation:

- CPU: `12th Gen Intel(R) Core(TM) i9-12900K`
- storage: ext4 on `Seagate FireCuda 530`
- benchmark methodology: release mode, `5s` warmup, `15s` measurement window,
  disk-backed journals, post-decode mixed-flow ingest

Low-cardinality mixed profile (`record_pool_size=256`):

- offered `5k flows/s`: achieved `5000`, CPU `80.0%` of one core, peak RSS
  `283.36 MiB`, write `26453 KiB/s`
- offered `10k flows/s`: achieved `6187`, CPU `98.8%`, peak RSS `345.93 MiB`,
  write `35996 KiB/s`
- offered `20k flows/s`: achieved `6305`, CPU `98.6%`, peak RSS `345.93 MiB`,
  write `32194 KiB/s`
- offered `30k flows/s`: achieved `6183`, CPU `96.9%`, peak RSS `333.27 MiB`,
  write `31989 KiB/s`

High-cardinality mixed profile (`record_pool_size=4096`):

- offered `5k flows/s`: achieved `5000`, CPU `83.5%` of one core, peak RSS
  `380.39 MiB`, write `24571 KiB/s`
- offered `10k flows/s`: achieved `5843`, CPU `97.5%`, peak RSS `404.55 MiB`,
  write `31394 KiB/s`
- offered `20k flows/s`: achieved `5814`, CPU `96.6%`, peak RSS `403.82 MiB`,
  write `35888 KiB/s`
- offered `30k flows/s`: achieved `5937`, CPU `98.0%`, peak RSS `410.14 MiB`,
  write `33189 KiB/s`

Interpretation:

- `5k flows/s` is sustainable on this host for both profiles with headroom left
  on one core
- on this host the post-decode ingest path saturates one core around
  `5.8k - 6.3k flows/s`
- higher field variability/cardinality mainly raises steady memory usage, not
  the one-core throughput ceiling
- disk reads stay near zero in this benchmark because it isolates append-only
  ingest, not query/rebuild workloads
- these numbers are host-specific and should be treated as a reference point,
  not as a universal guarantee

## plugins.d protocol

When stdout is not a TTY (normal `plugins.d` runtime), `netflow-plugin` emits
`PLUGIN_KEEPALIVE` periodically to avoid parser inactivity timeouts.
