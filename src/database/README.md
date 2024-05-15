# Database

Netdata is fully capable of long-term metrics storage, at per-second granularity, via its default database engine
(`dbengine`). But to remain as flexible as possible, Netdata supports several storage options:

1. `dbengine`, (the default) data are in database files. The [Database Engine](https://github.com/netdata/netdata/blob/master/src/database/engine/README.md) works like a
   traditional database. There is some amount of RAM dedicated to data caching and indexing and the rest of the data
   reside compressed on disk. The number of history entries is not fixed in this case, but depends on the configured
   disk space and the effective compression ratio of the data stored. This is the **only mode** that supports changing
   the data collection update frequency (`update every`) **without losing** the previously stored metrics. For more
   details see [here](https://github.com/netdata/netdata/blob/master/src/database/engine/README.md).

2. `ram`, data are purely in memory. Data are never saved on disk. This mode uses `mmap()` and supports [KSM](#ksm).

3. `alloc`, like `ram` but it uses `calloc()` and does not support [KSM](#ksm). This mode is the fallback for all others
   except `none`.

4. `none`, without a database (collected metrics can only be streamed to another Netdata).

## Which database mode to use

The default mode `[db].mode = dbengine` has been designed to scale for longer retentions and is the only mode suitable
for parent Agents in the _Parent - Child_ setups

The other available database modes are designed to minimize resource utilization and should only be considered on
[Parent - Child](https://github.com/netdata/netdata/blob/master/docs/observability-centralization-points/README.md) setups at the children side and only when the
resource constraints are very strict.

So,

- On a single node setup, use `[db].mode = dbengine`.
- On a [Parent - Child](https://github.com/netdata/netdata/blob/master/docs/observability-centralization-points/README.md) setup, use `[db].mode = dbengine` on the
  parent to increase retention, and a more resource-efficient mode like, `dbengine` with light retention settings, `ram`, or `none` for the children to minimize resource utilization.

## Choose your database mode

You can select the database mode by editing `netdata.conf` and setting:

```conf
[db]
  # dbengine (default), ram (the default if dbengine not available), alloc, none
  mode = dbengine
```

## Netdata Longer Metrics Retention

Metrics retention is controlled only by the disk space allocated to storing metrics. But it also affects the memory and
CPU required by the agent to query longer timeframes.

Since Netdata Agents usually run on the edge, on production systems, Netdata Agent **parents** should be considered.
When having a [**parent - child**](https://github.com/netdata/netdata/blob/master/docs/observability-centralization-points/README.md) setup, the child (the
Netdata Agent running on a production system) delegates all of its functions, including longer metrics retention and
querying, to the parent node that can dedicate more resources to this task. A single Netdata Agent parent can centralize
multiple children Netdata Agents (dozens, hundreds, or even thousands depending on its available resources).

## Running Netdata on embedded devices

Embedded devices typically have very limited RAM resources available.

There are two settings for you to configure:

1. `[db].update every`, which controls the data collection frequency
2. `[db].retention`, which controls the size of the database in memory (except for `[db].mode = dbengine`)

By default `[db].update every = 1` and `[db].retention = 3600`. This gives you an hour of data with per second updates.

If you set `[db].update every = 2` and `[db].retention = 1800`, you will still have an hour of data, but collected once
every 2 seconds. This will **cut in half** both CPU and RAM resources consumed by Netdata. Of course experiment a bit to find the right setting.
On very weak devices you might have to use `[db].update every = 5` and `[db].retention = 720` (still 1 hour of data, but
1/5 of the CPU and RAM resources).

You can also disable [data collection plugins](https://github.com/netdata/netdata/blob/master/src/collectors/README.md) that you don't need. Disabling such plugins will also
free both CPU and RAM resources.

## Memory optimizations

### KSM

KSM performs memory deduplication by scanning through main memory for physical pages that have identical content, and
identifies the virtual pages that are mapped to those physical pages. It leaves one page unchanged, and re-maps each
duplicate page to point to the same physical page. Netdata offers all of its in-memory database to kernel for
deduplication.

In the past, KSM has been criticized for consuming a lot of CPU resources. This is true when KSM is used for
deduplicating certain applications, but it is not true for Netdata. Agent's memory is written very infrequently
(if you have 24 hours of metrics in Netdata, each byte at the in-memory database will be updated just once per day). KSM
is a solution that will provide 60+% memory savings to Netdata.

### Enable KSM in kernel

To enable KSM in kernel, you need to run a kernel compiled with the following:

```sh
CONFIG_KSM=y
```

When KSM is enabled at the kernel, it is just available for the user to enable it.

If you build a kernel with `CONFIG_KSM=y`, you will just get a few files in `/sys/kernel/mm/ksm`. Nothing else
happens. There is no performance penalty (apart from the memory this code occupies into the kernel).

The files that `CONFIG_KSM=y` offers include:

- `/sys/kernel/mm/ksm/run` by default `0`. You have to set this to `1` for the kernel to spawn `ksmd`.
- `/sys/kernel/mm/ksm/sleep_millisecs`, by default `20`. The frequency ksmd should evaluate memory for deduplication.
- `/sys/kernel/mm/ksm/pages_to_scan`, by default `100`. The amount of pages ksmd will evaluate on each run.

So, by default `ksmd` is just disabled. It will not harm performance and the user/admin can control the CPU resources
they are willing to have used by `ksmd`.

### Run `ksmd` kernel daemon

To activate / run `ksmd,` you need to run the following:

```sh
echo 1 >/sys/kernel/mm/ksm/run
echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs
```

With these settings, ksmd does not even appear in the running process list (it will run once per second and evaluate 100
pages for de-duplication).

Put the above lines in your boot sequence (`/etc/rc.local` or equivalent) to have `ksmd` run at boot.

### Monitoring Kernel Memory de-duplication performance

Netdata will create charts for kernel memory de-duplication performance, the **deduper (ksm)** charts can be seen under the **Memory** section in the Netdata UI.

#### KSM summary

The summary gives you a quick idea of how much savings (in terms of bytes and in terms of percentage) KSM is able to achieve.

![image](https://user-images.githubusercontent.com/24860547/199454880-123ae7c4-071a-4811-95b8-18cf4e4f60a2.png)

#### KSM pages merge performance

This chart indicates the performance of page merging. **Shared** indicates used shared pages, **Unshared** indicates memory no longer shared (pages are unique but repeatedly checked for merging), **Sharing** indicates memory currently shared(how many more sites are sharing the pages, i.e. how much saved) and **Volatile** indicates volatile pages (changing too fast to be placed in a tree).

A high ratio of Sharing to Shared indicates good sharing, but a high ratio of Unshared to Sharing indicates wasted effort.

![image](https://user-images.githubusercontent.com/24860547/199455374-d63fd2c2-e12b-4ddf-947b-35371215eb05.png)

#### KSM savings

This chart shows the amount of memory saved by KSM. **Savings** indicates saved memory. **Offered** indicates memory marked as mergeable.

![image](https://user-images.githubusercontent.com/24860547/199455604-43cd9248-1f6e-4c31-be56-e0b9e432f48a.png)

#### KSM effectiveness

This chart tells you how well KSM is doing at what it is supposed to. It does this by charting the percentage of the mergeable pages that are currently merged.

![image](https://user-images.githubusercontent.com/24860547/199455770-4d7991ff-6b7e-4d96-9d23-33ffc572b370.png)
