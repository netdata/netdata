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
index of trace IDs so that a whole trace can be looked up by its ID. Queries also read the write-ahead log, so new
spans are visible before they are sealed. If a query cannot read the write-ahead log, the answer is partial with the
reason `source_failure`. A span sent without a kind or a status is indexed
with the OpenTelemetry defaults, `UNSPECIFIED` and `UNSET`, so those values can be filtered on like any other. Spans
stored by an earlier version keep no kind or status for those defaults, so until their files leave retention a filter
on `UNSET`, `UNSPECIFIED`, or a negation such as a status other than `ERROR` does not match them.
Each span is also indexed with its role in its trace: `ROOT` when it has no parent, `INBOUND` for a server or consumer
span, `OUTBOUND` for a client or producer span, and `INTERNAL` for all other spans. Spans stored by an earlier version
have no role, so a filter on the role does not match them until their files leave retention.
Spans stored by an earlier version also keep their status under an older name: the trace view shows no status for
them and does not count them as errors, and a status filter does not match them. The trace list can still count their
errors when the earlier version indexed their files. This stays true until their files leave retention; offloaded files
stay in object storage until you remove them.

## Retention

Retention applies to sealed indexed files, oldest first, when any of three limits is exceeded:

| Option | Default | Meaning |
|:-------|:--------|:--------|
| `traces.retention.default.max_files` | 100000 | Maximum number of indexed files kept |
| `traces.retention.default.max_total_size` | 1GB | Maximum total size of indexed files kept |
| `traces.retention.default.max_age` | 7 days | Maximum age of an indexed file, measured on the start time of its newest span |

`max_total_size` is not a cap on the plugin's disk usage: the active write-ahead log (up to `max_file_size`), catalogs,
and the download cache for offloaded data are additional. Retention runs when a file is sealed, and also every 30
seconds. When spans stop arriving, the last write-ahead log seals on idle (within about 15 minutes), and the files that
pass `max_age` are still deleted.

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
- Some requests do not download offloaded files. They report each offloaded file they select as
  `remote_unavailable`. These requests are:
  - the suggestions of the Traces search bar. They read the newest data first and can stop early, so they report
    only the offloaded files they reach before they stop;
  - a lookup of one trace by its ID without a time range. This lookup reads all local data. To include offloaded
    data, give a time range.
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
(`max_file_size`) and, when offloading is enabled, for the download cache, which logs and traces share. The service
map uses about 256 MiB under `<base_dir>/service-map/`. The Agent checks this size every 10 minutes, so the
directory can be larger for a short time.

## Exploring traces

Stored traces are explored in the Traces tab, which queries the Agent's `otel-traces` Function. As with logs, viewing
traces requires a signed-in Netdata Cloud user of the Agent's Space. Viewing traces also requires a paid plan
(Homelab, Business, or Enterprise On-Premise; an active Business trial also qualifies). The free Community plan does not include it. The Agent receives and stores
traces on all plans. Trace data is not stored in Netdata Cloud.

When a sender sends a span again, the Agent stores each copy. The span counts in the trace list include each stored
copy. The trace view shows each span one time. Traces in files that an earlier version stored show each span one time
in the list too, until those files leave retention.

## Service map

The Agent also builds a service map from the spans that it receives. Open it from the **Topology** section of the
**Live** view (Function `topology:tracing`). Viewing the map has the same requirements as viewing traces: a signed-in
Netdata Cloud user of the Agent's Space and a paid plan. The Agent builds the map on all plans. The map shows:

- each instrumented service (one per `service.name`, and per `service.namespace` when it is set);
- the calls between these services;
- the databases, message queues and external endpoints that the services call but that send no spans. The map names
  them from the attributes of the client spans, for example `db.system` or `server.address`.
- one **unknown caller** for calls from callers that send no spans, for example health checks.

For each call direction, the map shows the number of calls, the errors and the latency (p50, p95, p99 and maximum).
It shows the client side and the server side separately. Select a service to see the services that it calls and the
services that call it.

How the map counts calls:

- A call counts when a server span and its parent client span arrive within 30 seconds of each other.
- A client span that gets no server span in 30 seconds counts as a call to a database, queue or external endpoint.
- A server span that gets no parent client span in 30 seconds counts as a call from the unknown caller.
- The map shows the last 15 minutes by default, rounded out to whole minutes. The Agent keeps per-minute counts for
  7 days, in about 256 MiB at most (the Agent checks the size every 10 minutes).
- The map starts to count when the Agent starts. It does not use spans that the Agent stored before.

Limits:

- The map shows only the spans that this Agent receives. If the two services of a call send their spans to different
  Agents, each Agent shows only its half of the call.
- If the senders sample traces, the map shows fewer calls than the services made.
- Names longer than 256 bytes are cut. A database name or a protocol name (for example `redis`) never merges with an
  instrumented service of the same name; a peer service name or a host name does.

## Where to next

- [OTLP Ingestion](/docs/opentelemetry/otlp-ingestion.md) — send traces through a Collector.
- [Securing the OTLP Endpoint](/docs/opentelemetry/securing-the-otlp-endpoint.md) — TLS and mutual TLS for remote
  senders.
- [OpenTelemetry plugin reference](/src/crates/otel-plugin/README.md) — every `otel.yaml` option.
