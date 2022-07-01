<!--
title: "Change how long Netdata stores metrics"
description: "With a single configuration change, the Netdata Agent can store days, weeks, or months of metrics at its famous per-second granularity."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/longer-metrics-storage.md
-->

# Change how long Netdata stores metrics

Netdata helps you collect thousands of system and application metrics every second, but what about storing them for the
long term?

Many people think Netdata can only store about an hour's worth of real-time metrics, but that's simply not true any
more. With the right settings, Netdata is quite capable of efficiently storing hours or days worth of historical,
per-second metrics without having to rely on an [exporting engine](/docs/export/external-databases.md).

This guide gives two options for configuring Netdata to store more metrics. **We recommend the default [database
engine](#using-the-database-engine)**, but you can stick with or switch to the round-robin database if you prefer.

Let's get started.

## Using the database engine

The database engine uses RAM to store recent metrics while also using a "spill to disk" feature that takes advantage of
available disk space for long-term metrics storage. This feature of the database engine allows you to store a much
larger dataset than your system's available RAM.

The database engine is currently the default method of storing metrics, but if you're not sure which database you're
using, check out your `netdata.conf` file and look for the `[db].mode` setting:

```conf
[db]
    mode = dbengine
```

If `[db].mode` is set to anything but `dbengine`, change it and restart Netdata using the standard command for
restarting services on your system. You're now using the database engine!

What makes the database engine efficient? While it's structured like a traditional database, the database engine splits
data between RAM and disk. The database engine caches and indexes data on RAM to keep memory usage low, and then
compresses older metrics onto disk for long-term storage.

When the Netdata dashboard queries for historical metrics, the database engine will use its cache, stored in RAM, to
return relevant metrics for visualization in charts.

Now, given that the database engine uses _both_ RAM and disk, there are two other settings to consider: `page cache
size MB` and `dbengine multihost disk space MB`.

```conf
[db]
    dbengine page cache size MB = 32
    dbengine multihost disk space MB = 256
```

`[db].dbengine page cache size MB` sets the maximum amount of RAM the database engine will use for caching and indexing.
`[db].dbengine multihost disk space MB` sets the maximum disk space the database engine will use for storing
compressed metrics. The default settings retain about four day's worth of metrics on a system collecting 2,000 metrics
every second.

[**See our database engine
calculator**](/docs/store/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics)
to help you correctly set `[db].dbengine multihost disk space MB` based on your needs. The calculator gives an accurate estimate
based on how many child nodes you have, how many metrics your Agent collects, and more.

With the database engine active, you can back up your `/var/cache/netdata/dbengine/` folder to another location for
redundancy.

Now that you know how to switch to the database engine, let's cover the default round-robin database for those who
aren't ready to make the move.

## Using the round-robin database

In previous versions, Netdata used a round-robin database to store 1 hour of per-second metrics. 

To see if you're still using this database, or if you would like to switch to it, open your `netdata.conf` file and see
if `[db].mode` option is set to `save`.

```conf
[db]
    mode = save
```

If `[db].mode` is set to `save`, then you're using the round-robin database. If so, the `[db].retention` option is set to
`3600`, which is the equivalent to 3,600 seconds, or one hour. 

To increase your historical metrics, you can increase `[db].retention` to the number of seconds you'd like to store:

```conf
[db]
    # 2 hours = 2 * 60 * 60 = 7200 seconds
    retention = 7200
    # 4 hours = 4 * 60 * 60 = 14440 seconds
    retention = 14440
    # 24 hours = 24 * 60 * 60 = 86400 seconds
    retention = 86400
```

And so on.

Next, check to see how many metrics Netdata collects on your system, and how much RAM that uses. Visit the Netdata
dashboard and look at the bottom-right corner of the interface. You'll find a sentence similar to the following:

> Every second, Netdata collects 1,938 metrics, presents them in 299 charts and monitors them with 81 alarms. Netdata is
> using 25 MB of memory on **netdata-linux** for 1 hour, 6 minutes and 36 seconds of real-time history.

On this desktop system, using a Ryzen 5 1600 and 16GB of RAM, the round-robin databases uses 25 MB of RAM to store just
over an hour's worth of data for nearly 2,000 metrics.

You should base this number on two things: How much history you need for your use case, and how much RAM you're willing
to dedicate to Netdata.

How much RAM will a longer retention use? Let's use a little math.

The round-robin database needs 4 bytes for every value Netdata collects. If Netdata collects metrics every second,
that's 4 bytes, per second, per metric.

```text
4 bytes * X seconds * Y metrics = RAM usage in bytes
```

Let's assume your system collects 1,000 metrics per second.

```text
4 bytes * 3600 seconds * 1,000 metrics = 14400000 bytes = 14.4 MB RAM
```

With that formula, you can calculate the RAM usage for much larger history settings.

```conf
# 2 hours at 1,000 metrics per second
4 bytes * 7200 seconds * 1,000 metrics = 28800000 bytes = 28.8 MB RAM
# 2 hours at 2,000 metrics per second
4 bytes * 7200 seconds * 2,000 metrics = 57600000 bytes = 57.6 MB RAM
# 4 hours at 2,000 metrics per second
4 bytes * 14440 seconds * 2,000 metrics = 115520000 bytes = 115.52 MB RAM
# 24 hours at 1,000 metrics per second
4 bytes * 86400 seconds * 1,000 metrics = 345600000 bytes = 345.6 MB RAM
```

## What's next?

Now that you have either configured database engine or round-robin database engine to store more metrics, you'll
probably want to see it in action!

For more information about how to pan charts to view historical metrics, see our documentation on [using
charts](/web/README.md#using-charts).

And if you'd now like to reduce Netdata's resource usage, view our [performance
guide](/docs/guides/configure/performance.md) for our best practices on optimization.


