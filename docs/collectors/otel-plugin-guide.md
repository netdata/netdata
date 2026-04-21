# OpenTelemetry Plugin

## Overview

The **OpenTelemetry (OTel) plugin** enables Netdata Agent to receive OpenTelemetry metrics and logs via OTLP/gRPC protocol from any compatible source — collectors, SDKs, or instrumented applications.

It transforms OpenTelemetry's event-based metrics into Netdata's fixed-interval collection model and stores logs in systemd-compatible journal files.

:::info

The plugin automatically creates Netdata charts from incoming metrics with full alerting support. Logs are written to journal files and can be explored through the Netdata Logs tab.

:::note
This means charts are created automatically from incoming OTLP data points without requiring manual chart definitions.
:::


## How It Works

When an OpenTelemetry source sends data to Netdata:

1. **Data arrives** via OTLP/gRPC on the configured endpoint (default: `127.0.0.1:4317`)
2. **MetricsService** or **LogsService** processes the request
3. For metrics:
   - **ChartConfigManager** resolves chart configuration from YAML files
   - **ChartManager** creates or updates charts based on metric identity
   - **Aggregators** map event-based data points to fixed-interval values
4. For logs:
   - Data is flattened to JSON and sorted by timestamp
   - Entries are written to systemd-compatible journal files
5. **Tick loop** runs every second, finalizes charts, and emits data to Netdata

**Data Flow**

```text
┌──────────────────────┐         OTLP/gRPC          ┌──────────────────────┐
│ OTel Source          │ ─────────────────────► │ Netdata OTel Plugin │
│ (Collector/SDK)      │    (port 4317)       │                      │
│                      │                         │  MetricsService      │
│ Metrics:            │                         │  LogsService         │
│  - Gauge           │                         │                      │
│  - Sum (Delta)     │                         │  ChartConfigManager  │
│  - Sum (Cumulative)│                         │  ChartManager        │
│  - Histogram       │                         │  Aggregators         │
│ Logs:               │                         │                      │
│  - Log records     │                         │  Journal Writer       │
└──────────────────────┘                         └──────┬───────────────┘
                                                   │
                                                   ↓
                                            ┌──────────────────────┐
                                            │ Netdata Time-Series  │
                                            │ Database & Charts   │
                                            └──────────────────────┘
```

## Configuration

Configure the plugin via `otel.yaml` in the Netdata configuration directory.

### Basic Configuration

```yaml
endpoint:
  path: "127.0.0.1:4317"

metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics/
  interval_secs: 10
  grace_period_secs: 60
  expiry_duration_secs: 900
  max_new_charts_per_request: 100

logs:
  journal_dir: /var/log/netdata/otel-journals
```

### TLS Configuration

```yaml
endpoint:
  path: "0.0.0.0:4317"
  tls_cert_path: /etc/netdata/ssl/cert.pem
  tls_key_path: /etc/netdata/ssl/key.pem
  tls_ca_cert_path: /etc/netdata/ssl/ca.pem

metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics/
  interval_secs: 10

logs:
  journal_dir: /var/log/netdata/otel-journals
```

### Configuration Options

| Option | Description | Default | Required |
|--------|-------------|----------|----------|
| `endpoint.path` | gRPC endpoint to listen on for incoming OTLP data | `127.0.0.1:4317` | No |
| `endpoint.tls_cert_path` | Path to TLS certificate file. Enables TLS when provided. | — | No |
| `endpoint.tls_key_path` | Path to TLS private key file. Required when TLS certificate is provided. | — | No |
| `endpoint.tls_ca_cert_path` | Path to TLS CA certificate file for client authentication. | — | No |
| `metrics.chart_configs_dir` | Directory containing YAML files that define how OTLP metrics map to Netdata charts | `/etc/netdata/otel.d/v1/metrics/` | No |
| `metrics.interval_secs` | Collection interval in seconds (1–3600). Defines Netdata chart update frequency | `10` | No |
| `metrics.grace_period_secs` | Grace period in seconds. After last data point, plugin waits this long before gap-filling | `60` | No |
| `metrics.expiry_duration_secs` | Expiry duration in seconds. Charts with no data for this long are removed | `900` | No |
| `metrics.max_new_charts_per_request` | Maximum new charts per gRPC request. Limits cardinality explosion from high-cardinality label combinations | `100` | No |
| `logs.journal_dir` | Directory to store journal files for ingested logs | — | Yes |
| `logs.size_of_journal_file` | Maximum file size before rotating to a new journal file | `100MB` | No |
| `logs.entries_of_journal_file` | Maximum log entries per journal file | `50000` | No |
| `logs.duration_of_journal_file` | Maximum time span within a single journal file | `2 hours` | No |
| `logs.number_of_journal_files` | Maximum number of journal files to keep | `10` | No |
| `logs.size_of_journal_files` | Maximum total size of all journal files | `1GB` | No |
| `logs.duration_of_journal_files` | Maximum age of journal files | `7 days` | No |
| `logs.store_otlp_json` | Store complete OTLP JSON representation in OTLP_JSON field for debugging and reprocessing | `false` | No |

**Auto-derivation Rules:**

- When `interval_secs` is set but `grace_period_secs` is not, grace period auto-derives as `5 * interval`
- When auto-derived grace exceeds expiry and expiry was not explicitly set, expiry is bumped to match grace
- Per-metric `interval_secs` overrides also auto-derive grace period using the same `5 * interval` rule

## Examples

### Example: Host Metrics Receiver

Complete setup for collecting system-level metrics (CPU, memory, disk, network, filesystem, load, paging, processes) from the host OS.

<details><summary>View Collector configuration</summary>

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

</details>

<details><summary>View Netdata plugin configuration</summary>

```yaml
endpoint:
  path: "127.0.0.1:4317"

metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics/
  interval_secs: 10
  grace_period_secs: 60
  expiry_duration_secs: 900
  max_new_charts_per_request: 100

logs:
  journal_dir: /var/log/netdata/otel-journals
```

</details>

**What you'll see:**

| Metric | Description |
|--------|-------------|
| `system.cpu.time` | CPU time per core broken down by state (user, system, idle, iowait, etc.) |
| `system.memory.usage` | Memory usage by type (used, free, cached, buffered) |
| `system.disk.io` | Bytes read and written per disk device |
| `system.network.io` | Bytes transmitted and received per network interface |
| `system.filesystem.usage` | Used, free, and reserved bytes per mount point |
| `system.cpu.load_average.1m` | 1-minute load average |
| `system.paging.usage` | Swap usage by state (used, free) |
| `system.processes.count` | Process count by status (running, sleeping, blocked, etc.) |

</details>

### Example: Prometheus Endpoint

Scrape any HTTP endpoint that exposes metrics in Prometheus format. Bridges the Prometheus exporter ecosystem into OTel pipeline without running a separate Prometheus server.

<details><summary>View Collector configuration</summary>

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "node-exporter"
          scrape_interval: 10s
          static_configs:
            - targets: ["localhost:9100"]
        - job_name: "my-application"
          scrape_interval: 10s
          static_configs:
            - targets: ["localhost:8080"]
          metrics_path: "/metrics"

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

</details>

<details><summary>View Netdata plugin configuration</summary>

```yaml
endpoint:
  path: "127.0.0.1:4317"

metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics/
  interval_secs: 10
  max_new_charts_per_request: 100

logs:
  journal_dir: /var/log/netdata/otel-journals
```

</details>

<details><summary>View metric mapping for Prometheus</summary>

```yaml
metrics:
  "prometheus_requests_total":
    - instrumentation_scope:
        name: .*prometheus.*
      dimension_attribute_key: handler
```

</details>

**What you'll see:**

- Request rate per HTTP handler path (if labeled)
- Prometheus counter metrics converted to per-second rates
- Prometheus gauge metrics as instantaneous values

</details>

### Example: Redis

Collect metrics from a Redis instance (client connections, memory usage, keyspace statistics, command throughput, replication status).

<details><summary>View Collector configuration</summary>

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

</details>

<details><summary>View Collector with authentication</summary>

```yaml
receivers:
  redis:
    endpoint: "localhost:6379"
    collection_interval: 10s
    password: "${env:REDIS_PASSWORD}"

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

</details>

<details><summary>View metric mapping</summary>

```yaml
metrics:
  "redis.memory.used":
    - dimension_attribute_key: db

  "redis.commands.processed":
    - dimension_attribute_key: db

  "redis.keyspace.hits":
    - dimension_attribute_key: db
```

</details>

**What you'll see:**

- Memory usage per database (`used`, `rss`, etc.)
- Commands processed per database
- Keyspace hits/misses per database
- Network input/output per database
- Evicted and expired keys per database

**Key metrics:**

| Metric | Description |
|--------|-------------|
| `redis.memory.used` | Total memory allocated by Redis (bytes) |
| `redis.memory.rss` | Resident set size reported by OS (bytes) |
| `redis.clients.connected` | Number of connected clients |
| `redis.commands.processed` | Total commands processed since startup |
| `redis.keyspace.hits` | Successful key lookups |
| `redis.keyspace.misses` | Failed key lookups |
| `redis.keys.expired` | Keys removed due to TTL expiration |
| `redis.keys.evicted` | Keys evicted due to memory pressure |
| `redis.net.input` | Total bytes received |
| `redis.net.output` | Total bytes sent |
| `redis.db.keys` | Number of keys per database |

</details>

### Example: NGINX

Collect metrics from NGINX's stub status module (accepted/handled connections, current connections by state, total requests).

<details><summary>View Collector configuration</summary>

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

</details>

**Prerequisite:** Enable stub status endpoint in NGINX configuration:

```nginx
server {
    location /status {
        stub_status;
        allow 127.0.0.1;
        deny all;
    }
}
```

**What you'll see:**

- Total accepted client connections
- Total handled connections
- Current connections by state (`active`, `reading`, `writing`, `waiting`)
- Total client requests served

**Key metrics:**

| Metric | Description |
|--------|-------------|
| `nginx.connections_accepted` | Total accepted client connections |
| `nginx.connections_handled` | Total handled connections |
| `nginx.connections_current` | Current connections by state |
| `nginx.requests` | Total client requests served |

</details>

### Example: PostgreSQL

Collect database performance metrics from PostgreSQL (connection counts, query throughput, table and index statistics, buffer usage).

<details><summary>View Collector configuration</summary>

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

</details>

**Prerequisite:** Create dedicated monitoring user:

```sql
CREATE USER otel WITH PASSWORD 'your-secure-password';
GRANT pg_monitor TO otel;
```

<details><summary>View metric mapping</summary>

```yaml
metrics:
  "postgresql.commits":
    - dimension_attribute_key: database

  "postgresql.db.size":
    - dimension_attribute_key: database

  "postgresql.operations":
    - dimension_attribute_key: database
```

</details>

**What you'll see:**

- Transactions committed/rolled back per database
- Database size in bytes
- Row operations (inserts, updates, deletes) per database
- Block reads by source (heap, index, toast) per database
- Connection count and maximum allowed connections
- Number of tables and index scans per database

**Key metrics:**

| Metric | Description |
|--------|-------------|
| `postgresql.commits` | Transactions committed per database |
| `postgresql.rollbacks` | Transactions rolled back per database |
| `postgresql.db.size` | Database size in bytes |
| `postgresql.rows` | Number of rows by state (live, dead) |
| `postgresql.operations` | Row operations (inserts, updates, deletes) |
| `postgresql.blocks_read` | Block reads by source |
| `postgresql.connection.max` | Maximum allowed connections |
| `postgresql.table.count` | Number of user tables per database |
| `postgresql.index.scans` | Number of index scans |

</details>

### Example: Multiple Receivers

Combine hostmetrics, Redis, and NGINX receivers in a single pipeline. All metrics flow to Netdata through the same OTLP exporter.

<details><summary>View Collector configuration</summary>

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

</details>

<details><summary>View Netdata plugin configuration</summary>

```yaml
endpoint:
  path: "127.0.0.1:4317"

metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics/
  interval_secs: 10
  max_new_charts_per_request: 100

logs:
  journal_dir: /var/log/netdata/otel-journals
```

</details>

**What you'll see:**

- All host metrics (CPU, memory, disk, network, load, paging, processes)
- Redis metrics per database
- NGINX metrics (connections, requests)

**Note:** Each receiver operates independently. If one fails, the others continue to collect.

</details>

---

## Metric Mapping

Chart configuration files in `chart_configs_dir` control how OTLP metrics map to Netdata charts.

### Chart Configuration File Format

```yaml
metrics:
  "<metric_name>":
    - instrumentation_scope:
        name: <regex>           # Optional: Match instrumentation scope name
        version: <regex>          # Optional: Match instrumentation scope version
      dimension_attribute_key: <string>   # Attribute key whose value becomes dimension name
      interval_secs: <number>               # Optional: Override collection interval
      grace_period_secs: <number>          # Optional: Override grace period
```

**Fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `<metric_name>` | Yes | Exact OTLP metric name to match |
| `instrumentation_scope.name` | No | Regex to match instrumentation scope name |
| `instrumentation_scope.version` | No | Regex to match instrumentation scope version |
| `dimension_attribute_key` | No | Data point attribute whose value becomes dimension name. Default: `"value"` |
| `interval_secs` | No | Per-metric collection interval override (1–3600 seconds) |
| `grace_period_secs` | No | Per-metric grace period override |

**Configuration Resolution:**

1. User config files take priority over stock configs
2. Per-metric overrides take priority over global defaults
3. Missing values fall back to global defaults
4. First matching `instrumentation_scope` pattern wins

### Example: Host Metrics Receiver Mapping

```yaml
metrics:
  "system.cpu.time":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*cpuscraper$
      dimension_attribute_key: state

  "system.network.io":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*networkscraper$
      dimension_attribute_key: direction

  "system.memory.usage":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*memoryscraper$
      dimension_attribute_key: state
      interval_secs: 5
```

**What this does:**

- Maps `system.cpu.time` to one chart per CPU core with dimensions for each state (`user`, `system`, `idle`, `iowait`, etc.)
- Maps `system.network.io` to one chart per network interface with `transmit` and `receive` dimensions
- Maps `system.memory.usage` to one chart per memory type, updating every 5 seconds instead of default 10

### Chart Identity

A chart is uniquely identified by a hash computed from:

1. **Resource attributes** (service.name, host.name, etc.)
2. **Instrumentation scope** (name, version, attributes)
3. **Metric name** and **metadata** (description, unit)
4. **Metric data kind** (Gauge, Sum, Histogram, etc.)
5. **Data point attributes** (excluding the dimension attribute key)

This means:
- Different resources (services, hosts) produce separate charts
- Same metric from different collectors requires separate `instrumentation_scope` patterns
- High-cardinality labels (request IDs, trace IDs) cause chart explosion

### Cardinality Protection

The `max_new_charts_per_request` setting limits how many new charts can be created in a single gRPC request. This prevents runaway chart creation from metrics with high-cardinality labels (e.g., per-request metrics).

## Aggregation Types

The plugin converts OpenTelemetry's event-based metrics to Netdata's fixed-interval model using three aggregation types.

### Gauge

Represents instantaneous values with no defined aggregation semantics.

**Behavior:**

- When multiple values arrive within a collection interval, keep the latest (by timestamp)
- Gap filling repeats the last emitted value

**Example:** Temperature, queue depth, buffer size

```yaml
metrics:
  "queue.size":
    - dimension_attribute_key: queue_name
```

Result: One chart per queue name with a single dimension representing current size.

### Delta Sum

Represents change since the last report.

**Behavior:**

- When multiple delta values arrive within a collection interval, sum them
- Gap filling returns 0 (no change occurred)

**Example:** Request count per interval, bytes transferred per interval

```yaml
metrics:
  "http.server.requests":
    - dimension_attribute_key: method
```

Result: One chart per HTTP method with a dimension showing requests per second.

### Cumulative Sum

Represents total since a fixed start time. The plugin computes deltas automatically.

**Behavior:**

- Tracks the cumulative value and computes change since previous report
- Detects counter restarts via `start_time_unix_nano` changes
- On restart, returns 0 for that interval and establishes new baseline
- Gap filling returns 0 (no change in delta)

**Example:** Total bytes sent, total requests processed

```yaml
metrics:
  "process.cpu.seconds":
    - dimension_attribute_key: process_name
```

Result: One chart per process showing CPU seconds per second (delta of cumulative counter).

## Histogram Support

Histograms capture distribution data — how values fall into bucket ranges. Each histogram becomes four charts.

The bucket heatmap visualizes distribution: each bucket dimension shows observation count within its range. Temporality determines aggregation — delta histograms aggregate counts as they arrive, cumulative histograms convert to deltas automatically.

Three line charts provide statistical summary. Sum chart tracks total observed values. Count chart shows observation count. Together these give throughput and average. If histogram includes min/max data, a fourth chart displays both extremes, updating on each observation.

| Chart | Suffix | Type | Aggregation |
|-------|---------|------|-------------|
| Bucket heatmap | `.bucket` | Heatmap | DeltaSum or CumulativeSum |
| Sum | `.sum` | Line | DeltaSum or CumulativeSum |
| Count | `.count` | Line | DeltaSum or CumulativeSum |
| Min/Max | `.minmax` | Line | Gauge (latest) |

**Example:** `http.server.duration` metric produces `http.server.duration.<hash>.bucket` (heatmap), `sum`, `count`, and `minmax` charts.

## Summary Support

Summaries provide count, sum, and optional quantile statistics. The plugin treats all summaries as cumulative and converts to deltas automatically.

Count and sum charts track observation frequency and total value — both using CumulativeSum aggregation. The quantiles chart exists only when source provides `quantile_values`, showing percentile thresholds as latest observations.

| Chart | Suffix | Type | Aggregation |
|-------|---------|------|-------------|
| Count | `.count` | Line | CumulativeSum |
| Sum | `.sum` | Line | CumulativeSum |
| Quantiles | `.quantiles` | Line | Gauge (latest) |

## Chart Lifecycle

**Creation.** First data point triggers chart creation for each unique combination of resource attributes, instrumentation scope, metric metadata, and data point attributes. No pre-declaration required.

**Gap filling.** After grace period expires without new data: Gauge repeats last value, Delta and Cumulative sums return 0.

**Expiry.** Charts inactive for `expiry_duration_secs` are removed.

**Timing validation.** Must satisfy `0 < interval <= 3600` and `interval < grace <= expiry`. Violations fall back to defaults with warning.

## Log Processing

**Flattening.** OTLP records convert to systemd-compatible JSON: timestamps from nanoseconds to microseconds, attributes become key-value pairs. `OTLP_JSON` field stores original JSON when `store_otlp_json: true`.

**Sorting.** Entries sort by timestamp before writing to optimize structure, indexing, and compression.

**Journal storage.** Logs write to systemd-compatible files with configurable rotation (size, entry count, duration) and retention (file count, total size, age) policies.

**Timestamp selection.** Prefers `time_unix_nano` if present and non-zero, otherwise `observed_time_unix_nano`.

## Default Behavior

### Auto-Detection

The plugin starts automatically and listens on `127.0.0.1:4317` for incoming OTLP/gRPC connections.

### Limits

The default configuration does not impose limits on data collection other than `max_new_charts_per_request: 100`.

### Performance Impact

The default configuration is not expected to impose a significant performance impact on the system. The main performance considerations are:

- **Cardinality explosion:** High-cardinality labels (request IDs, trace IDs) can create many charts
- **Journal rotation:** Aggressive rotation policies may cause frequent I/O
- **TLS overhead:** Encryption adds CPU overhead for processing

## Security Considerations

### TLS Configuration

When enabling TLS:

1. **Endpoint exposure:** Binding to `0.0.0.0:4317` exposes the endpoint to all network interfaces. Ensure firewall rules restrict access to trusted sources.

2. **Certificate management:**
   - `tls_cert_path` and `tls_key_path` must be readable by the netdata user
   - Certificates should be renewed before expiration
   - Use strong cipher suites (minimum TLS 1.2 enforced)

3. **Client authentication:**
   - Use `tls_ca_cert_path` to require client certificates
   - Verify client certificates are from trusted sources

### Data Isolation

- Separate resources (services, applications) create separate charts and log streams
- Resource attributes like `service.name` provide natural isolation boundaries

## Troubleshooting

### No Charts Appearing

1. **Verify plugin is running:**
   ```bash
   ps aux | grep otel
   ```

2. **Check for configuration errors in logs:**
   ```bash
   journalctl -u netdata -f | grep otel
   ```

3. **Test OTLP source connectivity:**
   ```bash
   nc -zv 127.0.0.1 4317
   ```

### Charts Have Only One Dimension

- **Cause:** Metric likely needs a `dimension_attribute_key` configuration to group data points
- **Solution:** Add a chart configuration file specifying which attribute to use for dimensions

### High Chart Count

- **Cause:** High-cardinality labels creating many unique chart identities
- **Solutions:**
  1. Reduce `max_new_charts_per_request` to limit creation
  2. Add chart configuration to use a lower-cardinality attribute as `dimension_attribute_key`
  3. Configure OTLP source to drop or downsample high-cardinality attributes

### The Collector Reports Connection Refused

**Cause:** The Netdata OTel plugin listens on `127.0.0.1:4317` by default. If the Collector runs on a different host, it cannot reach the plugin endpoint.

**Solutions:**
1. **Change Netdata endpoint to accept remote connections:**

   ```yaml
   endpoint:
     path: "0.0.0.0:4317"
   ```

2. **Ensure firewall allows traffic on port 4317:**

   ```bash
   sudo ufw allow 4317/tcp
   # or
   sudo firewall-cmd --add-port=4317/tcp
   ```

3. **Verify Collector can reach the endpoint:**

   ```bash
   nc -zv <netdata-host-ip> 4317
   ```

**Security Note:** When binding to `0.0.0.0`, ensure that firewall or network policy restricts access to trusted sources. The gRPC endpoint does not require authentication by default. Consider enabling TLS for production environments.

### Logs Not Appearing

1. **Check journal directory exists and is writable:**
   ```bash
   ls -la /var/log/netdata/otel-journals
   ```

2. **Verify journal files contain entries:**
   ```bash
   journalctl --directory=/var/log/netdata/otel-journals --file=*.journal -n 20
   ```

3. **Check for journal rotation errors in plugin logs**

### Configuration File Not Loading

1. **Verify file format is correct YAML:**
   ```bash
   yamllint /etc/netdata/otel.d/v1/metrics/*.yaml
   ```

2. **Check for old format detection in logs:**
   - Plugin logs errors when detecting old chart config format
   - Migrate to new format: `metrics:` root key

3. **Ensure directory path is correct:**
   - Use absolute paths
   - Verify directory exists and is readable by netdata user

## Additional Resources

- [Netdata OTel plugin reference](https://github.com/netdata/netdata/tree/master/src/crates/netdata-otel/otel-plugin)
- [OpenTelemetry Collector documentation](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Collector Contrib receivers](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver)
- [Host Metrics receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/hostmetricsreceiver)
- [Prometheus receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver)
- [Redis receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/redisreceiver)
- [NGINX receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/nginxreceiver)
- [PostgreSQL receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/postgresqlexreceiver)
