# Database engine

DBENGINE is the time-series database of Netdata.

![image](https://user-images.githubusercontent.com/2662304/233838474-d4f8f0b9-61dc-4409-a708-97d403cd153a.png)

## Design

### Data Points

**Data points** represent the collected values of metrics.

A **data point** has:

1. A **value**, the data collected for a metric. There is a special **value** to indicate that the collector failed to collect a valid value, and thus the data point is a **gap**.
2. A **timestamp**, the time it has been collected.
3. A **duration**, the time between this and the previous data collection.
4. A flag which is set when machine-learning categorized the collected value as **anomalous** (an outlier based on the trained models).

Using the **timestamp** and **duration**, Netdata calculates for each point its **start time**, **end time** and **update every**.

For incremental metrics (counters), Netdata interpolates the collected values to align them to the expected **end time** at the microsecond level, absorbing data collection micro-latencies.

When data points are stored in higher tiers (time aggregations - see [Tiers](#Tiers) below), each data point has:

1. The **sum** of the original values that have been aggregated
2. The **count** of all the original values aggregated,
3. The **minimum** value among them,
4. The **maximum** value among them,
5. Their **anomaly rate**, i.e., the count of values that were detected as outliers based on the currently trained models for the metric
6. A **timestamp**, which is the equal to the **end time** of the last point aggregated,
7. A **duration**, which is the duration between the **first time** of the first point aggregated to the **end time** of the last point aggregated.

This design allows Netdata to accurately know the **average**, **minimum**, **maximum** and **anomaly rate** values even when using higher tiers to satisfy a query.

### Pages

Data points are organized into **pages**, i.e., segments of contiguous data collections of the same metric.

Each page:

1. Contains contiguous **data points** of a single metric.
2. Contains **data points** having the same **update every**. If a metric changes **update every** on the fly, the page is flushed and a new one with the new **update every** is created. If a data collection is missed, a **gap point** is inserted into the page, so that the data points in a page remain contiguous.
3. Has a **start time**, which is equivalent to the **end time** of the first data point stored into it,
4. Has an **end time**, which is equal to the **end time** of the last data point stored into it,
5. Has an **update every**, common for all points in the page.

A **page** is a simple array of values. Each slot in the array has a **timestamp** implied by its position in the array, and each value stored represents the **data point** for that time, for the metric the page belongs to.

This fixed step page design allows Netdata to collect several millions of points per second and pack all the values in a compact form with minimal metadata overhead.

#### Hot Pages

While a metric is collected, there is one **hot page** in memory for each of the configured tiers. Values collected for a metric are appended to its **hot page** until that page becomes full.

#### Dirty Pages

Once a **hot page** is full, it becomes a **dirty page**, and it is scheduled for immediate **flushing** (saving) to disk.

#### Clean Pages

Flushed (saved) pages are **clean pages**, i.e., read-only pages that reside primarily on disk, and are loaded on demand to satisfy data queries.

#### Pages Configuration

Pages are configured like this:

| Attribute                                                                             |                 Tier0                 |                              Tier1                              |                              Tier2                              |
|---------------------------------------------------------------------------------------|:-------------------------------------:|:---------------------------------------------------------------:|:---------------------------------------------------------------:|
| Point Size in Memory, in Bytes                                                        |                   4                   |                               16                                |                               16                                |
| Point Size on Disk, in Bytes<br/><small>after LZ4 compression, on the average</small> |                   1                   |                                4                                |                                4                                |
| Page Size in Bytes                                                                    | 4096<br/><small>2048 in 32bit</small> |              2048<br/><small>1024 in 32bit</small>              |               384<br/><small>192 in 32bit</small>               |
| Collections per Point                                                                 |                   1                   | 60x Tier0<br/><small>configurable in<br/>`netdata.conf`</small> | 60x Tier1<br/><small>configurable in<br/>`netdata.conf`</small> |
| Points per Page                                                                       | 1024<br/><small>512 in 32bit</small>  |               128<br/><small>64 in 32bit</small>                |                24<br/><small>12 in 32bit</small>                |

### Files

To minimize the amount of data written to disk and the amount of storage required for storing metrics, Netdata aggregates up to 64 **dirty pages** of independent metrics, packs them all together into one bigger buffer, compresses this buffer with LZ4 (about 75% savings on the average) and commits a transaction to the disk files.

#### Extents

This collection of 64 pages that is packed and compressed together is called an **extent**. Netdata tries to store together, in the same **extent**, metrics that are meant to be "close". Dimensions of the same chart are such. They are usually queried together, so it is beneficial to have them in the same **extent** to read all of them at once at query time.

#### Datafiles

Multiple **extents** are appended to **datafiles** (filename suffix `.ndf`), until these **datafiles** become full. The size of each **datafile** is determined automatically by Netdata. The minimum for each **datafile** is 4MB and the maximum 512MB. Depending on the amount of disk space configured for each tier, Netdata will decide a **datafile** size trying to maintain about 50 datafiles for the whole database, within the limits mentioned (4MB min, 512MB max per file). The maximum number of datafiles supported is 65536, and therefore the maximum database size (per tier) that Netdata can support is 32TB.

#### Journal Files

Each **datafile** has two **journal files** with metadata related to the stored data in the **datafile**.

- **journal file v1**, with filename suffix `.njf`, holds information about the transactions in its **datafile** and provides the ability to recover as much data as possible, in case either the datafile or the journal files get corrupted. This journal file has a maximum transaction size of 4KB, so in case data are corrupted on disk transactions of 4KB are lost. Each transaction holds the metadata of one **extent** (this is why DBENGINE supports up to 64 pages per extent).

- **journal file v2**, with filename suffix `.njfv2`, which is a disk-based index for all the **pages** and **extents**. This file is memory mapped at runtime and is consulted to find where the data of a metric are in the datafile. This journal file is automatically re-created from **journal file v1** if it is missing. It is safe to delete these files (when Netdata does not run). Netdata will re-create them on the next run. Journal files v2 are supported in Netdata Agents with version `netdata-1.37.0-115-nightly`. Older versions maintain the journal index in memory.

#### Database Rotation

Database rotation is achieved by deleting the oldest **datafile** (and its journals) and creating a new one (with its journals).

Data on disk are append-only. There is no way to delete, add, or update data in the middle of the database. If data are not useful for whatever reason, Netdata can be instructed to ignore these data. They will eventually be deleted from the disk when the database is rotated. New data are always appended.

#### Tiers

Tiers are supported in Netdata Agents with version `netdata-1.35.0.138.nightly` and greater.

**datafiles** and **journal files** are organized in **tiers**. All tiers share the same metrics and same collected values.

- **tier 0** is the high resolution tier that stores the collected data at the frequency they are collected.
- **tier 1** by default aggregates 60 values of **tier 0**.
- **tier 2** by default aggregates 60 values of **tier 1**, or 3600 values of **tier 0**.

Updating the higher **tiers** is automated, and it happens in real-time while data are being collected for **tier 0**.

When the Netdata Agent starts, during the first data collection of each metric, higher tier are automatically **backfilled** with
data from lower tiers, so that the aggregation they provide will be accurate.

Configuring how the number of tiers and the disk space allocated to each tier is how you can
[change how long netdata stores metrics](/src/database/CONFIGURATION.md#tiers).

### Data loss

Until **hot pages** and **dirty pages** are **flushed** to disk, they are at risk (e.g., due to a crash, or
power failure), as they are stored only in memory.

The supported way of ensuring high data availability is the use of Netdata Parents to stream the data in real-time to
multiple other Netdata Agents.

## Memory requirements and retention

See [change how long netdata stores metrics](/src/database/CONFIGURATION.md#tiers)

#### Exceptions

Netdata has several protection mechanisms to prevent the use of more memory (than the above), by incrementally fetching data from disk and aggressively evicting old data to make Room for new data, but still memory may grow beyond the above limit under the following conditions:

1. The number of pages concurrently used in queries does not fit in the above size. This can happen when multiple queries of unreasonably long time-frames run on lower, higher resolution tiers. The Netdata query planner attempts to avoid such situations by gradually loading pages, but still under extreme conditions, the system may use more memory to satisfy these queries.

2. The disks that host Netdata files are extremely slow for the workload required by the database so that data cannot be flushed to disk quickly to free memory. Netdata will automatically spawn more flushing workers in an attempt to parallelize and speed up flushing, but still if the disks cannot write the data quickly enough, they will remain in memory until they are written to disk.

### Caches

DBENGINE stores metric data to disk. To achieve high performance even under severe stress, it uses several layers of caches.

#### Main Cache

Stores page data. It is the primary storage of hot and dirty pages (before they are saved to disk), and its clean queue is the LRU cache for speeding up queries.

The entire DBENGINE is designed to use the hot queue size (the currently collected metrics) as the key for sizing all its memory consumption. We call this feature **memory ballooning**. More collected metrics, bigger main cache and vice versa.

In the equation:

```
memory in KiB = METRICS x (TIERS - 1) x 4KiB x 2 + 32768 KiB
```

the part `METRICS x (TIERS - 1) x 4KiB` is an estimate for the max hot size of the main cache. Tier 0 pages are 4KiB, but tier 1 pages are 2 KiB and tier 2 pages are 384 bytes. So a single metric in 3 tiers uses 4096 + 2048 + 384 = 6528 bytes. The equation estimates 8192 per metric, which includes cache internal structures and leaves some spare.

Then `x 2` is the worst case estimate for the dirty queue. If all collected metrics (hot) become available for saving at once, to avoid stopping data collection, all their pages will become dirty and new hot pages will be created instantly. To save memory, when Netdata starts, DBENGINE allocates randomly smaller pages for metrics, to spread their completion evenly across time.

The memory we saved with the above is used to improve the LRU cache. So, although we reserved 32MiB for the LRU, in bigger setups (Netdata Parents) the LRU grows a lot more, within the limits of the equation.

In practice, the main cache sizes itself with `hot x 1.5` instead of `hot x 2`. The reason is that 5% of the main cache is reserved for expanding open cache, 5% for expanding extent cache, and we need Room for the extensive buffers that are allocated in these setups. When the main cache exceeds `hot x 1.5` it enters a mode of critical evictions, and aggressively frees pages from the LRU to maintain a healthy memory footprint within its design limits.

#### Open Cache

Stores metadata about on disk pages. Not the data itself. Only metadata about the location of the data on disk.

Its primary use is to index information about the open datafile, the one that still accepts new pages. Once that datafile becomes full, all the hot pages of the open cache are indexed in journal v2 files.

The clean queue is an LRU for reducing the journal v2 scans during querying.

Open cache uses memory ballooning too, like the main cache, based on its own hot pages. Open cache hot size is mainly controlled by the size of the open datafile. This is why on netdata versions with journal files v2, we decreased the maximum datafile size from 1GB to 512MB, and we increased the target number of datafiles from 20 to 50.

On bigger setups open cache will get a bigger LRU by automatically sizing it (the whole open cache) to 5% to the size of (the whole) main cache.

#### Extent Cache

Caches compressed **extent** data, to avoid reading too repeatedly the same data from disks.

### Shared Memory

Journal v2 indexes are mapped into memory. Netdata attempts to minimize shared memory use by instructing the kernel about the use of these files, or even unmounting them when they are not needed.

The time-ranges of the queries running control the amount of shared memory required.

## Metrics Registry

DBENGINE uses 150 bytes of memory for every metric for which retention is maintained but is not currently being collected.




