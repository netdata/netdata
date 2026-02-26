# TODO - Dump Analyzer Trace (go.d.plugin)

## TL;DR
- Trace how `DumpAnalyzer` works in `src/go/plugin/agent`, including initialization, data capture, lifecycle hooks, and report generation, with concrete file/line evidence.

## Requirements
- "I need you to trace how dump analyzer works in go.d.plugin"
- "See src/go/plugin/agent/ for dump-related files in the directory."
- "I don't really know what that thing is and how it works."
- "ignore existing TODO files, they are from prev tasks"
- "inline files content here so I can see (some small sample)"
- "ok, this whole dump thing is needed only for v1 and for ibm.d plugin"
- "Can we move all dump-related into its own package and then somehow hook into job runtime. To isolate the whole thing."
- "We can rename it if needed. Dump sounds vague."
- "We don't need to keep backward compatibility, we can change anything. We should prefer clean design over anything."
- "We use go 1.25, so use generics if needed."
- "Both review verdict: REVISE. Check them thoughtfully"
- "Codex verdict: NOT READY. Check blockers before implementation."

## Facts
- Dump-related implementation in `agent/` is split into:
  - `../agent/dump_model.go` (types/state),
  - `../agent/dump_capture.go` (capture + persistence),
  - `../agent/dump_report.go` (analysis/report printing),
  - `../agent/dump.go` (split-file marker comment).
- CLI supports dump flags globally: `--dump`, `--dump-summary`, `--dump-data` (`../../pkg/cli/cli.go:22-24`).
- In this tree, `go.d` main wiring does **not** pass `DumpMode` or `DumpDataDir` to `agent.Config` (only `DumpSummary`) (`../../cmd/godplugin/main.go:97-101`), same for `scripts.d` (`../../cmd/scriptsdplugin/main.go:85-90`).
- `ibm.d` main wiring does pass dump options and parses duration (`../../cmd/ibmdplugin/main.go:85-117`), with default `DumpMode=10m` when `--dump-data` is provided (`../../cmd/ibmdplugin/main.go:61-63`).
- `Agent` creates/enables `DumpAnalyzer` when dump mode or dump data dir are configured (`../agent/agent.go:123-137`), then passes it into `jobmgr.Config` (`../agent/agent.go:230-243`).
- `agenthost` controls stop/report behavior:
  - starts a timer from `DumpModeDuration()` (`../../cmd/internal/agenthost/host.go:42-45`),
  - on timer expiry it calls `TriggerDumpAnalysis()` (`../../cmd/internal/agenthost/host.go:72-75`),
  - on `QuitCh` it exits without calling `TriggerDumpAnalysis()` (`../../cmd/internal/agenthost/host.go:69-76`).
- `jobmgr.Manager` dump integration:
  - creates per-job dump path `{dumpDataDir}/{sanitized module}/{sanitized job}` (`../agent/jobmgr/manager.go:562-565`),
  - registers job with analyzer (`../agent/jobmgr/manager.go:567-569`),
  - calls collector-level `EnableDump(dir)` when supported (`../agent/jobmgr/manager.go:582-584`, `../agent/jobmgr/manager.go:618-620`),
  - passes analyzer only to **V1** runtime config (`../agent/jobmgr/manager.go:623-637`).
- `framework/jobruntime` defines dump analyzer contract (`../framework/jobruntime/dump_analyzer.go:9-13`).
- Runtime hook call sequence (V1 jobs):
  - after successful `check/postCheck`, call `RecordJobStructure` once (`../framework/jobruntime/job_v1.go:268-271`),
  - each collect cycle, call `RecordCollection` with `intMetrics` only (`../framework/jobruntime/job_v1.go:480-484`),
  - each process cycle, call `UpdateJobStructure` to follow dynamic charts (`../framework/jobruntime/job_v1.go:560-563`).
- V2 jobs do not include analyzer hooks:
  - no dump fields in `JobV2Config` (`../framework/jobruntime/job_v2.go:27-41`),
  - no dump method calls in `job_v2*.go` (confirmed by search).
- Analyzer capture/persistence behavior:
  - `RegisterJob` prepares subdirs: `queries/`, `rows/`, `metrics/`, `meta/` (`../agent/dump_capture.go:38-42`),
  - `RecordJobStructure` stores chart/dimension topology and writes `meta/job.json` (`../agent/dump_capture.go:46-76`, `../agent/dump_capture.go:169-192`),
  - `RecordCollection` stores seen metric IDs + per-dimension values and writes `metrics/metrics-####.json` (`../agent/dump_capture.go:135-166`, `../agent/dump_capture.go:194-214`),
  - when all registered jobs have collected at least once, it writes root `manifest.json` and triggers `onComplete` callback (`../agent/dump_capture.go:216-270`).
- Analyzer report behavior:
  - `PrintReport` iterates jobs and runs structural checks (`../agent/dump_report.go:13-28`, `../agent/dump_report.go:232-485`),
  - checks include duplicate chart IDs, context/family conflicts, duplicate dimension IDs, missing/excess metric IDs, units/types/label/dimension inconsistencies, and family taxonomy heuristics (`../agent/dump_report.go:295-354`, `../agent/dump_report.go:487-1408`),
  - `PrintSummary` first calls `PrintReport`, then emits cross-job consolidated family/context summary (`../agent/dump_report.go:31-223`).
- Practical artifact ownership split:
  - analyzer writes `meta/job.json`, `metrics/*.json`, `manifest.json` (`../agent/dump_capture.go:169-271`);
  - collector-specific `EnableDump` implementations can additionally write `queries/` and `rows/` (example: AS400 `dumpContext`) (`../ibm.d/modules/as400/dump.go:30-127`, `../ibm.d/modules/as400/collector.go:1113-1122`, `../ibm.d/modules/as400/collect_data.go:656`, `../ibm.d/modules/as400/collect_data.go:700`).
- There are no dump-analyzer-focused tests under `agent/framework/cmd` test files (confirmed by grep).
- External-review findings validated against code:
  - Analyzer state is keyed only by `jobName` (`../agent/dump_model.go:15`, `../agent/dump_capture.go:33`, `../agent/dump_capture.go:74`), which allows cross-module name collisions.
  - `jobmgr` registers dump jobs for both V1 and V2 (`../agent/jobmgr/manager.go:562-569`), but runtime hooks (`Record*`) are V1-only (`../framework/jobruntime/job_v1.go:268-271`, `../framework/jobruntime/job_v1.go:482-483`, `../framework/jobruntime/job_v1.go:561-562`), so completion bookkeeping can be inconsistent when V2 jobs are present.
  - Host behavior differs between timeout and quit paths: timeout triggers analysis (`../../cmd/internal/agenthost/host.go:72-75`), quit path exits without triggering it (`../../cmd/internal/agenthost/host.go:69-71`).
  - Capture path performs file writes while holding analyzer mutex and intentionally ignores write errors (`../agent/dump_capture.go:135-167`, `../agent/dump_capture.go:191`, `../agent/dump_capture.go:213`, `../agent/dump_capture.go:270`).
  - `EnableDump` contract is an ad-hoc anonymous interface in two places (`../agent/jobmgr/manager.go:582-583`, `../agent/jobmgr/manager.go:618-619`), with AS400 as known implementation (`../ibm.d/modules/as400/collector.go:1113-1122`).
  - Shared CLI exposes dump flags globally (`../../pkg/cli/cli.go:22-24`) while `go.d`/`scripts.d` do not wire dump mode/data-dir (`../../cmd/godplugin/main.go:97-101`, `../../cmd/scriptsdplugin/main.go:85-90`).
- Additional `NOT READY` review points validated:
  - `pluginconfig.MustInit` and internal build flow require `*cli.Option` today (`../../pkg/pluginconfig/pluginconfig.go:69`, `../../pkg/pluginconfig/pluginconfig.go:119`), so Decision 5C needs an explicit parser/bridge design instead of direct shared option removal.
  - `pluginconfig` currently reads only `opts.ConfDir` and `opts.WatchPath` from the provided `*cli.Option` (`../../pkg/pluginconfig/pluginconfig.go:134`, `../../pkg/pluginconfig/pluginconfig.go:232`).
  - `MustInit` callsites are limited to three command mains (`../../cmd/godplugin/main.go:62`, `../../cmd/scriptsdplugin/main.go:50`, `../../cmd/ibmdplugin/main.go:65`), so changing the input contract is low blast radius.
  - Host has additional exit paths beyond timer/quit (`signal`, `keepAliveErr`, `runDone`) (`../../cmd/internal/agenthost/host.go:61-82`), so finalize policy must be defined for all paths.
- Latest external pass verdict was **NOT READY** because Decisions 6-9 were not fully locked in the plan at that time.
- Latest external pass recommended:
  - Decision 6 -> **A** (V1-only registration/audit),
  - Decision 7 -> **A** (finalize on all terminal host paths, idempotent),
  - Decision 8 -> **A** (ibm-specific parser carrying shared `cli.Option` subset for `pluginconfig.MustInit`),
  - Decision 9 -> **A** (best-effort, non-blocking hooks, deterministic error surfacing, no silent ignores).
  - Follow-up user decision supersedes Decision 8 recommendation: choose **8B** (minimal `pluginconfig` init contract).
- Latest external readiness pass verdict: **READY** (no blocking findings).

## User Made Decisions
- Ignore pre-existing TODO files from previous tasks; use a new task-specific TODO file for this analysis.
- Dump functionality target scope is V1 + ibm.d use case.
- Direction preference: isolate dump/analyzer code out of `agent` into its own package and hook via runtime interfaces.
- Rename is allowed and preferred if current terminology is vague.
- Backward compatibility is not required for this refactor; clean design has priority.
- Go version target is 1.25; generics are allowed when they improve design.
- Decision 1: **A** (`metricsaudit` naming).
- Decision 2: **A** (single package `framework/metricsaudit` with interface + implementation).
- Decision 3: **A** (no separate session type; keep host ownership with explicit agent lifecycle methods).
- Decision 4: **A + fix bundle** (scope remains V1 + ibm.d and include semantic correctness fixes in same refactor).
- Decision 5: **C** (remove audit-related flags from shared CLI; make them ibm.d-specific).
- Decision 6: **A** (V1-only registration/audit).
- Decision 7: **A** (finalize on all terminal host paths when audit mode is enabled; idempotent).
- Decision 8: **B** (replace `pluginconfig.MustInit(*cli.Option)` with a minimal init input contract carrying only required fields).
- Decision 9: **A** (best-effort non-blocking hooks; deterministic error surfacing; no silent write failures).
- Requested a thoughtful re-check after independent external reviews returned REVISE.
- External follow-up review returned **NOT READY** pending contract-level clarifications.
- Final external readiness review returned **READY** to start implementation.

## Implied Decisions
- Scope moved from explanation-only to architecture/refactor planning for isolation.
- Keep runtime integration through `jobruntime` interfaces instead of hard-coding analyzer logic in runtime internals.
- It is acceptable to rename CLI/config fields and exported identifiers if that yields cleaner ownership and terminology.

## Pending Decisions
- **Decision 1: Naming** — **RESOLVED: A (`metricsaudit`)**
  - Context: `metricsaudit` was chosen; one external review suggests `metrictrace`.
  - Options:
    - **A)** Keep `metricsaudit`.
    - **B)** Rename to `metrictrace`.
  - Recommendation: **A** (analysis/reporting is the primary function; capture is supporting behavior).

- **Decision 2: Package layering** — **RESOLVED: A (single package)**
  - Context: current choice was `framework/metricsaudit` + `framework/metricsaudit/api`; review feedback says this is unnecessary split.
  - Options:
    - **A)** Single package `framework/metricsaudit` containing both interface and implementation.
    - **B)** Keep split `framework/metricsaudit` + `framework/metricsaudit/api`.
  - Recommendation: **A** (simpler, less indirection, no demonstrated cycle pressure).

- **Decision 3: Lifecycle API shape** — **RESOLVED: A (no separate session type)**
  - Context: current choice was a dedicated session boundary.
  - Options:
    - **A)** No separate session type; keep host ownership but expose explicit agent-level lifecycle methods (`AuditDuration`, `FinalizeAudit`), and make quit/timer behavior consistent.
    - **B)** Introduce dedicated session object/interface consumed by host.
  - Recommendation: **A** (keeps ownership clear without adding a thin wrapper).

- **Decision 4: Include semantic correctness fixes in same refactor** — **RESOLVED: A**
  - Context: these are design-affecting defects, not mere cleanup.
  - Options:
    - **A)** Include now: Job identity keying, V2 registration gating/compatibility, quit/timer finalize consistency, named `EnableDump` capability interface, and write-path lock/error handling.
    - **B)** Move package/name only now; defer behavior fixes.
  - Recommendation: **A** (clean architecture without semantic fixes leaves known correctness bugs intact).

- **Decision 5: go.d/scripts.d CLI exposure in this scope** — **RESOLVED: C**
  - Context: V1 + ibm.d is target scope, but CLI flags are globally exposed.
  - Options:
    - **A)** Keep global flags for now, but ensure non-ibm binaries reject/ignore explicitly with clear messaging.
    - **B)** Keep as-is no-op behavior.
    - **C)** Remove dump-related flags from shared CLI and make ibm.d-specific CLI parsing.
  - Recommendation: **A** (truthful behavior without widening refactor blast radius too much).
  - User selection: **C**.

- **Decision 6: V2 handling policy for metricsaudit registration/completion** — **RESOLVED: A**
  - Context: current behavior registers before V1/V2 split while hooks are V1-only.
  - Options:
    - **A)** Register/audit only V1 jobs in this refactor.
    - **B)** Register V2 too, but explicitly mark as non-audited/non-blocking for completion.
  - Recommendation: **A** (simplest and aligns with declared scope).
  - User selection: **A**.

- **Decision 7: Finalization matrix across host exit paths** — **RESOLVED: A**
  - Context: timer/quit can be aligned, but signal/keepalive/runDone behavior still undefined.
  - Options:
    - **A)** Finalize on all terminal paths when audit mode is enabled (idempotent).
    - **B)** Finalize only on timer and explicit quit, skip others.
  - Recommendation: **A** (predictable operator behavior).
  - User selection: **A**.

- **Decision 8: Decision 5C parser bridge strategy** — **RESOLVED: B**
  - Context: shared parser returns `*cli.Option`, and `pluginconfig.MustInit` currently requires `*cli.Option`; however only `ConfDir` and `WatchPath` are consumed.
  - Options:
    - **A)** Keep common parser for shared fields; add ibm-specific parser struct embedding/carrying `cli.Option`, then pass `&common` to `pluginconfig.MustInit`.
    - **B)** Generalize `pluginconfig.MustInit` input contract away from `*cli.Option`.
  - Recommendation: **B** (aligns with Decision 5C + clean-design goal by shrinking `pluginconfig` contract to its real inputs).
  - User selection: **B**.

- **Decision 9: Write/error contract for capture path** — **RESOLVED: A**
  - Context: current path writes under lock and ignores errors.
  - Options:
    - **A)** Best-effort mode with explicit error accumulation/reporting (no silent ignore), non-blocking hooks.
    - **B)** Fail-fast mode where write errors terminate/fail collection flow.
  - Recommendation: **A** (safer for runtime stability while surfacing failures clearly).
  - User selection: **A**.

## Plan
- **Phase 0: Design lock (no code changes)**
  - All architecture decisions (1-9) are locked; implementation can proceed once external readiness gate is green.
  - Freeze naming map:
    - `DumpAnalyzer` -> `metricsaudit.Auditor` (concrete),
    - `jobruntime.DumpAnalyzer` -> `metricsaudit.Analyzer` (interface),
    - agent methods `DumpModeDuration`/`TriggerDumpAnalysis` -> `AuditDuration`/`FinalizeMetricsAudit`.

- **Phase 1: Create isolated package and move implementation**
  - Add `../framework/metricsaudit/` with split files (model/capture/report/api surface).
  - Move code from:
    - `../agent/dump_model.go`,
    - `../agent/dump_capture.go`,
    - `../agent/dump_report.go`.
  - Keep single package (no `metricsaudit/api` subpackage).
  - Introduce named capability interface for collectors:
    - `type Capturable interface { EnableDump(string) }` (name may be adjusted to match renamed terminology).

- **Phase 2: Contract ownership + runtime/jobmgr rewiring**
  - Remove `../framework/jobruntime/dump_analyzer.go`.
  - Update `../framework/jobruntime/job_v1.go`:
    - import `framework/metricsaudit`,
    - `JobConfig` analyzer field type -> `metricsaudit.Analyzer`,
    - runtime struct field type -> `metricsaudit.Analyzer`.
  - Update `../agent/jobmgr/manager.go`:
    - config and manager fields -> `metricsaudit.Analyzer`,
    - replace anonymous `interface{ EnableDump(string) }` assertions with `metricsaudit.Capturable`.

- **Phase 3: Agent and host lifecycle cleanup (Decision 3A)**
  - Update `../agent/agent.go`:
    - analyzer field uses `*metricsaudit.Auditor`,
    - rename lifecycle methods for host API (`AuditDuration`, `FinalizeMetricsAudit`),
    - keep ownership in agent (no separate session object).
  - Update `../../cmd/internal/agenthost/host.go`:
    - switch to renamed methods,
    - finalize on **all** terminal host paths (signal, quit, timer, keepalive failure, runDone) with idempotent semantics.

- **Phase 4: Semantic correctness fix bundle (Decision 4A)**
  - Implement stable job identity key (`module + job`) in `metricsaudit` state maps.
  - Apply chosen Decision 6 policy for V2 registration/completion semantics.
  - Remove/contain long lock + I/O coupling in capture path:
    - move expensive writes out of critical sections where safe,
    - apply Decision 9 write/error contract (no silent failures).

- **Phase 5: Command scope and CLI behavior (depends on Decision 5)**
  - `../../cmd/ibmdplugin/main.go`: rename config field wiring to new terminology.
  - `../../cmd/godplugin/main.go` and `../../cmd/scriptsdplugin/main.go`:
    - remove dependency on shared dump/audit flags.
  - `../../pkg/cli/cli.go`:
    - remove dump/audit flags from shared `Option`.
  - `../../pkg/pluginconfig/pluginconfig.go`:
    - replace `MustInit(*cli.Option)` with minimal init input (only `ConfDir`, `WatchPath`).
  - `../../cmd/ibmdplugin/main.go`:
    - define ibm.d-specific audit flags/parser and pass minimal pluginconfig init input.
  - `../../cmd/godplugin/main.go` and `../../cmd/scriptsdplugin/main.go`:
    - pass minimal pluginconfig init input built from shared/common parser fields.

- **Phase 6: Cleanup + verification**
  - Delete old agent dump files after rewiring:
    - `../agent/dump.go`,
    - `../agent/dump_model.go`,
    - `../agent/dump_capture.go`,
    - `../agent/dump_report.go`.
  - Run build/test/smoke matrix from Testing requirements.
  - Summarize diffs and residual risks.

## Testing requirements
- Post-refactor minimum:
  - compile/build of affected plugin command(s),
  - `go test` for `plugin/agent`, `plugin/framework/jobruntime`, and touched command package(s),
  - smoke run of dump flow for `ibm.d` path.

## Documentation updates required
- Update developer docs for new package location and ownership boundaries if refactor is accepted.
