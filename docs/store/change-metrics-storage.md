<!--
title: "Change how long Netdata stores metrics"
description: "With a single configuration change, the Netdata Agent can store days, weeks, or months of metrics at its famous per-second granularity."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/store/change-metrics-storage.md
-->

# Change how long Netdata stores metrics

import { Calculator } from '../../src/components/agent/dbCalc/'

The Netdata Agent uses a custom made time-series database (TSDB), named the [database engine
(`dbengine`)](/database/engine/README.md), to store metrics.

The default settings retain approximately two day's worth of metrics on a system collecting 2,000 metrics every second,
but the Netdata Agent is highly configurable if you want your nodes to store days, weeks, or months worth of per-second
data.

The Netdata Agent uses two settings in `netdata.conf` to change the behavior of the database engine:

```conf
[global]
    dbengine page cache size = 32
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

You do not need to edit the `dbengine page cache size` setting to store more metrics using the database engine. However,
if you want to store more metrics _specifically in memory_, you can increase the cache size.

:::info If you want to also store long term metrics you can always utilize
the [tiering mechanism](/database/engine/README.md#tiering) of the Agent.

:::

:::caution

This calculator provides an estimate of disk and RAM usage for **metrics storage** Real-life usage may vary based on the
accuracy of the values you enter below, changes in the compression ratio, and the types of metrics stored.

:::

<Calculator />

## Edit `netdata.conf` with recommended database engine settings

Now that you have a recommended setting for `dbengine multihost disk space`, open `netdata.conf` with
[`edit-config`](/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) and look for
the `dbengine multihost disk space` setting. Change it to the value recommended above. For example:

```conf
[db]
    dbengine multihost disk space = 1024
```

Save the file and restart the Agent with `sudo systemctl restart netdata`, or
the [appropriate method](/docs/configure/start-stop-restart.md) for your system, to change the database engine's size.

## What's next?

If you have multiple nodes with the Netdata Agent installed, you
can [stream metrics](/docs/metrics-storage-management/how-streaming-works.mdx) from any number of _child_ nodes to a _
parent_ node and store metrics using a centralized time-series database. Streaming allows you to centralize your data,
run Agents as headless collectors, replicate data, and more.

Storing metrics with the database engine is completely interoperable
with [exporting to other time-series databases](/docs/export/external-databases.md). With exporting, you can use the
node's resources to surface metrics when [viewing dashboards](/docs/visualize/interact-dashboards-charts.md), while also
archiving metrics elsewhere for further analysis, visualization, or correlation with other tools.

### Related reference documentation

- [Netdata Agent · Database engine](/database/engine/README.md)
- [Netdata Agent · Database engine configuration option](/daemon/config/README.md#[db]-section-options)


