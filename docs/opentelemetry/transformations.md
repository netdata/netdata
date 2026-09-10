# Transform and Filter OpenTelemetry Logs

Use Collector processors when several receivers need the same normalization, enrichment, or filtering before they export to Netdata. Prefer a receiver's built-in parser for source-specific work; use the `transform` processor for reusable OTTL changes and the `filter` processor to drop records.

Before you begin, complete [Ingest OpenTelemetry Metrics and Logs](/docs/opentelemetry/otlp-ingestion.md). These examples use the `transform` and `filter` processors from OpenTelemetry Collector Contrib `0.157.0`.

## Parse a JSON log body

When the log body is a JSON string, parse it and merge its keys into the log attributes:

```yaml
processors:
  transform/parse_json:
    error_mode: ignore
    log_statements:
      - 'merge_maps(log.attributes, ParseJSON(log.body), "insert") where IsString(log.body)'
```

For a body such as `{"level":"error","msg":"connection refused","duration_ms":312}`, the processor adds `level`, `msg`, and `duration_ms` attributes while retaining the original body. The `insert` strategy preserves an existing attribute when the parsed object has the same key. With `error_mode: ignore`, an invalid JSON record continues through the pipeline and the Collector logs the transformation error.

Use a receiver-level `json_parser` instead when every line from one file has the same format and you also need to parse its timestamp or severity during ingestion. See [Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md#parse-json-lines).

## Normalize severity and enrich resources

This processor promotes a parsed `level` attribute to severity text and adds stable resource identity used by the Netdata Logs tab:

```yaml
processors:
  transform/normalize_logs:
    error_mode: ignore
    log_statements:
      - 'set(log.severity_text, log.attributes["level"]) where log.attributes["level"] != nil'
      - 'set(resource.attributes["service.name"], "my-application") where resource.attributes["service.name"] == nil'
      - 'set(resource.attributes["service.namespace"], "production") where resource.attributes["service.namespace"] == nil'
```

The first statement preserves the source value as text; it does not infer an OpenTelemetry severity number. If conditions or dashboards depend on numeric severity, map the source's known levels explicitly instead of assuming every application uses the same scale.

Use static resource values only when a pipeline handles one known service. For shared pipelines, derive identity from trusted receiver metadata or preserve the application's existing OpenTelemetry resource attributes.

## Drop unwanted logs

Setting an attribute does not drop a record. Use the `filter` processor with `log_conditions`; a record is dropped when any condition matches:

```yaml
processors:
  filter/drop_noise:
    error_mode: ignore
    log_conditions:
      - 'IsMatch(log.body, "(?i)health.?check")'
```

Place the filter before the Netdata exporter and before any connector that should not count the dropped records:

```yaml
service:
  pipelines:
    logs:
      receivers: [file_log/my_application]
      processors: [transform/parse_json, transform/normalize_logs, filter/drop_noise]
      exporters: [otlp_grpc/netdata]
```

Filters permanently discard matching telemetry. Start with narrow conditions, inspect representative input, and test in a non-critical pipeline before deployment.

## Complete file-to-Netdata pipeline

This example tails JSON lines, parses their bodies, adds service identity when absent, drops health-check noise, and sends the remaining logs to the local Netdata Agent:

```yaml
receivers:
  file_log/my_application:
    include: [/var/log/myapp/*.json]
    start_at: end

processors:
  transform/prepare_logs:
    error_mode: ignore
    log_statements:
      - 'merge_maps(log.attributes, ParseJSON(log.body), "insert") where IsString(log.body)'
      - 'set(log.severity_text, log.attributes["level"]) where log.attributes["level"] != nil'
      - 'set(resource.attributes["service.name"], "my-application") where resource.attributes["service.name"] == nil'
  filter/drop_health_checks:
    error_mode: ignore
    log_conditions:
      - 'IsMatch(log.body, "(?i)health.?check")'

exporters:
  otlp_grpc/netdata:
    endpoint: "127.0.0.1:4317"
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [file_log/my_application]
      processors: [transform/prepare_logs, filter/drop_health_checks]
      exporters: [otlp_grpc/netdata]
```

For production file collection, add persistent offsets as described in [Logs Collection](/docs/opentelemetry/logs-collection.md#application-log-files).

## Create metrics from matching logs

Transform and filter processors stay within one signal pipeline; they do not turn logs into metrics. Use a connector to bridge the logs and metrics pipelines. The maintained [logs-to-metrics recipe](/docs/opentelemetry/logs-to-metrics.md) shows a count connector, a real filter processor, metric-unit normalization, rate semantics, verification, and alerting guidance.

## OTTL operating rules

- Use signal-specific paths such as `log.body`, `log.attributes`, and `resource.attributes` in `log_statements` and `log_conditions`.
- Use `where` on a statement to apply it conditionally. Filter conditions are different: any matching condition drops the telemetry.
- `error_mode: ignore` logs evaluation errors and continues processing. Use `propagate` only when dropping the affected payload is preferable to forwarding untransformed data.
- Processor order matters. Parse before reading parsed attributes, normalize before filtering on normalized values, and filter before connectors or exporters that should not receive discarded records.
- OTTL and processor configuration evolve. Check the documentation for the exact Collector release you deploy.

See the upstream [OTTL documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/ottl), [transform processor documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/transformprocessor), and [filter processor documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/filterprocessor) for the complete language and configuration surface.

## Troubleshoot transformations

- **The Collector rejects the configuration:** check the component identifiers and OTTL paths against your exact Collector release. Contrib components are not necessarily present in the Core distribution.
- **Parsed attributes are absent:** inspect the incoming body type and value with a temporary debug exporter. `ParseJSON` requires a JSON string and returns an error for invalid JSON.
- **Severity filters behave unexpectedly:** confirm whether the source set `severity_number`, only `severity_text`, or a custom attribute. Mapping text to numbers requires source-specific rules.
- **Too many records disappear:** remove or narrow the filter while inspecting representative records. Conditions within `log_conditions` are ORed.
- **Netdata receives logs but service selection is empty:** set `resource.attributes.service.name` before export and verify it in the `otel-logs` source.
