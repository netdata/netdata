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

The enforcement mechanism works as follows:

1. **Reactive timer-based enforcement**: The retention timer (`retention_timer_cb` in `src/database/engine/rrdengine.c`) fires every 60 seconds. Each tick calls `check_and_schedule_db_rotation()`, which calls `rrdeng_ctx_tier_cap_exceeded()` to decide whether to schedule a rotation job. The disk usage estimate used by `rrdeng_ctx_tier_cap_exceeded()` is a heuristic: `rrdeng_get_used_disk_space()` returns `ctx_current_disk_space_get()` plus `rrdeng_target_data_file_size()` minus the active datafile's current size, plus a tier-percentage share of any global database space — it is not a simple sum of datafile sizes and may differ from what `du` reports. This means up to 60 seconds of writes can accumulate before a cap violation is detected.

2. **Datafile-granular deletion**: When a cap is exceeded, Netdata schedules a rotation job that deletes the oldest **entire** datafile via `datafile_delete` (in `src/database/engine/rrdengine.c`). Datafiles range from 4 MB to 512 MB (see [Database Engine](/src/database/engine/README.md)). Deletion cannot be partial — a single datafile is always removed wholesale. After each rotation completes, `after_database_rotate()` calls `check_and_schedule_db_rotation()` again, so rotation jobs can chain until usage drops below the limit. Depending on ingestion rate and how quickly each rotation job completes, actual disk usage can exceed the configured limit by more than a single datafile before enforcement catches up.

3. **Why tier 0 overshoots more than tier 1/2**: Tier 0 has the highest write volume (per-second granularity), so more data arrives per timer interval. Combined with compression ratio variance in compressed extents, tier 0 datafiles can grow faster between enforcement cycles. Tier 1 and tier 2 aggregate fewer points per interval and their datafiles fill more predictably.

**Practical guidance**:

- To minimize overshoot, configure your retention size with a buffer margin. For example, if you need 30 GiB of actual disk usage, configure approximately 27 GiB.
- Setting retention days together with retention size uses **whichever limit is hit first**. This does not create a stricter cap, but provides a secondary bound on disk usage.
- There is currently no mechanism to enforce a true hard cap on dbengine disk usage.

## Cache sizes

There are two cache sizes that can be used to optimize the Database:

1. **Page cache size**: The main cache that keeps metrics data into memory. When data is not found in it, the extent cache is consulted, and if not found in that too, they are loaded from the disk.
2. **Extent cache size**: The compressed extent cache. It keeps in memory compressed data blocks, as they appear on disk, to avoid reading them again. Data found in the extent cache but not in the main cache have to be uncompressed to be queried.

