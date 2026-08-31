# Logs Management

Netdata is a complete logs management system: it ingests, stores, indexes, queries, and visualizes logs across your infrastructure. A defining architectural choice is that the storage layer is flexible — Netdata manages logs directly inside the native OS log stores, or inside Netdata's own high-performance indexed store when you centralize with OpenTelemetry.

## How Netdata manages logs

- **Logs managed where they are born.** On Linux, Windows, and macOS, Netdata reads the operating system's native log store and completes it with everything the store itself does not provide: full indexing of every field, faceted filtering, full-text search, histograms, live tail, and a dashboard. No forwarders to configure, no conversion, no duplicate storage — and `journalctl`, Event Viewer, the `log` command, and SIEM agents keep working on the same data.
- **Centralization when you decide, with the transport you choose.** Aggregate with the OS-native mechanisms (`systemd-journal-remote`/`systemd-journal-upload`, Windows Event Forwarding) or with OpenTelemetry. Netdata manages the aggregated logs identically on any centralization point.
- **As many centralization points as you need.** Centralize everything, centralize a slice, or stay fully distributed — per team, per datacenter, per environment, with mixed technologies side by side. Netdata Cloud brings every node and every centralization point into one dashboard with one role-based access model.
- **One way to work with every source.** All sources share the same interface in the Logs tab: field filters with live counters, full-text search, per-field histograms, and PLAY live tail. See [Managing Logs](/docs/dashboards-and-charts/logs-tab.md).

## Log sources and storage

| Source | Platform | Storage | Centralization transport | Managed as |
|:-------|:---------|:--------|:-------------------------|:-----------|
| [systemd journal](/src/collectors/systemd-journal.plugin/README.md) | Linux | Native journal files (`/var/log/journal`, `/run/log/journal`) | `systemd-journal-remote` / `systemd-journal-upload` (active or passive, optional encryption) | `systemd-journal` source in the Logs tab |
| [Windows Event Log](/src/collectors/windows-events.plugin/README.md) | Windows | Native event channels | Windows Event Forwarding (WEF) or any channel aggregation | `windows-events` source in the Logs tab |
| [macOS unified log](/src/collectors/macos-logs.plugin/README.md) | macOS | Native OSLog store | macOS has no OS-native forwarding; use OpenTelemetry below | `macos-logs` source in the Logs tab |
| [OpenTelemetry logs](/src/crates/otel-plugin/README.md) | Any, via OTLP | Netdata's indexed log store: write-ahead log, sealed index files, per-tenant retention, optional `fs`/`s3` archive with read-back cache | OTLP/gRPC is the transport | `otel-logs` source in the Logs tab |
| Text log files | Any | Plain files on disk | Convert to journal entries with [log2journal](/src/collectors/log2journal/README.md), or ship structured logs through an OpenTelemetry Collector | Via the journal or `otel-logs` source |
| [Network device syslog](/docs/npm/syslog/README.md) | Network devices | Netdata's indexed log store | Devices send syslog to an OpenTelemetry Collector, which forwards OTLP | `otel-logs` source in the Logs tab |

## Choose your deployment

| Your situation | Recommended path |
|:---------------|:-----------------|
| You want zero configuration and no pipelines | Install Netdata on each node; logs are managed in the native OS stores |
| You want a central point and you rely on OS-native tools or a SIEM | Aggregate with `systemd-journal-remote`/`systemd-journal-upload` or Windows Event Forwarding, and install Netdata on each centralization point. Journal files stay in the native format, so SIEM agents and log shippers can ingest them directly |
| You want long retention, per-tenant isolation, or object-storage archiving — or you already run an OpenTelemetry pipeline | Centralize with OTLP into Netdata's indexed log store |
| You have application text files and you use `journalctl` or a SIEM | Convert them with [log2journal](/src/collectors/log2journal/README.md); the result stays queryable by journal tooling and ingestible by SIEM agents |
| You have application text files and an existing OpenTelemetry or third-party pipeline | Ship structured logs through an [OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md) to Netdata, in parallel to your existing backend |
| Network devices send syslog | Point them at an [OpenTelemetry Collector syslog receiver](/docs/npm/syslog/otel-collector.md) that forwards to Netdata |

## What you get

- **Zero configuration on every OS.** Each native plugin auto-detects its sources: journal directories (including namespace and remote journals), Windows event channels, and the macOS unified log store.
- **Native formats are preserved.** Logs remain in the OS-native store, so your existing tooling, compliance processes, and SIEM integrations are undisturbed. Netdata manages the same data in place.
- **Performance at scale.** The query engine fully evaluates up to 1,000,000 entries per query and estimates beyond that, staying responsive on multi-gigabyte journal datasets — up to 25–30x faster than `journalctl` on multi-journal queries. See [Query performance](/src/collectors/systemd-journal.plugin/README.md#query-performance).
- **Logs next to per-second metrics.** The Logs tab sits beside the metrics of the same node, so you can correlate an event with the exact second the CPU spiked or the disk saturated — on the node itself, or on any centralization point.
- **Privacy by design.** When you explore logs, data flows directly from the Netdata Agent to your browser; log content is not stored in Netdata Cloud. A free Netdata Cloud account is used for authentication to the log functions.
- **Your own indexed store when you centralize.** The OpenTelemetry path stores logs in Netdata's indexed format with per-tenant retention and rotation, optional archiving to `s3` or filesystem remote storage, and a bounded read-back cache for querying archived data.

## Centralization at your own scale

A centralization point does not have to be infrastructure-wide. Common topologies:

- **One centralization point per environment or team**, each running Netdata, with Netdata Cloud providing a single dashboard and access control across all of them.
- **Mixed distributed and centralized nodes**: critical systems keep logs only locally; the rest forward to a shared point.
- **Mixed technologies**: Linux servers forward journals to a journal centralization point, Windows servers forward events with WEF to a Windows centralization point, and applications ship OpenTelemetry logs to Netdata's OTLP endpoint — all managed from the same Netdata Cloud space.

The setup guides live under [Systemd Journal Logs](/src/collectors/systemd-journal.plugin/README.md) in [Logs Centralization Points](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md), which also explains how Netdata Cloud unifies access across points.

## Current limitations

- **The Logs tab queries one node or centralization point at a time.** In Netdata Cloud you select which node's (or centralization point's) logs to work with; there is no merged cross-node log stream.
- **Alerts are metric-based.** Netdata does not evaluate alert expressions against log content. To alert on log patterns, derive a metric — for example with the [OpenTelemetry logs-to-metrics recipe](/docs/opentelemetry/logs-to-metrics.md) or a log-parsing collector such as `web_log` — and alert on that metric.
- **The OpenTelemetry endpoint accepts OTLP/gRPC only**, not OTLP/HTTP. Exponential histograms are not currently ingested for metrics.
- **macOS has no OS-native log forwarding.** To centralize macOS unified logs, ship them through the OpenTelemetry Collector Contrib `macosunifiedloggingreceiver` (alpha stability) to Netdata's OTLP endpoint.

## In this section

- [Managing Logs](/docs/dashboards-and-charts/logs-tab.md) — how logs are organized, stored, queried, and explored: sources, filters, full-text search, histograms, live tail, and sampling.
- [Systemd Journal Logs](/src/collectors/systemd-journal.plugin/README.md) — plugin reference, [Forward Secure Sealing](/src/collectors/systemd-journal.plugin/forward_secure_sealing.md), and the [Logs Centralization Points](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md) setup guides.
- [Windows Events Plugin Reference](/src/collectors/windows-events.plugin/README.md)
- [macOS Logs Plugin Reference](/src/collectors/macos-logs.plugin/README.md)
- [Text Files to Journals](/docs/logs/text-files-to-journals.md) — with [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md)
- Integrations — the per-source integration cards.
- OpenTelemetry: [OTLP ingestion](/docs/opentelemetry/otlp-ingestion.md), [logs collection](/docs/opentelemetry/logs-collection.md), [transformations](/docs/opentelemetry/transformations.md), [logs-to-metrics](/docs/opentelemetry/logs-to-metrics.md), and the [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md)
