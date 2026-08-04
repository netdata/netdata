# Collect logs with OpenTelemetry

The Netdata Agent includes a built-in OpenTelemetry plugin that receives logs
over the OTLP/gRPC protocol. You can pair it with the OpenTelemetry Collector to
collect logs from files, the systemd journal, syslog sources, and more — and
explore them in the Netdata Logs tab with zero custom code.

This guide walks you through the setup, explains how logs flow from the
Collector to Netdata, and provides ready-to-use pipeline configurations for
three common receivers.

## How it works

The data flows through three components:

```
┌──────────────────────┐       OTLP/gRPC         ┌──────────────────────┐
│  OTel Collector      │ ─────────────────────►  │  Netdata Agent       │
│                      │     (port 4317)         │                      │
│  receivers:          │                         │  otel.plugin:        │
│    journald          │                         │    receives OTLP     │
│    filelog            │                         │    writes journal    │
│    syslog            │                         │    files             │
│    ...               │                         │                      │
│                      │                         │  Logs tab:           │
│  exporters:          │                         │    search and        │
│    otlp ─────────────┼─────────────────────►   │    explore           │
└──────────────────────┘                         └──────────────────────┘
```

1. **Receivers** in the OTel Collector collect or receive log entries from
   infrastructure components.
2. The Collector's **OTLP exporter** sends those logs to the Netdata Agent
   over gRPC.
3. The Netdata **OTel plugin** (`otel.plugin`) writes incoming logs to
   systemd-compatible journal files. You can explore them in the **Logs tab**
   of the Netdata dashboard.

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
> Use the [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib.git) distribution.
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

The most commonly adjusted log options are:

| Option | Default | Description |
|:-------|:--------|:------------|
| `logs.journal_dir` | `/var/log/netdata/otel/v1` | Directory to store journal files for ingested logs. |
| `logs.size_of_journal_file` | `100MB` | Maximum file size before rotating to a new journal file. |
| `logs.entries_of_journal_file` | `50000` | Maximum log entries per journal file. |
| `logs.duration_of_journal_file` | `2 hours` | Maximum time span within a single journal file. |
| `logs.number_of_journal_files` | `10` | Maximum number of journal files to keep. |
| `logs.size_of_journal_files` | `1GB` | Maximum total size of all journal files. |
| `logs.duration_of_journal_files` | `7 days` | Maximum age of journal files. |
| `logs.store_otlp_json` | `no` | Store the complete OTLP JSON in each log entry. Useful for debugging. |

For example, to keep more log history:

```yaml
logs:
  number_of_journal_files: 20
  duration_of_journal_files: "14 days"
```

> **IMPORTANT**
>
> The OTel plugin shares a single gRPC endpoint for both metrics and logs. If
> you already configured the endpoint for metrics collection, no additional
> endpoint configuration is needed for logs.

For the full list of options, see the
[OTel plugin reference](/src/crates/netdata-otel/otel-plugin/integrations/opentelemetry.md).

## Set up the OTel Collector pipeline

Every OTel Collector configuration file has three top-level sections:

- **`receivers`** — where logs come from.
- **`exporters`** — where logs go.
- **`service.pipelines`** — which receivers and exporters to wire together.

All examples in this guide share the same exporter and pipeline structure. Only
the `receivers` section changes.

### The OTLP exporter

Every pipeline in this guide uses the OTLP exporter to send logs to Netdata:

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

Wire a receiver to the OTLP exporter in the `service.pipelines` section. Note
that log pipelines use the `logs` key, not `metrics`:

```yaml
service:
  pipelines:
    logs:
      receivers: [journald]
      exporters: [otlp]
```

You can list multiple receivers in the same pipeline. The sections below show
complete, working configurations for each receiver.

## Example 1: Journald

The `journald` receiver reads log entries directly from the systemd journal. It
requires no external service and is the easiest way to verify that your log
pipeline works, because every systemd-based Linux host already produces journal
entries.

### Prerequisites for this receiver

- The host uses systemd (most modern Linux distributions).
- The `journalctl` binary is available on the system.
- The Collector process has permission to read the journal (typically requires
  running as root or being in the `systemd-journal` group).

### Collector configuration

Create or edit the Collector configuration file (typically
`/etc/otelcol-contrib/config.yaml`):

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

### Start the Collector

```bash
sudo systemctl restart otelcol-contrib
```

After a few seconds, journal log entries appear in the Netdata dashboard under
the **Logs** tab.

### Collect from all units

To collect all journal entries without filtering by unit:

```yaml
receivers:
  journald:
    priority: info
    start_at: end
```

For the full list of configuration options, see the
[Journald receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/journaldreceiver).

## Example 2: File logs

The `filelog` receiver tails and parses log entries from files. It covers the
common case of applications that write structured or unstructured logs to files
on disk, such as web server access logs, application logs, or container logs.

### Prerequisites for this receiver

- One or more log files that the Collector process can read.

### Collector configuration

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

### Parse structured logs

If your application writes JSON-formatted logs, add a `json_parser` operator to
extract fields:

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

### Tail multiple paths

Add more entries under `include` or use glob patterns:

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

### Key configuration options

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

## Example 3: Syslog

The `syslog` receiver listens for syslog messages over TCP or UDP and parses
them according to RFC 3164 or RFC 5424. This covers the common case of network
devices, appliances, and legacy applications that send syslog messages.

### Prerequisites for this receiver

- One or more syslog sources configured to send messages to the Collector's
  listen address.

### Collector configuration

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

### Listen over TCP

To receive syslog messages over TCP instead of UDP:

```yaml
receivers:
  syslog:
    tcp:
      listen_address: "0.0.0.0:54526"
    protocol: rfc5424
```

### Use RFC 3164 format

Older devices and applications often use the RFC 3164 (BSD syslog) format. Set
the `protocol` field accordingly and specify a `location` for timestamp parsing:

```yaml
receivers:
  syslog:
    udp:
      listen_address: "0.0.0.0:54526"
    protocol: rfc3164
    location: UTC
```

### Key configuration options

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

## Combine multiple receivers in a single pipeline

You can run multiple receivers in the same Collector instance. List all receivers
in the pipeline definition:

```yaml
receivers:
  journald:
    priority: info
    start_at: end

  filelog:
    include:
      - /var/log/myapp/*.log
    start_at: end

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
      receivers: [journald, filelog, syslog]
      exporters: [otlp]
```

All logs from all receivers flow to Netdata through the same OTLP exporter.
Each receiver operates independently — if one fails, the others continue to
collect.

## Troubleshooting

### No logs appear in the Netdata Logs tab

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

4. Verify that the journal directory exists and contains files:

   ```bash
   ls -la /var/log/netdata/otel/v1/
   ```

### The Collector reports permission denied for journald

The Collector process must have permission to read the systemd journal. Either
run the Collector as root or add its user to the `systemd-journal` group:

```bash
sudo usermod -aG systemd-journal otelcol-contrib
sudo systemctl restart otelcol-contrib
```

### The Collector reports connection refused

The Netdata OTel plugin listens on `127.0.0.1:4317` by default. If the
Collector runs on a different host, change the endpoint in `otel.yaml` to
`0.0.0.0:4317` and ensure that the firewall allows traffic on port 4317.

### File log receiver does not pick up existing lines

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

## Additional resources

- [Netdata OTel plugin reference](/src/crates/netdata-otel/otel-plugin/integrations/opentelemetry.md)
- [OpenTelemetry Collector documentation](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Collector Contrib receivers](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver)
- [Journald receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/journaldreceiver)
- [File Log receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver)
- [Syslog receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/syslogreceiver)
