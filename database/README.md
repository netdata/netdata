<!--
title: "Database"
description: "The Netdata Agent leverages multiple, user-configurable time-series databases that use RAM and/or disk to store metrics on any type of node."
custom_edit_url: https://github.com/netdata/netdata/edit/master/database/README.md
-->

# Database

Netdata is fully capable of long-term metrics storage, at per-second granularity, via its default database engine
(`dbengine`). But to remain as flexible as possible, Netdata supports several storage options:

1. `dbengine`, (the default) data are in database files. The [Database Engine](/database/engine/README.md) works like a
   traditional database. There is some amount of RAM dedicated to data caching and indexing and the rest of the data
   reside compressed on disk. The number of history entries is not fixed in this case, but depends on the configured
   disk space and the effective compression ratio of the data stored. This is the **only mode** that supports changing
   the data collection update frequency (`update every`) **without losing** the previously stored metrics. For more
   details see [here](/database/engine/README.md).

2. `ram`, data are purely in memory. Data are never saved on disk. This mode uses `mmap()` and supports [KSM](#ksm).

3. `save`, data are only in RAM while Netdata runs and are saved to / loaded from disk on Netdata restart. It also
   uses `mmap()` and supports [KSM](#ksm).

4. `map`, data are in memory mapped files. This works like the swap. When Netdata writes data on its memory, the Linux
   kernel marks the related memory pages as dirty and automatically starts updating them on disk. Unfortunately we
   cannot control how frequently this works. The Linux kernel uses exactly the same algorithm it uses for its swap
   memory. This mode uses `mmap()` but does not support [KSM](#ksm). _Keep in mind though, this option will have a
   constant write on your disk._

5. `alloc`, like `ram` but it uses `calloc()` and does not support [KSM](#ksm). This mode is the fallback for all others
   except `none`.

6. `none`, without a database (collected metrics can only be streamed to another Netdata).

## Which database mode to use

The default mode `[db].mode = dbengine` has been designed to scale for longer retentions and is the only mode suitable
for parent Agents in the _Parent - Child_ setups

The other available database modes are designed to minimize resource utilization and should only be considered on
_Parent - Child_ setups at the children side and only when the resource constraints are very strict.

So,

- On a single node setup, use `[db].mode = dbengine`.
- On a [parent - child](/docs/metrics-storage-management/how-streaming-works) setup, use `[db].mode = dbengine` on the
  parent to increase retention and a more resource efficient modes like, `dbengine` with light retention settings or
  `save`, `ram` and `none` modes for the children to minimize resource utilization.

## Choose your database mode

You can select the database mode by editing `netdata.conf` and setting:

```conf
[db]
  # dbengine (default), ram, save (the default if dbengine not available), map (swap like), none, alloc
  mode = dbengine
```

## Netdata Longer Metrics Retention

Metrics retention is controlled only by the disk space allocated to storing metrics. But it also affects the memory and
CPU required by the agent to query longer timeframes.

Since Netdata Agents usually run on the edge, on production systems, Netdata Agent **parents** should be considered.
When having a [**parent - child**](/docs/metrics-storage-management/how-streaming-works.md) setup, the child (the
Netdata Agent running on a production system) delegates all its functions, including longer metrics retention and
querying, to the parent node that can dedicate more resources to this task. A single Netdata Agent parent can centralize
multiple children Netdata Agents (dozens, hundreds, or even thousands depending on its available resources).

## Running Netdata on embedded devices

Embedded devices usually have very limited RAM resources available.

There are 2 settings for you to tweak:

1. `[db].update every`, which controls the data collection frequency
2. `[db].retention`, which controls the size of the database in memory (except for `[db].mode = dbengine`)

By default `[db].update every = 1` and `[db].retention = 3600`. This gives you an hour of data with per second updates.

If you set `[db].update every = 2` and `[db].retention = 1800`, you will still have an hour of data, but collected once
every 2 seconds. This will **cut in half** both CPU and RAM resources consumed by Netdata. Of course experiment a bit.
On very weak devices you might have to use `[db].update every = 5` and `[db].retention = 720` (still 1 hour of data, but
1/5 of the CPU and RAM resources).

You can also disable [data collection plugins](/collectors/README.md) you don't need. Disabling such plugins will also
free both CPU and RAM resources.

## Memory optimizations

### KSM

KSM performs memory deduplication by scanning through main memory for physical pages that have identical content, and
identifies the virtual pages that are mapped to those physical pages. It leaves one page unchanged, and re-maps each
duplicate page to point to the same physical page. Netdata offers all its in-memory database to kernel for
deduplication.

In the past KSM has been criticized for consuming a lot of CPU resources. This is true when KSM is used for
deduplicating certain applications, but it is not true for Netdata. Agent's memory is written very infrequently
(if you have 24 hours of metrics in Netdata, each byte at the in-memory database will be updated just once per day). KSM
is a solution that will provide 60+% memory savings to Netdata.

### Enable KSM in kernel

You need to run a kernel compiled with:

```sh
CONFIG_KSM=y
```

When KSM is enabled at the kernel is just available for the user to enable it.

So, if you build a kernel with `CONFIG_KSM=y` you will just get a few files in `/sys/kernel/mm/ksm`. Nothing else
happens. There is no performance penalty (apart I guess from the memory this code occupies into the kernel).

The files that `CONFIG_KSM=y` offers include:

- `/sys/kernel/mm/ksm/run` by default `0`. You have to set this to `1` for the kernel to spawn `ksmd`.
- `/sys/kernel/mm/ksm/sleep_millisecs`, by default `20`. The frequency ksmd should evaluate memory for deduplication.
- `/sys/kernel/mm/ksm/pages_to_scan`, by default `100`. The amount of pages ksmd will evaluate on each run.

So, by default `ksmd` is just disabled. It will not harm performance and the user/admin can control the CPU resources
he/she is willing `ksmd` to use.

### Run `ksmd` kernel daemon

To activate / run `ksmd` you need to run:

```sh
echo 1 >/sys/kernel/mm/ksm/run
echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs
```

With these settings ksmd does not even appear in the running process list (it will run once per second and evaluate 100
pages for de-duplication).

Put the above lines in your boot sequence (`/etc/rc.local` or equivalent) to have `ksmd` run at boot.

### Monitoring Kernel Memory de-duplication performance

Netdata will create charts for kernel memory de-duplication performance, like this:

![image](https://cloud.githubusercontent.com/assets/2662304/11998786/eb23ae54-aab6-11e5-94d4-e848e8a5c56a.png)


