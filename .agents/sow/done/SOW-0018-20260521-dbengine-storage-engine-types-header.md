# SOW-0018 - Carve storage-engine-types.h so dbengine stops including rrd.h

## Status

Status: completed

Sub-state: completed 2026-05-21 on branch `dbengine/0018-storage-engine-types-header` after focused header reorganization, libnetdata-side protected-access relocation, full netdata build verification via `just cfg && just ninja`, and a preprocessor-level header isolation check. SOW-0016 remains paused in `current/` per its 2026-05-14 sub-state; this dbengine workstream is orthogonal and was authorized as a newer user instruction.

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

### 2026-05-21

- Created branch `dbengine/0018-storage-engine-types-header` off `master`.
- Committed planning artifacts (spec + 8 SOWs) as a separate prior commit on the branch.
- Created `src/database/storage-engine-types.h` carrying the POD type/enum surface previously embedded in `storage-engine.h`: `RRDDIM` and `RRDSET` forward declarations, `RRD_BACKFILL` enum (moved here from `rrddim-backfill.h`), `STORAGE_PRIORITY`, `STORAGE_ENGINE_BACKEND`, `is_valid_backend` macro, `storage_engine_query_handle`, `STORAGE_INSTANCE` / `STORAGE_METRIC_HANDLE` / `STORAGE_METRICS_GROUP` opaque typedefs, `STORAGE_COLLECT_HANDLE` struct, `STORAGE_QUERY_HANDLE` forward. Header includes only `libnetdata/libnetdata.h` and `rrd-database-mode.h`.
- Updated `src/database/storage-engine.h` to include `storage-engine-types.h` and removed the duplicated definitions.
- Updated `src/database/rrddim-backfill.h` to include `storage-engine-types.h` and removed its local `RRD_BACKFILL` definition.
- Switched `src/database/engine/rrdengine.h`, `mrg.h`, and `cache.h` from `#include "../rrd.h"` to `#include "../storage-engine-types.h"`.
- Removed the daemon-path `#include "daemon/protected-access.h"` from `rrdengine.h`.
- Relocated `protected-access` from `src/daemon/protected-access.{h,c}` to `src/libnetdata/protected-access/protected-access.{h,c}` via `git mv`. Updated the header to include `../common.h`, `../signals/signals.h`, `<setjmp.h>`, `<signal.h>` instead of `libnetdata/libnetdata.h` (matching the convention of other libnetdata sub-headers).
- Added `#include "protected-access/protected-access.h"` to `src/libnetdata/libnetdata.h` so all libnetdata consumers (including engine .c files and daemon code that previously had explicit includes) get it transitively.
- Removed now-redundant `#include "protected-access.h"` from `src/daemon/signal-handler.c`.
- Updated `CMakeLists.txt`: removed `src/daemon/protected-access.{c,h}` from `DAEMON_FILES`, added the new path under `LIBNETDATA_FILES`, added `src/database/storage-engine-types.h` to `RRD_PLUGIN_FILES`.
- Added a forward declaration of `netdata_conf_cpus(void)` to `cache.h` so its inline functions still compile after losing the transitive include via `rrd.h`. SOW-0021 will route this through the library config struct.
- Engine implementation files (.c) that previously got daemon-side symbols transitively via `rrd.h` (pulse hooks, `UV_EVENT_DBENGINE_*`, `populate_metrics_from_database`, `nd_profile`, `localhost`) now include `../rrd.h` explicitly. Files touched: `cache.c`, `datafile.c`, `dbengine-compression.c`, `journalfile.c`, `mrg.c`, `mrg-load.c`, `mrg-unittest.c`, `page.c`, `pagecache.c`, `pdc.c`, `rrdengine.c`, `rrdengineapi.c`. `rrdenginelib.c` did not need it (only uses `fatal`/`fatal_assert` which are libnetdata). SOWs 0020-0022 will eliminate these `.c`-side `rrd.h` includes by routing daemon facilities through the library configuration.
- Updated `src/daemon/pulse/pulse-db-dbengine-retention.c` to include `database/rrd.h` explicitly — previously it received that transitively via the now-cleaned `rrdengineapi.h`.

Files changed in this commit (implementation):

- New: `src/database/storage-engine-types.h`.
- Moved: `src/daemon/protected-access.{c,h}` -> `src/libnetdata/protected-access/protected-access.{c,h}`.
- Modified: `CMakeLists.txt`, `src/libnetdata/libnetdata.h`, `src/daemon/signal-handler.c`, `src/daemon/pulse/pulse-db-dbengine-retention.c`, `src/database/storage-engine.h`, `src/database/rrddim-backfill.h`, `src/database/engine/rrdengine.h`, `src/database/engine/mrg.h`, `src/database/engine/cache.h`, plus 12 engine `.c` files.

## Validation

Acceptance criteria evidence:

- `src/database/storage-engine-types.h` exists with the expected closure (lines 6-77).
- `src/database/engine/rrdengine.h` no longer includes `../rrd.h`; it includes `../storage-engine-types.h`. Verified by `grep -n include src/database/engine/rrdengine.h`.
- `src/database/storage-engine.h` includes `storage-engine-types.h`. Verified by inspection.
- `daemon/protected-access.h` no longer exists under `src/daemon/`; new path `src/libnetdata/protected-access/protected-access.h` is reachable via the libnetdata include path. Verified by `ls src/libnetdata/protected-access/` and the lack of `daemon/protected-access` references in the build.
- Full netdata build succeeds with no new diagnostics newly suppressed: `just cfg && just ninja` finished at `[189/189] Linking CXX executable netdata`. All prior warnings (e.g. `clean_directory`, `detect_ca_path`, `aral_used_total_size`, `epdl_get_cmd` unused) are pre-existing and unrelated.

Tests or equivalent validation:

- `just cfg && just ninja` — full build of the netdata executable plus all auxiliary targets (apps.plugin, systemd-units.plugin, log2journal, netdatacli, network-viewer.plugin, systemd-cat-native, systemd-journal.plugin, cgroup-network, otel-plugin, netflow-plugin, schema wrappers, and the netdata daemon executable). Final stage: `[189/189] Linking CXX executable netdata`.
- Header isolation check: a stub TU `#include "database/engine/rrdengine.h"` preprocessed with `clang -E` and grepped for streaming/sqlite/aclk/contexts/web/health/rrdhost/rrdset/rrddim headers — zero hits. Top-level `src/database/*.h` headers pulled in: zero. Total distinct headers transitively included: 142 (libnetdata + system + libuv + Judy + openssl + dbengine internals; no daemon graph).

Real-use evidence:

- Build passing on Linux + clang + Ninja with the project's `just cfg` debug configuration covering NETDATA_INTERNAL_CHECKS=1.
- The change is purely header reorganization; it cannot affect runtime semantics because no struct layouts, no macros, and no function bodies were modified. Runtime smoke-testing was therefore not separately performed.

Reviewer findings:

- None yet — awaiting PR.

Same-failure scan:

- `grep -rn '"daemon/protected-access\.h"' src/` -> no matches.
- `grep -rn '#include "../rrd\.h"' src/database/engine/*.h` -> no matches (only the .c files include it, intentionally).
- `grep -rn 'RRD_BACKFILL_NONE\|RRD_BACKFILL_NEW\|RRD_BACKFILL_FULL' src/` continues to compile cleanly (the enum is now in storage-engine-types.h, reachable via the same transitive include path).

Sensitive data gate:

- No secrets, credentials, customer data, private endpoints, or proprietary incident details in any artifact. The SOW, spec, and commit messages describe header reorganization only.

Artifact maintenance gate:

- AGENTS.md: no update (no workflow or guardrail change).
- Runtime project skills: no update (no developer-facing workflow change).
- Specs: `[[dbengine-library]]` already documents the header surface; this SOW realizes it. No new spec content required at this stage — the spec was authored as the durable target of the SOW series and remains current.
- End-user/operator docs: no update (no behavior change visible to users).
- End-user/operator skills: no update.
- SOW lifecycle: SOW-0018 moves from `current/` to `done/` with `Status: completed` in the same commit as the implementation. Planning artifacts (spec + 7 other pending SOWs) landed in the prior commit on the same branch.

Specs update:

- `[[dbengine-library]]` was authored alongside this SOW (prior commit on the branch) and already captures the target post-extraction contract. The carved header surface delivered here matches the spec's "Public C API" section; no edits needed.

Project skills update:

- No runtime project skill needed an update — the change does not alter how contributors interact with the engine source tree.

End-user/operator docs update:

- No update needed. The change is internal to the source tree and has no observable behavior, configuration, or interface impact.

End-user/operator skills update:

- No update needed for the same reason.

Lessons:

- Header decoupling exposes hidden dependencies in implementation files. Engine `.c` files were transitively grabbing pulse, libuv-workers, and sqlite-metadata symbols via `rrd.h`. The Phase-1 mitigation (explicit `#include "../rrd.h"` in `.c` files) keeps the build green while SOWs 0020-0022 progressively route those facilities through the library config and pulse callbacks.
- `protected-access` is a libnetdata-shaped utility (signal-safe TLS plus a sigsetjmp/siglongjmp dance, no daemon dependencies). Moving it to libnetdata cleaned both the engine and the daemon signal-handler include sites in one step.
- The clangd "is not used directly" warnings on umbrella headers are noise; they fire on intentional re-exposing includes.

Follow-up mapping:

- `[[dbengine-drop-rrddim-from-public-api]]` (SOW-0019) — removes the last `RRDDIM*` from the public API; the forward declaration in `storage-engine-types.h` survives until then.
- `[[dbengine-remove-nd-profile-localhost-reads]]` (SOW-0020) — eliminates the inline `nd_profile` / `localhost` reads that today still force engine `.c` files to include `../rrd.h`.
- `[[dbengine-library-config-struct]]` (SOW-0021) — replaces the forward-declared `netdata_conf_cpus()` in `cache.h` with a library-config field.
- `[[dbengine-pulse-callbacks-indirection]]` (SOW-0022) — replaces pulse direct calls in engine `.c` files with callbacks on the library config.

## Outcome

Completed. The dbengine header surface is now decoupled from the kitchen-sink `database/rrd.h`. External consumers of `rrdengine.h` get a 142-header closure containing only libnetdata, system, libuv, Judy, openssl, and dbengine internals — no streaming/sqlite/ACLK/contexts/web/health/RRDHOST coupling. `protected-access` now lives under libnetdata. The full netdata build passes. The remaining `.c`-side daemon dependencies are intentional Phase-1 placeholders to be progressively retired by SOWs 0020-0022.

## Lessons Extracted

See Validation > Lessons.

## Followup

See Validation > Follow-up mapping.

## Regression Log

None yet.
