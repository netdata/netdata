# Logs tab

The Logs tab provides a structured, searchable view of logs collected from across your infrastructure, supporting multiple log sources depending on the Node's operating system.

## Log sources

The Logs tab displays log entries from the following sources:

- **systemd-journal** — reads logs from `systemd` journald on Linux Nodes. See the [Systemd Journal Plugin Reference](/src/collectors/systemd-journal.plugin/README.md) for details on journal sources, fields, and query performance.
- **otel-logs** — displays logs received via OpenTelemetry (OTLP) log ingestion. See the [OpenTelemetry Signal Viewer plugin](/src/crates/netdata-log-viewer/otel-signal-viewer-plugin/README.md) for setup and configuration.
- **Windows Event Logs** — reads Windows event logs on Windows Nodes. See the [Windows Events Plugin Reference](/src/collectors/windows-events.plugin/README.md) for supported event channels and configuration.

You can also display custom application logs, such as web server access logs, under the systemd-journal source by piping them into `systemd` journald using [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md). For example, use the built-in `nginx-combined` log2journal configuration to pipe nginx access logs.

:::tip

For comprehensive documentation on log centralization and configuration, see [Working with Logs](https://learn.netdata.cloud/docs/logs). To keep custom log pipelines running persistently, create a systemd service unit and use `LogNamespace` to isolate piped logs from system journal entries. See the [log centralization points guide](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md) and [Monitor Nginx or Apache web server log files](/docs/developer-and-contributor-corner/collect-apache-nginx-web-logs.md) for setup details.

:::

## systemd journal plugin reference

The systemd journal plugin is the primary log source for Linux systems. The [`systemd` journal plugin](/src/collectors/systemd-journal.plugin/README.md) documentation covers:

- [Key features the plugin provides](/src/collectors/systemd-journal.plugin/README.md#key-features)
- [Journal sources](/src/collectors/systemd-journal.plugin/README.md#journal-sources)
- [Journal fields](/src/collectors/systemd-journal.plugin/README.md#journal-fields)
- [Full-text search](/src/collectors/systemd-journal.plugin/README.md#full-text-search)
- [Query performance](/src/collectors/systemd-journal.plugin/README.md#query-performance)
- [Performance at scale](/src/collectors/systemd-journal.plugin/README.md#performance-at-scale)

We recommend reading through that document to better understand how the plugin and the visualizations work.

## Accessing logs via the API

Netdata does not have a dedicated `/api/v1/logs` endpoint. Logs are retrieved through the [Functions API](/src/web/api/netdata-swagger.yaml) — the same mechanism used for processes, network connections, and other live data queries.

### Endpoint paths

You can query log functions through two paths:

- **Agent direct**: `POST http://localhost:19999/api/v3/function?function=<log-function>`
- **Netdata Cloud proxied**: `POST https://app.netdata.cloud/api/v2/nodes/{nodeId}/function?function=<log-function>`

Replace `<log-function>` with one of the available log functions:

| Function | Source | Platform |
|---|---|---|
| `systemd-journal` | systemd journal namespaces | Linux |
| `windows-events` | Windows event log channels | Windows |
| `otel-logs` | OpenTelemetry (OTLP) log ingestion | Any |

### Discovery

Call any log function with `{"info": true}` to list the accepted parameters, available log sources, and field definitions for the target Node:

```bash
curl -sS -X POST \
  'http://localhost:19999/api/v3/function?function=systemd-journal' \
  -H 'Content-Type: application/json' \
  -d '{"info": true}'
```

### Example: fetch recent logs

The following example fetches the last 50 journal entries from the past hour via the agent-direct endpoint:

```bash
curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  'http://localhost:19999/api/v3/function?function=systemd-journal' \
  -d '{"after":-3600,"before":0,"last":50,"direction":"backward"}'
```

### Key body parameters

| Parameter | Type | Description |
|---|---|---|
| `after` | int | Lower time bound in seconds. Negative values are relative to `before` |
| `before` | int | Upper time bound in seconds. `0` means now |
| `last` | int | Number of rows to return (default 200) |
| `direction` | string | `backward` (newest first, default) or `forward` |
| `query` | string | Free-text search across log fields |
| `facets` | string[] | Field names to group by (returns value counts) |
| `__logs_sources` | string | Source selector (e.g. `all-local-logs`, a specific namespace) |

:::note

Use `{"info": true}` to discover the full parameter set for your Agent version. The parameter list evolves across releases.

:::

For authentication details, see [Bearer Token Protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md). For the complete API reference, see the [swagger spec](/src/web/api/netdata-swagger.yaml) and the [REST API documentation](/src/web/api/README.md).
