<!--
title: "Overview"
sidebar_label: "Netdata Architecture"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/overview.md"
learn_status: "Unpublished"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
sidebar_position: "000"
learn_docs_purpose: "Overview page"
-->

Netdata is designed to be both simple to use and flexible for every monitoring, visualization, and troubleshooting use
case:

- **Collect**: Netdata collects all available metrics from your system and applications with 300+ collectors, Kubernetes
  service discovery, and in-depth container monitoring, all while using only 1% CPU and a few MB of RAM. It even
  collects metrics from Windows machines.
- **Visualize**: The dashboard meaningfully presents charts to help you understand the relationships between your
  hardware, operating system, running apps/services, and the rest of your infrastructure. Add nodes to Netdata Cloud for
  a complete view of your infrastructure from a single pane of glass.
- **Monitor**: Netdata's health watchdog uses hundreds of preconfigured alarms to notify you via Slack, email, PagerDuty
  and more when an anomaly strikes. Customize with dynamic thresholds, hysteresis, alarm templates, and role-based
  notifications.
- **Troubleshoot**: 1s granularity helps you detect and analyze anomalies other monitoring platforms might have missed.
  Interactive visualizations reduce your reliance on the console, and historical metrics help you trace issues back to
  their root cause.
- **Store**: Netdata's efficient database engine efficiently stores per-second metrics for days, weeks, or even months.
  Every distributed node stores metrics locally, simplifying deployment, slashing costs, and enriching Netdata's
  interactive dashboards.
- **Export**: Integrate per-second metrics with other time-series databases like Graphite, Prometheus, InfluxDB,
  TimescaleDB, and more with Netdata's interoperable and extensible core.
- **Stream**: Aggregate metrics from any number of distributed nodes in one place for in-depth analysis, including
  ephemeral nodes in a Kubernetes cluster.

Now that you're perusing the documentation for a better technical understanding of our product, let's break down some 
of the language you've almost certainly heard us use to talk about our capabilities. 


### Distributed Data Architecture

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

#### Holistic observability of an infrastructure in a distributed data architecture approach

Netdata Cloud bridges the gap between many distributed databases by centralizing only the interface. You query and
visualize your nodes' metrics real time and whenever your need to. When
you [look at charts in Netdata Cloud](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md), the metrics values are queried
directly from that node's database and securely streamed to Netdata Cloud, which proxies them to your browser.

#### Integrity of metrics in a distributed data architecture approach

Netdata Agents will collect and store the data for your nodes in any given moment even at partial outages. To ensure
though accessibility of your data in any given moment, Netdata Agents utilize technologies such
as [replication and streaming](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
of data. 

### High Fidelity Monitoring

Netdata provides you with a high fidelity monitoring solution to observe your systems. With thousands of metrics, even
though neatly arranged into charts and dashboards, sometimes even masters of their craft can become overwhelmed. In this
section we will explain the methods and components Netdata provides, to guide you troubleshoot your infrastructure's
issues. These components are:

1. The [Anomaly Advisor](#anomaly-advisor)
2. The [Metric Correlation](#metric-corellation) component

The ML driving our Anomaly Adviser and Metric Correlations feature works at the edge to learn what anomalies look like, so it can identify and alert you to them immediately. And the algorithm behind our Cloud-based Metric Correlations tool can take you straight to the potential root cause of your problem in seconds.

#### Anomaly Advisor

Netdata's Anomaly Advisor feature lets you quickly surface potentially anomalous metrics and charts related to a particular highlight window of interest. If you are running a Netdata version higher than v1.35.0-29-nightly, you will be able to use the Anomaly Advisor out of the box with zero configuration. If you are on an earlier Netdata version, you will need to first enable ML on your nodes by following the steps below.

Read more [here](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/machine-learning-powered-anomaly-advisor.md).

#### Metric correlation

The Metric Correlation (MC) component search and identifies correlations between metrics/charts on a particular window
of interest (timeline) that you specify. By displaying the standard Netdata dashboard, filtered to show only charts that
are relevant to the window of interest, you can get to the root cause sooner.

Because Metric Correlations uses every available metric from that node, with as high as 1-second granularity, you get
extraordinary insights for the status of your system.

Read more about Metric Correlations [here](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/metric-correlations.md).

### High Fidelity Monitoring 

Netdata is not just another monitoring solution, it's _the_ high fidelity monitoring solution.

What high fidelity means to us:

1. Real time metrics, view metrics/changes in seconds since their occur.
2. Highest resolution of metrics, observe changes occur between seconds.
3. Fixed step metric collection; quantify your observation windows
4. Unlimited data, search for patterns in data that you don't even believe they are correlated.

To identify problem in your systems you need to have deep understanding of what is going on. In systems where services,
apps, databases and containers change their status in milliseconds you need from your monitoring solution to provide you
the richest data so that system is observable.

#### Case study

Imagine that you have a database that has a 900ms delay in random moments, if this is acceptable for your case,
no problem. But what if it's a real time database for a financial institution? You can imagine right now the problem. We
live in a world that any latency can cascade to multiple services and introduce huge delays so this worth your time to
investigate.

How would you begin your troubleshooting? Is it a bottleneck in disks, caching latency, A garbage collection
process?

You need to inspect:

☑ Inspect all the metrics real time.

☑ With the highest possible resolution

☑ Fixed step between two observations of a metric

☑ See the actual effect in multiple resources

WIth Netdata's tools and resources, each of those steps becomes a whole lot easier, faster, cheaper, and more productive. 

### Distributed architecture 

Because of our [Distributed architecture approach](#distributed-data-architecture), Netdata can seamlessly observe a couple, hundreds or even thousands of nodes. There are no actual bottlenecks
especially if you retain metrics locally in the Agents. You only have to deploy the Agent in any system that you want to
monitor and claim them into the cloud. Netdata Cloud queries only slices of data when and if you request them on the
spot. 

### Zero Configuration 

Your Netdata journey doesn't have a cold start. Netdata is preconfigured and capable to autodetect and monitor any well
known application that runs on your system. You just deploy and claim Netdata Agents in your Netdata space, and monitor
them in seconds. Alerts are also preconfigured and fine-tuned, from community members with years of experience in any
component. Notifications about the health of your infrastructure are enabled by default in your Netdata Space.

:::info

There might some cases where you might need to configure a step or two but if you specific use our express installation
deployment there are just minor things that you need to tune (like application that need some authentication method to
be monitored by Netdata or tailored made configurations for your case)

:::
