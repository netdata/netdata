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
- Provides a histogram for log entries over time, with breakdowns per selected field value.
- Supports severity coloring based on the OSLog level.
- Supports PLAY mode through repeated bounded native queries.

## Prerequisites

- macOS with the OSLog framework available.
- Access to the local unified log store. Apple's OSLog API requires an admin account for local system logs.
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

## Play mode

PLAY mode uses repeated native OSLog queries with `if_modified_since`, `anchor`, and time-window bounds. It does not keep
a `log stream` process running.

## Full text search

Full-text search is implemented through Netdata's logs facets layer. The plugin reads bounded native OSLog entries and
then applies Netdata-side search and facet filtering.

## Query performance

The plugin enforces query bounds to protect the Agent:

- time range from `after` and `before`;
- row limit from `last`;
- Function timeout and cancellation;
- a hard scan cap of 1,000,000 entries per query.

If a query reaches the timeout or scan cap, Netdata returns partial data and reports that status in the Function response.
