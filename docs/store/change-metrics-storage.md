<!--
title: "Change how long Netdata stores metrics"
description: "With a single configuration change, the Netdata Agent can store days, weeks, or months of metrics at its famous per-second granularity."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/store/change-metrics-storage.md
-->

# Change how long Netdata stores metrics

import { Calculator } from '../../src/components/agent/dbCalc/'

The [database engine](/database/engine/README.md) uses RAM to store recent metrics. When metrics reach a certain age,
and based on how much system RAM you allocate toward storing metrics in memory, they are compressed and "spilled" to
disk for long-term storage.

The default settings retain about two day's worth of metrics on a system collecting 2,000 metrics every second, but the
Netdata Agent is highly configurable if you want your nodes to store days, weeks, or months worth of per-second data.

The Netdata Agent uses two settings in `netdata.conf` to change the behavior of the database engine:

```conf
[global]
    page cache size = 32
    dbengine multihost disk space = 256
```

`page cache size` sets the maximum amount of RAM (in MiB) the database engine uses to cache and index recent metrics.
`dbengine multihost disk space` sets the maximum disk space (again, in MiB) the database engine uses to store
historical, compressed metrics. When the size of stored metrics exceeds the allocated disk space, the database engine
removes the oldest metrics on a rolling basis.

## Calculate the system resources (RAM, disk space) needed to store metrics

You can store more or less metrics using the database engine by changing the allocated disk space. Use the calculator
below to find an appropriate value for `dbengine multihost disk space` based on how many metrics your node(s) collect,
whether you are streaming metrics to a parent node, and more.

You do not need to edit the `page cache size` setting to store more metrics using the database engine. However, if you
want to store more metrics _specifically in memory_, you can increase the cache size.

> ⚠️ This calculator provides an estimate of disk and RAM usage for **metrics storage**, along with its best
> recommendation for the `dbengine multihost disk space` setting. Real-life usage may vary based on the accuracy of the
> values you enter below, changes in the compression ratio, and the types of metrics stored.

<Calculator />

## Edit `netdata.conf` with recommended database engine settings

Now that you have a recommended setting for `dbengine multihost disk space`, open `netdata.conf` with
[`edit-config`](/docs/configure/nodes.md#use-edit-config-to-edit-netdataconf) and look for the `dbengine multihost disk
space` setting. Change it to the value recommended above. For example:

```conf
[global]
    dbengine multihost disk space = 1024
```

Save the file and restart the Agent with `service netdata restart` to change the database engine's size.

## What's next?

For more information about the database engine, see our [database reference doc](/database/engine/README.md).

Storing metrics with the database engine is completely interoperable with [exporting to other time-series
databases](/docs/export/external-databases.md). With exporting, you can use the node's resources to surface metrics
when [viewing dashboards](/docs/visualize/interact-dashboards-charts.md), while also archiving metrics elsewhere for
further analysis, visualization, or correlation with other tools. 

If you don't want to always store metrics on the node that collects them or run ephemeral nodes without dedicated
storage, you can use [streaming](/streaming/README.md). Streaming allows you to centralize your data, run Agents as
headless collectors, replicate data, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fstore%2Fchange-metrics-storage&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
