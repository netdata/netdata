<!--
title: "Export metrics to external time-series databases"
description: "Use the exporting engine to send Netdata metrics to popular external time series databases for long-term storage or further analysis."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/export/external-databases.md
-->

# Export metrics to external time-series databases

Netdata allows you to export metrics to external time-series databases with the [exporting
engine](/exporting/README.md). This system uses a number of **connectors** to intiate connections to [more than
thirty](#supported-databases) supported databases, including InfluxDB, Prometheus, Graphite, ElasticSearch, and much
more. 

The exporting engine resamples Netdata's thousands of per-second metrics at a user-configurable interval, and can export
metrics to multiple time-series databases simultaneously.

Based on your needs and resources you allocated to your external time-series database, you can configure the interval
that metrics are exported or export only certain charts with filtering. You can also choose whether metrics are exported
as-collected, a normalized average, or the sum/volume of metrics values over the configured interval.

Exporting is an important part of Netdata's effort to be [interoperable](/docs/overview/netdata-monitoring-stack.md)
with other monitoring software. You can use an external time-series database for long-term metrics retention, further
analysis, or correlation with other tools, such as application tracing.

## Supported databases

Netdata supports exporting metrics to the following databases through several
[connectors](/exporting/README.md#features). Once you find the connector that works for your database, open its
documentation and the [enabling a connector](/docs/export/enable-connector.md) doc for details on enabling it.

-   **AppOptics**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **AWS Kinesis**: [AWS Kinesis Data Streams](/exporting/aws_kinesis/README.md)
-   **Azure Data Explorer**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Azure Event Hubs**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Blueflood**: [Graphite](/exporting/graphite/README.md)
-   **Chronix**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Cortex**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **CrateDB**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **ElasticSearch**: [Graphite](/exporting/graphite/README.md), [Prometheus remote
    write](/exporting/prometheus/remote_write/README.md)
-   **Gnocchi**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Google BigQuery**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Google Cloud Pub/Sub**: [Google Cloud Pub/Sub Service](/exporting/pubsub/README.md)
-   **Graphite**: [Graphite](/exporting/graphite/README.md), [Prometheus remote
    write](/exporting/prometheus/remote_write/README.md)
-   **InfluxDB**: [Graphite](/exporting/graphite/README.md), [Prometheus remote
    write](/exporting/prometheus/remote_write/README.md)
-   **IRONdb**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **JSON**: [JSON document databases](/exporting/json/README.md)
-   **Kafka**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **KairosDB**: [Graphite](/exporting/graphite/README.md), [OpenTSDB](/exporting/opentsdb/README.md)
-   **M3DB**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **MetricFire**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **MongoDB**: [MongoDB](/exporting/mongodb/)
-   **New Relic**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **OpenTSDB**: [OpenTSDB](/exporting/opentsdb/README.md), [Prometheus remote
    write](/exporting/prometheus/remote_write/README.md)
-   **PostgreSQL**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Prometheus**: [Prometheus scraper](/exporting/prometheus/README.md)
-   **TimescaleDB**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md),
    [netdata-timescale-relay](/exporting/TIMESCALE.md)
-   **QuasarDB**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **SignalFx**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Splunk**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **TiKV**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Thanos**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **VictoriaMetrics**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)
-   **Wavefront**: [Prometheus remote write](/exporting/prometheus/remote_write/README.md)

Can't find your preferred external time-series database? Ask our [community](https://community.netdata.cloud/) for
solutions, or file an [issue on
GitHub](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage&template=bug_report.md).

## What's next?

We recommend you read our document on [enabling a connector](/docs/export/enable-connector.md) to learn about the
process and discover important configuration options. If you would rather skip ahead, click on any of the above links to
connectors for their reference documentation, which outline any prerequisites to install for that connector, along with
connector-specific configuration options.

Read about one possible use case for exporting metrics in our guide: [_Export and visualize Netdata metrics in
Graphite_](/docs/guides/export/export-netdata-metrics-graphite.md).

### Related reference documentation

-   [Exporting engine reference](/exporting/README.md)
-   [Backends reference (deprecated)](/backends/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fexporting%2Fexternal-databases&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
