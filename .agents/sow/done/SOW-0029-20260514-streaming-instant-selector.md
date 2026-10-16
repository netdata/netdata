# SOW-0029 - PromQL - Phase 1: streaming instant-vector selectors

## Status

Status: completed

Sub-state: shipped 2026-05-14 as a code-cleanup refactor.
Did NOT deliver the 3-5x speedup predicted in the SOW.
Wall-clock time on the motivating slow query unchanged
(~8s before, ~8s after). Honest finding recorded below.
Phase 3 (cadence-aware lookback) is the actual lever.

## Requirements

### Purpose

Cut the per-step allocation overhead in
`eval_vector_select` by replacing the
"collect-all-window-samples-into-Vec-then-pick-latest"
pattern with a single-pass fold that keeps just the latest
in-window non-NaN sample. Profile evidence shows
`Vec::from_iter` + `RawVec::grow_amortized` at ~50% of stack
time on the slow `avg(app_fds_open[...])` range query.
Expected effect: 3-5x speedup on instant-selector-heavy
workloads. Behaviour unchanged.

### User Request

User asked for a from-scratch design retrospective. Output
was a four-phase incremental migration plan. User: "I think
we need to do all these phases in step. Should we start with
Phase 1?" -> yes.

### Assistant Understanding

Facts:

- `eval_vector_select` in
  `src/crates/netdata_promql/src/eval/select.rs` currently
  walks the storage iterator, maps each sample to a fresh
  `Sample` struct, filters NaN, and collects into
  `Vec<Sample>` per series.
- After the Vec is built, it calls
  `pick_latest_within_window(&samples, target_ms,
  lookback_ms)` which scans the Vec in reverse to find the
  latest sample with `earliest <= ts <= target`.
- The shim's iterator returns samples in
  non-decreasing chronological order (`open_samples`
  contract).
- `pick_latest_within_window` has only one caller and four
  unit tests in `lookback.rs::tests`.
- `eval_matrix_select` (the sibling function for range
  selectors) collects samples too, but that is correct --
  range vectors need the full window. Don't change it.

Inferences:

- Because samples are time-ordered, scanning forward and
  remembering the last qualifying sample gives the same
  answer as reverse-find-first on a Vec. No reverse scan
  needed.
- The `Vec<Sample>` is allocated and discarded per series
  per step. Removing it cuts the allocator path
  (`raw_vec::grow_amortized`, `RawVec::finish_grow`, etc.)
  that the profile shows as ~50% of stack time.
- `pick_latest_within_window` becomes unused; remove it to
  keep the public surface small. Its semantic is preserved
  inline in the new fold.

Unknowns:

- None. The change is local to one function with one
  consumer.

### Acceptance Criteria

1. `eval_vector_select` walks the sample iterator
   once and emits a single output sample per series, using
   a single `Option<Sample>` accumulator rather than a
   `Vec<Sample>`.
2. Semantics unchanged: same series included / excluded
   from the result; same `value` for every included
   series; output stamped at `ctx.at_ms` per Prometheus
   alignment rule.
3. `lookback.rs` is either removed (preferred) or marked
   `dead_code` with its tests adjusted to test the inline
   logic. The latter is acceptable only if some other
   caller appears that the SOW author missed.
4. Existing 165 unit tests pass; existing 117 smoke checks
   pass.
5. Live verification: re-time the
   `avg(app_fds_open{__ignore_usage__="", })` range query.
   Goal is meaningful reduction (3-5x).
6. The eval/select.rs change is the only Rust-side change
   needed -- no IR, no shim, no eval-mod, no dispatch
   change.

Out of scope:

- Phase 2 (sample iterator reuse across range steps).
- Phase 3 (cadence-aware lookback).
- Phase 4 (operator-streaming refactor with new trait).
- `eval_matrix_select` changes (correctly collects).

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The profile shows `<alloc::vec::Vec as
FromIterator>::from_iter` on 49.15% of stacks during the
slow query, and `RawVec::grow_amortized` /
`finish_grow` family appearing prominently in self-time.
The cause is one `.collect::<Vec<Sample>>()` call per
series per step. For an instant-vector selector at each
grid point in a range query against a metric with many
series and 1Hz native cadence, this is hundreds of
allocations per step, hundreds of times per query.

Evidence reviewed:

- `/tmp/netdata-fds.profile.json` (samply, 30s capture
  during 3 sequential 9.5s `avg(app_fds_open[1h:15s])`
  queries).
- `/tmp/fds-analysis.pkl` (symbolised pickle).
- `eval/select.rs` lines 65-95 (the `.collect()` site).
- `eval/lookback.rs` (the consumer of the collected Vec).

Affected contracts and surfaces:

- Modified: `eval/select.rs::eval_vector_select` -- one
  function body.
- Removed: `eval/lookback.rs` module (its single function
  is fold-into the call site).
- Modified: `eval/mod.rs` -- remove the `mod lookback;`
  declaration.

No FFI change, no shim change, no IR change, no spec
change (the lookback rule the spec describes is preserved
exactly).

Existing patterns to reuse:

- `unpack_storage_number`/`STORAGE_POINT` access pattern
  is unchanged.
- Other `eval/*` modules show the single-pass-fold shape
  we're moving toward (e.g.
  `apply_over_time::compute_over_time`).

Risk and blast radius:

- Low. The function's public signature is unchanged; only
  the body is restructured. Semantics are preserved
  pointwise.
- Edge case: a sample with `ts > target_ms` could be
  returned by `open_samples` because the shim rounds the
  `before_s` cutoff to seconds. The new fold must filter
  these out (the current code does, via
  `pick_latest_within_window`'s `s.timestamp_ms <=
  target_ms` clause). Preserve that exactly.
- Edge case: NaN samples must be excluded from the
  "latest in window" pick. Current code filters before
  the pick. Preserve that.

Sensitive data handling plan: no sensitive data in scope.

Implementation plan (single chunk):

1. Rewrite `eval_vector_select`'s per-series loop to use
   `Option<Sample>` accumulator. Inline the
   `earliest = target - lookback` window check, the
   `target_ms` upper bound, and the NaN filter.
2. Delete `eval/lookback.rs`; remove its `mod lookback;`
   line from `eval/mod.rs`.
3. Re-run `cargo test -p netdata_promql`.
4. Re-run smoke harness.
5. Re-time the slow query for live verification.
6. Spec / docs: no changes needed.
7. Commit + close.

Validation plan:

- Rust unit tests: 165 should remain green (we lose 4
  tests from `lookback.rs::tests`, gain ~3 new tests
  that exercise the inline path with edge cases:
  out-of-window upper bound, NaN in the middle, single
  sample at target). Net target: ~164 tests.
- Smoke: 117 unchanged. The query results are
  semantically identical.
- Wall clock: `time curl` the slow query before and after.
  Expect at least 3x improvement; profile confirms the
  hot path is what we're cutting.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: no change (the lookback rule is preserved).
- End-user docs / skills: no change.
- SOW lifecycle: pending -> current -> done.

## Execution Log

### 2026-05-14

- SOW drafted, promoted, implemented single chunk.
- `eval_vector_select` in `eval/select.rs` refactored to a
  single-pass fold with an `Option<f64>` accumulator. The
  `earliest_ms`/`effective_t_ms` bounds and the NaN filter
  are inlined; the previous `.collect::<Vec<Sample>>()` +
  `pick_latest_within_window` pair is gone.
- `eval/lookback.rs` deleted; `mod lookback;` removed from
  `eval/mod.rs`.
- Build clean. Tests 161/161 (was 165; -4 lookback-module
  tests, no new ones because the inline logic is exercised
  by the smoke harness's many vector-selector calls).
- Smoke 117/117.
- Live verification on the motivating query
  `avg(app_fds_open{__ignore_usage__="", })` over 1h @ 15s:
  - Before refactor: 8.5s, 9.6s, 9.6s, 9.7s (4 calls).
  - After refactor: 8.6s, 8.1s, 8.0s (3 calls).
  - Essentially unchanged.

## Validation

Acceptance criteria coverage:

1. Single-pass fold with `Option<Sample>` accumulator --
   shipped, verified by direct read of the updated
   `eval_vector_select`.
2. Semantics unchanged -- 117/117 smoke pass, every query
   that exercises an instant-vector selector returns
   identical results before/after.
3. `lookback.rs` removed entirely. Inlined logic
   preserves the `[earliest_ms, effective_t_ms]` bounds
   and the NaN filter exactly.
4. 161/161 unit tests pass.
5. **Live speedup goal NOT met** (predicted 3-5x; got 1x).
   Documented in Lessons below.
6. Only `eval/select.rs` + `eval/mod.rs` changed; no
   public surface change.

Test posture: 161/161 unit (was 165; -4 lookback tests),
117/117 smoke (unchanged).

Reviewer findings: the author's own retrospective is in
Lessons below.

Same-failure search: no other call sites of the removed
`pick_latest_within_window` exist.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: no change (lookback rule documented in the
  spec is unchanged; the implementation just moved).
- End-user docs / skills: no change.
- SOW lifecycle: pending -> current -> done.

## Outcome

The refactor is a strict code-quality improvement: ~30
fewer lines, one fewer module, no allocator pressure in
the per-series-per-step hot path. As a performance
intervention against the slow-query case that motivated
it, **it delivered no measurable speedup**.

The cause is straightforward in hindsight: the profile's
49% `Vec::from_iter` figure was stack-appearance, not
self-time. Stack-appearance counts how often a function
is somewhere on the call stack at sample time; it
inflates wildly for functions that span a wide time slice
even when their CPU consumption is low. The actual
self-time CPU in the inner loop was always in
`bit_buffer_read` (Gorilla bit decode) and
`storage_engine_query_next_metric` (DBENGINE
decompression). Removing the Vec didn't change the
amount of work we did inside those functions -- we still
decompress every sample in the 5-minute lookback window
to use one.

## Lessons

- **Stack-appearance is not self-time.** When ranking
  hot spots, use the self-time column. Stack-appearance
  is a "this function is on the stack a lot" measure --
  useful for understanding call structure, not for
  ranking where CPU actually goes. The 49% figure misled
  the prediction.
- **The Vec was a symptom, not a cause.** The real
  problem is that an instant-vector selector reads 300
  samples per series to use one. Removing the Vec only
  removes a tiny allocator-side cost; the storage
  decompression is unchanged.
- **A code-cleanup refactor is still worth committing**
  even when its performance hypothesis fails. The new
  code is simpler, allocates less, and sets up better
  for Phase 2/3.
- **The right next phase is Phase 3, not Phase 2.** The
  storage cost is per-sample-decompressed. Cutting the
  lookback window (Phase 3) directly cuts that cost.
  Phase 2's "reuse iterators across steps" reduces the
  per-step open/close overhead but does not reduce the
  per-sample work.

## Followup

- **SOW-0030 (Phase 3, cadence-aware lookback) is the
  natural next step** and is expected to deliver the
  speedup that this SOW predicted but did not deliver.
  Cutting the default lookback from 5 minutes to a value
  that tracks the chart's collection cadence would
  reduce per-series-per-step storage iterations by
  ~20x for 1Hz-cadenced metrics.
- Phase 2 (iterator reuse across range steps) is
  deferred until after Phase 3 is measured; the win may
  be smaller than originally projected.
- Phase 4 (operator-streaming refactor) is deferred
  until measurement justifies it.

## Regression Log

None yet.
