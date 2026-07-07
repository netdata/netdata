# Working with Logs

This section talks about the ways Netdata collects and visualizes logs.

The [systemd journal plugin](/src/collectors/systemd-journal.plugin) is the core Netdata component for reading systemd journal logs.

For structured logs, Netdata provides tools like [log2journal](/src/collectors/log2journal/README.md) and [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md) to convert them into compatible systemd journal entries.

## Keeping logs local with journal namespaces

When collected logs are sent to the default systemd journal, they intermix with kernel, service, and audit messages — the same stream you see in `journalctl` with no filter or in `/var/log/messages`. systemd journal namespaces let you store collected logs in a separate, dedicated journal on the same server, keeping them out of the system log without forwarding them to a remote host. Pipe parsed logs through [`systemd-cat-native`](/src/libnetdata/log/systemd-cat-native.md) with the `--namespace` option to write them to a namespace journal instead of the default one:

```bash
tail -F /var/log/nginx/*.log | log2journal 'PATTERN' | systemd-cat-native --namespace=netdata_logs
```

The namespace must already be configured and running on the system. The [systemd journal plugin](/src/collectors/systemd-journal.plugin) automatically discovers namespace journals, so the collected logs appear in the Logs tab without additional collector configuration.

## Non-systemd Linux systems

Linux distributions without systemd, such as Alpine Linux, cannot use the [systemd journal plugin](/src/collectors/systemd-journal.plugin) locally because it requires a local `systemd-journald` installation. Note that [log2journal](/src/collectors/log2journal/README.md) itself does not require systemd — it is a standalone text processor that can run on any Linux system to convert log files to Journal Export Format. The converted output can then be piped to `systemd-cat-native --url` for remote forwarding (see option 1 below).

You can still make logs available in Netdata using these alternatives:

1. **Remote journal forwarding with `systemd-cat-native --url`** — Use [`systemd-cat-native --url=URL`](/src/libnetdata/log/systemd-cat-native.md) to send logs directly to a remote `systemd-journal-remote` running on another Linux system with systemd. This mode works even when the local system has no systemd, allowing the remote systemd journal to become the logs database for the local system. The receiving system must have `systemd-journal-remote` configured and accessible at the specified URL.

2. **OpenTelemetry (OTLP) log ingestion** — The [OpenTelemetry plugin](/src/crates/otel-plugin/README.md) receives logs via the OTLP/gRPC protocol and indexes them for fast querying in the Logs tab (the `otel-logs` source). This method does not depend on a local systemd installation.

You can also find useful guides on how to set up log centralization points in the [Observability Centralization Points](/docs/deployment-guides/deployment-with-centralization-points.md) section of our docs.
