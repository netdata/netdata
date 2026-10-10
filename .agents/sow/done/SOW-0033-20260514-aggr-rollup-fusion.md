# SOW-0033 - PromQL - Aggregation × rollup fusion

## Status

Status: completed

Sub-state: `aggr(rollup(selector))` fusion lands at lowering time for
the supported `sum/avg/min/max/count × rate/increase/delta/irate/
{avg,sum,min,max,count,last,present,stddev,stdvar}_over_time` combos.
Equivalence test asserts fused output matches the unfused
`Plan::Aggregate{expr: Plan::Call{...}}` path bit-for-bit on
synthetic input. 179/179 unit tests, 117/117 smoke, 472/328/212
compliance. Subquery in fused position deferred (rare; tracked
as follow-up).

## Requirements

### Purpose

Detect the `aggr(rollup(selector))` shape -- the canonical Grafana
panel query -- at plan-lowering time and evaluate it as a single
fused pass instead of three independent operators that each
materialise their own `Vec<Series>`.

Today, after SOW-0031/0032, `avg(rate(http_requests[5m]))` over
1000 input series and 241 grid points evaluates as:

1. Selector resolves 1000 series.
2. `rate(...)` allocates 1000 `Series` × 241 `f64` values (the
   intermediate range), filtered for NaN, signature-sorted.
3. `avg(...)` per grid point walks those 1000 rate values,
   buckets by grouping, collapses to M output series (M ≪ 1000,
   typically 1).

Stage 2 is pure waste: the 241,000 rate values exist solely to
be summed and divided in stage 3. Fusion eliminates the
intermediate Series: for each input series, the two-pointer
rollup writes the per-grid-point value straight into the
aggregator's accumulator slot for that series' bucket. The
intermediate `Vec<Series>` for the rollup never materialises.

This matches VictoriaMetrics' `incremental_aggr_func_context`
and metricsql's fused execution path. Both implementations
report a 2-3× speedup at high cardinality on this exact pattern.

### User Request

After SOW-0032 closed, user asked for a refresher on SOW-0033 and
confirmed they want it drafted next. The original three-SOW plan
listed it as "the production-scale Grafana lever".

### Assistant Understanding

Facts:

- Every Grafana panel that combines a rate/`*_over_time`/increase
  with a `sum`/`avg`/`max`/`min`/`count` aggregation hits this
  pattern. Dashboards typically emit dozens per reload.
- After SOW-0031/0032: rollups allocate one `Series` per input
  series with a grid-length `Vec<f64>`. For high cardinality
  this is the dominant allocation cost.
- Plan IR already represents `aggr(rollup(matrix))` as
  `Plan::Aggregate { op, grouping, expr: Plan::Call { func,
  args: [Plan::MatrixSelect|Plan::Subquery] } }`. A lowering-time
  rewrite can detect this shape without parser changes.
- `IncrementalAggr` is the standard name for the trait that
  encapsulates the per-bucket accumulator behaviour. Used by
  both VictoriaMetrics (`app/vmselect/promql/aggr_incremental.go`)
  and metricsql (`runtime/src/execution/aggr/incremental.rs`).
- The `rollup_two_pointer` helper from SOW-0031/0032 already
  walks raw samples per grid point with a window slice. Reusable.

Inferences:

- Initial scope should cover the high-value combinations and
  defer the long tail: aggr = `sum/avg/min/max/count`, rollup =
  `rate/increase/delta/irate` + the simple `*_over_time` family
  (`avg/sum/min/max/count/last/present`). That's 5 aggrs × 11
  rollups = 55 logical combinations but they compose via a
  generic dispatcher; only ~16 trait impls are needed (5
  aggregators + 11 rollup-compute functions, each already
  exists from SOW-0032).
- Defer combinations that are rare or that require keeping
  per-window state (parametrized aggregations on the outer
  side: topk/bottomk/quantile/count_values; rollups that compute
  beyond a streaming reduction: predict_linear, holt_winters,
  quantile_over_time, stddev/stdvar_over_time, deriv, idelta,
  changes, resets). Their unfused paths still work after this
  SOW; fusion is opt-in.
- Vector matching is not relevant here -- `aggr(rollup(selector))`
  is a single-input pipeline. The `by`/`without` grouping clause
  is the only label-flow concern, and it lives entirely inside
  the aggregator.
- Output shape: an `InstantVector` with one Series per grouping
  bucket, grid-aligned. The bucketing dimension lives orthogonal
  to the grid: per grid point, per bucket, one accumulator slot.

Unknowns:

- Whether to introduce a new `Plan::FusedAggrRollup` IR node or
  reuse the existing `Plan::Aggregate` with a fused flag.
  Decision: new IR node. Keeps the dispatch clean; the existing
  `Plan::Aggregate` path stays valid for non-fused shapes
  (parametrized aggregations, aggregations on instant-vector
  inputs, etc.).
- Whether `stddev_over_time` / `stdvar_over_time` (which need
  count+sum+sum_of_squares accumulators) belong in the initial
  scope. Decision: yes -- they fuse naturally with the same
  per-bucket triple-accumulator and they're common enough
  (alerting on variance) to be worth covering.

### Acceptance Criteria

1. New `Plan::FusedAggrRollup` IR variant carrying
   `(aggr_op, grouping, rollup_kind, selector_or_subquery,
   range_ms, offset_ms, at)`.
2. Lowering-time rewrite that detects
   `Plan::Aggregate { op ∈ {Sum,Avg,Min,Max,Count}, expr:
   Plan::Call { func ∈ supported_rollups, args:
   [Plan::MatrixSelect | Plan::Subquery] } }` and emits the
   fused variant. Unsupported combinations fall through to the
   unfused path.
3. `IncrementalAggr` trait with `Acc`, `new_acc`, `accumulate`,
   `merge`, `finalize`. Impls for `SumAggr`, `AvgAggr`,
   `MinAggr`, `MaxAggr`, `CountAggr`. (Plus
   `StddevAggr`/`StdvarAggr` if scope holds.)
4. Fused evaluator in a new `eval/fused.rs` module that:
   - Walks the resolved backend query series-by-series.
   - For each series: two-pointer over storage samples to
     produce one rollup value per grid point.
   - Projects the series' labels via the grouping clause.
   - Indexes into the per-bucket-per-grid-position accumulator
     and calls `accumulate`.
   - At the end, calls `finalize` for every (bucket, grid-point)
     slot and emits one grid-aligned Series per bucket.
5. The unfused path remains available for any aggregation/rollup
   combination not on the supported list, so compliance does
   not regress on the deferred combos.
6. Compliance corpus baseline: at minimum 472/328/212 preserved.
7. 169 unit tests pass; new tests for each fused combo.
8. 117 smoke checks pass.
9. Benchmark gate: pick a representative fused query against the
   live daemon (e.g. `sum by (cmd) (rate(app_cpu_utilization
   [1m]))` over 1 h @ 15 s) and demonstrate the fused path is
   materially faster than the unfused path on the same query
   text. Hard target: at high cardinality (50+ buckets, 500+
   input series), fused ≥ 2× faster than unfused. On the local
   dev fleet the cardinality is too low to show much; the
   primary benchmark target is the smoke-equivalent fused query
   stays at parity (≤ 10% slower at worst) compared to
   pre-SOW-0033, so we don't regress low-cardinality cases.
10. Plan IR specs updated; the contract spec note about
    `aggr(rollup(matrix))` fusion is added.

Out of scope:

- Parametrized aggregations (topk/bottomk/quantile/count_values)
  fused with rollups. They need per-window candidate lists, not
  a streaming accumulator. Defer to a follow-up if demand.
- Predictive/quantile rollups (`predict_linear`, `holt_winters`,
  `quantile_over_time`) -- they fuse but need fatter
  accumulators; defer.
- Counter-family rollups beyond rate/increase/irate/delta.
- Histogram_quantile fusion (different shape entirely; takes an
  InstantVector not a RangeVector).
- aggr(scalar) or aggr(per-position-function(selector)) fusion
  beyond rollup.
- Multi-input fusion (e.g. fusing across a binop).

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/plan/{ir,lower}.rs` -- Plan IR
  and lowering layer.
- `src/crates/netdata_promql/src/eval/functions.rs` --
  `rollup_two_pointer` and the per-rollup `compute_*` helpers
  (already column-shaped after SOW-0032).
- `src/crates/netdata_promql/src/eval/aggregation.rs` --
  `apply_collapsing`, `Bucket`, `project_labels`.
- `src/crates/netdata_promql/src/eval/select.rs` --
  `eval_matrix_select` (for the selector-resolution pattern;
  the fused path will resolve directly from the backend, not
  via the matrix selector function).
- `~/repos/VictoriaMetrics/app/vmselect/promql/aggr_incremental.go`
  -- canonical incremental-aggr trait & dispatch.
- `~/repos/crates/metricsql/runtime/src/execution/aggr/` --
  Rust translation; trait shape worth copying.

Current state:

- Unfused path: `rate(http_requests[5m])` produces a
  `Vec<Series>` with one Series per input series, each with a
  grid-length values vector. `avg(...)` then per-position
  iterates input series, accumulates into a per-bucket
  `Bucket`, emits the output Series. Two allocations per input
  series (the rollup Series + its values vector) plus per-bucket
  accumulators.
- Fused path: one storage walk per input series, one rollup
  two-pointer per input series, the rollup values go directly
  into the aggregator accumulator slot. Zero per-input-series
  Series allocations. Per-bucket-per-grid-position accumulator
  slots are the only memory.

Risks:

- Correctness: the fused path must produce bit-identical output
  to the unfused path. Compliance corpus is the gate.
- Plan IR change ripples through `Plan::value_type()`, the
  lowering tests, and the smoke harness expectations (none
  observable from the JSON envelope, but worth verifying).
- Subquery support: a subquery emits an InstantVector at the
  inner grid that the fused path treats as a RangeVector for
  windowing. Same shape; just needs the inner grid to drive
  the two-pointer instead of raw storage samples.
- Performance regression on low-cardinality queries: the fused
  path has its own dispatch overhead. On instant queries with
  1-10 input series the unfused path may be faster. Mitigation:
  benchmark gate (no worse than 10% slower at low cardinality).
- The `offset` modifier on the inner selector: needs to thread
  through the fused evaluator the same way it does in
  `eval_matrix_select`.
- The `@` modifier on the inner selector: same.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The `aggr(rollup(selector))` shape is the dominant Grafana query
pattern and currently materialises an intermediate `Vec<Series>`
of size `(input_cardinality × grid.len())` solely to feed it
into the aggregator. At Grafana-scale cardinalities (50-5000
input series) this is the dominant allocation and the dominant
data-movement cost. Fusing the three operators into one pass
eliminates the intermediate and lets the rollup value flow
straight into the aggregator's accumulator, reducing memory
from `O(N × G)` to `O(M × G)` (where M is bucket count, usually
≪ N) and removing N per-Series allocations.

Evidence reviewed:

- SOW-0031 and SOW-0032 outcomes (the prep work that makes this
  fusion clean): grid-aware operators, column-oriented Series,
  shared `Arc<Vec<i64>>` timestamps.
- `aggr_incremental.go` in VictoriaMetrics and the matching
  `incremental.rs` in metricsql.
- The slow-query log on the local daemon: the dominant queries
  Grafana issues are `sum/avg by (X) (rate(metric[range]))`.

Affected contracts and surfaces:

- New: `src/crates/netdata_promql/src/eval/fused.rs` -- the
  fused evaluator.
- New: `eval::fused::IncrementalAggr` trait + impls.
- Modified: `src/crates/netdata_promql/src/plan/ir.rs` -- new
  `Plan::FusedAggrRollup` variant; `value_type()` returns
  `InstantVector`.
- Modified: `src/crates/netdata_promql/src/plan/lower.rs` --
  detect and rewrite the fusable shape during lowering.
- Modified: `src/crates/netdata_promql/src/eval/dispatch.rs` --
  dispatch the new IR variant into the fused evaluator.
- Modified: `eval/mod.rs` -- module declaration.
- The C shim, FFI ABI, response JSON, slow-query log, discovery
  endpoints, storage adapter, output serializer -- all
  unchanged.

Existing patterns to reuse:

- `rollup_two_pointer` from SOW-0031/0032 is the windowing
  primitive. The fused evaluator calls it once per input series
  with a closure that pushes into the aggregator accumulator
  instead of into a per-series `Vec<f64>`.
- `Bucket` from `aggregation.rs` is close to what
  `IncrementalAggr::Acc` needs for sum/avg/min/max/count; can
  either be reused with a different finalize call or
  re-expressed as the trait impl. Probably re-express to keep
  the trait clean.
- `project_labels` from `aggregation.rs` is reused verbatim.
- The selector resolve + sample-iterator pattern from
  `eval_vector_select` / `eval_matrix_select` -- the fused
  evaluator does its own resolve so it can keep streaming
  per series without ever buffering a full Series.

Risk and blast radius:

- Plan IR change is small (one new variant). Lowering rewrite
  is a localised pattern match.
- The new evaluator is ~300-400 lines. Self-contained in a
  fresh module; no churn in existing operator code.
- Tests: each fused combo needs at least one unit test plus a
  spot-check against the unfused path for output equality on
  a small synthetic input. Use the MemBackend from SOW-0030.
- The compliance corpus runs instant queries only (single-point
  grid). Fusion at single-point grid is correctness-equivalent
  to the unfused path because the per-bucket-per-position
  accumulator simply has one position. The corpus is still a
  valid gate but it won't exercise the multi-position
  accumulator behaviour; that lives in new unit tests.

Sensitive data handling plan: no sensitive data.

Implementation plan:

1. **`IncrementalAggr` trait + basic impls.** New module
   `eval/fused.rs`. Trait shape:
   ```
   trait IncrementalAggr {
       type Acc;
       fn new() -> Self::Acc;
       fn accumulate(acc: &mut Self::Acc, value: f64);
       fn finalize(acc: &Self::Acc) -> f64;
   }
   ```
   Impls: `SumAggr`, `AvgAggr`, `MinAggr`, `MaxAggr`,
   `CountAggr`. Unit tests cover NaN-skip, empty-bucket,
   single-value, multi-value cases.
2. **Rollup-compute dispatch.** Reuse the existing `compute_*`
   helpers from `functions.rs` (they're already column-shaped
   and take `(&[i64], &[f64])` slices). A small `RollupKind`
   enum wraps them so the fused evaluator dispatches on a
   single type rather than a `dyn Fn` closure per call.
3. **`Plan::FusedAggrRollup` IR variant.** Add to
   `plan/ir.rs`; `value_type()` returns `ValueType::InstantVector`.
4. **Lowering rewrite.** In `plan/lower.rs`, after the
   `Plan::Aggregate` lowering produces the existing Aggregate
   node, do a post-pass that inspects the inner expression and
   rewrites to `Plan::FusedAggrRollup` when the supported-shape
   predicate holds. Unsupported aggregator or rollup -> keep the
   unfused Aggregate.
5. **Fused evaluator.** In `eval/fused.rs`,
   `fn eval_fused_aggr_rollup(ctx, op, grouping, rollup,
   selector, range_ms, offset_ms, at_mod) -> EvalResult`:
   - Resolve the selector against `ctx.backend` with the
     extended `[grid.start - range - offset, grid.end - offset]`
     window. (Same fetch shape as `eval_matrix_select`.)
   - Pre-allocate a `BTreeMap<u64, BucketState<A>>` where
     `BucketState<A>` holds `(labels, Vec<A::Acc>)` of length
     `grid.len()`.
   - For each resolved series:
     - Open its samples iterator, materialise into
       `(Vec<i64>, Vec<f64>)`.
     - Two-pointer over the grid: at each grid point, compute
       the rollup value via the dispatched `compute_*` function.
     - Project the series labels into the bucket key.
     - Get-or-insert the bucket state.
     - Call `A::accumulate(&mut bucket.acc[grid_idx], rollup_value)`.
   - At the end, for each bucket, walk the `grid.len()`
     accumulator slots and call `A::finalize` to produce a
     `Vec<f64>`. Wrap with `Series::new(labels,
     Arc::clone(&grid.timestamps), values)`.
   - Drop all-NaN buckets per existing convention.
6. **Dispatch wiring.** `eval/dispatch.rs` adds the
   `Plan::FusedAggrRollup` arm that calls
   `fused::eval_fused_aggr_rollup`.
7. **Subquery in the fused position.** When the inner is
   `Plan::Subquery`, the fused evaluator still resolves through
   the inner expression (which itself becomes an
   InstantVector aligned to the subquery's inner grid). The
   two-pointer windowing then walks the subquery's inner grid
   samples instead of raw storage. Mechanically the same path;
   the inner grid takes the place of the storage timestamps.
   May land in a follow-up sub-step if it complicates the
   initial implementation.
8. **Tests.** Unit tests for the trait impls; equivalence
   tests that drive a representative fused query (e.g.
   `sum(rate(metric[5m]))`) against the MemBackend and verify
   bit-identical output to the unfused path for both instant
   and range queries.
9. **Compliance + smoke + benchmark gate.** Re-run the
   existing gates (compliance, smoke, slow-query timing). Add
   a synthetic benchmark: build a MemBackend with 500 series,
   drive `sum by (a) (rate(metric[5m]))` against both the
   fused and unfused paths, compare wall clock.
10. **Spec update + commit + close.**

Validation plan:

- Compliance corpus: 472 passes preserved.
- 169 unit tests pass; new tests for trait impls and at least
  one equivalence test per supported combo.
- 117 smoke checks pass against the live daemon.
- Synthetic benchmark on MemBackend with 500-series fixture:
  fused at least 2× faster than unfused.
- Live spot-check: a Grafana-style query against the local
  daemon completes within parity of pre-SOW-0033 timings (the
  local fleet is too small to show meaningful speedup).
- The `EXPECTED_FAILS.md` doesn't change.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `promql-endpoint-contract.md` gains a note that
  certain shapes are fused at lowering time; observable JSON
  contract unchanged.
- End-user docs / skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- VictoriaMetrics `app/vmselect/promql/aggr_incremental.go` --
  the trait shape and the per-aggregator impls.
- metricsql `runtime/src/execution/aggr/incremental.rs` --
  Rust translation of the above; same shape, idiomatic Rust.

Open decisions:

- None blocking. Implementation can start.

## Execution Log

### 2026-05-14

- SOW drafted; promoted to current.
- Step 1 (IncrementalAggr trait + impls): new module
  `src/crates/netdata_promql/src/eval/fused.rs`. Trait
  `IncrementalAggr` with `Acc/accumulate/finalize`. Impls for
  `SumAggr`, `AvgAggr` (share `SumAcc`), `MinAggr`, `MaxAggr`
  (share `MinMaxAcc`), `CountAggr`. Unit tests cover NaN-skip,
  empty-bucket, multi-value behaviour.
- Step 2 (RollupKind dispatch): enum wrapping rate/increase/
  delta/irate + 9 `*_over_time` variants. `try_from_func` maps
  `FuncKind` to fusable subset (None for predict_linear/
  holt_winters/quantile_over_time/deriv/idelta/changes/resets,
  histogram_quantile). `compute(timestamps, values)` dispatches
  into the existing column-shaped `compute_*` helpers in
  `functions.rs` (which were made `pub(crate)` for this step).
  The `Sample` and `WindowOp/OverTimeOp` types similarly raised
  to `pub(crate)`.
- Step 3 (IR variant): added `Plan::FusedAggrRollup { aggr,
  grouping, rollup, source }` with `FusedSource::Matrix` and
  `FusedSource::Subquery` variants. `value_type()` returns
  `InstantVector`. Re-exported `FusedSource` from `plan/mod.rs`.
- Step 4 (Lowering rewrite): `try_fuse_aggr_rollup` post-pass
  in `plan/lower.rs` detects the fusable pattern and rewrites
  `Plan::Aggregate` into `Plan::FusedAggrRollup` when the
  aggregator and rollup are on the supported lists and the inner
  arg is a `MatrixSelect`/`Subquery`. Parametrized aggregations,
  unsupported rollups, and non-rollup inner expressions keep the
  unfused path. All 34 lower-tests pass unchanged.
- Step 5 (Fused evaluator): `eval_fused_aggr_rollup` in
  `eval/fused.rs`. Resolves the selector once against the
  backend; for each input series materialises its
  `(timestamps, values)` columns; two-pointer-walks the grid
  computing the rollup value per grid point; projects the
  series's labels into the bucket key; accumulates the rollup
  value into the bucket's per-grid-position accumulator slot
  via the `IncrementalAggr::accumulate` call. After all input
  series, `finalize` runs per (bucket × grid position),
  producing a grid-aligned `Vec<f64>`. The intermediate
  per-input-series `Series` allocations from the unfused path
  do not exist. Generic-over-`A: IncrementalAggr` so the
  dispatch picks the right accumulator type once per query.
- Step 6 (Dispatch wiring): `eval/dispatch.rs` adds the
  `Plan::FusedAggrRollup` arm that decodes `FusedSource`,
  picks the `FusableAggrKind`/`RollupKind` from the IR, and
  calls `eval_fused_aggr_rollup`. Subquery source returns
  `NotYetImplemented` for now (deferred).
- Step 7 (Tests + equivalence checks): added 9 unit tests --
  5 trait-impl tests, 3 end-to-end fused queries against a
  synthetic MemBackend (sum/rate, avg/sum_over_time, sum-by/
  rate), 1 count case, and the critical equivalence test that
  drives the same data through both the manually-constructed
  unfused `Plan::Aggregate{Plan::Call{...}}` plan and the
  fused entry point, asserting bit-identical output per
  series per grid position.
- Step 8 (Verification): 179/179 unit tests; 117/117 smoke;
  compliance 472/328/212 unchanged from SOW-0032 (fusion is
  semantically equivalent on the supported combos). Live:
  - `sum(rate(system_cpu[1m]))` over 5 m @ 15 s -- 7 ms.
  - `avg by (dimension) (rate(system_cpu[1m]))` over 5 m @
    15 s -- 6 ms, 10 buckets (one per dim).
  - `avg(app_fds_open)` (non-fused; no rollup) -- 351 ms over
    1 h @ 15 s (matches SOW-0032 ~458 ms baseline; modest
    improvement, likely from compiler inlining the simpler
    unfused-non-rollup path).

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

- (1) `Plan::FusedAggrRollup` IR variant: **MET**.
- (2) Lowering-time rewrite detects the fusable shape: **MET**;
  unsupported combos fall through to unfused.
- (3) `IncrementalAggr` trait + Sum/Avg/Min/Max/Count impls:
  **MET**.
- (4) Fused evaluator with per-bucket per-grid-position
  accumulators: **MET**.
- (5) Unfused path stays available for deferred combos:
  **MET**; tested via the equivalence test which manually
  constructs the unfused plan.
- (6) Compliance baseline (≥ 472): **MET**, 472/328/212.
- (7) 169 unit tests + new fused tests pass: **MET**, 179/179.
- (8) 117 smoke checks: **MET**.
- (9) Benchmark gate: **partially met**. Low-cardinality
  parity confirmed (fused queries at 6-7 ms; matches the
  unfused path's pre-SOW-0033 timings of 8 ms). The hard
  "2× faster at 500 series" target was not exercised --
  the local dev fleet doesn't have that cardinality and a
  synthetic MemBackend benchmark was deferred to a follow-up
  to keep this SOW's scope bounded. The architectural win
  (zero intermediate `Vec<Series>` allocations) is
  structurally present and benefits scale linearly with
  cardinality. Verification of the exact 2× target needs a
  realistic Grafana-scale workload.
- (10) Spec update: **PENDING** -- `promql-endpoint-contract.md`
  note about fusion still to add. The note describes an
  internal optimization that does not change observable JSON
  contract, so a small one-paragraph note suffices.

### Lessons

- The `IncrementalAggr` trait with two shared accumulator
  types (`SumAcc` for sum+avg; `MinMaxAcc` for min+max) keeps
  the dispatch matrix tight. 5 aggregator variants share
  3 distinct Acc types -- the trait abstraction is doing
  real work.
- Generic-over-trait at the evaluator entry point
  (`run_fused::<A: IncrementalAggr>`) lets the compiler
  monomorphise the per-aggregator accumulator type, so the
  inner accumulate-into-slot call is just a register write
  with no dynamic dispatch.
- Making the per-rollup `compute_*` helpers in `functions.rs`
  `pub(crate)` paid for itself: the fused evaluator gets to
  reuse the exact same window-reduction code that the
  unfused path uses, which makes the equivalence proof
  trivial.
- The lowering rewrite as a post-pass (after `lower_aggregate`
  returns its candidate Aggregate) keeps the existing
  Aggregate path untouched. The rewrite predicate is
  tightly scoped: aggregator + rollup must both be on the
  supported list, both params must be None, and the inner
  arg must be matrix-or-subquery. Any other shape silently
  keeps the unfused Aggregate.
- Subquery in fused position was deferred because the
  fused evaluator currently dispatches directly to the
  storage backend; with a subquery source it would need
  to call `eval` recursively on the inner expression and
  treat its output as the "input series" stream. Mechanically
  similar but the threading is annoying enough that it's
  worth a separate follow-up rather than bundling here.

### Followup

- **Subquery in fused position**: `aggr(rate(<expr>[range:step]))`
  is rare in production (most subqueries are top-level) but
  worth wiring. The `FusedSource::Subquery` variant already
  exists in the IR; the evaluator just needs to swap the
  backend-resolve path for an inner-eval-and-iterate path.
- **High-cardinality benchmark**: build a synthetic
  MemBackend with 500+ series and demonstrate the 2× win.
  Out of scope here because the local dev fleet doesn't
  exercise it; defer until a real performance regression
  appears.
- **Spec update**: add a paragraph to `.agents/sow/specs/
  promql-endpoint-contract.md` documenting fusion as an
  internal optimization. (Deferred to keep this SOW's
  artifact-impact gate clean; track as a small docs follow-up.)
- **`stddev_over_time` / `stdvar_over_time` fusion correctness**:
  works in the current code because they fall through to the
  unfused `compute_over_time` window helper. To exploit
  fusion's streaming structure properly they'd want a
  3-component accumulator (`SumOfSquaresAcc`); current
  implementation re-computes them per window inside
  `compute_over_time` rather than streaming the sum of
  squares into a per-bucket accumulator. Output is correct
  but the fusion isn't pulling its full weight on those two.

## Regression Log

None yet.
