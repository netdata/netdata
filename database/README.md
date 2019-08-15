# Database

Although `netdata` does all its calculations using `long double`, it stores all values using
a [custom-made 32-bit number](../libnetdata/storage_number/).

So, for each dimension of a chart, Netdata will need: `4 bytes for the value * the entries
of its history`. It will not store any other data for each value in the time series database.
Since all its values are stored in a time series with fixed step, the time each value
corresponds can be calculated at run time, using the position of a value in the round robin database.

The default history is 3.600 entries, thus it will need 14.4KB for each chart dimension.
If you need 1.000 dimensions, they will occupy just 14.4MB.

Of course, 3.600 entries is a very short history, especially if data collection frequency is set
to 1 second. You will have just one hour of data.

For a day of data and 1.000 dimensions, you will need: 86.400 seconds * 4 bytes * 1.000
dimensions = 345MB of RAM.

One option you have to lower this number is to use
**[Memory Deduplication - Kernel Same Page Merging - KSM](#ksm)**. Another possibility is to 
use the **[Database Engine](engine/)**.

## Memory modes

Currently Netdata supports 6 memory modes:

1.  `ram`, data are purely in memory. Data are never saved on disk. This mode uses `mmap()` and
    supports [KSM](#ksm).

2.  `save`, (the default) data are only in RAM while Netdata runs and are saved to / loaded from
    disk on Netdata restart. It also uses `mmap()` and supports [KSM](#ksm).

3.  `map`, data are in memory mapped files. This works like the swap. Keep in mind though, this
    will have a constant write on your disk. When Netdata writes data on its memory, the Linux kernel
    marks the related memory pages as dirty and automatically starts updating them on disk.
    Unfortunately we cannot control how frequently this works. The Linux kernel uses exactly the
    same algorithm it uses for its swap memory. Check below for additional information on running a
    dedicated central Netdata server. This mode uses `mmap()` but does not support [KSM](#ksm).

4.  `none`, without a database (collected metrics can only be streamed to another Netdata).

5.  `alloc`, like `ram` but it uses `calloc()` and does not support [KSM](#ksm). This mode is the
    fallback for all others except `none`.

6.  `dbengine`, data are in database files. The [Database Engine](engine/) works like a traditional
    database. There is some amount of RAM dedicated to data caching and indexing and the rest of
    the data reside compressed on disk. The number of history entries is not fixed in this case,
    but depends on the configured disk space and the effective compression ratio of the data stored.
    For more details see [here](engine/).

You can select the memory mode by editing `netdata.conf` and setting:

```
[global]
    # ram, save (the default, save on exit, load on start), map (swap like)
    memory mode = save

    # the directory where data are saved
    cache directory = /var/cache/netdata
```

## Running Netdata in embedded devices

Embedded devices usually have very limited RAM resources available.

There are 2 settings for you to tweak:

1.  `update every`, which controls the data collection frequency
2.  `history`, which controls the size of the database in RAM

By default `update every = 1` and `history = 3600`. This gives you an hour of data with per
second updates.

If you set `update every = 2` and `history = 1800`, you will still have an hour of data, but
collected once every 2 seconds. This will **cut in half** both CPU and RAM resources consumed
by Netdata. Of course experiment a bit. On very weak devices you might have to use
`update every = 5` and `history = 720` (still 1 hour of data, but 1/5 of the CPU and RAM resources).

You can also disable [data collection plugins](../collectors) you don't need.
Disabling such plugins will also free both CPU and RAM resources.

## Running a dedicated central Netdata server

Netdata allows streaming data between Netdata nodes. This allows us to have a central Netdata
server that will maintain the entire database for all nodes, and will also run health checks/alarms
for all nodes.

For this central Netdata, memory size can be a problem. Fortunately, Netdata supports several
memory modes. **One interesting option** for this setup is `memory mode = map`.

### map

In this mode, the database of Netdata is stored in memory mapped files. Netdata continues to read
and write the database in memory, but the kernel automatically loads and saves memory pages from/to
disk.

**We suggest _not_ to use this mode on nodes that run other applications.** There will always be
dirty memory to be synced and this syncing process may influence the way other applications work.
This mode however is useful when we need a central Netdata server that would normally need huge
amounts of memory. Using memory mode `map` we can overcome all memory restrictions.

There are a few kernel options that provide finer control on the way this syncing works. But before
explaining them, a brief introduction of how Netdata database works is needed.

For each chart, Netdata maps the following files:

1.  `chart/main.db`, this is the file that maintains chart information. Every time data are collected
    for a chart, this is updated.
2.  `chart/dimension_name.db`, this is the file for each dimension. At its beginning there is a
    header, followed by the round robin database where metrics are stored.

So, every time Netdata collects data, the following pages will become dirty:

1.  the chart file
2.  the header part of all dimension files
3.  if the collected metrics are stored far enough in the dimension file, another page will
    become dirty, for each dimension

Each page in Linux is 4KB. So, with 200 charts and 1000 dimensions, there will be 1200 to 2200 4KB
pages dirty pages every second. Of course 1200 of them will always be dirty (the chart header and
the dimensions headers) and 1000 will be dirty for about 1000 seconds (4 bytes per metric, 4KB per
page, so 1000 seconds, or 16 minutes per page).

Hopefully, the Linux kernel does not sync all these data every second. The frequency they are
synced is controlled by `/proc/sys/vm/dirty_expire_centisecs` or the
`sysctl` `vm.dirty_expire_centisecs`. The default on most systems is 3000 (30 seconds).

On a busy server centralizing metrics from 20+ servers you will experience this:

![image](https://cloud.githubusercontent.com/assets/2662304/23834750/429ab0dc-0764-11e7-821a-d7908bc881ac.png)

As you can see, there is quite some stress (this is `iowait`) every 30 seconds.

A simple solution is to increase this time to 10 minutes (60000). This is the same system
with this setting in 10 minutes:

![image](https://cloud.githubusercontent.com/assets/2662304/23834784/d2304f72-0764-11e7-8389-fb830ffd973a.png)

Of course, setting this to 10 minutes means that data on disk might be up to 10 minutes old if you
get an abnormal shutdown.

There are 2 more options to tweak:

1.  `dirty_background_ratio`, by default `10`.
2.  `dirty_ratio`, by default `20`.

These control the amount of memory that should be dirty for disk syncing to be triggered.
On dedicated Netdata servers, you can use: `80` and `90` respectively, so that all RAM is given
to Netdata.

With these settings, you can expect a little `iowait` spike once every 10 minutes and in case
of system crash, data on disk will be up to 10 minutes old.

![image](https://cloud.githubusercontent.com/assets/2662304/23835030/ba4bf506-0768-11e7-9bc6-3b23e080c69f.png)

To have these settings automatically applied on boot, create the file `/etc/sysctl.d/netdata-memory.conf` with these contents:

```
vm.dirty_expire_centisecs = 60000
vm.dirty_background_ratio = 80
vm.dirty_ratio = 90
vm.dirty_writeback_centisecs = 0
```

There is another memory mode to help overcome the memory size problem. What is **most interesting
for this setup** is `memory mode = dbengine`.

### dbengine

In this mode, the database of Netdata is stored in database files. The [Database Engine](engine/)
works like a traditional database. There is some amount of RAM dedicated to data caching and
indexing and the rest of the data reside compressed on disk. The number of history entries is not 
fixed in this case, but depends on the configured disk space and the effective compression ratio
of the data stored.

We suggest to use **this** mode on nodes that also run other applications. The Database Engine uses
direct I/O to avoid polluting the OS filesystem caches and does not generate excessive I/O traffic 
so as to create the minimum possible interference with other applications. Using memory mode
`dbengine` we can overcome most memory restrictions. For more details see [here](engine/).

## KSM

Netdata offers all its round robin database to kernel for deduplication
(except for `memory mode = dbengine`).

In the past KSM has been criticized for consuming a lot of CPU resources.
Although this is true when KSM is used for deduplicating certain applications, it is not true with
netdata, since the Netdata memory is written very infrequently (if you have 24 hours of metrics in
netdata, each byte at the in-memory database will be updated just once per day).

KSM is a solution that will provide 60+% memory savings to Netdata.

### Enable KSM in kernel

You need to run a kernel compiled with:

```sh
CONFIG_KSM=y
```

When KSM is enabled at the kernel is just available for the user to enable it.

So, if you build a kernel with `CONFIG_KSM=y` you will just get a few files in `/sys/kernel/mm/ksm`. Nothing else happens. There is no performance penalty (apart I guess from the memory this code occupies into the kernel).

The files that `CONFIG_KSM=y` offers include:

-   `/sys/kernel/mm/ksm/run` by default `0`. You have to set this to `1` for the kernel to spawn `ksmd`.
-   `/sys/kernel/mm/ksm/sleep_millisecs`, by default `20`. The frequency ksmd should evaluate memory for deduplication.
-   `/sys/kernel/mm/ksm/pages_to_scan`, by default `100`. The amount of pages ksmd will evaluate on each run.

So, by default `ksmd` is just disabled. It will not harm performance and the user/admin can control the CPU resources he/she is willing `ksmd` to use.

### Run `ksmd` kernel daemon

To activate / run `ksmd` you need to run:

```sh
echo 1 >/sys/kernel/mm/ksm/run
echo 1000 >/sys/kernel/mm/ksm/sleep_millisecs
```

With these settings ksmd does not even appear in the running process list (it will run once per second and evaluate 100 pages for de-duplication).

Put the above lines in your boot sequence (`/etc/rc.local` or equivalent) to have `ksmd` run at boot.

## Monitoring Kernel Memory de-duplication performance

Netdata will create charts for kernel memory de-duplication performance, like this:

![image](https://cloud.githubusercontent.com/assets/2662304/11998786/eb23ae54-aab6-11e5-94d4-e848e8a5c56a.png)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdatabase%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
