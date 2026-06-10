# SOW-0032 - PromQL - Column-oriented Series

## Status

Status: completed

Sub-state: all eval modules migrated to column-oriented Series.
Shared `Arc<Vec<i64>>` timestamps across InstantVector results; per-
series `Arc<Vec<i64>>` for RangeVector results from matrix selectors.
472/328/212 compliance preserved; 169/169 unit tests; 117/117 smoke;
slow query 0.420 s (target ≤ 1 s; SOW-0031 baseline 0.458 s -- no
regression, marginal improvement).

## Requirements

### Purpose

Replace the row-shaped `Vec<Sample>` carried on every `Series` with
a column-shaped pair: `timestamps: Arc<Vec<i64>>` (shared across all
series in a result) and `values: Vec<f64>` (per-series). After
SOW-0031 every InstantVector result is grid-aligned -- every series
in the same result has identical timestamps. Today we materialise
those timestamps once per series, wasting memory and breaking SIMD
tightness on per-position loops.

The shape change is mainly a prerequisite for SOW-0033 operator
fusion (which needs a clean contiguous f64 buffer to push folds
into), but it also buys a modest standalone perf win on big-fanout
queries: ~10-20% on the eval portion of the wall clock, plus
hundreds of KB of timestamp deduplication at typical Grafana fanout.

### User Request

User accepted the original three-SOW split (SOW-0031 / SOW-0032 /
SOW-0033) after the architectural investigation in
`.agents/sow/done/SOW-0031-...`. With SOW-0031 closed and the
slow query at 0.458 s, user chose SOW-0032 "first as planned"
rather than skipping to SOW-0033 directly.

### Assistant Understanding

Facts:

- After SOW-0031, every `InstantVector` result has the invariant
  `series[i].samples.len() == grid.len()` and
  `series[i].samples[j].timestamp_ms == grid.timestamps[j]`. The
  timestamps are redundant across series.
- `RangeVector` results (matrix selectors before rollup) are
  **not** grid-aligned: each series carries raw storage samples
  at their actual timestamps. These do not share timestamps with
  other series in the same result.
- Hot loops in `binop.rs::pair_samples`, `aggregation.rs::
  apply_collapsing`, `functions.rs::rollup_two_pointer`, and the
  position-iterators in `binop.rs::scalar_vec` all index
  `s.samples[i].value` -- pulling a 16-byte `Sample` struct out
  of memory when only the 8-byte value is needed. Contiguous
  `Vec<f64>` is auto-vectorisable; `Vec<Sample>` is not.
- The output serializer (`output/prometheus_json.rs`) reads
  `sample.timestamp_ms` and `sample.value` and writes them as the
  `[ts, "v"]` pairs in JSON. It can read them from a paired
  `(timestamps[i], values[i])` just as easily.
- `Series` is constructed in ~20 places via
  `Series { samples: vec![Sample{...}], ... }` style literals,
  and read in ~138 places via `s.samples` or `.samples.first()`
  etc.

Inferences:

- The grid timestamps carried on `EvalContext.grid` are already an
  `Arc<Vec<i64>>`. Every InstantVector series can carry an
  `Arc::clone(&grid.timestamps)` for free.
- For RangeVector series we still need per-series timestamps. Keep
  the `timestamps: Arc<Vec<i64>>` field; each RangeVector series
  gets its own private Arc holding the storage timestamps. The
  Arc enables sharing when applicable without forcing it.
- The `Sample` struct stays in the public types for the storage
  layer and for any external consumers (the `testing` module).
  Internally, Series stores columnar form and exposes a `samples`
  iterator for read sites that prefer the (ts, value) shape.
- Output serialization gains a small fast path: it can write the
  values vector directly without unpacking a Sample struct per
  point.

Unknowns:

- Whether some existing read-site idioms (`samples.first()`,
  `samples.last()`, `samples.iter().enumerate()`) translate
  cleanly to the new shape. They mostly do; first/last become
  cheap operations on values/timestamps slices.
- Whether the `EvalResult::RangeVector` shape needs to change
  too. Currently `RangeVector(Vec<Series>)`. With column-Series
  the inner Series carries its own timestamps Arc; no shape
  change at the EvalResult level.

### Acceptance Criteria

1. `eval::Series` shape becomes
   `{ labels, signature, timestamps: Arc<Vec<i64>>, values: Vec<f64> }`.
   `values.len() == timestamps.len()` is enforced at construction.
2. Selectors clone `grid.timestamps` into every emitted
   InstantVector series. The grid's timestamps Arc is shared across
   all output series.
3. Matrix selectors build a fresh per-series `Arc<Vec<i64>>` from
   the storage samples. No sharing across series in a RangeVector.
4. Every operator that produces an InstantVector preserves the
   shared-Arc invariant: aggregations, binops, transforms, label
   ops, unary, rollups, parametrized aggregations, count_values,
   absent. The output uses the same `Arc::clone(&grid.timestamps)`
   that selectors used.
5. The output serializer reads `(timestamps[i], values[i])` pairs
   instead of `samples[i]`. JSON shape unchanged.
6. The legacy `samples()` iterator on `Series` is available for
   any read site that prefers (ts, value) pairs. Iterator,
   nothing more.
7. Compliance corpus baseline: at minimum 472/328/212 preserved.
   Improvements welcome.
8. 169 unit tests pass.
9. 117 smoke checks pass.
10. Live verification: `avg(app_fds_open{__ignore_usage__="",})`
    over 1 h @ 15 s stays under 1 s. Hard target: ≤ 1 s; the
    SOW-0031 baseline is 0.458 s and this SOW must not regress
    it noticeably (allow ≤ 10% drift either way).

Out of scope:

- Operator fusion (SOW-0033).
- Per-grid-point time() variation.
- topk/bottomk `__name__` preservation (pre-existing divergence
  in EXPECTED_FAILS.md).

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/eval/types.rs` (Series shape)
- `src/crates/netdata_promql/src/eval/{select,aggregation,binop,functions,labelops,unary,absent,subquery,dispatch}.rs`
- `src/crates/netdata_promql/src/output/prometheus_json.rs`
- `src/crates/netdata_promql/src/testing.rs`
- `src/crates/netdata_promql/tests/compliance.rs`
- `~/repos/VictoriaMetrics/lib/promrelabel/timeseries.go` (Timeseries shape)
- `~/repos/crates/metricsql/runtime/src/types/timeseries.rs`
  (`Timeseries { metric, values: Vec<f64>, timestamps: Arc<Vec<i64>> }`)

Current state:

- 138 read sites of `s.samples` across the eval crate.
- 20 construction sites that build `Series { samples: vec![Sample{}] }`.
- One Sample type lives in `storage::samples` (with flags); a
  parallel `eval::types::Sample` exists with just `(timestamp_ms,
  value)`. The latter is what InstantVector series carry today.

Risks:

- High touch surface: every operator file changes, every test
  fixture changes. Compliance baseline is the verification floor.
- The output serializer must read from the new shape correctly
  including the NaN filter introduced in SOW-0031.
- Tests build Series via struct literals all over the place.
  Replacing each requires either a new constructor or a sed-style
  rewrite. Mechanical but tedious.
- Subtle invariant: shared-Arc timestamps for all InstantVector
  series in a result. Bugs that emit series with mismatched
  timestamps would produce wrong serialization for range queries.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

After SOW-0031, every InstantVector series in a result shares the
same logical timestamps (the grid). Today each series materialises
its own Vec<Sample> of (ts, value) pairs, duplicating the timestamp
column. Hot loops index .samples[i].value to pull an f64 out of a
16-byte struct, defeating SIMD vectorisation and wasting cache
lines on the unused timestamp column. The fix is a shape change:
column-oriented Series with shared Arc-of-timestamps for grid-
aligned results, contiguous Vec<f64> values for inner loops.

Evidence reviewed:

- `eval/types.rs` (current Series shape).
- SOW-0031 lessons (the slow query bottleneck was per-sample
  storage decompression, not eval; eval is ~10% of wall clock).
- `~/repos/crates/metricsql/runtime/src/types/timeseries.rs` and
  `~/repos/VictoriaMetrics/lib/promrelabel/timeseries.go` -- both
  use column-oriented `Timeseries { values: Vec<f64>, timestamps:
  Arc<Vec<i64>> }`.
- The compliance corpus (472/328/212 baseline post-SOW-0031).

Affected contracts and surfaces:

- Modified: `eval/types.rs` -- Series shape.
- Modified: `eval/{select,aggregation,binop,functions,labelops,unary,absent,subquery,dispatch}.rs`
  -- every Series construction and read site.
- Modified: `output/prometheus_json.rs` -- serialize from
  (timestamps[i], values[i]) instead of samples[i].
- Modified: `testing.rs` -- TestSeries unchanged externally, but
  the internal conversion reads from the new shape.
- Modified: `eval/grid.rs` -- no shape change; `grid.timestamps`
  is already an Arc<Vec<i64>>.
- Modified: every test fixture that constructs Series.
- FFI surface, JSON shape, slow-log format, discovery endpoints,
  the storage adapter -- all unchanged.

Existing patterns to reuse:

- `Arc<Grid>` plumbing from SOW-0031: selectors already access
  `ctx.grid` and can clone its Arc<Vec<i64>>.
- The `rollup_two_pointer` helper from SOW-0031 emits
  per-grid-point values; trivial to adapt to push into a
  pre-allocated Vec<f64>.
- `Bucket::finalize_value(op)` from SOW-0031 returns a single
  f64; the per-position aggregation loop already pushes f64s.
  Converting the BTreeMap value type from `Vec<Sample>` to
  `Vec<f64>` is mechanical.

Risk and blast radius:

- Behavioural preservation: every output must match the SOW-0031
  baseline bit-for-bit. Compliance corpus is the gate.
- Test mass: ~30-40 unit tests construct Series fixtures. All
  need translation. A `Series::from_pairs(labels, &[(ts, v)])`
  test helper minimises churn.
- Two Sample shapes can cause confusion: `storage::samples::Sample`
  has `flags`, `eval::types::Sample` is `(ts, value)`. After this
  SOW, the eval Sample is only used at boundaries (storage
  ingest, test fixtures, JSON output). Worth documenting.

Sensitive data handling plan: no sensitive data.

Implementation plan:

1. **Shape change**: rewrite `eval::Series` to column form. Add a
   `samples()` adapter iterator. Add a `Series::scalar(ts, value)`
   convenience constructor. Add a `Series::from_grid_values(labels,
   timestamps_arc, values)` constructor.
2. **Selectors**: `eval_vector_select` clones `Arc::clone(&grid
   .timestamps)` for each emitted series; the values vector is
   built in the existing two-pointer loop. `eval_matrix_select`
   builds a fresh `Arc<Vec<i64>>` per series from storage
   timestamps.
3. **Aggregations**: rewrite `apply_collapsing`, `apply_topk_bottomk`,
   `apply_quantile`, `apply_count_values` to operate on columnar
   input and emit columnar output. The grid timestamps Arc threads
   through from the input series.
4. **Rollups**: `rollup_two_pointer` now returns a `Vec<f64>`;
   each rollup outer wraps it into a Series with
   `Arc::clone(&grid.timestamps)`. Legacy single-window paths
   build a 1-element timestamps Arc.
5. **Per-position operators**: `binop`, `labelops`, `unary`,
   transforms in `functions.rs`. Each reads `s.values[i]` and
   pushes into a pre-allocated `Vec<f64>` of length `grid.len()`.
6. **Subquery + absent**: subquery's inner eval already produces
   InstantVector with grid-aligned series; just propagate the
   shape. Absent builds a single-series output with the outer
   grid's timestamps Arc.
7. **Output serializer**: `write_range_series` reads
   `(timestamps[i], values[i])` directly. Same NaN filter.
8. **Testing module + tests**: TestSeries unchanged; the
   conversion in `eval_instant_against` reads `s.values[0]`
   instead of `s.samples.first()`. Unit test fixtures use the new
   constructor.
9. **Compliance + smoke + live verification**: run the same
   gates as SOW-0031.
10. **Commit + close**.

Validation plan:

- Compliance corpus: 472 passes preserved.
- 169 unit tests pass.
- 117 smoke checks pass against the live daemon.
- Wall clock on the slow query: ≤ 1 s (currently 0.458 s); no
  significant regression.
- A spot-check on a couple of binop / aggregation queries that
  the JSON output is unchanged.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: small note in `promql-endpoint-contract.md` that the
  internal Series shape is column-oriented; observable JSON
  contract unchanged.
- End-user docs / skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- `valyala/VictoriaMetrics @ <track>` -- column Timeseries
  layout.
- `iredelmeier/metricsql @ <track>` -- Rust Timeseries
  `{values: Vec<f64>, timestamps: Arc<Vec<i64>>}`.

Open decisions:

- None blocking. Implementation can start.

## Execution Log

### 2026-05-14

- SOW drafted; promoted to current.
- Step 1 (Shape change): rewrote `eval::Series` to
  `{ labels, signature, timestamps: Arc<Vec<i64>>, values: Vec<f64> }`.
  Added `Series::new`, `Series::scalar`, `Series::from_samples`
  constructors and `samples()` / `collect_samples()` /
  `first()` / `last()` / `len()` accessor helpers. Sample remains as
  the boundary (ts, value) pair shape for storage I/O and JSON output.
- Step 2 (Selectors): `eval_vector_select` clones
  `Arc::clone(&grid.timestamps)` for every InstantVector series. The
  two-pointer lookback now writes into a `Vec<f64>` of length
  `grid.len()` rather than a `Vec<Sample>`. `eval_matrix_select`
  builds a fresh per-series `Arc<Vec<i64>>` from the storage
  samples; no sharing across series because storage timestamps
  differ per series.
- Step 3 (Aggregations): rewrote `apply_collapsing`,
  `apply_topk_bottomk`, `apply_quantile`, `apply_count_values` to
  iterate `s.values[i]` directly and emit grid-aligned output via
  `Series::new(labels, Arc::clone(&timestamps), values_vec)`. The
  `Bucket` accumulator dropped its `ts` field (now passed in via
  the per-position loop directly). topk/bottomk in particular gained
  a HashMap-of-ranked-input-indices pass per position so the output
  series carry NaN at positions where they didn't rank.
- Step 4 (Rollups): changed `rollup_two_pointer` signature from
  `(samples: &[Sample], grid_ts, range_ms, compute)` to
  `(timestamps: &[i64], values: &[f64], grid_ts, range_ms, compute)`
  returning `Vec<f64>`. Every `compute_*` helper (`compute_over_time`,
  `compute_window_op`, `compute_irate`, `compute_delta`,
  `compute_quantile_over_window`, `compute_predict_linear`,
  `compute_holt_winters`, `compute_deriv`, `compute_idelta`,
  `compute_changes`, `compute_resets`) now takes the column shape.
  Each `apply_*` grid-aware path constructs the output Series with
  `Arc::clone(&grid.timestamps)`. The `_legacy` companions use
  `Series::scalar`.
- Step 5 (Per-position operators): `binop`'s `scalar_vec` now
  iterates `s.values.iter_mut()` directly and sets NaN at
  comparison-as-filter failure positions (instead of dropping the
  whole series at the first failure -- the new model preserves
  grid alignment). `pair_samples` renamed to `pair_values` and
  takes `(&[f64], &[f64])` returning `Option<Vec<f64>>`.
  `combine_pair` now constructs output via `Series::new`.
  `map_samples` (the per-sample transform helper) takes column form.
  `unary::negate_series` writes through `s.values.iter_mut()`.
  Label ops construct output Series with `Series::new(labels,
  s.timestamps, s.values)` -- the timestamps Arc just propagates.
- Step 6 (Subquery + absent): subquery already produced an
  InstantVector via the SOW-0031 grid-recursion path; only the
  inner construction site needed adjustment. `eval_absent`
  rewrote to walk `s.values[i].is_nan()` per position, emit a
  grid-aligned values vector, and wrap it with
  `Arc::clone(&grid.timestamps)`.
- Step 7 (Output serializer): `write_instant_series` reads
  `s.timestamps.first()` / `s.values.first()`; `write_range_series`
  walks the values vector with the timestamps vector in lockstep
  and keeps the NaN filter introduced in SOW-0031.
- Step 8 (Tests + verification): `series()` test helpers updated
  to use `Series::from_samples`. ~30 tests had
  `v[0].samples[0].value` rewritten to `v[0].values[0]` and a few
  `samples[0].timestamp_ms` to `timestamps[0]`. Removed an
  unused `Sample` import from absent.rs. Compliance 472/328/212,
  unit 169/169, smoke 117/117. Live: avg(app_fds_open) over
  1 h @ 15 s in 0.420 s (target ≤ 1 s; SOW-0031 baseline 0.458 s).

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

- (1) Column-oriented Series shape with `Arc<Vec<i64>>` + `Vec<f64>`:
  **MET**.
- (2) Selectors clone `grid.timestamps` into every output series:
  **MET**.
- (3) Matrix selectors build per-series Arc from storage samples:
  **MET**.
- (4) Every InstantVector-producing operator preserves the
  shared-Arc invariant: **MET**.
- (5) Output serializer reads (timestamps[i], values[i]): **MET**.
- (6) `samples()` iterator adapter available for boundary code:
  **MET**.
- (7) Compliance baseline preserved (≥ 472): **MET**, 472/328/212.
- (8) 169 unit tests pass: **MET**.
- (9) 117 smoke checks: **MET**.
- (10) Live target ≤ 1 s: **MET**, 0.420 s.

### Lessons

- Most of the heavy mechanical churn lived in `functions.rs`'s
  legacy rollup paths -- not the grid-aware paths. The
  `_legacy` companions exist purely so the existing per-window
  tests stay valid without forcing every test to construct an
  EvalContext. Worth the duplication.
- The `compute_*` helpers' signature change from `&[Sample]` to
  `(&[i64], &[f64])` was the only mechanically painful part of
  this SOW; once those moved, the apply wrappers fell into place.
- The shared-Arc invariant for InstantVector results is easy to
  preserve if every Series-constructing operator threads through
  the input's `timestamps` Arc. Caught two regressions
  (label ops and timestamp()) where I'd nearly dropped the Arc
  by building a fresh vector; refactored to take `s.timestamps`
  out of the moved struct first.
- Performance gain was modest as predicted (~9% on the slow
  query). The substantive payoff is SOW-0033 fusion having a
  clean contiguous f64 buffer to push folds into.

### Follow-up

- The `pad_to_grid` helper in aggregation.rs (added in SOW-0031,
  now adapted to the column shape) is dead-ish for the common
  case where every bucket gets a value at every position; could
  be inlined or simplified further. Cosmetic.
- `Sample` is now mostly a boundary type. The storage adapter
  still emits Sample at the FFI edge; the eval crate uses it
  for tests and the small set of legacy single-window rollup
  paths. Long term it could move out of `eval::types` into a
  storage-boundary module, but that's not load-bearing.

## Regression Log

None yet.
