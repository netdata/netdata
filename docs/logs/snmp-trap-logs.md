# SNMP Trap Logs

Netdata receives SNMP traps from network devices and writes them, decoded, into journal-compatible files on the node
that runs the trap collector: the systemd journal file format, produced by Netdata's own writer, without
`systemd-journald`. They appear in the Logs tab as **SNMP Trap Logs** (`snmp:traps`), with the same field filters,
full-text search, histograms, and live tail as every other source.

- **Where they are stored:** `traps/<job>/<machine-id>/` under the Netdata log directory (`/var/log/netdata` by
  default), one directory per collector job.
- **Retention:** per job, `retention.max_size` (`10GB` by default) and an optional `max_duration`; see
  [Log Storage and Retention](/docs/logs/log-storage-and-retention.md#journals-written-by-netdata).
- **Command line and SIEM:** on Linux with systemd 252 or later, `journalctl --directory=<dir>` reads the files, and
  so do SIEM agents that ingest journal files. Netdata reads them on every platform it writes them on.
- **Forwarding:** traps can also be sent over OTLP to any OpenTelemetry backend.

The collector, its configuration, the trap fields, and the query workflows are documented under Network Performance
Monitoring:

- [SNMP Traps](/docs/npm/snmp-traps/README.md) — setup and overview.
- [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md) — paths, `journalctl` commands, export.
- [Forwarding to SIEM](/docs/npm/snmp-traps/forwarding-to-siem.md) — journal shippers and OTLP forwarding.
- [Configuration](/docs/npm/snmp-traps/configuration.md) — listeners, decoding, retention.
