# Disk Requirements &amp; Retention

## Database Modes and Tiers

Netdata offers two database modes to suit your needs for performance and data persistence:

|        Mode        | Description                                                                                                                                                                                                                            |
|:------------------:|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| dbengine (default) | High-performance, multi-tier storage with compression. Metric samples are cached in memory and then written to disk in multiple tiers for efficient retrieval and long-term storage.                                                   |
|        ram         | In-memory storage. Metric samples are stored in memory only, and older data is overwritten as new data arrives. This mode prioritizes speed, making it ideal for Netdata Child instances that stream data to a central Netdata parent. |

## `dbengine`

:::note

By default, `dbengine` stores its metrics database files on disk. The exact location depends on your installation method, operating system, and whether you run Netdata in Docker or Kubernetes — see [Backing up a Netdata Agent](/docs/netdata-agent/backup-and-restore-an-agent.md) for the default path and other Netdata data locations.

:::

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

### Default Disk Footprint

Netdata Agent metrics storage is limited to 3 GiB by default (configurable), using 1 GiB per tier × 3 tiers. In total, with SQLite databases, alert transitions, and other metadata, expect about 4 GiB of disk usage under normal conditions. The default retention limits are:

|  Tier   | Resolution | Size Limit | Time Limit |
|:-------:|:----------:|:----------:|:----------:|
| `tier0` | per-second |   1 GiB    |  14 days   |
| `tier1` | per-minute |   1 GiB    |  3 months  |
| `tier2` |  per-hour  |   1 GiB    |  2 years   |

Data is deleted when retention enforcement detects that **either** the size limit or the time limit has been reached, whichever comes first. Actual disk usage may temporarily exceed the configured size limit because retention size is a soft target, not a hard cap. For the detailed enforcement behavior, see [Retention Size Enforcement](/src/database/README.md#retention-size-enforcement).

In practice, with default settings and an ingestion rate of about 4,000 metrics per second, Netdata provides about 14 days of high resolution (per-second) data, 3 months of medium resolution (per-minute) data, and more than 1 year of low resolution (per-hour) data.

These limits are fully configurable. See [Changing how long Netdata stores metrics](/src/database/CONFIGURATION.md#tiers).

### Parent Retention Sizing

On Netdata Parents, retention size is enforced per tier for all metrics stored by that Parent, not per Child. All streaming Children share the Parent's tier quota, so there is no per-Child disk space limit.

When sizing a Parent, account for the total metric count across all Children and leave room for dbengine datafiles plus journal/index overhead (`.ndf`, `.njf`, and `.njfv2` files). Parent nodes with many Children can have higher aggregate metric cardinality, which can increase journal/index overhead compared to a standalone Agent.

For details about how dbengine enforces retention size limits, see [Retention Size Enforcement](/src/database/README.md#retention-size-enforcement).

Child and Parent storage are independent:

- **On the Child (local):** Controlled by the Child's `[db].db`.
- **On the Parent (received stream):** Controlled by the Parent's settings. Metrics streamed from Children can be persisted on the Parent and count against the Parent's per-tier retention limits.

**Configuring dbengine mode and retention**:

- Enable dbengine mode: The dbengine mode is already the default, so no configuration change is necessary. For reference, the dbengine mode can be configured by setting `[db].db` to `dbengine` in `netdata.conf`.
- Adjust retention (optional): see [Change how long Netdata stores metrics](/src/database/CONFIGURATION.md#tiers).

## `ram`

`ram` mode can help when Netdata shouldn’t introduce any disk I/O at all. In both of these modes, metric samples exist only in memory, and only while they’re collected.

When Netdata is configured to stream its metrics to a Metrics Observability Centralization Point (a Netdata Parent), metric samples are forwarded in real-time to that Netdata Parent. The ring buffers available in these modes are used to cache the collected samples for some time, in case there are network issues, or the Netdata Parent is restarted for maintenance.

The memory required per sample in these modes, is four bytes: `ram` mode uses `mmap()` behind the scene, and can be incremented in steps of 1024 samples (4KiB). Mode `ram` allows the use of the Linux kernel memory dedupper (Kernel-Same-Page or KSM) to deduplicate Netdata ring buffers and save memory.

**Configuring ram mode and retention**:

- Enable ram mode: To use in-memory storage, set `[db].db` to ram in your `netdata.conf` file. Remember, this mode won't retain historical data after restarts.
- Adjust retention (optional): While ram mode focuses on real-time data, you can optionally control the number of samples stored in memory. Set `[db].retention` in `netdata.conf` to the desired number in seconds. Note: If the value you choose isn't a multiple of 1024, Netdata will automatically round it up to the nearest multiple.
