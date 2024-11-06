# Database

Netdata is fully capable of long-term metrics storage, at per-second granularity, via its default Database engine
(`dbengine`). But to remain as flexible as possible, Netdata supports several storage options:

1. `dbengine`, data is stored in the database. The [Database engine](/src/database/engine/README.md) works like a traditional database. There is some amount of RAM dedicated to data caching and indexing and the rest of the data resides compressed on disk. The number of history entries is not fixed in this case, but depends on the configured disk space and the effective compression ratio of the data stored. This is the **only mode** that supports changing the data collection update frequency (`update every`) **without losing** the previously stored metrics. For more details see [here](/src/database/engine/README.md).

2. `ram`, data is purely in memory and it is never saved on disk. This mode uses `mmap()` and supports [KSM](/docs/netdata-agent/configuration/optimizing-metrics-database/optimization-with-ksm.md).

3. `alloc`, is like `ram` but it uses `calloc()` and does not support [KSM](/docs/netdata-agent/configuration/optimizing-metrics-database/optimization-with-ksm.md). This mode is the fallback for all others except `none`.

4. `none`, without a database (collected metrics can only be streamed to another Netdata).

## Select Database mode

The default mode `[db].mode = dbengine` has been designed to scale for longer retention and is the only mode suitable for Parents in a Centralization setup.

The other available database modes are designed to minimize resource utilization and should only be considered on [Centralization](/docs/observability-centralization-points/README.md) setups for the Children and only when the resource constraints are very strict.

- On a single node setup, use `[db].mode = dbengine`.
- On a [Centralization](/docs/observability-centralization-points/README.md) setup, use `[db].mode = dbengine` for the Parent to increase retention, and `ram`, or `none` for the Children to minimize resource utilization.

You can select the database mode by using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) to open `netdata.conf` and setting:

```text
[db]
  # dbengine, ram, alloc, none
  mode = dbengine
```
