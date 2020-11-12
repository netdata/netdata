<!--
title: How to optimize Netdata's performance
description: TK
image: /img/seo/guides/configure/performance.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/configure/performance.md
-->

# How to optimize Netdata's performance

TK




For example, you might run the Netdata Agent on a cloud VM with only 512 MiB of RAM, or on an IoT device like a
[Raspberry Pi](/docs/guides/monitor/pi-hole-raspberry-pi.md). In these cases, reducing Netdata's footprint beyond its
already diminuitive size can pay big dividends, giving your appliations more horsepower while still monitoring their
health and the performance of the node itself.

## Prerequisites

-   A node running the Netdata Agent.
-   Familiarity with configuring the Netdata Agent with `edit-config`.

If you're not familiar with how to configure a Netdata Agent, read our [node configuration
doc](/docs/configure/nodes.md) before continuing with this guide. This guide assumes familiarity with the Netdata config
directory, using `edit-config`, and the process of uncommenting/editing various settings in `netdata.conf` and other
configuration files.

## What affects Netdata's performance?

Netdata's performance is primarily affected by **data collection/retention** and **clients accessing data**.



You can control 

-   The number of charts for which data are collected
-   The number of plugins running
-   The technology of the plugins (i.e. BASH plugins are slower than binary plugins)
-   The frequency of data collection

You can control all the above.

**Web clients accessing the data**

-   the duration of the charts in the dashboard
-   the number of charts refreshes requested
-   the compression level of the web responses
-   Netdata Cloud usage/streaming/Metric Correlations

You can also control _some_ aspects of accessing data.

## Reduce the global collection frequency

TK

### Reduce a specific collector's frequency

TK

## Disable collectors or entire plugins

TK

## Run Netdata behind Nginx

TK

## Increase the open files limit

TK

### systemd

TK

### Non-systemd

TK

## Disable logs

```conf
[global]
	debug log = none
	error log = none
	access log = none
```

## Lower memory usage for metrics retention

You can reduce the disk space that the [database engine](/database/engine/README.md) uses to retain metrics by editing
the `dbengine multihost disk space` option in `netdata.conf`. The default value is `256`, but can be set to a minimum of
`64`. By reducing the disk space allocation, Netdata also needs to store less metadata in the node's memory.

The `page cache size` option also directly impacts Netdata's memory usage, but has a minimum value of `32`.

Reducing the value of `dbengine multihost disk space` does slim down Netdata's resource usage, but it also reduces how
long Netdata retains metrics. Find the right balance of performance and metrics retention by using the [dbengine
calculator](/docs/store/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics).

All the settings are found in the `[global]` section of `netdata.conf`:

```conf
[global]
	memory mode = dbengine
	page cache size = 32
  dbengine multihost disk space = 256
```

## Disable gzip compression for dashboard responses

TK

## Related reference documentation

-   [Database engine](/database/engine/README.md)

## What's next?

TK

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fconfigure%2Fperformance.md&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)


