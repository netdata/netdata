# SOW-20260614-netflow-facet-persistence - NetFlow Facet Persistence Disk I/O

## Status

Status: completed

Sub-state: Dirty tracking fix implemented and validated with targeted facet-runtime tests.

## Requirements

### Purpose

Reduce or eliminate unnecessary NetFlow facet-state serialization and disk writes while preserving facet autocomplete, restart recovery, and crash semantics.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers facet persistence disk I/O.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Active facet observation marks persisted state dirty even when the published contribution did not change.
- Dirty facet state serializes and writes the full persisted state.
- Live evidence recorded in the parent SOW showed unchanged facet-state payload hash while the file mtime advanced once per second.
- The existing active-observation paths already compute whether the published facet contribution changed.

Inferences:

- This is likely the highest-impact runtime optimization bucket.
- Dirty tracking is the clean fix because it avoids both redundant serialization and redundant disk writes.
- A hash-only write skip would avoid replacing identical bytes, but would still serialize the full persisted state on every false dirty tick.

Unknowns:

- None for this SOW. Broader facet cardinality costs are tracked in separate SOWs.

### Acceptance Criteria

- Add or verify tests for facet persistence when observations do not introduce new facet values.
- Add or verify tests for persistence after real new facet values.
- Add or verify restart/reload tests for persisted facet state.
- Implement the approved optimization without losing real facet changes.
- Prove redundant writes are reduced or eliminated in the production-shaped benchmark or equivalent targeted test.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/facet_runtime.rs`
- Parent inventory SOW.

Current state:

- Before this SOW, `observe_active_contribution()` and `observe_active_record()` called `mark_dirty()` unconditionally even when their existing `changed` checks were false.
- After this SOW, both active-observation paths mark dirty only when the durable/published facet contribution changes:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:317`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:318`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:336`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:341`
- Dirty state still serializes the full persisted state and writes a temp file before rename when there is a real durable change.

Risks:

- Incorrect dirty tracking can lose facet values after restart.
- Pure write-skip without fixing dirty tracking can leave CPU serialization overhead.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Low-rate traffic can trigger full unchanged facet-state serialization and rewrite because dirty state is set even when the durable state did not change.
- Root cause is not the persistence writer itself; it is false dirty marking in the active observation paths.

Evidence reviewed:

- Parent inventory SOW reviewer findings.
- `src/crates/netflow-plugin/src/facet_runtime.rs`.
- Existing facet runtime persistence/reload tests.
- New write-count tests using the existing facet disk-write hook.

Affected contracts and surfaces:

- Durable facet state file.
- Flow Function facet autocomplete.
- Restart recovery and crash window.
- Facet runtime tests.

Clean-end-state target:

- Facet persistence writes only when durable content actually changes, with tests covering unchanged observations, changed observations, and reload.
- Removed as redundant (i): unconditional active-observation dirty marking.
- Excluded coupled items (ii): facet in-memory cardinality costs are handled by `SOW-20260614-netflow-facet-runtime-cardinality.md`; chart-sampler facet memory walks are handled by `SOW-20260614-netflow-chart-sampler.md`.
- Reference search: no persisted file format, path, header, or public facet payload contract changed. Same-failure search was run for remaining dirty marking and persistence call sites.

Existing patterns to reuse:

- Existing facet runtime persistence/reload tests.
- Existing facet disk-write test hook.
- Existing persisted state hash code.

Risk and blast radius:

- Medium data/UX risk due to durable state and autocomplete.
- Low security risk with aggregate test data only.
- Low compatibility risk because the persisted format and public facet payload are unchanged.

Sensitive data handling plan:

- Use synthetic facet values only. Do not record real traffic values or IPs in durable artifacts.

Implementation plan:

1. Inspect existing facet persistence tests and identify missing edge cases.
2. Add tests proving unchanged active contribution and unchanged active record observations do not rewrite facet state.
3. Add/verify changed-value and reload coverage.
4. Move active-observation `mark_dirty()` calls behind the existing `changed` checks.
5. Validate with targeted tests and same-failure search.

Validation plan:

- Targeted Rust tests for facet persistence.
- Production-shaped low-rate benchmark or targeted write-count test.
- Same-failure search for facet writes and dirty marking.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if a durable facet-persistence invariant is created.
- End-user/operator docs: no update expected unless behavior/config becomes user-visible.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- Not required for this SOW because no persisted format, recovery contract, file path, public Function payload, or operator-facing configuration changed.

Open decisions:

- Resolved: dirty tracking was selected. Hash-only write skip was rejected for this SOW because it leaves the full-state serialization cost in place.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected: this bucket has its own SOW and must add/verify tests before behavior changes.
   - Recommendation classification: long-term-best.

2. Implementation decision: fix false dirty marking instead of adding a hash-only disk-write skip.
   - Selected: mark dirty only when the existing active-observation `changed` checks are true.
   - Evidence: unchanged active contribution and active record tests fail against unconditional dirty marking and pass after the dirty calls move behind `changed`.
   - Implication: unchanged traffic avoids both full-state serialization and disk rewrite.
   - Risk: a real new facet value could be lost if `changed` is wrong, so the changed-value reload test is mandatory.
   - Recommendation classification: long-term-best.

## Plan

1. Test gap audit for facet persistence.
2. Test additions.
3. Implementation decision.
4. Optimization and validation.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.
- No implementation files changed for this bucket.

### 2026-06-15

- Added unchanged active-contribution and unchanged active-record write-count tests in `src/crates/netflow-plugin/src/facet_runtime.rs`.
- Added changed active-contribution persistence/reload coverage in `src/crates/netflow-plugin/src/facet_runtime.rs`.
- Serialized the test-only facet disk-write hook so parallel tests cannot race on the global hook.
- Moved active-observation dirty marking behind existing `changed` checks.
- Rejected hash-only write skip for this SOW because it would still serialize unchanged full facet state.

## Validation

Acceptance criteria evidence:

- Unchanged active contribution no longer rewrites facet state:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:1385`
- Unchanged active record no longer rewrites facet state:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:1418`
- New active contribution values still persist and reload:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:1451`
- Existing compact-store, reconcile, stale snapshot, and reload tests still pass in the full facet-runtime test subset.

Tests or equivalent validation:

- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml runtime_skips_persistence_when_active -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml runtime_persists_and_reloads_new_active_contribution_value -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml runtime_persists_and_reloads_compact_field_stores -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml runtime_reconcile_persists_cleared_active_contributions -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml facet_runtime::tests:: -- --nocapture` (`20 passed; 1 ignored`)
- PASS: `git diff --check`
- Known pre-existing validation caveat: `cargo fmt --manifest-path src/crates/Cargo.toml --package netflow-plugin --check` still fails on unrelated package-wide formatting drift outside this SOW.

Real-use evidence:

- Targeted write-count tests prove the redundant facet-state rewrite is eliminated for unchanged active traffic. The production-shaped benchmark from the benchmark SOW can now expose lower fixed overhead when run against a large persisted facet state.

Reviewer findings:

- Parent inventory SOW findings apply.

Same-failure scan:

- Ran `rg -n "mark_dirty\\(&mut state\\)|observe_active_contribution\\(|observe_active_record\\(|persist_if_dirty\\(" src/crates/netflow-plugin/src/facet_runtime.rs src/crates/netflow-plugin/src/ingest src/crates/netflow-plugin/src/memory_tests.rs`.
- Remaining production `mark_dirty()` sites are reconciliation, rotation, deletion, and changed active-observation paths:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:257`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:262`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:271`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:276`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:281`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:319`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:342`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:370`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:417`
- Remaining `persist_if_dirty()` call sites are sync/shutdown paths and tests; no additional false-dirty source was found for this SOW.

Sensitive data gate:

- This SOW records only repo-relative code evidence and synthetic-test intent.

## Artifact Maintenance Gate

- AGENTS.md: no update needed; repository process rules did not change.
- Runtime project skills: no update needed; no reusable collector-authoring rule changed.
- Specs: no update needed; no durable persisted format or public contract changed.
- End-user/operator docs: no update needed; behavior is an internal optimization with unchanged configuration and user-visible semantics.
- End-user/operator skills: no update expected.
- SOW lifecycle: this active child SOW is completed for branch-local handoff and must not merge to `master`.

Specs update:

- Not needed.

Project skills update:

- Not needed.

End-user/operator docs update:

- Not needed.

End-user/operator skills update:

- Not needed.

Lessons:

- Dirty tracking should be preferred over payload hash skipping when the expensive part is building/serializing the payload, not only writing it.

Follow-up mapping:

- Parent inventory SOW tracks the remaining independent buckets: chart sampler, facet runtime cardinality, tier sync, decoder path, query payload, enrichment hot path, and raw journal sync.

## Outcome

Completed. Unchanged active facet observations no longer force full facet-state serialization or disk rewrite; real new facet values still persist and reload.

## Lessons Extracted

- Existing `changed` signals are the correct source of truth for active-observation dirty tracking.
- Global test hooks must be serialized when Rust tests may run in parallel.

## Follow-up Issues

None for this SOW.
