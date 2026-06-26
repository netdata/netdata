# Content-agnostic file-lifecycle substrate (`file-lifecycle` crate)

## Scope

The `file-lifecycle` crate (`src/crates/file-lifecycle`) is the reusable,
**content-agnostic** machinery that manages a signal's files from ingestion
through retention, knowing nothing about what the files contain (logs vs traces
vs metrics). It is the substrate a per-signal content binding composes; OTel
**logs** (`otel-ledger`) is the first and currently only consumer.

This spec records the durable contract: what lives in the substrate, what stays
in a content binding, the dependency boundary, and the `Pipeline` seam. It
complements:

- [otel-stream-identity.md](otel-stream-identity.md) — the content-plane
  identity (`ServiceStream`/`part_key`/`content_meta`) vs substrate split.
- [otel-remote-storage-config.md](otel-remote-storage-config.md) — the remote
  object-storage URI + credential contract.
- [otel-offline-wal-sfst-query.md](otel-offline-wal-sfst-query.md) — the offline
  query front doors.

## Substrate contents (in `file-lifecycle`)

The substrate owns the machinery that operates only on neutral types
(`FileId`, opaque `part_key: u64`, `seq`, timestamps, opaque
`content_meta: Vec<u8>`, `sfst::Summary`, `otel_catalog::CatalogEntry`):

- `registry` — `TenantRegistries` / `Registry`: per-tenant tracking of WAL,
  sealed SFST, and catalog files keyed by `FileId`/`FileSummary`, plus the
  `seq → tenant` routing, candidate selection, and the neutral stream selector
  (`enumerate_streams_from` yields `PartitionStat`s ordered by opaque
  `part_key`; a content binding decodes `content_meta` for display).
- `component` — the `Component` / `ComponentHandle` worker actor framework.
- `cleaner`, `uploader`, `catalog_builder` — the worker **components**
  (stateless deletion; concurrency-bounded upload; per-`(tenant,date,machine,boot)`
  catalog accumulation + rotation).
- `storage` — the `Storage` trait + `OpendalStorage` client + reachability probe.
- `remote_keys` — the object-storage key scheme, **signal-scoped at the root**
  (`v1/{signal}/tenants/{tenant}/sfst/{date}/{file_id}.sfst` and
  `v1/{signal}/catalog/{date}/{tenant}/…`); every signal carries its segment,
  none is implicit.
- `chunk` — the query-time chunk cache (build singleflight + byte-budget LRU).
- `query` — neutral candidate selection over the registry (local + remote).
- `recovery` — startup reconciliation (`local` + `remote`): orphan cleanup,
  unindexed-WAL sealing, catalog seeding, remote LIST/stat reconcile, retention.
- `upload_retry` — the failed-upload backoff queue.
- `helpers` — catalog-entry / upload-request / retention-policy builders.
- `ipc` — the worker request/response message types (each carries an opaque
  `pipeline_id` so a shared worker's response routes back to its pipeline).
- `pipeline::Pipeline` — the per-signal state shell (see below).

## Content binding (stays in `otel-ledger`, the logs consumer)

- The `Ledger` coordinator: supervisor/writer IPC, the process cancel token, the
  shared workers (cleaner, uploader, chunk cache, upload-retry), the outbound
  funnel, the `select!` run-loop, writer-event/worker-response/function-call
  routing, and the per-signal worker-response forwarders.
- The response-handler methods (`impl Ledger`): content-neutral coordination
  that drives the substrate through `Pipeline` accessors.
- The logs query handler + engine adapter (`ledger::rpc`, `sfsq::logs`).
- The logs **seal step**: the `Indexer` component (`otel-ledger::indexer`) is the
  one production bridge into content code — it calls `sfst_indexer::index`. The
  substrate orchestrates sealing only through the neutral
  `IndexerRequest{wal_path, sfst_path} → IndexerResponse{…, summary}` channel;
  the build impl is the content binding's.
- The logs identity decode for the selector
  (`otel_logs_identity::decode_content_meta_or_empty`).
- `build_logs_pipeline` (the thin logs binding: spawns the logs Indexer and
  passes a `make_handler` closure to the shared `build_pipeline`, which composes
  substrate helpers, spawns the catalog builder, runs recovery, and constructs
  the `Pipeline`).

## Dependency boundary (hard contract)

- `file-lifecycle` MAY depend on the neutral container/catalog crates `sfst` and
  `otel-catalog` (both store opaque `content_meta`; neither imports a log crate).
- `file-lifecycle` MUST NOT depend on any **log-content** crate: `sfsq`,
  `sfst-indexer`, `otel-logs-identity`. The crate manifest omits them (cargo then
  makes importing them impossible), and `file-lifecycle/tests/dep_guard.rs`
  fails if any is ever declared in `[dependencies]` or `[dev-dependencies]`.
- Direction is one-way: `otel-ledger → file-lifecycle`. The substrate never
  calls back into a content binding. Adding a second signal is a new crate
  depending on `file-lifecycle`, not an edit to it.
- Substrate tests use opaque fixtures (`opaque_part_key` std-hash + a local
  length-prefixed `content_meta` codec), never a content-plane identity codec, so
  no content dev-dependency leaks in.

## `Pipeline` seam

`pipeline::Pipeline` is the per-signal state a coordinator routes to, keyed by
`pipeline_id`. It carries: `pipeline_id`, the remote-key `signal` segment, the
`LifecycleConfig`, the tenant `registries`, the seal-index and catalog-builder
request senders, the query `handler` + its `declaration`, and the per-signal
args→payload `arg_shim`.

- Fields are **private**; consumers read them only through accessors
  (`pipeline_id()`, `signal()`, `config()`, `registries()`, `indexer_tx()`,
  `catalog_builder_tx()`, `handler()`, `declaration()`, `arg_shim()`,
  `function_name()`, `storage_enabled()`). This encapsulates the struct's layout
  and construction — a consumer cannot replace a field or skip `Pipeline::new` —
  not the mutability of the state it owns: `registries()` and the worker-sender
  accessors hand back the live handles the coordinator drives.
- A consumer assembles a pipeline with the public `Pipeline::new(...)` from inside
  its `build_*_pipeline`. In `otel-ledger` the shared, signal-neutral
  `ledger::pipeline::build_pipeline` holds the one assembly recipe (dirs,
  registries, recovery, forwarders, `Pipeline::new`); each signal's thin
  `build_logs_pipeline`/`build_traces_pipeline` pre-spawns its seal worker and
  passes a `make_handler` closure. Assembly stays consumer-side (it names the
  coordinator's `PipelineResp` merge vocabulary); the substrate is not the
  assembler.
- The seal step is NOT yet an injected provision: every seal worker (the logs
  `Indexer` and the content-light traces `TracesIndexer`) and both
  `build_*_pipeline` bindings live in `otel-ledger`, so the `sfst_indexer` call
  never crosses into the substrate. The traces pipeline is a content-light proof
  scaffold (a `SUMR`-only seal + stub query handler), not a real content-decoding
  consumer, so it does not yet justify promoting the seal step to an injected
  provision (or relocating the `Ledger` coordinator into the substrate); that is
  deferred to when a real second signal exercises the seam.

## Operator config & derived on-disk layout

The operator-facing config model (in `netdata-plugin/bridge` `PluginConfig`,
resolved by `otel-plugin` `config/{mod,signal,env}.rs`, consumed by both workers):

- **One mandatory `base_dir`.** The plugin derives every per-signal directory
  from it — there is no per-signal dir config. `PluginConfig::lifecycle_for(signal)`
  is the single derivation point, producing the runtime `LifecycleConfig`:
  - `{base_dir}/{signal}/wal`, `{base_dir}/{signal}/index`,
    `{base_dir}/{signal}/catalog` — the signal's WAL / SFST / catalog dirs;
  - `{base_dir}/{signal}/remote-read` — the signal's remote-read cache
    (`LifecycleConfig::read_cache_dir`);
  - `{base_dir}/shared/seq_highwater` — the **signal-neutral** seq high-water
    file (`PluginConfig::seq_highwater_path`). Global seq durability does not
    depend on any one signal's directory; the ingestor seeds the shared
    `SeqAllocator` from `max(per-signal dir scans, this file)`.
- **Global storage and auth** (one `storage` + one `auth` section for the whole
  plugin), combined into each signal's `LifecycleConfig` by `lifecycle_for`. See
  [otel-remote-storage-config.md](otel-remote-storage-config.md).
- **Per-signal tuning only** (`SignalConfig`: wal crc/compression/rotation, index
  retention, catalog rotation_count) — no dirs, no storage. The schema mirrors a
  `logs:` and a `traces:` section (both mandatory; the traces pipeline is always
  built). Each signal's tuning is independent (e.g. a small logs WAL rotation
  threshold does not affect traces, which keeps its own).
- **Migration note (flat → nested):** before this model, logs files lived directly
  at `{base}/{wal,index,catalog}` and the seq high-water at `{base}/index/.seq_highwater`.
  The derived layout adds a `{signal}/` segment (`{base}/logs/wal`, …) and moves the
  seq file to `{base}/shared/seq_highwater`. otel-logs is experimental (never GA), so
  old data is **orphaned, not migrated** — it stays on disk but is not read. Recover
  with `sfsq-cli --wal-dir/--sfst-dir` pointed at the old paths, or relocate the dirs.

## Invariants

- No on-disk format is owned here that the substrate decodes by content: WAL
  payloads, SFST chunks, and `content_meta` are opaque to `file-lifecycle`.
- One global `seq` counter across all signals (the writer is a single process);
  `pipeline_id` (opaque `u16` in `FileId`) is the signal axis. Signals are
  separated by **path** (per-signal local dirs + the `{signal}` remote segment),
  not by the filename.
- Extracting or moving substrate modules must keep the dep-guard green and must
  not reintroduce a content-crate dependency or a back-edge to a content binding.
