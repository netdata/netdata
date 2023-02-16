<!--
title: "Database engine"
description: "Netdata's highly-efficient database engine use both RAM and disk for distributed, long-term storage of per-second metrics."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/database/engine/README.md"
sidebar_label: "Database engine"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
-->

# DBENGINE

DBENGINE is the time-series database of Netdata.

## Design

### Data Points

**Data points** represent the collected values of metrics.

A **data point** has:

1. A **value**, the data collected for a metric.  There is a special **value** to indicate that the collector failed to collect a valid value, and thus the data point is a **gap**.
2. A **timestamp**, the time it has been collected.
3. A **duration**, the time between this and the previous data collection.
4. A flag which is set when machine-learning categorized the collected value as **anomalous** (an outlier based on the trained models).

Using the **timestamp** and **duration**, Netdata calculates for each point its **start time**, **end time** and **update every**.

For incremental metrics (counters), Netdata interpolates the collected values to align them to the expected **end time** at the microsecond level,  absorbing data collection micro-latencies.

When data points are stored in higher tiers (time aggregations - see [Tiers](#Tiers) below), each data point has:

1. The **sum** of the original values that have been aggregated,
2. The **count**  of all the original values aggregated,
3. The **minimum** value among them,
4. The **maximum** value among them,
5. Their **anomaly rate**, i.e. the count of values that were detected as outliers based on the currently trained models for the metric,
6. A **timestamp**, which is the equal to the **end time** of the last point aggregated,
7. A **duration**, which is the duration between the **first time** of the first point aggregated to the **end time** of the last point aggregated.

This design allows Netdata to accurately know the **average**, **minimum**, **maximum** and **anomaly rate** values even when using higher tiers to satisfy a query.

### Pages
Data points are organized into **pages**, i.e. segments of contiguous data collections of the same metric.

Each page:

1. Contains contiguous **data points** of a single metric.
2. Contains **data points** having the same **update every**. If a metric changes **update every** on the fly, the page is flushed and a new one with the new **update every** is created. If a data collection is missed, a **gap point** is inserted into the page, so that the data points in a page remain contiguous.
3. Has a **start time**, which is equivalent to the **end time** of the first data point stored into it,
4. Has an **end time**, which is equal to the **end time** of the last data point stored into it,
5. Has an **update every**, common for all points in the page.

A **page** is a simple array of values. Each slot in the array has a **timestamp** implied by its position in the array, and each value stored represents the **data point** for that time, for the metric the page belongs to.

This simple fixed step page design allows Netdata to collect several millions of points per second and pack all the values in a compact form with minimal metadata overhead.

#### Hot Pages

While a metric is collected, there is one **hot page** in memory for each of the configured tiers. Values collected for a metric are appended to its **hot page** until that page becomes full.

#### Dirty Pages

Once a **hot page** is full, it becomes a **dirty page**, and it is scheduled for immediate **flushing** (saving) to disk.

#### Clean Pages

Flushed (saved) pages are **clean pages**, i.e. read-only pages that reside primarily on disk, and are loaded on demand to satisfy data queries.

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

Data on disk are append-only. There is no way to delete, add, or update data in the middle of the database. If data are not useful for whatever reason, Netdata can be instructed to ignore these data. They will eventually be deleted from disk when the database is rotated. New data are always appended.

#### Tiers

Tiers are supported in Netdata Agents with version `netdata-1.35.0.138.nightly` and greater.

**datafiles** and **journal files** are organized in **tiers**. All tiers share the same metrics and same collected values.

- **tier 0** is the high resolution tier that stores the collected data at the frequency they are collected.
- **tier 1** by default aggregates 60 values of **tier 0**.
- **tier 2** by default aggregates 60 values of **tier 1**, or 3600 values of **tier 0**.

Updating the higher **tiers** is automated, and it happens in real-time while data are being collected for **tier 0**.

When the Netdata Agent starts, during the first data collection of each metric, higher tiers are automatically **backfilled** with 
data from lower tiers, so that the aggregation they provide will be accurate.

Configuring how the number of tiers and the disk space allocated to each tier is how you can 
[change how long netdata stores metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md).

### Data Loss

Until **hot pages** and **dirty pages** are **flushed** to disk they are at risk (e.g. due to a crash, or
power failure), as they are stored only in memory.

The supported way of ensuring high data availability is the use of Netdata Parents to stream the data in real-time to
multiple other Netdata agents.

## Memory Requirements

DBENGINE memory is related to the number of metrics concurrently being collected, the retention of the metrics on disk in relation with the queries running, and the number of metrics for which retention is maintained.

### Memory for concurrently collected metrics

DBENGINE is automatically sized to use memory according to this equation:

```
memory in KiB = METRICS x (TIERS - 1) x 4KiB x 2 + 32768 KiB
```

Where:
- `METRICS`: the maximum number of concurrently collected metrics (dimensions) from the time the agent started.
- `TIERS`: the number of storage tiers configured, by default 3 ( `-1` when using 3+ tiers)
- `x 2`, to accommodate room for flushing data to disk
- `x 4KiB`, the data segment size of each metric
- `+ 32768 KiB`, 32 MB for operational caches

So, for 2000 metrics (dimensions) in 3 storage tiers:

```
memory for 2k metrics = 2000 x (3 - 1) x 4 KiB x 2 + 32768 KiB = 64 MiB
```

For 100k concurrently collected metrics in 3 storage tiers:

```
memory for 100k metrics = 100000 x (3 - 1) x 4 KiB x 2 + 32768 KiB = 1.6 GiB
```

#### Exceptions

Netdata has several protection mechanisms to prevent the use of more memory (than the above), by incrementally fetching data from disk and aggressively evicting old data to make room for new data, but still memory may grow beyond the above limit under the following conditions:

1. The number of pages concurrently used in queries do not fit the in the above size. This can happen when multiple queries of unreasonably long time-frames run on lower, higher resolution, tiers. The Netdata query planner attempts to avoid such situations by gradually loading pages, but still under extreme conditions the system may use more memory to satisfy these queries.

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

Then `x 2` is the worst case estimate for the dirty queue. If all collected metrics (hot) become available for saving at once, to avoid stopping data collection all their pages will become dirty and new hot pages will be created instantly. To save memory, when Netdata starts, DBENGINE allocates randomly smaller pages for metrics, to spread their completion evenly across time.

The memory we saved with the above is used to improve the LRU cache. So, although we reserved 32MiB for the LRU, in bigger setups (Netdata Parents) the LRU grows a lot more, within the limits of the equation.

In practice, the main cache sizes itself with `hot x 1.5` instead of `host x 2`. The reason is that 5% of main cache is reserved for expanding open cache, 5% for expanding extent cache and we need room for the extensive buffers that are allocated in these setups. When the main cache exceeds `hot x 1.5` it enters a mode of critical evictions, and aggresively frees pages from the LRU to maintain a healthy memory footprint within its design limits.

#### Open Cache

Stores metadata about on disk pages. Not the data itself. Only metadata about the location of the data on disk.

Its primary use is to index information about the open datafile, the one that still accepts new pages. Once that datafile becomes full, all the hot pages of the open cache are indexed in journal v2 files.

The clean queue is an LRU for reducing the journal v2 scans during quering.

Open cache uses memory ballooning too, like the main cache, based on its own hot pages. Open cache hot size is mainly controlled by the size of the open datafile. This is why on netdata versions with journal files v2, we decreased the maximum datafile size from 1GB to 512MB and we increased the target number of datafiles from 20 to 50.

On bigger setups open cache will get a bigger LRU by automatically sizing it (the whole open cache) to 5% to the size of (the whole) main cache.

#### Extent Cache

Caches compressed **extent** data, to avoid reading too repeatedly the same data from disks.


### Shared Memory

Journal v2 indexes are mapped into memory. Netdata attempts to minimize shared memory use by instructing the kernel about the use of these files, or even unmounting them when they are not needed.

The time-ranges of the queries running control the amount of shared memory required.

## Metrics Registry

DBENGINE uses 150 bytes of memory for every metric for which retention is maintained but is not currently being collected.

---

--- OLD DOCS BELOW THIS POINT ---

---


## Legacy configuration

### v1.35.1 and prior

These versions of the Agent do not support [Tiers](#Tiers). You could change the metric retention for the parent and
all of its children only with the `dbengine multihost disk space MB` setting. This setting accounts the space allocation
for the parent node and all of its children.

To configure the database engine, look for the `page cache size MB` and `dbengine multihost disk space MB` settings in
the `[db]` section of your `netdata.conf`.

```conf
[db]
    dbengine page cache size MB = 32
    dbengine multihost disk space MB = 256
```

### v1.23.2 and prior

_For Netdata Agents earlier than v1.23.2_, the Agent on the parent node uses one dbengine instance for itself, and another instance for every child node it receives metrics from. If you had four streaming nodes, you would have five instances in total (`1 parent + 4 child nodes = 5 instances`).

The Agent allocates resources for each instance separately using the `dbengine disk space MB` (**deprecated**) setting. If `dbengine disk space MB`(**deprecated**) is set to the default `256`, each instance is given 256 MiB in disk space, which means the total disk space required to store all instances is, roughly, `256 MiB * 1 parent * 4 child nodes = 1280 MiB`.

#### Backward compatibility

All existing metrics belonging to child nodes are automatically converted to legacy dbengine instances and the localhost
metrics are transferred to the multihost dbengine instance.

All new child nodes are automatically transferred to the multihost dbengine instance and share its page cache and disk
space. If you want to migrate a child node from its legacy dbengine instance to the multihost dbengine instance, you
must delete the instance's directory, which is located in `/var/cache/netdata/MACHINE_GUID/dbengine`, after stopping the
Agent.

##### Information

For more information about setting `[db].mode` on your nodes, in addition to other streaming configurations, see
[streaming](https://github.com/netdata/netdata/blob/master/streaming/README.md).

## Requirements & limitations

### Memory

Using database mode `dbengine` we can overcome most memory restrictions and store a dataset that is much larger than the
available memory.

There are explicit memory requirements **per** DB engine **instance**:

- The total page cache memory footprint will be an additional `#dimensions-being-collected x 4096 x 2` bytes over what
  the user configured with `dbengine page cache size MB`.


- an additional `#pages-on-disk x 4096 x 0.03` bytes of RAM are allocated for metadata.

    - roughly speaking this is 3% of the uncompressed disk space taken by the DB files.

    - for very highly compressible data (compression ratio > 90%) this RAM overhead is comparable to the disk space
      footprint.

An important observation is that RAM usage depends on both the `page cache size` and the `dbengine multihost disk space`
options.

You can use
our [database engine calculator](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics)
to validate the memory requirements for your particular system(s) and configuration (**out-of-date**).

### Disk space

There are explicit disk space requirements **per** DB engine **instance**:

- The total disk space footprint will be the maximum between `#dimensions-being-collected x 4096 x 2` bytes or what the
  user configured with `dbengine multihost disk space` or `dbengine disk space`.

### File descriptor

The Database Engine may keep a **significant** amount of files open per instance (e.g. per streaming child or parent
server). When configuring your system you should make sure there are at least 50 file descriptors available per
`dbengine` instance.

Netdata allocates 25% of the available file descriptors to its Database Engine instances. This means that only 25% of
the file descriptors that are available to the Netdata service are accessible by dbengine instances. You should take
that into account when configuring your service or system-wide file descriptor limits. You can roughly estimate that the
Netdata service needs 2048 file descriptors for every 10 streaming child hosts when streaming is configured to use
`[db].mode = dbengine`.

If for example one wants to allocate 65536 file descriptors to the Netdata service on a systemd system one needs to
override the Netdata service by running `sudo systemctl edit netdata` and creating a file with contents:

```sh
[Service]
LimitNOFILE=65536
```

For other types of services one can add the line:

```sh
ulimit -n 65536
```

at the beginning of the service file. Alternatively you can change the system-wide limits of the kernel by changing
`/etc/sysctl.conf`. For linux that would be:

```conf
fs.file-max = 65536
```

In FreeBSD and OS X you change the lines like this:

```conf
kern.maxfilesperproc=65536
kern.maxfiles=65536
```

You can apply the settings by running `sysctl -p` or by rebooting.

## Files

With the DB engine mode the metric data are stored in database files. These files are organized in pairs, the datafiles  
and their corresponding journalfiles, e.g.:

```sh
datafile-1-0000000001.ndf
journalfile-1-0000000001.njf
datafile-1-0000000002.ndf
journalfile-1-0000000002.njf
datafile-1-0000000003.ndf
journalfile-1-0000000003.njf
...
```

They are located under their host's cache directory in the directory `./dbengine` (e.g. for localhost the default
location is `/var/cache/netdata/dbengine/*`). The higher numbered filenames contain more recent metric data. The user
can safely delete some pairs of files when Netdata is stopped to manually free up some space.

_Users should_ **back up** _their `./dbengine` folders if they consider this data to be important._ You can also set up
one or more [exporting connectors](https://github.com/netdata/netdata/blob/master/exporting/README.md) to send your Netdata metrics to other databases for long-term
storage at lower granularity.

## Operation

The DB engine stores chart metric values in 4096-byte pages in memory. Each chart dimension gets its own page to store
consecutive values generated from the data collectors. Those pages comprise the **Page Cache**.

When those pages fill up, they are slowly compressed and flushed to disk. It can
take `4096 / 4 = 1024 seconds = 17 minutes`, for a chart dimension that is being collected every 1 second, to fill a
page. Pages can be cut short when we stop Netdata or the DB engine instance so as to not lose the data. When we query
the DB engine for data we trigger disk read I/O requests that fill the Page Cache with the requested pages and
potentially evict cold (not recently used)
pages.

When the disk quota is exceeded the oldest values are removed from the DB engine at real time, by automatically deleting
the oldest datafile and journalfile pair. Any corresponding pages residing in the Page Cache will also be invalidated
and removed. The DB engine logic will try to maintain between 10 and 20 file pairs at any point in time.

The Database Engine uses direct I/O to avoid polluting the OS filesystem caches and does not generate excessive I/O
traffic so as to create the minimum possible interference with other applications.

## Evaluation

We have evaluated the performance of the `dbengine` API that the netdata daemon uses internally. This is **not** the web
API of netdata. Our benchmarks ran on a **single** `dbengine` instance, multiple of which can be running in a Netdata
parent node. We used a server with an AMD Ryzen Threadripper 2950X 16-Core Processor and 2 disk drives, a Seagate
Constellation ES.3 2TB magnetic HDD and a SAMSUNG MZQLB960HAJR-00007 960GB NAND Flash SSD.

For our workload, we defined 32 charts with 128 metrics each, giving us a total of 4096 metrics. We defined 1 worker
thread per chart (32 threads) that generates new data points with a data generation interval of 1 second. The time axis
of the time-series is emulated and accelerated so that the worker threads can generate as many data points as possible
without delays.

We also defined 32 worker threads that perform queries on random metrics with semi-random time ranges. The starting time
of the query is randomly selected between the beginning of the time-series and the time of the latest data point. The
ending time is randomly selected between 1 second and 1 hour after the starting time. The pseudo-random numbers are
generated with a uniform distribution.

The data are written to the database at the same time as they are read from it. This is a concurrent read/write mixed
workload with a duration of 60 seconds. The faster `dbengine` runs, the bigger the dataset size becomes since more data
points will be generated. We set a page cache size of 64MiB for the two disk-bound scenarios. This way, the dataset size
of the metric data is much bigger than the RAM that is being used for caching so as to trigger I/O requests most of the
time. In our final scenario, we set the page cache size to 16 GiB. That way, the dataset fits in the page cache so as to
avoid all disk bottlenecks.

The reported numbers are the following:

| device | page cache | dataset | reads/sec | writes/sec |
|:------:|:----------:|--------:|----------:|-----------:|
|  HDD   |   64 MiB   | 4.1 GiB |      813K |      18.0M |
|  SSD   |   64 MiB   | 9.8 GiB |      1.7M |      43.0M |
|  N/A   |   16 GiB   | 6.8 GiB |    118.2M |      30.2M |

where "reads/sec" is the number of metric data points being read from the database via its API per second and
"writes/sec" is the number of metric data points being written to the database per second.

Notice that the HDD numbers are pretty high and not much slower than the SSD numbers. This is thanks to the database
engine design being optimized for rotating media. In the database engine disk I/O requests are:

- asynchronous to mask the high I/O latency of HDDs.
- mostly large to reduce the amount of HDD seeking time.
- mostly sequential to reduce the amount of HDD seeking time.
- compressed to reduce the amount of required throughput.

As a result, the HDD is not thousands of times slower than the SSD, which is typical for other workloads.

An interesting observation to make is that the CPU-bound run (16 GiB page cache) generates fewer data than the SSD run
(6.8 GiB vs 9.8 GiB). The reason is that the 32 reader threads in the SSD scenario are more frequently blocked by I/O,
and generate a read load of 1.7M/sec, whereas in the CPU-bound scenario the read load is 70 times higher at 118M/sec.
Consequently, there is a significant degree of interference by the reader threads, that slow down the writer threads.
This is also possible because the interference effects are greater than the SSD impact on data generation throughput.
