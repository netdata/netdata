# Collect OpenTelemetry data with Netdata

The Netdata Agent includes a built-in OpenTelemetry plugin that receives
**metrics and logs** over the OTLP/gRPC protocol, on a single endpoint:

- **Metrics** become regular Netdata charts, with full dashboard and alerting
  support. Pair the plugin with the OpenTelemetry Collector to pull metrics
  from over a hundred receivers — host metrics, databases, web servers,
  message queues, Prometheus endpoints, and more.
- **Logs** are stored on the agent and explored in the Netdata **Logs tab**
  (the `otel-logs` source) — from files, the systemd journal, syslog sources,
  and more.

This guide walks you through the setup once, covers each signal in its own
section with ready-to-use pipeline configurations, and closes with
[advanced pipeline patterns](#advanced-pipelines) — converting logs to
metrics and parsing unstructured logs. For the complete plugin configuration
reference, see the [plugin reference](netdata-otel-plugin-reference.md).

## How it works

```
┌──────────────────────┐       OTLP/gRPC         ┌──────────────────────┐
│  OTel Collector      │ ─────────────────────►  │  Netdata Agent       │
│                      │     (port 4317)         │                      │
│  receivers:          │                         │  otel.plugin:        │
│    hostmetrics       │                         │    receives OTLP     │
│    prometheus        │                         │    metrics → charts  │
│    journald          │                         │    logs   → Logs tab │
│    filelog            │                         │                      │
│    syslog            │                         │  dashboard:          │
│    ...               │                         │    visualizes        │
│                      │                         │    alerts            │
│  exporters:          │                         │                      │
│    otlp ─────────────┼─────────────────────►   │                      │
└──────────────────────┘                         └──────────────────────┘
```

1. **Receivers** in the OTel Collector scrape or receive telemetry from
   infrastructure components and applications.
2. The Collector's **OTLP exporter** sends the data to the Netdata Agent over
   gRPC — both signals share the same endpoint.
3. The Netdata **OTel plugin** (`otel.plugin`) maps metrics to charts and
   stores logs for the Logs tab.

## Prerequisites

Before you begin, verify that the following conditions are met:

- The Netdata Agent is installed and running on a Linux host.
  See the [installation guide](/packaging/installer/README.md) if you need to
  install it.
- The OpenTelemetry Collector is installed on the same host (or on a host
  that can reach the Netdata Agent over the network).
  See the [official OTel Collector installation documentation](https://opentelemetry.io/docs/collector/installation/)
  for instructions.

> **NOTE**
>
> Use the [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib) distribution.
> The core distribution includes only the most basic receivers. The Contrib
> distribution bundles all community-maintained receivers used in this guide.

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

The endpoint speaks **gRPC only** — there is no OTLP/HTTP receiver on port
4318. Signal-specific options (chart mapping, retention) are covered in each
signal's section below; for the full list of options, see the
[Netdata OTel plugin reference](netdata-otel-plugin-reference.md).

## Set up the OTel Collector pipeline

Every OTel Collector configuration file has three top-level sections:

- **`receivers`** — where telemetry comes from.
- **`exporters`** — where telemetry goes.
- **`service.pipelines`** — which receivers and exporters to wire together.

All examples in this guide share the same exporter and pipeline structure.
Only the `receivers` section changes.

### The OTLP exporter

Every pipeline in this guide uses the OTLP exporter to send data to Netdata:

```yaml
exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true
```

Set `insecure: true` when the Netdata OTel plugin listens without TLS (the
default). If you enabled TLS on the Netdata side, remove this line and
configure the appropriate `ca_file`, `cert_file`, and `key_file` under
`tls:`.

Note that the plugin binds the IPv4 address `127.0.0.1` by default. If
`localhost` resolves to the IPv6 `::1` first on your system, the exporter
gets "connection refused" — use `127.0.0.1:4317` as the exporter endpoint
instead of `localhost:4317`.

### The service pipelines

Wire receivers to the OTLP exporter in the `service.pipelines` section. Each
signal has its own pipeline key — `metrics` or `logs` — and a single
Collector can run both side by side. This complete example collects host
metrics and two log sources at once:

```yaml
receivers:
  hostmetrics:
    collection_interval: 10s
    scrapers:
      cpu:
      memory:
      disk:
      network:

  journald:
    directory: /var/log/journal
    priority: info
    start_at: end

  filelog:
    include:
      - /var/log/myapp/*.log
    start_at: end

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
    logs:
      receivers: [journald, filelog]
      exporters: [otlp]
```

Each pipeline operates independently. Metrics appear as charts in the Netdata
dashboard and logs appear in the Logs tab. A single OTLP exporter handles
both, and you can list multiple receivers in the same pipeline or define
either or both pipelines.

The sections below show complete, working configurations for each signal.

## Metrics

Incoming metrics become regular Netdata charts, visualized in the dashboard
under the **OpenTelemetry** section with full alerting support.

The most commonly adjusted metrics options in `otel.yaml` are:

| Option | Default | Description |
|:-------|:--------|:------------|
| `metrics.interval_secs` | `10` | Chart update frequency in seconds (1–3600). |
| `metrics.chart_configs_dir` | `/etc/netdata/otel.d/v1/metrics/` | Directory for metric mapping files. |
| `metrics.expiry_duration_secs` | `900` | Remove charts with no data after this many seconds. |
| `metrics.max_new_charts_per_request` | `100` | Maximum new charts created per gRPC request (cardinality guard). |

### Example: Host metrics

The `hostmetrics` receiver collects system-level metrics — CPU, memory, disk,
network, filesystem, and more — directly from the host OS. It requires no
external service and is the easiest way to verify your pipeline works.

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

Each entry under `scrapers:` enables a group of related metrics. You can
remove scrapers you do not need.

Start the Collector:

```bash
sudo systemctl restart otelcol-contrib
```

After a few seconds, host metric charts appear in the Netdata dashboard under
the **OpenTelemetry** section.

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

For how this mapping works and how to create your own, see
[Organize metrics with chart configuration files](#organize-metrics-with-chart-configuration-files).

### Example: Prometheus endpoints

The `prometheus` receiver scrapes any HTTP endpoint that exposes metrics in
Prometheus format. This bridges the large ecosystem of Prometheus exporters
into the OTel pipeline without running a separate Prometheus server.

This example scrapes a local Prometheus-format endpoint:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "my-application"
          scrape_interval: 10s
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
`scrape_configs` section. You can add multiple jobs, use
`metric_relabel_configs` to filter metrics, and use service discovery
mechanisms (such as `kubernetes_sd_configs` or `file_sd_configs`):

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "node-exporter"
          scrape_interval: 10s
          static_configs:
            - targets: ["localhost:9100"]
        - job_name: "my-app"
          scrape_interval: 10s
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

### Example: Redis

The `redis` receiver collects metrics from a single Redis instance by issuing
the `INFO` command. It reports client connections, memory usage, keyspace
statistics, command throughput, replication status, and more. The Collector
must be able to reach the Redis endpoint (default: `localhost:6379`).

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

Key metrics:

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

### Example: NGINX

The `nginx` receiver collects metrics from NGINX's built-in stub status
module. It produces four metrics: accepted connections, handled connections,
current connections by state, and total requests.

Prerequisites for this receiver:

- NGINX is compiled with the `ngx_http_stub_status_module` (included by
  default in most distributions).
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

Collector configuration:

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

Key metrics:

| Metric | Description |
|:-------|:------------|
| `nginx.connections_accepted` | Total accepted client connections |
| `nginx.connections_handled` | Total handled connections |
| `nginx.connections_current` | Current connections by state (active, reading, writing, waiting) |
| `nginx.requests` | Total client requests served |

For more details, see the
[NGINX receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/nginxreceiver).

### Example: PostgreSQL

The `postgresql` receiver collects database performance metrics from
PostgreSQL by querying the `pg_stat_*` system views. It reports connection
counts, query throughput, table and index statistics, and buffer usage. It
requires a running PostgreSQL instance (version 9.6 or later) and a
monitoring user with `SELECT` permission on `pg_stat_database`.

Create a dedicated monitoring user:

```sql
CREATE USER otel WITH PASSWORD 'your-secure-password';
GRANT pg_monitor TO otel;
```

Collector configuration:

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

Key metrics:

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

### Organize metrics with chart configuration files

#### The problem

OTel metrics arrive as flat data points, each tagged with a set of attributes.
For example, the `system.cpu.time` metric includes a `cpu` attribute
(`cpu0`, `cpu1`, ...) and a `state` attribute (`user`, `system`, `idle`, ...).

Without any configuration, the OTel plugin creates one chart per unique
combination of attributes, each with a single dimension named `value`. This
results in many small charts that are difficult to navigate.

#### The solution

Chart configuration files tell the plugin which attribute to use as the
**dimension name**. Data points that share the same values for all *other*
attributes are then grouped into a single chart with multiple dimensions.

These YAML files live in the chart configs directory
(`/etc/netdata/otel.d/v1/metrics/` by default). The plugin loads stock files
first, then user files from the same directory. **User files take priority** —
if a metric name matches a rule in both a stock file and a user file, the user
rule wins.

#### File format

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

#### Example: CPU time by state

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

#### Example: Network I/O by direction

The `system.network.io` metric has attributes `device` and `direction`. The
stock mapping groups by `direction`:

```yaml
metrics:
  "system.network.io":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*networkscraper$
      dimension_attribute_key: direction
```

This produces one chart per network interface, with `transmit` and `receive`
as dimensions.

#### Write your own mapping file

To create a mapping for a receiver that does not have stock mappings:

1. Identify the metric names the receiver emits (check its documentation or
   inspect the data in the Netdata dashboard).
2. Determine which attribute you want as the dimension.
3. Create a YAML file in `/etc/netdata/otel.d/v1/metrics/`.

For example, to group Redis CPU time by state:

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

## Logs

Incoming logs are stored in the plugin's own compressed, indexed storage on
the agent, and explored in the **Logs tab** of the Netdata dashboard (the
`otel-logs` source).

Logs are stored under `base_dir` (default `/var/log/netdata/otel/v2`), and
how much is kept is controlled by the `logs.retention` policy. The most
commonly adjusted log options in `otel.yaml` are:

| Option | Default | Description |
|:-------|:--------|:------------|
| `logs.retention.default.max_age` | `7 days` | Maximum age of stored log data. |
| `logs.retention.default.max_total_size` | `1GB` | Maximum total size of stored log data on disk. |
| `logs.retention.default.max_files` | `100000` | Maximum number of stored log files. |
| `logs.rotation.default.max_file_size` | `25MB` | Maximum size of the current log file before it is sealed. |
| `logs.rotation.default.max_entries` | `50000` | Maximum log entries per file before it is sealed. |

The oldest data is deleted when **any** retention limit is exceeded. For
example, to keep more log history:

```yaml
logs:
  retention:
    default:
      max_age: "14 days"
      max_total_size: "4GB"
```

By default, log records with timestamps older than 24 hours (or more than 10
minutes in the future) are rejected at ingestion — relevant when replaying
old backlogs; see `ingest.max_age` in the
[plugin reference](netdata-otel-plugin-reference.md).

> **NOTE — upgrading from the experimental plugin**
>
> Earlier experimental builds stored OTel logs as systemd journal files and
> used `logs.size_of_journal_file`-style options. That schema is no longer
> accepted: the plugin refuses to start and prints the old-to-new option
> mapping.

### Example: Journald

The `journald` receiver reads log entries directly from the systemd journal.
It requires no external service and is the easiest way to verify that your
log pipeline works, because every systemd-based Linux host already produces
journal entries.

Prerequisites for this receiver:

- The host uses systemd (most modern Linux distributions).
- The `journalctl` binary is available on the system.
- The Collector process has permission to read the journal (typically requires
  running as root or being in the `systemd-journal` group).

Collector configuration:

```yaml
receivers:
  journald:
    directory: /var/log/journal
    units:
      - sshd.service
      - docker.service
      - kubelet.service
    priority: info
    start_at: end

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [journald]
      exporters: [otlp]
```

Each option filters which journal entries the receiver collects:

| Option | Description |
|:-------|:------------|
| `directory` | Path to the journal directory. Defaults to `/run/log/journal` or `/run/journal`. |
| `units` | List of systemd unit names (for example, `sshd.service`) to collect logs from. Filters on the `_SYSTEMD_UNIT` journal field. Omit to collect from all units. |
| `identifiers` | List of syslog identifiers (for example, `myapp`) to collect logs from. Filters on the `SYSLOG_IDENTIFIER` journal field. Use this to capture logs written with `logger -t`. |
| `priority` | Minimum log priority. Options: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`. |
| `start_at` | Where to start reading at startup: `beginning` or `end`. |

> **NOTE**
>
> The `units` and `identifiers` filters match different journal fields.
> Use `units` for systemd services (matches `_SYSTEMD_UNIT`) and `identifiers`
> for applications that log via syslog or `logger` (matches `SYSLOG_IDENTIFIER`).
> You can combine both in the same receiver configuration.

To collect all journal entries without filtering by unit:

```yaml
receivers:
  journald:
    priority: info
    start_at: end
```

After restarting the Collector, journal log entries appear in the Netdata
dashboard under the **Logs** tab.

For the full list of configuration options, see the
[Journald receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/journaldreceiver).

### Example: File logs

The `filelog` receiver tails and parses log entries from files. It covers the
common case of applications that write structured or unstructured logs to
files on disk, such as web server access logs, application logs, or container
logs. The Collector process must be able to read the files.

This example tails all `.log` files under `/var/log/myapp/`:

```yaml
receivers:
  filelog:
    include:
      - /var/log/myapp/*.log
    start_at: end

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [filelog]
      exporters: [otlp]
```

If your application writes JSON-formatted logs, add a `json_parser` operator
to extract fields:

```yaml
receivers:
  filelog:
    include:
      - /var/log/myapp/*.log
    start_at: end
    operators:
      - type: json_parser
        timestamp:
          parse_from: attributes.time
          layout: "%Y-%m-%dT%H:%M:%S.%LZ"
        severity:
          parse_from: attributes.level
```

Add more entries under `include` or use glob patterns to tail multiple paths:

```yaml
receivers:
  filelog:
    include:
      - /var/log/myapp/*.log
      - /var/log/nginx/access.log
      - /var/log/nginx/error.log
    exclude:
      - /var/log/myapp/debug.log
    start_at: end
```

Key configuration options:

| Option | Default | Description |
|:-------|:--------|:------------|
| `include` | required | List of file glob patterns to tail. |
| `exclude` | `[]` | List of file glob patterns to skip. |
| `start_at` | `end` | Where to start reading: `beginning` or `end`. |
| `multiline` | | Configuration for multi-line log entries (for example, Java stack traces). |
| `operators` | `[]` | Chain of parsers to extract timestamps, severity, and structured fields. |
| `encoding` | `utf-8` | File encoding. |
| `poll_interval` | `200ms` | How often to check files for new data. |

For the full list of configuration options, see the
[File Log receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver).

### Example: Syslog

The `syslog` receiver listens for syslog messages over TCP or UDP and parses
them according to RFC 3164 or RFC 5424. This covers the common case of
network devices, appliances, and legacy applications that send syslog
messages.

This example listens for RFC 5424 syslog messages over UDP on port 54526:

```yaml
receivers:
  syslog:
    udp:
      listen_address: "0.0.0.0:54526"
    protocol: rfc5424

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [syslog]
      exporters: [otlp]
```

To receive syslog messages over TCP instead of UDP:

```yaml
receivers:
  syslog:
    tcp:
      listen_address: "0.0.0.0:54526"
    protocol: rfc5424
```

Older devices and applications often use the RFC 3164 (BSD syslog) format.
Set the `protocol` field accordingly and specify a `location` for timestamp
parsing:

```yaml
receivers:
  syslog:
    udp:
      listen_address: "0.0.0.0:54526"
    protocol: rfc3164
    location: UTC
```

Key configuration options:

| Option | Description |
|:-------|:------------|
| `tcp` | TCP listener configuration. Set `listen_address` to `<ip>:<port>`. |
| `udp` | UDP listener configuration. Set `listen_address` to `<ip>:<port>`. |
| `protocol` | Syslog protocol to parse: `rfc3164` or `rfc5424`. |
| `location` | Timezone for timestamp parsing (RFC 3164 only). Defaults to `UTC`. |
| `enable_octet_counting` | Enable RFC 6587 octet counting (RFC 5424 + TCP only). |

> **NOTE**
>
> Configure either `tcp` or `udp`, not both. If you need to receive syslog over
> both protocols, define two receiver instances (`syslog/tcp` and `syslog/udp`)
> and list both in the pipeline.

For the full list of configuration options, see the
[Syslog receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/syslogreceiver).

## Advanced pipelines

Beyond ingesting each signal, the Collector can transform data between
pipelines: count log entries into metrics you can alert on, or parse
unstructured log lines into structured attributes.

### Convert logs to metrics

Counting log entries by severity or content is a common observability pattern.
Instead of searching logs after an incident, you can track error rates as a
metric and alert on them in real time.

The OpenTelemetry Collector supports this through **connectors** — components
that sit between two pipelines. The `count` connector consumes logs from one
pipeline and emits count metrics into another.

#### How connectors work

```
┌─────────────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐
│  journald   │────►│  count      │────►│  transform │────►│  otlp   │
│  receiver   │     │  connector  │     │  processor │     │ exporter│
└─────────────┘     └─────────────┘     └────────────┘     └─────────┘
       │                                                        ▲
       │              logs pipeline                             │
       └────────────────────────────────────────────────────────┘
                                                           metrics pipeline
```

1. The **logs pipeline** sends log entries to both the OTLP exporter (for
   storage in Netdata) and the `count` connector.
2. The `count` connector counts entries that match specified conditions and
   emits those counts as metrics.
3. The **metrics pipeline** receives those counts, optionally transforms them,
   and sends them to Netdata via the OTLP exporter.

#### Example: Count warning-level journal entries

This pipeline reads journal entries from a systemd unit and counts how many
have a priority of `warning` or higher (priority value 4 or lower in syslog
convention, where lower numbers indicate higher severity).

```yaml
receivers:
  journald:
    directory: /var/log/journal
    units:
      - bluetooth.service
    priority: info
    start_at: end

connectors:
  count:
    logs:
      warning.log.count:
        description: Number of warning or higher severity log entries.
        conditions:
          - 'Int(body["PRIORITY"]) <= 4'

processors:
  transform:
    error_mode: ignore
    metric_statements:
      - context: metric
        statements:
          - set(unit, "{message}") where IsMatch(name, "log.count$")

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [journald]
      exporters: [count, otlp]
    metrics:
      receivers: [count]
      processors: [transform]
      exporters: [otlp]
```

The key sections are:

| Section | Purpose |
|:--------|:--------|
| `connectors.count.logs` | Defines a custom metric `warning.log.count` that increments for each log entry where `PRIORITY <= 4` (warning, error, critical, alert, emergency). |
| `processors.transform` | Sets the metric unit to `{message}` — the OpenTelemetry (UCUM) convention for counted things, where the braces annotate *what* is being counted. Netdata displays it as the chart's unit. |
| `service.pipelines.logs` | Sends logs to both the `count` connector and the OTLP exporter. |
| `service.pipelines.metrics` | Receives counts from the connector, transforms them, and sends them to Netdata. |

> **NOTE**
>
> The `count` connector appears as an **exporter** in the logs pipeline and as a
> **receiver** in the metrics pipeline. This is how connectors bridge two
> pipelines.

#### Customize the counting conditions

The `conditions` field accepts
[OTTL (OpenTelemetry Transformation Language)](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl#opentelemetry-transformation-language)
expressions. You can filter on any log body field or attribute.

Count entries that contain a specific keyword:

```yaml
connectors:
  count:
    logs:
      oom.log.count:
        description: Log entries mentioning out of memory.
        conditions:
          - 'IsMatch(body["MESSAGE"], "(?i)out of memory|OOM")'
```

Count entries from a specific syslog identifier:

```yaml
connectors:
  count:
    logs:
      sshd.error.count:
        description: Error log entries from sshd.
        conditions:
          - 'body["SYSLOG_IDENTIFIER"] == "sshd" and Int(body["PRIORITY"]) <= 3'
```

Define multiple metrics in the same connector:

```yaml
connectors:
  count:
    logs:
      warning.log.count:
        description: Warning or higher severity entries.
        conditions:
          - 'Int(body["PRIORITY"]) <= 4'
      error.log.count:
        description: Error or higher severity entries.
        conditions:
          - 'Int(body["PRIORITY"]) <= 3'
```

#### What to expect

After starting the Collector with this configuration, a new chart appears in the
Netdata dashboard under the **OpenTelemetry** section. The chart shows the
count of matching log entries over time. You can use this chart as the basis
for a Netdata health alert.

### Parse unstructured logs into structured attributes

Log entries often arrive as plain-text strings with no structured fields. The
`transform` processor can parse these entries into structured attributes using
OTTL expressions, making them easier to filter, search, and process downstream.

#### Example: Parse JSON log lines

Applications that write JSON-formatted log lines (for example,
`{"level":"error","msg":"connection refused","duration_ms":312}`) send the
entire JSON string as the log body. The `transform` processor can parse it into
individual attributes:

```yaml
processors:
  transform:
    error_mode: ignore
    log_statements:
      - context: log
        statements:
          - merge_maps(attributes, ParseJSON(body), "insert")
```

This takes the JSON string in `body`, parses it, and merges the resulting
key-value pairs into the log entry's `attributes` map. After processing, each
JSON field (`level`, `msg`, `duration_ms`) becomes a separate attribute that you
can search and filter on in the Netdata Logs tab.

#### Full pipeline with JSON parsing

This pipeline tails a JSON log file, parses each line into structured
attributes, and sends the result to Netdata:

```yaml
receivers:
  filelog:
    include:
      - /var/log/myapp/*.log
    start_at: end

processors:
  transform:
    error_mode: ignore
    log_statements:
      - context: log
        statements:
          - merge_maps(attributes, ParseJSON(body), "insert")

exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [filelog]
      processors: [transform]
      exporters: [otlp]
```

> **NOTE**
>
> The `error_mode: ignore` setting causes the processor to silently skip log
> entries that are not valid JSON instead of failing the entire batch. This is
> useful when a log file contains a mix of structured and unstructured entries.

#### Other common transformations

OTTL supports many functions beyond JSON parsing. The following examples show
a few patterns that are useful for log processing.

Set a severity level from a parsed attribute:

```yaml
- set(severity_text, attributes["level"])
```

Add a static attribute to all log entries:

```yaml
- set(attributes["environment"], "production")
```

Drop log entries that match a condition:

```yaml
- set(attributes["drop"], true) where IsMatch(body, "(?i)health.check")
```

For the full list of available functions and syntax, see the
[OTTL documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl#opentelemetry-transformation-language)
and the
[transform processor documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/transformprocessor).

### Create a Netdata alert on a derived metric

Once the log-derived metric appears as a chart in Netdata, you can configure a
health alert on it — for example, to trigger a notification when warning-level
log entries spike. The chart name to use in the alert's `on` field is visible
in the Netdata dashboard under the **OpenTelemetry** section.

For instructions on creating and configuring health alerts, see the
[health alerts reference](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md).

## Troubleshooting

### No data arrives (any signal)

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

3. Confirm that the Collector (or SDK) can reach the Netdata endpoint:

   ```bash
   curl -v telnet://localhost:4317
   ```

4. Verify the sender uses **gRPC**, not OTLP/HTTP: the exporter endpoint must
   target port `4317` (the plugin has no HTTP receiver on 4318). SDKs need
   `OTEL_EXPORTER_OTLP_PROTOCOL=grpc`.

5. Check the plugin's own log for errors (each worker logs under its own
   identifier):

   ```bash
   journalctl SYSLOG_IDENTIFIER=otel-plugin --since "-10 min"
   journalctl SYSLOG_IDENTIFIER=otel-plugin/ingestor --since "-10 min"
   ```

### The sender reports connection refused

The Netdata OTel plugin listens on `127.0.0.1:4317` by default. If the
Collector runs on a different host, change the endpoint in `otel.yaml` to
`0.0.0.0:4317` and ensure that the firewall allows traffic on port 4317.

On the same host, note that the plugin binds the IPv4 address `127.0.0.1`.
If `localhost` resolves to the IPv6 `::1` first on your system, the exporter
gets "connection refused" — use `127.0.0.1:4317` as the exporter endpoint
instead of `localhost:4317`.

### Metrics: charts appear but have only one dimension

The metric likely needs a chart configuration file to group data points into
multi-dimension charts. See
[Organize metrics with chart configuration files](#organize-metrics-with-chart-configuration-files).

### Logs: the Collector reports permission denied for journald

The Collector process must have permission to read the systemd journal. Either
run the Collector as root or add its user to the `systemd-journal` group:

```bash
sudo usermod -aG systemd-journal otelcol-contrib
sudo systemctl restart otelcol-contrib
```

### Logs: the file log receiver does not pick up existing lines

By default, the `filelog` receiver starts reading from the end of files
(`start_at: end`). Change this to `beginning` if you need to ingest historical
log entries:

```yaml
receivers:
  filelog:
    include:
      - /var/log/myapp/*.log
    start_at: beginning
```

> **IMPORTANT**
>
> Setting `start_at: beginning` causes the receiver to re-read entire files on
> first start. For large files, this can produce a burst of log entries. Use
> the `storage` extension to persist file offsets across Collector restarts and
> avoid duplicate ingestion.

### Logs: entries missing although the sender reports success

Check their timestamps: log records older than 24 hours (or more than 10
minutes in the future) are rejected at ingestion and reported to the sender
via OTLP `partial_success` — most senders log that. Clock skew between sender
and agent shows up here. See `ingest.max_age` in the
[plugin reference](netdata-otel-plugin-reference.md).

### The log-derived count metric does not appear

1. Verify that the `count` connector is listed as an exporter in the logs
   pipeline **and** as a receiver in the metrics pipeline.

2. Check that the condition matches your log entries. Use the `debug` exporter
   to inspect log bodies:

   ```yaml
   exporters:
     debug:
       verbosity: detailed
   ```

   Add `debug` to the logs pipeline exporters and check the Collector output.

3. Verify that log entries contain the fields you are filtering on. Journal
   fields like `PRIORITY` and `MESSAGE` are inside the `body` map.

## Additional resources

- [Netdata OTel plugin reference](netdata-otel-plugin-reference.md)
- [Netdata health alerts configuration](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md)
- [OpenTelemetry Collector documentation](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Collector Contrib receivers](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver)
- [Count connector](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/countconnector)
- [Transform processor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/transformprocessor)
- [OTTL (OpenTelemetry Transformation Language)](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl)
- [Host Metrics receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/hostmetricsreceiver)
- [Prometheus receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver)
- [Redis receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/redisreceiver)
- [NGINX receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/nginxreceiver)
- [PostgreSQL receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/postgresqlreceiver)
- [Journald receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/journaldreceiver)
- [File Log receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver)
- [Syslog receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/syslogreceiver)
