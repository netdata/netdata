# SOW-0023 - Move dbengine test sources out of the production library

## Status

Status: open

Sub-state: drafted; independent of the other dbengine-* SOWs but required before [[dbengine-libdbengine-cmake-target]] can produce a clean static library.

## Requirements

### Purpose

Relocate the four test sources currently bundled into the dbengine source list — `dbengine-unittest.c`, `dbengine-stresstest.c`, `mrg-unittest.c`, `page_test.cc` — into a separate `dbengine-tests` CMake executable target. They reach into `RRDDIM`, `RRDSET`, `RRDHOST`, and the wider daemon, so they do not belong in `libdbengine.a`.

### User Request

Implicit: user asked for a clean library boundary. Tests bundled into the production library would force `libdbengine.a` to carry symbols it has no business exposing.

### Assistant Understanding

Facts:

- `src/database/engine/dbengine-unittest.c` (426 lines): references `RRDDIM`, `RRDSET`, `host->db[0].si`, `rrdset_name`, `rrddim_id`, `rrddim_set_collected_max_int`, `rrdeng_quiesce`, `rrdeng_flush_all`, `rrdeng_exit`.
- `src/database/engine/dbengine-stresstest.c` (459 lines): same coupling pattern plus `storage_engine_query_init` directly.
- `src/database/engine/mrg-unittest.c` (546 lines): tests internal `mrg.h` symbols; less daemon coupling, but still test-only.
- `src/database/engine/page_test.cc` (a C++ unit test for `page.c`): unit test for an internal module.
- All four are listed in `CMakeLists.txt:1744-1775` inside the `ENABLE_DBENGINE` block under `RRD_PLUGIN_FILES`, which feeds the main `netdata` executable at line 2151.
- The daemon `main.c` does not invoke `dbengine-unittest.c` or `dbengine-stresstest.c` on a normal startup; they are entry points reached via `--unittest` / `--stresstest` flags handled in `src/daemon/main.c`.

Inferences:

- The unittest/stresstest entry points are CLI test harnesses, not library code. They belong in a dedicated test executable that links libdbengine + libnetdata + the RRD layer.
- `page_test.cc` and `mrg-unittest.c` are unit tests of internal modules. They can live alongside the library source tree but should not be compiled into the production library archive.

Unknowns:

- Whether the existing `--unittest` flag in `netdata` (which today invokes `dbengine-unittest.c`) is referenced by CI or docs. If yes, we either keep an `--unittest dbengine` shim that exec's the new test binary, or accept that the flag moves to the new binary. Decide at activation.

### Acceptance Criteria

- `dbengine-unittest.c`, `dbengine-stresstest.c`, `mrg-unittest.c`, `page_test.cc` removed from `RRD_PLUGIN_FILES` in CMakeLists.txt.
- New target `add_executable(dbengine-tests ...)` exists, linking the production engine sources (post-[[dbengine-libdbengine-cmake-target]]: via `target_link_libraries(... dbengine)`).
- `netdata --unittest` either still works (via a shim that exec's `dbengine-tests`) or is removed with documentation noting the new binary.
- The production `libdbengine.a` (once SOW-0024 lands) contains no symbols from these four files.
- Existing CI invocations of `--unittest` / `--stresstest` (if any) continue to pass — verify by grepping CI configs.

## Analysis

Sources checked:

- `src/database/engine/dbengine-unittest.c`
- `src/database/engine/dbengine-stresstest.c`
- `src/database/engine/mrg-unittest.c`
- `src/database/engine/page_test.cc`
- `CMakeLists.txt:1744-1775, 2151`
- `src/daemon/main.c` for `--unittest` / `--stresstest` argument handling

Current state:

- The four files compile into `netdata` itself. The unittest/stresstest entry functions are called from `src/daemon/main.c` when the appropriate CLI flag is passed. This bloats `netdata` and forces `libdbengine.a` (if built today) to carry test code.

Risks:

- Low. The relocation is a CMake-level move + adjusting the argument dispatcher in `main.c` to either invoke the new binary via `execve` or to drop the flag.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Test sources are mixed into the library source list. The fix is to split the CMake target.

Evidence reviewed:

- File coupling listed in Facts.
- CMakeLists.txt structure.

Affected contracts and surfaces:

- CMakeLists.txt (target additions).
- `src/daemon/main.c` (`--unittest` / `--stresstest` dispatch).
- Optionally CI configs under `.github/workflows/` if they invoke `--unittest`.

Existing patterns to reuse:

- `add_executable(nd-run ...)` and `add_executable(spawn-tester ...)` at CMakeLists.txt:2615, 2680 demonstrate ancillary test/utility executable targets.

Risk and blast radius:

- Low. Mechanical move. The largest risk is breaking a CI workflow that relies on `netdata --unittest`.

Sensitive data handling plan:

- N/A — no secrets.

Implementation plan:

1. Grep `.github/workflows/` and `packaging/` for `--unittest` and `--stresstest` usage.
2. Add the new `dbengine-tests` executable target in CMakeLists.txt with the four files as sources. Link libnetdata + (until SOW-0024 lands) the same source list `netdata` uses for dbengine.
3. Remove the four files from `RRD_PLUGIN_FILES`.
4. Adjust `src/daemon/main.c` to either (a) invoke the new binary via `execve` when `--unittest`/`--stresstest` is passed, or (b) remove the flags and emit a friendly error message pointing to `dbengine-tests`.
5. Update any CI workflow if needed.
6. Confirm `dbengine-tests` runs and exits 0 on a fresh tree.

Validation plan:

- Build `netdata` and `dbengine-tests`; both link cleanly.
- Run `dbengine-tests` end-to-end; behavior matches pre-SOW `netdata --unittest`.
- If CI used the old flag, ensure the workflow uses the new binary.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: `[[dbengine-library]]` updated to note test sources are in a separate target.
- End-user/operator docs: minor README in `src/database/engine/` mentioning the new test binary, if a README pointer exists.
- End-user/operator skills: no update.
- SOW lifecycle: standard close.

Open decisions:

- Shim vs flag removal for `--unittest` / `--stresstest`. To decide at activation by checking CI/doc usage.

## Plan

1. CI/doc grep for old flags.
2. CMake target split.
3. main.c dispatch update.
4. Build + run verification.

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
