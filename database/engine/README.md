<!--
title: "Database engine"
description: "The highly-efficient database engine stores per-second metrics in RAM and then spills historical metrics to disk long-term storage."
custom_edit_url: https://github.com/netdata/netdata/edit/master/database/engine/README.md
-->

# Database engine

The Database Engine works like a traditional database. It dedicates a certain amount of RAM to data caching and
indexing, while the rest of the data resides compressed on disk. Unlike other [memory modes](/database/README.md), the
amount of historical metrics stored is based on the amount of disk space you allocate and the effective compression
ratio, not a fixed number of metrics collected.

By using both RAM and disk space, the database engine allows for long-term storage of per-second metrics inside of the
Agent itself.

In addition, the database engine is the only memory mode that supports changing the data collection update frequency
(`update_every`) without losing the metrics your Agent already gathered and stored.

## Configuration

To use the database engine, open `netdata.conf` and set `memory mode` to `dbengine`.

```conf
[global]
    memory mode = dbengine
```

To configure the database engine, look for the `page cache size` and `dbengine disk space` settings in the `[global]`
section of your `netdata.conf`. The Agent ignores the `history` setting when using the database engine.

```conf
[global]
    page cache size = 32
    dbengine disk space = 256
```

The above values are the default and minimum values for Page Cache size and DB engine disk space quota. Both numbers are
in **MiB**.

The `page cache size` option determines the amount of RAM in **MiB** dedicated to caching Netdata metric values. The
actual page cache size will be slightly larger than this figure—see the [memory requirements](#memory-requirements)
section for details.

The `dbengine disk space` option determines the amount of disk space in **MiB** that is dedicated to storing Netdata
metric values and all related metadata describing them.

Use the  [**database engine calculator**](https://learn.netdata.cloud/docs/agent/database/calculator) to correctly set
`dbengine disk space` based on your needs. The calculator gives an accurate estimate based on how many slave nodes you
have, how many metrics your Agent collects, and more.

### Streaming metrics to the database engine

When streaming metrics, the Agent on the master node creates one instance of the database engine for itself, and another
instance for every slave node it receives metrics from. If you have four streaming nodes, you will have five instances
in total (`1 master + 4 slaves = 5 instances`).

The Agent allocates resources for each instance separately using the `dbengine disk space` setting. If `dbengine disk
space` is set to the default `256`, each instance is given 256 MiB in disk space, which means the total disk space
required to store all instances is, roughly, `256 MiB * 1 master * 4 slaves = 1280 MiB`. 

See the [database engine calculator](https://learn.netdata.cloud/docs/agent/database/calculator) to help you correctly
set `dbengine disk space` and undertand the toal disk space required based on your streaming setup.

For more information about setting `memory mode` on your nodes, in addition to other streaming configurations, see
[streaming](/streaming/README.md).

### Memory requirements

Using memory mode `dbengine` we can overcome most memory restrictions and store a dataset that is much larger than the
available memory.

There are explicit memory requirements **per** DB engine **instance**, meaning **per** Netdata **node** (e.g. localhost
and streaming recipient nodes):

-   The total page cache memory footprint will be an additional `#dimensions-being-collected x 4096 x 2` bytes over what
    the user configured with `page cache size`.

-   an additional `#pages-on-disk x 4096 x 0.03` bytes of RAM are allocated for metadata.

    -   roughly speaking this is 3% of the uncompressed disk space taken by the DB files.

    -   for very highly compressible data (compression ratio > 90%) this RAM overhead is comparable to the disk space
        footprint.

An important observation is that RAM usage depends on both the `page cache size` and the `dbengine disk space` options.

You can use our [database engine calculator](https://learn.netdata.cloud/docs/agent/database/calculator) to
validate the memory requirements for your particular system(s) and configuration.

### File descriptor requirements

The Database Engine may keep a **significant** amount of files open per instance (e.g. per streaming slave or master
server). When configuring your system you should make sure there are at least 50 file descriptors available per
`dbengine` instance.

Netdata allocates 25% of the available file descriptors to its Database Engine instances. This means that only 25% of
the file descriptors that are available to the Netdata service are accessible by dbengine instances. You should take
that into account when configuring your service or system-wide file descriptor limits. You can roughly estimate that the
Netdata service needs 2048 file descriptors for every 10 streaming slave hosts when streaming is configured to use
`memory mode = dbengine`.

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

With the DB engine memory mode the metric data are stored in database files. These files are organized in pairs, the
datafiles and their corresponding journalfiles, e.g.:

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

When those pages fill up they are slowly compressed and flushed to disk. It can take `4096 / 4 = 1024 seconds = 17
minutes`, for a chart dimension that is being collected every 1 second, to fill a page. Pages can be cut short when we
stop Netdata or the DB engine instance so as to not lose the data. When we query the DB engine for data we trigger disk
read I/O requests that fill the Page Cache with the requested pages and potentially evict cold (not recently used)
pages. 

When the disk quota is exceeded the oldest values are removed from the DB engine at real time, by automatically deleting
the oldest datafile and journalfile pair. Any corresponding pages residing in the Page Cache will also be invalidated
and removed. The DB engine logic will try to maintain between 10 and 20 file pairs at any point in time. 

The Database Engine uses direct I/O to avoid polluting the OS filesystem caches and does not generate excessive I/O
traffic so as to create the minimum possible interference with other applications.

## Evaluation

We have evaluated the performance of the `dbengine` API that the netdata daemon uses internally. This is **not** the
web API of netdata. Our benchmarks ran on a **single** `dbengine` instance, multiple of which can be running in a
netdata master server. We used a server with an AMD Ryzen Threadripper 2950X 16-Core Processor and 2 disk drives, a
Seagate Constellation ES.3 2TB magnetic HDD and a SAMSUNG MZQLB960HAJR-00007 960GB NAND Flash SSD.

For our workload, we defined 32 charts with 128 metrics each, giving us a total of 4096 metrics. We defined 1 worker
thread per chart (32 threads) that generates new data points with a data generation interval of 1 second. The time axis
of the time-series is emulated and accelerated so that the worker threads can generate as many data points as possible
without delays. 

We also defined 32 worker threads that perform queries on random metrics with semi-random time ranges. The
starting time of the query is randomly selected between the beginning of the time-series and the time of the latest data
point. The ending time is randomly selected between 1 second and 1 hour after the starting time. The pseudo-random
numbers are generated with a uniform distribution.

The data are written to the database at the same time as they are read from it. This is a concurrent read/write mixed
workload with a duration of 60 seconds. The faster `dbengine` runs, the bigger the dataset size becomes since more
data points will be generated. We set a page cache size of 64MiB for the two disk-bound scenarios. This way, the dataset
size of the metric data is much bigger than the RAM that is being used for caching so as to trigger I/O requests most
of the time. In our final scenario, we set the page cache size to 16 GiB. That way, the dataset fits in the page cache
so as to avoid all disk bottlenecks.

The reported numbers are the following:

| device | page cache | dataset | reads/sec | writes/sec |
| :----: | :--------: | ------: | --------: | ---------: |
| HDD    | 64 MiB     | 4.1 GiB | 813K      | 18.0M      |
| SSD    | 64 MiB     | 9.8 GiB | 1.7M      | 43.0M      |
| N/A    | 16 GiB     | 6.8 GiB | 118.2M    | 30.2M      |

where "reads/sec" is the number of metric data points being read from the database via its API per second and
"writes/sec" is the number of metric data points being written to the database per second. 

Notice that the HDD numbers are pretty high and not much slower than the SSD numbers. This is thanks to the database
engine design being optimized for rotating media. In the database engine disk I/O requests are:

-   asynchronous to mask the high I/O latency of HDDs.
-   mostly large to reduce the amount of HDD seeking time.
-   mostly sequential to reduce the amount of HDD seeking time.
-   compressed to reduce the amount of required throughput.

As a result, the HDD is not thousands of times slower than the SSD, which is typical for other workloads.

An interesting observation to make is that the CPU-bound run (16 GiB page cache) generates fewer data than the SSD run
(6.8 GiB vs 9.8 GiB). The reason is that the 32 reader threads in the SSD scenario are more frequently blocked by I/O,
and generate a read load of 1.7M/sec, whereas in the CPU-bound scenario the read load is 70 times higher at 118M/sec.
Consequently, there is a significant degree of interference by the reader threads, that slow down the writer threads.
This is also possible because the interference effects are greater than the SSD impact on data generation throughput.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdatabase%2Fengine%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
