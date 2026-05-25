# SOW-0022 - Indirect dbengine pulse hooks through optional callbacks

## Status

Status: open

Sub-state: drafted; depends on [[dbengine-library-config-struct]] (callbacks live on `dbengine_library_config_t`).

## Requirements

### Purpose

Replace the ~15 direct calls to `pulse_aral_*` and `pulse_gorilla_*` inside dbengine with optional function-pointer callbacks supplied at library init time. This removes the last hard daemon dependency from the library so that [[dbengine-libdbengine-cmake-target]] can link without any pulse symbols, and so that test consumers (including Rust) can pass NULL callbacks for no-op behavior.

### User Request

Implicit: user asked for a clean library boundary suitable for standalone testing. The pulse subsystem lives under `src/daemon/pulse/` and is daemon-specific telemetry; the library should not require it.

### Assistant Understanding

Facts:

- Direct pulse calls inside the engine, by file:
  - `src/database/engine/mrg.c`: 2 sites (lines 22, 106) — `pulse_aral_register_statistics`, `pulse_aral_unregister_statistics`.
  - `src/database/engine/pdc.c`: 5 sites (lines 63, 92, 121, 151, 181) — `pulse_aral_register`.
  - `src/database/engine/page.c`: 3 sites (lines 332, 466, 936) — `pulse_aral_register_statistics`, `pulse_gorilla_hot_buffer_added` (x2). Also line 811: `pulse_gorilla_tier0_page_flush`.
  - `src/database/engine/cache.c`: 2 sites (lines 616, 2033) — `pulse_aral_register_statistics`, reads `pulse_enabled`.
  - `src/database/engine/rrdengine.c`: 5 sites (lines 186, 326, 353, 378, 498) — `pulse_aral_register`.
- The pulse subsystem lives at `src/daemon/pulse/` and is linked into the daemon. It is not currently a separate CMake target.
- `pulse_enabled` is a global flag read at cache.c:2033 (`cache->config.stats = pulse_enabled`).

Inferences:

- All pulse calls are fire-and-forget statistics registration / event-counting; they have no return value the engine acts on.
- Wrapping the calls as `if (callbacks.on_aral_register) callbacks.on_aral_register(...)` is the lightest possible change.
- The `pulse_enabled` read can be replaced with a boolean field on the library config (`cfg.collect_pulse_stats`).
- Adding a tiny header `src/database/engine/dbengine-callbacks.h` keeps the function-pointer typedefs together and out of the main library config struct.

Unknowns:

- Whether the gorilla callbacks need their full current signature preserved (yes — they pass payload sizes the daemon-side aggregator depends on).
- Whether a single grouped struct (`dbengine_callbacks_t`) embedded in `dbengine_library_config_t` is cleaner than a flat field list. Likely yes; will pick at activation.

### Acceptance Criteria

- `dbengine_library_config_t` exposes a `callbacks` sub-struct with all required hooks. Null callbacks are valid and produce no-op behavior.
- Zero references to `pulse_*` symbols inside `src/database/engine/*.{c,h}` (excluding the test files moved by [[dbengine-move-tests-out-of-library]]).
- Daemon supplies the existing pulse functions as callbacks at library init time. Production telemetry behavior is unchanged.
- A standalone test binary (or the future Rust harness) can construct and use the library with null callbacks, producing zero linker references to pulse symbols.

## Analysis

Sources checked:

- `src/database/engine/mrg.c`, `pdc.c`, `page.c`, `cache.c`, `rrdengine.c`
- `src/daemon/pulse/pulse-aral.{c,h}` (referenced by `pulse_aral_*`)
- `src/daemon/pulse/pulse-db-dbengine.{c,h}` (the daemon-side aggregator)

Current state:

- The engine calls into pulse directly; pulse is a separate set of files linked into the same monolithic netdata target. There is no link-level boundary today, but introducing one is purely additive.

Risks:

- Low. Function-pointer dispatch adds a single indirect call per registration site; these run at init or on cold paths (gorilla flush is the hottest, ~once per page completion).
- A missing callback wiring on the daemon side would silently break pulse stats. Mitigation: keep the daemon-side wiring as obvious as possible (one struct literal at the library-init call site).

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The library calls daemon-specific telemetry directly. The fix is well-known: indirection through caller-supplied function pointers.

Evidence reviewed:

- All ~15 pulse call sites enumerated above.

Affected contracts and surfaces:

- `dbengine_library_config_t` gains a `callbacks` sub-struct.
- The 15 call sites become null-guarded indirect calls.
- `src/daemon/config/netdata-conf-db.c` (or wherever `dbengine_library_init()` is called from after SOW-0021) supplies the function pointers.

Existing patterns to reuse:

- `aral_create()` already accepts a statistics struct that's published via a callback-like mechanism; the indirection pattern is already known here.

Risk and blast radius:

- Low. Hot-path overhead is one indirect call per gorilla-page-flush event (already a heavyweight event).

Sensitive data handling plan:

- N/A — no secrets.

Implementation plan:

1. Define `dbengine_callbacks_t` (in `dbengine-callbacks.h` or inline in `dbengine-library.h`).
2. Add a `callbacks` field to `dbengine_library_config_t`.
3. Store the callbacks in engine-internal storage at `dbengine_library_init()` time.
4. Replace each of the 15 call sites with a null-guarded indirect call.
5. Replace `pulse_enabled` read with the corresponding callback presence check (or a `cfg.collect_pulse_stats` boolean).
6. In `src/daemon/config/netdata-conf-db.c` (or the post-SOW-0021 init call site), supply the existing `pulse_*` functions as callbacks.
7. Build + run a standard daemon and confirm pulse charts populate identically to a pre-SOW build.
8. Build a tiny standalone "init dbengine with null callbacks, store a few points, exit" test program and confirm it links without any `pulse_*` symbol references.

Validation plan:

- Pulse chart `netdata.dbengine.*` continues to populate with identical values pre/post SOW.
- Standalone test binary links cleanly with null callbacks.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to enumerate the final callback list.
- End-user/operator docs: no update.
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Inline callbacks in `dbengine_library_config_t` vs a separate `dbengine_callbacks_t` sub-struct. To decide at activation; sub-struct is cleaner.

## Plan

1. Callback type definitions.
2. Engine-internal storage + null-guarded sites.
3. Daemon wiring.
4. Standalone link-test.

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
