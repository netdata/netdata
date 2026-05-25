# SOW-0024 - Build dbengine as a standalone libdbengine CMake target

## Status

Status: open

Sub-state: drafted; depends on [[dbengine-storage-engine-types-header]], [[dbengine-drop-rrddim-from-public-api]], [[dbengine-remove-nd-profile-localhost-reads]], [[dbengine-library-config-struct]], [[dbengine-pulse-callbacks-indirection]], [[dbengine-move-tests-out-of-library]]. This is the keystone SOW that makes the library extraction visible at the build level.

## Requirements

### Purpose

Produce a real `dbengine` CMake target (static, with an opt-in shared variant) and have the `netdata` executable consume it via `target_link_libraries(netdata PRIVATE dbengine)` instead of compiling the engine sources directly. After this lands, the fact that dbengine is a separate component is evident in the build system, and [[dbengine-rust-bindings-scaffold]] can link against the artifact.

### User Request

User explicitly asked: "a separate standalone `.a` or `.so` library" so that the separation is evident in both the source code and the build system.

### Assistant Understanding

Facts:

- Today, `src/database/engine/*.{c,h,cc}` files are listed in `RRD_PLUGIN_FILES` in `CMakeLists.txt:1744-1775` (within an `ENABLE_DBENGINE` block) and compiled directly into the `netdata` executable at `CMakeLists.txt:2151`.
- `libnetdata` already exists as `add_library(libnetdata STATIC ${LIBNETDATA_FILES})` at `CMakeLists.txt:2343` and is consumed by `netdata` as a normal CMake dependency.
- The engine depends on libnetdata + libuv + lz4 + zstd + Judy + OpenSSL. All of these are already discovered via the existing top-level CMake `find_package` / `pkg_check_modules` calls.
- Once SOWs 0018-0022 land, the engine no longer pulls daemon-specific includes (rrd.h, pulse, localhost, nd_profile).
- Once SOW-0023 lands, the engine source list excludes test files.

Inferences:

- A `STATIC` library is the right default. A `BUILD_DBENGINE_SHARED` CMake option lets the Rust binding work consume a `.so` if useful, but is not required for Phase 1.
- `POSITION_INDEPENDENT_CODE ON` should be set so the static library is suitable for linking into a `.so` later.
- The `dbengine` target's `target_include_directories` need to expose the public headers (`storage-engine-types.h`, `dbengine-library.h`, `rrdengineapi.h`) without leaking the engine's internal sub-headers.

Unknowns:

- Whether the existing build matrix (Linux/Windows/macOS, native + cross) needs any per-platform adjustments. Likely no — libnetdata already crosses these platforms with the same pattern.

### Acceptance Criteria

- `add_library(dbengine STATIC ...)` exists in `CMakeLists.txt` and builds when `ENABLE_DBENGINE=ON`.
- The engine source files are removed from `RRD_PLUGIN_FILES`; `netdata` consumes them via `target_link_libraries(netdata PRIVATE dbengine)`.
- The library exposes its public headers via `target_include_directories(dbengine PUBLIC ...)` and hides internal headers behind `PRIVATE`.
- `POSITION_INDEPENDENT_CODE ON` set on the static target.
- An opt-in `BUILD_DBENGINE_SHARED` option produces a `.so` / `.dylib` / `.dll` variant.
- Building `netdata`, `netdatacli`, `systemd-cat-native`, `nd-run`, `spawn-tester`, and `dbengine-tests` all succeed.
- Building only the `dbengine` target via `cmake --build build --target dbengine` succeeds and produces `libdbengine.a`.
- The artifact is consumable from a minimal external C TU that includes the public headers, links libdbengine + libnetdata + dependencies, calls `dbengine_library_init` + `rrdeng_init` + `rrdeng_exit` + `dbengine_library_shutdown`, and exits 0.

## Analysis

Sources checked:

- `CMakeLists.txt` (full file, with focus on lines 1636-1776, 2143-2160, 2286, 2343, 3188).
- `packaging/cmake/` if present (CMake helper modules).

Current state:

- Build system already does the right thing for libnetdata; libdbengine follows the same template.

Risks:

- Linking order: a circular dependency between libdbengine and libnetdata could surface if libnetdata's `protected-access` (relocated in SOW-0018) ends up needing engine-specific symbols. Mitigation: keep `protected-access.h` purely a libnetdata utility.
- Distribution / packaging files (RPM spec, deb rules) may need updating if they install the artifact. Mitigation: grep packaging/ at activation.

## Pre-Implementation Gate

Status: blocked (waits on 0018-0023)

Problem / root-cause model:

- The build system compiles dbengine sources into the netdata executable. The fix is to introduce a real CMake library target. The prerequisite SOWs make the source list clean enough that this is purely a CMake change.

Evidence reviewed:

- `CMakeLists.txt` structure.
- The libnetdata target as a working pattern.

Affected contracts and surfaces:

- `CMakeLists.txt` (new target + link line on `netdata` and `dbengine-tests`).
- Possibly `packaging/` if any spec / rules files reference the dbengine sources.
- The library contract spec at `.agents/sow/specs/dbengine-library.md`.

Existing patterns to reuse:

- `add_library(libnetdata STATIC ...)` at CMakeLists.txt:2343 is the model.
- `add_library(judy STATIC ${LIBJUDY_SOURCES})` at CMakeLists.txt:2286 also.

Risk and blast radius:

- Moderate. Build-system surgery touches all build matrices. Mitigation: keep the change pure-additive (the engine sources still exist in their current paths; only the consumer changes).

Sensitive data handling plan:

- N/A — no secrets.

Implementation plan:

1. Add `add_library(dbengine STATIC ${DBENGINE_FILES})` inside the `ENABLE_DBENGINE` block.
2. Set `target_include_directories(dbengine PUBLIC src src/database)` (the exact set is determined by what the post-0018 public headers expect).
3. Set `target_link_libraries(dbengine PUBLIC libnetdata judy uv lz4 zstd OpenSSL::Crypto)` (exact target names match what `libnetdata` uses today).
4. Set `set_target_properties(dbengine PROPERTIES POSITION_INDEPENDENT_CODE ON)`.
5. Drop the engine source files from `RRD_PLUGIN_FILES` and add `target_link_libraries(netdata PRIVATE dbengine)`.
6. Drop the engine source files from `dbengine-tests` (introduced in SOW-0023) and add `target_link_libraries(dbengine-tests PRIVATE dbengine)`.
7. Add an `option(BUILD_DBENGINE_SHARED "..." OFF)` and a conditional `add_library(... SHARED ...)` for that path.
8. Grep `packaging/` for any references to engine source files; update if found.
9. Build the full default target matrix on Linux. Build the `BUILD_DBENGINE_SHARED=ON` variant separately and confirm the `.so` produces.
10. Build the minimal external consumer TU described in acceptance criteria; confirm it links and runs.

Validation plan:

- All default targets build.
- `BUILD_DBENGINE_SHARED=ON` builds a `.so`.
- Minimal external consumer TU links and runs.
- `nm libdbengine.a` shows no undefined references to `pulse_*`, `localhost`, `nd_profile`, `RRDDIM`, `rrdset_*`, `rrddim_*`.

Artifact impact plan:

- AGENTS.md: minor update mentioning the new build artifact.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to mark the packaging contract as landed.
- End-user/operator docs: no update (the daemon binary behavior is unchanged).
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Whether to gate `BUILD_DBENGINE_SHARED` on a separate option or fold it into a more general `BUILD_SHARED_LIBS` switch. Likely the explicit option is clearer in our matrix.

## Plan

1. Library target + transitively-needed include / link properties.
2. Consumers (`netdata`, `dbengine-tests`) switch to link-against.
3. Shared variant option.
4. Minimal external consumer link-test.

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
