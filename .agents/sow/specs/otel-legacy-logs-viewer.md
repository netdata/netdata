# OTel legacy logs viewer contract

Scope: the read-only `legacy-otel-logs` function and the obsolete-plugin deny
that together let users view OpenTelemetry logs stored by the **former** otel
plugin after upgrading to the WAL/SFST-backed implementation.

## Background

The former otel plugin ingested OTLP logs into **systemd journal files**
(default `/var/log/netdata/otel/v1/<machine_id>/*.journal`) and a separate
`otel-signal-viewer` plugin served them through an `otel-logs` function. The new
implementation ingests into a WAL/SFST store and the `otel-ledger` worker now
serves `otel-logs`. The former ingestion and viewer were removed. This feature
restores **viewing only** of already-stored journal logs. OTel logs never
graduated from experimental, so the whole feature is best-effort.

## Contract

- **Function name:** `legacy-otel-logs` (tag `logs`, global, access
  `SIGNED_ID | SAME_SPACE | SENSITIVE_DATA`). It MUST stay distinct from the new
  `otel-logs` (served by `otel-ledger`); two plugins registering the same
  function name resolve nondeterministically in the agent functions registry
  (`DICT_OPTION_DONT_OVERWRITE_VALUE` + swap-on-conflict).
- **Read-only:** the viewer opens journal files for reading only. It never
  writes, rotates, prunes, or deletes them. Pruning is the user's
  responsibility (neither plugin supports remote config; users manage files /
  edit `otel.yaml` on the host).
- **Directory resolution:** the worker reads the journal directory from
  `logs.journal_dir` in the user `otel.yaml` (then stock). The runtime
  `PluginConfig` does not carry this field; it is read in place by the
  dedicated `resolve_legacy_journal_dir` probe, whose probe-local structs stay
  deliberately tolerant (a malformed `otel.yaml` is warned about and skipped,
  not silently treated as "no override"). The main config parsers are STRICT
  (`deny_unknown_fields`, 2026-07): `logs.journal_dir` is a declared, accepted
  key of the current schema, but a user file still carrying other
  former-schema keys (`size_of_journal_file`, ...) refuses startup with a
  migration guide (`config/legacy.rs`) — so in practice the viewer only runs
  once the user file passes strict parsing. The default mirrors the former plugin's
  `@logdir_POST@/otel/v1`: `$NETDATA_LOG_DIR/otel/v1`, falling back to
  `/var/log/netdata/otel/v1` only when `NETDATA_LOG_DIR` is unset. The initial
  directory scan is recursive (walkdir), so the per-`<machine_id>` subdirectory
  layout is discovered automatically.
- **Absent directory / init failure:** on most installs the resolved directory
  does not exist (the former logs feature was experimental). When it is missing —
  or when handler initialization fails (e.g. an unwritable cache dir) — the worker
  registers no function, signals `Ready { declarations: [] }`, and idles
  (reacting only to `Shutdown`). The supervisor then sees zero legacy
  declarations, installs no route, and the otel-plugin stays healthy.
- **On-disk format:** unchanged from the former plugin. The current
  `journal-*` crates read the former `journal-log-writer` output without
  migration.

## Architecture

- The `otel-plugin` supervisor spawns a third worker, `legacy-logs`
  (`otel-legacy-logs` crate), alongside `ingestor` and `ledger`. The worker
  carries a dedicated `LegacyLogsConfig` (journal dir + cache dir + former-viewer
  indexing defaults: memory 1000, disk 32 MB, cardinality 500, payload 100),
  not the shared `PluginConfig`.
- The handler is a faithful port of the former `otel-signal-viewer-plugin`
  catalog handler onto the `bridge::function::FunctionHandler` trait, driving the
  restored `journal-function` query stack (`JournalRequest`/`JournalResponse`).
- The worker replicates the args→payload shim (`info`, `after:`, `before:`) so
  GET-style logs-UI calls populate the request payload, matching the ledger.
- Restored-crate deltas: `journal-function` (restored under `src/crates/`)
  differs from the former `netdata-log-viewer` copy only in integration glue, not
  query logic or on-disk format. Intentional deltas: dependency paths point at
  this branch's workspace `journal-*` crates; `lib.rs` re-exports the engine /
  registry types the worker consumes (e.g. `Facets`, `FileIndexCache`,
  `Registry`); and `JournalRequest::after`/`before` carry `#[serde(default)]` so
  capability/`info` probes without a time range still deserialize.

## Failure isolation

The legacy viewer is best-effort and fully decoupled from the new pipeline: its
failures MUST NOT affect the ingestor/ledger, which keep their fatal-on-failure
contract by design.

- A supervisor-side `legacy_alive` flag gates the legacy worker. A configure
  failure at startup or a runtime disconnect flips it to `false` while the
  supervisor keeps serving ingestor + ledger. The flag is monotonic — workers are
  never restarted (the agent restarts the whole plugin if needed).
- On disable, the supervisor drops the legacy routing and in-flight transaction
  entries but does NOT send `FUNCTION_DEL`. The agent keeps the declaration, so a
  later call resolves to "no handler" and times out agent-side rather than
  erroring — an accepted trade-off for a best-effort surface.
- An oversized response (over the IPC message cap) is degraded to a small
  status-500 result instead of bailing the worker (mirrors the ledger), so one
  outsized query cannot crash-loop the viewer.
- The filesystem-watch forwarder holds a `Weak` handle reference, so it cannot
  keep the handler (and its watcher) alive after shutdown.

## Obsolete-plugin deny (upgrade safety)

- The agent refuses to launch a `plugins.d` binary named `otel-signal-viewer`
  (`is_obsolete_plugin()` in `src/plugins.d/plugins_d.c`, hard deny regardless of
  `[plugins]` config). Rationale: the updater does not delete former plugin
  binaries, so a stale `otel-signal-viewer-plugin` could re-register `otel-logs`
  and collide with the new ledger's `otel-logs`. Plugins are version-locked to
  the agent, so the agent is the authority on obsolete names.
- Physical removal of the obsolete package/binary across install channels
  (rpm `Obsoletes`, deb `Replaces`/`Conflicts`, static-installer cleanup) is
  tracked separately as netdata/netdata#22728; the deny makes the collision fix
  correct on every channel without it.
