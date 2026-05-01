# Sizing Netdata Parents

This page helps you estimate the CPU, RAM, and disk resources required for Netdata Parent nodes based on the number of metrics and nodes you plan to centralize.

## Disk Estimation

Parent nodes receive all metrics streamed from Children and store them using tiered retention. Each tier compresses samples to a different size:

| Tier   | Resolution    | Compressed Size per Sample |
|--------|---------------|----------------------------|
| Tier 0 | per-second    | ~0.6 B                     |
| Tier 1 | per-minute    | ~6 B                       |
| Tier 2 | per-hour      | ~18 B                      |

### Estimation Formula

For each tier, calculate disk usage as:

```text
disk_per_tier = total_metrics × samples_per_day × bytes_per_sample × retention_days
```

| Variable            | Tier 0                | Tier 1               | Tier 2              |
|---------------------|-----------------------|----------------------|----------------------|
| samples_per_day     | 86,400 (1/s)          | 1,440 (1/min)        | 24 (1/hour)          |
| bytes_per_sample    | 0.6 B                 | 6 B                  | 18 B                 |

### Worked Example

A Parent receiving **1,000,000 metrics** with this retention policy:

- **Tier 0:** 30 days (per-second)
- **Tier 1:** 6 months / 180 days (per-minute)
- **Tier 2:** 5 years / 1,825 days (per-hour)

```text
Tier 0: 1,000,000 × 86,400 × 0.6 B × 30 d  ≈ 1,555 GB (1.6 TB)
Tier 1: 1,000,000 × 1,440  × 6 B   × 180 d ≈ 1,555 GB (1.6 TB)
Tier 2: 1,000,000 × 24     × 18 B  × 1825 d ≈ 787 GB  (0.8 TB)
                                         Raw total ≈ 3.7 TB
```

Adding 5–15% overhead for replication buffers, indexes, and metadata, plan for approximately **4 TB per million metrics** under this retention policy.

:::tip

Adjust the retention days per tier to match your actual requirements. Shorter retention on any tier reduces disk proportionally.

:::

For details on how tiers work and how to configure retention, see [Disk Requirements & Retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md).

## RAM Requirements

Parent RAM usage depends on the number of metrics and nodes being managed:

| Description                   | Scope                | RAM per Entry |
|-------------------------------|----------------------|---------------|
| Metrics with retention        | Time-series in DB    | 1 KiB         |
| Metrics currently collected   | Time-series collected | 20 KiB       |
| Metrics with ML models        | Time-series collected | 5 KiB        |
| Nodes with retention          | Nodes in DB          | 10 KiB        |
| Nodes currently received      | Nodes collected      | 512 KiB       |
| Nodes currently sent          | Nodes collected      | 512 KiB       |

**Per-metric summary:** Each metric currently being collected requires approximately **26 KiB** (1 KiB index + 20 KiB collection + 5 KiB ML). Archived metrics need only **1 KiB** for indexing.

**Per-node summary:** Each node currently being collected requires approximately **1,034 KiB** (10 KiB index + 512 KiB reception + 512 KiB dispatch). Archived nodes need only **10 KiB** for indexing.

For the full RAM estimation formula and cache sizing, see [RAM Utilization](/docs/netdata-agent/sizing-netdata-agents/ram-requirements.md).

## CPU Requirements

CPU usage on Parents is driven primarily by three features:

| Feature             | Depends On                        | CPU Cores per Million Metrics |
|---------------------|-----------------------------------|-------------------------------|
| Metrics Ingest      | Samples received per second       | 2                             |
| Metrics Re-streaming | Samples resent per second        | 2                             |
| Machine Learning    | Unique time-series collected      | 2                             |

A Parent processing **1 million metrics** with all three features active requires approximately **6 CPU cores**. Keep total CPU utilization below 60% during steady-state operation to handle traffic spikes gracefully.

For details on startup CPU behavior and optimization, see [CPU Utilization](/docs/netdata-agent/sizing-netdata-agents/cpu-requirements.md).

## Production Configuration Example

The following `netdata.conf` snippet configures time-based retention for a production Parent:

```ini
[db]
    mode = dbengine
    update every = 1
    storage tiers = 3

    # Tier 0: per-second data for 30 days
    dbengine tier 0 retention time = 30d

    # Tier 1: per-minute data for 6 months
    dbengine tier 1 update every iterations = 60
    dbengine tier 1 retention time = 6mo

    # Tier 2: per-hour data for 5 years
    dbengine tier 2 update every iterations = 60
    dbengine tier 2 retention time = 5y
```

:::warning

The default retention size limit is 1 GiB per tier, which is insufficient for production Parents. You must configure retention properly before deployment. For guidance on choosing between time-based, space-based, and combined retention strategies, see [Parent Configuration Best Practices](/docs/observability-centralization-points/best-practices.md).

:::

## Related Documentation

- [Parent Configuration Best Practices](/docs/observability-centralization-points/best-practices.md) — retention strategy selection, deployment optimization, and cost strategies
- [Disk Requirements & Retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md) — agent-level database modes, tier details, and default disk footprint
- [RAM Utilization](/docs/netdata-agent/sizing-netdata-agents/ram-requirements.md) — full RAM estimation formula, cache configuration, and logs co-hosting guidance
- [CPU Utilization](/docs/netdata-agent/sizing-netdata-agents/cpu-requirements.md) — per-feature CPU breakdown and startup behavior
