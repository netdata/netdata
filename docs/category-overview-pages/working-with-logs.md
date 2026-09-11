# Logs Management

Netdata manages logs in a mix of distributed and centralized ways. It uses the indexed log databases each operating
system already maintains — on every node, and on the OS-native log centralization points you already run — and adds
its own indexed log store, with transparent offloading to object storage, for the logs you choose to
centralize. Every event stays queryable from the same interface, and you control what logs cost by deciding where each
source is stored and for how long, instead of filtering or discarding logs to fit a budget. See
[Log Storage and Retention](/docs/logs/log-storage-and-retention.md).

A node with Netdata installed is a complete setup: its logs are searchable and streaming live in the Logs tab, with
nothing to configure and no additional storage. Centralization is a per-source decision you take for the sources that
need it.

## Where logs live

| Tier | Storage | What Netdata adds | Setup |
|:-----|:--------|:------------------|:------|
| **In place, on each node** | The OS log store: systemd journal files on Linux, event channels on Windows, the unified log on macOS | Field filters with live counters, full-text search across every field, histograms, live tail, and one interface across all nodes | Install Netdata |
| **On the OS-native centralization points you already run** | Journals aggregated by `systemd-journal-remote`; forwarded-events channels on a Windows Event Collector | The same, over every sender the point aggregates, with the logs still in native format for `journalctl`, Event Viewer, and SIEM agents | Install Netdata on the centralization point |
| **Journals written by Netdata** | Journal-compatible files that Netdata itself writes for SNMP traps and network flows, on the node that receives them; no `systemd-journald` involved | The same indexing and querying as any journal; readable with `journalctl` and by SIEM agents on Linux | Configure the SNMP trap or network flow collector |
| **Centralized with OpenTelemetry** | Netdata's own indexed log store on the receiving node: retention per tenant, optional offloading to S3-compatible object storage with transparent read-back | Storage that outlives the sending nodes, retention beyond a node's disk, and the same interface | Point an OpenTelemetry Collector at Netdata's OTLP endpoint |

The OS-native tiers add no storage and no pipeline: Netdata reads the logs where the operating system writes them, and
the operating system's own tools keep working on the same data. The journals Netdata writes for SNMP traps and network
flows use the systemd journal file format without requiring systemd: Netdata reads them on every platform it writes
them on, and on Linux `journalctl` (systemd 252 or later) and SIEM agents read the same files. The OpenTelemetry tier
is for the logs that must survive their source, need retention beyond the node's disk, or come from platforms without
an OS log store, such as Kubernetes.

## Decide per source

| Your situation | Where the logs go |
|:---------------|:------------------|
| Servers and VMs whose logs you need for troubleshooting | In place. Retention follows the node's own log policy, and the logs live as long as the node does. |
| Logs that must outlive the node: audit trails, forensics, retention mandates | [Centralize these sources with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md) into Netdata's log store; leave everything else in place. |
| You already aggregate journals or Windows events with `systemd-journal-remote` or Windows Event Forwarding | Install Netdata on the aggregation point. The aggregated logs stay in native format for your existing tooling. |
| Kubernetes and containers | Run the [OpenTelemetry Collector in the cluster](/docs/opentelemetry/logs-collection.md#kubernetes-and-containers) and ship container logs to Netdata's log store. |
| Application text files | Convert them to journal entries with `log2journal` to keep using `journalctl` and your SIEM on them, or ship them with an OpenTelemetry Collector. |
| Network devices sending syslog | An [OpenTelemetry Collector syslog receiver](/docs/npm/syslog/README.md) forwarding to Netdata. |
| Network devices sending SNMP traps, NetFlow, sFlow, or IPFIX | Netdata receives them directly and writes journal-compatible files on the receiving node; see [SNMP Trap Logs](/docs/logs/snmp-trap-logs.md) and [Network Flows](/docs/logs/network-flows.md). |
| macOS | In place. macOS has no OS-native log forwarding; to centralize, use the [OpenTelemetry Collector's macOS receiver](/docs/opentelemetry/logs-collection.md#macos-unified-log). |

Any mix works. Centralization points do not need to be infrastructure-wide: run one per team, environment, or
datacenter, sized for its own volume, and keep critical systems' logs local. Netdata Cloud presents every node and every
centralization point in one dashboard with one role-based access model.

## Log sources

| Source | Platform | Storage | Where you query it |
|:-------|:---------|:--------|:-------------------|
| [systemd journal](/src/collectors/systemd-journal.plugin/README.md) | Linux | Native journal files: system, user, namespace, and remote journals | Logs tab, `systemd-journal` |
| [Windows Event Log](/src/collectors/windows-events.plugin/README.md) | Windows | Native event channels, including the forwarded-events channels on a collector | Logs tab, `windows-events` |
| [macOS unified log](/src/collectors/macos-logs.plugin/README.md) | macOS | The native unified log store | Logs tab, `macos-logs` |
| [OpenTelemetry logs](/src/crates/otel-plugin/README.md) | Any, via OTLP | Netdata's indexed log store | Logs tab, `otel-logs` |
| Text log files | Any | A journal, through [log2journal](/src/collectors/log2journal/README.md); or Netdata's log store, through an [OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md) | Logs tab, `systemd-journal` or `otel-logs` |
| [Network device syslog](/docs/npm/syslog/README.md) | Network devices | Netdata's log store, through an OpenTelemetry Collector | Logs tab, `otel-logs` |
| [SNMP traps](/docs/logs/snmp-trap-logs.md) | Network devices | Journal-compatible files written by Netdata under its log directory | Logs tab, `snmp:traps` (SNMP Trap Logs) |
| [Network flows](/docs/logs/network-flows.md) (NetFlow, sFlow, IPFIX) | Network devices | Journal-compatible files written by Netdata under its cache directory, in four time tiers | The Network Flows view |

## One interface for every source

All log sources share the Logs tab (network flows have their own view): field filters with live counters, full-text search, per-field histograms, live tail,
and the node's per-second metrics on the same dashboard, so you read an event next to the exact moment a metric
changed. Netdata Cloud brings every node and centralization point into one dashboard with role-based access, so
reading production logs does not require shell access to production systems. See
[Managing Logs](/docs/dashboards-and-charts/logs-tab.md).

Log content stays in your infrastructure. Viewing logs requires signing in with Netdata Cloud, which is free for
community use; the content is transmitted encrypted to your browser and is not stored in Netdata Cloud. For full data
sovereignty, [Netdata Cloud On-Prem](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/README.md)
runs the same service inside your own infrastructure.

## Current limitations

- **A query runs against one node or centralization point at a time.** Netdata Cloud presents all of them; you select
  which one to query.
- **Alerts are evaluated on metrics, not on log content.** To alert on a log pattern, derive a metric from it, for
  example with [logs-to-metrics](/docs/opentelemetry/logs-to-metrics.md), and alert on that metric.
- **The OTLP endpoint accepts OTLP/gRPC only** (port 4317). OTLP/HTTP is not supported.
- **macOS has no OS-native log forwarding.** Centralize macOS logs through the OpenTelemetry Collector.

## In this section

- [Managing Logs](/docs/dashboards-and-charts/logs-tab.md) — how logs are organized, queried, and explored: sources,
  filters, full-text search, histograms, live tail, and query behavior at scale.
- [Systemd Journal Logs](/src/collectors/systemd-journal.plugin/README.md) — the journal reference,
  [Forward Secure Sealing](/src/collectors/systemd-journal.plugin/forward_secure_sealing.md), and
  [Logs Centralization Points](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md)
  for environments that already aggregate journals.
- [Windows Event Logs](/src/collectors/windows-events.plugin/README.md) — event channels on nodes and on Windows
  Event Collectors.
- [macOS Unified Logs](/src/collectors/macos-logs.plugin/README.md) — the unified log store on macOS nodes.
- [Text Files to Journals](/docs/logs/text-files-to-journals.md) — application log files, with
  [log2journal](/src/collectors/log2journal/README.md) and
  [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md).
- [Centralizing Logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md) — which sources to
  centralize and what to set up; the Collector recipes live in the OpenTelemetry pages.
- [Log Storage and Retention](/docs/logs/log-storage-and-retention.md) — retention settings per tier, offloading to
  object storage, and sizing.
- [SNMP Trap Logs](/docs/logs/snmp-trap-logs.md) and [Network Flows](/docs/logs/network-flows.md) — the journals
  Netdata writes for network devices.
- Integrations — the per-source integration cards.
- Related: the OpenTelemetry section — [OTLP ingestion](/docs/opentelemetry/otlp-ingestion.md),
  [logs collection](/docs/opentelemetry/logs-collection.md),
  [transformations](/docs/opentelemetry/transformations.md),
  [logs-to-metrics](/docs/opentelemetry/logs-to-metrics.md), and the
  [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md).
