# Disk Requirements &amp; Retention

## Database Modes and Tiers

Netdata comes with 3 database modes:

1. `dbengine`: the default high-performance multi-tier database of Netdata. Metric samples are cached in memory and are saved to disk in multiple tiers, with compression.
2. `ram`: metric samples are stored in ring buffers in memory, with increments of 1024 samples. Metric samples are not committed to disk. Kernel-Same-Page (KSM) can be used to deduplicate Netdata's memory.
3. `alloc`: metric samples are stored in ring buffers in memory, with flexible increments. Metric samples are not committed to disk.

## `ram` and `alloc`

Modes `ram` and `alloc` can help when Netdata should not introduce any disk I/O at all. In both of these modes, metric samples exist only in memory, and only while they are collected.

When Netdata is configured to stream its metrics to a Metrics Observability Centralization Point (a Netdata Parent), metric samples are forwarded in real-time to that Netdata Parent. The ring buffers available in these modes is used to cache the collected samples for some time, in case there are network issues, or the Netdata Parent is restarted for maintenance.

The memory required per sample in these modes, is 4 bytes:

- `ram` mode uses `mmap()` behind the scene, and can be incremented in steps of 1024 samples (4KiB). Mode `ram` allows the use of the Linux kernel memory dedupper (Kernel-Same-Page or KSM) to deduplicate Netdata ring buffers and save memory.
- `alloc` mode can be sized for any number of samples per metric. KSM cannot be used in this mode.

To configure database mode `ram` or `alloc`, in `netdata.conf`, set the following:

- `[db].mode` to either `ram` or `alloc`.
- `[db].retention` to the number of samples the ring buffers should maintain. For `ram` if the value set is not a multiple of 1024, the next multiple of 1024 will be used.

## `dbengine`

`dbengine` supports up to 5 tiers. By default, 3 tiers are used, like this:

|   Tier   |                                          Resolution                                          | Uncompressed Sample Size | Usually On Disk |
|:--------:|:--------------------------------------------------------------------------------------------:|:------------------------:|:---------------:|
| `tier0`  |            native resolution (metrics collected per-second as stored per-second)             |         4 bytes          |    0.6 bytes    |
| `tier1`  | 60 iterations of `tier0`, so when metrics are collected per-second, this tier is per-minute. |         16 bytes         |     6 bytes     |
| `tier2`  |  60 iterations of `tier1`, so when metrics are collected per second, this tier is per-hour.  |         16 bytes         |    18 bytes     |

Data are saved to disk compressed, so the actual size on disk varies depending on compression efficiency.

`dbegnine` tiers are overlapping, so higher tiers include a down-sampled version of the samples in lower tiers:

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

## Disk Space and Metrics Retention

You can find information about the current disk utilization of a Netdata Parent, at <http://agent-ip:19999/api/v2/info>. The output of this endpoint is like this:

```json
{
  // more information about the agent
  // then, near the end:
  "db_size": [
    {
      "tier": 0,
      "metrics": 43070,
      "samples": 88078162001,
      "disk_used": 41156409552,
      "disk_max": 41943040000,
      "disk_percent": 98.1245269,
      "from": 1705033983,
      "to": 1708856640,
      "retention": 3822657,
      "expected_retention": 3895720,
      "currently_collected_metrics": 27424
    },
    {
      "tier": 1,
      "metrics": 72987,
      "samples": 5155155269,
      "disk_used": 20585157180,
      "disk_max": 20971520000,
      "disk_percent": 98.1576785,
      "from": 1698287340,
      "to": 1708856640,
      "retention": 10569300,
      "expected_retention": 10767675,
      "currently_collected_metrics": 27424
    },
    {
      "tier": 2,
      "metrics": 148234,
      "samples": 314919121,
      "disk_used": 5957346684,
      "disk_max": 10485760000,
      "disk_percent": 56.8136853,
      "from": 1667808000,
      "to": 1708856640,
      "retention": 41048640,
      "expected_retention": 72251324,
      "currently_collected_metrics": 27424
    }
  ]
}
```

In this example:

- `tier` is the database tier.
- `metrics` is the number of unique time-series in the database.
- `samples` is the number of samples in the database.
- `disk_used` is the currently used disk space in bytes.
- `disk_max` is the configured max disk space in bytes.
- `disk_percent` is the current disk space utilization for this tier.
- `from` is the first (oldest) timestamp in the database for this tier.
- `to` is the latest (newest) timestamp in the database for this tier.
- `retention` is the current retention of the database for this tier, in seconds (divide by 3600 for hours, divide by 86400 for days).
- `expected_retention` is the expected retention in seconds when `disk_percent` will be 100 (divide by 3600 for hours, divide by 86400 for days).
- `currently_collected_metrics` is the number of unique time-series currently being collected for this tier.

So, for our example above:

| Tier | # Of Metrics |  # Of Samples | Disk Used | Disk Free | Current Retention | Expected Retention | Sample Size |
|-----:|-------------:|--------------:|----------:|----------:|------------------:|-------------------:|------------:|
|    0 |        43.1K |  88.1 billion |    38.4Gi |     1.88% |         44.2 days |          45.0 days |      0.46 B |
|    1 |        73.0K |   5.2 billion |    19.2Gi |     1.84% |        122.3 days |         124.6 days |      3.99 B |
|    2 |       148.3K | 315.0 million |     5.6Gi |    43.19% |        475.1 days |         836.2 days |     18.91 B |

To configure retention, in `netdata.conf`, set the following:

- `[db].mode` to `dbengine`.
- `[db].dbengine multihost disk space MB`, this is the max disk size for `tier0`. The default is 256MiB.
- `[db].dbengine tier 1 multihost disk space MB`, this is the max disk space for `tier1`. The default is 50% of `tier0`.
- `[db].dbengine tier 2 multihost disk space MB`, this is the max disk space for `tier2`. The default is 50% of `tier1`.
