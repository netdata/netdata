# Centralizing Logs with OpenTelemetry

You centralize a log source by running an OpenTelemetry Collector where the logs are produced and pointing it at the
OTLP endpoint of a Netdata Agent. That Agent stores the logs in Netdata's log store, with retention per tenant and
optional offloading to object storage, and shows them in its Logs tab under the `otel-logs` source. Centralize the
sources that must outlive their node or that have no OS log store; leave the rest managed in place, where they cost
nothing extra. See [Logs Management](/docs/category-overview-pages/working-with-logs.md) for the decision per source.

## Set up the receiving Agent

Any Netdata Agent with the OpenTelemetry plugin can receive logs; official Linux and macOS builds include it. Use one
receiving Agent per team, environment, or datacenter, or one shared Agent with a tenant per sender group: every
tenant has its own retention policy, and senders select their tenant with the `X-Scope-OrgID` header. Size the disk
for the retention you want, as described in [Log Storage and Retention](/docs/logs/log-storage-and-retention.md).

The plugin listens on `127.0.0.1:4317` by default. To accept remote senders, edit `otel.yaml` with
[`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files):

```yaml
endpoint:
  path: "0.0.0.0:4317"
  tls_cert_path: /etc/netdata/ssl/server-cert.pem
  tls_key_path: /etc/netdata/ssl/server-key.pem
  # Optional: require client certificates (mutual TLS).
  tls_ca_cert_path: /etc/netdata/ssl/client-ca.pem
auth:
  enabled: true
```

Restart the Agent afterwards. Do not expose a plaintext listener beyond loopback, and restrict port 4317 with network
access controls. `auth.enabled` makes the tenant header mandatory; it selects the tenant and does not authenticate the
sender, so rely on TLS, mutual TLS, and network controls for that. The receiving Agent must be connected to Netdata
Cloud for the logs to be viewable. All options are in the
[OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md).

## Configure the senders

Every recipe below uses the same exporter and the same persistent queue, so a Collector restart or a network outage
does not lose records:

```yaml
extensions:
  file_storage:
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
      storage: file_storage

service:
  extensions: [file_storage]
```

Set `service.namespace` and `service.name` as resource attributes on every stream. They populate the **Services**
selector of the Logs tab, which is how you pick a stream to query. Use the `otlp_grpc` exporter and port 4317; the
Agent does not accept OTLP/HTTP.

The recipes are validated with OpenTelemetry Collector Contrib 0.157.0 and, for Kubernetes, the
`opentelemetry-collector` Helm chart 0.172.0. Older Collector releases may name components differently; check the
release's component list before copying a recipe.

## Linux: systemd journal

The `journald` receiver runs `journalctl` on the node, so the Collector's service account needs read access to the
journal. To centralize everything the node writes, do not filter by unit or priority:

```yaml
receivers:
  journald/netdata:
    priority: debug
    start_at: end
    storage: file_storage

processors:
  resource/journal:
    attributes:
      - key: service.namespace
        value: linux
        action: upsert
      - key: service.name
        value: systemd-journal
        action: upsert

service:
  pipelines:
    logs:
      receivers: [journald/netdata]
      processors: [resource/journal]
      exporters: [otlp_grpc/netdata]
```

Permissions, unit and priority filters, namespaces, and the option table are in
[Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md#systemd-journal). The journal
stays on the node as well, managed in place; centralizing adds a copy in Netdata's store, nothing else changes.

## Windows event channels

Install the Collector as a Windows service and use one `windowseventlog` receiver per channel. The service account
needs read access to each channel; the Security channel in particular requires it explicitly.

```yaml
extensions:
  file_storage:
    directory: C:\ProgramData\otelcol\netdata
    create_directory: true

receivers:
  windowseventlog/application:
    channel: Application
    start_at: end
    storage: file_storage
  windowseventlog/system:
    channel: System
    start_at: end
    storage: file_storage
  windowseventlog/security:
    channel: Security
    start_at: end
    storage: file_storage

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
  extensions: [file_storage]
  pipelines:
    logs:
      receivers: [windowseventlog/application, windowseventlog/system, windowseventlog/security]
      processors: [resource/windows]
      exporters: [otlp_grpc/netdata]
```

On a Windows Event Collector, add a receiver for the `ForwardedEvents` channel to centralize what the forwarders send.
The `storage` extension keeps a bookmark per channel, so a restart resumes where it stopped. Event bodies are
structured records by default; set `raw: true` to keep the original XML instead.

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
following the OpenTelemetry semantic conventions, so each workload appears as its own service in the Logs tab; the
container parser adds the pod, namespace, and container from the file path. `storeCheckpoints` keeps file offsets in
`/var/lib/otelcol` on the node so a Collector restart does not replay or skip lines.

Without Helm, the receiver the preset generates is:

```yaml
receivers:
  filelog:
    include: [/var/log/pods/*/*/*.log]
    start_at: end
    include_file_path: true
    include_file_name: false
    retry_on_failure:
      enabled: true
    storage: file_storage
    operators:
      - type: container
        id: container-parser
```

Mount `/var/log/pods` and `/var/lib/otelcol` from the node into the Collector pod, and grant the service account the
read permissions the `k8sattributes` processor needs (pods, namespaces, and replica sets across the cluster).

## macOS

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
with permission to read system logs.

## Application text files

Ship files with the `file_log` receiver, with glob includes and excludes, multiline joining, JSON parsing, and
persistent offsets; the recipes are in
[Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md#application-log-files). To keep
the files queryable with `journalctl` and readable by a SIEM instead, convert them into journal entries as described
in [Text Files to Journals](/docs/logs/text-files-to-journals.md).

## Network devices

Point the devices at a Collector `syslog` receiver that forwards to Netdata; see
[Syslog from Network Devices](/docs/npm/syslog/README.md). SNMP traps and network flows need no Collector: Netdata
receives them directly and stores them as journal-compatible files on the receiving node; see
[SNMP Traps](/docs/npm/snmp-traps/README.md) and [Network Flows](/docs/npm/network-flows/README.md).

## Verify

Open the receiving Agent's dashboard, select the Logs tab and the `otel-logs` source, and pick the stream with the
**Services** selector. If records are missing, work through
[Troubleshoot the pipeline](/docs/opentelemetry/otlp-ingestion.md#troubleshoot-the-pipeline): the most common
causes are an OTLP/HTTP exporter, a TLS mismatch, and timestamps outside the acceptance window (24 hours in the past
to 10 minutes in the future). Validate any Collector configuration before deploying it with
`otelcol-contrib validate --config <file>`.
