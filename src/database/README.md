# Database

Netdata stores detailed metrics at one-second granularity using its Database engine.

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

Use [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) to open `netdata.conf` and set your preferred mode:

```text
[db]
  # dbengine, ram, none
  mode = dbengine
```

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

### Retention Settings

> **Important**
>
> In a Parent-Child setup, these settings manage the entire storage space used by the Parent for storing metrics collected both by itself and its Children.

You can fine-tune retention for each tier by setting a time limit or size limit. Setting a limit to 0 disables it. This enables the following retention strategies:

| Setting                        | Retention Behavior                                                                                                                       |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| Size Limit = 0, Time Limit > 0 | **Time based:** data is stored for a specific duration regardless of disk usage                                                          |
| Time Limit = 0, Size Limit > 0 | **Space based:** data is stored with a disk space limit, regardless of time                                                              |
| Time Limit > 0, Size Limit > 0 | **Combined time and space limits:** data is deleted once it reaches either the time limit or the disk space limit, whichever comes first |

You can change these limits using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) to open `netdata.conf`:

```text
[db]
    mode = dbengine
    storage tiers = 3

    # Tier 0, per second data. Set to 0 for no limit.
    dbengine tier 0 retention size = 1GiB
    dbengine tier 0 retention time = 14d

    # Tier 1, per minute data. Set to 0 for no limit.
    dbengine tier 1 retention size = 1GiB
    dbengine tier 1 retention time = 3mo

    # Tier 2, per hour data. Set to 0 for no limit.
    dbengine tier 2 retention size = 1GiB
    dbengine tier 2 retention time = 2y
```

### Monitoring Retention Utilization

Netdata provides a visual representation of storage utilization for both the time and space limits across all Tiers under "Netdata" -> "dbengine retention" on the dashboard. This chart shows exactly how your storage space (disk space limits) and time (time limits) are used for metric retention.

### Legacy configuration

<details><summary>v1.99.0 and prior</summary>

Netdata prior to v2 supports the following configuration options in  `netdata.conf`.
They have the same defaults as the latest v2, but the unit of each value is given in the option name, not at the value.

```text
storage tiers = 3
# Tier 0, per second data. Set to 0 for no limit.
dbengine tier 0 disk space MB = 1024
dbengine tier 0 retention days = 14
# Tier 1, per minute data. Set to 0 for no limit.
dbengine tier 1 disk space MB = 1024
dbengine tier 1 retention days = 90
# Tier 2, per hour data. Set to 0 for no limit.
dbengine tier 2 disk space MB = 1024
dbengine tier 2 retention days = 730
```

</details>

<details><summary>v1.45.6 and prior</summary>

Netdata versions prior to v1.46.0 relied on disk space-based retention.

**Default Retention Limits**:

| Tier |     Resolution      | Size Limit |
|:----:|:-------------------:|:----------:|
|  0   |  high (per second)  |   256 MB   |
|  1   | middle (per minute) |   128 MB   |
|  2   |   low (per hour)    |   64 GiB   |

You can change these limits in `netdata.conf`:

```text
[db]
    mode = dbengine
    storage tiers = 3
    # Tier 0, per second data
    dbengine multihost disk space MB = 256
    # Tier 1, per minute data
    dbengine tier 1 multihost disk space MB = 1024
    # Tier 2, per hour data
    dbengine tier 2 multihost disk space MB = 1024
```

</details>

## Cache sizes

There are two cache sizes that can be configured in `netdata.conf` to better optimize the Database:

1. `[db].dbengine page cache size`: this is the main cache that keeps metrics data into memory. When data is not found in it, the extent cache is consulted, and if not found in that too, they are loaded from the disk.
2. `[db].dbengine extent cache size`: this is the compressed extent cache. It keeps in memory compressed data blocks, as they appear on disk, to avoid reading them again. Data found in the extent cache but not in the main cache have to be uncompressed to be queried.

Both of them are dynamically adjusted to use some of the total memory computed above. The configuration in `netdata.conf` allows providing additional memory to them, increasing their caching efficiency.
