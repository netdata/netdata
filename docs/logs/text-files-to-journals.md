# Text Files to Journals

Applications that write plain text log files — web servers, legacy services, cron jobs — can be brought into the same Netdata log exploration experience as native OS logs. There are two bridges, and you can use both at the same time for different consumers.

## Choose a bridge

| Your situation | Recommended bridge |
|:---------------|:-------------------|
| You use `journalctl`, journal-aware tooling, or a SIEM that ingests journal files | Convert files to journal entries with [log2journal](/src/collectors/log2journal/README.md) |
| You already run an OpenTelemetry pipeline, or you want Netdata's indexed log store with per-tenant retention and S3 archiving | Ship files through an [OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md) |
| You want both — SIEM keeps its journal feed, Netdata gets its own copy | Run log2journal into the journal and an OpenTelemetry Collector in parallel; the two paths do not interfere |

## Bridge 1: convert files to journal entries

[log2journal](/src/collectors/log2journal/README.md) reads a text log file and converts each line into a structured systemd journal entry. You describe the log format once, as a YAML file with PCRE2 patterns; log2journal then extracts, renames, injects, and rewrites fields per entry. Stock configurations for common formats (for example `nginx-combined`) ship with Netdata.

The pipeline looks like:

```bash
tail -F -n 0 /var/log/nginx/access.log \
  | log2journal -f /etc/netdata/log2journal.d/nginx-combined.yaml \
  | systemd-cat-native --namespace=nginx
```

- [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md) writes the structured entries to the local journal, a journal namespace, or a remote `systemd-journal-remote` over HTTP/HTTPS (`--url`). Because log2journal itself does not require systemd, this also works on non-systemd Linux distributions: convert locally, push to a remote journal.
- Use `LogNamespace=` in a systemd unit to keep the converted logs isolated from the system journal, and to make them appear as a separate source in the Logs tab.
- The result is a first-class journal stream: query it with `journalctl`, forward it with the standard journal centralization mechanisms, ingest it with SIEM agents, and explore it in Netdata under the `systemd-journal` source.

For an end-to-end, production-grade setup (persistent units, namespaces, rotation), follow [Monitor Nginx or Apache web server log files](/docs/developer-and-contributor-corner/collect-apache-nginx-web-logs.md).

## Bridge 2: ship files through an OpenTelemetry Collector

An OpenTelemetry Collector tails the files with its `file_log` receiver — with glob includes/excludes, multiline joining for stack traces, JSON parsing, and persistent offsets across restarts — and forwards structured records to Netdata's OTLP endpoint. The logs appear under the `otel-logs` source, identified by the `service.namespace` and `service.name` resource attributes you set.

The maintained recipes, including JSON lines and multiline parsing, live in [Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md). To normalize, enrich, or drop records before export, see [Transformations](/docs/opentelemetry/transformations.md).

## What you get either way

Once the entries reach a journal or the OpenTelemetry store, the full explorer applies: field filters with counters, full-text search, per-field histograms, live tail, and sampling at scale — see [Exploring Logs](/docs/dashboards-and-charts/logs-tab.md).

## Reference documentation

- [log2journal](/src/collectors/log2journal/README.md) — pattern syntax, field manipulation, stock configurations, performance and security notes.
- [systemd-cat-native](/src/libnetdata/log/systemd-cat-native.md) — pushing structured entries to local, namespace, and remote journals.
- [Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md) — `file_log` and `journald` receivers.
