# Spec - dbengine library contract

This spec describes WHAT the Netdata dbengine subsystem provides as a
standalone, library-shaped component once the extraction work tracked by the
`dbengine-*` SOW series lands. It captures durable contract decisions across
SOWs so that future contributors and Rust-binding work have a single
reference.

The on-disk format (`src/database/engine/rrddiskprotocol.h`) and journal v2
layout are owned by the existing source tree, not by this spec, and are not
changed by the extraction.

## Scope

In scope:

- The C public API of the dbengine library (instance lifecycle, metric
  lifecycle, store/load operations, retention, statistics).
- The process-wide initialization model and configuration struct.
- The dependency surface (which libnetdata facilities and external libraries
  the library may use).
- The packaging contract (static / shared library artifact, headers
  exposed).
- The optional callback surface for telemetry (pulse) and other hooks.

Out of scope:

- True per-instance isolation of caches, MRG, and event loop (referred to as
  "Phase 2" in the investigation; deferred).
- On-disk format changes.
- Query planner, exporting, streaming, contexts, health, web/api: these are
  consumers of the library and live in the daemon, not in the library.

## Ownership boundary

The library owns:

- `src/database/engine/*.c` and `src/database/engine/*.h` except test files.
- The dbengine compression module (`dbengine-compression.{c,h}`).
- A new public header that exposes the library-init contract
  (`dbengine-library.h`).

The library does NOT own:

- `src/database/storage-engine.{c,h}` — the storage-engine vtable lives in
  the daemon and dispatches into the library. The carved
  `storage-engine-types.h` is owned by the daemon side of the contract.
- `RRDDIM`, `RRDSET`, `RRDHOST`, query planner, contexts, streaming, ACLK,
  sqlite, health, web/api.

## Process-wide singletons (Phase 1 reality)

The library, in its Phase 1 form, retains a single shared:

- `rrdeng_main` event loop (one uv_loop, one worker thread pool, one
  command queue, one set of ARALs for opcodes/descriptors/handles/xt_io).
- `main_mrg` Metric Registry, keyed by `(Word_t)ctx` per instance.
- `main_cache`, `open_cache`, `extent_cache` page caches, also keyed by
  `(Word_t)ctx` per instance.
- `wal_globals`, `pdc_globals`, and the various ARAL singletons.

Multiple `struct rrdengine_instance *ctx` values coexist today and after
extraction; they share memory budgets, eviction, and worker capacity.

A consumer that needs hard isolation (independent memory budgets, separate
event loop, etc.) is out of scope here and tracked separately.

## Lifecycle

```
dbengine_library_init(&cfg)        // once per process
  rrdeng_init(&ctx, path, ...)     // per instance
  ... store / load / query ...
  rrdeng_quiesce(ctx)
  rrdeng_exit(ctx)
dbengine_library_shutdown()        // once per process
```

`dbengine_library_init` is idempotent and reentrant-safe but the first call
wins on configuration. Subsequent calls with a different configuration
return an error.

`rrdeng_init` may be called concurrently for different ctx values. The
library handles per-ctx initialization synchronization internally.

`dbengine_library_shutdown` requires all instances to have been
`rrdeng_exit`-ed first. It tears down the event loop, page caches, and MRG.

## Public C API (post-extraction shape)

Three header files form the library's public surface:

- `database/storage-engine-types.h` — POD types shared with the daemon:
  `STORAGE_PRIORITY`, `STORAGE_INSTANCE`, `STORAGE_METRIC_HANDLE`,
  `STORAGE_COLLECT_HANDLE`, `STORAGE_METRICS_GROUP`, `STORAGE_POINT`,
  `storage_engine_query_handle`, `SN_FLAGS`, `storage_number`,
  `storage_number_tier1_t`, `RRD_BACKFILL`, `RRD_STORAGE_TIERS`.
- `database/engine/dbengine-library.h` — process-wide init/shutdown +
  `dbengine_library_config_t`.
- `database/engine/rrdengineapi.h` — per-instance lifecycle and per-metric
  store/load/retention/statistics, all taking
  `struct rrdengine_instance *ctx` or `STORAGE_*` handles.

No public function takes `RRDDIM*`, `RRDSET*`, `RRDHOST*`, or any other
daemon type after the extraction is complete. UUIDs reach the library via
`UUIDMAP_ID` or `nd_uuid_t *`.

## Configuration model

`dbengine_library_config_t` is consumed once at library init. It holds:

Process-wide knobs (today loose `extern` globals):

- Page cache and extent cache size budgets (MB).
- Out-of-memory protection threshold; use-all-RAM-for-caches flag.
- Direct I/O preference; journal v2 unmount time; pages per extent.
- Storage tier topology: number of active tiers, default `update_every`
  in seconds, per-tier grouping factors.
- Debug switches (`journal_check`, `new_defaults`,
  `legacy_multihost_db_space`).
- Optional telemetry callbacks (`on_aral_register`,
  `on_aral_register_statistics`, `on_gorilla_hot_buffer_added`,
  `on_gorilla_tier0_page_flush`, and similar pulse hooks). Null callbacks
  are valid and behave as no-ops.
- Optional `unittest_mode` flag (replaces the libnetdata global of the
  same name for dbengine-internal purposes).

Per-instance knobs live on `struct rrdengine_instance::config`:

- `tier`, `page_type`, `dbfiles_path`, `max_disk_space`,
  `max_retention_s`, `global_compress_alg`, `backfill`, `update_every_s`.

The daemon's `src/daemon/config/netdata-conf-db.c` is the producer of the
library config. Consumers that drive the library outside the daemon
(unit tests, Rust harness) construct their own config.

## Dependency surface

The library may use:

- libnetdata (mallocz/freez/strdupz, aral, nd_log, nd_thread, BUFFER,
  SPINLOCK, completions, uuidmap, signal helpers).
- libuv (event loop, file I/O, async, timers).
- Judy.
- lz4, zstd (compression).
- OpenSSL (sha/evp used by journal v2 hashing).

The library MUST NOT depend on:

- `src/daemon/*` (except a small re-exported `protected-access.h` moved
  into libnetdata).
- `src/database/{contexts,sqlite,rrd.h,rrdhost.h,rrdset.h,rrddim.h,…}`.
- Streaming, ACLK, web/api, health, exporting.

`rrdengine.h` includes only `storage-engine-types.h` and libnetdata
headers; the kitchen-sink `database/rrd.h` is not on the library
include path.

## Packaging contract

CMake produces:

- `dbengine` static library target (`add_library(dbengine STATIC ...)`)
  built when `ENABLE_DBENGINE=ON` (the default).
- Optional `BUILD_DBENGINE_SHARED` flag that switches the library type to
  SHARED for Rust dynamic-link / external-consumer scenarios.
- `netdata` executable links `dbengine` as a `target_link_libraries(...
  PRIVATE dbengine)` dependency rather than compiling its sources.
- A separate `dbengine-tests` executable target owns
  `dbengine-unittest.c`, `dbengine-stresstest.c`, `mrg-unittest.c`,
  `page_test.cc`. It links libdbengine + libnetdata + the RRD layer.

## Rust binding contract

A `dbengine-sys` crate under `src/crates/` consumes the three public
headers via bindgen and links the static library. A safe wrapper crate
`dbengine` exposes RAII handles:

- `Library` — owns init/shutdown; constructed once per test process.
- `Instance` — owns `rrdeng_init`/`rrdeng_exit`.
- `Metric` — owns `rrdeng_metric_*` lifecycle.
- `Collector` / `QueryHandle` — own store and load handles.

The Rust binding is a test/research vehicle. Production code paths are
not expected to consume the library from Rust in Phase 1.

## Compatibility commitments

- On-disk format unchanged. Existing datafiles remain readable after the
  extraction.
- The dispatched storage-engine vtable in `src/database/storage-engine.c`
  remains the daemon's integration point; its signatures do not change
  except where SOW-0019 explicitly removes `RRDDIM*` from public
  dbengine entry points (the vtable continues to take a handle from the
  daemon side and translate at the wrapper).
- `multidb_ctx[RRD_STORAGE_TIERS]` and `rrdeng_init(NULL, …)` semantics
  (allocate-from-static-array vs allocate-fresh) are preserved.

## SOWs implementing this spec

The SOW series `dbengine-*` (SOW-0018 through SOW-0025) lands the
extraction in independent PRs. Each SOW updates this spec when it lands
durable contract changes (e.g. SOW-0021 finalizes the
`dbengine_library_config_t` shape; SOW-0024 finalizes the CMake
artifact; SOW-0025 finalizes the Rust crate boundary).
