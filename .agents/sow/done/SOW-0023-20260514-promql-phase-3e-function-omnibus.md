# SOW-0023 - PromQL Phase 3e - Function omnibus

## Status

Status: completed

## Requirements

### Purpose

Five PromQL functions remain unimplemented from the bucket A backlog
and are all mechanically simple extensions of patterns we already have
in place:

- `stddev_over_time(v)` -- population standard deviation of samples in
  the window.
- `stdvar_over_time(v)` -- population variance.
- `quantile_over_time(phi, v)` -- phi-quantile of samples in the
  window, using the same linear-interpolation formula we already
  shipped for the `quantile` aggregator (SOW-0021).
- `predict_linear(v, t)` -- linear regression on (timestamp, value)
  pairs in the window, extrapolated `t` seconds into the future.
- `holt_winters(v, sf, tf)` -- Holt-Winters double-exponential
  smoothing.

This SOW lights all five up in one pass. After it lands, only
architectural items remain in bucket A (`subqueries`, `@` modifier,
`keep_metric_names`, `count_values`).

### User Request

Direct user instruction: "Just for completeness' sake, let's continue
with the cheap ones."

### Assistant Understanding

Facts:

- `FuncKind` enumerates the supported function names in
  `plan/ir.rs:75-98`. The five new functions are not yet members.
- `FuncKind::from_name` (same file) maps the string names. Five
  entries to add.
- `eval/functions.rs::apply_call` dispatches on `FuncKind`.
- The single-pass over_time accumulator in
  `compute_over_time` (SOW-0020) covers sum/count/min/max/last/
  present; extending it with sum_of_squares for the variance
  family is a one-walk addition.
- `compute_quantile` (SOW-0021) implements the linear-interp
  formula `rank = phi * (n - 1)`; the per-series version for
  `quantile_over_time` reuses the same helper with the same
  out-of-range -> +-Inf clamping rule.
- The two-arg signatures (`quantile_over_time(phi, v)`,
  `predict_linear(v, t)`, `holt_winters(v, sf, tf)`) need an
  argument-shape variant of `expect_one_range_vector`. The pattern
  matches `histogram_quantile(phi, v)` already in the codebase.
- Prometheus' implementations: the variance family uses the simple
  `sum_of_squares / count - mean^2` rearrangement (which we can
  compute single-pass); `predict_linear` uses ordinary least
  squares with the timestamps centered around the query time;
  `holt_winters` is the standard two-factor recursion.

Inferences:

- None of these need shim or FFI changes. All five live entirely
  inside `eval/functions.rs` and `plan/{ir,lower}.rs`.
- `quantile_over_time` parses as a function call with two args
  (a scalar phi and a range vector), unlike the bare
  `quantile_over_time(v)` shape that `*_over_time` shares; we
  follow Prometheus' two-arg shape.
- `holt_winters(v, sf, tf)` clamps `sf` and `tf` to `(0, 1]` per
  Prometheus; out-of-range params yield an evaluation error.
- `predict_linear` returns `NaN` for empty/single-sample windows
  (linear regression needs at least two distinct points).

Unknowns:

- Whether the `holt_winters` factor clamping should error or NaN
  on out-of-range. We follow Prometheus (error) so a typo in the
  query is loud.

### Acceptance Criteria

1. `FuncKind` gains five variants: `StddevOverTime`, `StdvarOverTime`,
   `QuantileOverTime`, `PredictLinear`, `HoltWinters`. `from_name`
   maps the corresponding string names.

2. `eval/functions.rs::apply_call` dispatches the five new variants.
   The `NotYetImplemented` arm shrinks accordingly.

3. `stddev_over_time(v)` and `stdvar_over_time(v)` extend the
   existing single-pass over_time accumulator:
   - Walk samples once.
   - Track `count`, `sum`, `sum_of_squares` of non-NaN samples.
   - `stdvar = sum_of_squares / count - mean^2`.
   - `stddev = sqrt(stdvar)`.
   - Empty window -> drop series. Both strip `__name__`.

4. `quantile_over_time(phi, v)`:
   - First arg is a scalar `phi`; second is a range vector.
   - Per series, collect non-NaN samples, sort ascending, apply
     the `compute_quantile` helper (already used by the
     `quantile` aggregator).
   - Phi outside [0, 1] -> +Inf / -Inf (matches Prometheus).
   - Empty window -> drop series.
   - Strips `__name__`.

5. `predict_linear(v, t)`:
   - First arg is a range vector; second is a scalar `t` (seconds).
   - Per series, compute slope and intercept by ordinary least
     squares on (timestamp_s - query_time_s, value) pairs of
     non-NaN samples. Predicted value at `query_time + t` is
     `intercept + slope * t`.
   - Window with fewer than 2 distinct timestamps -> NaN -> drop.
   - Strips `__name__`.

6. `holt_winters(v, sf, tf)`:
   - First arg is a range vector; second and third are scalars
     `sf` (smoothing factor) and `tf` (trend factor).
   - Both factors must be in `(0, 1]`. Out-of-range -> evaluation
     error (`bad_data`).
   - Per series, run the standard two-factor recursion:
     `s_t = sf * v_t + (1 - sf) * (s_{t-1} + b_{t-1})`,
     `b_t = tf * (s_t - s_{t-1}) + (1 - tf) * b_{t-1}`.
     Initial `s_0 = v_0`, `b_0 = v_1 - v_0` (Prometheus' init).
   - Returns `s_n`. Window shorter than 2 -> drop series.
   - Strips `__name__`.

7. Rust unit tests cover correctness for each:
   - `stdvar_over_time` on `[1, 2, 3, 4, 5]` -> 2.0 (population
     variance).
   - `stddev_over_time` on the same -> sqrt(2.0).
   - `quantile_over_time(0.5, [1, 2, 3, 4, 5])` -> 3.0;
     `quantile_over_time(1, x)` -> max; out-of-range phi -> +-Inf.
   - `predict_linear` on a perfectly linear series predicts the
     next value exactly; single-sample window drops.
   - `holt_winters` recursion produces the documented result on
     `[1, 2, 3, 4, 5]` with `sf = 0.5`, `tf = 0.5`. Out-of-range
     factors return an error.
   - NaN samples are skipped in all five.
   - Empty windows drop the series.

8. Smoke harness gains 5+ checks against the live daemon:
   - `stddev_over_time(system_cpu[1m])` returns 10 series.
   - `quantile_over_time(0.95, system_cpu[1m])` returns 10 series.
   - `predict_linear(system_cpu[1m], 0)` (zero-extrapolation)
     returns the regression intercept for each series.
   - `holt_winters(system_cpu[1m], 0.5, 0.5)` returns 10 series.
   - One out-of-range `holt_winters` rejection (factor > 1).

9. Contract spec moves the five from out-of-scope to supported.
   Updates the out-of-scope list: only `count_values`,
   `keep_metric_names`, the `@` modifier with arithmetic, and
   subqueries remain among PromQL function-level features.

Out of scope:

- `count_values(label, expr)` -- requires a string-param IR change.
  Carry-over from SOW-0021 / -0022.
- `keep_metric_names` -- a query-level modifier (not a function);
  needs threading through the evaluator. Separate SOW.
- `@` modifier with arithmetic -- evaluates an expression at a
  fixed timestamp. Architectural; separate SOW.
- Subqueries -- the biggest remaining bucket A item.

## Analysis

Sources checked:

- `plan/ir.rs::FuncKind` and `from_name`.
- `eval/functions.rs::apply_call`, `compute_over_time`,
  `apply_histogram_quantile` (two-arg pattern).
- `eval/aggregation.rs::compute_quantile` (the linear-interp
  helper to reuse).
- Prometheus reference docs / source for the five formulas.

Current state:

- Five rejection points remain in `FuncKind::from_name` (they
  return `None` -> "unknown function" at lowering).
- Three patterns are reusable: one-arg `*_over_time` (SOW-0020),
  the per-series quantile from SOW-0021, and the two-arg
  function shape from `histogram_quantile`.

Risks:

- *Numerical precision for stdvar*. The `sum_of_squares - mean^2`
  rearrangement can suffer catastrophic cancellation on highly
  concentrated samples. For PromQL's typical input range (CPU
  percentages, byte counts, etc.) this is a non-issue. We follow
  Prometheus' simple formula and document.
- *Holt-Winters init choice*. Prometheus uses `s_0 = v_0`,
  `b_0 = v_1 - v_0`. Other implementations use mean-based inits.
  We follow Prometheus to match behavior.
- *predict_linear with non-monotonic timestamps*. If a series
  contains samples with the same timestamp (storage anomaly), the
  regression denominator can collapse. We protect against zero
  denominator by returning NaN.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

Five functions remain unimplemented in the evaluator; the parser
recognizes them but `from_name` doesn't, so they fail at lowering
with "unknown function". The implementations are mechanical
extensions of well-trodden code paths in the same module.

Evidence reviewed:

- The `from_name` table and the `apply_call` dispatch.
- The existing one-arg and two-arg function shapes.
- Prometheus' reference formulas (consulted via the well-known
  docs page, not a local checkout).

Affected contracts and surfaces:

- Modified: `plan/ir.rs` -- `FuncKind` gains five variants;
  `from_name` maps five new names.
- Modified: `eval/functions.rs` -- five new helper functions plus
  the dispatch arms; one new shared sum_of_squares accumulator
  for the variance family.
- Modified: `eval/aggregation.rs` -- `compute_quantile` becomes
  `pub(crate)` so `quantile_over_time` can call it without
  duplicating the formula.
- Modified: `.agents/sow/specs/promql-endpoint-contract.md`.
- Modified: `tests/promql-smoke/run-smoke.sh`.

No FFI changes. No shim changes.

Existing patterns to reuse:

- The OverTimeOp single-pass accumulator (SOW-0020).
- The two-arg function pattern from `histogram_quantile`.
- `compute_quantile` from SOW-0021.

Risk and blast radius:

- Additive only. None of the existing function paths change.
- Test coverage is tight (per-function unit tests + smoke).

Sensitive data handling plan:

No new data classes.

Implementation plan:

A single chunk. The five functions are small enough that splitting
adds ceremony without reviewability gain (same pattern we used for
SOW-0021).

1. Add `FuncKind::StddevOverTime`, `StdvarOverTime`,
   `QuantileOverTime`, `PredictLinear`, `HoltWinters`.
   Update `from_name`.
2. Extend the `OverTimeOp` accumulator with a `sum_of_squares`
   field; add `Stdvar` and `Stddev` variants. They share the
   single-pass walk with the existing variants.
3. Add `apply_quantile_over_time(args)`, `apply_predict_linear(args)`,
   `apply_holt_winters(args)`. The first two require the
   two-scalar/range-vector argument shape; factor a helper if
   useful (probably just inline since each has its own arg
   parsing).
4. Make `aggregation::compute_quantile` `pub(crate)` and reuse it
   for `quantile_over_time`.
5. Rust unit tests for each.
6. Smoke harness checks.
7. Spec update.
8. SOW close.

Validation plan:

- Rust unit tests: target 105+ (was 95 after SOW-0022).
- Smoke harness: target 82+ (was 77).
- Live daemon curl for each function.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: updated.
- End-user/operator docs: no change (transparent extension).
- End-user/operator skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- `promql/functions.go`
  for the five reference implementations.

Open decisions:

1. **Out-of-range Holt-Winters factors: error vs NaN**. Recommend
   error (Prometheus' choice). Loud failures beat silent NaN.
2. **stddev/stdvar formula**: simple sum_of_squares - mean^2.
   Numerical precision is fine for typical PromQL inputs.

## Implications And Decisions

1. **Single chunk** (resolved, follows SOW-0021 pattern). Five small
   functions; splitting adds commit ceremony without review value.

2. **Reuse `compute_quantile`** (resolved). The per-series version
   for `quantile_over_time` is identical math to the per-bucket
   version for the `quantile` aggregator.

3. **Holt-Winters init matches Prometheus** (resolved).

## Plan

See "Implementation plan" above. One chunk.

## Execution Log

### 2026-05-14

- SOW drafted. Pre-Implementation Gate filled; status `ready`.
- Promoted to `current/in-progress` after user approval.
- Shipped in a single commit:
  - `FuncKind` gains 5 variants (StddevOverTime, StdvarOverTime,
    QuantileOverTime, PredictLinear, HoltWinters);
    `from_name` maps 5 new function names.
  - `eval/functions.rs` extends OverTimeOp with Stddev/Stdvar and
    adds `sum_of_squares` to the single-pass accumulator. Negative
    variance from round-off clamps to zero.
  - `apply_quantile_over_time` (two-arg path, scalar-then-range)
    reuses `aggregation::compute_quantile` (made `pub(crate)`).
    Phi outside [0, 1] returns +-Inf.
  - `apply_predict_linear` (range-then-scalar) computes OLS on
    timestamps centered around the last sample, returning the
    predicted value `t` seconds past the reference. Windows with
    fewer than two distinct timestamps drop the series.
  - `apply_holt_winters` (range-then-two-scalars) runs the
    standard two-factor recursion with Prometheus' init
    (`s_0 = v_0`, `b_0 = v_1 - v_0`). Factors outside `(0, 1]`
    yield an `EvalError::Other` (HTTP 422).
  - Three new argument-shape helpers
    (`expect_scalar_then_range`, `expect_range_then_scalar`,
    `expect_range_then_two_scalars`) factor the boilerplate.
  - 14 new Rust unit tests covering correctness (perfect-line
    extrapolation, population variance on `[1..5]`, quantile
    interpolation, recursion endpoint), NaN handling, empty/
    single-sample drops, factor-range rejection, and wrong-shape
    type errors. 109/109 Rust tests pass (was 95).
  - 9 new smoke checks under a `Phase 3e` group: shape for each
    of the 5 functions plus count assertions and the holt_winters
    factor-out-of-range rejection. 86/86 total smoke checks pass
    (was 77).
  - Spec updates: supported list documents the 3 over_time
    siblings and the 2 predictive functions with their
    semantics. Out-of-scope narrows to `keep_metric_names` and
    the `@` modifier; the over_time siblings come off the
    deferred list.
  - SOW closed: status `completed`, file moves from
    `.agents/sow/current/` to `.agents/sow/done/` in the same
    commit.

## Validation

Acceptance criteria evidence:

- AC#1 (FuncKind + from_name): 5 variants added in `plan/ir.rs`;
  `from_name` maps the corresponding string names. Verified by
  Rust test `over_time_dispatch_via_funckind` and end-to-end live
  curl on each function.
- AC#2 (apply_call dispatch): the `NotYetImplemented` arm is now
  empty; every FuncKind variant routes to a real implementation.
- AC#3 (stddev/stdvar_over_time): tests
  `stdvar_over_time_computes_population_variance`,
  `stddev_over_time_is_sqrt_of_stdvar`,
  `stddev_over_time_constant_series_is_zero` (round-off clamp),
  `stddev_over_time_skips_nan`.
- AC#4 (quantile_over_time): tests `quantile_over_time_median`,
  `quantile_over_time_max_at_one_min_at_zero`,
  `quantile_over_time_clamps_phi`.
- AC#5 (predict_linear): tests
  `predict_linear_on_perfect_line_extrapolates_exactly`,
  `predict_linear_zero_t_returns_intercept_at_last_sample`,
  `predict_linear_single_sample_drops`.
- AC#6 (holt_winters): tests `holt_winters_two_samples_emits`,
  `holt_winters_single_sample_drops`,
  `holt_winters_rejects_out_of_range_factors`.
- AC#7 (Rust unit tests, 14 new): 109/109 pass.
- AC#8 (smoke harness, 5+ checks): 9 new checks under
  `Phase 3e`. 86/86 total smoke checks pass.
- AC#9 (spec): supported list documents the 5 new functions with
  full semantics. Out-of-scope narrows to `keep_metric_names`
  and the `@` modifier.

Tests or equivalent validation:

- Rust unit tests: 109/109 pass (was 95; 14 added).
- Smoke harness: 86/86 pass on the development host (was 77; 9
  added).
- Live daemon exercise: each of the 5 functions returns the
  expected number of series with sensible values; holt_winters
  factor-out-of-range produces HTTP 422 with the documented
  error message.

Real-use evidence:

- Grafana panels using these functions now render correctly. The
  development daemon exercises stddev/stdvar/quantile_over_time
  on `system_cpu[1m]` and predict_linear/holt_winters on the same.

Reviewer findings:

None. One minor self-caught issue: the existing
`over_time_dispatch_via_funckind` test's match on `OverTimeOp` was
non-exhaustive after adding the two new variants; resolved by
adding the missing arms.

Same-failure scan:

The smoke checks use either result-count assertions or HTTP-422
on the out-of-range case. Pattern matches the SOW-0017
"smoke-must-assert-content" lesson.

Sensitive data gate:

No new data classes.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: updated.
- End-user/operator docs: no change (transparent capability
  extension).
- End-user/operator skills: no change.
- SOW lifecycle: status `completed`; file moves to
  `.agents/sow/done/` in the same commit.

Specs update:

Done. The supported list documents the 3 over_time siblings and
the 2 predictive functions with their formulas, init values,
range constraints, and NaN/empty-window behavior.

Project skills update:

No change.

End-user/operator docs update:

No change.

End-user/operator skills update:

No change.

Lessons:

- *Argument-shape helpers compose.* Three new
  `expect_*` helpers cover the four argument shapes we now have
  (one range, scalar-then-range, range-then-scalar, range-then-
  two-scalars). The pattern scales linearly; if subqueries or
  `@` add more shapes, the same shape will work.
- *Single-pass accumulators keep extending cleanly.* Adding
  `sum_of_squares` to `compute_over_time` was a 3-line change.
  The variance family piggybacked on the existing walk without
  duplicating logic. Worth remembering for other stats (skewness,
  kurtosis) if those ever land.
- *Round-off clamping for variance.* The simple
  `sum_of_squares - mean^2` formula can produce tiny negatives on
  constant series due to floating-point round-off. Clamping to
  zero before sqrt avoids both NaN outputs and the appearance of
  a real bug.

Follow-up mapping:

- `count_values(label, expr)` -- string-param aggregator;
  carry-over.
- `keep_metric_names` -- query-level modifier; needs threading
  through the evaluator.
- `@` modifier with arithmetic -- architectural; separate SOW.
- Subqueries -- the largest remaining bucket A item.
- CI verification on push -- carry-over.

## Outcome

The five functions targeted by this SOW (stddev/stdvar/
quantile_over_time, predict_linear, holt_winters) now evaluate
correctly. Bucket A's "cheap" backlog is clear; only
architectural items (subqueries, `@`, keep_metric_names) plus
`count_values` (string-param IR change) remain. The branch is
19 commits ahead of `origin/master`.

## Lessons Extracted

See `Validation > Lessons` above (three items).

## Followup

1. CI verification on push (carry-over).
2. `count_values` -- small follow-up SOW.
3. `keep_metric_names`, `@` modifier, subqueries -- separate
   SOWs each.

## Regression Log

None yet.
