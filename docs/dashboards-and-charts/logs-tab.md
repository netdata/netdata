# Managing Logs

Netdata manages logs across your infrastructure — ingestion, storage, indexing, querying, and visualization — and the Logs tab is where you work with them. Every log source is managed and queried the same way, so the workflow you learn on one operating system or pipeline applies to all of them.

This page covers how logs are organized (sources), how they are queried and explored, and how queries behave at scale. For the deployment architecture — where logs can live and how to centralize them — see [Logs Management](/docs/category-overview-pages/working-with-logs.md).

## Log sources

The Logs tab displays entries from the following sources, depending on the node's operating system and configuration:

| Source | Platform | What it reads | Reference |
|:-------|:---------|:--------------|:----------|
| `systemd-journal` | Linux | systemd journal files — system, user, namespace, and remote journals, merged by default | [Systemd Journal Plugin Reference](/src/collectors/systemd-journal.plugin/README.md) |
| `windows-events` | Windows | Windows event channels, auto-detected and aggregated | [Windows Events Plugin Reference](/src/collectors/windows-events.plugin/README.md) |
| `macos-logs` | macOS | The native macOS unified log store through Apple's OSLog framework | [macOS Logs Plugin Reference](/src/collectors/macos-logs.plugin/README.md) |
| `otel-logs` | Any, via OTLP | Logs ingested by the OpenTelemetry plugin and stored in Netdata's indexed log store | [OpenTelemetry Logs](/integrations/logs/integrations/opentelemetry_logs.md) and the [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md) |

:::note

On Linux systems without systemd (such as Alpine Linux), the `systemd-journal` source is unavailable. You can still make logs explorable by forwarding them to a remote `systemd-journal-remote` with [`systemd-cat-native --url`](/src/libnetdata/log/systemd-cat-native.md), or by using [OpenTelemetry log ingestion](/docs/opentelemetry/otlp-ingestion.md). See [Logs Management](/docs/category-overview-pages/working-with-logs.md) for the deployment options.

:::

Custom application logs from text files can appear under the journal or OpenTelemetry sources — see [Text Files to Journals](/docs/logs/text-files-to-journals.md).

## Querying and exploring logs

### Sources and services

Use the **Sources** selector on the right sidebar to choose what to query. The available values depend on the source:

- `systemd-journal` offers the detected journal namespaces: `system`, `user`, any `LogNamespace=` namespaces, and `remote` journals received from other machines. On a journal centralization server, each sending machine's logs are part of the view.
- `windows-events` offers the detected event channels, aggregated by default.
- `macos-logs` exposes the local unified log store.
- `otel-logs` offers a **Services** selector driven by the `service.namespace` and `service.name` resource attributes of the ingested streams.

All sources are merged into one view by default. Selecting a specific source before a deep analysis improves query performance.

### Field filters with live counters

Every field of every log entry is indexed. Select fields are offered as filters — the facet panel — each with a real-time counter of matching entries:

- Multi-select values within a field (OR) and across fields (AND).
- Toggle values between inclusion and exclusion.
- Filter selections persist across page reloads.

Field allowlists and blocklists protect query performance. A "full data queries" mode enables negative and empty matches at the cost of slower queries.

### Full-text search

Full-text search applies across all fields of every entry:

| Pattern | Matches |
|:--------|:--------|
| `error` | Entries containing `error` in any field (default: substring match) |
| `a*b` | Wildcard match — `acb`, `a_long_b`, ... |
| `error\|warning` | Entries containing either pattern (OR) |
| `!systemd` | Entries that do not contain the pattern (negation) |

Combine full-text search with field filters for precise results.

### Table and entry details

- Scrollable, time-ordered entries (newest first by default) with pagination for large datasets.
- Customizable columns — use the ⚙️ icon to select which fields the table displays.
- Severity color coding for quick identification of errors and warnings.
- Click any entry to open the info panel with every field of that entry, raw and enriched values, copyable text, and one-click filtering on any field value.

Enrichment improves readability — priorities, facilities, UIDs/GIDs, errno values, capabilities, and well-known message IDs are displayed in human-readable form. Enrichments are visual only and are not searchable.

### Histograms

Histograms visualize log frequency per field value over time, color-coded by value:

- Zoom, pan, and click-to-navigate.
- Click a bar segment to filter the view to that field value.
- Time-correlated with the main timeline.

### PLAY mode — live tail

Click ▶️ to stream newly received entries continuously, on single nodes and centralization servers alike — the visual equivalent of `journalctl -f`, for every source.

### Sampling at scale

To stay responsive on very large datasets, the query engine samples:

1. It fully evaluates the newest entries, up to a budget of 1,000,000 entries per query.
2. Beyond the budget, entries are marked `[unsampled]` and counters become `[estimated]`.
3. Where sequence numbers are available, estimates use them for precision.

Because the evaluation budget is large, value distributions stay tight even on multi-million-entry datasets — for example, a 60% share in a 10M-entry window is estimated within roughly ±0.9% (95% confidence). See [Performance at scale](/src/collectors/systemd-journal.plugin/README.md#performance-at-scale) for the math and [Query performance](/src/collectors/systemd-journal.plugin/README.md#query-performance) for the factors that matter.

### Performance tips

- Keep the visible timeframe short and apply filters early.
- Select a specific source instead of querying across all sources.
- Limit the number of displayed rows.
- On busy centralization servers, prefer SSD/NVMe storage and compressed filesystems for journal files; the OS page cache makes repeated queries faster.

## Prerequisites

- A free Netdata Cloud account is required to use the log functions. When you access an Agent directly, log content flows from the Agent to your browser and is not stored in Netdata Cloud.
- Platform notes — package availability, container requirements, and build flags — are documented in each plugin's reference: [systemd journal](/src/collectors/systemd-journal.plugin/README.md#prerequisites), [Windows events](/src/collectors/windows-events.plugin/README.md#prerequisites), [macOS logs](/src/collectors/macos-logs.plugin/README.md#prerequisites), [OpenTelemetry](/src/crates/otel-plugin/README.md).

## Query logs programmatically

The same function API that powers the Logs tab is used by Netdata AI assistants and MCP clients to query log sources across your infrastructure. See [Netdata AI](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md).

## Where to next

- [Logs Management](/docs/category-overview-pages/working-with-logs.md) — deployment architecture: where logs live, storage choices, centralization options, and current limitations.
- [Text Files to Journals](/docs/logs/text-files-to-journals.md) — bring application log files into these sources.
- [Logs Centralization Points](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md) — aggregate logs from many machines.
