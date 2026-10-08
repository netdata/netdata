# Network Flows

Netdata receives NetFlow, sFlow, and IPFIX from network devices and writes the flows into journal-compatible files on
the node that runs the flows plugin: the systemd journal file format, produced by Netdata's own writer, without
`systemd-journald`. Flows have their own view, **Network Flows**, with summaries, Sankey diagrams, time series, maps,
and facets; they are not shown in the Logs tab.

- **Where they are stored:** `flows/` under the Netdata cache directory (`/var/cache/netdata/flows` by default), in
  four time tiers, `raw`, `1m`, `5m`, and `1h`; files rotate on size, and a file spans at most one hour.
- **Retention:** per tier, `size_of_journal_files` (`10GB` per tier by default, about 40 GB in total) and an optional
  `duration_of_journal_files`; see
  [Log Storage and Retention](/docs/logs/log-storage-and-retention.md#journals-written-by-netdata).
- **Command line:** on Linux with systemd 252 or later, `journalctl --file=<file> --output=json` reads the files;
  flow entries carry the flow fields and no `MESSAGE=`. Netdata reads them on every platform it writes them on.
- **Availability:** on Linux native packages the plugin is a separate, optional package, `netdata-plugin-netflow` —
  install it explicitly from Netdata's repository. Static builds (except 32-bit ARMv6) and all Docker images bundle
  it. It is not available on Windows. Once the plugin is present, Netdata starts it automatically and it listens on
  UDP ports 2055 (NetFlow/IPFIX) and 6343 (sFlow).

The plugin, its configuration, the flow fields, and the views are documented under Network Performance Monitoring:

- [Network Flows](/docs/npm/network-flows/README.md) — setup and overview.
- [Retention and Querying](/docs/npm/network-flows/retention-querying.md) — the four tiers, what each keeps, and
  reading the files with `journalctl`.
- [Configuration](/docs/npm/network-flows/configuration.md) — listeners, enrichment, retention per tier.
- [Field Reference](/docs/npm/network-flows/field-reference.md) — every flow field.
