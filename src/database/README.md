# Database

Netdata stores detailed metrics at one-second granularity using its Database engine. This document provides an overview of the various elements of the Database, if you want to configure it, check the [configuration reference page](/src/database/CONFIGURATION.md)

## Modes

| Mode       | Description                                                                                                                                                                                                                                                                                                                                                             |
|------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dbengine` | The high performance multi-tiered time-series database of Netdata, providing superior storage efficiency (~0.5 bytes per sample on disk for high resolution per-second data), and fast long term data queries (typically 20+ times faster) by transparently utilizing all available database tiers. For details, see [Database Engine](/src/database/engine/README.md). |
| `ram`      | Stores data entirely in memory without disk persistence. This is typically used in IoT devices or children that stream their metrics to Netdata parents, to avoid having any disk dependency on Netdata |
| `none`     | Operates without storage (metrics can only be streamed to a Netdata parent).                                                                                                                                                                                                                                                                                            |

## Tiers

Netdata offers a granular approach to data retention, allowing you to manage storage based on both **time** and **disk space**. This provides greater control and helps you optimize storage usage for your specific needs.

**Default Retention Limits**:

| Tier |     Resolution      | Time Limit | Size Limit (min 256 MB) |
|:----:|:-------------------:|:----------:|:-----------------------:|
|  0   |  high (per second)  |    14d     |          1 GiB          |
|  1   | middle (per minute) |    3mo     |          1 GiB          |
|  2   |   low (per hour)    |     2y     |          1 GiB          |

> **Note**
>
> If a user sets a disk space size less than 256 MB for a tier, Netdata will automatically adjust it to 256 MB.

Netdata Agent metrics storage is limited to 3 GiB by default (configurable), using 1 GiB per tier × 3 tiers. Data is deleted when it reaches **either** the size limit or the time limit, whichever comes first. The number of metrics collected determines how far back in time retention extends within the size limit.

In total, with SQLite databases, alert transitions, and other metadata, expect about 4 GiB of disk usage under normal conditions.

In practice, with default settings and an ingestion rate of about 4,000 metrics per second, Netdata provides about 14 days of high resolution (per-second) data, 3 months of medium resolution (per-minute) data, and more than 1 year of low resolution (per-hour) data.

These limits are fully configurable. See [Changing how long Netdata stores metrics](/src/database/CONFIGURATION.md#tiers).

### Monitoring Retention Utilization

Netdata provides a visual representation of storage utilization for both the time and space limits across all Tiers. In the dashboard, these are the **dbengine space and time retention** charts, found under the **Netdata** section → **dbengine retention** family — there is one chart per database tier. Each chart shows exactly how your storage space (disk space limits) and time (time limits) are used for metric retention.

Each tier has its own independent **space** and **time** lines on the chart:

- **Space (disk utilization %)**: the percentage of the tier's allocated disk space that is currently occupied, calculated as `disk_used / disk_max × 100`. When a size limit is configured for the tier, `disk_max` is that limit. When no size limit is configured (set to 0), `disk_max` is the total available disk space at the tier's data directory (free space plus already-used space). For example, a space value of 89% means 89% of the tier's allocated or available disk space is occupied by datafiles.
- **Time (retention time utilization %)**: the percentage of the configured retention time period that has been filled with data, calculated as `current_retention_duration / configured_time_limit × 100`. When no time limit is configured (set to 0), this dimension shows 0% and no time-based retention is enforced. For example, a time value of 61% means 61% of the configured time retention period has elapsed since the oldest sample. This value is capped at 100%.

Different tiers naturally show different percentages because they have different write volumes, compression ratios, and configured limits. When either the space or time dimension reaches 100%, the oldest datafiles are rotated out as described in the [Retention Size Enforcement](#retention-size-enforcement) section below.

Each tier has its own chart — `dbengine_retention_tier0`, `dbengine_retention_tier1`, `dbengine_retention_tier2`, and so on — and every chart tracks two dimensions, both shown as a percentage:

- **`space`**: how much of the tier's configured disk space limit is currently consumed. For example, 9% on tier 0 means dbengine is using 9% of that tier's size quota.
- **`time`**: how much of the tier's configured time limit the currently retained data spans. For example, if tier 0's time limit is 14 days and the oldest stored sample is 1.26 days old, the `time` dimension reads approximately 9%.

The percentages are calculated as follows:

| Dimension | Calculation |
|-----------|-------------|
| `space` | `(disk space used by this tier ÷ configured size limit) × 100` |
| `time` | `(current data span in seconds ÷ configured time limit in seconds) × 100`, capped at 100% |

> **Note**
>
> **When retention size is 0 (disabled):** the `space` dimension has no tier-specific quota to divide by, so it falls back to measuring the tier's disk usage against the total capacity of the partition that hosts the dbengine data directory. The denominator becomes `(free space on the partition + disk space already used by the tier)`, which is effectively the full partition size. Because the percentage is measured against the entire partition rather than a tier-specific quota, the `space` value will typically be very low — for example, 9%, 2%, or 1% across tiers on a system that has explicitly set retention size to 0.
>
> **When retention time is 0 (disabled):** the `time` dimension reports 0%, because there is no time limit to measure against.

**Interpreting consistently low percentages:** Low `space` percentages are normal and do not indicate a problem. With the default per-tier size limits (1 GiB each), a low percentage simply means the tier has not yet filled its quota — common on systems with modest metric volume or limited uptime. If you have explicitly set a tier's `dbengine tier N retention size` to 0, the `space` dimension instead measures against total partition capacity (see the note above), which also produces low values because the tier's data is a small fraction of the disk. To determine which mode is in effect, check your retention size settings in `netdata.conf`: a value of 0 activates fallback mode, while any non-zero value (including the default 1 GiB) measures against the tier quota. See [Retention Settings](/src/database/CONFIGURATION.md#retention-settings) for configuration details.

### Retention Size Enforcement

Retention size limits are **soft caps**, not hard caps. Netdata writes data unconditionally and checks limits afterwards — it does not block or reject writes when a cap is approaching.

How enforcement works:

1. **Periodic and asynchronous checks**: Netdata checks whether a tier has exceeded its configured size limit periodically and also after normal database activity (such as extent writes and rotation completions). Data continues to be written to disk without restriction between these checks.

2. **Whole-file deletion**: When the configured size limit is exceeded, Netdata schedules deletion of the oldest complete datafiles until the retention check no longer reports the tier over its limit. Multiple files may be deleted across one or more rotation passes. Datafile size is determined automatically (see [Database Engine datafiles](/src/database/engine/README.md#datafiles)). Because entire files are removed and cannot be partially deleted, actual disk usage can overshoot the configured limit before enforcement catches up.

3. **Why tier 0 overshoots more**: Tier 0 collects per-second data, producing the highest write volume. More data accumulates between enforcement checks, and data files fill faster. Tier 1 and tier 2 have lower write volumes and their disk usage grows more predictably.

**Practical guidance**:

- Provision storage with workload-specific headroom, especially on parent nodes streaming from many children. The required headroom depends on ingestion rate, compression, storage throughput, and how quickly rotation catches up.
- Setting both **retention size** and **retention time** for the same tier can reduce retained history when either threshold is reached, but it does not create a hard disk cap.
- There is currently no mechanism to enforce a true hard cap on dbengine disk usage. To reduce the risk of disk-full conditions, validate retention sizing under expected production load and ensure adequate storage headroom.

## Cache sizes

There are two cache sizes that can be used to optimize the Database:

1. **Page cache size**: The main cache that keeps metrics data into memory. When data is not found in it, the extent cache is consulted, and if not found in that too, they are loaded from the disk.
2. **Extent cache size**: The compressed extent cache. It keeps in memory compressed data blocks, as they appear on disk, to avoid reading them again. Data found in the extent cache but not in the main cache have to be uncompressed to be queried.
