# `netflow-plugin`

Rust NetFlow/IPFIX/sFlow ingestion and query plugin.

It stores flow entries in journal tiers under the Netdata cache directory and exposes
`flows:netflow`.

## Configuration

When running under Netdata, config is loaded from `netflow.yaml` in:

- `${NETDATA_USER_CONFIG_DIR}/netflow.yaml` (preferred)
- `${NETDATA_STOCK_CONFIG_DIR}/netflow.yaml` (fallback)

If `journal.journal_dir` is relative (default: `flows`), it is resolved against
`NETDATA_CACHE_DIR`. With the standard cache directory this becomes:

- `/var/cache/netdata/flows/raw`
- `/var/cache/netdata/flows/1m`
- `/var/cache/netdata/flows/5m`
- `/var/cache/netdata/flows/1h`

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
        skip_verify: false
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
- `name`, `role`, `site`, `region`, `country`, `state`, `city`, `tenant`, `asn` (optional)

`network_sources.*.tls` follows Akvorado-style source TLS controls:

- `enable` toggles TLS settings for this source.
- `verify` (`true` by default) enables certificate verification.
- `skip_verify` can be set to `true` to disable certificate verification.
- `ca_file` sets a custom CA bundle.
- `cert_file` and `key_file` set optional client identity (if `key_file` is empty, `cert_file` is reused).

Example:

```yaml
listener:
  listen: "0.0.0.0:2055"

protocols:
  netflow_v5: true
  netflow_v7: true
  netflow_v9: true
  ipfix: true
  sflow: true
  decapsulation_mode: srv6
  timestamp_source: input

journal:
  journal_dir: flows
  size_of_journal_file: 256MB
  duration_of_journal_file: 1h
  number_of_journal_files: 64
  size_of_journal_files: 10GB
  duration_of_journal_files: 7d
  query_1m_max_window: 6h
  query_5m_max_window: 24h
```
