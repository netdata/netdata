# Trace Storage and Retention

The Netdata Agent stores the OpenTelemetry traces it receives on its own disk, indexed, and serves them to the Traces
tab of the dashboard. This page covers how spans are stored, how long they are kept, offloading to object storage,
and sizing the receiving node. To send traces to the Agent, see
[OTLP Ingestion](/docs/opentelemetry/otlp-ingestion.md#send-traces).

## How spans are stored

Spans received over OpenTelemetry are stored by the receiving Netdata Agent under `base_dir` (default
`/var/log/netdata/otel/v2` on package installs), in its `traces/` directory. Incoming spans are appended to one
write-ahead log. When it reaches `traces.rotation.default.max_file_size` (25MB) or `traces.rotation.default.max_entries` (50000 spans), or about
15 minutes after its first span, it is sealed into an indexed file and the write-ahead log is deleted. Each indexed file
indexes the span names, kinds, and statuses and the span, resource, scope, event, and link attributes, and keeps an
index of trace IDs so that a whole trace can be looked up by its ID. A span sent without a kind or a status is indexed
with the OpenTelemetry defaults, `UNSPECIFIED` and `UNSET`, so those values can be filtered on like any other. Spans
stored by an earlier version keep no kind or status for those defaults, so until their files leave retention a filter
on `UNSET`, `UNSPECIFIED`, or a negation such as a status other than `ERROR` does not match them.

## Retention

Retention applies to sealed indexed files, oldest first, when any of three limits is exceeded:

| Option | Default | Meaning |
|:-------|:--------|:--------|
| `traces.retention.default.max_files` | 100000 | Maximum number of indexed files kept |
| `traces.retention.default.max_total_size` | 1GB | Maximum total size of indexed files kept |
| `traces.retention.default.max_age` | 7 days | Maximum age of an indexed file, measured on the start time of its newest span |

`max_total_size` is not a cap on the plugin's disk usage: the active write-ahead log (up to `max_file_size`), catalogs,
and the download cache for offloaded data are additional. Retention runs when a file is sealed; when spans stop
arriving, the last write-ahead log seals on idle (within about 15 minutes) and gets one final pass, and the remaining
files are kept until spans arrive again or the Agent restarts.

Traces have their own settings, separate from `logs`. Set the limits under `traces.retention.default`; a user
`otel.yaml` needs only the fields that change:

```yaml
traces:
  retention:
    default:
      max_total_size: "20GB"
      max_age: "30 days"
```

Edit `otel.yaml` with [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) and restart
the Agent. The full option list is in the [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md).

## Offloading to object storage

Traces use the same `remote_storage` settings as logs: one object storage backend, and one download cache at
`<base_dir>/remote-read`, bounded by `remote_storage.read_cache_max_size` and shared by both. With
`remote_storage.enabled: true`, every sealed trace file is also uploaded, a local file is not deleted by retention until
the remote confirms it, and a query that needs an offloaded file that is no longer local downloads it into the download
cache. Setup, credentials, upload retries, and the download cache are described in
[Log Storage and Retention](/docs/logs/log-storage-and-retention.md#offloading-to-object-storage).

Reading traces back from object storage has these limits:

- A query that needs more offloaded data than the download cache holds fails with a message to narrow the time range
  or raise `remote_storage.read_cache_max_size`. A search also reads up to 24 hours beyond each side of its window to
  complete the traces it finds.
- Looking up one trace by its ID without a time range searches everything kept, local and offloaded, so it fails
  once the offloaded history exceeds the download cache.
- A file that cannot be downloaded, or a catalog that cannot be read, makes the answer partial with the reason
  `remote_unavailable`; one missing file in a search's range marks every trace summary as possibly incomplete and
  empties searches that filter on the root service, root operation or trace duration. A file that downloads intact
  but cannot be read is reported as a failed source (`source_failure`).
- Files are downloaded one at a time, each within its own time limit. While the remote is down, each file waits out
  its limit in turn (a file that another query is already downloading can cost up to about three limits), so a query
  over many offloaded files can take minutes.

## Sizing the receiving node

Run the senders for a full day, then measure `du -sh` on `<base_dir>/traces/index/` and multiply by the retention you
want locally; set `max_total_size` to that size, since the 1GB default is usually reached
long before the 7-day `max_age`. For longer retention, keep `max_age` short, enable offloading, and size the object
storage for one day's index size × the time you keep objects there; the Agent does not delete offloaded files, so
expire them with the object storage's own lifecycle rules. Add headroom for the active write-ahead log
(`max_file_size`) and, when offloading is enabled, for the download cache, which logs and traces share.

## Exploring traces

Stored traces are explored in the Traces tab, which queries the Agent's `otel-traces` Function. As with logs, viewing
traces requires a signed-in Netdata Cloud user of the Agent's Space; trace data is not stored in Netdata Cloud.

## Where to next

- [OTLP Ingestion](/docs/opentelemetry/otlp-ingestion.md) — send traces from an SDK or a Collector.
- [Securing the OTLP Endpoint](/docs/opentelemetry/securing-the-otlp-endpoint.md) — TLS and mutual TLS for remote
  senders.
- [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md) — every `otel.yaml` option.
