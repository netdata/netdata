<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/sizing-capacity.md"
sidebar_label: "Sizing and Capacity Planning"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['sizing', 'capacity planning', 'storage', 'cpu', 'memory', 'benchmarks']
endmeta-->

# Sizing and Capacity Planning

Use the following benchmarks and formulas to estimate the CPU, memory, and storage requirements for your Network Flows deployment.

## What was measured

These numbers come from a release-mode benchmark on an Intel i9-12900K workstation with a Seagate FireCuda 530 NVMe SSD (ext4). The benchmark runs the full ingest pipeline — raw journal plus the 1-minute, 5-minute, and 1-hour tiers — writing to real disk-backed journals. **Enrichment is not loaded** (no GeoIP/MMDB, no static metadata, no classifiers); enrichment adds CPU on top of the figures below.

Cardinality is synthetic: low-cardinality cycles 256 unique flow records, high-cardinality cycles 4 096 unique records. Real exporter traffic falls between the two.

CPU is reported as percent of one core (100% = one core fully consumed). The post-decode ingest path is currently single-threaded, so the practical ceiling per agent is bounded by one core's worth of CPU.

## Practical headline

On this hardware class, plan for:

| Scenario | Practical ceiling |
|---|---|
| High-cardinality, all three protocols, four storage tiers, no enrichment, including UDP receive and decode | **~20-25 000 flows/s** |
| Low-cardinality | comfortably above 60 000 flows/s |

The 20-25k figure is conservative and includes decode cost (~10 µs/flow on top of the post-decode numbers below).

## Detailed measurements (post-decode, paced)

These tables show the cost of pre-decoded flows traversing the full ingest pipeline. To get the full UDP-to-disk cost, add roughly 10 µs/flow for protocol decoding.

### Low cardinality

| offered flows/s | NetFlow v9 CPU | IPFIX CPU | sFlow CPU | RAM peak |
|---:|---:|---:|---:|---|
| 1 000 | 1.3% | 1.0% | 1.5% | ~25 MiB |
| 10 000 | 12.6% | 11.5% | 16.9% | ~75 MiB |
| 30 000 | 35.7% | 32.9% | 46.2% | ~85 MiB |
| 60 000 | 70.3% | 64.1% | 87.1% | ~85 MiB |

All three protocols deliver 100% of offered rate at every tested point. Saturation is above 60 000 flows/s and was not reached in this matrix.

### High cardinality

| offered flows/s | NetFlow v9 achieved | IPFIX achieved | sFlow achieved | CPU at saturation |
|---:|---:|---:|---:|---|
| 10 000 | 10 000 | 9 970 | 9 990 | 28-37% |
| 20 000 | 20 000 | 19 970 | 19 980 | 56-74% |
| 30 000 | 29 331 | 29 985 | 29 257 | 84-98% |
| 40 000 | 29 087 | 35 771 | 30 543 | 99% (plateau) |
| 60 000 | 26 475 | 28 835 | 30 227 | 99% (plateau) |

Saturation is around 30 000 flows/s on this host. Beyond the knee, the achieved rate plateaus at roughly the saturation value while the offered rate grows.

:::warning
These are host-specific reference points. Actual throughput depends on your CPU clock, disk speed, real flow cardinality, the number of populated fields, and any enrichment you enable (GeoIP, classifiers, static networks, ASN providers, BMP routing).
:::

## Storage

Storage is governed by two things, not by the flow rate alone:

- **Retention policy per tier** — caps how long each tier is kept and how much disk it can use.
- **Cardinality and dedup** — flow records are indexed and key-value pairs are deduplicated. Low-cardinality traffic stores fewer bytes per flow than high-cardinality traffic, because repeated values share dictionary entries.

Because the journals are not append-only logs, `flow_rate × bytes_per_flow × time` is not a valid estimator.

### Empirical measurement on this hardware class

A 15-minute run of paced ingest at 10 000 flows/s with the full pipeline active (raw + 1m + 5m + 1h tiers, real disk-backed journals) produced:

| | Low cardinality (256 unique records) | High cardinality (4 096 unique records) |
|---|---:|---:|
| Flows ingested | 9.00 million | 8.97 million |
| On-disk total | 6.46 GiB | 7.29 GiB |
| Bytes per stored flow | **771** | **872** |
| Write amplification (real I/O / logical encoded) | 1.79× | 2.00× |
| Raw tier (final) | 6.45 GiB | 7.13 GiB |
| 1-minute tier | 8 MiB | 112 MiB |
| 5-minute tier | 8 MiB | 40 MiB |
| 1-hour tier | 0 (rollup not reached in 15 min) | 16 MiB |

Two key observations:

- **Dedup is effective.** High cardinality stores only 13% more per flow despite 16× more unique field combinations. Real exporter traffic, which has heavy repetition (same src/dst/protocol patterns), will compress closer to the low-cardinality figure.
- **Raw is 99% of the on-disk cost** at 15 minutes. The rollup tiers are small in absolute size because each rollup row aggregates many raw flows.

### Bounding storage for capacity planning

Set retention limits explicitly and let them bound the disk footprint:

- raw: typically 24 hours
- 1-minute tier: 14 days
- 5-minute tier: 30 days
- 1-hour tier: 365 days

Configure per-tier `size_of_journal_files` (hard cap) and `duration_of_journal_files` (time cap). The plugin enforces whichever limit is hit first.

For your own measurement, run the plugin against representative traffic for at least 15 minutes and inspect `du -sh` on each tier directory. The `bench_storage_footprint_child` test in this repository ships the same measurement harness used to produce the table above.

## Memory

Memory consumption is dominated by:

- **Active journal rows** — flow records currently being accumulated before they are flushed to disk
- **Field indexes** — structures that map field values (IPs, ASNs, ports) for fast filtering
- **Facet indexes** — structures that power the filter sidebar
- **GeoIP MMDB** — the IP-intelligence database (DB-IP-based by default) loaded into memory for enrichment

The plugin exposes memory charts you can monitor:

- `netflow.memory_resident_bytes` — total memory in use
- `netflow.memory_allocator_bytes` — memory from the system allocator
- `netflow.memory_accounted_bytes` — memory broken down by component (indexes, GeoIP, facets)
- `netflow.memory_tier_index_bytes` — memory used by tiered storage indexes
- `netflow.decoder_scopes` — protocol decoder memory usage

## Disk I/O

The plugin writes flow records to journal files continuously. Writes are dominated by the raw tier; the rollup tiers add a small amount on top. SSDs are recommended for collectors that handle thousands of flows per second — the index updates and frequent fsync calls benefit substantially from low-latency storage.

Read operations only happen during queries. There is no background read activity in steady state.

:::tip
For production deployments, monitor the `netflow.memory_resident_bytes` chart and set a threshold alert. If resident memory grows steadily without stabilising, check your cardinality and consider reducing retention or increasing the sync interval.
:::

## What's next

- [Configuration](/docs/network-flows/configuration.md) — Per-tier retention configuration and tuning knobs.
- [Retention and Querying](/docs/network-flows/retention-querying.md) — How tiers are picked at query time.
- [Validation and Data Quality](/docs/network-flows/validation.md) — How to confirm the numbers in your environment.
- [Plugin Health Charts](/docs/network-flows/visualization/dashboard-cards.md) — Monitoring the plugin itself.
