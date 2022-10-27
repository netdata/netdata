<!--
title: "Distributed data architecture"
sidebar_label: "Distributed data architecture"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/distributed-data-architecture.md"
sidebar_position: 4
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "netdata-architecture"
learn_docs_purpose: "Explain the Netdata approach, every Agent a standalone database+data collection tool+integration tools"
-->

## The Netdata approach

Netdata was built with a distributed data architecture mindset. All data are collected and stored on the edge, whenever
it's possible. This approach has a number of benefits:

- **Easy maintenance**: There is no centralized data lake to purchase, allocate, monitor or update. This removes the
  complexity from your monitoring infrastructure. You can implement deployments as part of your Continuous Integration
  with or without tailored-made configuration options for your nodes.
- **Performance**: The metric collection is as fast as it could be, you can't get any faster rather since it occur on
  the edge.
- **Optimized querying**: Whenever you need the data of a particular node, you can query them with minimum latency.
- **Scalability**: As your infrastructure scales, install the Netdata Agent on every new node to immediately add it to
  your monitoring solution without adding cost or complexity.
- **Minimum resource consumption**: A Netdata Agent in your node demand the minimum (physical) resources to implement
  metric collection jobs or to store them.
- **No filtering and boundaries on the metrics**: Netdata allows you to store all metrics, you don't have to configure
  which metrics you retain. Keep everything for full visibility during troubleshooting and root cause analysis.

## Holistic observability of an infrastructure in a distributed data architecture approach

Netdata Cloud bridges the gap between many distributed databases by centralizing only the interface. You query and
visualize your nodes' metrics real time and whenever your need to. When
you [look at charts in Netdata Cloud](https://github.com/netdata/learn/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md), the metrics values are queried
directly from that node's database and securely streamed to Netdata Cloud, which proxies them to your browser.

## Integrity of metrics in a distributed data architecture approach

Netdata Agents will collect and store the data for your nodes in any given moment even at partial outages. To ensure
though accessibility of your data in any given moment, Netdata Agents utilize technologies such
as [replication and streaming](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
of data. 

