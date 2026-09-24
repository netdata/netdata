# Advanced OpenTelemetry pipelines with Netdata

The previous guides covered collecting
[metrics](/docs/collecting-metrics-with-opentelemetry.md) and
[logs](/docs/collecting-logs-with-opentelemetry.md) individually. This guide
covers advanced pipeline patterns: combining metrics and logs in a single
Collector, converting logs into metrics, parsing unstructured logs into
structured attributes, and creating Netdata alerts on the derived data.

## Collect both metrics and logs

The Netdata OTel plugin accepts both metrics and logs on the same gRPC endpoint
(`localhost:4317`). You can define separate pipelines for each signal type in a
single Collector configuration file:

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
dashboard and logs appear in the Logs tab. A single OTLP exporter handles both.

## Convert logs to metrics

Counting log entries by severity or content is a common observability pattern.
Instead of searching logs after an incident, you can track error rates as a
metric and alert on them in real time.

The OpenTelemetry Collector supports this through **connectors** — components
that sit between two pipelines. The `count` connector consumes logs from one
pipeline and emits count metrics into another.

### How connectors work

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

### Example: Count warning-level journal entries

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
| `processors.transform` | Sets the metric unit to `{message}` so Netdata displays it correctly. |
| `service.pipelines.logs` | Sends logs to both the `count` connector and the OTLP exporter. |
| `service.pipelines.metrics` | Receives counts from the connector, transforms them, and sends them to Netdata. |

> **NOTE**
>
> The `count` connector appears as an **exporter** in the logs pipeline and as a
> **receiver** in the metrics pipeline. This is how connectors bridge two
> pipelines.

### Customize the counting conditions

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

### What to expect

After starting the Collector with this configuration, a new chart appears in the
Netdata dashboard under the **OpenTelemetry** section. The chart shows the
count of matching log entries over time. You can use this chart as the basis
for a Netdata health alert.

## Parse unstructured logs into structured attributes

Log entries often arrive as plain-text strings with no structured fields. The
`transform` processor can parse these entries into structured attributes using
OTTL expressions, making them easier to filter, search, and process downstream.

### Example: Parse JSON log lines

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

### Full pipeline with JSON parsing

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

### Other common transformations

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

## Create a Netdata alert on a derived metric

Once the log-derived metric appears as a chart in Netdata, you can configure a
health alert on it — for example, to trigger a notification when warning-level
log entries spike. The chart name to use in the alert's `on` field is visible
in the Netdata dashboard under the **OpenTelemetry** section.

For instructions on creating and configuring health alerts, see the
[health alerts reference](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md).

## Troubleshooting

### The count metric does not appear

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

- [Collecting metrics with OpenTelemetry](/docs/collecting-metrics-with-opentelemetry.md)
- [Collecting logs with OpenTelemetry](/docs/collecting-logs-with-opentelemetry.md)
- [Netdata OTel plugin reference](/src/crates/netdata-otel/otel-plugin/integrations/opentelemetry.md)
- [Netdata health alerts configuration](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md)
- [Count connector](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/countconnector)
- [Transform processor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/transformprocessor)
- [OTTL (OpenTelemetry Transformation Language)](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl)
