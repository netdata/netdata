# SOW-0018 - Carve storage-engine-types.h so dbengine stops including rrd.h

## Status

Status: open

Sub-state: drafted as the first SOW in the dbengine-library extraction series; awaits activation after SOW-0016 completes.

## Requirements

### Purpose

Decouple the dbengine source tree from the daemon's kitchen-sink `src/database/rrd.h` so that `rrdengine.h` and its siblings can be compiled standalone against libnetdata + the carved POD-types header. This is the first prerequisite for landing dbengine as a separate library artifact ([[dbengine-library]]) and for Rust bindings ([[dbengine-rust-bindings-scaffold]]).

### User Request

User asked to move toward a standalone dbengine library that can be packaged separately and bound from Rust. The investigation identified `rrd.h` (which transitively pulls in streaming, sqlite, ACLK, contexts, health, web/api, daemon/common) as the largest header-level coupling preventing standalone compilation.

### Assistant Understanding

Facts:

- `src/database/engine/rrdengine.h:11` includes `../rrd.h`.
- `src/database/rrd.h` includes (rrd.h:93-127) storage-engine, rrdhost, rrdset, rrddim, streaming, sqlite, ACLK, contexts, health, web/api, daemon/common, rrdcollector, rrdfunctions, sqlite_*.
- `rrdengine.h:23` includes `daemon/protected-access.h`, which has no daemon dependency beyond libnetdata.
- The engine internally needs from `rrd.h` only: `STORAGE_INSTANCE`, `STORAGE_METRIC_HANDLE`, `STORAGE_COLLECT_HANDLE`, `STORAGE_METRICS_GROUP`, `STORAGE_POINT`, `STORAGE_PRIORITY`, `storage_engine_query_handle`, `SN_FLAGS`, `storage_number`, `storage_number_tier1_t`, `RRD_BACKFILL`, `RRD_STORAGE_TIERS`, `nd_uuid_t`, `UUIDMAP_ID`.
- `src/database/storage-engine.h` already declares most of these; it includes only `libnetdata/libnetdata.h` and `rrd-database-mode.h` plus `daemon/config/netdata-conf-db.h`.

Inferences:

- A leaf header `src/database/storage-engine-types.h` can capture the POD types and forward declarations dbengine needs without any daemon dependency.
- `daemon/protected-access.h` is used by 5 sites inside the engine; moving it into `libnetdata/` (or creating a tiny standalone header) keeps the dependency surface clean.

Unknowns:

- Whether `rrd-database-mode.h` itself transitively reaches into the daemon (must verify before the carve).

### Acceptance Criteria

- New header `src/database/storage-engine-types.h` exists and contains the POD type and enum subset needed by the engine, plus opaque forward declarations.
- `src/database/engine/rrdengine.h` no longer includes `../rrd.h`; instead it includes `storage-engine-types.h`.
- `src/database/storage-engine.h` re-includes `storage-engine-types.h` (no behavior change for daemon consumers).
- `daemon/protected-access.h` is reachable from the engine without an `src/daemon/...` include path (moved into libnetdata or relocated).
- `make` / `ninja` build of `netdata` succeeds with no diagnostics newly suppressed.
- A focused test compiles a stub TU including only `storage-engine-types.h` + `rrdengine.h` and proves the include closure does not reach streaming, sqlite, ACLK, contexts, health, web/api.

## Analysis

Sources checked:

- `src/database/engine/rrdengine.h`
- `src/database/rrd.h`
- `src/database/storage-engine.h`
- `src/database/engine/*.c` for actual symbol usage
- `src/daemon/protected-access.h`

Current state:

- The engine compiles as part of the monolithic `netdata` executable. Every TU in the engine pulls in the full daemon header graph because of `rrdengine.h:11`.
- `protected-access.h` has only `libnetdata/libnetdata.h` + `setjmp.h` as direct dependencies and is used by 5 sites in pagecache/journalfile/rrdengine.

Risks:

- A type that looks POD in `storage-engine.h` might transitively depend on a daemon-only symbol via inline functions. Mitigated by compiling the stub TU as part of the acceptance test.
- Changes to `storage-engine.h` ripple to every daemon consumer. Mitigated by making the new header strictly additive — `storage-engine.h` keeps its current content and just `#include`s the new header.

## Pre-Implementation Gate

Status: ready (pending activation)

Problem / root-cause model:

- `rrd.h` is a leaky kitchen-sink header. The engine's tight inclusion via `rrdengine.h:11` means dbengine cannot be compiled without compiling almost the whole daemon. The carve makes the type surface explicit and small.

Evidence reviewed:

- `src/database/engine/rrdengine.h` line 11 and surrounding includes.
- `src/database/rrd.h` lines 80-127 (the kitchen-sink include block).
- `src/database/storage-engine.h` (the existing minimal-deps header).

Affected contracts and surfaces:

- Header include paths in 14 dbengine source files.
- Every daemon TU that includes `storage-engine.h` (no semantic change — the new header is included indirectly).
- The location of `protected-access.h` if relocated.

Existing patterns to reuse:

- `src/database/storage-engine.h` already demonstrates that a header in `src/database/` can be minimally-coupled.
- The libnetdata header tree (`src/libnetdata/`) is the right home for `protected-access.h`.

Risk and blast radius:

- Low. Pure header reorganization, no source changes beyond `#include` lines.

Sensitive data handling plan:

- No secrets, no customer data, no credentials. Headers contain type definitions only.

Implementation plan:

1. Audit the actual type/enum usage in `src/database/engine/*.{c,h}` and produce the minimal type closure.
2. Create `src/database/storage-engine-types.h` with that closure.
3. Update `storage-engine.h` to include the new header (forward, no removal).
4. Update `rrdengine.h:11` to include `storage-engine-types.h` instead of `rrd.h`.
5. Update `mrg.h:5` and `cache.h:6` similarly (they also include `../rrd.h`).
6. Move `daemon/protected-access.h` into `src/libnetdata/protected-access/` (header + implementation), update the 5 call sites in the engine, and update the daemon's signal-handler wiring.
7. Verify the rest of the daemon still builds (`netdata`, `netdatacli`, `systemd-cat-native`).
8. Add a tiny `tests/dbengine-include-isolation.c` that includes only the carved headers and asserts via a compile-time check that the include closure remains bounded.

Validation plan:

- `cmake --build` of the full `netdata` target succeeds.
- `gcc -E` on the isolation test TU produces a preprocessed output that does NOT mention streaming/sqlite/ACLK/contexts headers (grep-based check).
- No new warnings appear in the dbengine TUs.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: `dbengine-library.md` updated to mark the header-surface section as landed.
- End-user/operator docs: no update.
- End-user/operator skills: no update.
- SOW lifecycle: standard close, no split/merge.

Open decisions:

- Whether `protected-access.h` moves into `libnetdata/protected-access/` (preferred) or stays under `daemon/` and is re-exposed via a libnetdata-style include path. Will decide at activation by checking how the signal handler is registered.

## Plan

1. Stub TU + grep test for include isolation.
2. Carve types header.
3. Switch engine headers to the carved include.
4. Relocate `protected-access.h`.
5. Full build + isolation check.

## Execution Log

Pending.

## Validation

Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
