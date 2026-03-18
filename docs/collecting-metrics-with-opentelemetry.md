# Collect metrics with OpenTelemetry

The Netdata Agent includes a built-in OpenTelemetry plugin that receives metrics
over the OTLP/gRPC protocol. You can pair it with the OpenTelemetry Collector to
pull metrics from over a hundred different receivers — host metrics, databases,
web servers, message queues, Prometheus endpoints, and more — and visualize them
in Netdata with zero custom code.

This guide walks you through the setup, explains how metrics flow from the
Collector to Netdata, and provides ready-to-use pipeline configurations for five
common receivers.

## How it works

The data flows through three components:

```
┌──────────────────────┐       OTLP/gRPC         ┌──────────────────────┐
│  OTel Collector      │ ─────────────────────►  │  Netdata Agent       │
│                      │     (port 4317)         │                      │
│  receivers:          │                         │  otel.plugin:        │
│    hostmetrics       │                         │    receives OTLP     │
│    prometheus        │                         │    creates charts    │
│    redis             │                         │    stores in DB      │
│    ...               │                         │                      │
│                      │                         │  dashboard:          │
│  exporters:          │                         │    visualizes        │
│    otlp ─────────────┼─────────────────────►   │    alerts            │
└──────────────────────┘                         └──────────────────────┘
```

1. **Receivers** in the OTel Collector scrape or receive metrics from
   infrastructure components.
2. The Collector's **OTLP exporter** sends those metrics to the Netdata Agent
   over gRPC.
3. The Netdata **OTel plugin** (`otel.plugin`) maps incoming OTLP metrics to
   charts and stores them in the time-series database.

## Prerequisites

Before you begin, verify that the following conditions are met:

- The Netdata Agent is installed and running on a Linux host.
  See the [installation guide](/docs/install/README.md) if you need to install
  it.
- The OpenTelemetry Collector is installed on the same host (or on a host that
  can reach the Netdata Agent over the network).
  See the [official OTel Collector installation documentation](https://opentelemetry.io/docs/collector/installation/)
  for instructions.

> **NOTE**
>
> Use the **OpenTelemetry Collector Contrib** distribution. The core distribution
> includes only the most basic receivers. The Contrib distribution bundles all
> community-maintained receivers used in this guide.

## Configure the Netdata OTel plugin

The OTel plugin starts automatically and listens on `127.0.0.1:4317` with
sensible defaults. No configuration is required if the OTel Collector runs on
the same host.

If you need to change settings, edit `otel.yaml` in the Netdata configuration
directory:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config otel.yaml
```

The most commonly adjusted options are:

| Option | Default | Description |
|:-------|:--------|:------------|
| `endpoint.path` | `127.0.0.1:4317` | gRPC listen address. Change to `0.0.0.0:4317` to accept remote connections. |
| `metrics.interval_secs` | `10` | Chart update frequency in seconds (1–3600). |
| `metrics.chart_configs_dir` | `/etc/netdata/otel.d/v1/metrics/` | Directory for metric mapping files. |
| `metrics.expiry_duration_secs` | `900` | Remove charts with no data after this many seconds. |

For example, to accept connections from a remote Collector:

```yaml
endpoint:
  path: "0.0.0.0:4317"
```

> **IMPORTANT**
>
> When you expose the endpoint on `0.0.0.0`, ensure that a firewall or network
> policy restricts access to trusted sources. The gRPC endpoint does not require
> authentication by default. You can enable TLS by setting `endpoint.tls_cert_path`
> and `endpoint.tls_key_path`.

For the full list of options, see the
[OTel plugin reference](/src/crates/netdata-otel/otel-plugin/integrations/opentelemetry.md).

## Set up the OTel Collector pipeline

Every OTel Collector configuration file has three top-level sections:

- **`receivers`** — where metrics come from.
- **`exporters`** — where metrics go.
- **`service.pipelines`** — which receivers and exporters to wire together.

All examples in this guide share the same exporter and pipeline structure. Only
the `receivers` section changes.

### The OTLP exporter

Every pipeline in this guide uses the OTLP exporter to send metrics to Netdata:

```yaml
exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true
```

Set `insecure: true` when the Netdata OTel plugin listens without TLS (the
default). If you enabled TLS on the Netdata side, remove this line and configure
the appropriate `ca_file`, `cert_file`, and `key_file` under `tls:`.

### The service pipeline

Wire a receiver to the OTLP exporter in the `service.pipelines` section:

```yaml
service:
  pipelines:
    metrics:
      receivers: [hostmetrics]
      exporters: [otlp]
```

You can list multiple receivers in the same pipeline. The sections below show
complete, working configurations for each receiver.

## Example 1: Host metrics

The `hostmetrics` receiver collects system-level metrics — CPU, memory, disk,
network, filesystem, and more — directly from the host OS. It requires no
external service and is the easiest way to verify your pipeline works.

### Collector configuration

Create or edit the Collector configuration file (typically
`/etc/otelcol-contrib/config.yaml`):

```yaml
receivers:
  hostmetrics:
    collection_interval: 10s
    scrapers:
      cpu:
      memory:
      disk:
      filesystem:
      network:
      load:
      paging:
      processes:

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [hostmetrics]
      exporters: [otlp]
```

Each entry under `scrapers:` enables a group of related metrics. You can remove
scrapers you do not need.

### Start the Collector

```bash
sudo systemctl restart otelcol-contrib
```

After a few seconds, host metric charts appear in the Netdata dashboard under
the **OpenTelemetry** section.

### What to expect

The hostmetrics receiver produces metrics such as:

| Metric | Scraper | Description |
|:-------|:--------|:------------|
| `system.cpu.time` | `cpu` | CPU time per core, broken down by state (user, system, idle, iowait, etc.) |
| `system.memory.usage` | `memory` | Memory usage by state (used, free, cached, buffered) |
| `system.disk.io` | `disk` | Bytes read and written per disk device |
| `system.network.io` | `network` | Bytes transmitted and received per network interface |
| `system.filesystem.usage` | `filesystem` | Used, free, and reserved bytes per mount point |
| `system.cpu.load_average.1m` | `load` | 1-minute load average |
| `system.paging.usage` | `paging` | Swap usage by state (used, free) |
| `system.processes.count` | `processes` | Process count by status (running, sleeping, blocked, etc.) |

Netdata ships **stock metric mapping files** for the hostmetrics receiver that
automatically group related data points into multi-dimension charts. For
example, `system.cpu.time` appears as one chart per CPU core with dimensions
for each state — not as dozens of separate single-value charts.

The next section explains how this mapping works and how you can create your own.

## Organize metrics with chart configuration files

### The problem

OTel metrics arrive as flat data points, each tagged with a set of attributes.
For example, the `system.cpu.time` metric includes a `cpu` attribute
(`cpu0`, `cpu1`, ...) and a `state` attribute (`user`, `system`, `idle`, ...).

Without any configuration, the OTel plugin creates one chart per unique
combination of attributes, each with a single dimension named `value`. This
results in many small charts that are difficult to navigate.

### The solution

Chart configuration files tell the plugin which attribute to use as the
**dimension name**. Data points that share the same values for all *other*
attributes are then grouped into a single chart with multiple dimensions.

These YAML files live in the chart configs directory
(`/etc/netdata/otel.d/v1/metrics/` by default). The plugin loads stock files
first, then user files from the same directory. **User files take priority** —
if a metric name matches a rule in both a stock file and a user file, the user
rule wins.

### File format

```yaml
metrics:
  "<metric_name>":
    - instrumentation_scope:
        name: <regex>
      dimension_attribute_key: <attribute_key>
      interval_secs: <seconds>          # optional per-metric override
      grace_period_secs: <seconds>      # optional per-metric override
```

| Field | Description |
|:------|:------------|
| `metrics.<metric_name>` | The exact OTel metric name to match. |
| `instrumentation_scope.name` | A regular expression that matches the instrumentation scope name. Use this to distinguish between receivers that emit the same metric name. |
| `instrumentation_scope.version` | (Optional) A regular expression that matches the scope version. |
| `dimension_attribute_key` | The data point attribute whose value becomes the dimension name in the chart. |
| `interval_secs` | Override the collection interval for this metric (1–3600 seconds). |
| `grace_period_secs` | Override the grace period for this metric. |

### Example: CPU time by state

The hostmetrics receiver emits `system.cpu.time` with attributes `cpu` and
`state`. The stock mapping file groups by `state`:

```yaml
metrics:
  "system.cpu.time":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*cpuscraper$
      dimension_attribute_key: state
```

This produces one chart per CPU core, where each chart has dimensions like
`user`, `system`, `idle`, `iowait`, and so on.

### Example: Network I/O by direction

The `system.network.io` metric has attributes `device` and `direction`. The
stock mapping groups by `direction`:

```yaml
metrics:
  "system.network.io":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*networkscraper$
      dimension_attribute_key: direction
```

This produces one chart per network interface, with `transmit` and `receive` as
dimensions.

### Write your own mapping file

To create a mapping for a receiver that does not have stock mappings:

1. Identify the metric names the receiver emits (check its documentation or
   inspect the data in the Netdata dashboard).
2. Determine which attribute you want as the dimension.
3. Create a YAML file in `/etc/netdata/otel.d/v1/metrics/`.

For example, to group Redis memory metrics:

```yaml
metrics:
  "redis.cpu.time":
    - dimension_attribute_key: state
```

> **NOTE**
>
> The `instrumentation_scope` field is optional. Omit it when only one receiver
> emits a given metric name. Include it when you need to distinguish between
> receivers that share metric names.

## Example 2: Prometheus endpoints

The `prometheus` receiver scrapes any HTTP endpoint that exposes metrics in
Prometheus format. This bridges the large ecosystem of Prometheus exporters into
the OTel pipeline without running a separate Prometheus server.

### Collector configuration

This example scrapes a local Prometheus-format endpoint:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "my-application"
          scrape_interval: 15s
          static_configs:
            - targets: ["localhost:9090"]

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [prometheus]
      exporters: [otlp]
```

The `config` block uses the same syntax as a standard Prometheus
`scrape_configs` section. You can add multiple jobs, use `metric_relabel_configs`
to filter metrics, and use service discovery mechanisms (such as
`kubernetes_sd_configs` or `file_sd_configs`).

### Scrape multiple targets

Add more entries under `static_configs` or add more jobs:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "node-exporter"
          scrape_interval: 15s
          static_configs:
            - targets: ["localhost:9100"]
        - job_name: "my-app"
          scrape_interval: 30s
          static_configs:
            - targets: ["localhost:8080"]
          metrics_path: "/metrics"
```

> **TIP**
>
> If you already have a working `prometheus.yml` file, you can copy its
> `scrape_configs` section directly into the receiver's `config` block.

For the full list of configuration options, see the
[Prometheus receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver).

## Example 3: Redis

The `redis` receiver collects metrics from a single Redis instance by issuing
the `INFO` command. It reports client connections, memory usage, keyspace
statistics, command throughput, replication status, and more.

### Prerequisites for this receiver

- A running Redis instance.
- The Collector can reach the Redis endpoint (default: `localhost:6379`).

### Collector configuration

```yaml
receivers:
  redis:
    endpoint: "localhost:6379"
    collection_interval: 10s

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [redis]
      exporters: [otlp]
```

If your Redis instance requires authentication:

```yaml
receivers:
  redis:
    endpoint: "localhost:6379"
    collection_interval: 10s
    password: "${env:REDIS_PASSWORD}"
```

### Key metrics

| Metric | Description |
|:-------|:------------|
| `redis.memory.used` | Total memory allocated by Redis (bytes) |
| `redis.memory.rss` | Resident set size reported by the OS (bytes) |
| `redis.clients.connected` | Number of connected clients |
| `redis.commands.processed` | Total commands processed since startup |
| `redis.keyspace.hits` | Successful key lookups |
| `redis.keyspace.misses` | Failed key lookups |
| `redis.keys.expired` | Total keys removed due to TTL expiration |
| `redis.keys.evicted` | Total keys evicted due to memory pressure |
| `redis.net.input` | Total bytes received |
| `redis.net.output` | Total bytes sent |
| `redis.db.keys` | Number of keys per database |

For the full metric list, see the
[Redis receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/redisreceiver).

## Example 4: NGINX

The `nginx` receiver collects metrics from NGINX's built-in stub status module.
It produces four metrics: accepted connections, handled connections, current
connections by state, and total requests.

### Prerequisites for this receiver

- NGINX is compiled with the `ngx_http_stub_status_module` (included by default
  in most distributions).
- The stub status endpoint is enabled in the NGINX configuration.

Enable the stub status endpoint by adding a `location` block to your
`nginx.conf`:

```nginx
server {
    # ...existing configuration...

    location /status {
        stub_status;
        allow 127.0.0.1;
        deny all;
    }
}
```

Reload NGINX after making the change:

```bash
sudo systemctl reload nginx
```

### Collector configuration

```yaml
receivers:
  nginx:
    endpoint: "http://localhost:80/status"
    collection_interval: 10s

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [nginx]
      exporters: [otlp]
```

### Key metrics

| Metric | Description |
|:-------|:------------|
| `nginx.connections_accepted` | Total accepted client connections |
| `nginx.connections_handled` | Total handled connections |
| `nginx.connections_current` | Current connections by state (active, reading, writing, waiting) |
| `nginx.requests` | Total client requests served |

For more details, see the
[NGINX receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/nginxreceiver).

## Example 5: PostgreSQL

The `postgresql` receiver collects database performance metrics from PostgreSQL
by querying the `pg_stat_*` system views. It reports connection counts, query
throughput, table and index statistics, and buffer usage.

### Prerequisites for this receiver

- A running PostgreSQL instance (version 9.6 or later).
- A monitoring user with `SELECT` permission on `pg_stat_database`.

Create a dedicated monitoring user:

```sql
CREATE USER otel WITH PASSWORD 'your-secure-password';
GRANT pg_monitor TO otel;
```

### Collector configuration

```yaml
receivers:
  postgresql:
    endpoint: "localhost:5432"
    username: "otel"
    password: "${env:POSTGRESQL_PASSWORD}"
    databases:
      - "mydb"
    collection_interval: 10s
    tls:
      insecure: true

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [postgresql]
      exporters: [otlp]
```

> **NOTE**
>
> Set the `POSTGRESQL_PASSWORD` environment variable before starting the
> Collector, or replace `${env:POSTGRESQL_PASSWORD}` with the password directly
> (not recommended for production).

### Key metrics

| Metric | Description |
|:-------|:------------|
| `postgresql.commits` | Transactions committed per database |
| `postgresql.rollbacks` | Transactions rolled back per database |
| `postgresql.db.size` | Database size in bytes |
| `postgresql.rows` | Number of rows by state (live, dead) |
| `postgresql.operations` | Row operations (inserts, updates, deletes) |
| `postgresql.blocks_read` | Block reads by source (heap, index, toast) |
| `postgresql.connection.max` | Maximum allowed connections |
| `postgresql.table.count` | Number of user tables per database |
| `postgresql.index.scans` | Number of index scans |

For the full metric list, see the
[PostgreSQL receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/postgresqlreceiver).

## Combine multiple receivers in a single pipeline

You can run multiple receivers in the same Collector instance. List all receivers
in the pipeline definition:

```yaml
receivers:
  hostmetrics:
    collection_interval: 10s
    scrapers:
      cpu:
      memory:
      disk:
      network:
      load:

  redis:
    endpoint: "localhost:6379"
    collection_interval: 10s

  nginx:
    endpoint: "http://localhost:80/status"
    collection_interval: 10s

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [hostmetrics, redis, nginx]
      exporters: [otlp]
```

All metrics from all receivers flow to Netdata through the same OTLP exporter.
Each receiver operates independently — if one fails, the others continue to
collect.

## Troubleshooting

### No charts appear in Netdata

1. Verify that the Netdata OTel plugin is running:

   ```bash
   ps aux | grep otel
   ```

2. Verify that the Collector is running and check its logs for errors:

   ```bash
   sudo systemctl status otelcol-contrib
   ```

   ```bash
   sudo journalctl -u otelcol-contrib -f
   ```

3. Confirm that the Collector can reach the Netdata endpoint:

   ```bash
   curl -v telnet://localhost:4317
   ```

### Charts appear but have only one dimension

The metric likely needs a chart configuration file to group data points into
multi-dimension charts. See
[Organize metrics with chart configuration files](#organize-metrics-with-chart-configuration-files).

### The Collector reports connection refused

The Netdata OTel plugin listens on `127.0.0.1:4317` by default. If the
Collector runs on a different host, change the endpoint in `otel.yaml` to
`0.0.0.0:4317` and ensure that the firewall allows traffic on port 4317.

## Additional resources

- [Netdata OTel plugin reference](/src/crates/netdata-otel/otel-plugin/integrations/opentelemetry.md)
- [OpenTelemetry Collector documentation](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Collector Contrib receivers](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver)
- [Host Metrics receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/hostmetricsreceiver)
- [Prometheus receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver)
- [Redis receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/redisreceiver)
- [NGINX receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/nginxreceiver)
- [PostgreSQL receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/postgresqlreceiver)
