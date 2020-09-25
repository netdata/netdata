<!--
title: "Distributed data architecture"
description: "Netdata's distributed data architecture stores metrics on individual nodes for high performance and scalability using all your granular metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/store/distributed-data.md
-->

# Distributed data architecture

Netdata uses a distributed data architecture to help you collect and store per-second metrics from any number of nodes.
Every node in your infrastructure, whether it's one or a thousand, stores the metrics it collects.

Netdata Cloud bridges the gap between many distributed databases by _centralizing the interface you use_ to query and
visualize your nodes' metrics. When you [look at charts in Netdata
Cloud](/docs/visualize/interact-dashboards-charts.md), the metrics values are queried directly from that node's database
and securely streamed to Netdata Cloud, which proxies them to your browser.

Netdata's distributed data architecture has a number of benefits:

-   **Performance**: Every query to a node's database takes only a few milliseconds to complete for responsiveness when
    viewing dashboards or using features like [Metric
    Correlations](https://learn.netdata.cloud/docs/cloud/insights/metric-correlations).
-   **Scalability**: As your infrastructure scales, install the Netdata Agent on every new node to immediately add it to
    your monitoring solution without adding cost or complexity.
-   **1-second granularity**: Without an expensive centralized data lake, you can store all of your nodes' per-second
    metrics, for any period of time, while keeping costs down.
-   **No filtering or selecting of metrics**: Because Netdata's distributed data architecture allows you to store all
    metrics, you don't have to configure which metrics you retain. Keep everything for full visibility during
    troubleshooting and root cause analysis.
-   **Easy maintenance**: There is no centralized data lake to purchase, allocate, monitor, and update, removing
    complexity from your monitoring infrastructure.

## Does Netdata Cloud store my metrics?

Netdata Cloud does not store metric values. 

To enable certain features, such as [viewing active alarms](/docs/monitor/view-active-alarms.md) or [filtering by
service](/docs/visualize/view-all-nodes.md#filter-and-group-your-infrastructure), Netdata Cloud does store configured
alarms, their status, and a list of active collectors.

Netdata does not and never will sell your personal data or data about your deployment.

## Long-term metrics storage with Netdata

Any node running the Netdata Agent can store long-term metrics for any retention period, given you allocate the
appropriate amount of RAM and disk space.

Read our document on changing [how long Netdata stores metrics](/docs/store/change-metrics-storage.md) on your nodes for
details.

## Other options for your metrics data

While a distributed data architecture is the default when monitoring infrastructure with Netdata, you can also configure
its behavior based on your needs or the type of infrastructure you manage.

To archive metrics to an external time-series database, such as InfluxDB, Graphite, OpenTSDB, Elasticsearch,
TimescaleDB, and many others, see details on [integrating Netdata via exporting](/docs/export/external-databases.md).

You can also stream between nodes using [streaming](/streaming/README.md), allowing to replicate databases and create
your own centralized data lake of metrics, if you choose to do so.

When you use the database engine to store your metrics, you can always perform a quick backup of a node's
`/var/cache/netdata/dbengine/` folder using the tool of your choice.

## What's next?

You can configure the Netdata Agent to store days, weeks, or months worth of distributed, per-second data by
[configuring the database engine](/docs/store/change-metrics-storage.md). Use our calculator to determine the system
resources required to retain your desired amount of metrics, and expand or contract the database by editing a single
setting.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fstore%2Fdistributed-data&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
