# Export metrics to external time-series databases

Netdata allows you to export metrics to external time-series databases with the [exporting
engine](https://github.com/netdata/netdata/blob/master/src/exporting/README.md). This system uses a number of **connectors** to initiate connections to [more than
thirty](#supported-databases) supported databases, including InfluxDB, Prometheus, Graphite, ElasticSearch, and much
more. 

The exporting engine resamples Netdata's thousands of per-second metrics at a user-configurable interval, and can export
metrics to multiple time-series databases simultaneously.

Based on your needs and resources you allocated to your external time-series database, you can configure the interval
that metrics are exported or export only certain charts with filtering. You can also choose whether metrics are exported
as-collected, a normalized average, or the sum/volume of metrics values over the configured interval.

Exporting is an important part of Netdata's effort to be interoperable
with other monitoring software. You can use an external time-series database for long-term metrics retention, further
analysis, or correlation with other tools, such as application tracing.

## Supported databases

Netdata supports exporting metrics to the following databases through several
[connectors](https://github.com/netdata/netdata/blob/master/src/exporting/README.md#features). Once you find the connector that works for your database, open its
documentation and the [enabling a connector](https://github.com/netdata/netdata/blob/master/docs/exporting-metrics/enable-an-exporting-connector.md) doc for details on enabling it.

-   **AppOptics**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **AWS Kinesis**: [AWS Kinesis Data Streams](https://github.com/netdata/netdata/blob/master/src/exporting/aws_kinesis/README.md)
-   **Azure Data Explorer**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Azure Event Hubs**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Blueflood**: [Graphite](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md)
-   **Chronix**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Cortex**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **CrateDB**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **ElasticSearch**: [Graphite](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md), [Prometheus remote
    write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Gnocchi**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Google BigQuery**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Google Cloud Pub/Sub**: [Google Cloud Pub/Sub Service](https://github.com/netdata/netdata/blob/master/src/exporting/pubsub/README.md)
-   **Graphite**: [Graphite](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md), [Prometheus remote
    write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **InfluxDB**: [Graphite](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md), [Prometheus remote
    write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **IRONdb**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **JSON**: [JSON document databases](https://github.com/netdata/netdata/blob/master/src/exporting/json/README.md)
-   **Kafka**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **KairosDB**: [Graphite](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md), [OpenTSDB](https://github.com/netdata/netdata/blob/master/src/exporting/opentsdb/README.md)
-   **M3DB**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **MetricFire**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **MongoDB**: [MongoDB](https://github.com/netdata/netdata/blob/master/src/exporting/mongodb/README.md)
-   **New Relic**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **OpenTSDB**: [OpenTSDB](https://github.com/netdata/netdata/blob/master/src/exporting/opentsdb/README.md), [Prometheus remote
    write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **PostgreSQL**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
    via [PostgreSQL Prometheus Adapter](https://github.com/CrunchyData/postgresql-prometheus-adapter)
-   **Prometheus**: [Prometheus scraper](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/README.md)
-   **TimescaleDB**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md),
    [netdata-timescale-relay](https://github.com/netdata/netdata/blob/master/src/exporting/TIMESCALE.md)
-   **QuasarDB**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **SignalFx**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Splunk**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **TiKV**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Thanos**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **VictoriaMetrics**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)
-   **Wavefront**: [Prometheus remote write](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md)

Can't find your preferred external time-series database? Ask our [community](https://community.netdata.cloud/) for
solutions, or file an [issue on
GitHub](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml).
