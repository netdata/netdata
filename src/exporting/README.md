# Exporting Reference

This reference guide provides comprehensive information about enabling, configuring, and monitoring Netdata's exporting engine for sending metrics to external time-series databases.

For a quick introduction, read our [exporting metrics overview](/docs/exporting-metrics/README.md) or start with [enabling a connector](/docs/exporting-metrics/enable-an-exporting-connector.md).

## Core Capabilities

The exporting engine features a modular structure that supports:

- Multiple connector instances running simultaneously
- Different update intervals per connector
- Custom filters per connector instance
- Metric resampling to reduce database congestion

:::info

When you enable the exporting engine, Netdata exports metrics **starting from the restart time**, not the entire [historical database](/src/database/README.md).

:::

## Operation Modes

Netdata provides three data export modes:

|       Mode       | Description                              | Data Format                                    | Use Case                                                |
|:----------------:|:-----------------------------------------|:-----------------------------------------------|:--------------------------------------------------------|
| **as-collected** | Raw metrics in original units            | Counters remain counters, gauges remain gauges | Time-series database experts who need raw data          |
|   **average**    | Normalized metrics from Netdata database | All metrics sent as gauges in Netdata units    | Simplified visualization with Netdata-centric workflows |
|  **sum/volume**  | Sum of interpolated values               | Aggregated values over the export interval     | Long-term trend analysis                                |

:::tip

**Choosing the Right Mode**:

- Use `as-collected` if you're building monitoring around a time-series database and know how to convert units
- Use `average` for simpler long-term archiving that matches Netdata's visualization exactly

:::

## Supported Connectors

| Connector                                                                   | Protocol/Format            | Metric Format                      |
|:----------------------------------------------------------------------------|:---------------------------|:-----------------------------------|
| [AWS Kinesis](/src/exporting/aws_kinesis/README.md)                         | JSON                       | Stream-based                       |
| [Google Pub/Sub](/src/exporting/pubsub/README.md)                           | JSON                       | Message-based                      |
| [Graphite](/src/exporting/graphite/README.md)                               | Plaintext                  | `prefix.hostname.chart.dimension`  |
| [JSON Databases](/src/exporting/json/README.md)                             | JSON                       | Document-based                     |
| [OpenTSDB](/src/exporting/opentsdb/README.md)                               | Plaintext/HTTP             | `prefix.chart.dimension` with tags |
| [MongoDB](/src/exporting/mongodb/README.md)                                 | JSON                       | Document-based                     |
| [Prometheus](/src/exporting/prometheus/README.md)                           | HTTP scraping              | Prometheus exposition format       |
| [Prometheus Remote Write](/src/exporting/prometheus/remote_write/README.md) | Snappy-compressed protobuf | Binary over HTTP                   |
| [TimescaleDB](/src/exporting/TIMESCALE.md)                                  | JSON streams               | Time-series tables                 |

## How Metric Names Are Constructed

OpenTSDB and Graphite export dotted metric names. Prometheus scrape and remote write export context-based metric names with labels.

### Flat-name connectors: OpenTSDB and Graphite

These connectors join their name components with dots, so the chart and dimension are visible directly in the metric name:

```text
OpenTSDB: prefix.chart.dimension
Graphite: prefix.hostname.chart.dimension
```

- **OpenTSDB** sends the host as a separate tag (`host=...`).
- **Graphite** embeds the host in the dotted path: `prefix.hostname.chart.dimension`.

Example (OpenTSDB): `netdata.system.cpu.user` with a `host=myhost` tag.

OpenTSDB and Graphite preserve dots in chart and dimension names. They replace other non-alphanumeric characters with underscores.

### Prometheus exports: scrape and remote write

For homogeneous charts, both methods join the prefix and the chart **context** with underscores, and carry the chart, family, and dimension as labels:

```text
prefix_context{chart="...", family="...", dimension="..."}
```

- The chart **context** — the template shared by all charts of the same kind — is part of the metric name, not the individual chart ID or name.
- `average` appends the chart units and `_average`. The scrape endpoint can omit the units with `hideunits=yes`.
- `sum` appends `_sum` without the chart units.
- In `as-collected` mode, incremental and percentage-over-difference counters append `_total`. For charts produced by the Prometheus collector, Netdata does not append `_total`.
- For heterogeneous `as-collected` charts, the dimension moves into the metric name (`prefix_context_dimension`) and is omitted from the labels.
- Netdata sanitizes the context, units, and any dimension embedded in a Prometheus metric name, so dots and other unsupported characters become underscores. Chart, family, and dimension labels retain their label values. The configured prefix is used as provided.

Example (remote write): `netdata_system_cpu_percentage_average{chart="system.cpu", dimension="user", family="cpu", instance="myhost"}`.

For the complete naming rules — contexts, units, suffixes, and how to preview the exact metric names via the `allmetrics` endpoint — see the [Prometheus reference](/src/exporting/prometheus/README.md).

### Quick comparison

| Aspect             | OpenTSDB / Graphite                  | Prometheus scrape / remote write                                                   |
|:-------------------|:-------------------------------------|:-----------------------------------------------------------------------------------|
| OpenTSDB base      | `prefix.chart.dimension`             | `prefix_context`                                                                   |
| Graphite base      | `prefix.hostname.chart.dimension`    | `prefix_context`                                                                   |
| Dimension          | In the metric name                   | Label; in the metric name for heterogeneous `as-collected` charts                  |
| Data-source suffix | None                                 | `_total`, `_average`, or `_sum` when applicable                                    |
| Units in name      | No                                   | `average` only                                                                     |
| Host               | Tag (OpenTSDB) or path (Graphite)    | `instance` label for remote write and all-host scrape; otherwise the scrape target |
| Sanitization       | Chart/dimension preserve dots        | Context/units/embedded dimensions replace dots                                     |
| Default prefix     | `netdata`                            | `netdata`                                                                          |

Both approaches respect the `send names instead of ids` setting: when enabled, Netdata uses human-friendly chart and dimension names; when disabled, it uses the raw system IDs. See the [OpenTSDB connector options](/src/exporting/opentsdb/README.md) for the prefix and name settings.

## Configuration Structure

Your `exporting.conf` file contains these configuration blocks:

```text
[exporting:global]
    enabled = yes
    send configured labels = no
    send automatic labels = no
    update every = 10

[prometheus:exporter]
    send names instead of ids = yes
    send configured labels = yes
    send automatic labels = no
    send charts matching = *
    send hosts matching = localhost *
    prefix = netdata

[graphite:my_graphite_instance]
    enabled = yes
    destination = localhost:2003
    data source = average
    prefix = netdata
    hostname = my-name
    update every = 10
    buffer on failures = 10
    timeout ms = 20000
    send charts matching = *
    send hosts matching = localhost *
    send names instead of ids = yes
    send configured labels = yes
    send automatic labels = yes

[prometheus_remote_write:my_prometheus_remote_write_instance]
    enabled = yes
    destination = localhost
    remote write URL path = /receive

[kinesis:my_kinesis_instance]
    enabled = yes
    destination = us-east-1
    stream name = netdata
    aws_access_key_id = my_access_key_id
    aws_secret_access_key = my_aws_secret_access_key

[pubsub:my_pubsub_instance]
    enabled = yes
    destination = pubsub.googleapis.com
    credentials file = /etc/netdata/pubsub_credentials.json
    project id = my_project
    topic id = my_topic

[mongodb:my_mongodb_instance]
    enabled = yes
    destination = localhost
    database = my_database
    collection = my_collection

[json:my_json_instance]
    enabled = yes
    destination = localhost:5448

[opentsdb:my_opentsdb_plaintext_instance]
    enabled = yes
    destination = localhost:4242

[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
    username = my_username
    password = my_password

[opentsdb:https:my_opentsdb_https_instance]
    enabled = yes
    destination = localhost:8082
```

### Configuration Sections

| Section                 | Purpose                                     |
|:------------------------|:--------------------------------------------|
| `[exporting:global]`    | Default settings for all connectors         |
| `[prometheus:exporter]` | Prometheus API endpoint settings            |
| `[<type>:<name>]`       | Individual connector instance configuration |

### Connector Types

Available connector types with optional modifiers:

- `graphite` | `graphite:http` | `graphite:https`
- `opentsdb:telnet` | `opentsdb:http` | `opentsdb:https`
- `prometheus_remote_write` | `prometheus_remote_write:http` | `prometheus_remote_write:https`
- `json` | `json:http` | `json:https`
- `kinesis` | `pubsub` | `mongodb`

## Configuration Options

### Basic Settings

| Option         | Values                   | Description                                                   |
|:---------------|:-------------------------|:--------------------------------------------------------------|
| `enabled`      | yes/no                   | Activates the connector instance                              |
| `data source`  | as-collected/average/sum | Selects data export mode                                      |
| `hostname`     | string                   | Hostname for external database (default: `[global].hostname`) |
| `prefix`       | string                   | Prefix added to all metrics                                   |
| `update every` | seconds                  | Export interval with automatic randomization                  |

### Connection Settings

| Option               | Format               | Description                                             |
|:---------------------|:---------------------|:--------------------------------------------------------|
| `destination`        | space-separated list | Target servers in `[PROTOCOL:]IP[:PORT]` format         |
| `buffer on failures` | iterations           | Buffer size when database unavailable                   |
| `timeout ms`         | milliseconds         | Processing timeout (default: `2 * update_every * 1000`) |

#### Destination Examples

IPv4 configuration:

```text
destination = 10.11.14.2:4242 10.11.14.3:4242 10.11.14.4:4242
```

IPv6 and IPv4 combined:

```text
destination = [ffff:...:0001]:2003 10.11.12.1:2003
```

Special destinations:

- **Kinesis**: AWS region (e.g., `us-east-1`)
- **MongoDB**: [MongoDB URI](https://docs.mongodb.com/manual/reference/connection-string/)
- **Pub/Sub**: Service endpoint

### Filtering Options

| Option                 | Pattern Format           | Description                                       |
|:-----------------------|:-------------------------|:--------------------------------------------------|
| `send hosts matching`  | space-separated patterns | Filter hosts using `*` wildcard, `!` for negation |
| `send charts matching` | space-separated patterns | Filter charts by ID/name, `!` for negation        |

:::important

Pattern matching follows first-match logic. Order matters when using negative patterns (`!`).

Example: `!*child* *db*` matches all `*db*` hosts except those containing `*child*`.

:::

### Label Settings

| Option                      | Values | Description                                                 |
|:----------------------------|:-------|:------------------------------------------------------------|
| `send names instead of ids` | yes/no | Use human-friendly names vs system IDs                      |
| `send configured labels`    | yes/no | Include `[host labels]` from `netdata.conf`                 |
| `send automatic labels`     | yes/no | Include auto-generated labels (`_os_name`, `_architecture`) |

## Chart Filtering

Filter metrics through two methods:

1. **Configuration file**:

   ```text
   [prometheus:exporter]
       send charts matching = system.*
   ```

2. **URL parameter**:

   ```text
   http://localhost:19999/api/v1/allmetrics?format=shell&filter=system.*
   ```

## HTTPS Support

For databases without native TLS/SSL support, configure a reverse proxy:

- [Nginx reverse proxy setup](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-nginx.md)

## Performance Considerations

The exporting engine operates independently to avoid slowing down Netdata. However:

:::warning

Multiple connector instances running batches simultaneously can consume significant CPU resources. Configure different update intervals to prevent synchronization.

:::

## Monitoring the Exporting Engine

Netdata provides five monitoring charts under **Netdata Monitoring**:

| Chart                          | Monitors                                   |
|:-------------------------------|:-------------------------------------------|
| **Buffered metrics**           | Number of metrics added to dispatch buffer |
| **Exporting data size**        | Data volume (KB) added to buffer           |
| **Exporting operations**       | Operation count performed                  |
| **Exporting thread CPU usage** | CPU resources consumed by exporting thread |

![Exporting engine monitoring](https://cloud.githubusercontent.com/assets/2662304/20463536/eb196084-af3d-11e6-8ee5-ddbd3b4d8449.png)

## Built-in Alerts

The exporting engine includes three automatic alerts:

| Alert                      | Monitors                                |
|:---------------------------|:----------------------------------------|
| `exporting_last_buffering` | Seconds since last successful buffering |
| `exporting_metrics_sent`   | Percentage of successfully sent metrics |
| `exporting_metrics_lost`   | Metrics lost due to repeated failures   |

![Exporting alerts](https://cloud.githubusercontent.com/assets/2662304/20463779/a46ed1c2-af43-11e6-91a5-07ca4533cac3.png)

## Fallback Script

Netdata includes `nc-exporting.sh` for:

- Saving metrics to disk during database outages
- Pushing cached metrics when database recovers
- Monitoring/tracing/debugging metric generation
