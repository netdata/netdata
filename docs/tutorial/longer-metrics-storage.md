# Changing how long Netdata stores metrics

Netdata helps you collect thousands of system and application metrics every
second, but what about storing them for the long term?

A lot of people think Netdata can only store about an hour's worth of real-time
metrics. But that's just the default—Netdata is actually quite flexible when it
comes to long-term storage.

The truth is that the only thing holding you back from storing days, months, or
even _years_ of per-second metrics is the amount of system resources you'll let
Netdata use.

This tutorial will give you a few options for deeper storage: first, the default
database, followed by the _new, extremely efficient_ DB engine. Let's get
started.

## Using the default database

By default, Netdata uses a round-robin database to store exactly 1 hour of
per-second metrics. Here's the default setting for `history` in the `netdata.conf` file that comes pre-installed with Netdata.

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

To actually increase the `history` setting, you need to edit your `netdata.conf`
file. In most installations, you'll find it at `/etc/netdata/netdata.conf`.
Other operating systems place it at `/opt/netdata/etc/netdata`. Use the text
editor of your choosing.



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

The new database engine uses RAM for storing metadata about your metrics, while the values themselves are compressed and saved to disk. It'll help you store a much larger dataset than your system's RAM.

> Learn more about how we implemented the database engine and our vision for its
future on our blog: [_How and why we’re bringing long-term storage to
Netdata_](https://blog.netdata.cloud/posts/db-engine/).

To switch to the database engine, edit your `netdata.conf` file and 


The database engine uses RAM for indexing and caching while the rest of your metrics are saved to disk. It's 



In the future, it'll be the default method for storing long-term metrics, but we're still fleshing out more features, like 


Or, dive straight into the [database engine documentation](https://docs.netdata.cloud/database/engine/).
