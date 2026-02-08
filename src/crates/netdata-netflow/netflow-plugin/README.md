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
- flows are dropped when metadata/sampling requirements are not met

Example:

```yaml
enrichment:
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
```

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
