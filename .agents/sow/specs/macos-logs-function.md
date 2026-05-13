# macOS Logs Function

## Scope

This spec records the shipped contract for the native macOS unified logs
Function.

It applies to:

- `src/collectors/macos-logs.plugin/`
- Netdata Function registration for `macos-logs`
- Logs tab behavior on macOS nodes
- Cloud and direct-agent Function query skills that enumerate log Functions

## Function Contract

- Function name: `macos-logs`.
- Function category: `logs`.
- Plugin binary: `macos-logs.plugin`.
- Platform: macOS builds with the OSLog framework available.
- Access flags: same sensitive-data pattern as the Linux and Windows log
  Functions, requiring signed-id, same-space, and sensitive-data access.
- The Function is a Log Explorer Function with history, facets, histogram,
  pagination, `data_only`, tail, and PLAY-mode semantics provided through the
  shared logs query/status layer.

## Native Query Contract

- The implementation queries Apple's public OSLog framework directly.
- Normal query execution must not invoke `log show`, `log stream`, or another
  external log-query command.
- The primary source is the local macOS unified log store.
- On macOS versions exposing system-wide `OSLogStore` scope, the plugin uses
  that scope. Older supported versions fall back to the local store API.
- OSLog access is permission-dependent. Apple's API requires an admin account
  for local system log access.
- Installed source and static builds must make `macos-logs.plugin`
  `root:netdata` and mode `4750` where setuid-root plugins are supported, in
  line with other privileged Netdata Function plugins.
- Netdata must report store/enumerator open failures as Function errors rather
  than silently fabricating empty data.

## Source Selector Contract

- The source selector parameter remains `__logs_sources`, matching the shared
  logs Function contract.
- Supported native source ids are `all`, `system`, and `macos-unified-log`.
- `all` is the default selected source in `info=true` discovery.
- Unknown source selector values are treated as no matching native source unless
  future versions add explicit support for them.

## Field Contract

The Function exposes these fields when OSLog provides them:

- `MESSAGE`: composed log message.
- `LEVEL`: OSLog level.
- `LEVEL_ID`: hidden numeric level used for row severity.
- `PROCESS`: emitting process name.
- `PID`: emitting process id.
- `SENDER`: emitting binary image.
- `SUBSYSTEM`: OSLog subsystem.
- `CATEGORY`: OSLog category.
- `ENTRY_TYPE`: OSLog entry class.
- `STORE_CATEGORY`: unified-log storage category.
- `THREAD_ID`: emitting thread id.
- `ACTIVITY_ID`: associated activity id.

`MESSAGE` is the main visible text field and participates in full-text search.
`LEVEL`, `PROCESS`, `SENDER`, `SUBSYSTEM`, `CATEGORY`, `ENTRY_TYPE`, and
`STORE_CATEGORY` are default facet candidates.

## Bounds And Safety

- Queries must be bounded by the requested time range.
- Queries must honor Function timeout and cancellation.
- Queries must honor `last`, pagination anchors, `data_only`, tail, and
  `if_modified_since` behavior through the shared logs Function contract.
- The implementation must enforce a hard per-query scan cap.
- Timeout or scan-cap results must be marked partial instead of pretending the
  result set is complete.
- Real macOS log payloads, usernames, hostnames, process arguments, and
  identifiers must not be committed as fixtures, SOW evidence, docs, or specs.
