<!--
title: "Netdata Longer Metrics Retention"
description: ""
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/longer-metrics-storage.md
-->

# Netdata Longer Metrics Retention

Metrics retention affects 3 parameters on the operation of a Netdata Agent:

1. The disk space required to store the metrics.
2. The memory the Netdata Agent will require to have that retention available for queries.
3. The CPU resources that will be required to query longer time-frames.

As retention increases, the resources required to support that retention increase too.

Since Netdata Agents usually run at the edge, inside production systems, Netdata Agent **parents** should be considered. When having a **parent - child** setup, the child (the Netdata Agent running on a production system) delegates all its functions, including longer metrics retention and querying, to the parent node that can dedicate more resources to this task. A single Netdata Agent parent can centralize multiple children Netdata Agents (dozens, hundreds, or even thousands depending on its available resources). 

## Which database mode to use

Netdata Agents support multiple database modes.

The default mode `[db].mode = dbengine` has been designed to scale for longer retentions.

The other available database modes are designed to minimize resource utilization and should usually be considered on **parent - child** setups at the children side.

So,

* On a single node setup, use `[db].mode = dbengine` to increase retention.
* On a **parent - child** setup, use `[db].mode = dbengine` on the parent to increase retention and a more resource efficient mode (like `save`, `ram` or `none`) for the child to minimize resources utilization.

To use `dbengine`, set this in `netdata.conf` (it is the default):

```
[db]
    mode = dbengine
```

## Tiering

`dbengine` supports tiering. Tiering allows having up to 3 versions of the data:

1. Tier 0 is the high resolution data.
2. Tier 1 is the first tier that samples data every 60 data collections of Tier 0.
3. Tier 2 is the second tier that samples data every 3600 data collections of Tier 0 (60 of Tier 1).

To enable tiering set `[db].storage tiers` in `netdata.conf` (the default is 1, to enable only Tier 0):

```
[db]
    mode = dbengine
    storage tiers = 3
```

## Disk space requirements

Netdata Agents require about 0.34 bytes on disk per database point on Tier 0 and 4 times more on higher tiers (Tier 1 and 2).

### Tier 0 - per second for a week

For 2000 metrics, collected every second and retained for a week, Tier 0 needs: 0.34 bytes x 2000 metrics x 3600 secs per hour x 24 hours per day x 7 days per week = 392 MB.

The setting to control this is in `netdata.conf`:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enable only Tier 0
    storage tiers = 1
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 392
```

By setting it to `392` and restarting the Netdata Agent, this node will start maintaining about a week of data. But pay attention to the number of metrics. If you have more than 2000 metrics on a node, or you need more that an week of high resolution metrics, you may need to adjust this setting accordingly.

### Tier 1 - per minute for a month

Tier 1 is by default sampling the data every 60 points of Tier 0. If Tier 0 is per second, then Tier 1 is per minute.

Tier 1 needs 4 times more storage per point compared to Tier 0, because for every point it stores `min`, `max`, `sum`, `count` and `anomaly rate` (the values are 5, but they require 4 times the storage because `count` and `anomaly rate` are 16-bit integers). The `average` is calculated on the fly at query time using `sum / count`.

For 2000 metrics, with per minute resolution, retained for a month, Tier 1 needs: 0.34 bytes x 4 x 2000 metrics x 60 minutes per hour x 24 hours per day x 30 days per month = 112MB.

Do this in `netdata.conf`:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enable only Tier 0 and Tier 1
    storage tiers = 2
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 392
    
    # Tier 1, per minute data for a month
    dbengine tier 1 multihost disk space MB = 112
```

Once `netdata.conf` is edited, the Netdata Agent needs to be restarted for the changes to take effect.

### Tier 2 - per hour for a year

Tier 2 is by default sampling data every 3600 points of Tier 0 (60 of Tier 1). If Tier 0 is per second, then Tier 2 is per hour.

The storage requirements are the same to Tier 1.

For 2000 metrics, with per hour resolution, retained for a year, Tier 2 needs: 0.34 bytes x 4 x 2000 metrics x 24 hours per day x 365 days per year = 23MB.


Do this in `netdata.conf`:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enable only Tier 0 and Tier 1
    storage tiers = 3
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 392
    
    # Tier 1, per minute data for a month
    dbengine tier 1 multihost disk space MB = 112

    # Tier 2, per hour data for a year
    dbengine tier 2 multihost disk space MB = 23
```

Once `netdata.conf` is edited, the Netdata Agent needs to be restarted for the changes to take effect.

## Memory requirement

TBD

## TODO


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


