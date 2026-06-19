# Database Configuration Reference

You can configure the Agent's Database through the database settings. For a deeper understanding of the Database components, see the [Database overview](/src/database/README.md).

## Modes

Use [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) to open `netdata.conf` and set your preferred mode:

```text
[db]
  # dbengine, ram, none
  mode = dbengine
```

## RAM and ALLOC Mode Retention

When `mode` is set to `ram` or `alloc` (instead of the default `dbengine`), the Agent keeps each metric in a single in-memory ring buffer. There is no disk persistence and there are no storage tiers — the `dbengine tier ...` retention settings described in [Tiers](#tiers) do not apply to these modes.

The ring-buffer size is set by `retention` (default `3600`):

```text
[db]
    mode = ram
    retention = 3600
    update every = 10
```

`retention` is written as a duration in seconds, but the Agent uses it as the **number of ring-buffer slots** (entries) it keeps per chart dimension. Because one sample is stored per slot and one sample is collected every `update every` seconds, the history you can query is:

> **Effective wall-clock retention = slots × `update every`**

The slot count comes from `retention` alone — it does **not** change when you change `update every`. Raising `update every` therefore *lengthens* the wall-clock window for the same `retention` value and the same memory; it never shortens it.

### Worked example: `update every = 1` vs `update every = 10`

With `retention = 3600`, the slot count stays at 3600 in both cases. Only the wall-clock span each slot covers changes:

| `retention` | `update every` | Ring-buffer slots | Effective wall-clock retention | Memory per dimension |
|:-----------:|:--------------:|:-----------------:|:------------------------------:|:--------------------:|
| `3600`      | `1`            | 3600              | 3600s (1 hour)                 | unchanged            |
| `3600`      | `10`           | 3600              | 36000s (10 hours)              | unchanged            |

To choose a `retention` value for a target history window, divide the desired wall-clock seconds by `update every`. For roughly 1 hour (3600s) of local history at `update every = 10`, use `retention = 360` (360 slots × 10s = 3600s).

:::note

In `ram` mode the slot count is rounded up to the host's memory-page boundary (for example, 3600 becomes 4096 slots on a 4 KiB page system), so the actual figures are marginally higher than the nominal `retention`. `alloc` uses the `retention` value directly. The relationship to `update every` is identical in both modes.

:::

:::note

These `[db]` settings are read at Agent startup, so changing `mode`, `retention`, or `update every` takes effect only after restarting the Agent. `netdatacli reload-health` reloads health configuration only and does not apply `[db]` changes.

:::

In a Parent–Child streaming setup, a `ram`-mode Child normally keeps only this short local buffer while the Parent persists long-term history in `dbengine`. Child and Parent storage are independent — see the note in [Tiers](#tiers).

## Tiers

### Retention Settings

:::note

In a Parent-Child setup, these settings control the Parent's total storage for metrics collected locally and metrics received from Children.

Child and Parent storage are independent:

- A Child can keep local history based on its own `[db].mode`.
- Streamed metrics can also be persisted on the Parent, in the Parent's own dbengine files.

Retention size is enforced **per-tier**, not per Child, so all streaming Children share the Parent's tier quota. For Parent sizing guidance, see [Parent Retention Sizing](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md#parent-retention-sizing).

:::

You can fine-tune retention for each tier by setting a time limit or size limit. Setting a limit to 0 disables it. This enables the following retention strategies:

| Setting                        | Retention Behavior                                                                                                                       |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| Size Limit = 0, Time Limit > 0 | **Time based:** data is stored for a specific duration regardless of disk usage                                                          |
| Time Limit = 0, Size Limit > 0 | **Space based:** data is stored with a disk space limit, regardless of time                                                              |
| Time Limit > 0, Size Limit > 0 | **Combined time and space limits:** data is deleted once it reaches either the time limit or the disk space limit, whichever comes first |

:::note

Retention size limits are soft targets, not hard caps enforced at write time. Actual disk usage can temporarily exceed the configured limit. For the detailed enforcement behavior, see [Retention Size Enforcement](/src/database/README.md#retention-size-enforcement).

:::

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
