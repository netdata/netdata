<!-- markdownlint-disable-file -->

# Sizing and Capacity Planning

A practical guide to choosing the host, the storage, and the deployment shape for the netflow plugin. Read this before you decide where the plugin runs and how big it has to be.

## What this plugin is built for

The netflow plugin is designed to receive, decode, and store flow records (NetFlow / IPFIX / sFlow) from one network — typically the routers and switches at one site — directly on a Netdata Agent.

A single agent can ingest **50 000 to 100 000 flow records per second on modern hardware**, provided that the underlying disks can sustain the required journal write activity. This already approaches **ISP-level traffic capacities** for most enterprise / branch / data-centre profiles. Past that point you are at the scale of a regional service provider, and you should be running multiple agents (see "Distributed deployment" below), not pushing harder on one box.

If your worst-case sustained flow-record rate stays around **50 000 flow records/s** or lower, a modern NVMe-backed agent should have headroom when storage is not contended. If it does not, check your exporter batching, cardinality, and storage against the envelope below — or plan distributed before you plan harder iron.

## Plugin throughput cap

The post-decode ingest path has a single-thread hot path. For planning, treat **50 000-100 000 flow records/s sustained** as the per-agent envelope for the full pipeline (raw + 1m + 5m + 1h tiers) on modern hardware with fast storage. Where a given deployment lands inside that envelope depends on three factors:

- **Exporter batching** — routers usually batch multiple flow records into one UDP export packet. Larger batches reduce packet-rate overhead; one-record-per-packet exports push the collector toward a UDP packet-rate ceiling before the journal write path is saturated.
- **Traffic cardinality** — the per-flow CPU cost grows with how many *unique* flows the traffic contains (unique flows defeat journal deduplication and inflate index work). Low-cardinality traffic with normal exporter batching lands toward the high end of the envelope; high-cardinality traffic, expensive enrichment, or one-record-per-packet exports land toward the low end.
- **Disk I/O** — the raw tier is an indexed write stream (~400 bytes/flow). By default the plugin does not fsync on the hot path (see `sync_every_entries` in [Configuration](/docs/npm/network-flows/configuration.md)); data reaches disk via kernel writeback and every file is fully synced when rotated. Fast NVMe absorbs this easily; slow or contended storage (SATA SSD, HDD, busy shared volumes) backpressures the receive loop and can push the achievable rate below the envelope.

Practical guidance:

- Plan with **~50 000 flow records/s as a conservative target** for a well-provisioned modern agent with fast NVMe storage. Treat rates toward 100 000 flow records/s as achievable when exporter batching, cardinality, enrichment, and disk activity are favorable.
- Enabling periodic fsync (`sync_every_entries` > 0) trades throughput for a tighter crash-durability window: each fsync stalls the receive path, and on slow disks this causes `RcvbufErrors` (kernel-level UDP drops) well below the envelope.
- Bursts above your sustainable rate cost UDP queue backpressure and eventually `RcvbufErrors`. The plugin requests a 64 MiB receive buffer at startup, capped by `net.core.rmem_max` — raise that sysctl (and `net.core.netdev_max_backlog`) for burst headroom.
- The hot path is single-threaded. Adding cores does not raise the per-agent ceiling. The way to go past it is to add agents (next section).

## Distributed deployment is the scaling answer

Aggregation across many routers is rarely operationally meaningful for flow data — you almost always investigate one site, one router, or one interface at a time. So instead of pushing every flow to a single central collector, **deploy one Netdata Agent next to each router (or each site, or each branch office)**.

This pattern is how Netdata is built to scale: each agent owns its own flow journal, its own enrichment, and its own dashboard view; Netdata Cloud federates queries across them. The benefits compound:

- **Each agent's load is bounded by one router's flow rate**, not the whole network's. A 10 000 flow records/s router stays comfortably under the per-agent ceiling.
- **No single host becomes the bottleneck** for ingest, storage, or query latency.
- **Failure of one agent loses one router's history**, not the whole network's.
- **You don't pay the bandwidth cost** of moving every flow datagram to a central collector across WAN links.

For a multi-site / multi-data-centre / multi-branch deployment, this is the recommended shape: one Netdata Agent per router, federated through Netdata Cloud. Use the central collector pattern only if your sites are too small to host an agent each.

## Storage

Storage cost scales linearly with **sustained flow records per second** and the **retention** you configure on each tier.

### How ingestion rate maps to disk

Use **~400 bytes on disk per flow** as the journal sizing estimate for the raw tier. The journal uses the compact on-disk format, which roughly halves the per-flow footprint versus the legacy format (measured ~340 bytes/flow on low-cardinality traffic, ~490 bytes/flow on high-cardinality). For sustained ingestion this gives you:

| Sustained flow records/s | Disk used per day, raw tier |
|---|---|
| 1 000 | ~35 GB |
| 5 000 | ~175 GB |
| 10 000 | ~350 GB |
| 30 000 | ~1.0 TB |
| 50 000 | ~1.7 TB |
| 100 000 | ~3.5 TB |

These numbers are dominated by raw-tier writes; rollup tiers (1m, 5m, 1h) add a small constant on top because each rollup row aggregates many raw rows.

The number is sensitive to traffic cardinality (how unique the 5-tuples are). Real-world traffic with many repeated flows trends toward the low end (~350 bytes/flow); pathological all-unique traffic trends toward the high end (~490 bytes/flow).

### Raw tier dominates — keep it bounded

The raw tier carries every individual flow record. **Rollup tiers (1m, 5m, 1h) are tiny by comparison** — each row in a rollup tier aggregates many raw flows.

Set raw-tier retention to match your forensic window — typically **24 hours**. Rollup tiers can keep weeks to a year of history at a small fraction of raw-tier cost.

The example below is sized for a busy agent sustaining **50 000 flow records/s** — the raw tier needs about 1.7 TB / day at that rate (see the table above), so the 2.5 TB raw budget gives a safety margin to keep the duration limit (24 h) the one that fires first. Scale the raw budget with the table above if your agent runs higher inside the envelope:

```yaml
journal:
  tiers:
    raw:      { size_of_journal_files: 2.5TB, duration_of_journal_files: 24h }
    minute_1: { size_of_journal_files: 20GB,  duration_of_journal_files: 14d }
    minute_5: { size_of_journal_files: 20GB,  duration_of_journal_files: 30d }
    hour_1:   { size_of_journal_files: 20GB,  duration_of_journal_files: 365d }
```

For lighter loads, scale `size_of_journal_files` on the raw tier down proportionally — at 10 000 flow records/s ~350 GB / 24 h is enough; at 1 000 flow records/s ~35 GB / 24 h is enough. Whichever limit (size or duration) is hit first triggers rotation; size your raw tier so the **duration limit fires first** under normal load and the size cap is a safety net for traffic surges.

### Use fast NVMe for the raw tier

The raw tier is queried directly for any IP-level investigation, full-text search, city / latitude / longitude maps, and anything that filters on a raw-only field (see [Field Reference](/docs/npm/network-flows/field-reference.md) for which fields survive into rollups). At 50 000 flow records/s sustained, the raw tier produces ~1.7 TB / day of indexed writes that you may also be reading back in real time.

This is **fast-NVMe territory** — and it is also half of what positions you inside the throughput envelope, because the receive loop's writes must keep flowing to this device. ~1.7 TB/day of raw-tier writes at 50 000 flow records/s is well within a modern PCIe Gen4 / Gen5 NVMe drive but punishes SATA SSDs (queue-depth and write-endurance) and HDDs (IOPS) once concurrent queries land on the same device — on slower devices write backpressure drops UDP packets well below the envelope. A modern PCIe Gen4 / Gen5 NVMe is what you want for the raw-tier directory. Rollup tiers (1m / 5m / 1h) are far less I/O-intensive and can live on slower storage if needed, but in practice it's easier to put the whole journal directory on one fast device.

If the raw tier exceeds the device capacity for your retention target, **shorten raw-tier retention** before you switch to slower storage. A 12-hour raw tier on fast NVMe queries cleanly; a 7-day raw tier on slow storage will time out queries.

## Memory

The journal backend uses **free system memory as page cache** — the bigger the database on disk, the more free RAM you want to keep available so the kernel can keep the working set hot.

Concrete guidance:

- For the agent process itself, expect **a few hundred MB to ~1 GB of RSS** at typical loads across the envelope. Enrichment, classifiers, accumulators, and routing tries add to the base process footprint. BMP / BioRIS full-table feeds can add a few hundred MB per peer, depending on table count and prefix mix.
- For the kernel page cache, aim to **leave at least the size of the recently-queried working set free** — practically, plan a few GB of free RAM on a busy agent so query I/O lands in cache instead of hitting NVMe each time.
- Watch default lightweight state-cardinality charts such as `netflow.open_tiers`, `netflow.facet_values`, and `netflow.tier_index_entries` for the agent's internal growth drivers. Enable `charts.memory_diagnostics` only when you need byte-level process attribution. Watch the system's overall free memory for the page-cache headroom.

The plugin does **not** preload the journal into RAM. Memory consumption is driven by active accumulators (during ingestion) and routing tries (when configured). Storage growth pressures memory only via the page cache, which the kernel manages.

## Querying — what's fast and what isn't

The journal is **fully indexed**: every field is indexed, exact-match selections (`SRC_AS_NAME = "AS15169 GOOGLE"`, `PROTOCOL = 6`) hit the index and return quickly regardless of how much data the tier holds.

The exception is the **full-text search box** in the dashboard. FTS runs as a regex against the raw journal payload bytes — it is a **full scan** of the matching tier. Any non-empty FTS query also forces the query to the raw tier (FTS is meaningless on aggregated rollup rows). That means:

- A full-text search over a 24-hour raw tier with 50k flow records/s sustained scans on the order of multiple terabytes. It runs but it is not fast.
- For fast queries, use **filters on indexed fields** (the filter ribbon, exact selections). Reserve full-text for the cases where you don't have an indexed handle.
- The 30-second hard query timeout is a real ceiling for FTS over wide windows. Narrow the time range, add an indexed filter, or switch to a rollup tier (which means dropping the FTS).

## Practical checklist before you deploy

1. **Estimate sustained flow records/s** for the worst-case router or site. If it exceeds the per-agent envelope (50k-100k depending on exporter batching, cardinality, enrichment, and disk I/O), plan distributed before iron.
2. **Pick raw-tier retention** that matches your forensic window — typically 24 hours.
3. **Compute storage** for that retention from the table above; size the raw-tier directory with a realistic safety margin, usually **1.2× to 1.5×** depending on burstiness and available disk.
4. **Use NVMe** for the raw tier. Slower storage shortens raw-tier retention until queries are responsive.
5. **Leave RAM headroom** for the page cache on top of the agent's own ~1 GB RSS budget.
6. **Tune kernel UDP buffers** for burst headroom (see [Troubleshooting](/docs/npm/network-flows/troubleshooting.md)).
7. **For multi-site deployments**, run one Netdata Agent per router or per site rather than central aggregation.

## What's next

- [Configuration](/docs/npm/network-flows/configuration.md#per-tier-retention) — Per-tier retention schema.
- [Retention and Querying](/docs/npm/network-flows/retention-querying.md) — How tiers map to queries and the auto-tier-pick rules.
- [Field Reference](/docs/npm/network-flows/field-reference.md) — Which fields survive into rollups and which are raw-only.
- [Troubleshooting](/docs/npm/network-flows/troubleshooting.md) — UDP buffer tuning, query timeout, and disk write pressure.
