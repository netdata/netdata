# Working with Logs

This section talks about the ways Netdata collects and visualizes logs.

The [systemd journal plugin](/src/collectors/systemd-journal.plugin) is the core Netdata component for reading systemd journal logs.

For structured logs, Netdata provides tools like [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md) to convert them into compatible systemd journal entries.

## Non-systemd Linux systems

Linux distributions without systemd, such as Alpine Linux, cannot use the [systemd journal plugin](/src/collectors/systemd-journal.plugin) or [log2journal](/src/collectors/log2journal/README.md) locally, because both require a local `systemd-journald` installation.

You can still make logs available in Netdata using these alternatives:

1. **Remote journal forwarding with `systemd-cat-native --url`** — Use [`systemd-cat-native --url=URL`](/src/libnetdata/log/systemd-cat-native.md) to send logs directly to a remote `systemd-journal-remote` running on another Linux system with systemd. This mode works even when the local system has no systemd, allowing the remote systemd journal to become the logs database for the local system. The receiving system must have `systemd-journal-remote` configured and accessible at the specified URL.

2. **OpenTelemetry (OTLP) log ingestion** — The [OpenTelemetry Signal Viewer plugin](/src/crates/netdata-log-viewer/otel-signal-viewer-plugin/README.md) receives logs via the OTLP/gRPC protocol and stores them in systemd-compatible journal files, which are then displayed in the Logs tab. This method does not depend on a local systemd installation.

You can also find useful guides on how to set up log centralization points in the [Observability Centralization Points](/docs/deployment-guides/deployment-with-centralization-points.md) section of our docs.
