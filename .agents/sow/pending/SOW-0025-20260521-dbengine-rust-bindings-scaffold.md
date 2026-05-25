# SOW-0025 - Rust bindings scaffold for dbengine

## Status

Status: open

Sub-state: drafted; depends on [[dbengine-libdbengine-cmake-target]] (a real `libdbengine.a` must exist before the Rust side can link).

## Requirements

### Purpose

Expose the dbengine library to Rust via a `dbengine-sys` (raw bindgen) crate plus a thin safe wrapper crate `dbengine`, both under `src/crates/`. This lands the testing vehicle the user asked for: a Rust harness that can spin up dbengine instances with arbitrary configuration and run focused tests against the storage layer.

### User Request

User explicit: "We want to be able to create Rust bindings (for dbengine) easily. This will allow us to leverage Rust for doing all kind of tests."

### Assistant Understanding

Facts:

- `src/crates/` is the existing home for Rust crates in this repository (e.g. wal, sfst, docs/catalog plans per project memory).
- Once SOWs 0018-0024 land, the public C header surface is three files: `database/storage-engine-types.h`, `database/engine/dbengine-library.h`, `database/engine/rrdengineapi.h`. They depend only on libnetdata + libuv + Judy + lz4 + zstd + OpenSSL.
- `bindgen` 0.69+ handles the patterns used in these headers (typed enums, structs, function pointers, opaque types).
- `cargo:rustc-link-lib=static=dbengine` together with the matching `cargo:rustc-link-search=...` lets a Rust crate link against the artifact produced by [[dbengine-libdbengine-cmake-target]].

Inferences:

- `dbengine-sys` should be a thin `bindgen`-generated wrapper with no value-add beyond exposing the C symbols. Build script invokes CMake to produce libdbengine then runs bindgen.
- `dbengine` (safe wrapper) provides RAII guards for library + instance + metric + collector + query handle lifecycles, matching the C lifecycle described in `.agents/sow/specs/dbengine-library.md`.
- A smoke test in `dbengine` proves end-to-end: init library, open instance, store a few points, read them back, exit. This is the user's "do all kind of tests" foothold.
- The Rust side does not need to expose async — the dbengine event loop is process-wide and synchronous from the caller's perspective (blocking completions). Async wrappers are a future concern.

Unknowns:

- Whether the CMake-from-Rust pattern this repo prefers is `cmake` crate vs `cc`-based manual invocation. Likely `cmake` crate based on the existing crate set; will verify at activation.
- Whether the Rust binding should ship `libnetdata` symbols too. Likely yes — bindgen needs the libnetdata headers reachable, and the static link must pull in libnetdata.a. Same applies to libuv, Judy, etc. Build script will use `pkg-config` for the system libs.

### Acceptance Criteria

- `src/crates/dbengine-sys/` exists with a build.rs that builds libdbengine via CMake, runs bindgen, and emits the link flags.
- `src/crates/dbengine/` exists with RAII wrappers and a smoke test (`cargo test`) that:
  1. Constructs a `dbengine_library_config_t` with safe defaults.
  2. Calls `dbengine_library_init`.
  3. Opens an instance in a temporary directory.
  4. Stores N points for M synthetic metrics.
  5. Reads them back and asserts values match.
  6. Exits cleanly (`rrdeng_exit`, `dbengine_library_shutdown`).
- The smoke test runs in CI on Linux (Windows/macOS as follow-ups if useful).
- `cargo doc -p dbengine` produces useful documentation for the wrapper API.

## Analysis

Sources checked:

- `src/crates/` existing layout (cargo workspaces, build patterns).
- Existing crates like `wal` for the CMake-and-bindgen pattern (per project memory).
- `.agents/sow/specs/dbengine-library.md` for the C contract the Rust side wraps.

Current state:

- The repo already builds Rust crates as part of the broader build. The pattern for linking against a CMake-built static library is established.

Risks:

- Library initialization is process-wide; concurrent `cargo test` runs in the same process (e.g. `cargo test -- --test-threads=1` vs parallel) need careful sequencing. Mitigation: `Library` wrapper uses `Once` + per-test isolation via unique `dbfiles_path` directories.
- The Phase 1 library has shared caches/MRG/event loop. Multiple `Instance` objects share these. Tests need to be designed accordingly (one test per process, or accept shared state). Mitigation: document this clearly; provide test helpers that produce fresh directories per test.
- `bindgen` over `libnetdata` headers may pull in symbols we don't want exposed. Mitigation: allowlist the dbengine public surface via `bindgen::Builder::allowlist_function` / `allowlist_type` patterns.

## Pre-Implementation Gate

Status: blocked (waits on 0024)

Problem / root-cause model:

- No Rust bindings exist for dbengine because the C surface wasn't suitable. The prior SOWs make it suitable. Now we generate the bindings.

Evidence reviewed:

- The C public header inventory after SOWs 0018-0021.
- Existing Rust crate build patterns in `src/crates/`.
- Library contract spec.

Affected contracts and surfaces:

- New `src/crates/dbengine-sys/` and `src/crates/dbengine/` crates.
- Possibly the top-level `Cargo.toml` / workspace listing.
- The library spec gets a final note that Rust bindings are landed.

Existing patterns to reuse:

- Existing `wal`, `sfst` (or whichever Rust crates currently link against C libraries built by CMake) demonstrate the build.rs + cmake-crate + bindgen pattern.

Risk and blast radius:

- Low for the daemon (Rust crate addition is additive). Test infrastructure risk is limited to the Rust crate itself.

Sensitive data handling plan:

- N/A — no secrets.

Implementation plan:

1. At activation, audit existing `src/crates/*/build.rs` to pick the CMake-from-Rust pattern this repo prefers.
2. Scaffold `src/crates/dbengine-sys/` with `Cargo.toml`, `src/lib.rs` (re-export bindgen output), and `build.rs` that:
   - Builds the `dbengine` CMake target out-of-source.
   - Runs bindgen on the three public headers + the leaf libnetdata headers needed by bindgen.
   - Emits `cargo:rustc-link-lib=static=dbengine` etc.
3. Scaffold `src/crates/dbengine/` with the RAII wrappers and the smoke test.
4. Add the new crates to the workspace `Cargo.toml`.
5. Run `cargo test -p dbengine` locally; iterate until the smoke test passes.
6. Add a CI job (Linux first) that runs `cargo test -p dbengine` after the CMake build.

Validation plan:

- `cargo test -p dbengine` passes locally and in CI.
- A second test that opens multiple instances in different temp directories (sharing the process-wide event loop / caches by design) also passes — this validates the multi-instance story under the Phase 1 constraint.

Artifact impact plan:

- AGENTS.md: minor update noting the Rust crates for dbengine tests.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to mark the Rust binding section as landed and to record the exact crate names.
- End-user/operator docs: no update.
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Whether the safe wrapper exposes `unsafe fn` escape hatches at all (for tests that need to poke at internals). Likely yes — gate behind a `cfg(feature = "internal")` flag.

## Plan

1. Audit existing crate build pattern.
2. dbengine-sys scaffold with build.rs.
3. dbengine safe wrapper + smoke test.
4. CI integration.

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
