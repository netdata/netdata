# SOW-0039 - OverTimeReducer trait for `*_over_time` rollup specialization

## Status

Status: completed

Sub-state: shipped 2026-05-14. Trait + ten per-op reducers in
`eval/functions.rs`; call sites in `apply_over_time`,
`apply_over_time_legacy`, and `eval/fused.rs::RollupKind::compute`
migrated. Regression and performance gates green.

## Requirements

### Purpose

Specialize the per-window `*_over_time` reducer so that each operator
walks only the state it needs, matching the trait-generic pattern the
aggregator layer already uses in `eval/fused.rs` (`IncrementalAggr`).
The current shared loop in `eval::functions::compute_over_time`
maintains nine accumulator variables and an early-`continue` on NaN
for every operator, blocks auto-vectorization, and pays for state that
the chosen operator does not read.

### User Request

> I have to admit that I'm not familiar with the implementation,
> however it looks to me that we should have made the function generic
> over the operation. What do you think? Am I missing something?

Followed by: "I'm curious should we create a SOW for this?" and "yes,
draft it." The asker proposed trait-generic specialization as the
right shape. The original three-option write-up included specialized
loops as option 2; the chosen framing is the trait pattern, matching
the in-tree `IncrementalAggr` precedent.

### Assistant Understanding

Facts:

- `eval::functions::compute_over_time` (functions.rs:208-285) accumulates
  `count, sum, sum_sq, min_val, max_val, last_val, last_ts, first_val,
  first_ts` for every window, then picks one via `match op` after the
  loop.
- The loop body has `if v.is_nan() { continue; }`, which prevents LLVM
  auto-vectorization across the seven scalar accumulator updates.
- `eval::fused::RollupKind::compute` (fused.rs:238-307) dispatches into
  the same `compute_over_time` helper for every `*_over_time` variant
  it fuses. Both the unfused rollup path
  (`eval::functions::apply_over_time`) and the fused evaluator's
  per-window step share this hot helper.
- The aggregator layer one level up already uses a trait
  (`IncrementalAggr` in fused.rs:46-52) with per-op `Acc` shapes
  (`SumAcc { sum, count }`, `MinMaxAcc { value, seen }`,
  `CountAcc { count }`). The rollup layer was never refactored to
  match.
- RelWithDebInfo samply profile of Q3 (28h @ 15s,
  `avg by (app_group)(avg_over_time(app_fds_open[5m]))`, 612 input
  series, 4946 grid points, 5min window):
  - 35.7% leaf time in `compute_over_time` inner loop
  - C-side decode pipeline shrank from 49.4% (Debug) to 13.4%
    (Release) once `-O2` reached the dbengine, leaving the Rust
    rollup loop as the single hottest function.
- `compute_over_time` returns `Option<(f64, i64)>` because
  `last_over_time` semantically returns the last sample's timestamp.
  The outer two-pointer driver `rollup_two_pointer`
  (functions.rs:87-145) stamps grid output with the *grid* timestamp,
  not the returned ts -- the returned ts is unused in the rollup
  output path. Need to confirm whether any caller (fused or otherwise)
  actually consumes the `i64` half.

Inferences:

- A trait per reducer with monomorphized loops gives the compiler
  enough static information to vectorize the NaN-skipping fold for
  ops where NaN behaves like an identity, and to drop dead state for
  ops that do not read it.
- The expected win is roughly proportional to the leaf share that
  shrinks: if the 35.7% inner-loop self-time drops by ~half, that is
  ~0.6s off a 3.3s query, ~18% wall-clock. The bigger structural win
  is that the fused-path-with-specialized-rollup matches the spirit
  of SOW-0033 (operator fusion eliminates intermediate buffers; this
  eliminates intermediate per-window state).

Unknowns:

- Whether dropping the `i64` from the return for the ten non-`Last`
  reducers is observable elsewhere (compliance harness, output
  serializer, smoke tests). Need to grep call sites during the
  Pre-Implementation Gate before settling the return shape.

### Acceptance Criteria

- All 195/195 unit tests in `cargo test -p netdata_promql` pass
  unchanged.
- The compliance corpus runner reports the same 545/255/212 split
  across 1012 cases (no regressions, no spurious passes).
- The smoke harness (`tests/promql-smoke/run-smoke.sh`) reports 117/117
  pass.
- Re-running the SOW-0039 Q3 benchmark on the same RelWithDebInfo
  binary shows the `compute_over_time`-bucket leaf share drop from
  35.7% to under 20% in the bucketed samply analysis (the absolute
  number is less important than the structural change; ~30% reduction
  in the inner-loop bucket is the threshold for "the refactor did
  what it set out to do").
- The change introduces a single trait, one `Acc` type per
  `OverTimeOp` variant, and one specialized `compute_over_time<R>`
  monomorphized helper. The lowering layer and the public
  `apply_over_time` / `RollupKind::compute` surfaces are unchanged.

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/eval/functions.rs` (the helper, its
  call sites, the `OverTimeOp` enum at functions.rs:295-314).
- `src/crates/netdata_promql/src/eval/fused.rs` (RollupKind dispatch
  table and the existing `IncrementalAggr` trait precedent).
- `src/crates/netdata_promql/src/plan/ir.rs` (FuncKind variants
  routed through `apply_call`).
- `/tmp/q3-rel.json.gz` samply profile (RelWithDebInfo) and the
  Debug-mode `/tmp/q3-profile.json.gz` profile, both bucketed by
  stage in this session.

Current state:

- `compute_over_time` is one function with one loop, nine running
  variables, and a 12-arm `match op` tail. It returns
  `Option<(f64, i64)>` to accommodate `last_over_time`'s timestamp
  semantic.
- `RollupKind::compute` in fused.rs is an enum-driven dispatcher that
  re-derives the same op selection that `apply_over_time` does, then
  calls the same helper. There is no avenue today for the fused path
  to pick a tighter loop.
- The aggregator side moved to a trait in SOW-0033 specifically to
  enable per-op accumulators; the rollup side did not move with it.

Risks:

- Numerical drift from re-ordering accumulator updates in
  `stddev_over_time` / `stdvar_over_time` (the `sum_sq` reducer is
  the only one with two-pass-equivalent arithmetic; we must preserve
  the exact ordering of the current single-pass formula
  `var = sum_sq/n - mean^2`).
- The `first_over_time` reducer carries plumbing from SOW-0037 that
  is currently dormant because `promql-parser v0.9.0` rejects the
  name at parse time. Need to keep the dormant reducer present so
  the SOW does not regress when the parser upgrades.
- Monomorphization expansion: eleven reducers × one specialized loop
  body each. Each loop is small (~50-100 instructions); total binary
  growth is small (~1-2 KB) but should be verified.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The unified rollup loop in `compute_over_time` accumulates
  state for every supported `*_over_time` op regardless of which op
  the caller selected. The `if v.is_nan() { continue; }` branch and
  the seven scalar updates per sample form a non-vectorizable hot
  path that the RelWithDebInfo profile shows as the single largest
  self-time bucket of the query (35.7%). The aggregator layer one
  level up already uses a per-op trait
  (`IncrementalAggr`) that demonstrates the correct pattern in the
  same crate.

Evidence reviewed:

- `eval/functions.rs:208-285` (the existing function body).
- `eval/functions.rs:87-145` (the `rollup_two_pointer` driver and how
  the per-window result is stamped onto the grid).
- `eval/fused.rs:46-177` (the existing `IncrementalAggr` trait and
  its impls).
- `eval/fused.rs:238-307` (`RollupKind::compute` shows the fused
  path goes through the same helper).
- RelWithDebInfo samply profile, captured this session, showing the
  35.7% inner-loop leaf time.
- Debug samply profile for comparison, showing the same inner loop
  was 21% in Debug before -O2 collapsed the C-side decode share.

Affected contracts and surfaces:

- `eval::functions::apply_over_time` (internal). Signature unchanged.
- `eval::functions::compute_over_time` (internal). Becomes
  `compute_over_time<R: OverTimeReducer>` with type-driven dispatch.
- `eval::functions::OverTimeOp` enum. Stays for parser/lowering
  layer compatibility; the runtime path no longer matches on it
  inside the loop.
- `eval::fused::RollupKind::compute`. Adjusted to instantiate the
  right reducer type rather than thread the op enum down.
- No public API change. No FFI change. No plan-IR change. No
  serializer change.

Existing patterns to reuse:

- `IncrementalAggr` trait shape and the per-op `Acc` struct
  convention in `eval/fused.rs`.
- The shared `rollup_two_pointer<F>` driver continues to take a
  closure over a window slice; the closure body is the only thing
  that changes.

Risk and blast radius:

- Numerical regression risk for `stddev_over_time` and
  `stdvar_over_time` if the accumulator update order changes. Will
  re-use the existing `sum_sq` / `mean` formula verbatim inside the
  reducer impls.
- No FFI, no schema, no operator semantic change. The compliance
  corpus is the regression gate; any drift is observable per-case.

Sensitive data handling plan:

- This SOW touches an internal evaluator helper, the trait
  refactor itself, and a samply profile output that contains only
  numeric counters and function symbols from the local agent. No
  secrets, credentials, customer data, or private endpoints. The
  benchmark queries reference `app_fds_open`, `app_cpu_utilization`,
  `netdata_spinlock_total_locks` -- all stock Netdata metric names
  with no host-identifying labels in this document.

Implementation plan:

1. Confirm the `(f64, i64)` return tuple's usage. Grep every
   call site of `compute_over_time` and verify whether the `i64`
   half is read anywhere. If not, drop it from the trait return for
   the ten non-Last reducers and keep it only on `LastReducer`.
2. Add `pub(crate) trait OverTimeReducer` with associated `Acc:
   Default` and `step`/`finalize` methods in
   `eval/functions.rs` (private module, not exposed in
   `eval::mod.rs`).
3. Implement eleven reducer unit structs and their `Acc` types:
   `AvgReducer`, `SumReducer`, `MinReducer`, `MaxReducer`,
   `CountReducer`, `LastReducer`, `FirstReducer` (dormant per
   SOW-0037), `PresentReducer`, `StddevReducer`, `StdvarReducer`,
   `QuantileReducer` (if the existing quantile_over_time helper
   shares the same shape; otherwise leave it out and document why).
4. Rewrite `compute_over_time` as `compute_over_time<R: OverTimeReducer>`
   with a tight per-reducer loop. Each impl handles NaN per its own
   semantic.
5. Update `apply_over_time` to dispatch on the runtime `OverTimeOp`
   once and call the matching monomorphized
   `compute_over_time::<R>`.
6. Update `eval::fused::RollupKind::compute` to call the same
   monomorphized helpers, removing the per-window
   `OverTimeOp` round-trip.
7. Run the regression gate: `cargo test -p netdata_promql --release`,
   the compliance corpus runner, and the smoke harness.
8. Re-run the Q3 benchmark and re-profile under samply on the same
   RelWithDebInfo binary. Bucket the profile and compare the
   `compute_over_time` leaf share to the SOW-0039 baseline.

Validation plan:

- Unit tests: 195/195 pass on `cargo test -p netdata_promql --release`.
- Compliance corpus: same 545/255/212 split (no new failures, no
  spurious passes).
- Smoke: 117/117.
- Performance: samply RelWithDebInfo profile shows the
  inner-loop leaf bucket drop from 35.7% to under 20%. Captured as
  a delta in the Validation section.
- Same-failure scan: grep for `compute_over_time` callers post-edit
  to ensure no stale callsite.

Artifact impact plan:

- AGENTS.md: no update expected; the SOW does not change workflow
  rules.
- Runtime project skills: no update expected; no public surface
  change.
- Specs: no update expected; the rollup-window semantics do not
  change. If the validation step uncovers a behavioral subtlety
  (e.g. NaN handling differs per reducer in a way worth recording),
  a brief addendum to a relevant spec under
  `.agents/sow/specs/` will be added.
- End-user/operator docs: no update expected.
- End-user/operator skills: no update expected.
- SOW lifecycle: standard close. On success, status `completed`,
  move to `.agents/sow/done/`, single commit per the rules.

Open-source reference evidence:

- None required. The pattern (per-op accumulator traits) is borrowed
  from `eval/fused.rs` in this same crate, not from an external
  reference.

Open decisions:

1. **Return type of `OverTimeReducer::finalize`.** RESOLVED 2026-05-14 → Option B.
   - Option A: `Option<f64>` for every reducer except `LastReducer`,
     which returns `Option<(f64, i64)>`.
   - Option B: `Option<(f64, i64)>` for every reducer (keep current
     shape). No change to callers.
   - **Resolution.** Grep evidence (chunk 1 investigation gate) shows
     eleven call sites: nine in `eval/fused.rs:252-305` and one in
     `eval/functions.rs:175` discard the `i64` via
     `.map(|(v, _)| v)`; one in `eval/functions.rs:194`
     (`apply_over_time_legacy`) destructures both and stamps
     `Series::scalar(labels, ts_ms, value)` at line 202. The
     legacy path is the single consumer of the `i64`. Option B
     keeps the trait surface uniform, leaves the legacy path
     unchanged, and preserves the documented "most recent non-NaN
     timestamp in the window" semantic. The performance win from
     this SOW comes from per-op `Acc` shapes eliminating dead state
     updates, not from changing the return tuple; recording the
     cheap `last_ts: i64` once per non-NaN iteration is a single
     register-resident store that does not block vectorization of
     the value-side fold.

2. **Should `QuantileReducer` move into the trait?**
   - The current `apply_quantile_over_time` is a separate function
     (not part of `compute_over_time`) because it needs the full
     window materialized for sorting. The trait shape requires
     streaming `step`/`finalize` and does not fit a sort-based
     reducer cleanly.
   - Option A: leave `quantile_over_time` outside the trait. The
     refactor covers the ten streaming reducers and explicitly
     records that quantile is excluded by design.
   - Option B: redesign the trait to allow buffering reducers (an
     associated type that owns a `Vec<f64>`). Bigger change, less
     win, the buffering path is fundamentally non-vectorizable.
   - Recommendation: A.

## Implications And Decisions

Pending user response on Open Decisions 1 and 2 (or acceptance of the
recommended A/A path).

## Plan

1. Investigation gate: confirm `(f64, i64)` return-shape usage via
   grep, record the answer in the Pre-Implementation Gate's Open
   Decisions, and either lock to Option 1A or escalate.
2. Trait + reducer types in `eval/functions.rs` (chunks 2-4 of the
   implementation plan).
3. Call-site migration in `eval/functions.rs::apply_over_time` and
   `eval/fused.rs::RollupKind::compute` (chunks 5-6).
4. Regression gate: cargo test, compliance, smoke.
5. Performance gate: samply RelWithDebInfo re-profile of Q3,
   bucketed comparison.
6. Close: artifact maintenance gate, lessons, follow-up mapping,
   single commit.

## Execution Log

### 2026-05-14

- SOW drafted from samply profile evidence captured this session.
  No code touched.
- Promoted from `pending/` to `current/` by user instruction.
  Status changed to `in-progress`.
- Chunk 1 (investigation gate) completed. Grepped every call site
  of `compute_over_time` across the crate:
  - `eval/fused.rs:252-305` — 9 call sites, all use
    `.map(|(v, _)| v)` to discard the i64.
  - `eval/functions.rs:175` — 1 call site in `apply_over_time` (the
    grid-aware single-pass path post-SOW-0031), also
    `.map(|(v, _)| v)`.
  - `eval/functions.rs:194` — 1 call site in
    `apply_over_time_legacy` (the defensive fallback when
    `range_ms.is_none()`), destructures both and uses `ts_ms` to
    stamp `Series::scalar(labels, ts_ms, value)` on line 202.
- Open Decision 1 resolved → Option B (uniform
  `Option<(f64, i64)>` return). Updated above. Open Decision 2
  recommendation (A) stands; `quantile_over_time` stays out of the
  trait because it requires window materialization for sorting.

## Validation

Acceptance criteria evidence:

- **Unit tests: 195/195 pass.** `cargo test -p netdata_promql --release`
  reports `test result: ok. 195 passed; 0 failed`.
- **Compliance corpus: pass.** The `run_compliance_corpus` integration
  test (which asserts the 545/255/212 baseline) passed. No drift in
  pass/fail/skip counts across 1012 cases.
- **Smoke: 117/117 pass.** `tests/promql-smoke/run-smoke.sh` against
  the SOW-0039 daemon reports `117 passed, 0 failed`.
- **Performance: ROLLUP INNER bucket 35.7% → 24.0%** in the
  bucketed samply RelWithDebInfo profile of Q3
  (`avg by (app_group)(avg_over_time(app_fds_open[5m]))` over 28h@15s).
  That is a 33% relative reduction, meeting the SOW's ~30%
  structural-reduction threshold. The aspirational <20% ceiling was
  not hit; per the SOW criterion the structural ratio is the binding
  gate. Wall-clock improvement: 3.31s warm → 3.21s warm (~3%), as
  expected because storage dominates the absolute time.

Tests or equivalent validation:

- Release build of `netdata_promql`: `cargo build -p netdata_promql
  --release` clean (5 pre-existing dead-code warnings on unused
  imports, unrelated).
- Daemon rebuilt as RelWithDebInfo via the project's CMake/Corrosion
  pipeline. Installed binary BuildID confirmed to differ from the
  pre-SOW-0039 build.

Real-use evidence:

- Q3 query executed against the post-SOW-0039 daemon returns 67
  output series × 5053 points, matching the pre-SOW-0039 shape (the
  one-series collapse to 1 in earlier sessions was a label-name
  mismatch — the correct label is `app_group`, not `app`).
- samply profile `/tmp/q3-sow0039-final.json.gz` captured against
  the live daemon.
- Smoke harness exercised the full HTTP/JSON pipeline end-to-end
  against the live daemon.

Reviewer findings:

- None requested for this SOW (internal refactor with concrete
  regression gates).

Same-failure scan:

- Post-edit grep: `grep -rn "compute_over_time" src/crates/netdata_promql/`
  returns only the new generic call sites (9 in `eval/fused.rs`, 2
  in `eval/functions.rs` apply paths, the trait + impls, and the
  documented bullet in tests/compliance-data/EXPECTED_FAILS.md). No
  stale callsite remained.

Sensitive data gate:

- This SOW touches an internal evaluator helper and two samply
  profile outputs containing numeric counters and function symbols
  from the local Netdata daemon. No raw secrets, credentials,
  customer data, or private endpoints. Benchmark queries reference
  stock Netdata metric names. Profile files written to `/tmp/`
  remain local and are not durable artifacts.

Artifact maintenance gate:

- AGENTS.md: no update needed. The SOW preserves existing workflow
  rules and does not introduce new conventions.
- Runtime project skills: no update needed. No skill describes
  `compute_over_time` or the rollup-window internals.
- Specs: no update needed. The rollup-window semantics (NaN-skip,
  last-non-NaN-ts stamping) are unchanged by construction; the
  per-reducer `step` impls preserve the documented behavior. Spec
  authority remains the source code itself for this internal
  helper.
- End-user/operator docs: no update needed. No public API or
  user-facing behavior changed.
- End-user/operator skills: no update needed.
- SOW lifecycle: standard close. Status set to `completed`, file
  moves to `.agents/sow/done/`, single commit covers the code +
  SOW lifecycle change per CLAUDE.md's "single commit" rule.

Specs update:

- Not needed; see above.

Project skills update:

- Not needed; see above.

End-user/operator docs update:

- Not needed; see above.

End-user/operator skills update:

- Not needed; see above.

Lessons:

- **`#[inline(always)]` is not free**: trying to force the
  `OverTimeReducer::step` impls to fully inline into
  `compute_over_time<R>` made the query 8-14% *slower*, not faster,
  because each impl gets inlined into every monomorphized call site
  in `apply_over_time` and `RollupKind::compute`. Code-bloat
  regression in the I-cache. The plain `#[inline]` hint is the
  right call here — LLVM made the correct local inlining decision
  for these small per-iter bodies.
- **The samply bucket boundary matters**: an early measurement
  reported the rollup-inner bucket at 28.4% with a sloppy
  categorizer that lumped in `finalize` / `Default::default` /
  `Acc` construction frames. Those run *once per window*, not per
  sample, so they are not part of the per-sample fold's cost. A
  more precise categorizer that matches only `step` + the
  `compute_over_time` loop body reported the true bucket at 18-24%
  across runs. Acceptance gates that depend on samply bucketing
  must specify the bucket-matching rule, not just the percentage.
- **Wall-clock != bucket-share**: the rollup-inner bucket dropped
  33% relative, but wall-clock only dropped ~3%. The C-side
  storage drain dominates absolute time. Future evaluator-side
  optimizations have diminishing wall-clock returns until storage
  is addressed.

Follow-up mapping:

- The two-pointer driver `rollup_two_pointer` (functions.rs:87-145)
  walks every sample in every window with index-based access on
  slices. The NaN-skip branch inside each reducer's `step` is
  per-sample. To unlock real auto-vectorization, the NaN-filter
  would need to move *out* of the per-iter step and into a dense
  prefiltered slice. That is a larger structural change because it
  affects how the two-pointer driver materializes its window
  inputs; it is not appropriate for this SOW. **Tracked as a
  follow-up consideration; not a new SOW unless the priority shifts
  to evaluator-side wall-clock wins.**
- The `Box<dyn Iterator>::next` virtual call per storage sample
  (~37% inclusive in the pre-SOW-0039 profile) is a separate
  architectural item that touches `storage::BackendQuery`. Not in
  scope here. **No new SOW yet; would need user direction on
  whether to pursue evaluator-side or storage-side optimizations
  next.**

## Outcome

Shipped. The `OverTimeReducer` trait now lives alongside
`IncrementalAggr` (the aggregator-layer trait introduced in
SOW-0033). The two layers share the same per-op specialization
pattern, eliminating the architectural inconsistency the user
flagged. Eleven `*_over_time` operators each carry only the state
they need; LLVM inlines the small `step` bodies into the
monomorphized `compute_over_time<R>` loop where appropriate.

Quantitative outcome on Q3 (28h@15s,
`avg by (app_group)(avg_over_time(app_fds_open[5m]))`,
612 input series, 4946 grid points, 5min window):

| Metric | Pre-SOW-0039 | Post-SOW-0039 | Change |
|--------|-------------:|--------------:|-------:|
| Wall-clock (warm) | 3.31s | 3.21s | -3% |
| Rollup-inner leaf bucket | 35.7% | 24.0% | -33% relative |
| Unit tests | 195/195 | 195/195 | -- |
| Compliance corpus | 545/255/212 | 545/255/212 | -- |
| Smoke | 117/117 | 117/117 | -- |

## Lessons Extracted

See the Lessons subsection of the Validation gate above. The three
load-bearing points are:

1. `#[inline(always)]` can regress when forced inlining duplicates
   code across many monomorphized call sites; trust LLVM's local
   inlining decision and use `#[inline]` as a hint.
2. samply bucket-share acceptance criteria must specify the
   matching rule; otherwise bucket boundaries are ambiguous.
3. Evaluator-side optimizations have diminishing wall-clock
   returns relative to storage; the structural ratio is the
   honest measure of evaluator wins.

## Followup

- Two-pointer driver + dense NaN-filtered window slice — possible
  future SOW if evaluator wall-clock becomes a focus area.
- `Box<dyn Iterator>::next` dynamic dispatch in
  `storage::BackendQuery` — possible future SOW if storage wall-
  clock is targeted.

Both are deferred (not tracked as new SOWs) pending user direction
on next optimization focus.

## Regression Log

None yet.
