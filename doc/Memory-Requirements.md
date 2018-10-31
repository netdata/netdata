# Memory requirements

netdata is a real-time performance monitoring system. Its goal is to help you diagnose and troubleshoot performance problems. Normally, it does not need too much data. The default is to keep one hour of metrics in memory, which depending on number of metrics may lead to 15-25 MB of RAM. This is more than enough for evaluating the current situation and finding the performance issue at hand.

Many users ask for historical performance statistics. Currently netdata is not very efficient in this. It needs a lot of memory (mainly because the metrics are too many - a few thousands per server - and the resolution is too high - per second).

If you want more than a few hours of data in netdata, we suggest to enable KSM (the kernel memory deduper). In the past KSM has been criticized for consuming a lot of CPU resources. Although this is true when KSM is used for deduplicating certain applications, it is not true with netdata, since the netdata memory is written very infrequently (if you have 24 hours of metrics in netdata, each byte at the in-memory database will be updated just once per day). So, KSM is a solution that will provide 60+% memory savings.

Of course, you can always stream netdata metrics to graphite, opentsdb, prometheus, influxdb, kairosdb, etc. So, if you need statistics of past performance, we suggest to use a dedicated time-series database.

---

## Netdata Memory Requirements

Although `netdata` does all its calculations using `long double` (128 bit) arithmetics, it stores all values using a **custom-made 32-bit number**.

This custom-made number can store in 29 bits values from `-167772150000000.0` to  `167772150000000.0` with a precision of 0.00001 (yes, it's a floating point number, meaning that higher integer values have less decimal precision) and 3 bits for flags.

This provides an extremely optimized memory footprint with just 0.0001% max accuracy loss.

### Sizing memory

So, for each dimension of a chart, netdata will need: `4 bytes for the value * the entries of its history`. It will not store any other data for each value in the time series database. Since all its values are stored in a time series with fixed step, the time each value corresponds can be calculated at run time, using the position of a value in the round robin database.

The default history is 3.600 entries, thus it will need 14.4KB for each chart dimension. If you need 1.000 dimensions, they will occupy just 14.4MB.

Of course, 3.600 entries is a very short history, especially if data collection frequency is set to 1 second. You will have just one hour of data.

For a day of data and 1.000 dimensions, you will need: 86.400 seconds * 4 bytes * 1.000 dimensions = 345MB of RAM.

Currently the only option you have to lower this number is to use **[[Memory Deduplication - Kernel Same Page Merging - KSM]]**.

### Memory modes

Currently netdata supports 5 memory modes:

1. `ram`, data are purely in memory. Data are never saved on disk. This mode uses `mmap()` and supports KSM.
2. `save`, (the default) data are only in RAM while netdata runs and are saved to / loaded from disk on netdata restart. It also uses `mmap()` and supports KSM.
3. `map`, data are in memory mapped files. This works like the swap. Keep in mind though, this will have a constant write on your disk. When netdata writes data on its memory, the Linux kernel marks the related memory pages as dirty and automatically starts updating them on disk. Unfortunately we cannot control how frequently this works. The Linux kernel uses exactly the same algorithm it uses for its swap memory. Check below for additional information on running a dedicated central netdata server. This mode uses `mmap()` but does not support KSM.
4. `none`, without a database (collected metrics can only be streamed to another netdata).
5. `alloc`, like `ram` but it uses `calloc()` and does not support KSM. This mode is the fallback for all others except `none`.

You can select the memory mode by editing netdata.conf and setting:

```
[global]
    # ram, save (the default, save on exit, load on start), map (swap like)
    memory mode = save

    # the directory where data are saved
    cache directory = /var/cache/netdata
```

### Running netdata in embedded devices

Embedded devices usually have very limited RAM resources available.

There are 2 settings for you to tweak:

1. `update every`, which controls the data collection frequency
2. `history`, which controls the size of the database in RAM

By default `update every = 1` and `history = 3600`. This gives you an hour of data with per second updates.

If you set `update every = 2` and `history = 1800`, you will still have an hour of data, but collected once every 2 seconds. This will **cut in half** both CPU and RAM resources consumed by netdata. Of course experiment a bit. On very weak devices you might have to use `update every = 5` and `history = 720` (still 1 hour of data, but 1/5 of the CPU and RAM resources).

You can also disable plugins you don't need. Disabling the plugins will also free both CPU and RAM resources.


### running a dedicated central netdata server

netdata allows streaming data between netdata nodes. This allows us to have a central netdata server that will maintain the entire database for all nodes, and will also run health checks/alarms for all nodes.

For this central netdata, memory size can be a problem. Fortunately, netdata supports several memory modes:

1. `memory mode = save` is the default mode, data are maintained in memory and saved to disk when netdata exits.
2. `memory mode = ram`, data are exclusively on memory and never saved on disk.
3. `memory mode = map`, like swap, files are mapped to memory on demand.
4. `memory mode = none`, no local database (used when data are streamed to a remote netdata).

#### `memory mode = map`

In this mode, the database of netdata is stored in memory mapped files. netdata continues to read and write the database in memory, but the kernel automatically loads and saves memory pages from/to disk.

**We suggest _not_ to use this mode on nodes that run other applications.** There will always be dirty memory to be synced and this syncing process may influence the way other applications work. This mode however is ideal when we need a central netdata server that would normally need huge amounts of memory. Using memory mode `map` we can overcome all memory restrictions.

There are a few kernel options that allow us to have finer control on the way this syncing works. But before explaining them, a brief introduction of how netdata database works is needed.

For each chart, netdata maps the following files:

1. `chart/main.db`, this is the file that maintains chart information. Every time data are collected for a chart, this is updated.
2. `chart/dimension_name.db`, this is the file for each dimension. At its beginning there is a header, followed by the round robin database where metrics are stored.

So, every time netdata collects data, the following pages will become dirty:

1. the chart file
2. the header part of all dimension files
3. if the collected metrics are stored far enough in the dimension file, another page will become dirty, for each dimension

Each page in Linux is 4KB. So, with 200 charts and 1000 dimensions, there will be 1200 to 2200 4KB pages dirty pages every second. Of course 1200 of them will always be dirty (the chart header and the dimensions headers) and 1000 will be dirty for about 1000 seconds (4 bytes per metric, 4KB per page, so 1000 seconds, or 16 minutes per page).

Hopefully, the Linux kernel does not sync all these data every second. The frequency they are synced is controlled by `/proc/sys/vm/dirty_expire_centisecs` or the `sysctl` `vm.dirty_expire_centisecs`. The default on most systems is 3000 (30 seconds).

On a busy server centralizing metrics from 20+ servers you will experience this:

![image](https://cloud.githubusercontent.com/assets/2662304/23834750/429ab0dc-0764-11e7-821a-d7908bc881ac.png)

As you can see, there is quite some stress (this is `iowait`) every 30 seconds.

A simple solution is to increase this time to 10 minutes (60000). This is the same system with this setting in 10 minutes:

![image](https://cloud.githubusercontent.com/assets/2662304/23834784/d2304f72-0764-11e7-8389-fb830ffd973a.png)

A lot better.

Of course, setting this to 10 minutes means that data on disk might be up to 10 minutes old if you get an abnormal shutdown.

There are 2 more options to tweak:

1. `dirty_background_ratio`, by default `10`.
2. `dirty_ratio`, by default `20`.

These control the amount of memory that should be dirty for disk syncing to be triggered. On dedicated netdata servers, I pick: `80` and `90` respectively, so that all RAM is given to netdata.

With these settings, you can expect a little `iowait` spike once every 10 minutes and in case of system crash, data on disk will be up to 10 minutes old.

![image](https://cloud.githubusercontent.com/assets/2662304/23835030/ba4bf506-0768-11e7-9bc6-3b23e080c69f.png)

You can see this server live at [https://build.my-netdata.io](https://build.my-netdata.io). 20+ netdata are streaming data to it (check the `my-netdata` menu on the top left of the dashboard).

To have these settings automatically applied on boot, create the file `/etc/sysctl.d/netdata-memory.conf` with these contents:

```
vm.dirty_expire_centisecs = 60000
vm.dirty_background_ratio = 80
vm.dirty_ratio = 90
vm.dirty_writeback_centisecs = 0
```

### The future

We investigate several alternatives to lower the memory requirements of netdata. The best so far is to split the in-memory round robin database in a small **realtime** database (e.g. an hour long) and a larger compressed **archive** database to store longer durations. So (for example) every hour netdata will compress the last hour of data using LZ4 (which is very fast: 350MB/s compression, 1850MB/s decompression) and append these compressed data to an **archive** round robin database. This **archive** database will be saved to disk and loaded back to memory on demand, when a chart is zoomed or panned to the compressed timeframe.
