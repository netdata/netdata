# SOW-20260614-netflow-facet-runtime-cardinality - NetFlow Facet Runtime Cardinality Costs

## Status

Status: completed

Sub-state: Facet runtime cardinality cleanup implemented, externally reviewed, and validated.

## Requirements

### Purpose

Make in-memory facet runtime operations reasonable under high facet cardinality without weakening facet UX or persistence correctness.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers facet runtime cloning, rebuild, sorting, dead allocation, and active-contribution scan costs.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Publishing clones the full published facet snapshot.
- Rebuild clones archived stores, merges active stores, collects strings, and allocates field names.
- Text facet full collection sorts every time.
- `build_reconcile_plan()` allocates an unused active-path set.
- New active values may scan other active contributions.

Inferences:

- These are likely secondary to disk persistence at low rate but important for high-cardinality and rotation/reconcile spikes.

Unknowns:

- Which sub-items are worth fixing after measurement and tests.

### Acceptance Criteria

- Add or verify tests for published facet snapshots after active changes, rotation, deleted paths, and reconcile.
- Remove pure dead work if no contract depends on it.
- Reduce avoidable full clones/sorts/scans where tests prove equivalent behavior.
- Preserve exact facet value ordering and autocomplete behavior unless explicitly changed by user decision.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/facet_runtime.rs`
- `src/crates/netflow-plugin/src/facet_runtime/store.rs`
- Parent inventory SOW.

Current state:

- Parent inventory records specific file:line evidence for this bucket.

Risks:

- Published snapshot ordering and autocomplete behavior are user-visible through the Flow Function.
- Optimizing rebuild paths can accidentally omit active or archived values.

## Pre-Implementation Gate

Status: in-progress

Problem / root-cause model:

- Several facet runtime operations scale with total facet cardinality, not with the small delta being applied.

Evidence reviewed:

- Parent inventory SOW reviewer findings.

Affected contracts and surfaces:

- Flow Function facet payloads.
- Autocomplete.
- Reconcile after journal rotation/deletion.
- Facet runtime tests.

Clean-end-state target:

- Facet runtime avoids pure dead allocations and avoids full cardinality work where incremental or borrowed behavior can preserve the same contract.
- Removed as redundant (i): unused active-path allocation in reconcile planning.
- Reduced as redundant/costly: archived-only published snapshot rebuild now borrows archived stores instead of cloning every archived store; active duplicate values already present in archived stores now skip the cross-active contribution scan.
- Excluded coupled items (ii): disk-write skip/dirty tracking belongs to facet persistence SOW; query payload response construction belongs to query SOW; full published snapshot structural sharing and sorted text-store caching are not included because they require an internal representation redesign and a separate decision about whether to shift cost into inserts, add caches, or add field-level sharing.
- Reference search: required if public facet payload contract or field ordering changes.

Existing patterns to reuse:

- Existing facet runtime tests.
- Existing facet store APIs and value ordering helpers.

Risk and blast radius:

- Medium UX risk due to facet payload/autocomplete.
- Medium performance risk if data structures become more complex.

Sensitive data handling plan:

- Use synthetic facet values in tests and SOW evidence.

Implementation plan:

1. Audit existing facet runtime test coverage.
2. Add edge-case tests for rebuild/publish/reconcile behavior.
3. Fix pure dead work first.
4. Optimize clone/sort/scan paths only where tests and measurements justify it.

Validation plan:

- Targeted facet runtime tests.
- Production-shaped benchmark comparison if available.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if durable facet runtime invariants are added.
- End-user/operator docs: no update expected unless contract changes.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- Not checked yet; local behavior and tests define this bucket unless a data-structure redesign is proposed.

Open decisions:

- None for the initial cleanup and focused optimizations that preserve facet payload ordering and autocomplete behavior. A broader data-structure redesign remains a user-owned fork and would require a separate decision.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

2. User decision: continue with the next priority bucket after rebasing and pushing the current PR.
   - Selected on 2026-06-15.
   - Scope: activate this child SOW for test-first facet runtime cardinality cleanup while preserving existing facet payload ordering and autocomplete behavior.
   - Implication: pure dead work and low-risk internal optimizations are approved; public facet contract changes are not approved.
   - Risk: changes around rebuild/publish paths can regress Flow Function facet UX if tests miss ordering or active/archive interactions.
   - Recommendation classification: long-term-best.

## Plan

1. Coverage audit.
2. Test additions.
3. Dead-work cleanup and measured optimizations.
4. Validation.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Rebased branch `netflow-overheads` onto `upstream/master` and pushed commit `d22f704d75`.
- Activated this SOW as priority 4 after user instruction to continue.
- Re-read the NetFlow collector guidance and relevant SOW specs before implementation.
- Baseline facet runtime tests passed before changes: 21 passed, 1 ignored.
- Added focused tests for archived duplicate active values and cross-active duplicate values.
- Removed the unused active-path `BTreeSet` allocation from reconcile planning.
- Changed active value insertion so archived duplicates skip the cross-active contribution scan while preserving duplicate retention for later rotation.
- Changed archive-only published snapshot rebuild to borrow archived stores directly instead of cloning them before collecting published values, with the empty-active branch hoisted outside the field loop.
- Added focused coverage for archive-only rebuild of high-cardinality archived text values crossing the autocomplete threshold.
- Added a short comment documenting why archived duplicates return before the cross-active contribution scan.

## Validation

Acceptance criteria evidence:

- Active changes: `runtime_active_duplicate_of_archived_value_does_not_republish_or_persist`, `runtime_active_duplicate_across_active_paths_counts_once`, and existing dirty/persistence tests cover active contribution behavior.
- Rotation: existing rotation and sidecar tests still pass after the archive-only rebuild borrowing path.
- Deleted paths: existing archived-deletion rebuild test still passes.
- Reconcile: existing cleared-active reconcile test still passes.
- Archive-only rebuild: `archive_only_published_rebuild_uses_archived_values_and_autocomplete_threshold` covers non-empty archived text values with empty active contributions and verifies autocomplete promotion semantics.
- Pure dead work: same-failure scan found no remaining `current_active_paths` allocation.
- Public behavior: no facet field names, payload ordering, autocomplete policy, persisted file path, or persisted schema changed.

Tests or equivalent validation:

- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml facet_runtime::tests:: -- --nocapture`: 24 passed, 1 ignored.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml production_shaped_resource_layer_ -- --nocapture`: 2 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`: 513 passed, 25 ignored; `tests/grpc_build.rs`: 1 passed.
- Short manual low-rate production-shaped benchmark:
  - command shape: production-shaped layer, low-cardinality mixed profile, 30 flows/s, 2s warmup, 5s measurement.
  - result: achieved about 30 flows/s, CPU about 0.20% of one core, sync tick about 88.8 usec/s, chart sampler about 4.94 usec/s.
- `git diff --check`: passed.

Real-use evidence:

- Not collected for this internal runtime cleanup. Synthetic tests cover the changed contracts; the low-rate production-shaped benchmark covers resource-envelope regressions.

Reviewer findings:

- External reviewers run for this SOW before push: glm, minimax, kimi, mimo, deepseek, and qwen.
- Overall verdict: production-grade; no blocking findings.
- Accepted findings:
  - Added focused archive-only high-cardinality autocomplete coverage after reviewers identified the missing direct fast-path test.
  - Hoisted the empty-active branch outside the facet-spec loop after reviewers identified it as a loop-invariant branch.
  - Added a short comment for the reordered active duplicate scan after minimax identified the equivalence as subtle for future maintainers.
- Non-blocking findings left intentionally unchanged:
  - `published_field_from_store()` helper style and threshold logic duplication are acceptable in this narrow cleanup; a more general published-field construction redesign belongs with broader structural-sharing work.
  - Full published snapshot structural sharing, sorted text-store caching, and field-name allocation reductions remain excluded because they require a separate internal representation decision.
  - Package-wide rustfmt drift and unrelated dead-code warnings are pre-existing and outside this SOW.

Same-failure scan:

- `rg` scan found no remaining `current_active_paths` allocation and only the intended `active_value_exists_elsewhere()` call site remains.

Sensitive data gate:

- No sensitive data recorded.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update needed.
- Specs: no update needed; no durable public contract or cross-SOW invariant changed.
- End-user/operator docs: no update needed; behavior is an internal optimization with unchanged configuration and user-visible semantics.
- End-user/operator skills: no update needed.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- No durable spec update needed; no public contract or cross-SOW invariant changed.

Project skills update:

- Not needed.

End-user/operator docs update:

- No end-user/operator docs update needed; no operator-facing config or behavior changed.

End-user/operator skills update:

- Not needed.

Lessons:

- Borrowing archived stores is safe only for archive-only rebuilds. Active overlays still require a union structure to preserve exact distinct counts and sorted published values.

Follow-up mapping:

- Parent inventory SOW tracks ordering.

## Outcome

Completed. Facet runtime now avoids the unused reconcile active-path allocation, skips cross-active duplicate scans for archived duplicates, and avoids archived-store cloning when rebuilding a published snapshot with no active contributions. Public facet payloads, ordering, autocomplete behavior, and persistence format are unchanged.

## Lessons Extracted

- Archive-only rebuilds can borrow archived stores safely because published snapshots own their collected values.
- Duplicate active values must still be retained per active path for later rotation, even when they do not change the published facet snapshot.
- Small hot-path reorderings need a short `why` comment when equivalence depends on helper behavior such as excluding the current path.

## Follow-up Issues

None for this SOW.
