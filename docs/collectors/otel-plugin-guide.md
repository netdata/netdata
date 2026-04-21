# OpenTelemetry Plugin

## Overview

The **OpenTelemetry (OTel) plugin** enables Netdata Agent to receive OpenTelemetry metrics and logs via OTLP/gRPC protocol from any compatible source — collectors, SDKs, or instrumented applications.

It transforms OpenTelemetry's event-based metrics into Netdata's fixed-interval collection model and stores logs in systemd-compatible journal files.

:::info

The plugin automatically creates Netdata charts from incoming metrics with full alerting support. Logs are written to journal files and can be explored through the Netdata Logs tab.

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

Histogram metrics are decomposed into multiple charts:

### Bucket Chart (Heatmap)

- **Metric name:** `<metric>.<chart_hash>.bucket`
- **Type:** Heatmap
- **Dimensions:** One per histogram bucket, named by bucket bounds
- **Value:** Count of observations in that bucket
- **Aggregation:** DeltaSum or CumulativeSum based on temporality

### Sum Chart

- **Metric name:** `<metric>.<chart_hash>.sum`
- **Type:** Line
- **Dimensions:** Single dimension named `sum`
- **Value:** Sum of all observed values
- **Aggregation:** DeltaSum or CumulativeSum based on temporality

### Count Chart

- **Metric name:** `<metric>.<chart_hash>.count`
- **Type:** Line
- **Dimensions:** Single dimension named `count`
- **Value:** Number of observations
- **Aggregation:** DeltaSum or CumulativeSum based on temporality

### Min/Max Chart

- **Metric name:** `<metric>.<chart_hash>.minmax`
- **Type:** Line
- **Dimensions:** Two dimensions: `min` and `max`
- **Values:** Minimum and maximum observed values
- **Aggregation:** Gauge (latest observation)

**Example:**

```yaml
metrics:
  "http.server.duration":
    - dimension_attribute_key: service_name
```

Results in five charts for each service:
- `http.server.duration.<hash>.bucket` - Request duration distribution (heatmap)
- `http.server.duration.<hash>.sum` - Total request duration
- `http.server.duration.<hash>.count` - Request count
- `http.server.duration.<hash>.minmax` - Minimum and maximum request duration
- `http.server.duration.<hash>.quantiles` - (if quantile values provided by source) Quantile values

## Summary Support

Summary metrics provide sum and count statistics, and optional quantile values, and are decomposed into multiple charts:

### Count Chart

- **Metric name:** `<metric>.<chart_hash>.count`
- **Type:** Line
- **Dimensions:** Single dimension named `count`
- **Value:** Number of observations
- **Aggregation:** CumulativeSum

### Sum Chart

- **Metric name:** `<metric>.<chart_hash>.sum`
- **Type:** Line
- **Dimensions:** Single dimension named `sum`
- **Value:** Sum of all observed values
- **Aggregation:** CumulativeSum

### Quantiles Chart

- **Metric name:** `<metric>.<chart_hash>.quantiles`
- **Type:** Line
- **Dimensions:** One dimension per quantile (e.g., `0.5`, `0.9`, `0.95`, `0.99`)
- **Values:** Quantile threshold values
- **Aggregation:** Gauge (latest observation)
- **Note:** Only created if the source provides `quantile_values` field

## Chart Lifecycle

### Creation

Charts are created when the first data point arrives for a unique combination of:

- Resource attributes
- Instrumentation scope
- Metric name and metadata
- Data point attributes (excluding dimension attribute)

The plugin creates charts automatically — no pre-declaration required.

### Gap Filling

After the grace period expires without new data:

- **Gauge:** Repeats last emitted value
- **Delta Sum:** Returns 0
- **Cumulative Sum:** Returns 0 (no change observed)

### Expiry

Charts with no data for longer than `expiry_duration_secs` are removed and no longer emitted.

**Timing Constraints:**

- Must satisfy: `0 < interval <= 3600`
- Must satisfy: `interval < grace <= expiry`
- Violated timing falls back to hardcoded defaults with a warning

## Log Processing

Logs received via OTLP are processed as follows:

### Flattening

OTLP log records are flattened to a JSON format compatible with systemd journal format:

- Timestamps converted from nanoseconds to microseconds
- Resource, scope, and log attributes become key-value pairs
- `OTLP_JSON` field stores complete original JSON when `store_otlp_json: true`

### Sorting

Log entries are sorted by timestamp before writing to journal files to:

- Optimize journal file structure and indexing
- Improve query performance
- Enhance compression efficiency

### Journal Storage

Logs are written to systemd-compatible journal files with configurable policies:

**Rotation Policy:**
- `size_of_journal_file`: Rotate when file exceeds this size
- `entries_of_journal_file`: Rotate when entry count exceeds this limit
- `duration_of_journal_file`: Rotate when time span exceeds this duration

**Retention Policy:**
- `number_of_journal_files`: Keep at most this many files
- `size_of_journal_files`: Keep at most this total size
- `duration_of_journal_files`: Delete files older than this age

### Timestamp Selection

Per OTLP spec:

1. Use `time_unix_nano` if present and non-zero
2. Otherwise use `observed_time_unix_nano`

This prioritizes the explicit event time over the observation time.

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
