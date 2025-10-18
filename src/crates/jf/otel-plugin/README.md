# OpenTelemetry Metrics (otel.plugin)

`otel.plugin` is a [Netdata](https://github.com/netdata/netdata) external plugin that enables you to ingest, store and visualize OpenTelemetry metrics and logs.

## Protocol Support

**Supported**: gRPC (OTLP/gRPC) on port 4317  
**Not Supported**: HTTP/JSON (OTLP/HTTP) on port 4318

The plugin only accepts OpenTelemetry data via gRPC. Configure your OpenTelemetry exporters accordingly:

```yaml
# OpenTelemetry Collector configuration
exporters:
  otlp:
    endpoint: "localhost:4317"
    tls:
      insecure: true  # or configure TLS certificates
```

:::info

**Supported Telemetry Types**

**[Metrics](#metrics)**: Full support via OTLP/gRPC
- Gauge, Sum, Histogram, Summary, ExponentialHistogram
- Automatic chart creation based on metric attributes
- Configurable metric-to-chart mapping via profiles

**[Logs](#logs)**: Full support via OTLP/gRPC
- Stored in systemd-journal format
- Configurable rotation and retention policies
- Queryable via Netdata Log Explorer

:::

## Configuration

Edit the [otel.yml](https://github.com/netdata/netdata/blob/master/src/crates/jf/otel-plugin/configs/otel.yml) configuration file using `edit-config` from the Netdata [config directory](/docs/netdata-agent/configuration/README.md#the-netdata-config-directory), which is typically located under `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config otel.yml
```

### Configuration File Resolution

The plugin loads configuration in this order:
1. **User config**: `/etc/netdata/otel.yml` (takes precedence)
2. **Stock config**: `/usr/lib/netdata/conf.d/otel.yml` (fallback)
3. **CLI arguments**: Used when not running under Netdata

### gRPC Endpoint

By default `otel.plugin` listens for incoming OTLP-formatted metrics on `localhost:4317` via gRPC. You can set up a secure TLS connection by updating the TLS configuration in the `endpoint` section:

```yaml
endpoint:
  # gRPC endpoint to listen on for OpenTelemetry data
  path: "127.0.0.1:4317"
  # Path to TLS certificate file (enables TLS when provided)
  tls_cert_path: null
  # Path to TLS private key file (required when TLS certificate is provided)
  tls_key_path: null
  # Path to TLS CA certificate file for client authentication (optional)
  tls_ca_cert_path: null
```

:::warning

**TLS Validation Rules:**
- If `tls_cert_path` is provided, `tls_key_path` **must** also be provided (and vice versa)
- `tls_ca_cert_path` is optional (for client authentication)
- Empty paths are rejected - omit the field entirely if not using TLS

:::

### Metrics

The `metrics` section allows you to configure metric ingestion, chart creation limits, and debugging options:

```yaml
metrics:
  # Directory with configuration files for mapping OTEL metrics to Netdata charts
  # (relative paths are resolved based on Netdata's user configuration directory)
  chart_configs_dir: otel.d/v1/metrics
  
  # Number of samples to buffer for collection interval detection
  buffer_samples: 10
  
  # Maximum number of new charts to create per collection interval
  # Prevents resource exhaustion during metric bursts
  throttle_charts: 100
  
  # Print flattened metrics to stdout for debugging, instead of ingesting them
  print_flattened: false
```

:::tip

**Performance Tuning:**
- Use `throttle_charts` to prevent overwhelming Netdata when monitoring dynamic workloads (e.g., Kubernetes auto-scaling)
- Enable `print_flattened` to debug metric mapping issues - metrics will be printed to stdout instead of creating charts

:::

### Logs

The `logs` section configures storage and retention for OpenTelemetry logs:

```yaml
logs:
  # Directory to store journal files for logs
  # (relative paths are resolved based on Netdata's log directory)
  journal_dir: otel/v1

  # Rotation policies
  size_of_journal_file: "100MB"        # Max size per journal file
  duration_of_journal_file: "2 hours"  # Max time span per journal file

  # Retention policies
  number_of_journal_files: 10          # Max number of journal files
  size_of_journal_files: "1GB"         # Max total size of all journal files
  duration_of_journal_files: "7 days"  # Max age of journal entries
```

**Rotation Triggers**: A new journal file is created when either:
- The current file exceeds `size_of_journal_file`, OR
- The time span of entries exceeds `duration_of_journal_file`

**Retention Enforcement**: Old journal files are deleted when any of these limits are exceeded:
- Total file count > `number_of_journal_files`
- Total size > `size_of_journal_files`
- Oldest entry age > `duration_of_journal_files`

Logs are stored in systemd-journal format and can be queried using Netdata's Log Explorer or standard journalctl tools.

## Mapping OpenTelemetry Metrics to Netdata Charts

Without explicit mapping, the `otel.plugin` defaults to creating distinct chart instances based on the attributes of each data point in a metric. You can place YAML chart configuration files under `otel.d/v1/metrics` to override or fine-tune the default mapping.

For each instrumentation scope and metric name, the configuration defines the attributes that the `otel.plugin` uses when creating new chart instances and dimension names.

### Configuration Schema

```yaml
- select:
    # Match instrumentation scope name (regex pattern, optional)
    instrumentation_scope_name: hostmetricsreceiver.*networkscraper
    
    # Match instrumentation scope version (regex pattern, optional)
    instrumentation_scope_version: v0\.112\..*
    
    # Match metric name (regex pattern, required)
    metric_name: system\.network\.connections
    
  extract:
    # Pattern for creating chart instances (optional)
    chart_instance_pattern: metric.attributes.protocol
    
    # Pattern for dimension names (optional)
    dimension_name: metric.attributes.state
```

### Configuration Examples

```yaml
# Example 1: Single chart with multiple dimensions (no chart_instance_pattern)
- select:
    instrumentation_scope_name: .*hostmetricsreceiver.*cpuscraper$
    metric_name: system\.cpu\.utilization
  extract:
    dimension_name: metric.attributes.state  # All CPUs in one chart

# Example 2: Multiple charts, one per instance (with chart_instance_pattern)
- select:
    instrumentation_scope_name: .*hostmetricsreceiver.*cpuscraper$
    metric_name: system\.cpu\.utilization
  extract:
    chart_instance_pattern: metric.attributes.cpu  # One chart per CPU
    dimension_name: metric.attributes.state

# Example 3: Version-specific matching (optional)
- select:
    instrumentation_scope_name: .*hostmetricsreceiver.*networkscraper$
    instrumentation_scope_version: v0\.112\..*  # Match specific versions
    metric_name: system\.network\.connections
  extract:
    chart_instance_pattern: metric.attributes.protocol
    dimension_name: metric.attributes.state
```

:::tip

**Configuration Flexibility:**
- Both `chart_instance_pattern` and `dimension_name` are **optional**
- Omit `chart_instance_pattern` to create a single chart for all data points
- Use `instrumentation_scope_version` to handle breaking changes in OpenTelemetry collector versions

:::

### Hostmetrics Receiver Example

The following example from the [hostmetrics-receiver.yml](https://github.com/netdata/netdata/blob/master/src/crates/jf/otel-plugin/configs/otel.d/v1/metrics/hostmetrics-receiver.yml) configuration file shows how to map [hostmetrics](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/hostmetricsreceiver/internal/scraper/networkscraper/documentation.md) receiver metrics:

```yaml
- select:
    instrumentation_scope_name: hostmetricsreceiver.*networkscraper
    metric_name: system\.network\.connections
  extract:
    chart_instance_pattern: metric.attributes.protocol
    dimension_name: metric.attributes.state
```

This configuration:
- Matches metrics from the hostmetrics receiver's network scraper
- Creates separate charts for each network protocol (TCP, UDP)
- Uses connection states (ESTABLISHED, TIME_WAIT, etc.) as dimension names

## Debugging and Troubleshooting

### Debug Mode

Enable debug mode to troubleshoot metric mapping issues:

```yaml
metrics:
  print_flattened: true
```

When enabled, the plugin prints flattened metric data points to stdout instead of creating charts. This helps you understand the structure of incoming metrics and debug mapping configurations.

### Common Issues

**Metrics not appearing in charts:**
1. Check that your OpenTelemetry exporter is configured for gRPC (port 4317, not 4318)
2. Verify TLS configuration if using secure connections
3. Enable `print_flattened` to see raw metric data
4. Check chart configuration files for correct regex patterns

**Too many charts being created:**
1. Adjust `throttle_charts` to limit chart creation rate
2. Review your chart configuration to use `chart_instance_pattern` appropriately
3. Consider creating fewer, more aggregated charts

## SDK Configuration Examples

<details>
<summary><strong>Python</strong></summary><br/>

```python
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter

exporter = OTLPMetricExporter(
    endpoint="localhost:4317",
    insecure=True  # or configure TLS
)
```

</details>

<br/>

<details>
<summary><strong>Node.js</strong></summary><br/>

```javascript
const { OTLPMetricExporter } = require('@opentelemetry/exporter-otlp-grpc');

const exporter = new OTLPMetricExporter({
  url: 'grpc://localhost:4317',
});
```

</details>

<br/>

<details>
<summary><strong>Go</strong></summary><br/>

```go
import "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"

exporter, err := otlpmetricgrpc.New(ctx,
    otlpmetricgrpc.WithEndpoint("localhost:4317"),
    otlpmetricgrpc.WithTLSCredentials(insecure.NewCredentials()),
)
```

</details>