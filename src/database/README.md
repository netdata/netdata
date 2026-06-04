# Database

Netdata stores detailed metrics at one-second granularity using its Database engine. This document provides an overview of the various elements of the Database, if you want to configure it, check the [configuration reference page](/src/database/CONFIGURATION.md)

## Modes

| Mode       | Description                                                                                                                                                                                                                                           |
|------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dbengine` | The high performance multi-tiered time-series database of Netdata, providing superior storage efficiency (~0.5 bytes per sample on disk for high resolution per-second data), and fast long term data queries (typically 20+ times faster) by transparently utilizing all available database tiers. For details, see [Database Engine](/src/database/engine/README.md). |
| `ram`      | Stores data entirely in memory without disk persistence. This is typically used in IoT deviced or children that stream their metrics to Netdata parents, to avoid having any disk dependency on Netdata                                                                                                                                                                |
| `none`     | Operates without storage (metrics can only be streamed to a Netdata parent).                                                                                                                                                                             |

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

Netdata provides a visual representation of storage utilization for both the time and space limits across all Tiers under "Netdata" -> "dbengine retention" on the dashboard. This chart shows exactly how your storage space (disk space limits) and time (time limits) are used for metric retention.

### Retention Size Enforcement

Retention size limits are **soft caps**, not hard caps. Netdata writes data unconditionally and checks limits afterwards — it does not block or reject writes when a cap is approaching.

How enforcement works:

1. **Periodic and asynchronous checks**: Netdata checks whether a tier has exceeded its configured size limit periodically and also after normal database activity (such as extent writes and rotation completions). Data continues to be written to disk without restriction between these checks.

2. **Whole-file deletion**: When the configured size limit is exceeded, Netdata deletes the oldest complete data files until usage is back under the limit. Multiple files may be deleted across one or more rotation passes. Data files range from 4 MB to 512 MB (see [Database Engine](/src/database/engine/README.md#datafiles)). Because entire files are removed and cannot be partially deleted, actual disk usage can overshoot the configured limit before enforcement catches up.

3. **Why tier 0 overshoots more**: Tier 0 collects per-second data, producing the highest write volume. More data accumulates between enforcement checks, and data files fill faster. Tier 1 and tier 2 have lower write volumes and their disk usage grows more predictably.

**Practical guidance**:

- Provision storage to accommodate 2–3× your configured tier 0 retention size, especially on parent nodes streaming from many children.
- Setting both **retention size** and **retention time** for the same tier provides a dual bound — data is deleted when either threshold is reached.
- There is currently no mechanism to enforce a true hard cap on dbengine disk usage. To minimize overshoot, set both size and time limits and ensure adequate storage headroom.

## Cache sizes

There are two cache sizes that can be used to optimize the Database:

1. **Page cache size**: The main cache that keeps metrics data into memory. When data is not found in it, the extent cache is consulted, and if not found in that too, they are loaded from the disk.
2. **Extent cache size**: The compressed extent cache. It keeps in memory compressed data blocks, as they appear on disk, to avoid reading them again. Data found in the extent cache but not in the main cache have to be uncompressed to be queried.

