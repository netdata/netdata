# TODO: godv2 Full-Pass Test Coverage Audit & Execution Plan

## TL;DR
- This file is the single source of truth for the godv2 test-coverage full pass before documentation work.
- Scope is all requested godv2 packages (charttpl, chartengine, chartemit, jobmgr, runtimemgr, metrix, module, tickstate).
- Verified high-confidence gaps/issues are concentrated in `jobmgr` + `module` correctness paths and missing dedicated tests in chartengine.
- External-review claims were reconciled; stale/false-positive findings are explicitly documented below.

## Requirements
- User requirement (verbatim):
  - "ok, i think the biggest missing part is documentation now. But before that we need to do a full pass on every newly added package and look for missing test coverage, it will allow us to find bugs we may missed."
- Scope clarification (verbatim):
  - "it is not recent commit, but all packages we added in godv2"
  - `src/go/plugin/go.d/agent/charttpl`
  - `src/go/plugin/go.d/agent/chartengine (subpackages included)`
  - `src/go/plugin/go.d/agent/chartemit`
  - `src/go/plugin/go.d/agent/jobmgr`
  - `src/go/plugin/go.d/agent/runtimemgr`
  - `src/go/pkg/metrix`
  - `src/go/plugin/go.d/agent/module`
  - `src/go/plugin/go.d/agent/internal/tickstate`
- Additional user request:
  - "Put everything in some file so we don't miss anything."

## Facts (Verified)
- `jobmgr/dyncfg.go` can send shutdown 503 and still execute queued path due to missing return.
  - Evidence: `src/go/plugin/go.d/agent/jobmgr/dyncfg.go:26`, `src/go/plugin/go.d/agent/jobmgr/dyncfg.go:31`.
- `jobmgr/dyncfg_collector.go` sends 400 on payload error but still proceeds to `SendYAML`.
  - Evidence: `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector.go:159`, `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector.go:165`.
- `jobmgr/filestatus.go` has self-deadlock in `contains()` (`contains` locks mutex, calls `lookup`, `lookup` locks again).
  - Evidence: `src/go/plugin/go.d/agent/jobmgr/filestatus.go:85`, `src/go/plugin/go.d/agent/jobmgr/filestatus.go:89`, `src/go/plugin/go.d/agent/jobmgr/filestatus.go:97`.
- `jobmgr/runProcessConfGroups` does not handle closed input channel (`ok` missing).
  - Evidence: `src/go/plugin/go.d/agent/jobmgr/manager.go:257`.
- `module/job_v1.go` accepts `UpdateVnode(nil)` and later dereferences vnode pointer in run path.
  - Evidence: `src/go/plugin/go.d/agent/module/job_v1.go:293`, `src/go/plugin/go.d/agent/module/job_v1.go:514`.
- Chartengine has major uncovered surfaces by dedicated test files:
  - `src/go/plugin/go.d/agent/chartengine/autogen.go`
  - `src/go/plugin/go.d/agent/chartengine/planner_labels.go`
  - `src/go/plugin/go.d/agent/chartengine/planner_lifecycle.go`
  - Evidence: no `autogen_test.go`, no `planner_labels_test.go`, no `planner_lifecycle_test.go` present in package.

## False Positives / Rejected Claims
- Claim: chartengine template-vs-autogen collision depends on Go map iteration order.
  - Rejected for current path.
  - Evidence: routes are sorted deterministically before accumulation in `src/go/plugin/go.d/agent/chartengine/matcher.go:134`; collision logic relies on chart ownership in `src/go/plugin/go.d/agent/chartengine/planner.go:432`.
- Claim: chartemit has partial buffer-write corruption risk via staged buffer writes.
  - Rejected as stale.
  - Evidence: current `ApplyPlan` path writes through API phase calls directly; no `b.Write` staged protocol path exists in `src/go/plugin/go.d/agent/chartemit/apply.go:37`.
- Claim: route-cache concurrency race is primary risk.
  - De-prioritized for current design.
  - Evidence: cache synchronization is intentionally delegated to engine serialization in `src/go/plugin/go.d/agent/chartengine/internal/cache/route_cache.go:26`.

## User Made Decisions
1. Scope for next pass: `A` (tests-only pass first).
2. Priority package order: `A` (`jobmgr` + `module` first).
3. `UpdateVnode(nil)` contract in v1: `A` (ignore nil, align with v2 behavior).
4. After tests exposed defects, proceed with bug-fix pass: `A` (fix all 5 verified bugs now).

## Implied Decisions
- Tests should explicitly encode current or intended contracts on request lifecycle and concurrency-sensitive paths.
- High-confidence defects from audit should be covered by tests before doc freeze.
- Lower-confidence findings remain parked until direct evidence is validated.

## Pending Decisions
- None currently.

## Package Coverage Snapshot (Audit Summary)
- `charttpl`: generally strong; low-risk edge hardening only.
- `chartengine`: strong core tests, but major dedicated gaps for autogen/labels/lifecycle slices.
- `chartemit`: moderate coverage; boundary/error-path tests still needed.
- `jobmgr`: highest risk concentration; multiple control-flow correctness gaps and untested infrastructure.
- `runtimemgr`: partial coverage; lifecycle/scheduler edge tests needed.
- `metrix`: broad coverage, but selector logical combinator and some reader edge contracts can be expanded.
- `module`: substantial coverage, but vnode and seam edge cases remain.
- `tickstate`: generally good, low priority.

## Master Backlog (Prioritized Tests)

### Milestone 1 (Priority: P0) — `jobmgr` + `module`
1. `TestDyncfgConfig_ShutdownDoesNotQueue`
   - File: `src/go/plugin/go.d/agent/jobmgr/dyncfg_test.go`
   - Covers: shutdown select path must return after 503.
2. `TestDyncfgConfigUserconfig_InvalidPayload_Returns400Only`
   - File: `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector_test.go`
   - Covers: no YAML emission after payload error.
3. `TestFileStatusContains_DoesNotDeadlock`
   - File: `src/go/plugin/go.d/agent/jobmgr/filestatus_test.go`
   - Covers: nested lock hazard in `contains/lookup`.
4. `TestRunProcessConfGroups_ChannelCloseDoesNotSpin`
   - File: `src/go/plugin/go.d/agent/jobmgr/manager_test.go`
   - Covers: closed channel behavior in `runProcessConfGroups`.
5. `TestJobV1UpdateVnode_NilIgnored`
   - File: `src/go/plugin/go.d/agent/module/job_v1_test.go`
   - Covers: decision `3.A` contract parity with v2.

### Milestone 2 (Priority: P1) — chartengine dedicated gaps
6. `TestAutogenScalarRouteBuildsCorrectly`
   - File: `src/go/plugin/go.d/agent/chartengine/autogen_test.go`
7. `TestAutogenHistogramBucketRouteExcludesLeBucket`
   - File: `src/go/plugin/go.d/agent/chartengine/autogen_test.go`
8. `TestAutogenSummaryQuantileRouteResolvesQuantileLabel`
   - File: `src/go/plugin/go.d/agent/chartengine/autogen_test.go`
9. `TestAutogenStateSetRouteResolvesStateLabelKey`
   - File: `src/go/plugin/go.d/agent/chartengine/autogen_test.go`
10. `TestAutogenTypeIDBudgetEnforcement`
   - File: `src/go/plugin/go.d/agent/chartengine/autogen_test.go`
11. `TestEnforceLifecycleCaps_DimensionCapEvictsLRU`
   - File: `src/go/plugin/go.d/agent/chartengine/planner_lifecycle_test.go`
12. `TestCollectExpiryRemovals_DimensionAndChartExpiry`
   - File: `src/go/plugin/go.d/agent/chartengine/planner_lifecycle_test.go`
13. `TestChartLabelAccumulatorIntersectsLabels`
   - File: `src/go/plugin/go.d/agent/chartengine/planner_labels_test.go`

### Milestone 3 (Priority: P2) — seam and boundary hardening
14. `TestApplyPlanRejectsEmptyTypeID`
   - File: `src/go/plugin/go.d/agent/chartemit/apply_test.go`
15. `TestNormalizeActionsOrderingDeterminism`
   - File: `src/go/plugin/go.d/agent/chartemit/apply_test.go`
16. `TestRuntimeMetricsJobStartStopLifecycle`
   - File: `src/go/plugin/go.d/agent/runtimemgr/job_test.go`
17. `TestRuntimeMetricsJobTickSkipWhenBusy`
   - File: `src/go/plugin/go.d/agent/runtimemgr/job_test.go`
18. `TestSelectorLogicalCombinators_TruthTable`
   - File: `src/go/pkg/metrix/selector/selector_test.go` (or `logical_test.go`)
19. `TestStoreReaderForEachMatch_VisibilityAndPredicate`
   - File: `src/go/pkg/metrix/reader_test.go`
20. `TestRegisterPanicOnMethodsAndJobMethodsConflict`
   - File: `src/go/plugin/go.d/agent/module/registry_test.go`

## Execution Plan
1. Milestone 1 tests only (`jobmgr` + `module`), no production behavior changes besides test-driven contract verification.
2. Milestone 2 add dedicated chartengine test files for autogen/labels/lifecycle.
3. Milestone 3 seam-level and boundary tests (`chartemit`, `runtimemgr`, `metrix`, `module registry`).
4. After each milestone, run package-level tests and race tests for touched areas.

## Testing Requirements
- For each milestone:
  - `go test ./src/go/plugin/go.d/agent/<touched-package>/... -count=1`
  - `go test ./src/go/pkg/metrix/... -count=1` (when metrix touched)
  - `go test -race ./src/go/plugin/go.d/agent/<touched-package>/... -count=1`
- Before closing task:
  - Full targeted sweep for all listed godv2 packages with race where feasible.

## Implementation Status (Milestone 1 Tests-Only Pass)
- Added new test files:
  - `src/go/plugin/go.d/agent/jobmgr/dyncfg_test.go`
  - `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector_test.go`
  - `src/go/plugin/go.d/agent/jobmgr/filestatus_test.go`
  - `src/go/plugin/go.d/agent/jobmgr/manager_process_test.go`
- Extended:
  - `src/go/plugin/go.d/agent/module/job_v1_test.go`
- All tests were added in table-driven style (`map[string]struct{}`) per user preference.

### Test Results (Current Tree)
- Command:
  - `go test ./plugin/go.d/agent/jobmgr -run 'TestDyncfgConfig_ShutdownDoesNotQueue|TestDyncfgConfigUserconfig_InvalidPayload_Returns400Only|TestFileStatusContains_DoesNotDeadlock|TestRunProcessConfGroups_ChannelCloseDoesNotSpin' -count=1`
- Failures (verified as production bugs, not test expectation issues):
  1. `TestDyncfgConfig_ShutdownDoesNotQueue`: got 2 responses instead of 1.
     - Confirms `dyncfgConfig` sends shutdown 503 and still executes queue path.
     - Evidence: `src/go/plugin/go.d/agent/jobmgr/dyncfg.go:26`, `src/go/plugin/go.d/agent/jobmgr/dyncfg.go:31`.
  2. `TestDyncfgConfigUserconfig_InvalidPayload_Returns400Only`: got both 400 JSON and 200 YAML.
     - Confirms missing return after payload error in userconfig path.
     - Evidence: `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector.go:159`, `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector.go:165`.
  3. `TestFileStatusContains_DoesNotDeadlock`: timeout/deadlock in both table cases.
     - Confirms nested mutex lock in `contains -> lookup`.
     - Evidence: `src/go/plugin/go.d/agent/jobmgr/filestatus.go:85`, `src/go/plugin/go.d/agent/jobmgr/filestatus.go:89`, `src/go/plugin/go.d/agent/jobmgr/filestatus.go:97`.
  4. `TestRunProcessConfGroups_ChannelCloseDoesNotSpin`: closed channel case did not exit.
     - Confirms missing `ok` handling on channel receive.
     - Evidence: `src/go/plugin/go.d/agent/jobmgr/manager.go:257`.
- Command:
  - `go test ./plugin/go.d/agent/module -run 'TestJob_UpdateVnode_NilIgnored' -count=1`
- Failure:
  5. `TestJob_UpdateVnode_NilIgnored`: panic `nil pointer dereference`.
     - Confirms `UpdateVnode(nil)` can reach nil dereference in `processMetrics`.
     - Evidence: `src/go/plugin/go.d/agent/module/job_v1.go:293`, `src/go/plugin/go.d/agent/module/job_v1.go:516`.

## Implementation Status (Bug-Fix Pass for 5 Verified Defects)
- Production fixes implemented:
  1. `dyncfg` shutdown path now returns immediately after 503.
     - File: `src/go/plugin/go.d/agent/jobmgr/dyncfg.go`.
  2. `dyncfg` userconfig payload error path now returns after sending 400.
     - File: `src/go/plugin/go.d/agent/jobmgr/dyncfg_collector.go`.
  3. `fileStatus.contains` deadlock removed (no nested lock path).
     - File: `src/go/plugin/go.d/agent/jobmgr/filestatus.go`.
  4. `runProcessConfGroups` now exits on closed input channel.
     - File: `src/go/plugin/go.d/agent/jobmgr/manager.go`.
  5. `job_v1.UpdateVnode` now ignores nil vnode updates (parity with v2).
     - File: `src/go/plugin/go.d/agent/module/job_v1.go`.

### Verification Results After Fix
- Targeted tests:
  - `go test ./plugin/go.d/agent/jobmgr -run 'TestDyncfgConfig_ShutdownDoesNotQueue|TestDyncfgConfigUserconfig_InvalidPayload_Returns400Only|TestFileStatusContains_DoesNotDeadlock|TestRunProcessConfGroups_ChannelCloseDoesNotSpin' -count=1` ✅
  - `go test ./plugin/go.d/agent/module -run 'TestJob_UpdateVnode_NilIgnored' -count=1` ✅
- Full package tests:
  - `go test ./plugin/go.d/agent/jobmgr -count=1` ✅
  - `go test ./plugin/go.d/agent/module -count=1` ✅
- Race tests:
  - `go test -race ./plugin/go.d/agent/jobmgr ./plugin/go.d/agent/module -count=1` ✅

## Documentation Updates Required
- After tests land and gaps are closed, add/update one coverage status document for godv2 packages summarizing:
  - new tests added,
  - risk areas remaining,
  - consciously deferred items.
