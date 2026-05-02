# Disk Requirements &amp; Retention

## Database Modes and Tiers

Netdata offers two database modes to suit your needs for performance and data persistence:

|        Mode        | Description                                                                                                                                                                                                                            |
|:------------------:|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| dbengine (default) | High-performance, multi-tier storage with compression. Metric samples are cached in memory and then written to disk in multiple tiers for efficient retrieval and long-term storage.                                                   |
|        ram         | In-memory storage. Metric samples are stored in memory only, and older data is overwritten as new data arrives. This mode prioritizes speed, making it ideal for Netdata Child instances that stream data to a central Netdata parent. |

## `dbengine`

Netdata's `dbengine` mode efficiently stores data on disk using compression. The actual disk space used depends on how well the data compresses.
This mode uses a tiered storage approach: data is saved in multiple tiers on disk. Each tier retains data at a different resolution (detail level). Higher tiers store a down-sampled (less detailed) version of the data found in lower tiers.

```mermaid
gantt
    dateFormat  YYYY-MM-DD
    tickInterval 1week
    axisFormat    
    todayMarker off
    tier0, 14d       :a1, 2023-12-24, 7d
    tier1, 60d       :a2, 2023-12-01, 30d
    tier2, 365d      :a3, 2023-11-02, 59d
```

`dbengine` supports up to five tiers. By default, three tiers are used:

|  Tier   |                                          Resolution                                          | Uncompressed Sample Size | Usually On Disk |
|:-------:|:--------------------------------------------------------------------------------------------:|:------------------------:|:---------------:|
| `tier0` |            native resolution (metrics collected per-second as stored per-second)             |         4 bytes          |    0.6 bytes    |
| `tier1` | 60 iterations of `tier0`, so when metrics are collected per-second, this tier is per-minute. |         16 bytes         |     6 bytes     |
| `tier2` |  60 iterations of `tier1`, so when metrics are collected per second, this tier is per-hour.  |         16 bytes         |    18 bytes     |

### Configuring Custom Tier Resolutions

You can enable up to 5 tiers by setting `storage tiers` in `netdata.conf`, and you can customize the aggregation multiplier for each tier using the `dbengine tier N update every iterations` setting. This setting controls how many data points from the previous tier are aggregated into one data point for the current tier.

The default iteration multiplier is `60` for each tier, producing the default per-second / per-minute / per-hour progression. By changing this value (range: 2–255), you can create intermediate resolutions such as per-5-minute or per-5-second tiers.

:::note

The multiplication of all enabled tiers' `update every iterations` values must be less than `65535`.

:::

For example, to achieve a 4-tier retention profile with per-second, per-minute, per-5-minute, and per-hour resolutions:

```text
[db]
    mode = dbengine
    storage tiers = 4

    # Tier 0: per-second data
    dbengine tier 0 retention time = 14d

    # Tier 1: per-minute data (60 iterations of tier 0)
    dbengine tier 1 update every iterations = 60
    dbengine tier 1 retention time = 35d

    # Tier 2: per-5-minute data (5 iterations of tier 1)
    dbengine tier 2 update every iterations = 5
    dbengine tier 2 retention time = 400d

    # Tier 3: per-hour data (12 iterations of tier 2)
    dbengine tier 3 update every iterations = 12
    dbengine tier 3 retention time = 2y
```

In this configuration, tier 2 aggregates every 5 minutes instead of the default 60 minutes, and tier 3 aggregates every 12 iterations of tier 2 (60 minutes / 5 = 12) to produce per-hour data.

### Default Disk Footprint

Netdata Agent metrics storage is limited to 3 GiB by default (configurable), using 1 GiB per tier × 3 tiers. In total, with SQLite databases, alert transitions, and other metadata, expect about 4 GiB of disk usage under normal conditions. The default retention limits are:

| Tier    | Resolution | Size Limit | Time Limit |
|:-------:|:----------:|:----------:|:----------:|
| `tier0` | per-second |   1 GiB    |   14 days  |
| `tier1` | per-minute |   1 GiB    |  3 months  |
| `tier2` | per-hour   |   1 GiB    |   2 years  |

Data is deleted when it reaches **either** the size limit or the time limit, whichever comes first. The number of metrics collected determines how far back in time retention extends within the size limit.

In practice, with default settings and an ingestion rate of about 4,000 metrics per second, Netdata provides about 14 days of high resolution (per-second) data, 3 months of medium resolution (per-minute) data, and more than 1 year of low resolution (per-hour) data.

These limits are fully configurable. See [Changing how long Netdata stores metrics](/src/database/CONFIGURATION.md#tiers).

**Configuring dbengine mode and retention**:

- Enable dbengine mode: The dbengine mode is already the default, so no configuration change is necessary. For reference, the dbengine mode can be configured by setting `[db].mode` to `dbengine` in `netdata.conf`.
- Adjust retention (optional): see [Change how long Netdata stores metrics](/src/database/CONFIGURATION.md#tiers).

## `ram`

`ram` mode can help when Netdata shouldn’t introduce any disk I/O at all. In both of these modes, metric samples exist only in memory, and only while they’re collected.

When Netdata is configured to stream its metrics to a Metrics Observability Centralization Point (a Netdata Parent), metric samples are forwarded in real-time to that Netdata Parent. The ring buffers available in these modes are used to cache the collected samples for some time, in case there are network issues, or the Netdata Parent is restarted for maintenance.

The memory required per sample in these modes, is four bytes: `ram` mode uses `mmap()` behind the scene, and can be incremented in steps of 1024 samples (4KiB). Mode `ram` allows the use of the Linux kernel memory dedupper (Kernel-Same-Page or KSM) to deduplicate Netdata ring buffers and save memory.

**Configuring ram mode and retention**:

- Enable ram mode: To use in-memory storage, set `[db].mode` to ram in your `netdata.conf` file. Remember, this mode won't retain historical data after restarts.
- Adjust retention (optional): While ram mode focuses on real-time data, you can optionally control the number of samples stored in memory. Set `[db].retention` in `netdata.conf` to the desired number in seconds. Note: If the value you choose isn't a multiple of 1024, Netdata will automatically round it up to the nearest multiple.
