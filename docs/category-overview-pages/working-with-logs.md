# Working with Logs

Netdata presents platform-native and ingested logs as structured, searchable sources in the [Logs tab](/docs/dashboards-and-charts/logs-tab.md).

## Choose a log source

- **Linux with systemd** — Use the [systemd journal plugin](/src/collectors/systemd-journal.plugin/README.md) to explore local, namespace, and centralized journal files.
- **Windows** — Use the [Windows Events plugin](/src/collectors/windows-events.plugin/README.md) to explore Windows Event Logs.
- **macOS** — Use the [macOS Logs plugin](/src/collectors/macos-logs.plugin/README.md) to explore the local unified log store.
- **Applications and services that emit OTLP logs** — Use the [OpenTelemetry plugin](/src/crates/otel-plugin/README.md) to receive and index them.
- **Text application logs on a systemd host** — Use [log2journal](/src/collectors/log2journal/README.md) to parse them and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md) to write the structured entries to a journal.

## Keeping logs local with journal namespaces

On systems running systemd 245 or later, journal namespaces can keep collected application logs in a separate local journal instead of mixing them with the default journal. This does not require forwarding logs to another host.

Follow the [`log2journal` service setup](/src/collectors/log2journal/README.md#best-practices) to create a persistent pipeline with `LogNamespace=`. The [systemd journal plugin](/src/collectors/systemd-journal.plugin/README.md) automatically discovers namespace journals in the standard journal locations, so they appear in the Logs tab without additional collector configuration.

## Non-systemd Linux systems

Linux distributions without systemd, such as Alpine Linux, cannot use the [systemd journal plugin](/src/collectors/systemd-journal.plugin/README.md) locally because it requires a local `systemd-journald` installation. Note that [log2journal](/src/collectors/log2journal/README.md) itself does not require systemd — it is a standalone text processor that can run on any Linux system to convert log files to Journal Export Format. The converted output can then be piped to `systemd-cat-native --url` for remote forwarding (see option 1 below).

You can still make logs available in Netdata using these alternatives:

1. **Remote journal forwarding with `systemd-cat-native --url`** — Use [`systemd-cat-native --url=URL`](/src/libnetdata/log/systemd-cat-native.md) to send logs directly to a remote `systemd-journal-remote` running on another Linux system with systemd. This mode works even when the local system has no systemd, allowing the remote systemd journal to become the logs database for the local system. To view these logs in Netdata, the receiving system must run a Netdata Agent with the systemd journal plugin, and `systemd-journal-remote` must be configured and accessible at the specified URL.

2. **OpenTelemetry (OTLP) log ingestion** — The [OpenTelemetry plugin](/src/crates/otel-plugin/README.md) receives logs via the OTLP/gRPC protocol and indexes them for fast querying in the Logs tab (the `otel-logs` source). This method does not depend on a local systemd installation.

You can also find useful guides on how to set up log centralization points in the [Observability Centralization Points](/docs/deployment-guides/deployment-with-centralization-points.md) section of our docs.
