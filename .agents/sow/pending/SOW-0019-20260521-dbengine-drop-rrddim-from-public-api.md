# SOW-0019 - Drop RRDDIM* from the dbengine public API

## Status

Status: open

Sub-state: drafted as the second SOW in the dbengine-library extraction series.

## Requirements

### Purpose

Remove the last daemon-typed parameter (`RRDDIM*`) from the dbengine public API so that the library can be consumed without including any RRDDIM / RRDSET / RRDHOST types. This unblocks Rust bindings ([[dbengine-rust-bindings-scaffold]]) and aligns the API with the library contract in [[dbengine-library]].

### User Request

User asked for a clean library boundary suitable for Rust binding and for standalone testing. The investigation identified `rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si)` as the single public function that pierces the boundary.

### Assistant Understanding

Facts:

- `src/database/engine/rrdengineapi.h:36` declares `rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si)`.
- `src/database/engine/rrdengineapi.c:172-205` implements it; the body reads `rd->uuid` (always) and `rd->rrdset` (only in the unittest branch via `rrdset_id(rd->rrdset)`).
- The storage-engine vtable in `src/database/storage-engine.c:63` registers this function as `metric_get_or_create`. The vtable also exposes `metric_get_by_uuid` and `metric_get_by_id` variants that already take only UUIDs.
- `rrdeng_metric_create(STORAGE_INSTANCE *si, nd_uuid_t *uuid)` (rrdengineapi.c:155) already does the actual creation without RRDDIM.
- The unittest branch calls `rrdeng_metric_unittest(si, rrddim_id(rd), rrdset_id(rd->rrdset))` which is a test-only helper.

Inferences:

- The RRDDIM-shaped public API exists for ergonomic reasons (callers pass `rd` and the engine extracts `rd->uuid`), not for any algorithmic reason.
- The unittest branch can be moved to the vtable wrapper layer (or to the dbengine-tests target after [[dbengine-move-tests-out-of-library]] lands).

Unknowns:

- Whether any non-vtable caller passes `RRDDIM*` directly. Quick grep at activation will confirm.

### Acceptance Criteria

- New API: `STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create_by_uuid_id(STORAGE_INSTANCE *si, UUIDMAP_ID id)` (or equivalent name) exposed in `rrdengineapi.h`.
- The old `rrdeng_metric_get_or_create(RRDDIM*, …)` is either removed entirely or remains only as a thin wrapper outside the engine.
- The RRDDIM-aware wrapping (uuid lookup, unittest branch, mismatch assertion) moves to `src/database/storage-engine.c`.
- `grep -rn 'RRDDIM\|rrdset_\|rrddim_' src/database/engine/*.c src/database/engine/*.h` shows zero matches outside the test files.
- Full netdata build succeeds; storage-engine vtable still dispatches correctly.

## Analysis

Sources checked:

- `src/database/engine/rrdengineapi.h`
- `src/database/engine/rrdengineapi.c` lines 155-205
- `src/database/storage-engine.c`
- `src/database/storage-engine.h`

Current state:

- The vtable already has separate slots for `metric_get_or_create` (RRDDIM-aware), `metric_get_by_uuid`, and `metric_get_by_id`. The engine itself implements all three.

Risks:

- Low. Moving the wrapper out of the engine is a mechanical refactor; the unittest branch is reachable only when the global `unittest_running` is set, so the wrapper-side translation needs to keep that guard.
- The `mrg_metric_ctx(metric) != ctx` invariant check in `NETDATA_INTERNAL_CHECKS` mode currently fires inside the engine; it can remain inside the engine (it does not need RRDDIM).

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- A single public function takes a daemon type for ergonomic convenience. The same work can be done at the vtable wrapper.

Evidence reviewed:

- See Facts above with file:line cites.

Affected contracts and surfaces:

- Public API of dbengine.
- The storage-engine vtable.

Existing patterns to reuse:

- `rrdeng_metric_get_by_uuid` and `rrdeng_metric_get_by_id` already exist as UUID-only entry points.

Risk and blast radius:

- Low. The wrapper translation is small; no behavior change.

Sensitive data handling plan:

- N/A — no secrets touched.

Implementation plan:

1. Add the new `_by_uuid_id` (or chosen name) entry to `rrdengineapi.h` and implement it in `rrdengineapi.c` using the existing `rrdeng_metric_create` and `mrg_metric_get_and_acquire_by_id`.
2. In `storage-engine.c`, change the `metric_get_or_create` vtable slot to use a new wrapper function that takes `RRDDIM*`, extracts the uuid, optionally runs the unittest branch, and calls the new engine entry.
3. Remove the old engine signature (or downgrade it to a wrapper that lives in `storage-engine.c`).
4. Move `rrdeng_metric_unittest` out of `rrdengineapi.c` and into `storage-engine.c` (or into the future dbengine-tests target — decide at activation).
5. Grep-confirm zero `RRDDIM`/`rrdset_`/`rrddim_` matches inside `src/database/engine/`.
6. Full build + run the existing dbengine unit/stress tests.

Validation plan:

- Existing `dbengine-unittest` continues to pass.
- `grep` confirmation from acceptance criteria.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to mark the no-RRDDIM-in-API guarantee as landed.
- End-user/operator docs: no update.
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Final name for the new entry. Candidates: `rrdeng_metric_get_or_create_by_uuid_id`, `rrdeng_metric_acquire_by_uuid_id`. To be picked at activation.

## Plan

1. Add new engine entry.
2. Move wrapper to vtable layer.
3. Drop the old signature from the engine.
4. Build + test.

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
