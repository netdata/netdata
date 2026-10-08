# Managing Logs

With Netdata you query the logs of any node or centralization point from the Logs tab of its dashboard: filter by
field, search the full text, see how log volume changes over time, follow new entries live, and read an event next to
the per-second metrics of the same node. Every log source works the same way, so the workflow you learn on one
operating system applies to all of them. For where logs are stored and how to centralize them, see
[Logs Management](/docs/category-overview-pages/working-with-logs.md).

## Log sources

The Logs tab shows entries from the sources available on the node:

| Source | Platform | What it reads | Reference |
|:-------|:---------|:--------------|:----------|
| `systemd-journal` | Linux | systemd journal files: system, user, namespace, and remote journals, merged by default | [Systemd Journal Plugin Reference](/src/collectors/systemd-journal.plugin/README.md) |
| `windows-events` | Windows | Windows event channels, auto-detected and aggregated, including forwarded-events channels on a Windows Event Collector | [Windows Events Plugin Reference](/src/collectors/windows-events.plugin/README.md) |
| `macos-logs` | macOS | The native unified log store, through Apple's OSLog framework | [macOS Logs Plugin Reference](/src/collectors/macos-logs.plugin/README.md) |
| `otel-logs` | Any, via OTLP | Logs received by the OpenTelemetry plugin and stored in Netdata's indexed log store | [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md) |
| `snmp:traps` | Any | Journal-compatible files Netdata writes for the traps received by its SNMP traps collector | [SNMP Trap Logs](/docs/logs/snmp-trap-logs.md) |

Application text files appear under `systemd-journal` when converted with log2journal, or under `otel-logs` when
shipped through an OpenTelemetry Collector; see [Text Files to Journals](/docs/logs/text-files-to-journals.md).
Network flows are stored the same way as SNMP traps but have their own view,
[Network Flows](/docs/logs/network-flows.md), instead of the Logs tab.

:::note

On Linux systems without systemd, such as Alpine Linux, the `systemd-journal` source is unavailable. Convert and push
logs to a remote `systemd-journal-remote` with [`systemd-cat-native --url`](/src/libnetdata/log/systemd-cat-native.md),
or send them through [OpenTelemetry](/docs/opentelemetry/otlp-ingestion.md).

:::

## Selecting what to query

The **Sources** selector on the right sidebar narrows a query to part of a source:

- `systemd-journal` offers the detected journals: `system`, `user`, each `LogNamespace=` namespace, and the `remote`
  journals received from other machines. On a journal centralization point, each sending machine is a selectable
  source.
- `windows-events` offers the detected event channels.
- `macos-logs` offers the unified log store.
- `otel-logs` offers a **Services** selector built from the `service.namespace` and `service.name` resource attributes
  of the ingested streams.

Each source merges its own sub-sources by default — journals within `systemd-journal`, channels within
`windows-events`; different sources are separate queries. Narrowing the selection before a deep analysis makes
queries faster.

## Filtering by field

The filter panel offers a set of fields, each with a live counter of matching entries per value:

- Select several values of one field (any of them) and several fields at once (all of them).
- Toggle a value between inclusion and exclusion.
- Selections persist across page reloads.

Each source offers a default set of fields as filters, and you can enable any other field of your logs as a filter or
histogram dimension from the panel — the query then evaluates it like any default field. Message text is covered by
full-text search rather than filters. For OpenTelemetry logs, fields with very many distinct values stay searchable
and filterable but cannot drive facet counters or histograms. A "full data queries" mode enables negative and empty
matches at the cost of slower queries.

## Full-text search

Full-text search matches against every field of every entry:

| Pattern | Matches |
|:--------|:--------|
| `error` | Entries containing `error` in any field (substring match) |
| `a*b` | Wildcard match: `acb`, `a_long_b`, ... |
| `error\|warning` | Entries containing either pattern |
| `!systemd` | Entries that do not contain the pattern |

Full-text search is the most expensive query type. Combine it with a short time range and field filters.

## Reading entries

- Entries are listed newest first, with pagination for large results.
- Choose the columns with the ⚙️ icon.
- Severity is color-coded.
- Click an entry to open its details: every field, raw and enriched values, copyable text, and one-click filtering on
  any field value.

Enrichment renders priorities, facilities, UIDs and GIDs, errno values, capabilities, and well-known message IDs in
human-readable form. Enrichments are visual only and are not searchable.

## Histograms

The histogram shows log volume over time per value of a selected field, color-coded by value. Zoom and pan follow the
main timeline; clicking a bar segment filters the view to that value.

## Live tail

Click ▶️ (PLAY) to stream new entries as they arrive, on single nodes and centralization points alike. Live tail works
for every source.

## Query behavior at scale

The OS-native sources and the OpenTelemetry log store use different query engines.

### systemd journal

- Netdata reads the journal files directly, one file at a time — with cached file metadata lookups on builds that use
  `libsystemd` — so a query costs no more than the equivalent `journalctl` query on the same files.
- Each query fully evaluates the newest 500,000 entries, distributes the rest of its 1,000,000-entry budget across
  the queried files, and samples beyond it. Beyond the budget:
  - the rows shown are always real entries;
  - the histogram shows the remaining volume as `[unsampled]` and `[estimated]`;
  - the filter counters stop counting beyond the budget, they are not extrapolated.
- Where journal sequence numbers are available, estimates use them for precision. See
  [Performance at scale](/src/collectors/systemd-journal.plugin/README.md#performance-at-scale) for the math.

### Windows events and macOS logs

- Both plugins evaluate every entry in the queried timeframe — no sampling is applied yet — so on very busy systems
  narrow the timeframe and the selected channels before broad queries.

### OpenTelemetry log store

- Counts are exact; no sampling is applied.
- The time range bounds the work of a query: the default window is the last 15 minutes. The store itself applies no
  entry budget, but the Agent cancels the function after its timeout (currently 10 seconds for `otel-logs`), so on a
  large store narrow the window before adding a full-text search.
- Files offloaded to object storage are fetched transparently when a query needs them; the first query over archived
  data waits for the download.
- Fields with very many distinct values are searchable and filterable but are not offered as filter facets.

### Query tips

- Keep the time range short and apply filters early.
- Select a specific source instead of querying all of them.
- Limit the number of displayed rows.
- On busy centralization points, use SSD or NVMe storage for the log files; the OS page cache makes repeated queries
  faster.

## Prerequisites

- Sign in with Netdata Cloud, which is free for community use. Log content is transmitted encrypted to your browser
  and is not stored in Netdata Cloud; for full data sovereignty, run
  [Netdata Cloud On-Prem](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/README.md).
- Platform requirements, packages, and container settings are documented in each plugin's reference:
  [systemd journal](/src/collectors/systemd-journal.plugin/README.md#prerequisites),
  [Windows events](/src/collectors/windows-events.plugin/README.md#prerequisites),
  [macOS logs](/src/collectors/macos-logs.plugin/README.md#prerequisites),
  [OpenTelemetry](/src/crates/otel-plugin/README.md).

## Querying logs programmatically

The Logs tab is built on the same function API that Netdata AI assistants and MCP clients use to query logs across
your infrastructure. See [Netdata AI](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md).

## Where to next

- [Logs Management](/docs/category-overview-pages/working-with-logs.md) — where logs are stored, how to decide per
  source, and the current limitations.
- [Text Files to Journals](/docs/logs/text-files-to-journals.md) — bring application log files into these sources.
- [Logs Centralization Points](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md)
  — Netdata on journal aggregation points you already run.
