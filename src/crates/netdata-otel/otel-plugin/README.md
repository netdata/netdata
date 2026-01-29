# OpenTelemetry Metrics (otel.plugin)

`otel.plugin` is a [Netdata](https://github.com/netdata/netdata) external plugin,
enabling users to ingest, store and visualize OpenTelemetry metrics in charts.

## Configuration

Edit the [otel.yaml](https://github.com/netdata/netdata/blob/master/src/crates/jf/otel-plugin/configs/otel.yaml)
configuration file using `edit-config` from the Netdata
[config directory](/docs/netdata-agent/configuration/README.md#locate-your-config-directory),
which is typically located under `/etc/netdata`.

```bash
cd /etc/netdata # Replace this path with your Netdata config directory
sudo ./edit-config otel.yaml
```

### gRPC Endpoint

By default `otel.plugin` listens for incoming OTLP-formatted metrics on
`localhost:4317` via gRPC. Users can set up a secure TLS connection by
updating the TLS configuration in the `endpoint` section:

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

### Metrics

The `metrics` section allows users to specify the directory containing
configuration files for mapping OpenTelemetry metrics to Netdata chart
instances, and the number of metric samples the `otel.plugin` will use for
detecting their collection interval:

```yaml
metrics:
  # Directory with configuration files for mapping OTEL metrics to Netdata charts
  # (relative paths are resolved based on Netdata's user configuration directory)
  chart_configs_dir: otel.d/v1/metrics/

  # Number of samples to buffer for collection interval detection
  buffer_samples: 10
```

## Mapping OpenTelemetry metrics to Netdata chart instances

Without an explicit mapping, the `otel.plugin` defaults to creating distinct
chart instances based on the attributes of each data point in a metric. Users
can place their YAML chart configuration files under `otel.d/v1/metrics` to
override, or fine-tune, the default mapping.

For each instrumentation scope and metric name, the configuration defines
the attributes that the `otel.plugin` will use when creating new chart
instances and dimension names.

For example, the following bit from the
[otel.d/v1/metrics/hostmetrics.yaml](https://github.com/netdata/netdata/blob/master/src/crates/jf/otel-plugin/configs/otel.d/v1/metrics/hostmetrics-receiver.yaml)
 configuration file for the [hostmetrics](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/hostmetricsreceiver/internal/scraper/networkscraper/documentation.md) receiver:
```yaml
select:
  instrumentation_scope_name: hostmetricsreceiver.*networkscraper
  metric_name: system.network.connections
extract:
    chart_instance_pattern: metric.attributes.protocol
    dimension_name: metric.attributes.state
```
will apply to metrics whose instrumentation scope and metric names match the
corresponding regular expressions specified in the values of the
`instrumentation_scope_name` and `metric_name` keys. Similarly, the values of
the `protocol` and `state` attributes of each data point in the matched metric
will be used to create a new chart instance with the proper dimension names.
