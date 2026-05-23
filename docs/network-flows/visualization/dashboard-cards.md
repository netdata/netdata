<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/visualization/dashboard-cards.md"
sidebar_label: "Plugin Health Charts"
learn_status: "Published"
learn_rel_path: "Network Flows/Visualization"
keywords: ['plugin health', 'metrics', 'operational charts', 'monitoring']
endmeta-->

<!-- markdownlint-disable-file -->

# Plugin Health Charts

The netflow plugin publishes its own operational charts under the `netdata.netflow.*` chart context. These appear on the standard Netdata charts page (alongside system metrics like CPU and memory), **not** inside the Network Flows view. They are how you monitor the plugin itself: is it receiving data, are templates flowing, is memory growing, is disk being written.

This is also where you look first when something seems wrong — long before opening the Network Flows view.

All charts update every 1 second.

## The charts

| Chart | Type | What it shows |
|---|---|---|
| `netflow.input_packets` | line, packets/s | Datagrams: received, parsed, errored, per-protocol counts |
| `netflow.input_bytes` | line, bytes/s | UDP byte rate |
| `netflow.raw_journal_ops` | line, ops/s | Raw journal: writes, sync calls, errors |
| `netflow.raw_journal_bytes` | line, bytes/s | Raw journal logical bytes written |
| `netflow.materialized_tier_ops` | line, ops/s | Rollup tiers: rows produced per tier, flushes, errors |
| `netflow.materialized_tier_bytes` | stacked, bytes/s | Rollup tier byte rate, broken down by tier |
| `netflow.open_tiers` | stacked, rows | Rows currently open per tier |
| `netflow.journal_io_ops` | line, ops/s | Decoder-state persist operations and errors |
| `netflow.journal_io_bytes` | line, bytes/s | Decoder-state persist byte rate |
| `netflow.decoder_scopes` | line, scopes | Distinct (exporter, observation domain) scopes the decoder tracks |
| `netflow.memory_resident_bytes` | line, bytes | Process RSS, peak RSS, breakdown |
| `netflow.memory_resident_mapping_bytes` | stacked, bytes | RSS broken down by what's in it |
| `netflow.memory_allocator_bytes` | line, bytes | Allocator-internal stats |
| `netflow.memory_accounted_bytes` | stacked, bytes | RSS attributed to known components, plus `unaccounted` |
| `netflow.memory_tier_index_bytes` | stacked, bytes | Tier-index memory drilldown |

## Reading the most useful charts

### `netflow.input_packets`

The single most important chart. Five families of dimensions:

- **`udp_received`** — datagrams pulled off the socket. If this is zero, nothing is reaching the plugin (firewall, no exporter, wrong port).
- **`parse_attempts`, `parsed_packets`** — should track each other on a healthy collector. If `parse_attempts` is high but `parsed_packets` is low, datagrams are arriving but failing to decode.
- **`parse_errors`** — counts datagrams that failed parsing for any reason (truncated, malformed, unsupported version).
- **`template_errors`** — counts data records arriving before their template (v9 / IPFIX). Should be near zero in steady state. A sustained non-zero rate means the exporter is sending templates too rarely or your collector has lost template state.
- **`netflow_v5`, `netflow_v7`, `netflow_v9`, `ipfix`, `sflow`** — per-protocol successful counts. Useful to identify "which protocol is actually arriving".

### `netflow.decoder_scopes`

Cardinality of decoder state. Reports how many distinct `(exporter, observation domain)` template caches the plugin currently holds. Watch for unbounded growth — an exporter that frequently rotates template IDs (rare but real) will inflate this without bound.

### `netflow.materialized_tier_*`

Show the rollup pipeline working. `*_rows` should track ingest. `flushes` should tick steadily; if it stops, tiering is stalled.

### `netflow.memory_resident_bytes` and `netflow.memory_accounted_bytes`

If RSS climbs over time:

- Check `netflow.memory_accounted_bytes` to see where it's going.
- The `unaccounted` dimension is `RSS - sum(known components)`. **A growing `unaccounted` is your leak signal.**
- `tier_indexes` and `open_tiers` are normal sources of growth — they should track ingest rate.
- `geoip_asn` and `geoip_geo` are mmap'd MMDB files. Their size grows as the kernel pages the file in under read pressure.

### `netflow.memory_resident_mapping_bytes`

This one breaks RSS down by what's mapped. Useful when you want to attribute "this process is using 800 MB" — heap, journals (per tier), MMDB files, anonymous mappings, etc.

## What's NOT in these charts

These charts do not include:

- **Per-exporter ingest counter.** No per-source rate dimension. Decoder-scope cardinality tells you how many sources, not how busy each one is.
- **UDP socket drops.** Kernel-level drops (full receive buffer, NIC drops) are not surfaced. Use OS-level signals: `sudo ss -uamn sport = :2055` for per-socket drops, or `grep ^Udp: /proc/net/snmp` for the system-wide `RcvbufErrors` counter.
- **Template cache hit ratio.** `template_errors` counts misses; there's no corresponding "hits" counter to form a ratio.
- **GeoIP staleness signal.** No "MMDB last loaded" timestamp or version. The mapping memory dimensions tell you if a database is loaded, not how old it is.
- **Per-tier query latency.** These charts cover ingest and storage; query-side performance isn't observable.
- **BioRIS counters.** BioRIS routing-state details are not published as chart dimensions.

## How to use these charts for diagnosis

| Symptom | Look at | What it means |
|---|---|---|
| Network Flows view is empty | `netflow.input_packets` `udp_received` | Zero = no datagrams arriving (firewall? wrong port?). Non-zero with `parsed_packets` zero = wrong protocol or all datagrams malformed. |
| Sudden drop in flows | per-protocol dimensions | Identifies which protocol stopped (helps narrow whether it's a router, a router class, or all routers). |
| Templates failing | `template_errors` rising | Exporter not sending templates often enough; collector lost cache; cache mismatch after firmware update. |
| Cache growing without bound | `decoder_scopes` rising over hours | Exporter churn or unstable template IDs. Investigate per-router behaviour. |
| Memory pressure | `netflow.memory_resident_bytes`, `netflow.memory_accounted_bytes` | If `rss` climbs and `unaccounted` is the dimension growing → unattributed allocation, possibly a leak. If `tier_indexes` or `open_tiers` climbs → ingest backpressure, flushing stalled. |
| Disk write stalls | `netflow.raw_journal_ops` `write_errors`, `sync_errors` | Disk full, permission denied, fs error. |
| Decoder state not persisting | `netflow.journal_io_ops` | `decoder_state_persist_calls` should tick periodically. `*_errors` should be 0. |

## Where these are NOT shown

These charts are **not** in the Network Flows view. Look for them on the standard Netdata charts page, in the family `netflow`. The Network Flows view itself shows traffic data, not plugin health.

## What's next

- [Troubleshooting](/docs/network-flows/troubleshooting.md) — Concrete diagnostic workflows.
- [Validation and Data Quality](/docs/network-flows/validation.md) — Cross-checking plugin counters against SNMP.
- [Configuration](/docs/network-flows/configuration.md) — Tuning that affects what these charts show.
