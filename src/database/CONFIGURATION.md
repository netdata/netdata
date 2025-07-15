# Database Configuration Reference

You can configure the Agent's Database through the database settings. For a deeper understanding of the Database components, see the [Database overview](/src/database/README.md).

## Modes

Use [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) to open `netdata.conf` and set your preferred mode:

```text
[db]
  # dbengine, ram, none
  mode = dbengine
```

## Tiers

### Retention Settings

:::note

In a Parent-Child setup, these settings manage the entire storage space used by the Parent for storing metrics collected both by itself and its Children.

:::

You can fine-tune retention for each tier by setting a time limit or size limit. Setting a limit to 0 disables it. This enables the following retention strategies:

| Setting                        | Retention Behavior                                                                                                                       |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| Size Limit = 0, Time Limit > 0 | **Time based:** data is stored for a specific duration regardless of disk usage                                                          |
| Time Limit = 0, Size Limit > 0 | **Space based:** data is stored with a disk space limit, regardless of time                                                              |
| Time Limit > 0, Size Limit > 0 | **Combined time and space limits:** data is deleted once it reaches either the time limit or the disk space limit, whichever comes first |

You can change these limits using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) to open `netdata.conf`:

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

### Legacy Configuration

<details>
<summary><strong>v1.99.0 and prior</strong></summary>

Netdata prior to v2 supports the following configuration options in `netdata.conf`. They have the same defaults as the latest v2, but the unit of each value is given in the option name, not at the value.

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

<details>
<summary><strong>v1.45.6 and prior</strong></summary>

Netdata versions prior to v1.46.0 relied on disk space-based retention.

**Default Retention Limits:**

| Tier | Resolution          | Size Limit |
|------|---------------------|------------|
| 0    | high (per second)   | 256 MB     |
| 1    | middle (per minute) | 128 MB     |
| 2    | low (per hour)      | 64 GiB     |

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

## Cache Sizes

There are two cache sizes that you can configure in `netdata.conf` to better optimize the Database:

1. `[db].dbengine page cache size`: controls the size of the cache that keeps metric data on memory.
2. `[db].dbengine extent cache size`: controls the size of the cache that keeps in memory compressed data blocks.

:::info

Both of them are dynamically adjusted to use some of the total memory computed above. The configuration in `netdata.conf` allows providing additional memory to them, increasing their caching efficiency.

:::
