# Export metrics to external time-series databases

Netdata natively supports long term retention of metrics. Its tiered database design typically provides significantly longer retention (months to years) and faster queries (typically 20+ times faster), compared to other common time-series databases.

For integration with other observability tools, Netdata provides a number of exporters allow you to copy metrics to third party time-series databases for additional analysis or integration with other tools.

Exporters enable connections to [more than thirty](#supported-databases) supported databases, including InfluxDB, Prometheus, Graphite, ElasticSearch, and much more.

The exporting engine is able to downsample Netdata's per-second metrics at a user-configurable interval (e.g per minute), and can export metrics to multiple time-series databases simultaneously.

Based on your needs and resources you allocated to your external time-series database, you can configure the interval
that metrics are exported or export only certain charts with filtering. You can also choose whether metrics are exported
as-collected, a normalized average, or the sum/volume of metrics values over the configured interval.

## Supported databases

Netdata supports exporting metrics to the following databases through several
[connectors](/src/exporting/README.md#features). Once you find the connector that works for your database, open its
documentation and the [enabling a connector](/docs/exporting-metrics/enable-an-exporting-connector.md) doc for details on enabling it.

- **AppOptics**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **AWS Kinesis**: [AWS Kinesis Data Streams](/src/exporting/aws_kinesis/README.md).
- **Azure Data Explorer**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Azure Event Hubs**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Blueflood**: [Graphite](/src/exporting/graphite/README.md).
- **Chronix**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Cortex**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **CrateDB**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **ElasticSearch**: [Graphite](/src/exporting/graphite/README.md), [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Gnocchi**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Google BigQuery**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Google Cloud Pub/Sub**: [Google Cloud Pub/Sub Service](/src/exporting/pubsub/README.md).
- **Graphite**: [Graphite](/src/exporting/graphite/README.md), [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **InfluxDB**: [Graphite](/src/exporting/graphite/README.md), [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **IRONdb**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **JSON**: [JSON document databases](/src/exporting/json/README.md).
- **Kafka**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **KairosDB**: [Graphite](/src/exporting/graphite/README.md), [OpenTSDB](/src/exporting/opentsdb/README.md).
- **M3DB**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **MetricFire**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **MongoDB**: [MongoDB](/src/exporting/mongodb/README.md).
- **New Relic**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **OpenTSDB**: [OpenTSDB](/src/exporting/opentsdb/README.md), [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **PostgreSQL**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md) via [PostgreSQL Prometheus Adapter](https://github.com/CrunchyData/postgresql-prometheus-adapter).
- **Prometheus**: [Prometheus scraper](/src/exporting/prometheus/README.md).
- **TimescaleDB**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md), [netdata-timescale-relay](/src/exporting/TIMESCALE.md).
- **QuasarDB**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **SignalFx**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Splunk**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **TiKV**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Thanos**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **VictoriaMetrics**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).
- **Wavefront**: [Prometheus remote write](/src/exporting/prometheus/remote_write/README.md).

Can't find your preferred external time-series database? Ask our [community](https://community.netdata.cloud/) for solutions, or file an [issue on GitHub](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml).
