# SOW-0020 - Remove inline nd_profile and localhost reads from dbengine

## Status

Status: open

Sub-state: drafted as the third SOW in the dbengine-library extraction series.

## Requirements

### Purpose

Remove the two non-config global reads that prevent dbengine from being a self-contained library: `nd_profile.*` (consumed inline at 6 sites for tier count and update_every) and `localhost->db[tier].eng` (consumed at 2 sites for "is dbengine wired in"). Pre-requisite for [[dbengine-library-config-struct]] and for [[dbengine-libdbengine-cmake-target]] producing a standalone artifact.

### User Request

User identified inline configuration reads as a key blocker. This SOW addresses the non-extern-knob portion of that problem (the daemon-state reads); SOW-0021 addresses the extern-knob portion.

### Assistant Understanding

Facts:

- `nd_profile.storage_tiers` is read at:
  - `src/database/engine/journalfile.c:308`
  - `src/database/engine/rrdengine.c:421, 2230, 2370, 2384`
- `nd_profile.update_every` is read at:
  - `src/database/engine/pagecache.c:226, 399`
  - `src/database/engine/rrdengineapi.c:787, 788, 1332, 1418`
- `localhost` is dereferenced at:
  - `src/database/engine/rrdengine.c:2225-2235` (retention timer callback)
  - `src/database/engine/rrdengine.c:2366-2387` (`rrdeng_calculate_tier_disk_space_percentage`)
- `nd_profile` is defined in `src/daemon/config/netdata-conf-profile.h:50-61` as a process-wide struct.
- `localhost` is defined in `src/database/rrdhost.{c,h}` and is the global default RRDHOST pointer.
- The retention timer at rrdengine.c:2228 calls `worker_is_busy(RRDENG_RETENTION_TIMER_CB)`; the `if (!localhost) return;` guard is an "engine not yet wired" check.
- `tier_grouping` is computed in `netdata-conf-db.c:225-240` into `storage_tiers_grouping_iterations[]` and consumed at `rrdengineapi.c:1332, 1418` indirectly via `get_tier_grouping(tier)`.

Inferences:

- `nd_profile.update_every` is fundamentally a per-ctx value (each ctx has its own update_every for tier 0 and a derived one for higher tiers). It can live on `ctx->config.update_every_s` set at `rrdeng_init` time.
- `nd_profile.storage_tiers` is a process-wide value used for "how many tiers are active". It can live as a library-level field set by `dbengine_library_init()` (introduced in SOW-0021).
- The two `localhost`-dereferencing sites are both "is the engine initialized for this tier?" checks. They can be replaced by inspecting the engine's own per-tier initialization flag (`multidb_ctx[tier]` non-null + `multidb_ctx[tier]->config._internal.enabled`).
- The retention timer callback runs on the dbengine event loop thread; it only needs to know "do something for each active ctx", which the engine can answer itself.

Unknowns:

- Whether `tier_grouping[]` should live on `ctx->config` (array of one element per ctx) or as a library-level array. Will decide at activation; per-ctx is cleaner and aligns with how `tier` itself lives on ctx.
- Whether SOW-0020 should land before or after SOW-0021. Working hypothesis: 0020 first (it adds per-ctx fields and removes external reads); 0021 then introduces the library config struct that wires them in. Cleaner if 0020 lands first so 0021 is a pure config-wrangling change.

### Acceptance Criteria

- Zero references to `nd_profile` inside `src/database/engine/*.c` (excluding the test files moved out by [[dbengine-move-tests-out-of-library]]).
- Zero references to `localhost` inside `src/database/engine/*.c` (same exclusion).
- New `ctx->config.update_every_s` field set at `rrdeng_init`; new `tier_grouping` plumbing (either per-ctx or library-level).
- New library-internal "tiers active" registry replacing the `localhost->db[tier].eng` checks.
- Retention timer continues to call rotation for every active ctx; existing daemon-driven `rrdeng_calculate_tier_disk_space_percentage()` behavior unchanged.
- Full netdata build succeeds; an end-to-end run with multiple tiers shows identical retention/rotation behavior.

## Analysis

Sources checked:

- `src/database/engine/journalfile.c`, `rrdengine.c`, `rrdengineapi.c`, `pagecache.c`
- `src/daemon/config/netdata-conf-profile.{c,h}`
- `src/daemon/config/netdata-conf-db.c`
- `src/database/rrdhost.{c,h}`

Current state:

- All 8 sites are reachable on hot paths (query prep, store, retention, rotation). Replacement values must be correctly initialized before the first use.

Risks:

- Read-during-init: if `rrdeng_init` runs before `dbengine_library_init`, the new library-level "tiers active" value may be wrong. Mitigation: library-level init is the first call from the daemon; `rrdeng_init` already requires `rrdeng_dbengine_spawn` to have run. Sequence is well-defined.
- Behavioral regression on `update_every`: today `nd_profile.update_every` is the daemon default, applied universally. If we plumb it per-ctx, the value at `rrdeng_init` must match what was previously the daemon default. The daemon passes it once at init time; tests construct it explicitly.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The engine reads daemon state (`nd_profile.*`, `localhost`) inline because there is no library-level configuration channel. The fix is to introduce per-ctx fields and an engine-owned "active tiers" registry.

Evidence reviewed:

- 8 read sites listed in Facts.
- Daemon initialization order in `netdata-conf-db.c` and `rrdhost.c:250`.

Affected contracts and surfaces:

- `struct rrdengine_instance::config` gains `update_every_s` and `tier_grouping` (or a library-level alternative).
- `rrdeng_init` signature gains parameters (or, preferred: extended via `dbengine_library_config_t` finalized in [[dbengine-library-config-struct]]).
- Retention timer callback internal: no external observable change.

Existing patterns to reuse:

- `ctx->config.tier`, `ctx->config.max_disk_space`, `ctx->config.max_retention_s` already live on `ctx`. New fields slot in alongside.

Risk and blast radius:

- Moderate. Hot-path reads on store/query. Wrong initialization could degrade silently (e.g. a per-ctx `update_every_s` of 0 could trigger interpolation bugs). Mitigation: keep the existing daemon-driven default values; the SOW changes the *source* of the value, not the value itself.

Sensitive data handling plan:

- N/A — no secrets.

Implementation plan:

1. Add `update_every_s` to `ctx->config`; populate it in `rrdeng_init` from the value the daemon passes (initially via an extra parameter; SOW-0021 collects parameters into the library config).
2. Add `tier_grouping` plumbing — decide per-ctx vs library-level at activation.
3. Add a library-internal "active tiers" registry (`bool dbengine_tier_active[RRD_STORAGE_TIERS]` or equivalent). Set on successful `rrdeng_init`, clear on `rrdeng_exit`.
4. Rewrite the 8 read sites to use the new sources.
5. Replace `localhost->db[tier].eng != STORAGE_ENGINE_BACKEND_DBENGINE` checks with `dbengine_tier_active[tier]`.
6. Compile + run multi-tier startup and observe retention + rotation logs unchanged.

Validation plan:

- Multi-tier startup (storage tiers = 3) with non-default `update_every` produces correct first-page timestamps. Compare to a pre-SOW build on the same dbfiles.
- Retention rotation fires per active tier at the expected cadence.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to reflect that `update_every_s` and the active-tier registry are now library-owned.
- End-user/operator docs: no update.
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Per-ctx vs library-level `tier_grouping`. To decide at activation.

## Plan

1. Per-ctx fields + active-tier registry.
2. Rewrite reads.
3. Validate multi-tier behavior.

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
