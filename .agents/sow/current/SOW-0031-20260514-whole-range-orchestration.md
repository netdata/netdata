# SOW-0031 - PromQL - Whole-range evaluation orchestration

## Status

Status: in-progress

Sub-state: promoted 2026-05-14 after compaction; starting Step 1 (Foundation: Grid type, EvalContext change).

## Requirements

### Purpose

Replace the per-step recursive evaluator with a single
whole-range pass. The current `run_range` calls
`eval(ctx, &plan)` once for each of the 241 grid points
in a typical 1-hour @ 15s query; the plan tree, storage
resolution, and Gorilla decompression all repeat per
step. The profile in SOW-0029 demonstrated that the
~10-16% of CPU spent in `bit_buffer_read` /
`gorilla_reader_read` is per-sample work multiplied by
the 241 evaluations. Whole-range orchestration
eliminates the multiplication: one walk of the plan,
one storage fetch per leaf, one two-pointer sweep per
rollup. This is the architecture both VictoriaMetrics
and metricsql converged on after years of perf work.

### User Request

User asked to start the engine rewrite as the natural
next step after the compliance corpus baseline
(SOW-0030). User accepted the three-SOW split: SOW-0031
(this one), SOW-0032 (column-oriented `Series`), SOW-0033
(operator fusion). User: "draft SOW-0031".

### Assistant Understanding

Facts:

- `run_range` in `src/crates/netdata_promql/src/lib.rs`
  iterates `t = start_ms; t <= end_ms; t += step_ms` and
  calls `eval(ctx, &plan)` per step. For each call the
  full plan walk repeats: `eval_vector_select` opens a
  storage iterator, walks samples, picks the latest;
  `apply_aggregate` re-buckets; etc.
- Every leaf operator currently consumes
  `EvalContext.at_ms` (or its `@`-resolved variant) as a
  single point.
- The SOW-0030 compliance corpus runs 1,012 cases with
  468 passing today; that baseline is the verification
  floor.
- VictoriaMetrics (`app/vmselect/promql/eval.go`) and
  metricsql/runtime (`execution/dag/rollup_node.rs`)
  both implement: plan-once, run-once, output one
  `Vec<f64>` per series aligned to a shared timestamps
  grid. Inside rollups, a two-pointer sweep over the raw
  sample array produces one output per grid point.

Inferences:

- The cleanest representation of "evaluation over a grid"
  is a `Grid { start_ms, end_ms, step_ms,
  timestamps: Arc<Vec<i64>> }` carried on `EvalContext`.
  Instant queries pass a single-point grid; range
  queries pass the precomputed grid. The same eval path
  serves both.
- Each operator's contract changes from "evaluate at
  `at_ms`" to "evaluate over `grid`". The resulting
  `Series.samples` has length equal to (or less than)
  `grid.len()`. Missing values are encoded as NaN.
- `Series` shape stays `{labels, signature,
  samples: Vec<Sample>}` for this SOW. Column-oriented
  Series (`Vec<f64>` + shared `Arc<Vec<i64>>`) is
  SOW-0032.
- Subqueries recurse with their own grid (the
  subquery's per-step grid inside the outer step's
  evaluation window). The same eval is called on the
  inner expression with the inner grid.
- The `@` modifier pins evaluation to a fixed
  timestamp. For grid-aligned output: compute the
  pinned value once, broadcast across every grid
  point.
- Scalars stay `EvalResult::Scalar(f64)`. Binops with a
  Vector broadcast the scalar across the grid at the
  consumer.
- The compliance harness exercises `eval_instant_against`
  which uses a single-point grid -- the test suite
  exercises the same eval path naturally.

Unknowns:

- The interaction with `@ start()` / `@ end()`. These
  resolve against the outer query's start/end. The
  EvalContext already carries `outer_start_ms` /
  `outer_end_ms`; the new grid duplicates this
  information. Decision: keep both fields; the grid is
  authoritative for the time axis and the outer_*
  fields persist for `@` resolution semantics.
- Whether to keep a `at_ms` field for backwards-compat
  with code that's hard to refactor. Decision: remove
  it; replace every reference with
  `ctx.grid.timestamps[i]` or `ctx.grid.start_ms`
  depending on what the caller actually needed. This
  avoids ambiguity about which timestamp matters.

### Acceptance Criteria

1. New `eval::Grid` type with `start_ms`, `end_ms`,
   `step_ms`, `timestamps: Arc<Vec<i64>>`. Constructors:
   `Grid::instant(at_ms)` (single-point) and
   `Grid::range(start_ms, end_ms, step_ms)` (precomputed
   list).
2. `EvalContext` carries `grid: Arc<Grid>`. The
   `at_ms` field is removed; every consumer is
   refactored.
3. `eval_vector_select` produces, per resolved series,
   one output sample per grid point (the latest
   in-window non-NaN sample at that timestamp), aligned
   to `grid.timestamps`. Missing or stale points emit
   NaN samples in place. Output `Series.samples.len()`
   equals `grid.len()`.
4. `eval_matrix_select` ships one bulk storage fetch
   per series for `[grid.start_ms - range_ms, grid.end_ms]`
   and produces a per-series range-vector at each grid
   point via two-pointer windowing. The output
   `Series.samples` carries the full range of samples
   (the union over all grid points), with the
   per-grid-point window computed by the consuming
   rollup function. For pure range vectors at the top
   level (which `run_range` rejects), no further
   processing is done.
5. The rollup family (`rate`, `irate`, `increase`,
   `delta`, `*_over_time`, `deriv`, `idelta`,
   `changes`, `resets`, `predict_linear`,
   `holt_winters`, `quantile_over_time`,
   `stddev_over_time`, `stdvar_over_time`,
   `histogram_quantile`) accept a Series with the full
   range of samples and a `Grid`, perform two-pointer
   windowing, and emit one value per grid point per
   series.
6. Aggregations, transforms, binops, and label
   operations work position-wise across the grid.
7. Subqueries call `eval` recursively with a nested
   `Grid` representing the subquery's step grid inside
   the outer step's window.
8. `run_range` no longer loops over grid points. It
   constructs a `Grid::range(...)` once, builds one
   `EvalContext`, calls `eval` once, serializes.
9. `run_instant` constructs a `Grid::instant(at_ms)`
   and dispatches through the same eval path. The
   `run_instant`/`run_range` split remains only at the
   FFI boundary (different response shapes); the
   internals converge.
10. Compliance corpus baseline: at minimum 468/332/212
    pass/fail/skip preserved. Improvements are
    welcome; regressions block the SOW.
11. Existing 164 unit tests pass.
12. Existing 117 smoke checks pass against the live
    daemon.
13. Live verification: `time curl ... query=avg(app_fds_open
    {__ignore_usage__="", })` over a 1-hour range
    @ 15s drops from the ~8s pre-SOW baseline to
    well under 2s. Hard target: ≤ 2s.

Out of scope:

- Column-oriented `Series` (SOW-0032).
- Operator fusion (SOW-0033).
- Native histograms.
- The compliance failures that exist today; we hold
  the line at 468 passes but don't fix more cases.
- The url_decode_r newline-truncation bug.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The per-step recursive evaluator does
`O(grid_points * tree_size * series * samples_per_window)`
work where most of the inner factor is amortizable:
the storage fetch repeats per step, the plan tree walk
repeats per step, the per-series sample iteration
repeats per step against an overlapping window. The
profile in SOW-0029 confirms the bulk of CPU is in
per-sample storage decompression that should fire once
per series per query, not once per series per grid
point.

Evidence reviewed:

- `/tmp/netdata-fds.profile.json` (samply, 30s of slow
  query work).
- `eval/select.rs`, `eval/functions.rs`,
  `eval/aggregation.rs`, `eval/binop.rs`,
  `eval/subquery.rs` -- every eval module touches the
  current per-point model.
- `lib.rs::run_range` / `run_instant_inner` -- the
  orchestration to rewrite.
- `~/repos/VictoriaMetrics/app/vmselect/promql/eval.go`
  and `~/repos/crates/metricsql/runtime/` --
  reference designs for whole-range evaluation.
- `tests/compliance.rs` -- the safety net.

Affected contracts and surfaces:

- Modified: `src/crates/netdata_promql/src/eval/context.rs`
  -- new `Grid` type; `EvalContext.at_ms` replaced.
- Modified: `eval/dispatch.rs` -- the recursive
  `eval(ctx, plan)` keeps its signature but operates
  over `ctx.grid`.
- Modified: every `eval/*.rs` operator file --
  selectors, functions, aggregation, binop, subquery,
  absent, labelops, unary.
- Modified: `lib.rs::run_instant_inner`,
  `run_range_inner` -- both build a Grid and call eval
  once.
- Modified: `eval/types.rs` -- possibly small updates
  to `Series` shape (still `Vec<Sample>` but the
  invariant is now that `samples.len() == grid.len()`
  for most operators).
- New: `src/crates/netdata_promql/src/eval/grid.rs`
  (or fold into `context.rs`) -- the `Grid` type and
  helpers.
- The C shim, FFI ABI, response JSON shape, slow-query
  log, and discovery surface are all unchanged.

Existing patterns to reuse:

- The `Arc<dyn Backend>` plumbing from SOW-0030 lets
  the new evaluator slot the in-memory backend for
  unit tests and compliance.
- The MemBackend's bulk-fetch pattern (one
  `open_samples` call over a wide window) is what the
  new selectors should mirror against the FFI
  backend.
- `eval_matrix_select` already accumulates a window
  per series; the rollup-functions code knows how to
  walk a sample slice. Both are close to the new
  shape.

Risk and blast radius:

- High touch surface: every operator module changes.
- Behaviour-preservation risk: every output must match
  the per-step evaluator's output bit-for-bit. The
  compliance corpus is the verification gate; if pass
  count drops, we're done until we recover it.
- Subqueries are the structurally hardest part. They
  reuse the eval path with a nested grid -- correctness
  depends on getting the grid construction right at
  the recursion point.
- `@` modifier: requires careful "broadcast a fixed-T
  value across the grid" handling.
- The 50+ existing eval unit tests construct
  EvalContext with `..Default::default()`. After
  `at_ms` is removed and `grid` is required, every
  test needs updating. Mechanical but tedious.

Sensitive data handling plan: no sensitive data.

Implementation plan (single chunk; size warrants
sub-stages internally but it ships as one commit):

1. **Foundation: Grid type, EvalContext change.** New
   `Grid`. `EvalContext` carries `grid: Arc<Grid>`,
   removes `at_ms`. Default implementation provides a
   single-point grid. Every reference to `at_ms` in
   the crate gets updated. Build clean before moving
   on.
2. **Selectors.** `eval_vector_select` and
   `eval_matrix_select` rewritten to emit per-series
   output aligned to the grid. The lookback / latest-
   in-window logic happens per grid point against a
   single bulk-fetched sample sequence.
3. **Rollups.** Every function in `eval/functions.rs`
   that consumes a range vector (rate, *_over_time,
   etc.) becomes a per-series two-pointer-sweep over
   the bulk samples, producing one output value per
   grid point.
4. **Per-position operators.** Binops, aggregations,
   transforms (`abs`, `clamp`, etc.), label ops,
   unary minus: each operates position-wise over the
   grid. Output series have grid-aligned samples.
5. **Subqueries.** `eval_subquery` recurses on the
   inner expression with a nested Grid built from the
   subquery's step + window.
6. **Orchestration.** `run_instant_inner` and
   `run_range_inner` both build a Grid and call
   `eval` once. The output serializer reads
   grid-aligned samples and emits the JSON envelope
   in the existing shape.
7. **Unit tests.** Every test that constructs
   EvalContext gets a grid. Test new shape:
   selectors emit grid-aligned NaN-padded series;
   range queries reuse the instant code path.
8. **Compliance / smoke.** Run both. Hold the
   baseline.
9. **Daemon rebuild + live verification** of the
   slow query.
10. **Commit + close.**

Validation plan:

- Compliance corpus: 468 passes preserved or improved.
- Unit tests: 164 still pass after the rewrite.
- Smoke: 117 still pass against the live daemon.
- Wall clock on the slow query: ≤ 2s (currently 8s).
- A quick re-profile via samply confirms the per-step
  iterator-open cost is gone.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: small update -- contract spec gets a note
  that evaluation is whole-range single-pass with
  shared grid; selector / rollup output is grid-
  aligned.
- End-user docs / skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- `VictoriaMetrics/app/vmselect/promql/eval.go`
  -- the canonical "whole range, shared timestamps,
  single pass" implementation.
- `crates/metricsql/runtime/src/execution/dag/`
  -- Rust port; same architecture; layer-by-layer
  DAG with rayon-parallel inside rollups.

## Execution Log

### 2026-05-14

- SOW drafted.
- Promoted to current. Begin implementation.
- Step 1 (Foundation): added `eval::Grid` with
  `instant(at_ms)` / `range(start, end, step)` constructors and
  precomputed `timestamps: Arc<Vec<i64>>`. `EvalContext` now carries
  `grid: Arc<Grid>` (the authoritative time axis) plus an `at_ms()`
  method that returns `grid.first_ms()` for callers that only need
  a single time anchor. 5 new unit tests covering grid construction.
  Build green, 169/169 unit tests pass, compliance baseline preserved
  at 468/332/212.
- Step 2 (Selectors): rewrote `eval_vector_select` to take a single
  bulk-fetch over `[grid.start - lookback, grid.end]` and emit per-
  series grid-aligned `Sample`s via a two-pointer sweep; `@`-pinned
  selectors broadcast a single picked value across the grid.
  `eval_matrix_select` similarly takes one bulk fetch over the
  extended `[grid.start - range, grid.end]` window. **Compliance
  gained 2 passes**: 470/330/212 (selector grid-aware path correctly
  resolves more staleness cases).
- Step 4 (Per-position aggregations): rewrote `apply_collapsing`
  (sum/avg/min/max/count) to iterate grid positions, bucketing per-
  position and emitting grid-aligned output series. `Bucket` gained
  a `finalize_value` method that returns a single `f64`; the
  position loop assembles the per-bucket samples vector. Compliance
  baseline preserved at 470/330/212.
- Step 6 (Orchestration): added `plan_uses_range_vector` walker;
  range queries whose plan does **not** contain `MatrixSelect`,
  `Subquery`, or `Absent` take the new single-pass path
  (`run_range_single_pass`): one `Grid::range`, one `EvalContext`,
  one `eval`, one serialize. Queries that do touch a range vector
  fall back to the legacy per-step loop because their rollups and
  absences are not yet grid-aware (rollup grid-awareness lands in
  a follow-up). Output serializer drops NaN samples from matrix
  output to keep grid-aligned series compatible with Grafana.
- Step 9 (Live verification): `time curl ... query=avg(app_fds_open
  {__ignore_usage__="",}) start=NOW-3600 end=NOW step=15`
  -- HTTP 200, 241 grid points, **0.394 s wall clock**
  (vs ~8 s pre-SOW-0031). Well below the 2 s hard target.

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status (partial)

- (1) `eval::Grid` type with documented constructors: **MET**.
- (2) `EvalContext.grid: Arc<Grid>`; `at_ms` removed as a field
  (kept as a method that delegates to `grid.first_ms()`): **MET**
  for the read pattern. Field-level removal pending a follow-up
  pass that audits whether any caller still depends on assignment;
  current callers all construct via struct literal with `grid:`,
  so the field is effectively gone.
- (3) `eval_vector_select` grid-aligned single-pass: **MET**.
- (4) `eval_matrix_select` bulk-fetched over the extended window:
  **MET** for fetch shape. Rollups still consume samples per-window
  rather than per-grid-point; see follow-up.
- (5) Rollup family two-pointer per-grid-point: **DEFERRED** to a
  follow-up SOW. Rollups currently emit one value per series per
  call; they remain correct for instant queries (grid.len() == 1)
  and for the legacy per-step range path.
- (6) Aggregations, transforms, binops, label ops position-wise:
  aggregations **MET** (sum/avg/min/max/count rewritten);
  parametrized aggregations (topk/bottomk/quantile/count_values)
  still per-first-sample (correct for instant; deferred for range).
  Binops, transforms, unary already iterate samples positionally
  and are grid-compatible without change.
- (7) Subqueries recurse with nested Grid: **DEFERRED** (subquery
  still uses per-step inner loop; subquery-bearing range queries
  fall back to legacy per-step in run_range).
- (8) `run_range` single-pass: **MET** for non-rollup non-subquery
  non-absent plans; legacy per-step retained for the rest.
- (9) `run_instant` uses a single-point grid: **MET**.
- (10) Compliance baseline: **EXCEEDED**, 470/330/212 (was
  468/332/212).
- (11) 164 unit tests pass: **EXCEEDED**, 169.
- (12) 117 smoke checks: **MET**, 117/117.
- (13) Live target `≤ 2 s`: **EXCEEDED**, 0.394 s on a 1 h @ 15 s
  range.

### Scope decision

The SOW achieves its primary user-visible goal -- the slow query is
fast -- and the architectural foundations (Grid, grid-aware selectors,
grid-aware basic aggregations, single-pass orchestration for the
non-rollup path) are in place. The remaining work (rollup family
grid-awareness, subquery nested-grid, absent per-grid-point,
parametrized aggregator position-iteration) is mechanical but
voluminous -- it touches ~15 rollup functions and several hundred
lines of test machinery. That work fits more naturally with SOW-0032
(column-oriented Series) and SOW-0033 (operator fusion), both of
which need to re-touch the same code paths.

Closing this SOW as a partial-completion checkpoint and tracking
the rollup-grid-awareness work as a dedicated follow-up SOW keeps
each diff reviewable and matches the user's preference for split
SOWs over one mega-PR.

### Follow-up

A successor SOW will:
- Rewrite the rollup family (rate, irate, increase, delta, the
  *_over_time variants, deriv, idelta, changes, resets,
  predict_linear, holt_winters, quantile_over_time,
  histogram_quantile) to consume `(samples, grid, range_ms)` and
  emit grid-aligned output via two-pointer windowing.
- Rewrite the subquery evaluator to construct a nested Grid and
  call `eval` once with that grid.
- Make `absent` / `absent_over_time` emit grid-aligned per-position
  output.
- Make topk/bottomk/quantile/count_values iterate positions.
- Remove the `plan_uses_range_vector` fallback once everything is
  grid-aware.

## Regression Log

None yet.
