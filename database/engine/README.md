<!--
title: "Database engine"
description: "Netdata's highly-efficient database engine use both RAM and disk for distributed, long-term storage of per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/database/engine/README.md
-->

# Database engine

The Database Engine works like a traditional time series database. Unlike other [database modes](/database/README.md),
the amount of historical metrics stored is based on the amount of disk space you allocate and the effective compression
ratio, not a fixed number of metrics collected.

## Tiering

Tiering is a mechanism of providing multiple tiers of data with
different [granularity on metrics](/docs/store/distributed-data-architecture.md#granularity-of-metrics).

For Netdata Agents with version `netdata-1.35.0.138.nightly` and greater, `dbengine` supports Tiering, allowing almost
unlimited retention of data.


### Metric size

Every Tier down samples the exact lower tier (lower tiers have greater resolution). You can have up to 5
Tiers **[0. . 4]** of data (including the Tier 0, which has the highest resolution)

Tier 0 is the default that was always available in `dbengine` mode. Tier 1 is the first level of aggregation, Tier 2 is
the second, and so on.

Metrics on all tiers except of the _Tier 0_ also store the following five additional values for every point for accurate
representation:

1. The `sum` of the points aggregated
2. The `min` of the points aggregated
3. The `max` of the points aggregated
4. The `count` of the points aggregated (could be constant, but it may not be due to gaps in data collection)
5. The `anomaly_count` of the points aggregated (how many of the aggregated points found anomalous)

Among `min`, `max` and `sum`, the correct value is chosen based on the user query. `average` is calculated on the fly at
query time.

### Tiering in a nutshell

The `dbengine` is capable of retaining metrics for years. To further understand the `dbengine` tiering mechanism let's
explore the following configuration.

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enables Tier 1 and Tier 2, Tier 0 is always enabled in dbengine mode
    storage tiers = 3
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 1100
    
    # Tier 1, per minute data for a month
    dbengine tier 1 multihost disk space MB = 330

    # Tier 2, per hour data for a year
    dbengine tier 2 multihost disk space MB = 67
```

For 2000 metrics, collected every second and retained for a week, Tier 0 needs: 1 byte x 2000 metrics x 3600 secs per
hour x 24 hours per day x 7 days per week = 1100MB.

By setting `dbengine multihost disk space MB` to `1100`, this node will start maintaining about a week of data. But pay
attention to the number of metrics. If you have more than 2000 metrics on a node, or you need more that a week of high
resolution metrics, you may need to adjust this setting accordingly.

Tier 1 is by default sampling the data every **60 points of Tier 0**. In our case, Tier 0 is per second, if we want to
transform this information in terms of time then the Tier 1 "resolution" is per minute.

Tier 1 needs four times more storage per point compared to Tier 0. So, for 2000 metrics, with per minute resolution,
retained for a month, Tier 1 needs: 4 bytes x 2000 metrics x 60 minutes per hour x 24 hours per day x 30 days per month
= 330MB.

Tier 2 is by default sampling data every 3600 points of Tier 0 (60 of Tier 1, which is the previous exact Tier). Again
in term of "time" (Tier 0 is per second), then Tier 2 is per hour.

The storage requirements are the same to Tier 1.

For 2000 metrics, with per hour resolution, retained for a year, Tier 2 needs: 4 bytes x 2000 metrics x 24 hours per day
x 365 days per year = 67MB.

## Legacy configuration

### v1.35.1 and prior

These versions of the Agent do not support [Tiering](#Tiering). You could change the metric retention for the parent and
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

_For Netdata Agents earlier than v1.23.2_, the Agent on the parent node uses one dbengine instance for itself, and
another instance for every child node it receives metrics from. If you had four streaming nodes, you would have five
instances in total (`1 parent + 4 child nodes = 5 instances`).

The Agent allocates resources for each instance separately using the `dbengine disk space MB` (**deprecated**) setting.
If
`dbengine disk space MB`(**deprecated**) is set to the default `256`, each instance is given 256 MiB in disk space,
which means the total disk space required to store all instances is,
roughly, `256 MiB * 1 parent * 4 child nodes = 1280 MiB`.

#### Backward compatibility

All existing metrics belonging to child nodes are automatically converted to legacy dbengine instances and the localhost
metrics are transferred to the multihost dbengine instance.

All new child nodes are automatically transferred to the multihost dbengine instance and share its page cache and disk
space. If you want to migrate a child node from its legacy dbengine instance to the multihost dbengine instance, you
must delete the instance's directory, which is located in `/var/cache/netdata/MACHINE_GUID/dbengine`, after stopping the
Agent.

##### Information

For more information about setting `[db].mode` on your nodes, in addition to other streaming configurations, see
[streaming](/streaming/README.md).

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
our [database engine calculator](/docs/store/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics)
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
one or more [exporting connectors](/exporting/README.md) to send your Netdata metrics to other databases for long-term
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


