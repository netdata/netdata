# Collect Logs with OpenTelemetry Collector

Use these recipes to send systemd journal entries, application log files, Windows event channels, Kubernetes container logs, and the macOS unified log through an OpenTelemetry Collector to Netdata. For network-device syslog, use the dedicated [OpenTelemetry Collector syslog setup](/docs/npm/syslog/otel-collector.md). For when to centralize a source at all, see [Centralizing Logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md).

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

When the Collector sends to a remote Netdata Agent, use TLS, select the tenant, and give the exporter a persistent
queue so a Collector restart or a network outage does not lose records:

```yaml
extensions:
  file_storage/netdata:
    directory: /var/lib/otelcol/netdata
    create_directory: true

exporters:
  otlp_grpc/netdata:
    endpoint: "logs.example.com:4317"
    tls:
      ca_file: /etc/otelcol/netdata-ca.pem
      # For mutual TLS, add the client certificate and key:
      # cert_file: /etc/otelcol/client-cert.pem
      # key_file: /etc/otelcol/client-key.pem
    headers:
      X-Scope-OrgID: production
    sending_queue:
      storage: file_storage/netdata

service:
  extensions: [file_storage/netdata]
```

The receiving Agent's endpoint, TLS, and tenant settings are described in
[Securing the OTLP Endpoint](/docs/opentelemetry/securing-the-otlp-endpoint.md). When you combine blocks, list every
extension you use in one `service.extensions` list.

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

See the upstream [journald receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/journaldreceiver) for container settings, field matches, namespaces, retry behavior, and the complete option set.

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

See the upstream [file log receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver) for rotation behavior, retry settings, maximum record size, supported encodings, and the complete operator set.

## Windows event channels

Install the Collector as a Windows service and use one `windows_event_log` receiver per channel. The service account
needs read access to each channel; the Security channel in particular requires it explicitly.

```yaml
extensions:
  file_storage/windows:
    directory: C:\ProgramData\otelcol\netdata
    create_directory: true

receivers:
  windows_event_log/application:
    channel: Application
    start_at: end
    storage: file_storage/windows
  windows_event_log/system:
    channel: System
    start_at: end
    storage: file_storage/windows
  windows_event_log/security:
    channel: Security
    start_at: end
    storage: file_storage/windows

processors:
  resource/windows:
    attributes:
      - key: service.namespace
        value: windows
        action: upsert
      - key: service.name
        value: event-log
        action: upsert

service:
  extensions: [file_storage/windows]
  pipelines:
    logs:
      receivers: [windows_event_log/application, windows_event_log/system, windows_event_log/security]
      processors: [resource/windows]
      exporters: [otlp_grpc/netdata]
```

On a Windows Event Collector, add a receiver for the `ForwardedEvents` channel to centralize what the forwarders send.
The `storage` extension keeps a bookmark per channel, so a restart resumes where it stopped. Event bodies are
structured records by default; set `raw: true` to keep the original XML instead. The receiver builds only on Windows,
so validate this configuration on a Windows host. See the upstream
[windowseventlog receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/windowseventlogreceiver)
for remote collection, XML queries, and the complete option set.

## Kubernetes and containers

Container logs are files under `/var/log/pods` on each node, not journal entries. Run the Collector as a DaemonSet with
the OpenTelemetry Collector Helm chart, whose `logsCollection` preset configures the file receiver and the container
log parser, and whose `kubernetesAttributes` preset adds pod, namespace, and workload metadata:

```yaml
mode: daemonset

image:
  repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-k8s

presets:
  logsCollection:
    enabled: true
    includeCollectorLogs: false
    storeCheckpoints: true
  kubernetesAttributes:
    enabled: true

extraVolumes:
  - name: netdata-ca
    secret:
      secretName: netdata-ca
extraVolumeMounts:
  - name: netdata-ca
    mountPath: /etc/otelcol/netdata-ca.pem
    subPath: ca.pem
    readOnly: true

config:
  processors:
    k8sattributes:
      extract:
        metadata:
          - k8s.namespace.name
          - k8s.pod.name
          - k8s.container.name
          - k8s.deployment.name
          - k8s.node.name
          - service.namespace
          - service.name
  exporters:
    otlp_grpc/netdata:
      endpoint: "logs.example.com:4317"
      tls:
        ca_file: /etc/otelcol/netdata-ca.pem
      headers:
        X-Scope-OrgID: kubernetes
      sending_queue:
        storage: file_storage
  service:
    pipelines:
      logs:
        exporters: [otlp_grpc/netdata]
```

```bash
kubectl create namespace otel-logs
kubectl -n otel-logs create secret generic netdata-ca --from-file=ca.pem=/path/to/netdata-ca.pem
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm install otel-logs open-telemetry/opentelemetry-collector -n otel-logs -f values.yaml
```

The `k8sattributes` processor derives `service.namespace` and `service.name` from the pod's labels and annotations
following the OpenTelemetry semantic conventions — from the `app.kubernetes.io` labels and
`resource.opentelemetry.io` annotations, falling back to the workload name and the Kubernetes namespace — so each
workload appears as its own service in the Logs tab; the
container parser adds the pod, namespace, and container from the file path.

`storeCheckpoints` keeps the file receiver's offsets in `/var/lib/otelcol` on the node, so a restarted Collector
resumes where it stopped instead of starting at the end of each file (the preset's `start_at: end`). It has two
consequences. The chart runs the Collector as root (`runAsUser: 0`) to write that host directory; to run as a
non-root user instead, set `securityContext` yourself, which the chart then leaves untouched, and make
`/var/lib/otelcol` writable by that user on every node. And checkpoints cover reading only: records already read but
still in the exporter's in-memory queue are lost if the Collector is killed or crashes before the queue drains. The
`sending_queue.storage` line above keeps that queue on the same `file_storage` extension the preset registers, so
queued records survive that too.

The values above are validated by rendering them with the `opentelemetry-collector` Helm chart 0.172.0 and
validating the resulting Collector configuration with Contrib 0.157.0. The chart deploys its own Collector build
(chart 0.172.0 ships Collector 0.159.0); pin `image.tag` to run a specific Collector version.

The `kubernetes` tenant the exporter selects needs its own retention policy on the receiving Agent, or its records
land under the 7-day, 1GB defaults:

```yaml
auth:
  enabled: true
logs:
  retention:
    kubernetes:
      max_total_size: "50GB"
      max_age: "30 days"
```

See [Log Storage and Retention](/docs/logs/log-storage-and-retention.md) for how tenant retention and offloading work.

Without Helm, this is the receiver the preset generates, abridged — the rendered configuration also carries the
collector's own log exclusions and the parser's `max_log_size`. It is not a standalone recipe; pair it with an
exporter, a storage extension for the checkpoints, and a logs pipeline as in the recipes above:

```yaml
receivers:
  file_log:
    include: [/var/log/pods/*/*/*.log]
    start_at: end
    include_file_path: true
    include_file_name: false
    retry_on_failure:
      enabled: true
    storage: file_storage  # under Helm, the storeCheckpoints preset provides this extension
    operators:
      - type: container
        id: container-parser
```

Mount `/var/log/pods` and `/var/lib/otelcol` from the node into the Collector pod, and grant the service account the
read permissions the `k8sattributes` processor needs (pods, namespaces, and replica sets across the cluster). See the
upstream [opentelemetry-collector Helm chart documentation](https://github.com/open-telemetry/opentelemetry-helm-charts/tree/main/charts/opentelemetry-collector)
and the [k8sattributes processor documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/k8sattributesprocessor)
for every preset and association option.

## macOS unified log

macOS has no OS-native log forwarding. The Collector Contrib `macos_unified_logging` receiver, at alpha stability,
runs the macOS `log` command and streams the unified log:

```yaml
receivers:
  macos_unified_logging:
    max_log_age: 1h
    max_poll_interval: 30s

processors:
  resource/macos:
    attributes:
      - key: service.namespace
        value: macos
        action: upsert
      - key: service.name
        value: unified-log
        action: upsert

service:
  pipelines:
    logs:
      receivers: [macos_unified_logging]
      processors: [resource/macos]
      exporters: [otlp_grpc/netdata]
```

`max_log_age` bounds how far back the receiver reads on its first start. Run the Collector as a `launchd` service
with permission to read system logs. The receiver builds only on macOS, so validate this configuration on a Mac. See
the upstream
[macOS unified logging receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/macosunifiedloggingreceiver)
for archive mode, predicates, and the complete option set.

## Syslog

The Collector Contrib `syslog` receiver can listen over TCP or UDP and parse RFC 3164 or RFC 5424 records. Netdata maintains a separate end-to-end guide because durable network ingestion also requires listener exposure, firewall rules, transport choices, and persistent Collector queues. Follow the dedicated [OpenTelemetry Collector syslog setup](/docs/npm/syslog/otel-collector.md) instead of duplicating that configuration here.

## Verify and troubleshoot logs

- In Netdata, open the node's Logs tab, select `otel-logs`, then choose the configured service with the **Services** selector. The stored service field is `resource.attributes.service.name`.
- If journald reports permission errors, verify access by running `journalctl` as the Collector service account, then restart the Collector after changing group membership.
- If existing file lines are absent, check `start_at` and whether a saved cursor or offset already exists. Do not delete storage state casually; doing so can replay data.
- If the Collector reports successful export but records are absent, inspect their timestamps. Netdata accepts records from up to 24 hours in the past through 10 minutes in the future and reports rejected records through OTLP `partial_success`.
- If records are duplicated after restart, enable a `file_storage` extension and reference it from the receiver. Confirm that its directory survives service and container restarts.
- On Windows, if a channel is missing, check that the Collector service account can read it; the Security channel needs explicit permission.
- On Kubernetes, if pods show no logs, check the DaemonSet pods with `kubectl logs` for file permission errors on `/var/log/pods` and for RBAC errors from the `k8sattributes` processor.
- On macOS, if the receiver reports permission errors, run the Collector with an account allowed to read system logs with the `log` command.
