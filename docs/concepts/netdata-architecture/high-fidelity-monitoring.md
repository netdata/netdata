<!--
title: "High fidelity monitoring"
sidebar_label: "High fidelity monitoring"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/high-fidelity-monitoring.md"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "netdata-architecture"
learn_docs_purpose: "Present what high fidelity monitoring is: (real time, high resolution, unlimited, fixed step metric databases)"
-->

**********************************************************************
Template:

Small intro, what we are about to cover

// every concept we will explain to this document (grouped) should be a different heading (h2) and followed by an example
// we need at any given moment to provide a reference (a anchored link to this concept)
## concept title

A concept introduces a single feature or concept. A concept should answer the questions:

1. What is this?
2. Why would I use it?

For instance, for example etc etc

Give a small taste for this concept, not trying to cover it's reference page. 

In the end of the document:

## Related topics

list of related topics

*****************Suggested document to be transformed**************************
From netdata repo's commit : 3a672f5b4ba23d455b507c8276b36403e10f953d<!--
title: "Distributed data architecture"
description: "Netdata's distributed data architecture stores metrics on individual nodes for high performance and scalability using all your granular metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/store/distributed-data-architecture.md
-->

# Distributed data architecture

Netdata uses a distributed data architecture to help you collect and store per-second metrics from any number of nodes.
Every node in your infrastructure, whether it's one or a thousand, stores the metrics it collects.

Netdata Cloud bridges the gap between many distributed databases by _centralizing the interface_ you use to query and
visualize your nodes' metrics. When you [look at charts in Netdata Cloud](/docs/visualize/interact-dashboards-charts.md)
, the metrics values are queried directly from that node's database and securely streamed to Netdata Cloud, which
proxies them to your browser.

Netdata's distributed data architecture has a number of benefits:

- **Performance**: Every query to a node's database takes only a few milliseconds to complete for responsiveness when
  viewing dashboards or using features
  like [Metric Correlations](https://learn.netdata.cloud/docs/cloud/insights/metric-correlations).
- **Scalability**: As your infrastructure scales, install the Netdata Agent on every new node to immediately add it to
  your monitoring solution without adding cost or complexity.
- **1-second granularity**: Without an expensive centralized data lake, you can store all of your nodes' per-second
  metrics, for any period of time, while keeping costs down.
- **No filtering or selecting of metrics**: Because Netdata's distributed data architecture allows you to store all
  metrics, you don't have to configure which metrics you retain. Keep everything for full visibility during
  troubleshooting and root cause analysis.
- **Easy maintenance**: There is no centralized data lake to purchase, allocate, monitor, and update, removing
  complexity from your monitoring infrastructure.

## Ephemerality of metrics

The ephemerality of metrics plays an important role in retention. In environments where metrics collection is dynamic and
new metrics are constantly being generated, we are interested about 2 parameters:

1. The **expected concurrent number of metrics** as an average for the lifetime of the database. This affects mainly the
   storage requirements.

2. The **expected total number of unique metrics** for the lifetime of the database. This affects mainly the memory
   requirements for having all these metrics indexed and available to be queried.

## Granularity of metrics

The granularity of metrics (the frequency they are collected and stored, i.e. their resolution) is significantly
affecting retention.

Lowering the granularity from per second to every two seconds, will double their retention and half the CPU requirements
of the Netdata Agent, without affecting disk space or memory requirements.

## Long-term metrics storage with Netdata

Any node running the Netdata Agent can store long-term metrics for any retention period, given you allocate the
appropriate amount of RAM and disk space.

Read our document on changing [how long Netdata stores metrics](/docs/store/change-metrics-storage.md) on your nodes for
details.

You can also stream between nodes using [streaming](/streaming/README.md), allowing to replicate databases and create
your own centralized data lake of metrics, if you choose to do so.

While a distributed data architecture is the default when monitoring infrastructure with Netdata, you can also configure
its behavior based on your needs or the type of infrastructure you manage.

To archive metrics to an external time-series database, such as InfluxDB, Graphite, OpenTSDB, Elasticsearch,
TimescaleDB, and many others, see details on [integrating Netdata via exporting](/docs/export/external-databases.md).

When you use the database engine to store your metrics, you can always perform a quick backup of a node's
`/var/cache/netdata/dbengine/` folder using the tool of your choice.

## Does Netdata Cloud store my metrics?

Netdata Cloud does not store metric values.

To enable certain features, such as [viewing active alarms](/docs/monitor/view-active-alarms.md)
or [filtering by hostname/service](https://learn.netdata.cloud/docs/cloud/war-rooms#node-filter), Netdata Cloud does
store configured alarms, their status, and a list of active collectors.

Netdata does not and never will sell your personal data or data about your deployment.

## What's next?

You can configure the Netdata Agent to store days, weeks, or months worth of distributed, per-second data by
[configuring the database engine](/docs/store/change-metrics-storage.md). Use our calculator to determine the system
resources required to retain your desired amount of metrics, and expand or contract the database by editing a single
setting.


*******************************************************************************
