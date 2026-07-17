<!-- markdownlint-disable-file MD043 -->

# macOS Logs plugin

[KEY FEATURES](#key-features) | [LOG SOURCE](#log-source) | [LOG FIELDS](#log-fields) |
[PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [PERFORMANCE](#query-performance) |
[PREREQUISITES](#prerequisites)

The macOS Logs plugin by Netdata makes viewing, exploring, and analyzing macOS unified logs simple and efficient.

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
- A signed-in Netdata Cloud user in the same Space as the node with permission to view sensitive data. This authorization is required for direct Agent, Parent, and Netdata Cloud access.

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
