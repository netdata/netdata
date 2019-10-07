# Change how long Netdata stores metrics

Netdata helps you collect thousands of system and application metrics every second, but what about storing them for the
long term?

Many people think Netdata can only store about an hour's worth of real-time metrics, but that's just the default
configuration today. With the right settings, Netdata is quite capable of efficiently storing hours or days worth of
historical, per-second metrics without having to rely on a [backend](../../backends/).

This tutorial gives two options for configuring Netdata to store more metrics. We recommend the [**database
engine**](#using-the-database-engine), as it will soon be the default configuration. However, you can stick with the
current default **round-robin database** if you prefer.

Let's get started.

## Using the database engine

The database engine uses RAM to store recent metrics while also using a "spill to disk" feature that takes advantage of
available disk space for long-term metrics storage.This feature of the database engine allows you to store a much larger
dataset than your system's available RAM.

The database engine will eventually become the default method of retaining metrics, but until then, you can switch to
the database engine by changing a single option.

Edit your `netdata.conf` file and change the `memory mode` setting to `dbengine`:

```conf
[global]
    memory mode = dbengine
```

Next, restart Netdata. On Linux systems, we recommend running `sudo service netdata restart`. You're now using the
database engine!

> Learn more about how we implemented the database engine, and our vision for its future, on our blog: [_How and why
> we're bringing long-term storage to Netdata_](https://blog.netdata.cloud/posts/db-engine/).

What makes the database engine efficient? While it's structured like a traditional database, the database engine splits
data between RAM and disk. The database engine caches and indexes data on RAM to keep memory usage low, and then
compresses older metrics onto disk for long-term storage.

When the Netdata dashboard queries for historical metrics, the database engine will use its cache, stored in RAM, to
return relevant metrics for visualization in charts.

Now, given that the database engine uses _both_ RAM and disk, there are two other settings to consider: `page cache
size` and `dbengine disk space`.

```conf
[global]
    page cache size = 32
    dbengine disk space = 256
```

`page cache size` sets the maximum amount of RAM (in MiB) the database engine will use for caching and indexing.
`dbengine disk space` sets the maximum disk space (again, in MiB) the database engine will use for storing compressed
metrics.

Based on our testing, these default settings will retain about two day's worth of metrics when Netdata collects 2,000
metrics every second.

If you'd like to change these options, read more about the [database engine's memory
footprint](../../database/engine/README.md#memory-requirements).

With the database engine active, you can back up your `/var/cache/netdata/dbengine/` folder to another location for
redundancy.

Now that you know how to switch to the database engine, let's cover the default round-robin database for those who
aren't ready to make the move.

## Using the round-robin database

By default, Netdata uses a round-robin database to store 1 hour of per-second metrics. Here's the default setting for
`history` in the `netdata.conf` file that comes pre-installed with Netdata.

```conf
[global]
    history = 3600
```

One hour has 3,600 seconds, hence the `3600` value!

To increase your historical metrics, you can increase `history` to the number of seconds you'd like to store:

```conf
[global]
    # 2 hours = 2 * 60 * 60 = 7200 seconds
    history = 7200
    # 4 hours = 4 * 60 * 60 = 14440 seconds
    history = 14440
    # 24 hours = 24 * 60 * 60 = 86400 seconds
    history = 86400
```

And so on.

Next, check to see how many metrics Netdata collects on your system, and how much RAM that uses. Visit the Netdata
dashboard and look at the bottom-right corner of the interface. You'll find a sentence similar to the following:

> Every second, Netdata collects 1,938 metrics, presents them in 299 charts and monitors them with 81 alarms. Netdata is
> using 25 MB of memory on **netdata-linux** for 1 hour, 6 minutes and 36 seconds of real-time history.

On this desktop system, using a Ryzen 5 1600 and 16GB of RAM, the round-robin databases uses 25 MB of RAM to store just
over an hour's worth of data for nearly 2,000 metrics.

To increase the `history` option, you need to edit your `netdata.conf` file and increase the `history` setting. In most
installations, you'll find it at `/etc/netdata/netdata.conf`, but some operating systems place it at
`/opt/netdata/etc/netdata/netdata.conf`. 

Use `/etc/netdata/edit-config netdata.conf`, or your favorite text editor, to replace `3600` with the number of seconds
you'd like to store.

You should base this number on two things: How much history you need for your use case, and how much RAM you're willing
to dedicate to Netdata.

> Take care when you change the `history` option on production systems. Netdata is configured to stop its process if
> your system starts running out of RAM, but you can never be too careful. Out of memory situations are very bad.

How much RAM will a longer history use? Let's use a little math.

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
charts](../../web/README.md#using-charts).

And if you'd now like to reduce Netdata's resource usage, view our [performance guide](../../docs/Performance.md) for
our best practices on optimization.
