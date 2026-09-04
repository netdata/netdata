# Log Storage and Retention

You control what logs cost by setting, per source, where the logs are stored and for how long they are kept. Nothing
has to be filtered or discarded to fit a budget: logs managed in place use the disk the node already has, under the
operating system's own retention settings, and logs centralized with OpenTelemetry use Netdata's log store on the
receiving node, with per-tenant retention and optional offloading to object storage. Netdata meters nothing by volume.

| Tier | Where the bytes are | What sets the retention |
|:-----|:--------------------|:------------------------|
| In place on Linux nodes | systemd journal files on the node | `journald.conf`, per namespace |
| In place on Windows nodes | Event log channels on the node | Per-channel maximum size and retention policy |
| In place on macOS nodes | The unified log store on the node | Managed by macOS |
| On an existing journal centralization point | Journal files written by `systemd-journal-remote` | `journal-remote.conf` |
| On an existing Windows Event Collector | The forwarded-events channels of the collector | Per-channel maximum size and retention policy |
| Journals written by Netdata (SNMP traps, network flows) | Journal-compatible files on the node that receives them | The collector's retention settings |
| Centralized with OpenTelemetry | Netdata's log store on the receiving node, optionally offloaded to object storage | `otel.yaml`, per tenant |

Netdata reads whatever each store retains. Changing a retention setting changes what is queryable from that moment on;
it does not require any change in Netdata.

## systemd journal

`systemd-journald` keeps its files under `/var/log/journal` (persistent) or `/run/log/journal` (volatile, lost on
reboot), as selected by `Storage=` in `/etc/systemd/journald.conf`. Set `Storage=persistent` on nodes whose logs must
survive a reboot. The limits below are size-based first; time-based deletion is off by default.

| Option | Default | Meaning |
|:-------|:--------|:--------|
| `SystemMaxUse=` | 10% of the file system, at most 4G | Total disk the journal may use under `/var/log/journal` |
| `SystemKeepFree=` | 15% of the file system, at most 4G | Disk the journal leaves free for other uses; the smaller of the two limits wins |
| `SystemMaxFileSize=` | One eighth of `SystemMaxUse=`, at most 128M | Size at which a journal file rotates |
| `SystemMaxFiles=` | 100 | Maximum number of journal files kept |
| `MaxRetentionSec=` | 0 (off) | Delete files whose entries are all older than this; use it to enforce a retention policy |
| `MaxFileSec=` | 1 month | Rotate a file after this time even if it is not full; smaller values lose less data at once when old files are deleted |
| `Compress=` | yes | Compress data objects larger than 512 bytes; small fields are stored as written |
| `Seal=` | yes | Forward Secure Sealing; see [FSS](/src/collectors/systemd-journal.plugin/forward_secure_sealing.md) |

The `Runtime*` variants apply the same limits to `/run/log/journal`. After editing, apply with
`systemctl restart systemd-journald` (or `systemctl kill --signal=SIGUSR1 systemd-journald` to flush volatile entries
to persistent storage first) and confirm the result with `journalctl --disk-usage`.

Planning rule: journal files take roughly the size of the raw log text they hold. To keep everything a node produces,
raise `SystemMaxUse=` above the node's daily log volume multiplied by the retention you need, and set
`MaxRetentionSec=` to that retention so the policy is enforced by time as well as by size.

### Journal namespaces

Each journal namespace runs its own `systemd-journald` instance with its own files and its own limits, configured in
`/etc/systemd/journald@NAMESPACE.conf`. Use a namespace to give an application its own retention budget, isolated from
the system journal, and to make it a separate source in the Logs tab. Services opt in with `LogNamespace=` in their
unit; converted text files can be written to a namespace with
[systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md).

### Journal centralization points

On a node that receives journals with `systemd-journal-remote`, the received files are subject to
`/etc/systemd/journal-remote.conf`, not to `journald.conf`. The `[Remote]` section provides `MaxUse=`, `KeepFree=`,
`MaxFileSize=`, and `MaxFiles=`, analogous to the `System*` options above, and `SplitMode=` decides whether each
sending host gets its own file. Size the receiving disk for the sum of the senders' volumes multiplied by the retention
you need on the point.

## Windows Event Log

Every event channel has a maximum size and a policy for what happens when it is reached. Set both in Event Viewer
(right-click a log, **Properties**) or with `wevtutil`:

```powershell
wevtutil sl Application /ms:1073741824 /rt:false
```

- `/ms:<bytes>` sets the maximum size in bytes. The minimum is 1048576 (1 MB) and sizes are rounded to a multiple of
  64 KB.
- `/rt:false` (the default) overwrites the oldest events when the channel is full. `/rt:true` keeps the existing events
  and discards new ones, which is only useful together with auto-backup.
- `/ab:true` archives the channel to a file when it is full instead of losing events; it requires `/rt:true`.

On a Windows Event Collector, size the `ForwardedEvents` channel (or the custom channels your subscriptions target) for
the combined volume of all forwarders multiplied by the retention you need.

## macOS unified log

macOS manages the retention of its unified log store itself. Netdata reads what the store holds; there is no
Netdata-side retention setting. To keep macOS logs longer than the OS does, centralize them with OpenTelemetry (see
[Centralizing Logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md)).

## Journals written by Netdata

Netdata writes SNMP traps and network flows into journal-compatible files itself: the systemd journal file format,
produced by Netdata's own writer, with no `systemd-journald` involved. Netdata reads them on every platform it writes
them on; on Linux, `journalctl` from systemd 252 or later reads the same files (they use the compact journal mode), and
so do SIEM agents that ingest journal files.

- **[SNMP traps](/docs/logs/snmp-trap-logs.md)** are written under `traps/<job>/<machine-id>/` in the Netdata log directory (`/var/log/netdata` on
  package installs; static installs use `/opt/netdata/var/log/netdata`), one directory per collector job. Retention is
  set per job in the collector configuration:
  `retention.max_size` (`10GB` by default) and an optional `max_duration`; files rotate automatically. Query them from
  the Logs tab (`snmp:traps`) or with `journalctl --directory=<dir>`. See
  [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md) and
  [Configuration](/docs/npm/snmp-traps/configuration.md).
- **[Network flows](/docs/logs/network-flows.md)** (NetFlow, sFlow, IPFIX) are written under `flows/` in the Netdata cache directory
  (`/var/cache/netdata/flows` on package installs) in four tiers, `raw`, `1m`, `5m`, and `1h`; files rotate on size, and
  a file spans at most one hour.
  Retention is set per tier: `size_of_journal_files` (`10GB` per tier by default, about 40 GB in total) and an
  optional `duration_of_journal_files`. Query them from the Network Flows view or with `journalctl --file=<file>`. See
  [Retention and Querying](/docs/npm/network-flows/retention-querying.md) and
  [Configuration](/docs/npm/network-flows/configuration.md).

## Netdata's log store

Logs received over OpenTelemetry are stored by the receiving Netdata Agent under `base_dir`
(default `/var/log/netdata/otel/v2` on package installs), with per-tenant subdirectories under each signal's
write-ahead-log and index directories. Incoming records are appended to a write-ahead log; when it reaches
`max_file_size` (25 MB), `max_entries` (50000), or `max_file_duration` (about 15 minutes), it is sealed
into an indexed file and the write-ahead log is deleted. Each indexed file stores every distinct `field=value` pair
once,
in compressed dictionaries local to that file, and references it from every entry, so every field is indexed and the
disk usage stays close to that of the compressed raw text.

Retention applies to sealed indexed files, per tenant, oldest first, when any of three limits is exceeded:

| Option | Default | Meaning |
|:-------|:--------|:--------|
| `logs.retention.default.max_files` | 100000 | Maximum number of indexed files kept |
| `logs.retention.default.max_total_size` | 1GB | Maximum total size of indexed files kept |
| `logs.retention.default.max_age` | 7 days | Maximum age of an indexed file, measured on its newest entry |

`max_total_size` is not a cap on the plugin's disk usage: active write-ahead logs (up to `max_file_size` per stream),
catalogs, and the remote read cache are additional. Retention runs when a file is sealed; a tenant that stops sending
gets one final pass when its last write-ahead log
seals on idle (within about 15 minutes), then keeps its remaining files until it sends again or the Agent restarts.

Per-tenant sections inherit every field they omit from `default`. The section name is the tenant, which is the
`X-Scope-OrgID` header value when tenant selection is enabled:

```yaml
auth:
  enabled: true
logs:
  retention:
    default:
      max_total_size: "20GB"
      max_age: "30 days"
    audit:
      max_total_size: "200GB"
      max_age: "400 days"
```

With tenant selection enabled, a log or trace sender that omits the `X-Scope-OrgID` header is rejected (metrics are
not tenant-scoped); with it disabled, every record lands in the `default` tenant regardless of headers.

Edit `otel.yaml` with [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) and restart
the Agent. The full option list is in the [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md).

### Offloading to object storage

With `remote_storage.enabled: true`, every sealed indexed file is also uploaded to `remote_storage.uri`, an `s3://` or
`fs://` location. Uploading changes nothing locally: files stay under local retention, and a local file is not deleted
by retention until its catalog entry confirms it is in the remote. When a query needs an offloaded file that is no
longer
local, the Agent downloads it through a cache bounded by `remote_storage.read_cache_max_size` (1GB), and the query
waits for the download. A query whose files exceed the cache fails with a message to narrow the time window or stream
filter.

```yaml
remote_storage:
  enabled: true
  uri: "s3://my-bucket/netdata-logs?region=us-east-1"
  read_cache_max_size: "4GB"
```

- Never put credentials in the URI or in `otel.yaml`. For S3, use the AWS environment variables, credentials file, or
  an instance role available to the Netdata service account; to pass environment variables to the `netdata` service,
  use a root-only systemd `EnvironmentFile`. Non-secret backend options such as `region` and `endpoint` go in the query
  string.

  ```bash
  # /etc/systemd/system/netdata.service.d/s3-credentials.conf
  [Service]
  EnvironmentFile=/etc/netdata/s3-credentials.env

  # /etc/netdata/s3-credentials.env  (root-only, mode 0600)
  AWS_ACCESS_KEY_ID=...
  AWS_SECRET_ACCESS_KEY=...
  ```

  Apply with `systemctl daemon-reload && systemctl restart netdata`.
- Uploads that fail are retried with backoff; while the remote is unreachable, sealed files accumulate locally without
  a ceiling. Monitor free disk on the receiving node.
- This is how long retention is made cheap: keep days locally with a small `max_age`, keep months or years in object
  storage, and query both from the same Logs tab.

### Sizing the receiving node

Run the pipeline for a full day, then measure `du -sh` on the tenant's directory under `<base_dir>/logs/index/` and
multiply
by the retention you want locally. For long retention there are two shapes: keep everything on local disk, sizing it
as one day's index size × `max_age`, as the `audit` example above does with its 400 days; or keep `max_age` short
(30 days, say), enable offloading, and size the object storage for one day's index size × the total retention you
want reachable — older files are then fetched back from S3 through the read cache when queried. Add headroom for active
write-ahead logs (`max_file_size` per stream) and for the
read cache when offloading is enabled. Queries are bounded by their time range and by the Agent's function timeout, so
on large stores keep the
default window and narrow it further before running a full-text search; see
[Managing Logs](/docs/dashboards-and-charts/logs-tab.md#query-behavior-at-scale).

## Command-line access

The Netdata Agent ships a command-line query tool for its log store: `otel-plugin logs`, a subcommand of the plugin
binary (`/usr/libexec/netdata/plugins.d/otel-plugin` on most installs). It reads the store's files directly — offline,
without a running Agent — which makes it usable for forensics: a stopped node, or a disk mounted on another machine.

```bash
sudo /usr/libexec/netdata/plugins.d/otel-plugin logs \
  --config /etc/netdata/otel.yaml --tenant default \
  --name checkout --since -1h --filter 'level=error' --limit 1000
```

- Select the window with `--since`/`--until` (epoch seconds, relative values such as `-1h`, or UTC datetimes), the
  stream with `--name`/`--namespace`, rows with `--filter 'field=value,field~regex'` and `--query REGEX`, and the
  output fields with `--fields`.
- Output is NDJSON on stdout, one object per row and 50 rows by default (raise it with `--limit`), ready for `jq`;
  a `matched=N returned=M window=...` summary and any warnings go to stderr.
- It reads local files only: records offloaded to object storage and already evicted locally are not visible to it,
  and the newest records of an actively written stream may be missing. The Logs tab is authoritative for live data.
- It runs read-only, takes no locks, and does not disturb ingestion; it needs read access to the store's directory
  (run it as root or the `netdata` user). Exit code 0 means the query ran; check stderr for skipped files.

## Where to next

- [Centralizing Logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md) — choose which sources
  to centralize and how to send them.
- [Logs Centralization Points](/docs/observability-centralization-points/logs-centralization-points-with-systemd-journald/README.md)
  — Netdata on journal aggregation points you already run.
- [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md) — every `otel.yaml` option.
