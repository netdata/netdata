# SOW-0032 - Rust-on-Windows smoke crate

## Status

Status: in-progress

Sub-state: Pre-impl gate filled; user confirmed crate placement (standalone), CMake gating (ENABLE_RUST_DEMO default ON), and call site (next to NETDATA STARTUP log).

## Requirements

### Purpose

Verify that the MSYS2/UCRT64-based Windows build path can actually compile a
Rust staticlib and link it into the main `netdata` Agent binary. The Windows
build today never exercises Rust because every Rust-using plugin
(OTEL/journal/NetFlow/signal-viewer) is disabled in
`packaging/windows/compile-on-windows.sh`. We want a small, self-contained
Rust crate exposing a C FFI that gets linked into `netdata` and called during
agent startup so a draft PR can prove the Windows toolchain is wired.

### User Request

> The issue is that Windows relies on msys2, and the whole compilation process
> is a little bit complex. I want you to investigate the way we build the
> agent on Windows, and create a small Rust crate (eg. something that adds
> two numbers) that provides C FFI bindings. The C FFI bindings should be
> merged into the main agent process (eg. during initialization we could
> call the function and log a message). This will allow me to open a draft
> PR and check the Windows build.

### Assistant Understanding

Facts:

- `packaging/windows/compile-on-windows.sh` already sets
  `-DRust_COMPILER=/ucrt64/bin/rustc` but explicitly disables every existing
  Rust-using plugin (`-DENABLE_PLUGIN_OTEL=Off`,
  `-DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off`; OTEL signal viewer and NetFlow are
  not requested either).
- `CMakeLists.txt:245` only invokes `FetchContent_Declare(Corrosion ...)` and
  `corrosion_import_crate(...)` when one of those four plugins is enabled.
  Therefore Corrosion is never executed in the current Windows path.
- `packaging/windows/msys2-dependencies.sh` already installs
  `mingw-w64-ucrt-x86_64-rust` and `mingw-w64-ucrt-x86_64-toolchain`, so the
  Rust toolchain is available even though it is not used.
- Existing pattern for a Rust staticlib linked into a C target:
  `src/crates/jf/journal_reader_ffi` with `crate-type = ["staticlib"]`,
  cbindgen-generated `journal_reader_ffi.h` committed in tree,
  `corrosion_import_crate(...)` and `target_link_libraries(<target> <crate>)`.
- `libnetdata` is built with `${CMAKE_SOURCE_DIR}/src` as a PUBLIC include
  directory (`CMakeLists.txt:2387`), so any header at
  `src/crates/<crate>/<header>.h` is reachable via
  `#include "crates/<crate>/<header>.h"`.
- The big `src/crates/Cargo.toml` workspace pulls in heavy transitive
  dependencies (tokio, foyer, opentelemetry, etc.) that are not needed to
  test whether Rust can be built on Windows.

Inferences:

- A standalone single-crate manifest minimises Cargo compile time on Windows
  CI and isolates the smoke test from unrelated dependency breakage in the
  large workspace.
- Hand-writing the C header for one or two FFI functions is faster than
  pulling cbindgen as a build dependency for a smoke test. cbindgen would
  also drag in clap/log/etc. just to emit two lines of C declarations.

Unknowns:

- Whether the resulting static library needs extra Windows system libraries
  (e.g. `userenv`, `ws2_32`, `bcrypt`) to be linked into `netdata`. The
  Rust `std` crate-type=staticlib pulls these in automatically through
  the `.rlib` metadata when Corrosion drives `cargo build`; Corrosion already
  forwards these to the consumer. If linkage breaks we will add the missing
  libraries explicitly to the netdata target.

### Acceptance Criteria

- A new crate `src/crates/rust-demo` builds with `cargo build` in the
  standard host environment.
- `cmake --build` succeeds on Linux with `ENABLE_RUST_DEMO=ON` (default).
- `cmake --build` succeeds on Linux with `ENABLE_RUST_DEMO=OFF` (no Rust
  used at all — proves the option is non-mandatory).
- The Windows MSYS2/UCRT64 build, when triggered via the draft PR, exercises
  Corrosion and links the static library into `netdata.exe`.
- Starting `netdata` (Linux build, locally) emits a log line containing the
  result of the Rust FFI call right next to the existing `NETDATA STARTUP`
  log line.

## Analysis

Sources checked:

- `CMakeLists.txt` (Rust/Corrosion block at L244-L334; main `netdata` target
  at L3229-L3304; libnetdata include directory at L2387).
- `packaging/windows/compile-on-windows.sh`,
  `packaging/windows/msys2-dependencies.sh`.
- `src/crates/Cargo.toml`, `src/crates/jf/Cargo.toml`,
  `src/crates/jf/journal_reader_ffi/{Cargo.toml,build.rs,src/lib.rs}`,
  `src/collectors/systemd-journal.plugin/provider/rust_provider.h`.
- `src/daemon/main.c` `netdata_main()` (L261-) and the NETDATA STARTUP log
  line at L1202.

Current state:

- Windows build never invokes cargo/rustc despite installing the Rust
  toolchain and exporting `Rust_COMPILER`.
- Linux builds that enable any Rust-using plugin already pull Corrosion and
  build static Rust libraries successfully.

Risks:

- Compile-time cost: standalone crate has no runtime dependencies so the
  cost is essentially `rustc` plus libstd; negligible.
- Link-time symbol clashes: keep the FFI prefix `nd_rust_` to avoid clashing
  with libc or other libraries.
- Cargo network access during configure/build on Windows CI: this is the
  whole point of the smoke test. If the CI machine cannot reach crates.io
  the build will fail and we will know.
- Adds a new always-on CMake option. Set the default to ON and gate Rust
  presence behind it so projects can opt out if needed.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Rust is installed in the Windows MSYS2/UCRT64 environment but the CMake
  guard `if(ENABLE_NETDATA_JOURNAL_FILE_READER OR ENABLE_PLUGIN_OTEL OR
  ENABLE_PLUGIN_OTEL_SIGNAL_VIEWER OR ENABLE_PLUGIN_NETFLOW)` means
  Corrosion is never declared and no Rust source is ever compiled. As a
  result we have no signal whether the Windows toolchain can build Rust at
  all. The fix is to introduce a minimal Rust crate behind a new option
  `ENABLE_RUST_DEMO` that is included in the guard.

Evidence reviewed:

- See "Sources checked" above. Existing reference implementation is
  `src/crates/jf/journal_reader_ffi`.

Affected contracts and surfaces:

- New top-level CMake option `ENABLE_RUST_DEMO`.
- New crate `src/crates/rust-demo` (Cargo.toml, src/lib.rs, header).
- Patch in `src/daemon/main.c` adding `#include` and a single call near
  `NETDATA STARTUP`.
- Windows build script (`packaging/windows/compile-on-windows.sh`) — needs
  no change because the default is ON and there is no override; on Linux
  CI the option also defaults ON.

Existing patterns to reuse:

- `journal_reader_ffi` (FFI staticlib + Corrosion import + target_link).
- `CMakeLists.txt:245-334` Corrosion block (extend the gating condition,
  add another `corrosion_import_crate` call).

Risk and blast radius:

- Small and isolated. Default-ON adds Rust to all builds, but it is one
  tiny crate with no transitive dependencies and can be turned off via
  `-DENABLE_RUST_DEMO=OFF`.

Sensitive data handling plan:

- No sensitive data involved. The crate is a constant-folded integer
  adder and a static version string; the log message is plain text.

Implementation plan:

1. Add CMake option `ENABLE_RUST_DEMO` (default ON), extend the Corrosion
   `if(...)` guard, `corrosion_import_crate(MANIFEST_PATH
   src/crates/rust-demo/Cargo.toml CRATES rust_demo)` and
   `target_link_libraries(netdata PRIVATE rust_demo)` guarded by the option.
2. Create `src/crates/rust-demo/Cargo.toml` with `crate-type =
   ["staticlib"]` and no runtime dependencies.
3. Create `src/crates/rust-demo/src/lib.rs` exposing
   `int32_t nd_rust_add(int32_t, int32_t)` and
   `const char *nd_rust_version(void)`.
4. Hand-write `src/crates/rust-demo/rust_demo.h` declaring those two
   functions in a standard include guard.
5. Patch `src/daemon/main.c` to `#include "crates/rust-demo/rust_demo.h"`
   (guarded by `#ifdef HAVE_RUST_DEMO`) and emit one `netdata_log_info`
   line next to the NETDATA STARTUP message. Add
   `target_compile_definitions(netdata PRIVATE HAVE_RUST_DEMO)` in the
   CMake block.
6. Local validation on Linux: `cmake --build` and run `netdata -W buildinfo`
   or normal `netdata -D` long enough to see the startup log.

Validation plan:

- Local Linux: `cmake -B build -DENABLE_RUST_DEMO=ON && cmake --build build`
  with the smoke crate compiled and linked.
- Local Linux: `cmake -B build -DENABLE_RUST_DEMO=OFF && cmake --build
  build` to confirm option is honoured and Corrosion is not pulled in (if
  no other Rust plugin is enabled).
- Open a draft PR; rely on the existing Windows packaging workflow
  (`packaging.yml` / `build.yml`) for the actual Windows CI signal —
  this SOW is intentionally scoped so the user can verify on a draft PR.

Artifact impact plan:

- AGENTS.md: no update needed. SOW framework rules and conventions are
  unchanged.
- Runtime project skills: no update needed. No new workflow surface.
- Specs: no update needed. No public/contract behaviour added.
- End-user/operator docs: no update needed. The smoke crate is internal
  scaffolding and emits a single log line.
- End-user/operator skills: no update needed.
- SOW lifecycle: single SOW, will be moved to `done/` once the PR is
  opened and the local Linux build validates. (Windows CI is the user's
  responsibility on the draft PR.)

Open-source reference evidence:

- None used. All evidence is local to this repository.

Open decisions:

- None. All three user decisions have been recorded (standalone crate,
  ENABLE_RUST_DEMO default ON, call site next to NETDATA STARTUP).

## Implications And Decisions

1. Crate placement: standalone `src/crates/rust-demo/` (not a workspace
   member). Selected by user. Rationale: isolates the smoke test from
   the heavy `src/crates` workspace deps; the goal is to prove the
   toolchain works, not to prove the workspace builds on Windows.
2. CMake gating: `ENABLE_RUST_DEMO` default ON. Selected by user.
   Rationale: easy to opt out for bisecting, but ON by default so the
   smoke test runs everywhere.
3. Call site: next to `NETDATA STARTUP` log in `daemon/main.c`. Selected
   by user. Rationale: logger is fully initialised, message is visible
   in normal startup logs.

## Plan

1. CMake wiring — extend the Corrosion guard, register the crate, link
   into the `netdata` target, define `HAVE_RUST_DEMO`.
2. Rust crate — `Cargo.toml`, `src/lib.rs`, `rust_demo.h`.
3. C wiring — include the header in `daemon/main.c`, call FFI, log result.
4. Local Linux validation.
5. Commit and open draft PR (user action, per the no-push-without-instruction
   memory).

## Execution Log

### 2026-05-22

- Investigation complete. SOW filed.
- Implemented:
  - `src/crates/rust-demo/Cargo.toml` (own workspace root via empty
    `[workspace]`, `crate-type = ["staticlib"]`, no runtime deps).
  - `src/crates/rust-demo/src/lib.rs` exposing `nd_rust_add` and
    `nd_rust_version` via `extern "C"` with `#[no_mangle]`.
  - `src/crates/rust-demo/rust_demo.h` hand-written C header matching
    the two exports.
  - `CMakeLists.txt`: new `ENABLE_RUST_DEMO` option (default ON),
    extended Corrosion guard, new `corrosion_import_crate` block, and
    a guarded `target_link_libraries(netdata PRIVATE rust_demo)` plus
    `target_compile_definitions(netdata PRIVATE HAVE_RUST_DEMO)`.
  - `src/daemon/main.c`: include `crates/rust-demo/rust_demo.h` under
    `HAVE_RUST_DEMO` and emit `netdata_log_info("RUST FFI smoke: ...")`
    right after the existing `NETDATA STARTUP` log line.
- Validated locally on Linux:
  - `cargo build` in the crate directory succeeds and exports
    `nd_rust_add` and `nd_rust_version` (verified with `nm`).
  - `cmake --build build --target netdata` succeeds; Corrosion builds
    `build/cargo/.../debug/librust_demo.a`; `nm build/netdata` shows
    both FFI symbols; `strings build/netdata` shows both the C log
    format and the Rust version string. `./build/netdata -h` runs.
- Not yet validated:
  - The Windows MSYS2/UCRT64 build path. That is the whole point of
    the draft PR the user will open from this branch.

## Validation

Acceptance criteria evidence:

- Pending.

Tests or equivalent validation:

- Pending.

Real-use evidence:

- Pending.

Reviewer findings:

- Pending.

Same-failure scan:

- N/A — no failure pattern exists yet.

Sensitive data gate:

- No raw secrets, credentials, tokens, customer data, private endpoints, or
  proprietary incident details touched by this SOW. The smoke crate emits a
  constant integer sum and a static version string.

Artifact maintenance gate:

- Pending (will be filled at close).

Specs update:

- Not needed. No public/contract behaviour added.

Project skills update:

- Not needed. No new workflow surface.

End-user/operator docs update:

- Not needed. Internal scaffolding.

End-user/operator skills update:

- Not needed.

Lessons:

- Pending.

Follow-up mapping:

- Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
