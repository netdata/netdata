# SOW-0027 - PromQL - Function omnibus (Tier 1 + Tier 2)

## Status

Status: completed

Sub-state: shipped 2026-05-14; 27 new functions across 7 groups.
156 Rust unit tests / 117 smoke checks (was 125 / 95).

## Requirements

### Purpose

Close the everyday-function gap in the PromQL evaluator. The
implementation currently supports 17 functions out of Prometheus'
~65. The 27 missing functions covered by this SOW are the ones
real-world queries hit first: `abs`, `vector`/`scalar`, the
`clamp` family, `sort`/`sort_desc`, `absent`/`absent_over_time`,
`label_replace`/`label_join`, `time`/`timestamp`, the
`ceil`/`floor`/`round` rounding family, the `ln`/`log2`/`log10`/
`exp`/`sqrt`/`sgn` math family, the `deriv`/`idelta` gauge
family, and `changes`/`resets`. Together they cover the Tier 1
and Tier 2 buckets identified in conversation.

### User Request

"Which other frequently used functions are not supported by our
implementation?" -> user accepted the tiered breakdown and
explicitly asked: "Yes, please." (draft the SOW for the omnibus).

### Assistant Understanding

Facts:

- `FuncKind::from_name` in `plan/ir.rs` currently routes 17
  function names. Every other name returns `None`, which
  surfaces as `LowerError::Call("unknown function ...")` at
  HTTP 400.
- Each existing function variant has one arm in
  `eval/functions.rs::apply_call`. The dispatch shape is stable
  and accommodates more arms cheaply.
- The crate already depends on `regex_lite` (or equivalent --
  see `storage/matchers.rs`) so `label_replace` adds no new
  build dependency.
- The shared linear-regression helper used by `predict_linear`
  in `eval/functions.rs` is reusable for `deriv` -- both compute
  slope; `deriv` simply returns the slope where `predict_linear`
  extrapolates by the user-supplied offset.
- Per the convention dropped on every existing transform,
  function output strips `__name__` from the per-series labels
  unless the function explicitly preserves it (Prometheus
  semantics: `rate`, all aggregations, math transforms drop the
  name; `last_over_time` and a few others keep it). The same
  rule applies here: transforms drop, label munging keeps.

Inferences:

- The functions split into seven implementation patterns. Group
  A (per-sample f64 transforms), B (per-sample with scalar
  params), C (vector restructuring), D (timestamp/time), E
  (range-vector reductions), F (label manipulation), G (empty
  -vector handling). Each group is a coherent block in the
  evaluator.
- `absent` needs the original `Expr` to copy static `=`
  matchers onto the synthesized output series. The Plan IR
  currently discards source-level expression info at lowering;
  the path of least resistance is to lower `absent` into a
  dedicated `Plan::Absent { matchers, expr }` carrying the
  pre-extracted matcher set rather than retaining the AST. Same
  shape for `absent_over_time`.
- `sort`/`sort_desc` collapse into the existing instant-vector
  output shape; no new IR variant needed -- they're regular
  `Call` variants whose evaluator arm sorts before returning.
- `label_replace`/`label_join` need argument types beyond
  scalar/vector. The parser already represents string literal
  args distinctly; the lowering layer needs to accept and carry
  them. Path of least resistance is a dedicated `Plan::LabelOp`
  variant that pre-extracts the string args at lowering, same
  way `count_values` carries its label-name string in
  `Plan::Aggregate::param_string`.

Unknowns:

- Whether `absent_over_time` should return a value timestamped
  at the eval time or at the window's right edge. Prometheus
  uses eval time; we mirror.

### Acceptance Criteria

1. All 27 functions parse and execute without
   `LowerError::Call("unknown function ...")`.
2. Group A transforms (`abs`, `ceil`, `floor`, `sgn`, `ln`,
   `log2`, `log10`, `exp`, `sqrt`) operate element-wise over an
   instant vector, drop `__name__`, and return an instant
   vector.
3. Group B transforms (`clamp(v,lo,hi)`, `clamp_min(v,lo)`,
   `clamp_max(v,hi)`, `round(v,to_nearest)`) accept their
   scalar arguments evaluated at lowering time, apply
   element-wise.
4. `vector(s)` returns a single instant-vector series with no
   labels and value `s`. `scalar(v)` returns the value of a
   single-series instant vector, or NaN if the input has zero
   or more-than-one series.
5. `sort(v)` / `sort_desc(v)` return the input instant vector
   sorted ascending / descending by value.
6. `absent(v)` returns 1 with synthesized labels from the
   query's static `=` matchers when `v` is empty; returns empty
   otherwise. `absent_over_time(v[w])` mirrors the rule for
   range vectors -- empty range -> single series of 1 at eval
   time.
7. `label_replace(v, dst, repl, src, regex)` regex-replaces the
   value of label `src` into label `dst` per Prometheus
   semantics. `label_join(v, dst, sep, src1, ...)` concatenates
   listed source label values with `sep` into `dst`.
8. `time()` returns the eval timestamp as a scalar in Unix
   seconds. `timestamp(v)` returns the timestamp of each
   series' latest sample.
9. `deriv(v[w])` returns the per-series linear-regression slope
   over the window. `idelta(v[w])` returns the difference of
   the last two stored samples. `changes(v[w])` and
   `resets(v[w])` count value transitions / counter resets.
10. Function output drops `__name__` except for
    `label_replace`, `label_join`, `sort`, `sort_desc`,
    `timestamp` (which preserve all labels including
    `__name__`).
11. Rust unit tests: target 30+ new tests covering parsing,
    type checks, and at least one positive-result test per
    function.
12. Smoke harness: 15+ live-daemon checks against
    representative inputs. The four functions least convenient
    in the harness (`label_replace`, `label_join`, `vector`,
    `scalar`) each get an explicit assertion of result shape.
13. Contract spec lists every function in the supported set;
    the "out of scope" section narrows further accordingly.

Out of scope:

- The full trigonometric family (`sin`, `cos`, `tan`, hyperbolic,
  `deg`, `rad`). Trivial individually but rarely used in
  monitoring queries; deferred to a future small SOW if user
  requests.
- Calendar functions (`minute`, `hour`, `day_of_week`, etc.).
  Needs local-time conversion plumbing; deferred.
- `histogram_*` (count, sum, avg, fraction). Tied to native
  histograms, which Netdata storage does not produce.
- `mad_over_time`, `double_exponential_smoothing` (newer
  Prometheus). Niche.
- `info()` (newer label-join-via-info-metric). Niche.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The dispatch and IR shapes are stable; the only obstruction is
that `FuncKind` does not enumerate the missing names and
`apply_call` has no arms for them. Adding them is mechanical
group by group; the only non-mechanical pieces are the IR
carrying of pre-extracted string/matcher args (for
`label_replace`, `label_join`, `absent`, `absent_over_time`) and
the regex compilation for `label_replace`.

Affected contracts and surfaces:

- Modified: `src/crates/netdata_promql/src/plan/ir.rs` --
  `FuncKind` enum gains 27 variants; new `Plan::Absent` and
  `Plan::LabelOp` variants for the two special shapes.
- Modified: `src/crates/netdata_promql/src/plan/lower.rs` --
  three new lowering arms (`absent`, `absent_over_time`,
  `label_replace`/`label_join`); the rest fold into the
  existing `lower_call`.
- Modified: `src/crates/netdata_promql/src/plan/mod.rs` --
  exports.
- Modified: `src/crates/netdata_promql/src/eval/dispatch.rs` --
  routes the two new Plan variants.
- New: `src/crates/netdata_promql/src/eval/labelops.rs` --
  `label_replace`, `label_join` implementation; reuses the
  storage layer's regex helper.
- New: `src/crates/netdata_promql/src/eval/absent.rs` --
  `absent` and `absent_over_time` with the synthesized-label
  rule.
- Modified: `src/crates/netdata_promql/src/eval/functions.rs` --
  new dispatch arms for groups A through E, organized into a
  dedicated section per group.
- Modified: `src/crates/netdata_promql/src/eval/mod.rs` --
  expose new modules.
- Modified: `tests/promql-smoke/run-smoke.sh` -- new Phase 3i
  section.
- Modified: `.agents/sow/specs/promql-endpoint-contract.md` --
  supported function list grows; out-of-scope list narrows.

No FFI changes; no shim changes.

Existing patterns to reuse:

- `eval/functions.rs::apply_call` dispatch shape -- one arm per
  function name routed from `FuncKind`.
- `compute_over_time` / `apply_predict_linear` for shared
  numeric helpers (linear regression, last-two-sample reduction).
- `plan/lower.rs::lower_aggregate` 's three-flavor parameter
  dispatch for the `param_string` precedent -- carries from
  parser AST string literal into the Plan IR.
- `aggregation::compute_quantile` and the wider sorted-output
  pattern for `sort`/`sort_desc`.
- `storage/matchers.rs::compile_regex` for `label_replace`'s
  regex compilation.

Risk and blast radius:

- Low. All work is additive to the function surface. No
  existing function semantics change. No FFI change.
- Performance: 27 functions, each O(series) or O(samples)
  per call; no new quadratic paths.
- Correctness: `label_replace` regex semantics are the only
  area where a subtle mismatch with Prometheus could surface.
  Mitigated by porting the relevant test cases from
  Prometheus' `functions.test` and `name_label_dropping.test`
  into smoke or unit form.

Sensitive data handling plan:

No sensitive data in scope. The SOW, tests, and code touch
only synthetic PromQL fixtures and live local Netdata storage.

Implementation plan (single chunk):

1. Extend `FuncKind` and `from_name` with the 27 names.
2. Group A: implement `apply_per_sample` helper plus the nine
   `f64`-mapped transforms.
3. Group B: extend the helper to accept 1 or 2 scalar params;
   wire `clamp`, `clamp_min`, `clamp_max`, `round`.
4. Group C: `vector`, `scalar`, `sort`, `sort_desc`.
5. Group D: `time` (scalar from ctx), `timestamp` (per-series
   from `samples[0].timestamp_ms`).
6. Group E: `deriv`, `idelta`, `changes`, `resets` -- new
   arms in the existing range-vector dispatch.
7. Group F: new `eval/labelops.rs` for `label_replace` and
   `label_join`; new `Plan::LabelOp` variant in the IR; new
   lowering arm that pre-extracts the string args.
8. Group G: new `eval/absent.rs` for `absent` and
   `absent_over_time`; new `Plan::Absent` variant carrying
   the static-`=` matcher set captured at lowering time;
   new lowering arm.
9. Rust unit tests (per group).
10. Smoke harness Phase 3i block.
11. Spec update (`.agents/sow/specs/promql-endpoint-contract.md`).
12. Close (status: completed; move to `done/`).

Validation plan:

- Rust unit tests: target 30+ new (~155 total).
- Smoke harness: target 15+ new (~110 total) against the
  live daemon at localhost:19999.
- Live verification of the originally-broken query:
  `abs(rate(disk_io{device_type="physical"}[1m])) /
   ignoring(chart, family)
   abs(rate(disk_ops{device_type="physical"}[1m]))`
  succeeds and returns positive values across reads and writes.

Artifact impact plan:

- `AGENTS.md`: no change (no workflow or policy change).
- Runtime project skills: no change.
- End-user docs / skills: no change (the PromQL endpoint is
  still phase-gated and not yet announced to end users).
- Spec: updated -- supported function list grows; out-of-scope
  list narrows to the four explicitly deferred families
  (trig, calendar, native-histogram, niche-newer).
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence: Prometheus
`prometheus/prometheus @ <current commit>` --
`promql/functions.go` for per-function semantics;
`promql/promqltest/testdata/functions.test` for behavioral
fixtures.

## Execution Log

### 2026-05-14

- SOW drafted, promoted, implemented single chunk.
- IR: `FuncKind` gained 27 variants; new `Plan::LabelOp` (with
  `LabelOpKind::{Replace, Join}`) and `Plan::Absent` variants
  for the surfaces that needed pre-extracted args.
- Lowering: three new functions in `plan/lower.rs` --
  `lower_label_replace`, `lower_label_join`, `lower_absent` --
  plus `extract_absent_labels` to copy static `=` matchers
  onto the synthetic absent series.
- Evaluator: Groups A-E folded into `eval/functions.rs`
  (~500 lines added). New modules `eval/labelops.rs` and
  `eval/absent.rs` for Groups F and G. `apply_per_sample`
  helper for the simple per-element transforms; `map_samples`
  for closure-capturing variants. `time()` and `vector(s)`
  intercepted in `eval/dispatch.rs` so they can see
  `ctx.at_ms`.
- `return_type()` on `FuncKind` now branches: `Scalar` for
  `Time` and `Scalar`, `InstantVector` for everything else.
- Tests: 19 new lowering unit tests, 12 new function-eval
  unit tests, 7 new tests in `labelops.rs` and `absent.rs`.
  Total: 156/156 Rust unit (was 125; +31).
- Smoke: 22 new live-daemon checks under "Phase 3i: function
  omnibus" in `tests/promql-smoke/run-smoke.sh`. Total:
  117/117 (was 95; +22).
- Spec updated: function set documented in detail;
  out-of-scope list narrowed.
- Daemon rebuilt and restarted; the originally-broken
  `abs(rate(disk_io)) / ignoring(chart, family) abs(rate(disk_ops))`
  bytes-per-op query verified live.

## Validation

Acceptance criteria coverage:

1. All 27 functions parse and execute -- `lower_ok` tests on
   every name; `apply_call` dispatch arms wired.
2. Group A elementwise transforms -- `apply_per_sample` +
   `f64::abs`/etc.; `abs_strips_negatives` test; smoke
   check `"abs strips negative sign"`.
3. Group B clamp + round -- `expect_vec_then_two_scalars` /
   `expect_vec_then_scalar`; `clamp_bounds_in_both_directions`
   and `round_nearest_default_is_one` tests; smoke check
   `"clamp(_, 0, 100) keeps values in [0,100]"` etc.
4. `vector(s)` / `scalar(v)` -- dispatched in `dispatch.rs`
   with `ctx.at_ms` for `vector`; `scalar_returns_nan_for_multi_series`
   test; smoke check `"vector(42) returns a labelless series of value 42"`.
5. `sort` / `sort_desc` -- `sort_orders_ascending_nans_last`
   test; smoke check `"sort_desc(system_cpu) is monotonically non-increasing"`.
6. `absent` / `absent_over_time` -- `extract_absent_labels` in
   lowering; `eval/absent.rs` with 3 unit tests;
   `absent_extracts_static_eq_matchers` and
   `absent_over_complex_inner_has_empty_labels` lowering
   tests; live verification through smoke check
   `"absent of a non-existent metric returns synthesized labels"`.
7. `label_replace` / `label_join` -- `eval/labelops.rs` with
   4 unit tests; smoke check
   `"label_replace adds dev_short from regex capture"` plus
   `"label_join concatenates labels with separator"`. Regex
   validation lifted to lowering time so bad patterns fail
   fast.
8. `time()` / `timestamp(v)` -- dispatch.rs special-case;
   `timestamp_returns_sample_ts_in_seconds` test; smoke
   check `"time() returns a scalar"`.
9. `deriv` / `idelta` / `changes` / `resets` --
   `deriv_returns_slope_per_second`,
   `idelta_is_last_minus_previous`,
   `changes_counts_transitions`,
   `resets_counts_strict_decreases` tests; four smoke
   checks.
10. Name-stripping convention -- transforms drop, label
    munging keeps. Verified in tests and in smoke
    assertions that show `__name__` absent on transform
    outputs.
11. Unit tests: +31 across `plan/lower.rs`, `eval/functions.rs`,
    `eval/labelops.rs`, `eval/absent.rs`. Total 156/156.
12. Smoke checks: +22 in `tests/promql-smoke/run-smoke.sh`
    Phase 3i section. All pass against the live daemon.
13. Contract spec updated --
    `.agents/sow/specs/promql-endpoint-contract.md` documents
    every new function and removes them from the out-of-scope
    list; the remaining out-of-scope set narrows to trig,
    calendar, native-histogram, and the four newer-Prometheus
    niches.

Test posture: 156/156 Rust unit, 117/117 live smoke.

Live verification: the originally-broken bytes-per-op
expression `abs(rate(disk_io{device_type="physical"}[1m])) /
ignoring(chart, family)
abs(rate(disk_ops{device_type="physical"}[1m]))` now returns
reads=15.36 KiB/op, writes=96.45 KiB/op against nvme0n1 --
positive, sensible values that match disk request-size
expectations.

Reviewer findings: none yet (branch stays local per
`feedback_no_push_without_instruction`).

Same-failure search: no other paths in the codebase
referenced the missing function names. The Phase 3e
function omnibus (SOW-0023) followed the same shape as
this work and remains intact.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Spec: updated.
- End-user docs / skills: no change (PromQL endpoint still
  phase-gated and not yet announced).
- SOW lifecycle: pending -> current -> done; `completed`.

## Outcome

The 27-function omnibus closes the everyday-function gap.
The motivating bytes-per-op query is now correct end to
end. The IR gained two new variants but no new dispatch
shape -- everything routes through the existing `Plan::Call`
or the two purpose-built variants. The evaluator's helper
shape (`apply_per_sample`, `map_samples`, the
`expect_*_then_*` family) carries the bulk of the new
functions in a uniform style.

## Lessons

- `label_replace` was the only function with non-trivial
  semantics. The regex must be anchored and the
  replacement supports both `$N` and `${name}`. The
  capture-group expander needed care to handle multi-digit
  references and `$$` escapes correctly.
- `absent` cleanly demonstrates the "pre-extract static
  matchers at lowering" pattern that may be useful for
  other future surfaces (e.g., a hypothetical
  `info(metric)` join).
- The promql-parser pre-rejects many bad argument shapes
  before our lowering has a chance to. Defense-in-depth
  type checks remain useful but should not be tested
  directly through the parser; the unit-test path is to
  construct synthetic Plan nodes and call the evaluator.

## Followup

- Tier 3 / trig / calendar functions remain deferred. A
  small follow-up SOW could ship them if user need
  emerges.
- The `url_decode_r` newline-rejection bug discovered in
  the same session is a separate issue and remains
  unfixed in this branch. A future SOW (with its own
  regression-flow handling on SOW-0018 if the user
  prefers) can address it.
- CI verification (gcc-build, clang-build, license check)
  on push remains gated on user authorization.

## Regression Log

None yet.
