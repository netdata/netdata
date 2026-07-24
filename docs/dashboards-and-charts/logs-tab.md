# Logs tab

The Logs tab provides a structured, searchable view of logs collected from across your infrastructure, supporting multiple log sources depending on the Node's operating system.

## Log sources

The Logs tab displays log entries from the following sources:

- **systemd-journal** — reads logs from `systemd` journald on Linux Nodes. See the [Systemd Journal Plugin Reference](/src/collectors/systemd-journal.plugin/README.md) for details on journal sources, fields, and query performance.
- **otel-logs** — displays logs ingested via OpenTelemetry (OTLP/gRPC) by the [OpenTelemetry plugin](/src/crates/otel-plugin/README.md), which indexes them for fast querying in this tab.
- **Windows Event Logs** — reads Windows event logs on Windows Nodes. See the [Windows Events Plugin Reference](/src/collectors/windows-events.plugin/README.md) for supported event channels and configuration.
- **macOS Unified Logs** — reads native macOS unified logs through Apple's OSLog framework. See the [macOS Logs Plugin Reference](/src/collectors/macos-logs.plugin/README.md) for supported fields and query behavior.

:::note

On Linux systems without systemd (such as Alpine Linux), the systemd-journal source is unavailable. You can forward logs with `systemd-cat-native --url` to a remote `systemd-journal-remote` on a system running a Netdata Agent, or use OTLP log ingestion. See [Working with Logs](/docs/category-overview-pages/working-with-logs.md) for details.

:::

You can also display custom application logs, such as web server access logs, under the systemd-journal source by piping them into `systemd` journald using [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md). For example, use the built-in `nginx-combined` log2journal configuration to pipe nginx access logs.

:::tip

For an overview of log collection and local or remote storage options, see [Working with Logs](/docs/category-overview-pages/working-with-logs.md). To keep custom log pipelines running persistently, follow the [`log2journal` service setup](/src/collectors/log2journal/README.md#best-practices). See the [log centralization points guide](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md) and [Monitor Nginx or Apache web server log files](/docs/developer-and-contributor-corner/collect-apache-nginx-web-logs.md) for related setup details.

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
