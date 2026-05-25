# SOW-0021 - Consolidate dbengine extern knobs into a library-config struct

## Status

Status: open

Sub-state: drafted; depends on [[dbengine-remove-nd-profile-localhost-reads]].

## Requirements

### Purpose

Fold the 13 loose `extern` knobs that dbengine reads at runtime into a single `dbengine_library_config_t` consumed by a new `dbengine_library_init()`. This makes the library's process-wide configuration explicit, eliminates inline daemon-state reads, and is a prerequisite for [[dbengine-pulse-callbacks-indirection]] and [[dbengine-libdbengine-cmake-target]].

### User Request

User explicitly identified inline configuration reads as a major blocker for the library extraction. This SOW addresses the extern-knob portion (SOW-0020 addresses the daemon-state portion).

### Assistant Understanding

Facts:

- The following externs live in dbengine and are set by `src/daemon/config/netdata-conf-db.c`:
  - `dbengine_out_of_memory_protection` (rrdengineapi.c:62)
  - `dbengine_use_all_ram_for_caches` (rrdengineapi.c:63)
  - `db_engine_journal_check` (rrdengineapi.c:64)
  - `new_dbengine_defaults` (rrdengineapi.c:65)
  - `legacy_multihost_db_space` (rrdengineapi.c:66)
  - `default_rrdeng_disk_quota_mb` (rrdengineapi.c:67)
  - `default_multidb_disk_quota_mb` (rrdengineapi.c:68)
  - `default_backfill` (rrdengineapi.c:69)
  - `default_rrdeng_page_cache_mb` (rrdengineapi.c:72 or 75)
  - `default_rrdeng_extent_cache_mb` (rrdengineapi.c:73 or 76)
  - `rrdeng_pages_per_extent` (rrdengine.h:25)
  - `dbengine_use_direct_io` (declared in netdata-conf-db.h:9, read at datafile.c:285,369)
  - `dbengine_journal_v2_unmount_time` (declared in netdata-conf-db.c:18)
- `nd_profile.dbengine_journal_v2_unmount_time` (netdata-conf-profile.h:56) feeds the per-profile default that the daemon then copies into the extern.
- `src/daemon/config/netdata-conf-db.c` is the producer of all these values; it reads them via `inicfg_get_*` and sets the externs.
- `rrdeng_init` at rrdengineapi.c:1156 takes (path, disk_space_mb, tier, max_retention_s). These are per-ctx; they remain on `rrdeng_init` (or move onto a per-ctx config sub-struct).

Inferences:

- A struct-shaped config has three benefits: (a) one explicit init point instead of a write-many globals dance, (b) easier to construct from Rust / tests, (c) the daemon's responsibility for configuration becomes a single function call, easier to audit.
- The new `dbengine_library_config_t` lives in a new public header `src/database/engine/dbengine-library.h` so consumers don't need to include `rrdengine.h`.
- `dbengine_library_init()` should be idempotent and idempotent-safe: calling it twice with the same config is a no-op; calling it with a different config returns an error.

Unknowns:

- Whether `backfill` should be process-wide or per-ctx. Reading the code: `default_backfill` is a process-wide default applied to all newly-collected metrics. Per-ctx makes sense future-proofing; process-wide reflects today's reality. Will pick process-wide for Phase 1 and move to per-ctx later if needed.

### Acceptance Criteria

- New header `src/database/engine/dbengine-library.h` exposes `dbengine_library_config_t` and `dbengine_library_init()` / `dbengine_library_shutdown()`.
- The 13 externs listed above are removed from the public header surface (they become file-static inside the engine after init copies them into engine-internal storage, or they're collapsed into a single internal struct).
- `src/daemon/config/netdata-conf-db.c` is rewritten to populate the config struct from `inicfg_*` reads and call `dbengine_library_init(&cfg)` exactly once.
- `rrdeng_init(NULL, …)` and `rrdeng_init(&ctxp, …)` continue to work; their per-ctx-specific parameters are preserved.
- No behavior change observable to end users: identical defaults, identical `netdata.conf` keys honored.
- Full netdata build succeeds; existing dbengine unit/stress tests pass.

## Analysis

Sources checked:

- `src/database/engine/rrdengineapi.{c,h}`
- `src/database/engine/rrdengine.h`
- `src/database/engine/datafile.c`
- `src/daemon/config/netdata-conf-db.{c,h}`
- `src/daemon/config/netdata-conf-profile.{c,h}`

Current state:

- The externs are written exactly once (at config read time) and then read on hot paths. They form a closed contract between daemon config and engine internals.

Risks:

- Initialization order: `dbengine_library_init()` must run before any `rrdeng_init()`. The daemon already calls `netdata_conf_db_init()` then `rrdhost_create()` (which calls `rrdeng_init`); the new sequence is `netdata_conf_db_init()` then `dbengine_library_init(&cfg)` then `rrdeng_init()`.
- Stale value bugs if a test forgets to call `dbengine_library_init()`. Mitigation: assert-or-fatal in `rrdeng_init` if library not yet initialized.
- Renamed defaults: must be careful that `RRDENG_MIN_PAGE_CACHE_SIZE_MB`, `RRDENG_MIN_DISK_SPACE_MB`, `RRDENG_DEFAULT_TIER_DISK_SPACE_MB` (rrdengineapi.h:8-10) remain the documented defaults.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- 13 loose externs is a fragile contract that requires consumers to know the right write order. Replace with an explicit struct passed once.

Evidence reviewed:

- All 13 extern declarations and the daemon-side reads/writes in netdata-conf-db.c.
- Initialization order in daemon startup (`main.c` -> `netdata_conf_*` -> `rrd_init` -> `rrdhost_create` -> `rrdeng_init`).

Affected contracts and surfaces:

- New public header `dbengine-library.h`.
- `src/daemon/config/netdata-conf-db.c` (becomes the producer of the config struct).
- Removal of 13 externs from public headers.
- The library contract spec at `.agents/sow/specs/dbengine-library.md` finalizes the config-struct shape here.

Existing patterns to reuse:

- `nd_profile_t` (netdata-conf-profile.h:50-61) demonstrates a config-struct pattern used elsewhere in the daemon.
- `TIER_CONFIG_PROTOTYPE` (rrdengine.h:395) already exists as a per-ctx config sub-struct.

Risk and blast radius:

- Moderate. Touches every site reading these externs (small set, ~20 sites total). Hot-path reads stay equally fast (we copy the values into engine-internal storage at init).

Sensitive data handling plan:

- N/A — no secrets.

Implementation plan:

1. Define `dbengine_library_config_t` in a new `src/database/engine/dbengine-library.h`. Include only POD fields + (in SOW-0022) optional function-pointer callbacks; null callbacks here are placeholders until that SOW lands.
2. Define internal storage in `src/database/engine/rrdengineapi.c` (or a new `dbengine-library.c`) that the engine reads from. The 13 externs become file-static.
3. Implement `dbengine_library_init(&cfg)` that copies the config into engine storage with idempotence guard.
4. Implement `dbengine_library_shutdown()` matching the existing `dbengine_shutdown()` (or rename and re-purpose the existing function).
5. Rewrite `src/daemon/config/netdata-conf-db.c` to construct the config struct and call `dbengine_library_init()`. Remove the direct extern writes.
6. Add a fatal-on-uninit assertion at the top of `rrdeng_init`.
7. Update the library spec to reflect the final config-struct shape.
8. Full build + run a 3-tier startup with custom values for `default_rrdeng_page_cache_mb`, `dbengine_use_direct_io`, `rrdeng_pages_per_extent`; observe identical behavior.

Validation plan:

- `diff` of pre-SOW vs post-SOW config dump (`/api/v1/info` or equivalent that exposes dbengine config) is empty.
- Existing dbengine unit/stress tests pass.
- Manual: edit a value in `netdata.conf` (e.g. `dbengine page cache size`), restart, confirm the value reaches the engine.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to mark the configuration model as landed and to record the final field list.
- End-user/operator docs: no update (config keys unchanged, defaults unchanged).
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Whether to consolidate `default_rrdeng_disk_quota_mb` and `default_multidb_disk_quota_mb` (currently both default to the same value). Likely keep both for compatibility.

## Plan

1. Header + config struct.
2. Engine internalization of values.
3. `dbengine_library_init()` + assertion.
4. Daemon-side producer rewrite.
5. Validation.

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
