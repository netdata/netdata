# Database

Netdata stores detailed metrics at one-second granularity using its Database engine. This document provides an overview of the various elements of the Database, if you want to configure it, check the [configuration reference page](/src/database/CONFIGURATION.md)

## Modes

| Mode       | Description                                                                                                                                                                                                                                           |
|------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dbengine` | Stores data in a database with RAM for caching and indexing, while keeping compressed data on disk. Storage capacity depends on available disk space and data compression ratio.  For details, see [Database Engine](/src/database/engine/README.md). |
| `ram`      | Stores data entirely in memory without disk persistence.                                                                                                                                                                                              |
| `none`     | Operates without storage (metrics can only be streamed to another Agent).                                                                                                                                                                             |

The default `dbengine` mode is optimized for:

- Long-term data retention
- Parent nodes in [Centralization](/docs/observability-centralization-points/README.md) setups

For resource-constrained environments, particularly Child nodes in Centralization setups, consider using `ram`.

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

With these defaults, Netdata requires approximately 4 GiB of storage space (including metadata).

### Monitoring Retention Utilization

Netdata provides a visual representation of storage utilization for both the time and space limits across all Tiers under "Netdata" -> "dbengine retention" on the dashboard. This chart shows exactly how your storage space (disk space limits) and time (time limits) are used for metric retention.

## Cache sizes

There are two cache sizes that can be used to optimize the Database:

1. **Page cache size**: The main cache that keeps metrics data into memory. When data is not found in it, the extent cache is consulted, and if not found in that too, they are loaded from the disk.
2. **Extent cache size**: The compressed extent cache. It keeps in memory compressed data blocks, as they appear on disk, to avoid reading them again. Data found in the extent cache but not in the main cache have to be uncompressed to be queried.
