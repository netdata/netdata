# Netdata OpenTelemetry plugin reference

The Netdata OpenTelemetry plugin (`otel.plugin`) lets a Netdata Agent receive
OpenTelemetry **metrics and logs** over the OTLP/gRPC protocol, from any
compatible source — an OpenTelemetry Collector, an SDK, or an instrumented
application.

- **Metrics** become regular Netdata charts, with full dashboard and alerting
  support.
- **Logs** are stored in the plugin's own storage engine and queried from the
  Netdata Logs tab (the `otel-logs` source).

This page is the configuration reference. For end-to-end OpenTelemetry
Collector pipelines, see
[collecting OpenTelemetry data with Netdata](collecting-opentelemetry-signals.md).

## How the plugin runs

The plugin is an external Netdata plugin: the Netdata Agent starts and
supervises the `otel-plugin` binary (installed under
`/usr/libexec/netdata/plugins.d/`). It appears in Netdata as plugin
`otel.plugin`. It starts automatically with the Agent and needs no
configuration to run with defaults.

Both signals arrive on a **single gRPC endpoint** (default
`127.0.0.1:4317`). There is no OTLP/HTTP receiver — senders must use
OTLP/gRPC.

## Configuration file: `otel.yaml`

Edit the configuration with:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config otel.yaml
```

Configuration is resolved from three layers, highest priority first:

1. Environment variables (`NETDATA_OTEL_CFG_*`, see
   [Environment variable overrides](#environment-variable-overrides))
2. The user file `/etc/netdata/otel.yaml`
3. The stock file `/usr/lib/netdata/conf.d/otel.yaml`

The user file is a **partial override**: include only the fields you want to
change; everything else keeps its stock or built-in default.

**Parsing is strict.** An unknown key anywhere in `otel.yaml` — or an
unrecognized `NETDATA_OTEL_CFG_*` variable — is a hard error and the plugin
refuses to start, naming the offending file and key. A typo cannot be silently
ignored. A user file written for the former (experimental) plugin schema is
also refused, with a migration guide printed in the error.

At startup the plugin logs its full resolved configuration (with
`remote_storage.uri` redacted to its scheme) for supportability.

## Option reference

### `endpoint` — the OTLP/gRPC listener

Shared by metrics and logs.

| Option | Default | Description |
|:-------|:--------|:------------|
| `endpoint.path` | `127.0.0.1:4317` | Bind address (`host:port`) for incoming OTLP/gRPC. Use `0.0.0.0:4317` to accept remote senders. |
| `endpoint.tls_cert_path` | unset | Path to a TLS certificate file. Providing it enables TLS; requires `tls_key_path`. |
| `endpoint.tls_key_path` | unset | Path to the TLS private key. Required when the certificate is provided. |
| `endpoint.tls_ca_cert_path` | unset | Path to a CA certificate used to verify **client** certificates (mutual TLS). Requires both `tls_cert_path` and `tls_key_path`. |

Certificate and key must be provided together; a CA certificate without both
is a startup error.

```yaml
endpoint:
  path: "0.0.0.0:4317"
  tls_cert_path: /etc/netdata/ssl/cert.pem
  tls_key_path: /etc/netdata/ssl/key.pem
```

### `metrics` — mapping OTLP metrics to charts

| Option | Default | Description |
|:-------|:--------|:------------|
| `metrics.chart_configs_dir` | `/etc/netdata/otel.d/v1/metrics/` | Directory with YAML files mapping OTLP metrics to Netdata charts (see [Metric chart configuration files](#metric-chart-configuration-files)). |
| `metrics.interval_secs` | `10` | Collection interval in seconds (1–3600). Defines the chart update frequency. |
| `metrics.grace_period_secs` | `60` | After the last data point, the plugin waits this long before gap-filling. When `interval_secs` is overridden, this auto-derives as 5 × `interval_secs` unless set explicitly. |
| `metrics.expiry_duration_secs` | `900` | Charts receiving no data for this long are removed. |
| `metrics.max_new_charts_per_request` | `100` | Maximum new charts created per gRPC request. Caps cardinality explosions from high-cardinality label combinations. |

Metrics are not written to the plugin's log storage — they flow into the
Netdata database like any other collector's metrics. Charts get the context
`otel.<metric name>` (for example `otel.system.cpu.time`).

### `base_dir` — storage root for logs

| Option | Default | Description |
|:-------|:--------|:------------|
| `base_dir` | `/var/log/netdata/otel/v2` | Mandatory absolute path. Log storage lives under it, at `{base_dir}/logs/...`. |

A relative path is rejected at startup.

### `remote_storage` — optional object-storage offload

Data is uploaded **in addition to**
being kept on local disk. Locally deleted (retention-expired) data that is
still in remote storage is transparently fetched back into a bounded local
cache when a query needs it.

| Option | Default | Description |
|:-------|:--------|:------------|
| `remote_storage.enabled` | `false` | Enable the remote backend. |
| `remote_storage.uri` | `fs:///var/log/netdata/otel/v2/remote` | Backend location. The scheme selects the backend (`fs`, `s3`); non-secret options go in the query string, e.g. `s3://bucket/prefix?region=us-east-1` or `s3://bucket/prefix?region=us-east-1&endpoint=https://minio.example:9000`. |
| `remote_storage.read_cache_max_size` | `1GB` | Byte cap for the local read-back cache. |
| `remote_storage.startup_op_timeout` | `5min` | Per-operation timeout for the startup catalog sync (each remote LIST and download). Large fleets sharing one bucket may need a higher value. |

**Never put credentials in `otel.yaml` or in the URI.** Credentials are read
from the netdata process environment and standard cloud locations — for S3:
`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`, `~/.aws/credentials`, or an
attached instance role. To pass environment variables to the netdata service,
use a root-only systemd `EnvironmentFile`. The URI is never logged verbatim —
startup logging redacts it to its scheme.

### `logs` — storage tuning

The `logs` section contains a `rotation` and a `retention` policy; the
settings of each live under a `default` entry:

```yaml
logs:
  rotation:
    default:
      max_file_size: "25MB"
      max_entries: 50000
  retention:
    default:
      max_files: 100000
      max_total_size: "1GB"
      max_age: "7 days"
```

**Rotation** — when the current data file rolls over (whichever limit is hit
first):

| Option | Default | Description |
|:-------|:--------|:------------|
| `rotation.default.max_file_size` | `25MB` | Maximum size of the current file. |
| `rotation.default.max_entries` | `50000` | Maximum log records per file. |
| `rotation.default.max_file_duration` | `15min` | Maximum age of the current file; seals idle streams promptly. Hidden from the stock file. |

**Retention** — how much data is kept on local disk (oldest files are deleted
when any limit is exceeded):

| Option | Default | Description |
|:-------|:--------|:------------|
| `retention.default.max_files` | `100000` | Maximum number of stored files. |
| `retention.default.max_total_size` | `1GB` | Maximum total size on disk. |
| `retention.default.max_age` | `7 days` | Maximum age of stored data. |
| `retention.default.horizon` | `10 years` | How long catalog (index metadata) files are kept — the remote-archive depth. Hidden from the stock file. Must exceed `max_age` by more than a day; a violation is a startup error. |

**Advanced knobs** (hidden from the stock file, available in the user file
and via environment variables):

| Option | Default | Description |
|:-------|:--------|:------------|
| `crc_enabled` | `true` | Verify stored data with checksums. |
| `compression_enabled` | `true` | Compress stored data (LZ4). |
| `catalog.rotation_count` | `10` | Index-file entries accumulated before a catalog file is written. |
| `catalog.rotation_period` | `15min` | Age at which a non-empty catalog accumulator is written even below `rotation_count`. |
| `ingest.max_age` | `24h` | Reject records with timestamps older than this. Rejections are reported to the sender via OTLP `partial_success`. |
| `ingest.future_skew` | `10min` | Accept records dated at most this far in the future (clock-skew tolerance). |

**Legacy viewer:**

| Option | Default | Description |
|:-------|:--------|:------------|
| `logs.journal_dir` | Netdata log directory + `/otel/v1` (typically `/var/log/netdata/otel/v1`) | Read-only pointer to the systemd-journal files written by the **former, experimental** OTel plugin, served by the `legacy-otel-logs` function. The plugin never writes there. |

## Environment variable overrides

Every option can be overridden with a `NETDATA_OTEL_CFG_*` environment
variable — the highest-priority layer. The name is the option path in
capitals with dots replaced by underscores. An unrecognized
`NETDATA_OTEL_CFG_*` name is a startup error (typos are caught, not ignored).

| Variable | Overrides |
|:---------|:----------|
| `NETDATA_OTEL_CFG_ENDPOINT_PATH` | `endpoint.path` |
| `NETDATA_OTEL_CFG_ENDPOINT_TLS_CERT_PATH` | `endpoint.tls_cert_path` |
| `NETDATA_OTEL_CFG_ENDPOINT_TLS_KEY_PATH` | `endpoint.tls_key_path` |
| `NETDATA_OTEL_CFG_ENDPOINT_TLS_CA_CERT_PATH` | `endpoint.tls_ca_cert_path` |
| `NETDATA_OTEL_CFG_METRICS_CHART_CONFIGS_DIR` | `metrics.chart_configs_dir` |
| `NETDATA_OTEL_CFG_METRICS_INTERVAL_SECS` | `metrics.interval_secs` |
| `NETDATA_OTEL_CFG_METRICS_GRACE_PERIOD_SECS` | `metrics.grace_period_secs` |
| `NETDATA_OTEL_CFG_METRICS_EXPIRY_DURATION_SECS` | `metrics.expiry_duration_secs` |
| `NETDATA_OTEL_CFG_METRICS_MAX_NEW_CHARTS_PER_REQUEST` | `metrics.max_new_charts_per_request` |
| `NETDATA_OTEL_CFG_BASE_DIR` | `base_dir` |
| `NETDATA_OTEL_CFG_REMOTE_STORAGE_ENABLED` | `remote_storage.enabled` |
| `NETDATA_OTEL_CFG_REMOTE_STORAGE_URI` | `remote_storage.uri` |
| `NETDATA_OTEL_CFG_REMOTE_STORAGE_READ_CACHE_MAX_SIZE` | `remote_storage.read_cache_max_size` |
| `NETDATA_OTEL_CFG_REMOTE_STORAGE_STARTUP_OP_TIMEOUT` | `remote_storage.startup_op_timeout` |
| `NETDATA_OTEL_CFG_LOGS_ROTATION_MAX_FILE_SIZE` | `logs.rotation.default.max_file_size` |
| `NETDATA_OTEL_CFG_LOGS_ROTATION_MAX_ENTRIES` | `logs.rotation.default.max_entries` |
| `NETDATA_OTEL_CFG_LOGS_ROTATION_MAX_FILE_DURATION` | `logs.rotation.default.max_file_duration` |
| `NETDATA_OTEL_CFG_LOGS_RETENTION_MAX_FILES` | `logs.retention.default.max_files` |
| `NETDATA_OTEL_CFG_LOGS_RETENTION_MAX_TOTAL_SIZE` | `logs.retention.default.max_total_size` |
| `NETDATA_OTEL_CFG_LOGS_RETENTION_MAX_AGE` | `logs.retention.default.max_age` |
| `NETDATA_OTEL_CFG_LOGS_RETENTION_HORIZON` | `logs.retention.default.horizon` |
| `NETDATA_OTEL_CFG_LOGS_CATALOG_ROTATION_COUNT` | `logs.catalog.rotation_count` |
| `NETDATA_OTEL_CFG_LOGS_CATALOG_ROTATION_PERIOD` | `logs.catalog.rotation_period` |
| `NETDATA_OTEL_CFG_LOGS_INGEST_MAX_AGE` | `logs.ingest.max_age` |
| `NETDATA_OTEL_CFG_LOGS_INGEST_FUTURE_SKEW` | `logs.ingest.future_skew` |
| `NETDATA_OTEL_CFG_LOGS_CRC_ENABLED` | `logs.crc_enabled` |
| `NETDATA_OTEL_CFG_LOGS_COMPRESSION_ENABLED` | `logs.compression_enabled` |

`logs.journal_dir` has no environment variable (YAML-only).

## Metric chart configuration files

Without any configuration, every incoming OTLP metric becomes a chart with
default settings — one dimension per attribute combination. Chart
configuration files let you group data points of a metric into
multi-dimension charts and tune per-metric timing.

Files live in `metrics.chart_configs_dir` (default
`/etc/netdata/otel.d/v1/metrics/`). Netdata ships a stock mapping for the
OpenTelemetry Collector's `hostmetrics` receiver at
`/usr/lib/netdata/conf.d/otel.d/v1/metrics/hostmetrics-receiver.yaml`; user
files take priority.

Each file maps metric names to a list of rules:

```yaml
metrics:
  "system.cpu.time":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*cpuscraper$
      dimension_attribute_key: state

  "system.memory.usage":
    - instrumentation_scope:
        name: .*hostmetricsreceiver.*memoryscraper$
      dimension_attribute_key: state
      interval_secs: 5
```

| Rule option | Description |
|:------------|:------------|
| `instrumentation_scope.name` | Regex matched against the metric's instrumentation scope name. |
| `instrumentation_scope.version` | Regex matched against the instrumentation scope version. |
| `dimension_attribute_key` | Data-point attribute whose value becomes the dimension name. Data points sharing the remaining attributes are grouped into one chart, one dimension per value of this attribute. |
| `interval_secs` | Per-metric collection interval override (1–3600 seconds). |
| `grace_period_secs` | Per-metric grace period override. |

Resulting charts have context `otel.<metric name>` and family
`<metric name>` with dots replaced by slashes. Cumulative and delta sums are
rate-normalized (their units gain a `/s` suffix).

## Startup validation

The plugin refuses to start — with a specific error — when:

- any key in `otel.yaml` is unknown (strict parsing, all sections), or an
  unrecognized `NETDATA_OTEL_CFG_*` variable is set;
- the user file uses the former experimental schema (`logs.size_of_journal_file`
  and friends) — the error includes the old-to-new key mapping;
- `base_dir` is empty or not absolute;
- `remote_storage.enabled` is `true` with an empty `uri`;
- TLS certificate and key are not provided together, or a CA certificate is
  provided without both;
- `endpoint.path` is not `host:port`;
- `retention.horizon` does not exceed `retention.max_age` by more than a day.
