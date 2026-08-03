# Collect Logs with OpenTelemetry Collector

Use these recipes to send systemd journal entries and application log files through an OpenTelemetry Collector to Netdata. For network-device syslog, use the dedicated [OpenTelemetry Collector syslog setup](/docs/npm/syslog/otel-collector.md).

Before you begin, complete [Ingest OpenTelemetry Metrics and Logs](/docs/opentelemetry/otlp-ingestion.md). The examples use OpenTelemetry Collector Contrib `0.157.0` and the local, plaintext loopback endpoint from that guide. Use TLS when the Collector and Netdata Agent are on different hosts.

## Shared exporter

Add this exporter once, then combine it with a receiver and pipeline block from a recipe below:

```yaml
exporters:
  otlp_grpc/netdata:
    endpoint: "127.0.0.1:4317"
    tls:
      insecure: true
```

Set `service.name` and, when useful, `service.namespace` as resource attributes so operators can identify each stream consistently in the Netdata Logs tab.

The `service.pipelines.logs` blocks are alternatives. To run several receivers in one Collector, define one logs pipeline and list every enabled receiver in its `receivers` array; apply only processors appropriate to all records in that pipeline or use separate named pipelines.

## Systemd journal

The `journald` receiver runs `journalctl`, so the binary must be available to the Collector and the Collector service account must be able to read the journal. On a typical host installation, add that account to the `systemd-journal` group and restart the Collector service. Container deployments also need the host journal, a compatible `journalctl` binary, and the required permissions mounted into the container.

For an installation whose service account and unit are both named `otelcol-contrib`, grant access with:

```bash
sudo usermod -aG systemd-journal otelcol-contrib
sudo systemctl restart otelcol-contrib
```

Check the service unit and substitute its configured user when your package uses a different account.

This configuration reads new warning-or-higher entries from two units, persists the journal cursor across Collector restarts, and identifies the stream in Netdata:

```yaml
receivers:
  journald/netdata:
    units: [sshd.service, docker.service]
    priority: warning
    start_at: end
    storage: file_storage/journald

processors:
  resource/journald:
    attributes:
      - key: service.name
        value: systemd-journal
        action: upsert

extensions:
  file_storage/journald:
    directory: /var/lib/otelcol/netdata-journald
    create_directory: true

service:
  extensions: [file_storage/journald]
  pipelines:
    logs:
      receivers: [journald/netdata]
      processors: [resource/journald]
      exporters: [otlp_grpc/netdata]
```

The Collector service account needs write access to the storage directory. With storage enabled, the receiver resumes from its saved cursor instead of replaying entries after a normal restart.

Useful journald options:

| Option        | Default                              | Purpose                                                                             |
|:--------------|:-------------------------------------|:------------------------------------------------------------------------------------|
| `directory`   | `/run/log/journal` or `/run/journal` | Read a specific journal directory. Omit it to use the receiver's default discovery. |
| `units`       | none                                 | Match any listed systemd unit.                                                      |
| `identifiers` | none                                 | Match any listed syslog identifier.                                                 |
| `matches`     | none                                 | Match explicit journal field/value combinations.                                    |
| `priority`    | `info`                               | Include this priority and more important entries.                                   |
| `start_at`    | `end`                                | Start at `beginning` or `end` when no saved cursor applies.                         |
| `storage`     | none                                 | Persist cursors through a named storage extension.                                  |

Different filter families are combined with logical AND; values within `units`, `identifiers`, or `matches` are combined with logical OR. To collect all units at `info` or higher, omit `units` and `identifiers`:

```yaml
receivers:
  journald/netdata:
    priority: info
    start_at: end
```

Use `start_at: beginning` only for an intentional backfill. It can replay a large journal, increase Collector load, and send records that Netdata rejects because their timestamps are outside its acceptance window.

See the pinned upstream [journald receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.157.0/receiver/journaldreceiver) for container settings, field matches, namespaces, retry behavior, and the complete option set.

## Application log files

The `file_log` receiver tails files matched by glob patterns. The Collector service account must be able to read every selected file and write to the storage directory when persistent offsets are enabled.

This configuration tails application logs, excludes a noisy debug file, persists offsets, and identifies the stream in Netdata:

```yaml
receivers:
  file_log/my_application:
    include:
      - /var/log/myapp/*.log
      - /var/log/nginx/access.log
      - /var/log/nginx/error.log
    exclude:
      - /var/log/myapp/debug.log
    start_at: end
    storage: file_storage/my_application

processors:
  resource/my_application:
    attributes:
      - key: service.name
        value: my-application
        action: upsert

extensions:
  file_storage/my_application:
    directory: /var/lib/otelcol/netdata-filelog
    create_directory: true

service:
  extensions: [file_storage/my_application]
  pipelines:
    logs:
      receivers: [file_log/my_application]
      processors: [resource/my_application]
      exporters: [otlp_grpc/netdata]
```

The receiver tracks files by identity and fingerprint, including through common rotation patterns. Persistent storage lets it resume from saved offsets after a restart. Without storage, offsets exist only in memory; restarting with `start_at: beginning` can re-ingest old records, while `start_at: end` can skip existing records that are no longer being written.

### Parse JSON lines

When each physical line is one JSON object, add a `json_parser` operator. The embedded timestamp and severity settings promote fields into the OpenTelemetry log record:

```yaml
receivers:
  file_log/my_application:
    include: [/var/log/myapp/*.json]
    start_at: end
    operators:
      - type: json_parser
        timestamp:
          parse_from: attributes.time
          layout: "%Y-%m-%dT%H:%M:%S.%LZ"
        severity:
          parse_from: attributes.level
```

If formats vary or transformation must happen after several receivers, use the shared [Transformations](/docs/opentelemetry/transformations.md) guide instead.

### Join multiline entries

For stack traces in which each new record starts with an ISO-style timestamp, add a multiline rule:

```yaml
receivers:
  file_log/my_application:
    include: [/var/log/myapp/*.log]
    start_at: end
    multiline:
      line_start_pattern: '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}'
```

Configure exactly one of `line_start_pattern` or `line_end_pattern`. Test the expression against real samples: a wrong boundary can merge many records into a very large entry or split a stack trace into unrelated records.

### Important file options

| Option          | Default  | Purpose                                                                           |
|:----------------|:---------|:----------------------------------------------------------------------------------|
| `include`       | required | File glob patterns to read.                                                       |
| `exclude`       | `[]`     | Patterns removed from the included set.                                           |
| `start_at`      | `end`    | Start at `beginning` or `end` when no saved offset applies.                       |
| `multiline`     | none     | Join physical lines using one start or end pattern.                               |
| `operators`     | `[]`     | Parse timestamps, severity, JSON, regex fields, or other structure before export. |
| `encoding`      | `utf-8`  | Decode the input using the selected character encoding.                           |
| `poll_interval` | `200ms`  | Control how often the receiver checks for file changes.                           |
| `storage`       | none     | Persist offsets through a named storage extension.                                |

See the pinned upstream [file log receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.157.0/receiver/filelogreceiver) for rotation behavior, retry settings, maximum record size, supported encodings, and the complete operator set.

## Syslog

The Collector Contrib `syslog` receiver can listen over TCP or UDP and parse RFC 3164 or RFC 5424 records. Netdata maintains a separate end-to-end guide because durable network ingestion also requires listener exposure, firewall rules, transport choices, and persistent Collector queues. Follow [Collect syslog with the OpenTelemetry Collector](/docs/npm/syslog/otel-collector.md) instead of duplicating that configuration here.

## Verify and troubleshoot logs

- In Netdata, open the node's Logs tab, select `otel-logs`, then choose the configured service with the **Services** selector. The stored service field is `resource.attributes.service.name`.
- If journald reports permission errors, verify access by running `journalctl` as the Collector service account, then restart the Collector after changing group membership.
- If existing file lines are absent, check `start_at` and whether a saved cursor or offset already exists. Do not delete storage state casually; doing so can replay data.
- If the Collector reports successful export but records are absent, inspect their timestamps. Netdata accepts records from up to 24 hours in the past through 10 minutes in the future and reports rejected records through OTLP `partial_success`.
- If records are duplicated after restart, enable a `file_storage` extension and reference it from the receiver. Confirm that its directory survives service and container restarts.
