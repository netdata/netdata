# SOW-0033 - Migrate the Windows build to UCRT64

## Status

Status: in-progress

Sub-state: Pre-impl gate filled; investigation evidence inherited from
SOW-0032. Phase plan drafted; awaiting user confirmation on phase
sequencing before implementation begins. The `rwin` worktree is dedicated
to this migration. SOW-0032 (Rust-on-Windows smoke) is paused and will
resume to validate this SOW's outcome at close.

## Requirements

### Purpose

Make the Windows build of the Netdata Agent compatible with the MSYS2
UCRT64 environment so that the entire `netdata.exe` (and its companion
binaries) is produced by a single, internally-consistent toolchain
(`/ucrt64/bin/gcc`, UCRT C runtime, LLP64 data model). The current MSYS
shell path links against `msys-2.0.dll` and uses the Cygwin POSIX
emulation, which is incompatible with `/ucrt64/bin/rustc` output — making
non-trivial in-process Rust integration unsafe (split CRT, `long` size
mismatch, separate heap). This migration unblocks "a lot of Rust code"
landing as in-process FFI as well as standalone Rust plugins, and shrinks
the installer footprint by replacing the bundled MSYS2 root with a small
set of UCRT64 runtime DLLs.

### User Request

> I think that this worktree then should become migration of our codebase
> to be compatible with UCRT64. It should still include the test Rust
> crate so that we can exercise a Rust build. However, it should be off
> while we are working on the UCRT64 migration.

Following an earlier exchange in the same thread investigating ABI
compatibility and migration scope:

> Eventually, we will write a lot of Rust code. Can you check how big
> that change would be?

### Assistant Understanding

Facts:

- Current Windows build runs in MSYS shell. `MSYSTEM` is never set by
  the netdata build scripts; per `msys2/MSYS2-packages/filesystem/msystem`
  it defaults to `MSYS`, which makes `/etc/profile` use
  `/usr/local/bin:/usr/bin:/bin` as PATH (no `/ucrt64/bin`).
- `packaging/cmake/Modules/NetdataPlatform.cmake:74` explicitly accepts
  `CMAKE_SYSTEM_NAME == "MSYS"` as the Windows configuration, confirming
  the MSYS shell is the intended host.
- C source uses Cygwin-specific path-translation in 5 files:
  - `src/libnetdata/os/os.c` (4 calls)
  - `src/libnetdata/os/file_lock.c` (2 calls)
  - `src/libnetdata/os/disk_space.c` (2 calls)
  - `src/libnetdata/spawn_server/spawn_server_windows.c` (2 calls)
  - `src/web/api/v2/api_v2_claim.c` (1 call)
- `src/libnetdata/common.h:463` includes `<sys/cygwin.h>` under
  `OS_WINDOWS`, and the same header unconditionally pulls in POSIX
  headers that MSYS provides but MinGW provides only via Winsock2-shaped
  variants (different `SOCKET`/`int`, `closesocket`/`close`,
  `WSAGetLastError`/`errno`).
- The Windows installer (`packaging/windows/netdata.wxs.in`) ships the
  entire MSYS2 root by globbing `C:\msys64\opt\netdata\**`, populated by
  `packaging/windows/package-windows.sh` which untars
  `/msys2-latest.tar.zst` into `/opt/netdata/`.
- Existing Rust-in-Linux integrations (`otel-plugin`, `netflow-plugin`,
  `otel-signal-viewer-plugin`) are out-of-process plugins installed via
  `corrosion_install(TARGETS ...)`. Migrating to UCRT64 is **not** a
  prerequisite for out-of-process Rust plugins on Windows; it is a
  prerequisite for in-process Rust FFI (the smoke crate added in
  SOW-0032 demonstrates the in-process pattern).

Inferences:

- Most pervasive cost is the POSIX↔Winsock2 audit, not the Cygwin path
  translation. Path translation is bounded (5 files, ~10 call sites).
- Migrating in phases is necessary; an "everything at once" change will
  fail CI for many cycles and be hard to bisect.
- Each MSYS package family in `msys2-dependencies.sh` has a UCRT64
  counterpart (`mingw-w64-ucrt-x86_64-*`). The `mingw-w64-ucrt-x86_64-toolchain`
  meta-package already covers the basics; per-library replacements are
  the named `msys/brotli-devel`, `msys/libuv-devel`, `msys/pcre2-devel`,
  `msys/zlib-devel`, `msys/libcurl-devel`, plus `liblz4-devel`,
  `libutil-linux-devel`, `libyaml-devel`, `libzstd-devel`,
  `openssl-devel`, `protobuf-devel`. All have ucrt64 mirrors.
- Removing the MSYS bundle from the installer is a separate workstream
  inside this SOW but is high-leverage (installer size drop, no more
  shipping a POSIX userspace to end users).

Unknowns:

- Whether any Netdata C code paths depend on Cygwin-specific behaviour
  beyond `cygwin_conv_path` (e.g., `fork()` semantics, signal forwarding
  through msys-2.0.dll, file-descriptor-as-int assumptions). Discovered
  during the socket/spawn audit phase.
- Whether bundled dependencies (jsonc, OpenSSL/protobuf vendoring) need
  changes when the C toolchain switches.
- Whether the Windows event log / driver subsystem
  (`src/collectors/windows.plugin/driver/*`, `wevt_netdata.dll`) needs
  any toolchain adjustment beyond what MinGW already provides.
- Whether the Go plugin and Go-built binaries (already from
  `/ucrt64/bin/go`) interact correctly with a UCRT64 C runtime when the
  agent spawns them.
- Exact installer size delta — to be measured.

### Acceptance Criteria

- `packaging/windows/compile-on-windows.sh` succeeds with
  `MSYSTEM=UCRT64` and no `msys/*` packages installed (only
  `mingw-w64-ucrt-x86_64-*`).
- `netdata.exe` runs on a clean Windows machine without
  `msys-2.0.dll` present anywhere on the system, and starts as a
  Windows service.
- All collectors that currently work on Windows continue to collect
  data (verified by inspecting the Windows agent in a live test).
- The resulting MSI installer is smaller than the current MSI (target:
  meaningful drop, exact figure measured during validation).
- `packaging/windows/compile-on-windows.sh` no longer passes
  `-DENABLE_RUST_DEMO=Off`. The Windows build picks up the default
  `ENABLE_RUST_DEMO=On` from CMake and exercises the toolchain via the
  rust-demo smoke crate added in SOW-0032.
- `packaging/cmake/Modules/NetdataPlatform.cmake` no longer treats
  `CMAKE_SYSTEM_NAME == "MSYS"` as a Windows variant (or, if kept for
  backwards-compat with developer setups, is clearly documented as
  deprecated).
- No active source-tree references to `<sys/cygwin.h>` or
  `cygwin_conv_path()` outside the vendored `sqlite3.c` (which can keep
  its own conditional and is untouched by us).

## Analysis

Sources checked:

- `packaging/windows/{build.ps1,install-dependencies.ps1,invoke-msys2.ps1,functions.ps1,compile-on-windows.sh,msys2-dependencies.sh,package-windows.sh,netdata.wxs.in,bash_execute.sh,find-sdk-path.sh,get-win-build-path.sh,win-build-dir.sh,clion-msys-*-environment.bat}`
- `packaging/cmake/Modules/NetdataPlatform.cmake`
- `CMakeLists.txt` (Rust/Corrosion block L244-L334; main `netdata`
  target L3229-L3320)
- `src/libnetdata/common.h` (L90-L209 POSIX headers; L453-L486 OS_WINDOWS
  block)
- `src/libnetdata/os/{os.c,file_lock.c,disk_space.c,run_dir.c}`
- `src/libnetdata/spawn_server/spawn_server_windows.c`
- `src/web/api/v2/api_v2_claim.c`
- `src/daemon/buildinfo.c:1200-1202` (`__CYGWIN__ + __MSYS__` identifier check)
- Local mirror clones for cross-reference:
  - `msys2/MINGW-packages @ HEAD` (sparse) — `mingw-w64-rust/{PKGBUILD,bootstrap.toml,*.patch}`
  - `msys2/MSYS2-packages @ HEAD` (sparse) — `filesystem/{msystem,profile,profile.000-msys2.sh,msystem.d.*}`
  - `corrosion-rs/corrosion @ stable/v0.5` (netdata fork, sparse) —
    `cmake/FindRust.cmake` (cargo discovery via `Rust_COMPILER` toolchain
    walk)

Current state:

- The Windows build is a working but Cygwin-bound build. It ships a
  ~100+ MB MSYS2 root inside the MSI so that `msys-2.0.dll` and POSIX
  emulation are available on the target machine. Code paths that span
  the POSIX/Win32 boundary go through `cygwin_conv_path()` and the
  cygwin runtime.
- Rust on Linux already follows an out-of-process plugin pattern via
  `corrosion_install(TARGETS ...)`. There are no in-process Rust FFI
  call sites in production code today; the smoke crate from SOW-0032
  is the first.

Risks:

- Socket-layer regressions: the most pervasive change. Easy to
  introduce subtle bugs in error-handling paths (`errno` vs
  `WSAGetLastError()`).
- Path-handling regressions: replacing `cygwin_conv_path()` with Win32
  equivalents has edge cases (UNC paths, long paths, drive-relative
  paths, symlinks, mounted POSIX paths the cygwin runtime resolves
  from `/etc/fstab`).
- Installer regressions: switching from "ship all of MSYS2" to "ship
  only the UCRT64 runtime DLLs" could miss a transitively-required DLL
  and produce an installer that fails on clean machines. Mitigation:
  use `dumpbin /dependents` or `ntldd` during validation.
- Driver/ETW build: `wevt_netdata.dll`, `netdata_driver.sys`, and the
  WiX `CustomAction` invocations depend on Windows SDK tools (`mc.exe`,
  `rc.exe`, `wevtutil`), accessed today via paths the build computes
  through MSYS-style cygpath. May or may not work identically in
  UCRT64.
- Go toolchain interaction: `mingw-w64-ucrt-x86_64-go` is already used.
  Plugins spawned by `netdata.exe` need to continue to receive correct
  POSIX-style paths (or be migrated to native paths) for things like
  log file targets.
- WiX 5 + UCRT64 paths: the `<Files Include="C:\msys64\opt\netdata\**">`
  glob assumes a layout. If we change `package-windows.sh` to populate
  a smaller subset under a different staging directory, WiX needs to
  follow.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The Windows build is composed of two C runtimes — `msys-2.0.dll` for
  the agent's C side and `ucrtbase.dll` for the Rust side — which
  cannot safely share heap allocations, file descriptors, errno, or
  signed-long structs. To use Rust as in-process FFI in a "lot of
  Rust code" future, the agent's C side must move onto the same UCRT
  runtime as Rust. The MSYS2 ucrt64 environment is the standard,
  supported toolchain for that.

Evidence reviewed:

- All "Sources checked" above. Open-source cross-references recorded
  with upstream `owner/repo @ commit` instead of local mirror paths.

Affected contracts and surfaces:

- The Windows installer (MSI): file layout under `/opt/netdata`,
  bundled runtime, WiX file globs.
- The Windows agent runtime: every C file under `src/libnetdata/os/`,
  `src/libnetdata/spawn_server/`, the daemon, the streaming and
  cloud-connection code paths that touch sockets or paths.
- Build scripts: `packaging/windows/{build.ps1,install-dependencies.ps1,compile-on-windows.sh,msys2-dependencies.sh,package-windows.sh}` and the WiX template.
- Developer-facing helpers: `packaging/windows/clion-msys-*-environment.bat`
  (set MSYSTEM=MSYS today; needs an UCRT64 variant).
- CMake: `packaging/cmake/Modules/NetdataPlatform.cmake` MSYS branch.

Existing patterns to reuse:

- The codebase already has `OS_WINDOWS`, `HAVE_*` feature macros, and
  per-OS `.c` shards in `src/libnetdata/os/`. The migration extends
  this pattern rather than introducing new abstractions.
- The Linux Rust out-of-process plugin pattern (otel-plugin,
  netflow-plugin) — see SOW-0032 analysis — is the recommended target
  for new Rust code that does not strictly need in-process FFI.

Risk and blast radius:

- Repository-wide: any file touching paths, sockets, or process spawn
  may need to change. Estimated touch set: ~20-40 C files.
- User-visible: installer behavior, binary compatibility for users
  with custom integrations referring to paths under `/opt/netdata`,
  service registration. Each must be re-verified.
- Reversibility: the MSYS path stays available via developer scripts;
  CI cuts over to UCRT64. Reverting at the SOW level is `git revert`
  on the merge commit.

Sensitive data handling plan:

- No sensitive data involved in any of the affected files. All
  validation evidence we capture is build/test output, never customer
  data. The installer changes are public packaging.

Implementation plan (phased):

1. **Phase 0 — Infrastructure (paused-SOW already covers smoke crate).**
   Done in SOW-0032 + the override in `compile-on-windows.sh`. Sets up
   the gating so Windows CI can iterate without the Rust crate
   interfering.

2. **Phase 1 — Shell switch.** Set `MSYSTEM=UCRT64` in `build.ps1` (and
   `install-dependencies.ps1`), then make the bash login pick up the
   UCRT64 PATH. Add an UCRT64 variant of the CLion environment .bat
   for developer parity. Update `NetdataPlatform.cmake` to accept
   `CMAKE_SYSTEM_NAME == "Windows"` as the production path (the
   "MSYS" branch becomes a developer-only fallback). Confirms cmake
   picks `/ucrt64/bin/gcc` automatically. Expect the build to fail
   loudly here — that is the signal we need.

3. **Phase 2 — Dependency package swap.** Replace `msys/*` and
   `*-devel` entries in `msys2-dependencies.sh` with their
   `mingw-w64-ucrt-x86_64-*` counterparts. Re-verify cmake's package
   discovery finds them under `/ucrt64/lib/pkgconfig`.

4. **Phase 3 — Cygwin path translation removal.** Replace the ~10
   `cygwin_conv_path()` call sites with a small new helper in
   `src/libnetdata/os/` that uses `MultiByteToWideChar` +
   `GetFullPathNameW` (or `_wfullpath`) under `OS_WINDOWS`. Delete the
   `#include <sys/cygwin.h>` from `common.h`. Audit `run_dir.c` and
   `buildinfo.c` for the same patterns.

5. **Phase 4 — POSIX↔Winsock2 audit and reconciliation.** Per file
   under `src/libnetdata/` (sockets, fd/handle, signals, fork/spawn,
   pwd/grp, errno), wrap or replace the Cygwin-emulated POSIX call
   with the Win32-native pattern. This is the longest phase and may
   produce several intermediate commits.

6. **Phase 5 — Spawn server and plugins.** Audit
   `spawn_server_windows.c` and the cross-process surface used by Go
   and Rust plugins. Confirm spawned children inherit the right
   environment under UCRT64.

7. **Phase 6 — Installer slim-down.** Rewrite `package-windows.sh` to
   stop untarring all of MSYS2 into `/opt/netdata`. Instead, copy only
   the required UCRT64 runtime DLLs (`libgcc_s_seh-1.dll`,
   `libwinpthread-1.dll`, `libstdc++-6.dll` if used, and the DLLs for
   OpenSSL/curl/libuv/pcre2/zstd/lz4/brotli). Use `ntldd` or
   `dumpbin /dependents` on `netdata.exe` to produce the exact list.
   Adjust WiX `<Files Include=...>` glob and any explicit `<File>`
   entries.

8. **Phase 7 — Re-enable rust-demo and validate.** Remove
   `-DENABLE_RUST_DEMO=Off` from `compile-on-windows.sh`. Confirm
   Windows CI builds Rust, links the staticlib, runs `netdata.exe`
   and emits the `RUST FFI smoke:` log line. Close SOW-0032 at this
   point.

9. **Phase 8 — Wrap-up.** Remove dead code, update
   `packaging/windows/WINDOWS_INSTALLER.md`, archive the MSYS path in
   developer docs, capture installer size delta in this SOW.

Each phase ends with the build passing on CI. Each phase is committed
separately so any regression can be bisected to one phase.

Validation plan:

- Per phase: `compile-on-windows.sh` succeeds on a draft PR on a
  clean Windows runner.
- Phase 3: unit-test the new path-translation helper on representative
  inputs (`/`, `/c/Users/x`, `C:\Users\x`, UNC paths, long paths,
  symlinks).
- Phase 4: targeted manual tests for streaming connect, ACLK connect,
  systemd-journal Function (Windows journal), Function Spawn, and HTTP
  ingest.
- Phase 6: `ntldd netdata.exe` on the staged installer payload to prove
  no transitive DLL is missed.
- Phase 7: `RUST FFI smoke:` line visible in the Windows agent log;
  installer size measured against pre-migration baseline.
- End-to-end: install MSI on a clean Windows 11 VM with no MSYS2
  present; verify service starts, dashboard loads, claims work.

Artifact impact plan:

- AGENTS.md: no change. The CRITICAL RULES (root-cause, no
  duplication, communication style) remain applicable.
- Runtime project skills: no update needed. The migration is too
  cross-cutting to be captured as a recurring workflow.
- Specs: capture the final Windows toolchain decision in a new spec
  `.agents/sow/specs/windows-toolchain.md` describing UCRT64 as the
  production target, ucrt-only CRT, the smaller installer footprint,
  and the rule that new Rust may be either out-of-process plugin or
  in-process FFI subject to safe-FFI rules (no `long`, no shared heap).
- End-user/operator docs: `packaging/windows/WINDOWS_INSTALLER.md`
  may need updates if installer behavior changes user-visibly (size,
  paths, requirements). If end-user docs reference paths under
  `/opt/netdata/msys64/...` they will need updating.
- End-user/operator skills: none expected, but verify
  `docs/netdata-ai/skills/query-netdata-agents/` does not depend on
  MSYS-style paths.
- SOW lifecycle: SOW-0032 paused; resumes at Phase 7 close. SOW-0033
  closes when all acceptance criteria pass.

Open-source reference evidence:

- `msys2/MINGW-packages @ HEAD`
  - `mingw-w64-rust/PKGBUILD` — confirms target triple
    `x86_64-pc-windows-gnu` for the ucrt64 rust package, with
    UCRT-linked toolchain via clang from `$MINGW_PREFIX/bin`.
  - `mingw-w64-rust/bootstrap.toml` — confirms `-Ctarget-feature=-crt-static`
    and `-Clink-arg=-lsecur32` defaults.
- `msys2/MSYS2-packages @ HEAD`
  - `filesystem/profile` and `filesystem/msystem` — confirms
    `MSYSTEM=MSYS` default and the PATH consequences.
- `corrosion-rs/corrosion @ stable/v0.5` (netdata fork)
  - `cmake/FindRust.cmake:489-500` — confirms cargo is discovered next
    to `Rust_COMPILER`, no `Rust_CARGO` override needed.

Open decisions:

1. Phase 1 needs a choice on how aggressively to break the MSYS path.
   Recommended: set MSYSTEM=UCRT64 unconditionally in build.ps1; keep
   the developer CLion-MSYS .bat for hand-holding; mark
   NetdataPlatform.cmake's MSYS branch deprecated but functional.
2. Phase 4 may discover blocking issues (e.g., an entire subsystem
   that relies on `<sys/un.h>` AF_UNIX semantics). The plan is to
   surface these as sub-decisions when they appear, not pre-plan
   for unknown unknowns.
3. Phase 6 needs a choice on how minimal the installer should be:
   strictly the DLLs `dumpbin` finds, or also keep a small set of
   common UCRT64 tools (e.g., `bash.exe`) for users who today rely
   on the MSYS shell shipped inside the installer. Recommended:
   strictly DLLs; add helper tools only if user reports show demand.

## Implications And Decisions

1. Scope pivot from "rust-on-Windows smoke" (SOW-0032) to
   "UCRT64 migration" (this SOW). Recorded as: SOW-0032 status
   `paused`, SOW-0033 status `in-progress`, smoke crate stays in
   tree behind a Windows-only opt-out.
2. The `rwin` worktree is now dedicated to this migration. The smoke
   crate is the canary that proves the migration is complete: Phase 7
   removes the opt-out and validates Rust runs in the migrated build.
3. Out-of-process Rust plugins remain the recommended pattern for most
   new Rust code; in-process FFI is reserved for the cases where IPC
   overhead is unacceptable. This SOW unblocks both, with safety on
   the in-process path.

## Plan

See "Implementation plan (phased)" in the Pre-Implementation Gate
above. Phases 1-8 are sequential and each gates the next on a green
CI run.

## Execution Log

### 2026-05-22

- Investigation complete; SOW filed.
- Smoke crate (SOW-0032) committed (`8733eadf75`) with `ENABLE_RUST_DEMO`
  defaulting ON in CMake. `packaging/windows/compile-on-windows.sh` now
  passes `-DENABLE_RUST_DEMO=Off` to opt the Windows path out of the
  smoke crate while this migration is in flight (`0b02218107`).
- User confirmed: start at Phase 2 (shell switch) and use a **hard cut**
  for developer-experience changes (no MSYS-compatibility fallback).
- Phase 2 implementation:
  - `packaging/windows/build.ps1`,
    `packaging/windows/install-dependencies.ps1`,
    `packaging/windows/invoke-msys2.ps1`, and
    `packaging/windows/package.ps1` all now set
    `$env:MSYSTEM = 'UCRT64'` before invoking msysbash. This is the
    one-line change that makes `/etc/profile` add `/ucrt64/bin` to
    PATH and routes `gcc`, `pkg-config`, and friends through the
    UCRT64 toolchain.
  - `packaging/cmake/Modules/NetdataPlatform.cmake`:
    - Dropped the `CYGWIN` and `MSYS` `CMAKE_SYSTEM_NAME` branches.
      Only `Windows` is accepted now.
    - Added a `FATAL_ERROR` guard inside `_nd_windows_config()` that
      refuses to configure unless `MSYSTEM=UCRT64`. Other MSYS2
      environments (`MSYS`, `MINGW64`, `CLANG64`) now fail fast with
      a clear message pointing at this SOW.
    - The CLion `include_directories` branches collapsed to a single
      `c:/msys64/ucrt64/include` line.
  - `packaging/windows/compile-on-windows.sh` dropped
    `-DRust_COMPILER=/ucrt64/bin/rustc`. Under UCRT64,
    `/ucrt64/bin/{rustc,cargo}` are on PATH and Corrosion's
    `FindRust.cmake` will discover them automatically.
  - Deleted `packaging/windows/clion-msys-msys-environment.bat` and
    `packaging/windows/clion-msys-mingw64-environment.bat`. Added
    `packaging/windows/clion-msys-ucrt64-environment.bat` as the
    only supported CLion developer entry point.
- Expected next CI signal from Phase 2:
  - `pacman -Syuu` from `msys2-dependencies.sh` may behave differently
    under UCRT64 vs MSYS (still updates the runtime; should be fine).
  - The C compile will likely fail on `<sys/cygwin.h>` and
    `cygwin_conv_path()` calls because `/ucrt64/include` does not
    provide them. That is the expected entry point for Phase 3
    (path translation removal).
  - It may also fail on POSIX↔Winsock2 type mismatches noticed by
    `/ucrt64/include/winsock2.h` (Phase 4).
  - Build failure here is informative, not regressive: this is the
    signal we are migrating to surface.

## Validation

Acceptance criteria evidence:

- Pending — fills as phases complete.

Tests or equivalent validation:

- Pending.

Real-use evidence:

- Pending. Final acceptance requires installing the resulting MSI on a
  clean Windows machine.

Reviewer findings:

- Pending.

Same-failure scan:

- Pending.

Sensitive data gate:

- Confirmed clean. No customer data, credentials, or private endpoints
  referenced. Public packaging only.

Artifact maintenance gate:

- Pending. Will be filled at close.

Specs update:

- Pending. New spec `.agents/sow/specs/windows-toolchain.md` planned
  for Phase 7/8.

Project skills update:

- Pending.

End-user/operator docs update:

- Pending — likely `packaging/windows/WINDOWS_INSTALLER.md`.

End-user/operator skills update:

- Pending audit.

Lessons:

- Pending.

Follow-up mapping:

- Pending. SOW-0032 will be resumed and closed at Phase 7.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

- Resume SOW-0032 at Phase 7 of this SOW.

## Regression Log

None yet.
