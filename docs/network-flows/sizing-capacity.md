<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/sizing-capacity.md"
sidebar_label: "Sizing and Capacity Planning"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['sizing', 'capacity', 'planning', 'storage', 'memory', 'scaling', 'distributed']
endmeta-->

<!-- markdownlint-disable-file -->

# Sizing and Capacity Planning

A practical guide to choosing the host, the storage, and the deployment shape for the netflow plugin. Read this before you decide where the plugin runs and how big it has to be.

## What this plugin is built for

The netflow plugin is designed to receive, decode, and store flow records (NetFlow / IPFIX / sFlow) from one network — typically the routers and switches at one site — directly on a Netdata Agent.

Sustained ingestion of about **25 000 flows per second** on a single agent already approaches **ISP-level traffic capacities** for most enterprise / branch / data-centre profiles. Past that point you are at the scale of a regional service provider, and you should be running multiple agents (see "Distributed deployment" below), not pushing harder on one box.

If your worst-case sustained flow rate stays at or below **~25 000 flows/s**, you have headroom on a single agent. If it does not, plan distributed before you plan harder iron.

## Plugin throughput cap

The post-decode ingest path has a single-thread hot path. For planning, treat **~25 000 flows/s sustained** as the comfortable ceiling for a well-provisioned single agent running the full pipeline (raw + 1m + 5m + 1h tiers). High-cardinality traffic reaches the ceiling sooner; low-cardinality traffic has more headroom.

Practical guidance:

- Plan for ~25 000 flows/s sustained on a well-provisioned agent. That is the comfortable steady-state ceiling and includes UDP receive, protocol decode, all four storage tiers, and the typical enrichment stack without BMP / BioRIS.
- Bursts above the ceiling cost UDP queue backpressure and eventually `RcvbufErrors` (kernel-level drops). Tune the kernel UDP receive buffer (`net.core.rmem_max`, `net.core.rmem_default`, `net.core.netdev_max_backlog`) for headroom.
- The hot path is single-threaded. Adding cores does not raise the per-agent ceiling. The way to go past it is to add agents (next section).

## Distributed deployment is the scaling answer

Aggregation across many routers is rarely operationally meaningful for flow data — you almost always investigate one site, one router, or one interface at a time. So instead of pushing every flow to a single central collector, **deploy one Netdata Agent next to each router (or each site, or each branch office)**.

This pattern is how Netdata is built to scale: each agent owns its own flow journal, its own enrichment, and its own dashboard view; Netdata Cloud federates queries across them. The benefits compound:

- **Each agent's load is bounded by one router's flow rate**, not the whole network's. A 10 000 flows/s router stays comfortably under the per-agent ceiling.
- **No single host becomes the bottleneck** for ingest, storage, or query latency.
- **Failure of one agent loses one router's history**, not the whole network's.
- **You don't pay the bandwidth cost** of moving every flow datagram to a central collector across WAN links.

For a multi-site / multi-data-centre / multi-branch deployment, this is the recommended shape: one Netdata Agent per router, federated through Netdata Cloud. Use the central collector pattern only if your sites are too small to host an agent each.

## Storage

Storage cost scales linearly with **sustained flows per second** and the **retention** you configure on each tier.

### How ingestion rate maps to disk

Use **~800 bytes on disk per flow** as the journal sizing estimate for the raw tier. For sustained ingestion this gives you:

| Sustained flows/s | Disk used per day, raw tier |
|---|---|
| 1 000 | ~70 GB |
| 5 000 | ~350 GB |
| 10 000 | ~700 GB |
| 25 000 | ~1.7 TB |

These numbers are dominated by raw-tier writes; rollup tiers (1m, 5m, 1h) add a small constant on top because each rollup row aggregates many raw rows.

The number is sensitive to traffic cardinality (how unique the 5-tuples are). Real-world traffic with many repeated flows trends a bit lower; pathological all-unique traffic trends a bit higher.

### Raw tier dominates — keep it bounded

The raw tier carries every individual flow record. **Rollup tiers (1m, 5m, 1h) are tiny by comparison** — each row in a rollup tier aggregates many raw flows.

Set raw-tier retention to match your forensic window — typically **24 hours**. Rollup tiers can keep weeks to a year of history at a small fraction of raw-tier cost.

The example below is sized for an agent at the **upper end of the per-host scaling envelope (25 000 flows/s sustained)** — the raw tier needs about 1.7 TB / day at that rate (see the table above), so the 2 TB raw budget gives a small safety margin to keep the duration limit (24 h) the one that fires first:

```yaml
journal:
  tiers:
    raw:      { size_of_journal_files: 2TB,  duration_of_journal_files: 24h }
    minute_1: { size_of_journal_files: 20GB, duration_of_journal_files: 14d }
    minute_5: { size_of_journal_files: 20GB, duration_of_journal_files: 30d }
    hour_1:   { size_of_journal_files: 20GB, duration_of_journal_files: 365d }
```

For lighter loads, scale `size_of_journal_files` on the raw tier down proportionally — at 10 000 flows/s ~700 GB / 24 h is enough; at 1 000 flows/s ~70 GB / 24 h is enough. Whichever limit (size or duration) is hit first triggers rotation; size your raw tier so the **duration limit fires first** under normal load and the size cap is a safety net for traffic surges.

### Use fast NVMe for the raw tier

The raw tier is queried directly for any IP-level investigation, full-text search, city / latitude / longitude maps, and anything that filters on a raw-only field (see [Field Reference](/docs/network-flows/field-reference.md) for which fields survive into rollups). At 25 000 flows/s sustained, the raw tier produces 1.7 TB / day of indexed writes that you may also be reading back in real time.

This is **fast-NVMe territory**. 1.7 TB/day of write throughput is well within a modern PCIe Gen4 / Gen5 NVMe drive but punishes SATA SSDs (queue-depth and write-endurance) and HDDs (IOPS) once concurrent queries land on the same device. A modern PCIe Gen4 / Gen5 NVMe is what you want for the raw-tier directory. Rollup tiers (1m / 5m / 1h) are far less I/O-intensive and can live on slower storage if needed, but in practice it's easier to put the whole journal directory on one fast device.

If the raw tier exceeds the device capacity for your retention target, **shorten raw-tier retention** before you switch to slower storage. A 12-hour raw tier on fast NVMe queries cleanly; a 7-day raw tier on slow storage will time out queries.

## Memory

The journal backend uses **free system memory as page cache** — the bigger the database on disk, the more free RAM you want to keep available so the kernel can keep the working set hot.

Concrete guidance:

- For the agent process itself, expect **a few hundred MB to ~1 GB of RSS** at typical 5-25k flows/s loads. Enrichment, classifiers, accumulators, and routing tries add to the base process footprint. BMP / BioRIS full-table feeds can add a few hundred MB per peer, depending on table count and prefix mix.
- For the kernel page cache, aim to **leave at least the size of the recently-queried working set free** — practically, plan a few GB of free RAM on a 25k flows/s agent so query I/O lands in cache instead of hitting NVMe each time.
- Watch `netflow.memory_resident_bytes`, `netflow.memory_resident_mapping_bytes`, and `netflow.memory_accounted_bytes` for the agent's own footprint. Watch the system's overall free memory for the page-cache headroom.

The plugin does **not** preload the journal into RAM. Memory consumption is driven by active accumulators (during ingestion) and routing tries (when configured). Storage growth pressures memory only via the page cache, which the kernel manages.

## Querying — what's fast and what isn't

The journal is **fully indexed**: every field is indexed, exact-match selections (`SRC_AS_NAME = "AS15169 GOOGLE"`, `PROTOCOL = 6`) hit the index and return quickly regardless of how much data the tier holds.

The exception is the **full-text search box** in the dashboard. FTS runs as a regex against the raw journal payload bytes — it is a **full scan** of the matching tier. Any non-empty FTS query also forces the query to the raw tier (FTS is meaningless on aggregated rollup rows). That means:

- A full-text search over a 24-hour raw tier with 25k flows/s sustained scans hundreds of GB. It runs but it is not fast.
- For fast queries, use **filters on indexed fields** (the filter ribbon, exact selections). Reserve full-text for the cases where you don't have an indexed handle.
- The 30-second hard query timeout is a real ceiling for FTS over wide windows. Narrow the time range, add an indexed filter, or switch to a rollup tier (which means dropping the FTS).

## Practical checklist before you deploy

1. **Estimate sustained flows/s** for the worst-case router or site. If it exceeds ~25k/s, plan distributed before iron.
2. **Pick raw-tier retention** that matches your forensic window — typically 24 hours.
3. **Compute storage** for that retention from the table above; size the raw-tier directory with a realistic safety margin, usually **1.2× to 1.5×** depending on burstiness and available disk.
4. **Use NVMe** for the raw tier. Slower storage shortens raw-tier retention until queries are responsive.
5. **Leave RAM headroom** for the page cache on top of the agent's own ~1 GB RSS budget.
6. **Tune kernel UDP buffers** for burst headroom (see [Troubleshooting](/docs/network-flows/troubleshooting.md)).
7. **For multi-site deployments**, run one Netdata Agent per router or per site rather than central aggregation.

## What's next

- [Configuration](/docs/network-flows/configuration.md#per-tier-retention) — Per-tier retention schema.
- [Retention and Querying](/docs/network-flows/retention-querying.md) — How tiers map to queries and the auto-tier-pick rules.
- [Field Reference](/docs/network-flows/field-reference.md) — Which fields survive into rollups and which are raw-only.
- [Troubleshooting](/docs/network-flows/troubleshooting.md) — UDP buffer tuning, query timeout, and disk write pressure.
