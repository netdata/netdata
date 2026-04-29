# Logs tab

The Logs tab is using the [`systemd` journal plugin](/src/collectors/systemd-journal.plugin/README.md), to present a structured view into your infrastructure's `systemd` logs.

We have a thorough section explaining how you can [work with logs](https://learn.netdata.cloud/docs/logs), detailing how the plugin works, and what other utilities are used under the hood to provide you with the visualizations and the log entries.

The [`systemd` journal plugin](/src/collectors/systemd-journal.plugin/README.md) documentation has information about:

- [Key features the plugin provides](/src/collectors/systemd-journal.plugin/README.md#key-features)
- [Journal sources](/src/collectors/systemd-journal.plugin/README.md#journal-sources)
- [Journal fields](/src/collectors/systemd-journal.plugin/README.md#journal-fields)
- [Full-text search](/src/collectors/systemd-journal.plugin/README.md#full-text-search)
- [Query performance](/src/collectors/systemd-journal.plugin/README.md#query-performance)
- [Performance at scale](/src/collectors/systemd-journal.plugin/README.md#performance-at-scale)

We recommend you to read through that document, to better understand how the plugin and the visualizations work.

## Log sources

The Logs tab displays log entries from the following sources, depending on the Node's operating system:

- **systemd-journal** — reads logs from `systemd` journald on Linux Nodes. This includes system service logs and any custom logs piped into the journal using tools like [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md). See the [Systemd Journal Plugin Reference](/src/collectors/systemd-journal.plugin/README.md) for details on journal sources, fields, and query performance.
- **otel-logs** — displays logs received via OpenTelemetry (OTLP) log ingestion. See the [OpenTelemetry Signal Viewer plugin](/src/crates/netdata-log-viewer/otel-signal-viewer-plugin/README.md) for setup and configuration.
- **Windows Event Logs** — reads Windows event logs on Windows Nodes. See the [Windows Events Plugin Reference](/src/collectors/windows-events.plugin/README.md) for supported event channels and configuration.

:::tip

Custom application logs, such as web server access logs, can be displayed under the systemd-journal source by piping them through [log2journal](/src/collectors/log2journal/README.md) into `systemd` journald. See the [log centralization points guide](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md) for structured log pipeline setup and [Monitor Nginx or Apache web server log files](/docs/developer-and-contributor-corner/collect-apache-nginx-web-logs.md) for web log collector configuration.

:::
