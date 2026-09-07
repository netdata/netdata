<!-- markdownlint-disable-file MD043 -->

# macOS Logs plugin

[KEY FEATURES](#key-features) | [LOG SOURCE](#log-source) | [LOG FIELDS](#log-fields) |
[PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [PERFORMANCE](#query-performance) |
[PREREQUISITES](#prerequisites) | [MANAGING](#managing-the-logs) | [FAQ](#faq) |
[TROUBLESHOOTING](#how-to-troubleshoot-common-issues) | [HOW TO VERIFY SETUP](#how-to-verify-setup)

You view, explore, and analyze the macOS unified log from the Netdata dashboard: filter on the OSLog fields, search
their full text, and break down log frequency per field value over time.

`macos-logs.plugin` is a native Netdata Function plugin. It queries Apple's OSLog framework directly and does not invoke
`log show`, `log stream`, or other external log-query commands during normal query execution.

## Key features

- Supports the native **macOS unified log** store.
- Uses Apple's public **OSLog** framework.
- Allows filtering on macOS log fields for any selected time frame.
- Allows full-text search across collected log fields.
- Uses native OSLog predicates for supported selected filters when `slice=true`.
- Provides a histogram for log entries over time, with breakdowns per selected field value.
- Supports severity coloring based on the OSLog level.
- Supports PLAY mode through repeated bounded native queries.

## Prerequisites

- macOS with the OSLog framework available.
- Access to the local unified log store. Apple's OSLog API requires elevated privileges for local system logs, so the installed `macos-logs.plugin` runs with the same root-owned setuid permission model used by Netdata's other privileged Function plugins.
- A Netdata Cloud account to access sensitive Netdata Functions.

## Log source

The plugin exposes one source named `macOS unified log`. It represents the local macOS unified log store available through
`OSLogStore`.

On macOS versions that expose system-wide `OSLogStore` scope, the plugin uses that scope. On older supported versions, it
falls back to the local store API.

## Log fields

The plugin exposes the following fields when they are available from the OSLog entry:

- `MESSAGE`: the composed log message.
- `LEVEL`: the OSLog level, such as `Debug`, `Info`, `Notice`, `Error`, or `Fault`.
- `PROCESS`: the process name that emitted the entry.
- `PID`: the process ID that emitted the entry.
- `SENDER`: the binary image that emitted the entry.
- `SUBSYSTEM`: the OSLog subsystem.
- `CATEGORY`: the OSLog category.
- `ENTRY_TYPE`: the OSLog entry type, such as `Log`, `Activity`, `Signpost`, or `Boundary`.
- `STORE_CATEGORY`: the storage category assigned by the unified logging system.
- `THREAD_ID`: the emitting thread ID.
- `ACTIVITY_ID`: the activity identifier associated with the entry.
- `PARENT_ACTIVITY_ID`: the parent activity identifier for activity entries.
- `FORMAT_STRING`: the OSLog payload format string.
- `COMPONENT_COUNT`: the number of payload components reported by OSLog.
- `SIGNPOST_ID`: the signpost identifier for signpost entries.
- `SIGNPOST_NAME`: the signpost name for signpost entries.
- `SIGNPOST_TYPE`: the signpost type, such as `IntervalBegin`, `IntervalEnd`, or `Event`.

Default facets are `LEVEL`, `PROCESS`, `SENDER`, `SUBSYSTEM`, `CATEGORY`, `ENTRY_TYPE`, and `STORE_CATEGORY`.
`COMPONENT_COUNT`, `SIGNPOST_NAME`, and `SIGNPOST_TYPE` are available as additional facets. High-cardinality fields
such as messages, process IDs, thread IDs, activity IDs, parent activity IDs, signpost IDs, and format strings are exposed
for search and table output but are not used as facets by default.

## Play mode

PLAY mode uses repeated native OSLog queries with `if_modified_since`, `anchor`, and time-window bounds. It does not keep
a `log stream` process running.

## Full text search

Full-text search is implemented through Netdata's logs facets layer. The plugin reads bounded native OSLog entries and
then applies Netdata-side search and facet filtering. Full-text search covers all exposed fields.

## Query performance

The plugin enforces query bounds to protect the Agent:

- time range from `after` and `before`;
- result limit from `last`;
- Function timeout extension while the viewer polls progress;
- cancellation when the viewer leaves.

There is no fixed row-count scan cap. Broad queries run until the OSLog time range is exhausted, the Function timeout is
not extended, or the viewer cancels the request.

`slice=true` is the default when native slicing is supported. In this mode, the plugin builds safe OSLog `NSPredicate`
filters for selected fields that the local OSLog runtime proves it can handle. Unsupported or unproven filters remain in
Netdata's facets layer, so correctness does not depend on OSLog predicate support. When slicing is active, facet counters
come from the selected slice, while previously discovered facet values are retained with zero counters so the UI can add
more values to an existing selection. `slice=false` disables native predicate filtering and scans the requested OSLog
range with userspace filtering only.

Progress reports use the scanned OSLog timestamp range when the enumerator order makes a percentage meaningful. If the
timestamp range cannot produce a safe percentage, progress falls back to a scanned-row working counter.

For broad queries without full-text search or filters on high-cardinality detail fields, the plugin avoids materializing
expensive row-detail fields for entries that cannot be returned in the current page. In that fast path, diagnostic
`bytes_read` counters report materialized message bytes, not every raw OSLog payload byte scanned.

## Managing the logs

macOS logs are managed and queried the same way as every other log source in Netdata — field filters with counters,
full-text search, per-field histograms, and PLAY live tail. See [Managing Logs](/docs/dashboards-and-charts/logs-tab.md) for the shared workflow.

## FAQ

<details>
<summary><strong>Can I centralize macOS logs?</strong></summary>

macOS provides no OS-native forwarding transport for the unified log (no equivalent of `systemd-journal-remote` or
Windows Event Forwarding). You centralize macOS logs with an OpenTelemetry Collector, using the Collector Contrib
`macos_unified_logging` receiver (alpha stability), which reads the unified log by running the macOS `log` command.

See [Centralizing Logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md) for the setup.

</details>

<details>
<summary><strong>Can I use this plugin from a parent Netdata?</strong></summary>

Yes — when your nodes are connected to a Netdata parent, all their functions are accessible via the parent's UI,
including `macos-logs` for each child node.

</details>

<details>
<summary><strong>Does this plugin expose any data to Netdata Cloud?</strong></summary>

No — when accessing the Agent directly, no log data is exposed to Netdata Cloud. The Cloud account is used for
authentication; data flows directly from your Netdata Agent to your web browser.

</details>

<details>
<summary><strong>Why does the plugin need elevated privileges?</strong></summary>

Apple's OSLog API requires elevated privileges to read the local system log store. The installed plugin uses the same
root-owned setuid permission model as Netdata's other privileged Function plugins.

</details>

## How to troubleshoot common issues

| Symptom | Possible cause | Solution |
|---------|----------------|----------|
| `macos-logs` is missing from the Logs tab | The plugin is not installed or not executable | See [How to check if the plugin is running](#how-to-check-if-the-plugin-is-running) |
| Queries return no entries | The selected timeframe or filters exclude everything | Widen the timeframe, clear filters, try a full-text search for a common term |
| Permission errors in the Agent log | The setuid permission model was not applied at install time | Reinstall or repair the Netdata package so the plugin is installed root-owned setuid |
| Slow or cancelled broad queries | The Function timeout expired on a very wide scan | Narrow the timeframe, add filters, or keep `slice=true` so supported filters run natively in OSLog |
| Facet counters look stale after filtering | With `slice=true`, previously discovered facet values are retained with zero counters by design | This is expected; counters reflect the selected slice |

## How to verify setup

### How to check if the plugin is running

Confirm the plugin binary is present and executable. `/usr/local/netdata` is the install prefix the kickstart script
uses on macOS; if Netdata is installed elsewhere, use that prefix instead:

```bash
ls -l /usr/local/netdata/usr/libexec/netdata/plugins.d/macos-logs.plugin
```

Confirm the plugin process is running while the Agent is active:

```bash
ps aux | grep '[m]acos-logs.plugin'
```

### How to test basic queries

1. Open the **Logs** tab in the Netdata UI and confirm `macos-logs` is offered, with its `macOS unified log` source.
2. Apply a single filter (for example `LEVEL = Error`) and confirm entries are returned.
3. Use full-text search for a common term and verify results.
4. Toggle **PLAY** mode and confirm new entries stream in as they are logged.
