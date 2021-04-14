<!--
title: "Use Netdata standalone or as part of your monitoring stack"
description: "Netdata can run independently or as part of a larger monitoring stack thanks to its flexibility, interoperable core, and exporting features."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/overview/netdata-monitoring-stack.md
-->

# Use Netdata standalone or as part of your monitoring stack

Netdata is an extremely powerful monitoring, visualization, and troubleshooting platform. While you can use it as an
effective standalone tool, we also designed it to be open and interoperable with other tools you might already be using.

Netdata helps you collect everything and scales to infrastructure of any size, but it doesn't lock-in data or force you
to use specific tools or methodologies. Each feature is extensible and interoperable so they can work in parallel with
other tools. For example, you can use Netdata to collect metrics, visualize metrics with a second open-source program,
and centralize your metrics in a cloud-based time-series database solution for long-term storage or further analysis.

You can build a new monitoring stack, including Netdata, or integrate Netdata's metrics with your existing monitoring
stack. No matter which route you take, Netdata helps you monitor infrastructure of any size.

Here are a few ways to enrich your existing monitoring and troubleshooting stack with Netdata:

## Collect metrics from Prometheus endpoints

Netdata automatically detects 600 popular endpoints and collects per-second metrics from them via the [generic
Prometheus collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/prometheus). This even
includes support for Windows 10 via [`windows_exporter`](https://github.com/prometheus-community/windows_exporter).

This collector is installed and enabled on all Agent installations by default, so you don't need to waste time
configuring Netdata. Netdata will detect these Prometheus metrics endpoints and collect even more granular metrics than
your existing solutions. You can now use all of Netdata's meaningfully-visualized charts to diagnose issues and
troubleshoot anomalies.

## Export metrics to external time-series databases

Netdata can send its per-second metrics to external time-series databases, such as InfluxDB, Prometheus, Graphite,
TimescaleDB, ElasticSearch, AWS Kinesis Data Streams, Google Cloud Pub/Sub Service, and many others.

To [export metrics to external time-series databases](/docs/export/external-databases.md), you configure an [exporting
_connector_](/docs/export/enable-connector.md). These connectors support filtering and resampling for granular control
over which metrics you export, and at what volume. You can export resampled metrics as collected, as averages, or the
sum of interpolated values based on your needs and other monitoring tools.

Once you have Netdata's metrics in a secondary time-series database, you can use them however you'd like, such as
additional visualization/dashboarding tools or aggregation of data from multiple sources.

## Visualize metrics with Grafana

One popular monitoring stack is Netdata, Graphite, and Grafana. Netdata acts as the stack's metrics collection
powerhouse, Graphite the time-series database, and Grafana the visualization platform. With Netdata at the core, you can
be confident that your monitoring stack is powered by all possible metrics, from all possible sources, from every node
in your infrastructure.

Of course, just because you export or visualize metrics elsewhere, it doesn't mean Netdata's equivalent features
disappear. You can always build new dashboards in Netdata Cloud, drill down into per-second metrics using Netdata's
charts, or use Netdata's health watchdog to send notifications whenever an anomaly strikes.

## What's next?

Whether you're using Netdata standalone or as part of a larger monitoring stack, the next step is the same: [**Get
Netdata**](/docs/get/README.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Foverview%2Fnetdata-monitoring-stacka&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
