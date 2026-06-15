# SOW-20260614-netflow-tier-sync - NetFlow Tier Sync And Open-Tier Overhead

## Status

Status: completed

Sub-state: Implementation validated locally and by final external reviewer rerun; source changes ready to push.

## Requirements

### Purpose

Make tier sync, open-tier snapshots, and tier-index pruning reasonable under low-rate and high-cardinality workloads without weakening tier commit correctness.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers tier sync/open-tier overhead.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Sync tick prunes tier flow indexes and refreshes open-tier state.
- Open-tier refresh allocates row vectors.
- Active-hour pruning scans flow refs and then takes a write lock.
- Existing tier worker spec records design intent, but design intent is not proof of acceptable cost.

Inferences:

- The tier sync bucket should be decomposed into open-row allocation, active-hour scan, in-flight-hour scan, and write-lock hold time.

Unknowns:

- Real cost at representative open rollup cardinality.

### Acceptance Criteria

- Add or verify tests for open-tier snapshot correctness and prune correctness.
- Measure or test each tier-sync sub-bucket.
- Reduce unnecessary O(open cardinality) work where possible.
- Preserve tier commit worker correctness and query visibility for open tiers.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/ingest/service/tiers.rs`
- `src/crates/netflow-plugin/src/tiering/index/accumulator.rs`
- `.agents/sow/specs/netflow-tier-commit-workers.md`
- Parent inventory SOW.

Current state:

- Parent inventory records exact tier sync evidence.

Risks:

- Open-tier query accuracy can regress if snapshots are made approximate without a contract decision.
- Pruning can break tier indexes if active/in-flight hours are missed.

## Pre-Implementation Gate

Status: completed

Problem / root-cause model:

- Tier sync performs O(open rollup cardinality) work periodically even when the live workload may not need full row copies.
- Open-tier refresh rebuilds fresh row vectors every tick and then swaps them into the published `OpenTierState`, so high open-tier cardinality can allocate repeatedly even when the current contract only needs a fresh owned snapshot.
- Prune currently builds one temporary active-hour set per accumulator before extending the final active-hour set.

Evidence reviewed:

- Parent inventory SOW.
- Tier commit worker spec.
- `src/crates/netflow-plugin/src/ingest/service/runtime.rs`
- `src/crates/netflow-plugin/src/ingest/service/tiers.rs`
- `src/crates/netflow-plugin/src/tiering/index/accumulator.rs`
- `src/crates/netflow-plugin/src/tiering/model.rs`
- Existing tier accumulator, tier worker, open-tier query, and benchmark tests.

Affected contracts and surfaces:

- Open-tier query behavior.
- Tier flow index lifecycle.
- Tier commit worker handoff invariants.
- NetFlow internal charts.

Clean-end-state target:

- Tier sync maintains correctness with measured and justified work per tick.
- Removed/reduced as redundant (i): repeated open-tier row vector allocation on each refresh; per-accumulator temporary active-hour sets during prune.
- Excluded coupled items (ii): chart sampler cadence belongs to chart sampler SOW; benchmark harness belongs to benchmark SOW; changing the one-second open-tier freshness contract is not included because it is a user-visible Function/query behavior decision; changing tier worker doorbell or commit semantics is not included because the tier worker spec makes those invariants load-bearing.
- Reference search: no open-tier query contract or chart dimension change is planned. Search covered `refresh_open_tier_state`, `snapshot_open_rows`, `active_hours`, `OpenTierState`, and `open_tiers` references before implementation.

Existing patterns to reuse:

- Existing tier accumulator tests.
- Existing tier worker tests and spec.
- Existing `handle_sync_tick_for_test()`.

Risk and blast radius:

- Medium correctness risk for tier queries and pruning.
- Medium performance risk at high cardinality.

Sensitive data handling plan:

- Use synthetic flow refs and aggregate counts only.

Implementation plan:

1. Audit tier sync/open-tier/prune tests.
2. Add edge-case tests for in-flight hours, active hours, and open-tier visibility.
3. Measure current sub-buckets.
4. Optimize only with tests proving equivalent behavior.

Validation plan:

- Targeted tiering tests.
- Production-shaped benchmark at low and high cardinality.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update `netflow-tier-commit-workers.md` if invariants change.
- End-user/operator docs: no update expected unless behavior changes.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- Not checked yet; local tier worker spec is the primary reference unless a data-structure redesign is proposed.

Open decisions:

- None for the internal scratch-buffer and active-hour temporary reduction because they preserve the published open-tier snapshot contract. Any change to open-tier freshness, row timestamps, worker handoff timing, or prune cadence remains a separate user-owned decision.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

2. Implementation decision: preserve the one-second open-tier snapshot contract while reducing allocation churn.
   - Selected: clear and refill the published `OpenTierState` vectors in place under the existing write lock.
   - Evidence: current `refresh_open_tier_state()` builds a fresh `OpenTierState` and assigns it to the `RwLock`; `TierAccumulator::snapshot_open_rows()` returns a new `Vec` for each tier.
   - Implication: row vector capacity is retained and counted by existing open-tier memory diagnostics; row timestamp/open-bucket filtering behavior stays unchanged.
   - Risk: open-tier readers can block while the refresh refills row vectors. This does not block ingest, but tests must cover destination clearing, capacity retention, and published state correctness.
   - Recommendation classification: surgical.

3. Implementation decision: avoid per-accumulator active-hour temporary sets.
   - Selected: let accumulators extend one caller-owned active-hour set.
   - Evidence: current `active_hours()` allocates a `BTreeSet` per accumulator and the caller extends another set from it.
   - Implication: prune still scans active flow refs and still includes worker in-flight hours; only temporary set construction changes.
   - Risk: missing hours would prune flow-index entries needed by open or in-flight buckets, so tests must cover existing and appended hours.
   - Recommendation classification: surgical.

## Plan

1. Test audit.
2. Sub-bucket measurement.
3. Tests and optimization.
4. Validation and spec update if needed.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Activated this SOW after completing the facet runtime cardinality SOW.
- Re-read NetFlow collector guidance and the tier commit worker spec.
- Audited sync tick, open-tier snapshot, prune, tier accumulator, and open-tier query references.
- Replaced `TierAccumulator::snapshot_open_rows()` production use with `snapshot_open_rows_into()` so the caller can reuse row-vector capacity.
- Refilled the published `OpenTierState` in place under the existing `open_tiers` write lock, preserving memory-accounting visibility and avoiding hidden scratch buffers.
- Replaced per-accumulator `active_hours()` temporary sets with `extend_active_hours()` into the prune's caller-owned set.
- Added focused tests for open-row destination clearing/capacity reuse, open-tier state capacity retention, and active-hour extension.
- Added an ingest-level concurrent-reader test for open-tier publication so a reader can observe only the old empty snapshot or the complete refreshed snapshot, never a partial generation.
- Added a memory-diagnostics assertion proving retained open-tier row capacity remains counted after clearing.
- Added an ingest-level prune test proving `prune_unused_tier_flow_indexes()` removes inactive closed hours and keeps open hours materializable.
- Added edge-case coverage for empty, all-closed, and zero-capacity `snapshot_open_rows_into()` destinations.
- Restored `prune_unused_tier_flow_indexes()` and `refresh_open_tier_state()` to `&self`; their observable mutations are through existing interior mutability and all current call sites remain valid.
- Documented the `open_tiers` field reader contract: production readers should use non-blocking reads and fall back on contention because refresh refills under the write lock.

## Validation

Acceptance criteria evidence:

- Open-tier snapshot correctness:
  - Existing open-tier chart/query tests still pass.
  - Existing `take_closed_buckets_*` tests still pass.
  - New `snapshot_open_rows_into_clears_destination_and_reuses_capacity` verifies stale destination rows are cleared and capacity is retained.
  - New `open_tier_state_clear_retain_capacity_keeps_row_buffers` verifies published state buffers are cleared without losing capacity.
  - New `refresh_open_tier_state_publishes_complete_snapshots_under_concurrent_reads` verifies concurrent readers cannot observe a partially refilled published snapshot.
- Prune correctness:
  - New `extend_active_hours_preserves_existing_hours_and_adds_accumulator_hours` verifies caller-owned active-hour sets preserve pre-existing in-flight hours and add all accumulator hours.
  - New `prune_unused_tier_flow_indexes_removes_inactive_hours_and_keeps_open_hour` verifies the ingest service removes inactive closed-hour flow-index entries while preserving open-hour materialization.
  - Existing tier worker and rebuild tests still pass in the full package suite.
- Public behavior:
  - No open-tier row fields, query behavior, chart dimensions, tier commit handoff semantics, or prune cadence changed.

Tests or equivalent validation:

- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml prune_unused_tier_flow_indexes_removes_inactive_hours_and_keeps_open_hour -- --nocapture`: 1 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml refresh_open_tier_state_publishes_complete_snapshots_under_concurrent_reads -- --nocapture`: 1 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml tiering::tests:: -- --nocapture`: 15 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml open_tier -- --nocapture`: 8 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`: 519 passed, 25 ignored; `tests/grpc_build.rs`: 1 passed.
- `git diff --check`: passed after the final test/comment update.
- Short low-rate production-shaped benchmark:
  - command shape: production-shaped layer, low-cardinality mixed profile, 30 flows/s, 2s warmup, 5s measurement.
  - result: achieved about 30 flows/s, CPU about 0.20% of one core, sync tick about 64.2 usec/s and 64.2 usec/call, chart sampler about 4.47 usec/s.
- Short high-cardinality production-shaped benchmark:
  - command shape: production-shaped layer, high-cardinality mixed profile, 5,000 flows/s, 1s warmup, 3s measurement.
  - result: achieved about 5,000 flows/s, CPU about 15.7% of one core, sync tick about 291.5 usec/s and 291.5 usec/call, chart sampler about 5.76 usec/s.

Real-use evidence:

- Not collected from a live deployment. Synthetic tests and production-shaped benchmarks cover the changed internal sync paths.

Reviewer findings:

- Parent inventory SOW findings apply.
- `glm`: production-grade; no blockers. Low notes: in-place refill widens write-lock hold time, high-water buffer retention is monotonic but memory-accounted, and `&mut self` on prune is cosmetic.
- `mimo`: production-grade; no blockers. Low note: document the write-lock/reader behavior and avoid future blocking readers on `open_tiers`.
- `qwen`: production-grade; no blockers. Low note: add a why comment for capacity retention and optionally add a read-vs-refill test.
- `kimi`: production-grade with reservations; no blockers for push. Actioned recommendation: added concurrent-reader publication test before declaring this SOW complete.
- `minimax`: first run returned no usable final verdict after context gathering; superseded by final rerun below.
- `deepseek`: first run ended before a final verdict was retrievable; superseded by final rerun below.
- Final review rerun after prune/edge-test fixes:
  - `glm`: production-grade; no blockers. Notes: high-water capacity is monotonic by design; double-clear is harmless; no correctness/security issues.
  - `minimax`: production-grade; no blockers. Notes: optional worker-mode prune integration test and A/B benchmark row, both non-blocking because worker handoff was unchanged and this SOW records envelope validation.
  - `kimi`: production-grade; no blockers. Notes: no correctness/security issues; memory-ordering and contention comments are speculative/non-blocking.
  - `mimo`: production-grade; no blockers. Notes: optional mixed open/closed direct `snapshot_open_rows_into` test; existing edge and integration tests cover the behavior.
  - `qwen`: production-grade; no blockers. Note: facet-runtime commit was noticed in branch history but is covered by a separate completed SOW, not this tier-sync SOW.
  - `deepseek`: production-grade; no blockers. Notes: monotonic retained capacity is intentional and memory-accounted.

Same-failure scan:

- `rg` scan found remaining `snapshot_open_rows()` references only in tests and the wrapper method.
- `rg` scan found production `refresh_open_tier_state()` and `prune_unused_tier_flow_indexes()` call sites are still the expected rebuild/tick maintenance paths.

Sensitive data gate:

- No sensitive data recorded.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: not needed; tier worker handoff invariants and public behavior did not change.
- End-user/operator docs: not needed; no operator-visible behavior or configuration changed.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- Not needed. Tier worker handoff invariants and public behavior did not change.

Project skills update:

- Not needed. No runtime project skill behavior changed.

End-user/operator docs update:

- Not needed. No public behavior, configuration, metric name, or chart dimension changed.

End-user/operator skills update:

- Not needed. No operator workflow changed.

Lessons:

- Open-tier row buffers are now intentionally retained in the published state so the memory-diagnostics proxy remains honest; hidden scratch buffers were avoided for this reason.

Follow-up mapping:

- Parent inventory SOW tracks ordering.
- No new blocker follow-up is required for this SOW. A shrink heuristic for retained high-water open-tier buffers remains a future design choice only if production evidence shows long-lived RSS pressure after cardinality spikes.

## Outcome

Completed.

- Open-tier refresh reuses published row-vector capacity instead of allocating fresh row vectors every sync tick.
- Tier prune reuses one caller-owned active-hour set instead of allocating per-accumulator temporary sets.
- Published open-tier memory diagnostics remain honest because retained high-water row buffers are counted.
- Tests cover row-vector clearing/reuse, empty/closed/zero-capacity snapshot destinations, active-hour extension, end-to-end prune behavior, concurrent open-tier publication, and full package behavior.

## Lessons Extracted

- Avoid hidden scratch buffers for memory reductions when the product needs lightweight real-time memory proxies; retained published buffers are preferable here because diagnostics can count them directly.
- A write-lock refill trade-off is acceptable only while production readers use non-blocking reads with fallback. The field-level comment records this contract for future readers.
- Reviewer reruns were useful: the first clean review round still missed the ingest-level prune test gap.

## Follow-up Issues

None yet.
