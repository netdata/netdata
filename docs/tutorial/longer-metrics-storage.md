# Changing how long Netdata stores metrics

Netdata helps you collect thousands of system and application metrics every second, but what about storing them for the long term?

A lot of people think Netdata can only store about an hour's worth of real-time
metrics, but that's just the default configuration. When it comes to long-term metrics storage, Netdata is actually quite capable.

By configuring Netdata properly, you can store days, months, or even _years_ of per-second metrics without having to rely on a [backend](../../backends/).

This tutorial gives two options for configuring Netdata to store metrics longer. First, you can configure the round-robin database. And second, you can switch to the _extremely efficient_ database engine.

We recommend the [database engine](#using-the-database-engine), as it will soon be the default database, but the choice is yours.

Let's get started.

## Using the round-robin database

By default, Netdata uses a round-robin database to store exactly 1 hour of
per-second metrics. Here's the default setting for `history` in the
`netdata.conf` file that comes pre-installed with Netdata.

```conf
[global]
    history = 3600
```

One hour has 3,600 seconds, hence the `3600` value!

To make Netdata store more metrics, you can increase `history` to the number of seconds you'd like to store:

```conf
[global]
    # 2 hours = 2 * 60 * 60 = 7200 seconds
    history = 7200
    # 4 hours = 4 * 60 * 60 = 14440 seconds
    history = 14440
    # 24 hours = 24 * 60 * 60 = 86400 seconds
    history = 86400
```

And so on. When using the default database, the only restriction on historical
metrics is how much RAM you're willing to dedicate to Netdata.

> A word of warning: Take care when you change these the `history` value,
> especially on production systems. Netdata is configured to stop its own
> process if your system starts running out of RAM, but you can never be too
> careful. Out of memory situations are very bad.

To actually increase the `history` option, you need to edit your `netdata.conf`
file. In most installations, you'll find it at `/etc/netdata/netdata.conf`, but some operating systems place it at `/opt/netdata/etc/netdata`. Use the text
editor of your choosing and replace the `history` setting with the number of seconds you'd like to store.

How much RAM will a longer history use? Well, let's use a little math.

The default database needs 4 bytes for every value. If Netdata collects metrics
every second, that's 4 bytes, per second, per metric.

```text
4 bytes * X seconds * Y metrics = RAM usage in bytes
```

Here's how that works with the default configuration, assuming Netdata is collecting 1,000 metrics per second:

```text
4 bytes * 3600 seconds * 1,000 metrics = 14400000 bytes = 14.4MB RAM
```

With that formula, you can calculate the RAM usage for any system and configuration.

```conf
# 2 hours at 1,000 metrics per second
4 bytes * 7200 seconds * 1,000 metrics = 28800000 bytes = 28.8MB RAM
# 2 hours at 2,000 metrics per second
4 bytes * 7200 seconds * 2,000 metrics = 57600000 bytes = 57.6MB RAM
# 4 hours at 2,000 metrics per second
4 bytes * 14440 seconds * 2,000 metrics = 115520000 bytes = 115.52MB RAM
# 24 hours at 1,000 metrics per second
4 bytes * 86400 seconds * 1,000 metrics = 345600000 bytes = 345.6MB RAM
```

Plug in the number of metrics from your system, and the history you'd like to
store, to see the exact RAM usage for any duration of stored metrics.

Now that you understand how to reconfigure and extend your `history` value,
let's talk about the new **database engine**, which uses both RAM and your
system's disk to store a ton of historical metrics with a minimal footprint.

## Using the database engine

The new database engine, released in v1.15 and undergoing constant improvement, uses both RAM and disk space to efficiently compress metrics. It allows you to store a much larger dataset than your system's available RAM, which means it solves the primary limitation of the default database.

The database engine will eventually become the default method of retaining metrics, but until then, you can switch to the database engine by changing a single option.

To switch to the database engine, edit your `netdata.conf` file and change the `memory mode` setting to `dbengine`:

```conf
[global]
    memory mode = dbengine
```

Restart Netdata and you'll be using the database engine!

> Learn more about how we implemented the database engine and our vision for its
> future on our blog: [_How and why we’re bringing long-term storage to
> Netdata_](https://blog.netdata.cloud/posts/db-engine/).

What makes the database engine efficient? It's a traditional database split between RAM and disk. Caching and indexing data is stored on RAM to keep memory usage low, while metrics are compressed and saved to disk.

When the Netdata dashboard queries for historic metrics, the database engine will use its cache, stored in RAM, to return relevant metrics for visualization in charts.

Now, given that the database engine uses _both_ RAM and disk, there are two other settings to consider: `page cache size` and `dbengine disk space`.

```conf
[global]
    page cache size = 32
    dbengine disk space = 256
```

`page cache size` sets the maximum amount of RAM (in MiB) the database engine will use for caching and indexing. `dbengine disk space` sets the maximum disk space (again, in MiB) the database engine will use for storing compressed metrics.

If you'd like to change these options from their default, read more about the [database engine's memory footprint](../../database/engine/README.md#memory-requirements).

With the database engine active, you can even back up your `/var/cache/netdata/dbengine/` folder, where the database engine stores all the datafiles and their corresponding journalfiles, to another system for safekeeping.

## What's next?

Now that you have either configured the round-robin database or the database engine to store more metrics, you'll probably want to see it in action!

For more information about how to pan charts to view historical metrics, see our documentation on [using charts](../../web/README.md#using-charts).

And if Netdata is now using more resources than you'd like, view our [performance guide](../Performance.md) for details on how to optimize your installation.
